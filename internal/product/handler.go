// Package product 负责产品资料接口。
package product

import (
	"errors"
	"net/http"
	"strings"
	"time"

	erpmiddleware "bb_erp_echo/internal/middleware"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	"gorm.io/gorm"
)

// Handler 处理产品资料。
type Handler struct {
	DB          *gorm.DB
	StorageRoot string
}

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

type productInput struct {
	ProductModel  string `json:"product_model" validate:"required,max=160"`
	CustomerModel string `json:"customer_model" validate:"max=160"`
	Material      string `json:"material" validate:"max=120"`
	InkRequired   bool   `json:"ink_required"`
	Status        string `json:"status" validate:"omitempty,oneof=active disabled"`
}

type ProductResponse struct {
	model.Product
	MoldCount int64 `json:"mold_count"`
}

type ProductPageResponse struct {
	Items    []ProductResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Keyword  string            `json:"keyword,omitempty"`
}

// NewHandler 创建产品模块接口处理器。
func NewHandler(db *gorm.DB) *Handler { return &Handler{DB: db} }

// NewHandlerWithStorage 创建带文件目录的产品模块接口处理器。
func NewHandlerWithStorage(db *gorm.DB, storageRoot string) *Handler {
	return &Handler{DB: db, StorageRoot: storageRoot}
}

// RegisterRoutes 注册产品资料路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/products", audit)
	group.GET("", h.ListProducts, require("/api/v1/products", "read"))
	group.POST("", h.CreateProduct, require("/api/v1/products", "write"))
	group.GET("/import-template", h.ImportTemplate, require("/api/v1/products/import", "import"))
	group.POST("/import/preview", h.ImportPreview, require("/api/v1/products/import", "import"), echomiddleware.BodyLimit(MaxProductPackageSize+32<<20), erpmiddleware.TransferDeadline(2*time.Hour), echomiddleware.ContextTimeout(2*time.Hour))
	group.POST("/import/commit", h.ImportCommit, require("/api/v1/products/import", "import"), echomiddleware.BodyLimit(MaxProductPackageSize+32<<20), erpmiddleware.TransferDeadline(2*time.Hour), echomiddleware.ContextTimeout(2*time.Hour))
	group.GET("/export", h.Export, require("/api/v1/products", "read"))
	group.GET("/:id/molds", h.ListProductMolds, require("/api/v1/products", "read"), require("/api/v1/molds", "read"))
	group.GET("/:id", h.GetProduct, require("/api/v1/products", "read"))
	group.PATCH("/:id", h.UpdateProduct, require("/api/v1/products", "write"))
	group.DELETE("/:id", h.DeleteProduct, require("/api/v1/products", "write"))
}

// ListProducts 查询产品资料。
// @Summary 查询产品资料
// @Tags product
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "产品型号、客户型号或材料"
// @Param material query string false "产品材料"
// @Param ink_required query bool false "是否刷墨"
// @Param status query string false "状态"
// @Success 200 {object} ProductPageResponse
// @Router /api/v1/products [get]
func (h *Handler) ListProducts(c *echo.Context) error {
	query := h.DB.Model(&model.Product{})
	if keyword := strings.TrimSpace(c.QueryParam("q")); keyword != "" {
		query = pagination.ApplyKeyword(query, keyword, "product_model", "customer_model", "material")
	}
	if material := strings.TrimSpace(c.QueryParam("material")); material != "" {
		query = query.Where("material = ?", material)
	}
	if ink := strings.TrimSpace(c.QueryParam("ink_required")); ink != "" {
		query = query.Where("ink_required = ?", ink == "true" || ink == "1")
	}
	if status := strings.TrimSpace(c.QueryParam("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	result, err := pagination.Page[model.Product](query, pagination.FromEcho(c), "id desc", nil)
	if err != nil {
		return err
	}
	items := make([]ProductResponse, 0, len(result.Items))
	ids := make([]uint, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, ProductResponse{Product: item})
		ids = append(ids, item.ID)
	}
	if len(ids) > 0 {
		type countRow struct {
			ProductID uint
			Count     int64
		}
		var counts []countRow
		if err := h.DB.Model(&model.Mold{}).Select("product_id, count(*) as count").Where("product_id IN ?", ids).Group("product_id").Scan(&counts).Error; err != nil {
			return err
		}
		byID := make(map[uint]int64, len(counts))
		for _, row := range counts {
			byID[row.ProductID] = row.Count
		}
		for i := range items {
			items[i].MoldCount = byID[items[i].ID]
		}
	}
	return c.JSON(http.StatusOK, ProductPageResponse{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, Keyword: result.Keyword})
}

// GetProduct 查询产品详情。
// @Summary 查询产品详情
// @Tags product
// @Security BearerAuth
// @Produce json
// @Param id path int true "产品 ID"
// @Success 200 {object} ProductResponse
// @Router /api/v1/products/{id} [get]
func (h *Handler) GetProduct(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.Product
	if err := h.DB.First(&item, id).Error; err != nil {
		return productHTTPError(err)
	}
	var count int64
	if err := h.DB.Model(&model.Mold{}).Where("product_id = ?", item.ID).Count(&count).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, ProductResponse{Product: item, MoldCount: count})
}

// CreateProduct 创建产品资料。
// @Summary 创建产品资料
// @Tags product
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body productInput true "产品资料"
// @Success 201 {object} model.Product
// @Router /api/v1/products [post]
func (h *Handler) CreateProduct(c *echo.Context) error {
	var input productInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	item, err := normalizeProductInput(input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.DB.Create(&item).Error; err != nil {
		return productHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

// UpdateProduct 更新产品资料。
// @Summary 更新产品资料
// @Tags product
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "产品 ID"
// @Param body body productInput true "产品资料"
// @Success 200 {object} model.Product
// @Router /api/v1/products/{id} [patch]
func (h *Handler) UpdateProduct(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var input productInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	normalized, err := normalizeProductInput(input)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var item model.Product
	if err := h.DB.First(&item, id).Error; err != nil {
		return productHTTPError(err)
	}
	item.ProductModel = normalized.ProductModel
	item.CustomerModel = normalized.CustomerModel
	item.Material = normalized.Material
	item.InkRequired = normalized.InkRequired
	item.Status = normalized.Status
	if err := h.DB.Save(&item).Error; err != nil {
		return productHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteProduct 删除未被业务引用的产品资料。
// @Summary 删除产品资料
// @Tags product
// @Security BearerAuth
// @Param id path int true "产品 ID"
// @Success 204
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/products/{id} [delete]
func (h *Handler) DeleteProduct(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.Product
	if err := h.DB.First(&item, id).Error; err != nil {
		return productHTTPError(err)
	}
	checks := []struct {
		name  string
		model any
		query string
	}{
		{"模具", &model.Mold{}, "product_id = ?"},
		{"库存余额", &model.InventoryBalance{}, "item_type = 'product' AND item_id = ?"},
		{"库存流水", &model.InventoryLedger{}, "item_type = 'product' AND item_id = ?"},
		{"任务单", &model.WorkOrder{}, "product_id = ?"},
	}
	for _, check := range checks {
		var count int64
		if err := h.DB.Model(check.model).Where(check.query, id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return echo.NewHTTPError(http.StatusConflict, "产品型号仍被"+check.name+"引用，不能删除；请先停用或清理关联数据")
		}
	}
	unlock := lockProductAssetMutation()
	defer unlock()
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		var images []model.ImageFile
		if err := tx.Where("owner_type = ? AND owner_id = ?", "product", id).Find(&images).Error; err != nil {
			return err
		}
		paths := imagePaths(images)
		if err := tx.Unscoped().Where("owner_type = ? AND owner_id = ?", "product", id).Delete(&model.ImageFile{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&item).Error; err != nil {
			return err
		}
		return queueImageCleanup(tx, paths)
	}); err != nil {
		return productHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListProductMolds 查询产品关联模具。
// @Summary 查询产品关联模具
// @Tags product
// @Security BearerAuth
// @Produce json
// @Param id path int true "产品 ID"
// @Success 200 {array} model.Mold
// @Router /api/v1/products/{id}/molds [get]
func (h *Handler) ListProductMolds(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var product model.Product
	if err := h.DB.First(&product, id).Error; err != nil {
		return productHTTPError(err)
	}
	var items []model.Mold
	if err := h.DB.Preload("Product").Preload("Location").Where("product_id = ?", id).Order("mold_type asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func normalizeProductInput(input productInput) (model.Product, error) {
	input.ProductModel = strings.TrimSpace(input.ProductModel)
	input.CustomerModel = strings.TrimSpace(input.CustomerModel)
	input.Material = strings.TrimSpace(input.Material)
	if input.ProductModel == "" {
		return model.Product{}, errors.New("产品型号不能为空")
	}
	if strings.ContainsAny(input.ProductModel, `/\\`) {
		return model.Product{}, errors.New("产品型号不能包含 / 或 \\")
	}
	if input.Status == "" {
		input.Status = model.StatusActive
	}
	return model.Product{ProductModel: input.ProductModel, CustomerModel: input.CustomerModel, Material: input.Material, InkRequired: input.InkRequired, Status: input.Status}, nil
}

func productHTTPError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "产品资料不存在")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		return echo.NewHTTPError(http.StatusConflict, "产品型号已存在")
	}
	return err
}
