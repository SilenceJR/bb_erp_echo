// Package department 负责组织、部门和终端管理接口。
package department

import (
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理组织、部门和终端接口。
type Handler struct {
	// DB 是组织、部门和终端读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建部门模块处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册组织、部门和终端路由。
//
// 参数说明：
// - system：/api/v1/system 路由组。
// - require：权限中间件工厂。
func (h *Handler) RegisterRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/organizations", h.ListOrganizations, require("/api/v1/system/organizations", "read"))
	system.POST("/organizations", h.CreateOrganization, require("/api/v1/system/organizations", "write"))
	system.GET("/departments", h.ListDepartments, require("/api/v1/system/departments", "read"))
	system.POST("/departments", h.CreateDepartment, require("/api/v1/system/departments", "write"))
	system.GET("/terminals", h.ListTerminals, require("/api/v1/system/terminals", "read"))
	system.POST("/terminals", h.CreateTerminal, require("/api/v1/system/terminals", "write"))
}

// ListOrganizations 查询组织列表。
func (h *Handler) ListOrganizations(c *echo.Context) error {
	var items []model.Organization
	if err := h.DB.Order("id").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateOrganization 创建组织。
func (h *Handler) CreateOrganization(c *echo.Context) error {
	var req struct {
		Name string `json:"name" validate:"required"`
		Code string `json:"code" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item := model.Organization{Name: req.Name, Code: req.Code, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// ListDepartments 查询当前用户组织内的部门列表。
func (h *Handler) ListDepartments(c *echo.Context) error {
	var items []model.Department
	query := h.DB.Order("id")
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateDepartment 创建部门，并校验当前用户是否能访问目标组织。
func (h *Handler) CreateDepartment(c *echo.Context) error {
	var req struct {
		OrganizationID uint   `json:"organization_id" validate:"required"`
		Name           string `json:"name" validate:"required"`
		Code           string `json:"code" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !h.canAccessOrg(c, req.OrganizationID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	item := model.Department{OrganizationID: req.OrganizationID, Name: req.Name, Code: req.Code, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// ListTerminals 查询当前用户组织内的终端列表。
func (h *Handler) ListTerminals(c *echo.Context) error {
	var items []model.Terminal
	query := h.DB.Model(&model.Terminal{}).Order("terminals.id")
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Joins("JOIN departments ON departments.id = terminals.department_id").Where("departments.organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateTerminal 创建部门终端。
func (h *Handler) CreateTerminal(c *echo.Context) error {
	var req struct {
		DepartmentID uint   `json:"department_id" validate:"required"`
		Code         string `json:"code" validate:"required"`
		Name         string `json:"name" validate:"required"`
		Location     string `json:"location"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if !h.canAccessDepartment(c, req.DepartmentID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该部门数据")
	}
	item := model.Terminal{DepartmentID: req.DepartmentID, Code: req.Code, Name: req.Name, Location: req.Location, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) canAccessOrg(c *echo.Context, orgID uint) bool {
	current := auth.GetCurrentUser(c)
	return current == nil || current.OrganizationID == orgID
}

func (h *Handler) canAccessDepartment(c *echo.Context, departmentID uint) bool {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return true
	}
	var dept model.Department
	if err := h.DB.First(&dept, departmentID).Error; err != nil {
		return false
	}
	return dept.OrganizationID == current.OrganizationID
}
