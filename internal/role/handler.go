package role

import (
	"net/http"

	"bb_erp_echo/internal/model"
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
	if err := h.DB.Order("id").Find(&items).Error; err != nil {
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
	return c.JSON(http.StatusOK, result)
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
	item := model.Role{Name: req.Name, Code: req.Code, Description: req.Description}
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
	if err := h.Service.ReplaceRolePermissions(id, *req.PermissionIDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ListPermissions 查询系统内置权限列表。
func (h *Handler) ListPermissions(c *echo.Context) error {
	var items []model.Permission
	if err := h.DB.Order("object, action").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}
