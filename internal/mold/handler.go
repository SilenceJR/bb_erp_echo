// Package mold 负责产品资料下的模具档案、位置、图片和图纸接口。
package mold

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	erpmiddleware "bb_erp_echo/internal/middleware"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	"gorm.io/gorm"
)

type Handler struct {
	Service     *gormService
	DB          *gorm.DB
	StorageRoot string
}

type ErrorResponse = response.ErrorBody

func NewHandler(db *gorm.DB) *Handler { return NewHandlerWithStorage(db, "") }
func NewHandlerWithStorage(db *gorm.DB, storageRoot string) *Handler {
	return &Handler{Service: NewServiceWithStorage(db, storageRoot), DB: db, StorageRoot: storageRoot}
}

func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/molds", audit)
	group.GET("", h.ListMolds, require("/api/v1/molds", "read"))
	group.GET("/:id/drawings", h.ListDrawings, require("/api/v1/molds", "read"))
	group.POST("/:id/drawings", h.UploadDrawing, require("/api/v1/molds", "write"), echomiddleware.BodyLimit(MaxDrawingSize+32<<20), erpmiddleware.TransferDeadline(2*time.Hour), echomiddleware.ContextTimeout(2*time.Hour))
	group.GET("/:id/drawings/:drawing_id/content", h.DrawingContent, require("/api/v1/molds", "read"))
	group.DELETE("/:id/drawings/:drawing_id", h.DeleteDrawing, require("/api/v1/molds", "write"))
	group.GET("/export", h.Export, require("/api/v1/molds", "read"))
	group.GET("/import-template", h.ImportTemplate, require("/api/v1/molds/import", "import"))
	group.POST("/import/preview", h.ImportPreview, require("/api/v1/molds/import", "import"), echomiddleware.BodyLimit(MaxPackageSize+32<<20), erpmiddleware.TransferDeadline(2*time.Hour), echomiddleware.ContextTimeout(2*time.Hour))
	group.POST("/import/commit", h.ImportCommit, require("/api/v1/molds/import", "import"), echomiddleware.BodyLimit(MaxPackageSize+32<<20), erpmiddleware.TransferDeadline(2*time.Hour), echomiddleware.ContextTimeout(2*time.Hour))
	group.GET("/:id", h.GetMold, require("/api/v1/molds", "read"))
	group.POST("", h.CreateMold, require("/api/v1/molds", "write"))
	group.PATCH("/:id", h.UpdateMold, require("/api/v1/molds", "write"))
	group.DELETE("/:id", h.DeleteMold, require("/api/v1/molds", "write"))
	group.POST("/bulk-location", h.BulkMove, require("/api/v1/molds", "write"))

	locations := v1.Group("/mold-locations", audit)
	locations.GET("", h.ListLocations, require("/api/v1/molds", "read"))
	locations.POST("", h.CreateLocation, require("/api/v1/molds", "write"))
	locations.POST("/bulk", h.BulkCreateLocations, require("/api/v1/molds", "write"))
	locations.PATCH("/:id", h.UpdateLocation, require("/api/v1/molds", "write"))
}

// ListMolds 查询模具列表。
// @Summary 查询模具
// @Tags mold
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "产品型号、共模组号或备注"
// @Param product_model query string false "产品型号"
// @Param mold_type query string false "single 或 common"
// @Param location_id query int false "位置 ID"
// @Param common_group_no query string false "共模组号"
// @Success 200 {object} MoldPageResponse
// @Router /api/v1/molds [get]
func (h *Handler) ListMolds(c *echo.Context) error {
	locationID, err := strconv.ParseUint(c.QueryParam("location_id"), 10, 64)
	if c.QueryParam("location_id") != "" && err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "位置 ID 无效")
	}
	result, err := h.Service.List(pagination.FromEcho(c), ListFilter{
		ProductModel: c.QueryParam("product_model"),
		Type:         c.QueryParam("mold_type"),
		LocationID:   uint(locationID),
		GroupNo:      c.QueryParam("common_group_no"),
	})
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, result)
}

// GetMold 查询模具详情。
// @Summary 查询模具详情
// @Tags mold
// @Security BearerAuth
// @Produce json
// @Param id path int true "模具 ID"
// @Success 200 {object} MoldResponse
// @Router /api/v1/molds/{id} [get]
func (h *Handler) GetMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.Get(id)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// CreateMold 创建模具档案。
// @Summary 创建模具档案
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "模具档案参数"
// @Success 201 {object} model.Mold
// @Router /api/v1/molds [post]
func (h *Handler) CreateMold(c *echo.Context) error {
	var input Input
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	item, err := h.Service.Create(input)
	if err != nil {
		return moldHTTPError(err)
	}
	response, err := h.Service.Get(item.ID)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusCreated, response)
}

// UpdateMold 更新模具档案。
// @Summary 更新模具档案
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "模具 ID"
// @Param body body Input true "模具档案参数"
// @Success 200 {object} model.Mold
// @Router /api/v1/molds/{id} [patch]
func (h *Handler) UpdateMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var input Input
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	if _, err := h.Service.Update(id, input); err != nil {
		return moldHTTPError(err)
	}
	item, err := h.Service.Get(id)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteMold 物理删除模具及其图片和图纸。
// @Summary 删除模具档案
// @Tags mold
// @Security BearerAuth
// @Param id path int true "模具 ID"
// @Success 204
// @Router /api/v1/molds/{id} [delete]
func (h *Handler) DeleteMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	if err := h.Service.Delete(id); err != nil {
		return moldHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// BulkMove 批量移动模具到固定位置。
// @Summary 批量移动模具
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Param body body BulkMoveInput true "模具 ID 和目标位置"
// @Success 204
// @Router /api/v1/molds/bulk-location [post]
func (h *Handler) BulkMove(c *echo.Context) error {
	var input BulkMoveInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	if err := h.Service.BulkMove(input); err != nil {
		return moldHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListLocations 查询固定位置字典。
// @Summary 查询模具位置
// @Tags mold
// @Security BearerAuth
// @Produce json
// @Param include_disabled query bool false "是否包含停用位置"
// @Success 200 {array} model.MoldLocation
// @Router /api/v1/mold-locations [get]
func (h *Handler) ListLocations(c *echo.Context) error {
	items, err := h.Service.Locations(c.QueryParam("include_disabled") == "true")
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateLocation 新增固定位置。
// @Summary 新增模具位置
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body LocationInput true "位置编码"
// @Success 201 {object} model.MoldLocation
// @Router /api/v1/mold-locations [post]
func (h *Handler) CreateLocation(c *echo.Context) error {
	var input LocationInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	item, err := h.Service.CreateLocation(input)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

// BulkCreateLocations 批量补充货架位置。
// @Summary 批量新增模具位置
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body BulkLocationInput true "批量位置参数"
// @Success 201 {object} BulkLocationResult
// @Router /api/v1/mold-locations/bulk [post]
func (h *Handler) BulkCreateLocations(c *echo.Context) error {
	var input BulkLocationInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	result, err := h.Service.BulkCreateLocations(input)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusCreated, result)
}

// UpdateLocation 更新位置状态。
// @Summary 更新模具位置状态
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Param id path int true "位置 ID"
// @Param body body LocationStatusInput true "位置状态"
// @Success 200 {object} model.MoldLocation
// @Router /api/v1/mold-locations/{id} [patch]
func (h *Handler) UpdateLocation(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var input LocationStatusInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	item, err := h.Service.UpdateLocation(id, input)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

func moldHTTPError(err error) error {
	switch {
	case errors.Is(err, ErrMoldNotFound), errors.Is(err, ErrProductNotFound):
		return echo.NewHTTPError(http.StatusNotFound, publicMoldError(err))
	case errors.Is(err, ErrProductRequired), errors.Is(err, ErrProductDisabled), errors.Is(err, ErrMoldInvalidType),
		errors.Is(err, ErrMoldGroupRequired), errors.Is(err, ErrMoldGroupForbidden), errors.Is(err, ErrMoldLocationRequired),
		errors.Is(err, ErrMoldLocationNotFound), errors.Is(err, ErrMoldLocationDisabled), errors.Is(err, ErrMoldLocationZone),
		errors.Is(err, ErrMoldLocationRange):
		return echo.NewHTTPError(http.StatusBadRequest, publicMoldError(err))
	case errors.Is(err, ErrMoldLocationInUse):
		return echo.NewHTTPError(http.StatusConflict, publicMoldError(err))
	case errors.Is(err, ErrMoldSelectionRequired):
		return echo.NewHTTPError(http.StatusBadRequest, publicMoldError(err))
	default:
		return err
	}
}

func publicMoldError(err error) string {
	switch {
	case errors.Is(err, ErrMoldNotFound):
		return "模具不存在"
	case errors.Is(err, ErrProductRequired):
		return "请选择产品型号"
	case errors.Is(err, ErrProductNotFound):
		return "产品资料不存在"
	case errors.Is(err, ErrProductDisabled):
		return "产品资料已停用"
	case errors.Is(err, ErrMoldInvalidType):
		return "模具类型无效"
	case errors.Is(err, ErrMoldGroupRequired):
		return "共模必须填写共模组号"
	case errors.Is(err, ErrMoldGroupForbidden):
		return "单模不能填写共模组号"
	case errors.Is(err, ErrMoldLocationRequired):
		return "请选择模具位置"
	case errors.Is(err, ErrMoldLocationNotFound):
		return "模具位置不存在"
	case errors.Is(err, ErrMoldLocationDisabled):
		return "模具位置已停用"
	case errors.Is(err, ErrMoldLocationInUse):
		return "模具位置正在使用，不能停用"
	case errors.Is(err, ErrMoldSelectionRequired):
		return "请选择模具"
	case errors.Is(err, ErrMoldLocationZone):
		return "位置区名无效"
	case errors.Is(err, ErrMoldLocationRange):
		return "位置范围无效"
	default:
		return err.Error()
	}
}
