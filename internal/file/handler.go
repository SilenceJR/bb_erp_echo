// Package file 提供受 JWT 和业务权限保护的图片资产接口。
package file

import (
	"errors"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/response"

	"github.com/casbin/casbin/v2"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type ErrorResponse = response.ErrorBody

type Handler struct {
	service  *Service
	db       *gorm.DB
	enforcer *casbin.Enforcer
}

func NewHandler(service *Service, db *gorm.DB, enforcer *casbin.Enforcer) *Handler {
	return &Handler{service: service, db: db, enforcer: enforcer}
}

// RegisterRoutes 注册图片列表、上传、内容、替换和删除接口。
func (h *Handler) RegisterRoutes(v1 *echo.Group, audit echo.MiddlewareFunc) {
	g := v1.Group("/files")
	read, write := h.permission("read"), h.permission("write")
	g.GET("", h.List, read)
	g.POST("/images", h.Create, audit, write)
	g.GET("/:id/content", h.Content, read)
	g.PUT("/:id/content", h.Replace, audit, write)
	g.DELETE("/:id", h.Delete, audit, write)
}

func (h *Handler) permission(action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			current := auth.GetCurrentUser(c)
			if current == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
			}
			if h.enforce(current, "/api/v1/warehouse", action) || h.enforce(current, "/api/v1/mold", action) || h.enforce(current, "/api/v1/workorder", action) || h.enforce(current, "/api/v1/tasks", action) {
				return next(c)
			}
			return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
		}
	}
}
func (h *Handler) enforce(u *auth.CurrentUser, object, action string) bool {
	dept := "*"
	if u.DepartmentID != nil {
		dept = strconv.FormatUint(uint64(*u.DepartmentID), 10)
	}
	ok, _ := h.enforcer.Enforce(u.Username, object, action, strconv.FormatUint(uint64(u.OrganizationID), 10), dept)
	return ok
}
func (h *Handler) canAccess(u *auth.CurrentUser, ownerType, action string) bool {
	switch ownerType {
	case OwnerProduct:
		return h.enforce(u, "/api/v1/warehouse", action)
	case OwnerMold:
		return h.enforce(u, "/api/v1/mold", action)
	case OwnerWorkOrder, OwnerDepartmentTask:
		return h.enforce(u, "/api/v1/workorder", action) || h.enforce(u, "/api/v1/tasks", action)
	}
	return false
}

// List 查询当前用户有权访问的图片资产。
// @Summary 查询业务图片
// @Tags files
// @Produce json
// @Param owner_type query string true "owner 类型"
// @Param owner_id query uint true "业务对象 ID"
// @Param category query string false "精确分类"
// @Success 200 {array} ImageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/files [get]
func (h *Handler) List(c *echo.Context) error {
	ownerType := strings.TrimSpace(c.QueryParam("owner_type"))
	ownerID, err := parseID(c.QueryParam("owner_id"), false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "owner_id 无效")
	}
	if !validOwnerType(ownerType) {
		return echo.NewHTTPError(http.StatusBadRequest, ownerError(ownerType).Error())
	}
	if !h.canAccess(auth.GetCurrentUser(c), ownerType, "read") {
		return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
	}
	if err := h.ensureOwnerExists(ownerType, ownerID); err != nil {
		return err
	}
	query := h.db.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", ownerType, ownerID)
	if category := c.QueryParam("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	var assets []model.ImageFile
	if err := query.Order("id desc").Find(&assets).Error; err != nil {
		return err
	}
	result := make([]ImageResponse, 0, len(assets))
	for i := range assets {
		result = append(result, toResponse(&assets[i]))
	}
	return c.JSON(http.StatusOK, result)
}

// Create 上传一张或多张业务图片。
// @Summary 上传业务图片
// @Tags files
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "图片文件，可重复传入"
// @Param owner_type formData string true "product、mold、workorder 或 department_task"
// @Param owner_id formData uint true "业务对象 ID"
// @Param category formData string false "图片分类"
// @Success 201 {array} ImageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/files/images [post]
func (h *Handler) Create(c *echo.Context) error {
	form, err := parseMultipartForm(c)
	if err != nil {
		return err
	}
	if form != nil {
		defer form.RemoveAll()
	}
	ownerType := strings.TrimSpace(c.FormValue("owner_type"))
	ownerID, err := parseID(c.FormValue("owner_id"), false)
	if err != nil || !validOwnerType(ownerType) {
		return echo.NewHTTPError(http.StatusBadRequest, "owner_type 或 owner_id 无效")
	}
	if !h.canAccess(auth.GetCurrentUser(c), ownerType, "write") {
		return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
	}
	if err := h.ensureOwnerExists(ownerType, ownerID); err != nil {
		return err
	}
	if err := h.ensureDepartmentWrite(c, ownerType, ownerID); err != nil {
		return err
	}
	var headers []*multipart.FileHeader
	if form != nil {
		headers = form.File["file"]
	}
	assets, err := h.service.SaveImages(headers, ownerType, ownerID, c.FormValue("category"), auth.GetCurrentUser(c).ID)
	if err != nil {
		return mapServiceError(err)
	}
	result := make([]ImageResponse, 0, len(assets))
	for _, asset := range assets {
		result = append(result, toResponse(asset))
	}
	return c.JSON(http.StatusCreated, result)
}

// Replace 替换图片，成功后旧记录软删除并移除旧物理文件。
// @Summary 替换业务图片
// @Tags files
// @Accept multipart/form-data
// @Produce json
// @Param id path uint true "图片 ID"
// @Param file formData file true "图片文件"
// @Param category formData string false "图片分类"
// @Success 200 {object} ImageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/files/{id}/content [put]
func (h *Handler) Replace(c *echo.Context) error {
	id, err := parseID(c.Param("id"), false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id 无效")
	}
	var old model.ImageFile
	if err := h.db.First(&old, id).Error; err != nil {
		return notFound(err)
	}
	if !h.canAccess(auth.GetCurrentUser(c), old.OwnerType, "write") {
		return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
	}
	if err := h.ensureOwnerExists(old.OwnerType, old.OwnerID); err != nil {
		return err
	}
	if err := h.ensureDepartmentWrite(c, old.OwnerType, old.OwnerID); err != nil {
		return err
	}
	asset, err := h.service.ReplaceImage(id, fileHeader(c), c.FormValue("category"), auth.GetCurrentUser(c).ID)
	if err != nil {
		return mapServiceError(err)
	}
	return c.JSON(http.StatusOK, toResponse(asset))
}

// Delete 软删除图片记录并删除物理文件。
// @Summary 删除业务图片
// @Tags files
// @Param id path uint true "图片 ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/files/{id} [delete]
func (h *Handler) Delete(c *echo.Context) error {
	id, err := parseID(c.Param("id"), false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id 无效")
	}
	var asset model.ImageFile
	if err := h.db.First(&asset, id).Error; err != nil {
		return notFound(err)
	}
	if !h.canAccess(auth.GetCurrentUser(c), asset.OwnerType, "write") {
		return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
	}
	if err := h.ensureOwnerExists(asset.OwnerType, asset.OwnerID); err != nil {
		return err
	}
	if err := h.ensureDepartmentWrite(c, asset.OwnerType, asset.OwnerID); err != nil {
		return err
	}
	if err := h.service.DeleteImage(&asset); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Content 返回受保护的图片内容。
// @Summary 读取业务图片内容
// @Tags files
// @Produce image/jpeg,image/png,image/webp,image/gif
// @Param id path uint true "图片 ID"
// @Success 200 {file} binary
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/files/{id}/content [get]
func (h *Handler) Content(c *echo.Context) error {
	id, err := parseID(c.Param("id"), false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id 无效")
	}
	var asset model.ImageFile
	if err := h.db.First(&asset, id).Error; err != nil {
		return notFound(err)
	}
	if !h.canAccess(auth.GetCurrentUser(c), asset.OwnerType, "read") {
		return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
	}
	if err := h.ensureOwnerExists(asset.OwnerType, asset.OwnerID); err != nil {
		return err
	}
	f, err := h.service.Open(&asset)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return echo.NewHTTPError(http.StatusNotFound, "图片文件不存在")
		}
		return err
	}
	defer f.Close()
	c.Response().Header().Set(echo.HeaderContentDisposition, "inline")
	c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(asset.Size, 10))
	return c.Stream(http.StatusOK, asset.MimeType, f)
}

func (h *Handler) ensureOwnerExists(ownerType string, ownerID uint) error {
	if ownerID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "owner_id 无效")
	}
	switch ownerType {
	case OwnerProduct:
		var item model.Product
		return ownerRecordError(h.db.First(&item, ownerID).Error)
	case OwnerMold:
		var item model.Mold
		return ownerRecordError(h.db.First(&item, ownerID).Error)
	case OwnerWorkOrder:
		var item model.WorkOrder
		return ownerRecordError(h.db.First(&item, ownerID).Error)
	case OwnerDepartmentTask:
		var task model.DepartmentTask
		return ownerRecordError(h.db.First(&task, ownerID).Error)
	}
	return echo.NewHTTPError(http.StatusBadRequest, "owner_type 无效")
}

func (h *Handler) ensureDepartmentWrite(c *echo.Context, ownerType string, ownerID uint) error {
	if ownerType != OwnerDepartmentTask {
		return nil
	}
	var task model.DepartmentTask
	if err := h.db.First(&task, ownerID).Error; err != nil {
		return ownerRecordError(err)
	}
	u := auth.GetCurrentUser(c)
	if u.DepartmentID != nil && *u.DepartmentID != task.DepartmentID {
		return echo.NewHTTPError(http.StatusForbidden, "不能操作其他部门的任务图片")
	}
	return nil
}

func parseID(value string, optional bool) (uint, error) {
	if value == "" && optional {
		return 0, nil
	}
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || n == 0 || uint64(uint(n)) != n {
		return 0, errors.New("invalid id")
	}
	return uint(n), nil
}
func parseMultipartForm(c *echo.Context) (*multipart.Form, error) {
	if err := c.Request().ParseMultipartForm(32 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return nil, nil
		}
		return nil, echo.NewHTTPError(http.StatusBadRequest, "multipart 表单无效")
	}
	return c.Request().MultipartForm, nil
}
func fileHeader(c *echo.Context) *multipart.FileHeader {
	header, _ := c.FormFile("file")
	return header
}
func ownerRecordError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "关联业务对象不存在")
	}
	return err
}
func mapServiceError(err error) error {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return echo.NewHTTPError(http.StatusBadRequest, validation.Error())
	}
	return err
}
func idString(id uint) string { return strconv.FormatUint(uint64(id), 10) }
func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "图片不存在")
	}
	return err
}
