// Package department 负责组织、部门和终端管理接口。
package department

import (
	"errors"
	"net/http"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/employee"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"
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
	system.GET("/departments", h.ListDepartments, require("/api/v1/system/departments", "read"))
	system.POST("/departments", h.CreateDepartment, require("/api/v1/system/departments", "write"))
	system.PUT("/departments/:id", h.UpdateDepartment, require("/api/v1/system/departments", "write"))
	system.PATCH("/departments/:id", h.UpdateDepartment, require("/api/v1/system/departments", "write"))
	system.PATCH("/departments/:id/status", h.UpdateDepartmentStatus, require("/api/v1/system/departments", "write"))
	system.GET("/departments/:id/employees", h.ListDepartmentEmployees,
		require("/api/v1/system/departments", "read"), require("/api/v1/system/employees", "read"))
	system.PUT("/departments/:id/employees", h.ReplaceDepartmentEmployees,
		require("/api/v1/system/departments", "write"), require("/api/v1/system/employees", "read"))
	system.GET("/terminals", h.ListTerminals, require("/api/v1/system/terminals", "read"))
	system.POST("/terminals", h.CreateTerminal, require("/api/v1/system/terminals", "write"))
}

// ListDepartments 查询当前用户组织内的部门列表。
func (h *Handler) ListDepartments(c *echo.Context) error {
	pageQuery := pagination.FromEcho(c)
	query := h.DB.Model(&model.Department{})
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "name", "code", "status")
	result, err := pagination.Page[model.Department](query, pageQuery, "id", nil)
	if err != nil {
		return err
	}
	if err := h.populateEmployeeCounts(result.Items); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// CreateDepartment 创建部门，并校验当前用户是否能访问目标组织。
func (h *Handler) CreateDepartment(c *echo.Context) error {
	var req struct {
		Name string `json:"name" validate:"required"`
		Code string `json:"code" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	organizationID := uint(1)
	if current := auth.GetCurrentUser(c); current != nil && current.OrganizationID != 0 {
		organizationID = current.OrganizationID
	}
	item := model.Department{OrganizationID: organizationID, Name: req.Name, Code: req.Code, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// ListTerminals 查询当前用户组织内的终端列表。
func (h *Handler) ListTerminals(c *echo.Context) error {
	pageQuery := pagination.FromEcho(c)
	query := h.DB.Model(&model.Terminal{})
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Joins("JOIN departments ON departments.id = terminals.department_id").Where("departments.organization_id = ?", current.OrganizationID)
	}
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "terminals.code", "terminals.name", "terminals.location", "terminals.status")
	result, err := pagination.Page[model.Terminal](query, pageQuery, "terminals.id", nil)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
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
	department, err := h.loadDepartmentForCurrent(c, req.DepartmentID)
	if err != nil {
		return err
	}
	if department.Status != model.StatusActive {
		return echo.NewHTTPError(http.StatusConflict, "停用部门不能绑定终端")
	}
	item := model.Terminal{DepartmentID: req.DepartmentID, Code: req.Code, Name: req.Name, Location: req.Location, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// UpdateDepartment 更新部门名称和编码，不改变组织与状态。
// @Summary 更新部门
// @Tags 部门管理
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "部门 ID"
// @Success 200 {object} model.Department
// @Failure 400 {object} response.ErrorBody
// @Failure 403 {object} response.ErrorBody
// @Failure 404 {object} response.ErrorBody
// @Router /api/v1/system/departments/{id} [put]
func (h *Handler) UpdateDepartment(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	name, code := strings.TrimSpace(req.Name), strings.TrimSpace(req.Code)
	if name == "" || code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "部门名称和编码不能为空")
	}
	item, err := h.loadDepartmentForCurrent(c, id)
	if err != nil {
		return err
	}
	item.Name, item.Code = name, code
	if err := h.DB.WithContext(c.Request().Context()).Save(&item).Error; err != nil {
		return err
	}
	if err := h.populateEmployeeCounts([]model.Department{item}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// UpdateDepartmentStatus 启用或停用部门，停用不删除历史关系。
// @Summary 启用或停用部门
// @Tags 部门管理
// @Security BearerAuth
// @Accept json
// @Param id path int true "部门 ID"
// @Success 204
// @Failure 400 {object} response.ErrorBody
// @Failure 403 {object} response.ErrorBody
// @Failure 404 {object} response.ErrorBody
// @Router /api/v1/system/departments/{id}/status [patch]
func (h *Handler) UpdateDepartmentStatus(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Status != model.StatusActive && req.Status != model.StatusDisabled {
		return echo.NewHTTPError(http.StatusBadRequest, "部门状态必须为 active 或 disabled")
	}
	item, err := h.loadDepartmentForCurrent(c, id)
	if err != nil {
		return err
	}
	if err := h.DB.WithContext(c.Request().Context()).Model(&item).Update("status", req.Status).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ListDepartmentEmployees 查询当前部门的全部员工关系，包含停用员工供管理员移除。
// @Summary 查询部门员工
// @Tags 部门管理
// @Security BearerAuth
// @Produce json
// @Param id path int true "部门 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} response.ErrorBody
// @Failure 404 {object} response.ErrorBody
// @Router /api/v1/system/departments/{id}/employees [get]
func (h *Handler) ListDepartmentEmployees(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	department, err := h.loadDepartmentForCurrent(c, id)
	if err != nil {
		return err
	}
	var items []model.Employee
	if err := h.DB.WithContext(c.Request().Context()).Model(&model.Employee{}).
		Joins("JOIN employee_departments ON employee_departments.employee_id = employees.id").
		Where("employee_departments.department_id = ? AND employees.organization_id = ?", id, department.OrganizationID).
		Preload("Departments").Order("employees.name, employees.id").Find(&items).Error; err != nil {
		return err
	}
	responses := make([]employee.EmployeeResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, employee.EmployeeResponseForModel(item))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"department": department,
		"employees":  responses,
	})
}

// ReplaceDepartmentEmployees 原子替换部门员工关系；空数组表示移除当前部门全部成员。
// @Summary 替换部门员工
// @Tags 部门管理
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "部门 ID"
// @Param body body map[string]interface{} true "员工 ID 列表"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorBody
// @Failure 403 {object} response.ErrorBody
// @Failure 409 {object} response.ErrorBody
// @Router /api/v1/system/departments/{id}/employees [put]
func (h *Handler) ReplaceDepartmentEmployees(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		EmployeeIDs *[]uint `json:"employee_ids"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.EmployeeIDs == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "employee_ids 必须为数组")
	}
	var department model.Department
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		ids := make([]uint, 0, len(*req.EmployeeIDs))
		for _, employeeID := range *req.EmployeeIDs {
			if employeeID == 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "employee_ids 不能包含 0")
			}
			ids = append(ids, employeeID)
		}
		ids = uniqueUint(ids)
		var err error
		department, err = h.loadDepartmentForCurrentDB(tx, c, id)
		if err != nil {
			return err
		}
		if department.Status != model.StatusActive {
			// 停用部门仍允许移除关系，但不能借此新增员工。读取当前集合
			// 后要求新集合是其子集，保证 PUT 的原子替换语义不绕过停用限制。
			var existingIDs []uint
			if err := tx.Model(&model.EmployeeDepartment{}).
				Where("department_id = ?", department.ID).Pluck("employee_id", &existingIDs).Error; err != nil {
				return err
			}
			existing := make(map[uint]struct{}, len(existingIDs))
			for _, existingID := range existingIDs {
				existing[existingID] = struct{}{}
			}
			for _, employeeID := range ids {
				if _, ok := existing[employeeID]; !ok {
					return echo.NewHTTPError(http.StatusConflict, "停用部门不能新增员工")
				}
			}
		} else if err := h.validateEmployeesForDepartmentDB(tx, c, department, ids); err != nil {
			return err
		}
		if err := tx.Where("department_id = ?", department.ID).Delete(&model.EmployeeDepartment{}).Error; err != nil {
			return err
		}
		for _, employeeID := range ids {
			if err := tx.Create(&model.EmployeeDepartment{EmployeeID: employeeID, DepartmentID: department.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return h.ListDepartmentEmployees(c)
}

func (h *Handler) populateEmployeeCounts(items []model.Department) error {
	if len(items) == 0 {
		return nil
	}
	var rows []struct {
		DepartmentID uint
		Count        int64
	}
	if err := h.DB.Model(&model.EmployeeDepartment{}).
		Select("employee_departments.department_id, COUNT(employees.id) AS count").
		Joins("JOIN employees ON employees.id = employee_departments.employee_id AND employees.status = ?", model.StatusActive).
		Where("employee_departments.department_id IN ?", departmentIDs(items)).
		Group("employee_departments.department_id").Scan(&rows).Error; err != nil {
		return err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.DepartmentID] = row.Count
	}
	for index := range items {
		items[index].EmployeeCount = counts[items[index].ID]
	}
	return nil
}

func (h *Handler) validateEmployeesForDepartment(c *echo.Context, department model.Department, ids []uint) error {
	return h.validateEmployeesForDepartmentDB(h.DB, c, department, ids)
}

func (h *Handler) validateEmployeesForDepartmentDB(db *gorm.DB, c *echo.Context, department model.Department, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	var employees []model.Employee
	if err := db.WithContext(c.Request().Context()).Where("id IN ?", ids).Find(&employees).Error; err != nil {
		return err
	}
	byID := make(map[uint]model.Employee, len(employees))
	for _, item := range employees {
		byID[item.ID] = item
	}
	for _, id := range ids {
		item, ok := byID[id]
		if !ok {
			return echo.NewHTTPError(http.StatusForbidden, "员工不属于当前组织")
		}
		if item.OrganizationID != department.OrganizationID {
			return echo.NewHTTPError(http.StatusForbidden, "员工不属于当前组织")
		}
		if item.Status != model.StatusActive {
			return echo.NewHTTPError(http.StatusConflict, "停用员工不能新增到部门")
		}
	}
	return nil
}

func (h *Handler) loadDepartmentForCurrent(c *echo.Context, id uint) (model.Department, error) {
	return h.loadDepartmentForCurrentDB(h.DB, c, id)
}

func (h *Handler) loadDepartmentForCurrentDB(db *gorm.DB, c *echo.Context, id uint) (model.Department, error) {
	var item model.Department
	if err := db.WithContext(c.Request().Context()).First(&item, id).Error; err != nil {
		return item, departmentNotFound(err)
	}
	if item.OrganizationID != currentOrganization(c) {
		return item, echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	return item, nil
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

func currentOrganization(c *echo.Context) uint {
	if current := auth.GetCurrentUser(c); current != nil && current.OrganizationID != 0 {
		return current.OrganizationID
	}
	return 1
}

func departmentNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "部门不存在")
	}
	return err
}

func departmentIDs(items []model.Department) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func uniqueUint(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
