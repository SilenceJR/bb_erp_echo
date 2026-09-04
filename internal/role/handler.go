package role

import (
	"errors"
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理角色和权限接口。
type Handler struct {
	// DB 是角色、权限读写数据库连接。
	DB *gorm.DB
	// Service 是角色权限策略服务。
	Service AssignmentService
}

// NewHandler 创建角色接口处理器。
func NewHandler(db *gorm.DB, service AssignmentService) *Handler {
	return &Handler{DB: db, Service: service}
}

// RegisterRoutes 注册角色和权限路由。
//
// 参数说明：
// - system：/api/v1/system 路由组。
// - require：权限中间件工厂。
func (h *Handler) RegisterRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/roles", h.ListRoles, require("/api/v1/system/roles", "read"))
	system.POST("/roles", h.CreateRole, require("/api/v1/system/roles", "write"))
	system.POST("/roles/:id/permissions", h.AssignRolePermissions, require("/api/v1/system/roles", "write"))
	system.GET("/permissions", h.ListPermissions, require("/api/v1/system/permissions", "read"))
}

// ListRoles 查询角色列表。
func (h *Handler) ListRoles(c *echo.Context) error {
	var items []model.Role
	pageQuery := pagination.FromEcho(c)
	query := h.DB.Model(&model.Role{})
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "name", "code", "description")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return err
	}
	if err := query.Order("id").Offset(pageQuery.Offset).Limit(pageQuery.PageSize).Find(&items).Error; err != nil {
		return err
	}
	roleIDs := make([]uint, 0, len(items))
	for _, item := range items {
		roleIDs = append(roleIDs, item.ID)
	}
	permissionIDs, err := h.Service.RolePermissionIDs(roleIDs)
	if err != nil {
		return err
	}
	type roleItem struct {
		model.Role
		PermissionIDs []uint `json:"permission_ids"`
	}
	result := make([]roleItem, 0, len(items))
	for _, item := range items {
		result = append(result, roleItem{Role: item, PermissionIDs: permissionIDs[item.ID]})
	}
	return c.JSON(http.StatusOK, pagination.Result[roleItem]{
		Items: result, Total: total, Page: pageQuery.Page, PageSize: pageQuery.PageSize, Keyword: pageQuery.Keyword,
	})
}

// CreateRole 创建角色。
func (h *Handler) CreateRole(c *echo.Context) error {
	var req struct {
		Name        string `json:"name" validate:"required"`
		Code        string `json:"code" validate:"required"`
		Description string `json:"description"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Code == SuperAdminCode {
		return echo.NewHTTPError(http.StatusBadRequest, "超级管理员角色由系统维护，不能重复创建")
	}
	item := model.Role{Name: req.Name, Code: req.Code, Description: req.Description, System: false}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// AssignRolePermissions 为角色重新绑定权限。
func (h *Handler) AssignRolePermissions(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		PermissionIDs *[]uint `json:"permission_ids" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var assignmentErr error
	if authorized, ok := h.Service.(AuthorizedAssignmentService); ok {
		assignmentErr = authorized.ReplaceRolePermissionsForActor(auth.GetCurrentUser(c), id, *req.PermissionIDs)
	} else {
		assignmentErr = h.Service.ReplaceRolePermissions(id, *req.PermissionIDs)
	}
	if assignmentErr != nil {
		return mapAssignmentError(assignmentErr)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListPermissions 查询系统内置权限列表。
func (h *Handler) ListPermissions(c *echo.Context) error {
	pageQuery := pagination.FromEcho(c)
	query := h.DB.Model(&model.Permission{})
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "name", "code", "object", "action", "description")
	result, err := pagination.Page[model.Permission](query, pageQuery, "object, action", nil)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

func mapAssignmentError(err error) error {
	switch {
	case errors.Is(err, ErrAssignmentActorRequired):
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	case errors.Is(err, ErrAssignmentPermissionDenied):
		return echo.NewHTTPError(http.StatusForbidden, "没有权限管理角色或权限")
	case errors.Is(err, ErrAssignmentOrganizationDenied), errors.Is(err, ErrAssignmentSelfDenied), errors.Is(err, ErrManagerCannotGrant):
		return echo.NewHTTPError(http.StatusForbidden, "无权执行该授权操作")
	case errors.Is(err, ErrSystemRoleLocked):
		return echo.NewHTTPError(http.StatusForbidden, "超级管理员系统角色已锁定，不能修改")
	case errors.Is(err, ErrInvalidRoleID), errors.Is(err, ErrInvalidPermissionID), errors.Is(err, ErrDuplicateAssignmentID):
		return echo.NewHTTPError(http.StatusBadRequest, "角色或权限 ID 无效")
	case errors.Is(err, ErrLastSuperAdmin):
		return echo.NewHTTPError(http.StatusConflict, "至少需要保留一个启用中的超级管理员")
	case errors.Is(err, ErrSuperAdminNotAllowed):
		return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号不能授予超级管理员角色")
	default:
		return err
	}
}
