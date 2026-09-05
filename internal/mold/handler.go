// Package mold 负责模具产品档案、固定位置、图片和图纸资料。
package mold

import (
	"errors"
	"net/http"
	"strconv"
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

type Handler struct {
	Service     Service
	DB          *gorm.DB
	StorageRoot string
}
type ErrorResponse = response.ErrorBody
type moldModel = model.Mold

func NewHandler(db *gorm.DB) *Handler { return NewHandlerWithStorage(db, "") }
func NewHandlerWithStorage(db *gorm.DB, storageRoot string) *Handler {
	return &Handler{Service: NewServiceWithStorage(db, storageRoot), DB: db, StorageRoot: storageRoot}
}
func NewHandlerWithService(service Service) *Handler { return &Handler{Service: service} }

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
	locations.PATCH("/:id", h.UpdateLocation, require("/api/v1/molds", "write"))
}

// ListMolds 查询模具列表。
// @Summary 查询模具
// @Tags mold
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "编号、型号或备注"
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
	result, err := h.Service.List(pagination.FromEcho(c), ListFilter{Type: c.QueryParam("mold_type"), LocationID: uint(locationID), GroupNo: c.QueryParam("common_group_no")})
	if err != nil {
		return err
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
	return c.JSON(http.StatusCreated, item)
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
	item, err := h.Service.Update(id, input)
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteMold 物理删除模具及其图片、DWG 文件记录。
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

// UpdateLocation 启用或停用固定位置。
// @Summary 更新模具位置状态
// @Tags mold
// @Security BearerAuth
// @Accept json
// @Produce json
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
	case errors.Is(err, ErrMoldNotFound), errors.Is(err, ErrMoldLocationNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "模具或位置不存在")
	case errors.Is(err, ErrMoldNumberConflict):
		return echo.NewHTTPError(http.StatusConflict, "模具编号已存在")
	case errors.Is(err, ErrMoldInvalidType), errors.Is(err, ErrMoldGroupRequired), errors.Is(err, ErrMoldGroupForbidden), errors.Is(err, ErrMoldLocationRequired), errors.Is(err, ErrMoldLocationDisabled), errors.Is(err, ErrMoldSelectionRequired):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrMoldLocationInUse):
		return echo.NewHTTPError(http.StatusConflict, "位置仍被模具使用，不能停用")
	default:
		return err
	}
}
