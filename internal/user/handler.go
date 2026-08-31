// Package user 负责用户账号管理接口。
package user

import (
	"errors"
	"net/http"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

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

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

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
	system.PATCH("/users/:id/affiliation", h.UpdateUserAffiliation, require("/api/v1/system/users", "write"))
	system.POST("/users/:id/reset-password", h.ResetUserPassword, require("/api/v1/system/users", "write"))
	system.POST("/users/:id/roles", h.AssignUserRoles, require("/api/v1/system/users", "write"))
}

// UpdateUserAffiliation 修改账号当前部门和终端归属。
//
// 部门和终端必须同属当前管理员组织；停用部门或终端不能绑定。请求采用
// 全量替换语义，department_id/terminal_id 均可传 null 清除归属。
// @Summary 修改用户部门和终端归属
// @Tags 用户管理
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param body body map[string]interface{} true "归属信息"
// @Success 200 {object} model.User
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/system/users/{id}/affiliation [patch]
func (h *Handler) UpdateUserAffiliation(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		DepartmentID *uint `json:"department_id"`
		TerminalID   *uint `json:"terminal_id"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	db := h.DB.WithContext(c.Request().Context())
	var target model.User
	if err := db.Transaction(func(tx *gorm.DB) error {
		// 目标账号读取、组织边界和部门/终端状态校验、条件更新必须在
		// 同一事务内完成，避免管理员校验旧归属后覆盖并发变更。
		if err := tx.First(&target, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
			}
			return err
		}
		if target.OrganizationID != current.OrganizationID {
			return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
		}
		if err := h.validateAffiliationsDB(tx, target.OrganizationID, req.DepartmentID, req.TerminalID); err != nil {
			return err
		}
		if target.AccountType == model.AccountTypeDepartmentTerminal && (req.DepartmentID == nil || req.TerminalID == nil) {
			return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号必须绑定部门和终端")
		}
		result := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND updated_at = ?", target.ID, target.OrganizationID, target.UpdatedAt).
			Updates(map[string]any{"department_id": req.DepartmentID, "terminal_id": req.TerminalID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "用户归属已发生变化，请刷新后重试")
		}
		target.DepartmentID = req.DepartmentID
		target.TerminalID = req.TerminalID
		return tx.First(&target, target.ID).Error
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, target)
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
	if err := auth.ValidatePassword(req.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "密码长度必须至少为 8 个字符且不超过 bcrypt 支持的 72 字节")
	}
	if err := h.validateAffiliations(req.OrganizationID, req.DepartmentID, req.TerminalID); err != nil {
		return err
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}

	item := model.User{
		Username:        req.Username,
		AccountType:     req.AccountType,
		Name:            req.Name,
		OrganizationID:  req.OrganizationID,
		DepartmentID:    req.DepartmentID,
		TerminalID:      req.TerminalID,
		Status:          model.StatusActive,
		PasswordHash:    hash,
		PasswordVersion: auth.InitialPasswordVersion,
	}
	db := h.DB.WithContext(c.Request().Context())
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		if req.AccountType == model.AccountTypeDepartmentTerminal {
			if err := h.RoleService.AssignRoleCodesTx(tx, item.ID, []string{role.TerminalOperatorCode}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if req.AccountType == model.AccountTypeDepartmentTerminal {
		if err := h.RoleService.ReloadPolicies(); err != nil {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "账号已创建，但权限策略刷新失败，请稍后重试").Wrap(err)
		}
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
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}

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
	if err := auth.ValidatePassword(req.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "密码长度必须至少为 8 个字符且不超过 bcrypt 支持的 72 字节")
	}

	db := h.DB.WithContext(c.Request().Context())
	var target model.User
	if err := db.First(&target, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
		}
		return err
	}
	if target.OrganizationID != current.OrganizationID {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	if target.ID == current.ID {
		return echo.NewHTTPError(http.StatusForbidden, "不能通过重置密码接口修改自己的密码")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		currentVersion := target.PasswordVersion
		nextVersion := currentVersion + 1
		result := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND password_version = ?", id, current.OrganizationID, currentVersion).
			Updates(map[string]any{
				"password_hash":    hash,
				"password_version": nextVersion,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "密码已发生变化，请重试")
		}
		return auth.RevokeRefreshTokensForUser(tx, target.ID, time.Now())
	}); err != nil {
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

// validateAffiliations 校验用户填写的部门和终端归属。
//
// 部门通过 OrganizationID 直接关联组织，终端则通过所属部门间接关联组织。
// 因此不能只验证两个 ID 存在：必须同时保证它们属于目标组织；当两者都填写
// 时，还必须保证终端正是该部门的终端。所有校验在创建前完成，避免非法关联
// 留下半成品用户数据。
func (h *Handler) validateAffiliations(organizationID uint, departmentID, terminalID *uint) error {
	return h.validateAffiliationsDB(h.DB, organizationID, departmentID, terminalID)
}

func (h *Handler) validateAffiliationsDB(db *gorm.DB, organizationID uint, departmentID, terminalID *uint) error {
	if departmentID != nil {
		var department model.Department
		if err := db.Where("id = ?", *departmentID).First(&department).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusForbidden, "部门不属于目标组织")
			}
			return err
		}
		if department.OrganizationID != organizationID {
			return echo.NewHTTPError(http.StatusForbidden, "部门不属于目标组织")
		}
		if department.Status != model.StatusActive {
			return echo.NewHTTPError(http.StatusConflict, "部门已停用，不能绑定账号")
		}
	}

	if terminalID == nil {
		return nil
	}
	if departmentID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "绑定终端时必须同时选择所属部门")
	}

	var terminal model.Terminal
	if err := db.First(&terminal, *terminalID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusForbidden, "终端不属于目标组织")
		}
		return err
	}
	var terminalDepartment model.Department
	if err := db.First(&terminalDepartment, terminal.DepartmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusForbidden, "终端不属于目标组织")
		}
		return err
	}
	if terminalDepartment.OrganizationID != organizationID {
		return echo.NewHTTPError(http.StatusForbidden, "终端不属于目标组织")
	}
	if terminal.Status != model.StatusActive {
		return echo.NewHTTPError(http.StatusConflict, "终端已停用，不能绑定账号")
	}
	if terminal.DepartmentID != *departmentID {
		return echo.NewHTTPError(http.StatusForbidden, "终端不属于所选部门")
	}
	return nil
}
