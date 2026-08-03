// Package user 负责用户账号管理接口。
package user

import (
	"errors"
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理用户账号接口。
type Handler struct {
	// DB 是用户读写数据库连接。
	DB *gorm.DB
	// RoleService 用于用户角色绑定和策略刷新。
	RoleService role.UserRoleService
}

// NewHandler 创建用户接口处理器。
func NewHandler(db *gorm.DB, roleService role.UserRoleService) *Handler {
	return &Handler{DB: db, RoleService: roleService}
}

// RegisterRoutes 注册用户路由。
//
// 参数说明：
// - system：/api/v1/system 路由组。
// - require：权限中间件工厂。
func (h *Handler) RegisterRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/users", h.ListUsers, require("/api/v1/system/users", "read"))
	system.POST("/users", h.CreateUser, require("/api/v1/system/users", "write"))
	system.PATCH("/users/:id/status", h.UpdateUserStatus, require("/api/v1/system/users", "write"))
	system.POST("/users/:id/reset-password", h.ResetUserPassword, require("/api/v1/system/users", "write"))
	system.POST("/users/:id/roles", h.AssignUserRoles, require("/api/v1/system/users", "write"))
}

// ListUsers 查询当前用户组织内的账号列表。
func (h *Handler) ListUsers(c *echo.Context) error {
	var items []model.User
	pageQuery := pagination.FromEcho(c)
	query := h.DB.Model(&model.User{})
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "username", "name", "account_type", "status")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return err
	}
	if err := query.Order("id").Offset(pageQuery.Offset).Limit(pageQuery.PageSize).Find(&items).Error; err != nil {
		return err
	}
	userIDs := make([]uint, 0, len(items))
	for _, item := range items {
		userIDs = append(userIDs, item.ID)
	}
	roleIDs, err := h.RoleService.UserRoleIDs(userIDs)
	if err != nil {
		return err
	}
	type userItem struct {
		model.User
		RoleIDs []uint `json:"role_ids"`
	}
	result := make([]userItem, 0, len(items))
	for _, item := range items {
		result = append(result, userItem{User: item, RoleIDs: roleIDs[item.ID]})
	}
	return c.JSON(http.StatusOK, pagination.Result[userItem]{
		Items: result, Total: total, Page: pageQuery.Page, PageSize: pageQuery.PageSize, Keyword: pageQuery.Keyword,
	})
}

// CreateUser 创建登录账号。
func (h *Handler) CreateUser(c *echo.Context) error {
	var req struct {
		Username       string `json:"username" validate:"required"`
		Password       string `json:"password" validate:"required,min=8"`
		AccountType    string `json:"account_type" validate:"required,oneof=personal department_terminal"`
		Name           string `json:"name" validate:"required"`
		OrganizationID uint   `json:"organization_id" validate:"required"`
		DepartmentID   *uint  `json:"department_id"`
		TerminalID     *uint  `json:"terminal_id"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !h.canAccessOrg(c, req.OrganizationID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	if req.AccountType == model.AccountTypeDepartmentTerminal && (req.DepartmentID == nil || req.TerminalID == nil) {
		return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号必须绑定部门和终端")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}

	item := model.User{
		Username:       req.Username,
		AccountType:    req.AccountType,
		Name:           req.Name,
		OrganizationID: req.OrganizationID,
		DepartmentID:   req.DepartmentID,
		TerminalID:     req.TerminalID,
		Status:         model.StatusActive,
		PasswordHash:   hash,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	if req.AccountType == model.AccountTypeDepartmentTerminal {
		h.RoleService.AssignRoleCodes(item.ID, []string{role.TerminalOperatorCode})
	}
	return c.JSON(http.StatusCreated, item)
}

// UpdateUserStatus 启用或停用账号。
func (h *Handler) UpdateUserStatus(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		Status string `json:"status" validate:"required,oneof=active disabled"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.DB.Model(&model.User{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ResetUserPassword 重置账号密码。
func (h *Handler) ResetUserPassword(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		Password string `json:"password" validate:"required,min=8"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	if err := h.DB.Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// AssignUserRoles 为用户重新绑定角色。
func (h *Handler) AssignUserRoles(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		RoleIDs *[]uint `json:"role_ids" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.User
	if err := h.DB.First(&item, id).Error; err != nil {
		return err
	}
	allowSuperAdmin := item.AccountType != model.AccountTypeDepartmentTerminal
	if err := h.RoleService.ReplaceUserRoles(id, *req.RoleIDs, allowSuperAdmin); err != nil {
		if errors.Is(err, role.ErrSuperAdminNotAllowed) {
			return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号不能授予系统管理权限")
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) canAccessOrg(c *echo.Context, orgID uint) bool {
	current := auth.GetCurrentUser(c)
	return current == nil || current.OrganizationID == orgID
}
