// Package employee 负责员工档案、部门成员关系和业务操作人候选接口。
package employee

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

var shanghaiLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return location
}()

// Handler 处理员工档案和员工-部门关系。
type Handler struct {
	DB *gorm.DB
}

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// NewHandler 创建员工处理器。
func NewHandler(db *gorm.DB) *Handler { return &Handler{DB: db} }

// DepartmentSummary 是员工档案中的部门摘要。
type DepartmentSummary struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status string `json:"status"`
}

// EmployeeResponse 是员工档案 API 返回结构；年龄由出生日期实时计算。
type EmployeeResponse struct {
	ID                 uint                `json:"id"`
	OrganizationID     uint                `json:"organization_id"`
	Name               string              `json:"name"`
	Phone              string              `json:"phone,omitempty"`
	HireDate           string              `json:"hire_date"`
	Birthplace         string              `json:"birthplace,omitempty"`
	ResidentialAddress string              `json:"residential_address,omitempty"`
	BirthDate          string              `json:"birth_date"`
	Age                int                 `json:"age"`
	Status             string              `json:"status"`
	Departments        []DepartmentSummary `json:"departments"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

// OperatorEmployee 是业务表单可选择的最小员工信息，禁止返回敏感档案字段。
type OperatorEmployee struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// OperatorDepartment 是当前账号用于选择操作人的部门上下文。
type OperatorDepartment struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// OperatorEmployeesResponse 是操作人候选接口响应。
type OperatorEmployeesResponse struct {
	Department OperatorDepartment `json:"department"`
	Employees  []OperatorEmployee `json:"employees"`
}

type employeeRequest struct {
	Name               string `json:"name"`
	Phone              string `json:"phone"`
	HireDate           string `json:"hire_date"`
	Birthplace         string `json:"birthplace"`
	ResidentialAddress string `json:"residential_address"`
	BirthDate          string `json:"birth_date"`
}

type employeeStatusRequest struct {
	Status string `json:"status"`
}

// RegisterRoutes 注册员工、部门成员和操作人候选路由。
func (h *Handler) RegisterRoutes(system *echo.Group, protected *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	employees := system.Group("/employees")
	employees.GET("", h.ListEmployees, require("/api/v1/system/employees", "read"))
	employees.POST("", h.CreateEmployee, require("/api/v1/system/employees", "write"))
	employees.PUT("/:id", h.UpdateEmployee, require("/api/v1/system/employees", "write"))
	employees.PATCH("/:id/status", h.UpdateEmployeeStatus, require("/api/v1/system/employees", "write"))
	employees.DELETE("/:id", h.DeleteEmployee, require("/api/v1/system/employees", "write"))

	// 候选接口只要求登录态，和员工完整档案权限分离，供业务表单复用。
	protected.GET("/operator-employees", h.ListOperatorEmployees)
}

// ListEmployees 分页查询当前组织员工档案。
// @Summary 分页查询员工档案
// @Tags 员工管理
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "姓名、电话、籍贯或地址关键词"
// @Param keyword query string false "兼容关键词参数"
// @Param department_id query int false "部门 ID"
// @Param status query string false "员工状态 active 或 disabled"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /api/v1/system/employees [get]
func (h *Handler) ListEmployees(c *echo.Context) error {
	query := pagination.FromEcho(c)
	base := h.DB.Model(&model.Employee{})
	current := auth.GetCurrentUser(c)
	organizationID := uint(1)
	if current != nil && current.OrganizationID != 0 {
		organizationID = current.OrganizationID
	}
	base = base.Where("employees.organization_id = ?", organizationID)
	if status := strings.TrimSpace(c.QueryParam("status")); status != "" {
		base = base.Where("employees.status = ?", status)
	}
	if departmentID := strings.TrimSpace(c.QueryParam("department_id")); departmentID != "" {
		base = base.Joins("JOIN employee_departments ON employee_departments.employee_id = employees.id").
			Where("employee_departments.department_id = ?", departmentID)
	}
	base = pagination.ApplyKeyword(base, query.Keyword, "employees.name", "employees.phone", "employees.birthplace", "employees.residential_address")

	var total int64
	if err := base.Distinct("employees.id").Count(&total).Error; err != nil {
		return err
	}
	var ids []uint
	if err := base.Select("employees.id").Distinct("employees.id").Order("employees.id desc").Offset(query.Offset).Limit(query.PageSize).Pluck("employees.id", &ids).Error; err != nil {
		return err
	}
	items := make([]EmployeeResponse, 0, len(ids))
	if len(ids) > 0 {
		var employees []model.Employee
		if err := h.DB.Preload("Departments").Where("organization_id = ? AND id IN ?", organizationID, ids).Find(&employees).Error; err != nil {
			return err
		}
		byID := make(map[uint]model.Employee, len(employees))
		for _, item := range employees {
			byID[item.ID] = item
		}
		for _, id := range ids {
			if item, ok := byID[id]; ok {
				items = append(items, employeeResponse(item, time.Now()))
			}
		}
	}
	return c.JSON(http.StatusOK, pagination.Result[EmployeeResponse]{
		Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Keyword: query.Keyword,
	})
}

// CreateEmployee 创建员工档案。
//
// @Summary 新增员工档案
// @Tags 员工管理
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body employeeRequest true "员工档案"
// @Success 201 {object} EmployeeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/system/employees [post]
func (h *Handler) CreateEmployee(c *echo.Context) error {
	var req employeeRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	hireDate, birthDate, err := parseEmployeeDates(req)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "员工姓名不能为空")
	}
	organizationID := currentOrganization(c)
	item := model.Employee{
		OrganizationID: organizationID, Name: name, Phone: strings.TrimSpace(req.Phone),
		HireDate: hireDate, Birthplace: strings.TrimSpace(req.Birthplace),
		ResidentialAddress: strings.TrimSpace(req.ResidentialAddress), BirthDate: birthDate,
		Status: model.StatusActive,
	}
	if err := h.DB.WithContext(c.Request().Context()).Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, employeeResponse(item, time.Now()))
}

// UpdateEmployee 全量更新员工档案，不改变部门关系和状态。
// @Summary 更新员工档案
// @Tags 员工管理
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "员工 ID"
// @Param body body employeeRequest true "员工档案"
// @Success 200 {object} EmployeeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/system/employees/{id} [put]
func (h *Handler) UpdateEmployee(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req employeeRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	hireDate, birthDate, err := parseEmployeeDates(req)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "员工姓名不能为空")
	}
	item, err := h.loadEmployeeForCurrent(c, id)
	if err != nil {
		return err
	}
	item.Name = name
	item.Phone = strings.TrimSpace(req.Phone)
	item.HireDate = hireDate
	item.Birthplace = strings.TrimSpace(req.Birthplace)
	item.ResidentialAddress = strings.TrimSpace(req.ResidentialAddress)
	item.BirthDate = birthDate
	if err := h.DB.WithContext(c.Request().Context()).Save(&item).Error; err != nil {
		return err
	}
	if err := h.DB.Preload("Departments").First(&item, item.ID).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, employeeResponse(item, time.Now()))
}

// UpdateEmployeeStatus 启用或停用员工；停用不会删除其部门关系。
// @Summary 启用或停用员工
// @Tags 员工管理
// @Security BearerAuth
// @Accept json
// @Param id path int true "员工 ID"
// @Param body body employeeStatusRequest true "状态"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/system/employees/{id}/status [patch]
func (h *Handler) UpdateEmployeeStatus(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req employeeStatusRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Status != model.StatusActive && req.Status != model.StatusDisabled {
		return echo.NewHTTPError(http.StatusBadRequest, "员工状态必须为 active 或 disabled")
	}
	item, err := h.loadEmployeeForCurrent(c, id)
	if err != nil {
		return err
	}
	if err := h.DB.WithContext(c.Request().Context()).Model(&item).Update("status", req.Status).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteEmployee 将员工标记为停用，不做物理删除。
// @Summary 离职停用员工
// @Tags 员工管理
// @Security BearerAuth
// @Param id path int true "员工 ID"
// @Success 204
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/system/employees/{id} [delete]
func (h *Handler) DeleteEmployee(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.loadEmployeeForCurrent(c, id)
	if err != nil {
		return err
	}
	if err := h.DB.WithContext(c.Request().Context()).Model(&item).Update("status", model.StatusDisabled).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ListOperatorEmployees 返回当前账号部门下所有 active 员工的最小候选信息。
// @Summary 查询当前部门操作人候选
// @Tags 操作人
// @Security BearerAuth
// @Produce json
// @Success 200 {object} OperatorEmployeesResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/operator-employees [get]
func (h *Handler) ListOperatorEmployees(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	if current.DepartmentID == nil || *current.DepartmentID == 0 {
		return echo.NewHTTPError(http.StatusForbidden, "当前账号未绑定部门")
	}
	var department model.Department
	if err := h.DB.WithContext(c.Request().Context()).Where("id = ? AND organization_id = ?", *current.DepartmentID, current.OrganizationID).First(&department).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusForbidden, "当前账号部门不存在或不属于当前组织")
		}
		return err
	}
	if department.Status != model.StatusActive {
		return echo.NewHTTPError(http.StatusForbidden, "当前账号部门已停用")
	}
	var employees []model.Employee
	if err := h.DB.WithContext(c.Request().Context()).Model(&model.Employee{}).
		Select("employees.id, employees.name").
		Joins("JOIN employee_departments ON employee_departments.employee_id = employees.id").
		Where("employees.organization_id = ? AND employees.status = ? AND employee_departments.department_id = ?", current.OrganizationID, model.StatusActive, department.ID).
		Order("employees.name, employees.id").Find(&employees).Error; err != nil {
		return err
	}
	result := OperatorEmployeesResponse{Department: OperatorDepartment{ID: department.ID, Name: department.Name}, Employees: make([]OperatorEmployee, 0, len(employees))}
	for _, item := range employees {
		result.Employees = append(result.Employees, OperatorEmployee{ID: item.ID, Name: item.Name})
	}
	return c.JSON(http.StatusOK, result)
}

// EmployeeResponseForModel 用于部门模块复用员工响应格式。
func EmployeeResponseForModel(item model.Employee) EmployeeResponse {
	return employeeResponse(item, time.Now())
}

func employeeResponse(item model.Employee, now time.Time) EmployeeResponse {
	departments := make([]DepartmentSummary, 0, len(item.Departments))
	for _, department := range item.Departments {
		departments = append(departments, DepartmentSummary{ID: department.ID, Name: department.Name, Code: department.Code, Status: department.Status})
	}
	return EmployeeResponse{
		ID: item.ID, OrganizationID: item.OrganizationID, Name: item.Name, Phone: item.Phone,
		HireDate: formatDate(item.HireDate), Birthplace: item.Birthplace, ResidentialAddress: item.ResidentialAddress,
		BirthDate: formatDate(item.BirthDate), Age: AgeAt(item.BirthDate, now), Status: item.Status,
		Departments: departments, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// AgeAt 按 Asia/Shanghai 日期计算周岁。
func AgeAt(birthDate, now time.Time) int {
	if birthDate.IsZero() {
		return 0
	}
	localBirth := birthDate.In(shanghaiLocation)
	localNow := now.In(shanghaiLocation)
	age := localNow.Year() - localBirth.Year()
	if localNow.Month() < localBirth.Month() || (localNow.Month() == localBirth.Month() && localNow.Day() < localBirth.Day()) {
		age--
	}
	if age < 0 {
		return 0
	}
	return age
}

func parseEmployeeDates(req employeeRequest) (time.Time, time.Time, error) {
	hireDate, err := parseDate(req.HireDate, "入职日期", true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	birthDate, err := parseDate(req.BirthDate, "出生日期", true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	today := dateNow()
	if birthDate.After(today) {
		return time.Time{}, time.Time{}, echo.NewHTTPError(http.StatusBadRequest, "出生日期不能晚于今天")
	}
	return hireDate, birthDate, nil
}

func parseDate(raw, name string, required bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return time.Time{}, echo.NewHTTPError(http.StatusBadRequest, name+"不能为空")
		}
		return time.Time{}, nil
	}
	value, err := time.ParseInLocation("2006-01-02", raw, shanghaiLocation)
	if err != nil || value.Format("2006-01-02") != raw {
		return time.Time{}, echo.NewHTTPError(http.StatusBadRequest, name+"格式必须为 YYYY-MM-DD")
	}
	return value, nil
}

func dateNow() time.Time {
	now := time.Now().In(shanghaiLocation)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, shanghaiLocation)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(shanghaiLocation).Format("2006-01-02")
}

func currentOrganization(c *echo.Context) uint {
	if current := auth.GetCurrentUser(c); current != nil && current.OrganizationID != 0 {
		return current.OrganizationID
	}
	return 1
}

func employeeNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "员工不存在")
	}
	return err
}

func (h *Handler) loadEmployeeForCurrent(c *echo.Context, id uint) (model.Employee, error) {
	var item model.Employee
	if err := h.DB.WithContext(c.Request().Context()).First(&item, id).Error; err != nil {
		return item, employeeNotFound(err)
	}
	if item.OrganizationID != currentOrganization(c) {
		return item, echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	return item, nil
}

// ParseDepartmentID 是部门模块使用的安全查询参数解析辅助函数。
func ParseDepartmentID(raw string) (uint, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || value == 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "无效部门 ID")
	}
	return uint(value), nil
}
