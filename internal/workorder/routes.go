// Package workorder 实现可流转任务单和部门子任务接口。
package workorder

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

const (
	TypeProduction = "production"
	TypeGeneral    = "general"

	StatusDraft           = "draft"
	StatusProcessing      = "processing"
	StatusPaused          = "paused"
	StatusPendingClose    = "pending_close"
	StatusCompletedNormal = "completed_normal"
	StatusCompletedForced = "completed_forced"
	StatusCancelled       = "cancelled"

	PriorityNormal = "normal"
	PriorityUrgent = "urgent"

	DepartmentTaskDraft            = "draft"
	DepartmentTaskReceived         = "received"
	DepartmentTaskProcessing       = "processing"
	DepartmentTaskPartialCompleted = "partial_completed"
	DepartmentTaskCompleted        = "completed"
)

// Handler 处理任务单和部门子任务流转。
type Handler struct {
	DB *gorm.DB
}

// ListResponse 是任务单分页列表的 Swagger 文档响应。
type ListResponse struct {
	Items    []model.WorkOrder `json:"items"`
	Total    int64             `json:"total" example:"1"`
	Page     int               `json:"page" example:"1"`
	PageSize int               `json:"page_size" example:"20"`
	Keyword  string            `json:"keyword,omitempty" example:"白色"`
}

type createRequest struct {
	Code                string `json:"code"`
	Title               string `json:"title"`
	Type                string `json:"type"`
	CustomerID          *uint  `json:"customer_id"`
	ProductID           *uint  `json:"product_id"`
	PlannedQuantity     int64  `json:"planned_quantity"`
	DueAt               string `json:"due_at"`
	Priority            string `json:"priority"`
	Description         string `json:"description"`
	TargetDepartmentIDs []uint `json:"target_department_ids"`
	OperatorEmployeeID  uint   `json:"operator_employee_id" validate:"required"`
}

// temporaryProductRequest 是生产单内临时建立仓库产品档案的请求体。
type temporaryProductRequest struct {
	Name               string `json:"name" validate:"required" example:"白色外壳"`
	Code               string `json:"code" validate:"required" example:"P-001"`
	Spec               string `json:"spec" example:"标准"`
	Unit               string `json:"unit" example:"个"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type reasonRequest struct {
	Reason             string `json:"reason"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type urgentRequest struct {
	Urgent             bool `json:"urgent"`
	OperatorEmployeeID uint `json:"operator_employee_id" validate:"required"`
}

type completeRequest struct {
	Mode               string `json:"mode"`
	Reason             string `json:"reason"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type partialCompleteRequest struct {
	CompletedQuantity  int64  `json:"completed_quantity"`
	Remark             string `json:"remark"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type remarkRequest struct {
	Remark             string `json:"remark"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type operatorActionRequest struct {
	OperatorEmployeeID uint `json:"operator_employee_id" validate:"required"`
}

// RegisterRoutes 注册任务单模块路由。
func RegisterRoutes(v1 *echo.Group, db *gorm.DB, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	handler := &Handler{DB: db}
	handler.register(v1, "workorder", require, audit)
	handler.register(v1, "tasks", require, audit)
}

func (h *Handler) register(v1 *echo.Group, path string, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	object := "/api/v1/" + path
	group := v1.Group("/"+path, audit)
	group.GET("", h.List, require(object, "read"))
	group.POST("", h.Create, require(object, "write"))
	if path == "workorder" {
		group.POST("/products", h.CreateTemporaryProduct, require(object, "write"), require(temporaryProductObject, "write"))
	}
	group.POST("/:id/dispatch", h.Dispatch, require(object, "write"))
	group.POST("/:id/pause", h.Pause, require(object, "write"))
	group.POST("/:id/resume", h.Resume, require(object, "write"))
	group.POST("/:id/urgent", h.SetUrgent, require(object, "write"))
	group.POST("/:id/complete", h.Complete, require(object, "write"))
	group.GET("/:id/logs", h.ListLogs, require(object, "read"))
	group.POST("/department-tasks/:id/start", h.StartDepartmentTask, require(object, "write"))
	group.POST("/department-tasks/:id/partial-complete", h.PartialCompleteDepartmentTask, require(object, "write"))
	group.POST("/department-tasks/:id/complete", h.CompleteDepartmentTask, require(object, "write"))
}

const temporaryProductObject = "/api/v1/workorder/products"

// List 分页查询任务单，并返回部门子任务摘要。
// @Summary 分页查询任务单
// @Tags workorder
// @Security BearerAuth
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "模糊关键字"
// @Param status query string false "主任务状态"
// @Param type query string false "任务类型"
// @Param department_id query int false "流转部门 ID"
// @Param priority query string false "优先级"
// @Success 200 {object} ListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/workorder [get]
// @Router /api/v1/tasks [get]
func (h *Handler) List(c *echo.Context) error {
	query := pagination.FromEcho(c)
	db := h.DB.Model(&model.WorkOrder{})
	db = pagination.ApplyKeyword(db, query.Keyword, "work_orders.code", "work_orders.title", "work_orders.product_name", "work_orders.description")
	if status := strings.TrimSpace(c.QueryParam("status")); status != "" {
		db = db.Where("status = ?", status)
	}
	if value := strings.TrimSpace(c.QueryParam("type")); value != "" {
		db = db.Where("type = ?", value)
	}
	if priority := strings.TrimSpace(c.QueryParam("priority")); priority != "" {
		db = db.Where("priority = ?", priority)
	}
	if departmentID := strings.TrimSpace(c.QueryParam("department_id")); departmentID != "" {
		db = db.Joins("JOIN department_tasks ON department_tasks.work_order_id = work_orders.id").
			Where("department_tasks.department_id = ?", departmentID).
			Group("work_orders.id")
	}
	result, err := pagination.Page[model.WorkOrder](db, query, "id desc", func(tx *gorm.DB) *gorm.DB {
		return tx.Preload("DepartmentTasks", func(preload *gorm.DB) *gorm.DB {
			return preload.Order("id asc")
		})
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// Create 创建草稿任务单。
// @Summary 创建草稿任务单
// @Description 创建生产单时必须提供启用仓库产品的 product_id；服务端会从产品主数据写入 product_name 和 unit 快照。通用任务不会关联产品。
// @Tags workorder
// @Security BearerAuth
// @Param body body createRequest true "任务单创建参数"
// @Success 201 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/workorder [post]
// @Router /api/v1/tasks [post]
func (h *Handler) Create(c *echo.Context) error {
	var req createRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := validateCreateRequest(req); err != nil {
		return err
	}
	dueAt, err := parseOptionalTime(req.DueAt)
	if err != nil {
		return err
	}
	itemType := defaultString(req.Type, TypeProduction)
	current := auth.GetCurrentUser(c)
	item := model.WorkOrder{
		Code:            strings.TrimSpace(req.Code),
		Title:           strings.TrimSpace(req.Title),
		Type:            itemType,
		Status:          StatusDraft,
		Priority:        defaultString(req.Priority, PriorityNormal),
		CustomerID:      req.CustomerID,
		PlannedQuantity: req.PlannedQuantity,
		DueAt:           dueAt,
		Description:     strings.TrimSpace(req.Description),
	}
	if item.Code == "" {
		item.Code = fmt.Sprintf("WO-%s-%d", time.Now().Format("20060102"), time.Now().UnixNano())
	}
	if item.Title == "" {
		item.Title = titleFromRequest(item)
	}
	if current != nil {
		item.CreatedBy = current.ID
	}
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		if err := h.validateTargetDepartmentsDB(tx, c, req.TargetDepartmentIDs); err != nil {
			return err
		}
		if itemType == TypeProduction {
			product, err := h.loadActiveProductDB(tx, req.ProductID)
			if err != nil {
				return err
			}
			item.ProductID = &product.ID
			item.ProductName = product.Name
			item.Unit = product.Unit
		}
		item.DepartmentTasks = make([]model.DepartmentTask, 0, len(req.TargetDepartmentIDs))
		for _, departmentID := range uniqueUint(req.TargetDepartmentIDs) {
			item.DepartmentTasks = append(item.DepartmentTasks, model.DepartmentTask{
				DepartmentID:      departmentID,
				Title:             item.Title,
				Status:            DepartmentTaskDraft,
				PlannedQuantity:   item.PlannedQuantity,
				CompletedQuantity: 0,
			})
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return h.createFlowLog(tx, c, item.ID, nil, nil, "create", "", item.Status, 0, item.PlannedQuantity, "", item.Description)
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("DepartmentTasks", func(tx *gorm.DB) *gorm.DB { return tx.Order("id asc") }).First(&item, item.ID).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// CreateTemporaryProduct 在生产单内创建尚未建档的仓库产品。
//
// @Summary 临时建立仓库产品档案
// @Description 创建启用状态的正式产品档案；初始安全库存和当前库存均为 0，不创建库存流水。接口同时需要 workorder:write 和 workorder:temporary-product:write 权限。
// @Tags workorder
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body temporaryProductRequest true "产品建档参数"
// @Success 201 {object} model.Product
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/workorder/products [post]
func (h *Handler) CreateTemporaryProduct(c *echo.Context) error {
	var req temporaryProductRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	req.Spec = strings.TrimSpace(req.Spec)
	req.Unit = defaultString(req.Unit, "个")
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "产品名称不能为空")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "产品编码不能为空")
	}

	var item model.Product
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		var existing model.Product
		err := tx.Where("code = ?", req.Code).First(&existing).Error
		if err == nil {
			return echo.NewHTTPError(http.StatusConflict, "产品编码已存在")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		item = model.Product{
			Name:             req.Name,
			Code:             req.Code,
			Spec:             req.Spec,
			Unit:             req.Unit,
			Status:           model.StatusActive,
			OperatorSnapshot: operator.Snapshot(c),
		}
		if err := tx.Create(&item).Error; err != nil {
			if isUniqueConstraintError(err) {
				return echo.NewHTTPError(http.StatusConflict, "产品编码已存在")
			}
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) loadActiveProduct(id *uint) (*model.Product, error) {
	return h.loadActiveProductDB(h.DB, id)
}

func (h *Handler) loadActiveProductDB(db *gorm.DB, id *uint) (*model.Product, error) {
	if id == nil || *id == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "生产单必须选择仓库产品")
	}
	var product model.Product
	if err := db.First(&product, *id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "仓库产品不存在")
		}
		return nil, err
	}
	if product.Status != model.StatusActive {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "仓库产品已停用，不能用于生产单")
	}
	return &product, nil
}

func isUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique violation")
}

// Dispatch 派发任务，目标部门自动进入已收到状态。
// @Summary 派发任务单
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Param body body operatorActionRequest true "操作人参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/dispatch [post]
// @Router /api/v1/tasks/{id}/dispatch [post]
func (h *Handler) Dispatch(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req operatorActionRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		if err := tx.Preload("DepartmentTasks").First(&item, id).Error; err != nil {
			return recordNotFound(err, "任务单不存在")
		}
		if item.Status != StatusDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "只有草稿任务单可以派发")
		}
		if len(item.DepartmentTasks) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "请先选择流转部门")
		}
		now := time.Now()
		before := item.Status
		result := tx.Model(&model.WorkOrder{}).
			Where("id = ? AND status = ?", item.ID, StatusDraft).
			Updates(map[string]any{"status": StatusProcessing, "dispatched_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "任务状态已变化，请刷新后重试")
		}
		item.Status = StatusProcessing
		item.DispatchedAt = &now
		for index := range item.DepartmentTasks {
			task := &item.DepartmentTasks[index]
			taskBefore := task.Status
			task.Status = DepartmentTaskReceived
			task.AcceptedAt = &now
			result := tx.Model(&model.DepartmentTask{}).
				Where("id = ? AND status = ?", task.ID, taskBefore).
				Updates(map[string]any{"status": task.Status, "accepted_at": task.AcceptedAt})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return echo.NewHTTPError(http.StatusConflict, "部门子任务状态已变化，请刷新后重试")
			}
			if err := h.createFlowLog(tx, c, item.ID, &task.ID, &task.DepartmentID, "dispatch_department", taskBefore, task.Status, 0, task.PlannedQuantity, "", ""); err != nil {
				return err
			}
		}
		return h.createFlowLog(tx, c, item.ID, nil, nil, "dispatch", before, item.Status, 0, item.PlannedQuantity, "", "")
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

// Pause 暂停任务，暂停后部门不能继续提交进度。
// @Summary 暂停任务单
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Param body body reasonRequest true "暂停原因"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/pause [post]
// @Router /api/v1/tasks/{id}/pause [post]
func (h *Handler) Pause(c *echo.Context) error {
	var req reasonRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "暂停必须填写原因")
	}
	return h.changeWorkOrderStatus(c, req.OperatorEmployeeID, StatusPaused, "pause", req.Reason, func(item model.WorkOrder) error {
		if item.Status != StatusProcessing && item.Status != StatusPendingClose {
			return echo.NewHTTPError(http.StatusBadRequest, "只有正在处理或待确认任务可以暂停")
		}
		return nil
	})
}

// Resume 恢复暂停任务。
// @Summary 恢复任务单
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Param body body operatorActionRequest true "操作人参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/resume [post]
// @Router /api/v1/tasks/{id}/resume [post]
func (h *Handler) Resume(c *echo.Context) error {
	var req operatorActionRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.changeWorkOrderStatus(c, req.OperatorEmployeeID, StatusProcessing, "resume", "", func(item model.WorkOrder) error {
		if item.Status != StatusPaused {
			return echo.NewHTTPError(http.StatusBadRequest, "只有暂停任务可以恢复")
		}
		return nil
	})
}

// SetUrgent 设置或取消加急，只改变优先级，不改变主状态。
// @Summary 设置或取消加急
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Param body body urgentRequest true "加急参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/urgent [post]
// @Router /api/v1/tasks/{id}/urgent [post]
func (h *Handler) SetUrgent(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req urgentRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		if err := tx.First(&item, id).Error; err != nil {
			return recordNotFound(err, "任务单不存在")
		}
		before := item.Priority
		after := PriorityNormal
		if req.Urgent {
			after = PriorityUrgent
		}
		if before == after {
			return nil
		}
		result := tx.Model(&model.WorkOrder{}).
			Where("id = ? AND priority = ?", item.ID, before).
			Update("priority", after)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "任务优先级已变化，请刷新后重试")
		}
		item.Priority = after
		return h.createFlowLog(tx, c, item.ID, nil, nil, "urgent", before, item.Priority, 0, 0, "", "")
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

// Complete 由办公室确认正常完成或强制完成任务单。
// @Summary 办公室确认完成任务单
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Param body body completeRequest true "完成参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/complete [post]
// @Router /api/v1/tasks/{id}/complete [post]
func (h *Handler) Complete(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req completeRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		if err := tx.Preload("DepartmentTasks").First(&item, id).Error; err != nil {
			return recordNotFound(err, "任务单不存在")
		}
		targetStatus := ""
		action := ""
		if req.Mode == "normal" {
			if item.Status != StatusPendingClose {
				return echo.NewHTTPError(http.StatusBadRequest, "正常完成只能在待办公室确认状态执行")
			}
			if !allDepartmentTasksCompleted(item.DepartmentTasks) {
				return echo.NewHTTPError(http.StatusBadRequest, "所有部门完成后才能正常完成")
			}
			targetStatus = StatusCompletedNormal
			action = "complete_normal"
		} else if req.Mode == "forced" {
			if item.Status != StatusProcessing && item.Status != StatusPaused && item.Status != StatusPendingClose {
				return echo.NewHTTPError(http.StatusBadRequest, "当前状态不能强制完成")
			}
			if strings.TrimSpace(req.Reason) == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "强制完成必须填写原因")
			}
			targetStatus = StatusCompletedForced
			action = "complete_forced"
		} else {
			return echo.NewHTTPError(http.StatusBadRequest, "完成模式必须为 normal 或 forced")
		}
		before := item.Status
		now := time.Now()
		result := tx.Model(&model.WorkOrder{}).
			Where("id = ? AND status = ?", item.ID, before).
			Updates(map[string]any{"status": targetStatus, "completed_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "任务状态已变化，请刷新后重试")
		}
		item.Status = targetStatus
		item.CompletedAt = &now
		return h.createFlowLog(tx, c, item.ID, nil, nil, action, before, item.Status, 0, item.PlannedQuantity, req.Reason, "")
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

// StartDepartmentTask 让部门子任务从已收到进入正在处理。
// @Summary 部门开始处理子任务
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "部门子任务 ID"
// @Param body body remarkRequest true "操作人和备注参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/department-tasks/{id}/start [post]
// @Router /api/v1/tasks/department-tasks/{id}/start [post]
func (h *Handler) StartDepartmentTask(c *echo.Context) error {
	var req remarkRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.updateDepartmentTask(c, req.OperatorEmployeeID, "department_start", func(item model.WorkOrder, task *model.DepartmentTask) error {
		if task.Status != DepartmentTaskReceived {
			return echo.NewHTTPError(http.StatusBadRequest, "只有已收到任务可以开始处理")
		}
		now := time.Now()
		task.Status = DepartmentTaskProcessing
		task.AcceptedAt = &now
		if current := auth.GetCurrentUser(c); current != nil {
			task.AssigneeUserID = &current.ID
		}
		task.Remark = strings.TrimSpace(req.Remark)
		return nil
	})
}

// PartialCompleteDepartmentTask 部门提交部分完成数量。
// @Summary 部门提交部分完成数量
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "部门子任务 ID"
// @Param body body partialCompleteRequest true "部分完成参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/department-tasks/{id}/partial-complete [post]
// @Router /api/v1/tasks/department-tasks/{id}/partial-complete [post]
func (h *Handler) PartialCompleteDepartmentTask(c *echo.Context) error {
	var req partialCompleteRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.updateDepartmentTask(c, req.OperatorEmployeeID, "department_partial_complete", func(item model.WorkOrder, task *model.DepartmentTask) error {
		if req.CompletedQuantity <= 0 || req.CompletedQuantity >= task.PlannedQuantity {
			return echo.NewHTTPError(http.StatusBadRequest, "部分完成数量必须大于 0 且小于计划数量")
		}
		if req.CompletedQuantity <= task.CompletedQuantity {
			return echo.NewHTTPError(http.StatusBadRequest, "已完成数量必须大于当前已完成数量")
		}
		if task.Status != DepartmentTaskReceived && task.Status != DepartmentTaskProcessing && task.Status != DepartmentTaskPartialCompleted {
			return echo.NewHTTPError(http.StatusBadRequest, "当前子任务状态不能部分完成")
		}
		now := time.Now()
		task.Status = DepartmentTaskPartialCompleted
		task.CompletedQuantity = req.CompletedQuantity
		task.Progress = int(req.CompletedQuantity * 100 / task.PlannedQuantity)
		if task.Progress >= 100 {
			task.Progress = 99
		}
		task.PartialCompletedAt = &now
		task.Remark = strings.TrimSpace(req.Remark)
		return nil
	})
}

// CompleteDepartmentTask 部门完成子任务；全部完成后主任务进入待办公室确认。
// @Summary 部门完成子任务
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "部门子任务 ID"
// @Param body body remarkRequest true "操作人和备注参数"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/department-tasks/{id}/complete [post]
// @Router /api/v1/tasks/department-tasks/{id}/complete [post]
func (h *Handler) CompleteDepartmentTask(c *echo.Context) error {
	var req remarkRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.updateDepartmentTask(c, req.OperatorEmployeeID, "department_complete", func(item model.WorkOrder, task *model.DepartmentTask) error {
		if task.Status != DepartmentTaskReceived && task.Status != DepartmentTaskProcessing && task.Status != DepartmentTaskPartialCompleted {
			return echo.NewHTTPError(http.StatusBadRequest, "当前子任务状态不能完成")
		}
		now := time.Now()
		task.Status = DepartmentTaskCompleted
		task.CompletedQuantity = task.PlannedQuantity
		task.Progress = 100
		task.CompletedAt = &now
		task.Remark = strings.TrimSpace(req.Remark)
		return nil
	})
}

// ListLogs 查询任务单流转日志。
// @Summary 查询任务流转日志
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Success 200 {array} model.WorkOrderFlowLog
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/logs [get]
// @Router /api/v1/tasks/{id}/logs [get]
func (h *Handler) ListLogs(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var logs []model.WorkOrderFlowLog
	if err := h.DB.Where("work_order_id = ?", id).Order("id asc").Find(&logs).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, logs)
}

func (h *Handler) changeWorkOrderStatus(c *echo.Context, operatorEmployeeID uint, targetStatus string, action string, reason string, validate func(model.WorkOrder) error) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, operatorEmployeeID); err != nil {
			return err
		}
		if err := tx.First(&item, id).Error; err != nil {
			return recordNotFound(err, "任务单不存在")
		}
		if err := validate(item); err != nil {
			return err
		}
		before := item.Status
		result := tx.Model(&model.WorkOrder{}).
			Where("id = ? AND status = ?", item.ID, before).
			Update("status", targetStatus)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "任务状态已变化，请刷新后重试")
		}
		item.Status = targetStatus
		return h.createFlowLog(tx, c, item.ID, nil, nil, action, before, item.Status, 0, item.PlannedQuantity, reason, "")
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

func (h *Handler) updateDepartmentTask(c *echo.Context, operatorEmployeeID uint, action string, mutate func(model.WorkOrder, *model.DepartmentTask) error) error {
	taskID, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var task model.DepartmentTask
	var item model.WorkOrder
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, operatorEmployeeID); err != nil {
			return err
		}
		if err := tx.First(&task, taskID).Error; err != nil {
			return recordNotFound(err, "部门子任务不存在")
		}
		if err := tx.Preload("DepartmentTasks").First(&item, task.WorkOrderID).Error; err != nil {
			return recordNotFound(err, "任务单不存在")
		}
		if item.Status == StatusPaused {
			return echo.NewHTTPError(http.StatusBadRequest, "任务已暂停，部门不能继续提交进度")
		}
		if item.Status != StatusProcessing && item.Status != StatusPendingClose {
			return echo.NewHTTPError(http.StatusBadRequest, "当前主任务状态不能提交部门进度")
		}
		if err := h.ensureDepartmentTaskAccess(c, task.DepartmentID); err != nil {
			return err
		}
		beforeStatus := task.Status
		beforeQuantity := task.CompletedQuantity
		if err := mutate(item, &task); err != nil {
			return err
		}
		result := tx.Model(&model.DepartmentTask{}).
			Where("id = ? AND status = ? AND completed_quantity = ?", task.ID, beforeStatus, beforeQuantity).
			Updates(map[string]any{
				"status":               task.Status,
				"completed_quantity":   task.CompletedQuantity,
				"assignee_user_id":     task.AssigneeUserID,
				"progress":             task.Progress,
				"remark":               task.Remark,
				"accepted_at":          task.AcceptedAt,
				"partial_completed_at": task.PartialCompletedAt,
				"completed_at":         task.CompletedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "部门子任务状态已变化，请刷新后重试")
		}
		if action == "department_complete" {
			var remaining int64
			if err := tx.Model(&model.DepartmentTask{}).
				Where("work_order_id = ? AND status <> ?", item.ID, DepartmentTaskCompleted).
				Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 && item.Status != StatusPendingClose {
				before := item.Status
				result := tx.Model(&model.WorkOrder{}).
					Where("id = ? AND status = ?", item.ID, before).
					Update("status", StatusPendingClose)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return echo.NewHTTPError(http.StatusConflict, "任务状态已变化，请刷新后重试")
				}
				item.Status = StatusPendingClose
				if err := h.createFlowLog(tx, c, item.ID, nil, nil, "pending_close", before, item.Status, 0, item.PlannedQuantity, "", "所有部门已完成"); err != nil {
					return err
				}
			}
		}
		return h.createFlowLog(tx, c, item.ID, &task.ID, &task.DepartmentID, action, beforeStatus, task.Status, beforeQuantity, task.CompletedQuantity, "", task.Remark)
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

func (h *Handler) ensureDepartmentTaskAccess(c *echo.Context, departmentID uint) error {
	current := auth.GetCurrentUser(c)
	if current == nil || current.DepartmentID == nil {
		return nil
	}
	if *current.DepartmentID == departmentID {
		return nil
	}
	return echo.NewHTTPError(http.StatusForbidden, "不能操作其他部门任务")
}

func (h *Handler) respondWorkOrder(c *echo.Context, id uint) error {
	var item model.WorkOrder
	if err := h.DB.Preload("DepartmentTasks", func(tx *gorm.DB) *gorm.DB { return tx.Order("id asc") }).First(&item, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

func (h *Handler) createFlowLog(tx *gorm.DB, c *echo.Context, workOrderID uint, taskID *uint, departmentID *uint, action string, before string, after string, quantityBefore int64, quantityAfter int64, reason string, remark string) error {
	log := model.WorkOrderFlowLog{
		WorkOrderID:      workOrderID,
		DepartmentTaskID: taskID,
		DepartmentID:     departmentID,
		Action:           action,
		StatusBefore:     before,
		StatusAfter:      after,
		QuantityBefore:   quantityBefore,
		QuantityAfter:    quantityAfter,
		Reason:           strings.TrimSpace(reason),
		Remark:           strings.TrimSpace(remark),
	}
	if current := auth.GetCurrentUser(c); current != nil {
		log.ActorUserID = &current.ID
		log.ActorUsername = current.Username
		log.ActorTerminalID = current.TerminalID
	}
	if identity, ok := operator.Get(c); ok {
		log.OperatorEmployeeID = &identity.EmployeeID
		log.OperatorEmployeeName = identity.EmployeeName
		log.OperatorDepartmentID = &identity.DepartmentID
		log.OperatorDepartmentName = identity.DepartmentName
	}
	return tx.Create(&log).Error
}

func validateCreateRequest(req createRequest) error {
	itemType := defaultString(req.Type, TypeProduction)
	if itemType != TypeProduction && itemType != TypeGeneral {
		return echo.NewHTTPError(http.StatusBadRequest, "任务类型必须为 production 或 general")
	}
	priority := defaultString(req.Priority, PriorityNormal)
	if priority != PriorityNormal && priority != PriorityUrgent {
		return echo.NewHTTPError(http.StatusBadRequest, "优先级必须为 normal 或 urgent")
	}
	if len(uniqueUint(req.TargetDepartmentIDs)) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择至少一个流转部门")
	}
	if itemType == TypeProduction {
		if req.ProductID == nil || *req.ProductID == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "生产单必须选择仓库产品")
		}
		if req.PlannedQuantity <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "生产单计划数量必须大于 0")
		}
		return nil
	}
	if strings.TrimSpace(req.Title) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "通用任务必须填写标题")
	}
	if strings.TrimSpace(req.Description) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "通用任务必须填写说明")
	}
	return nil
}

func (h *Handler) validateTargetDepartments(c *echo.Context, ids []uint) error {
	return h.validateTargetDepartmentsDB(h.DB, c, ids)
}

func (h *Handler) validateTargetDepartmentsDB(db *gorm.DB, c *echo.Context, ids []uint) error {
	ids = uniqueUint(ids)
	if len(ids) == 0 {
		return nil
	}
	current := auth.GetCurrentUser(c)
	organizationID := uint(1)
	if current != nil && current.OrganizationID != 0 {
		organizationID = current.OrganizationID
	}
	var departments []model.Department
	if err := db.WithContext(c.Request().Context()).Where("id IN ?", ids).Find(&departments).Error; err != nil {
		return err
	}
	byID := make(map[uint]model.Department, len(departments))
	for _, department := range departments {
		byID[department.ID] = department
	}
	for _, id := range ids {
		department, ok := byID[id]
		if !ok || department.OrganizationID != organizationID {
			return echo.NewHTTPError(http.StatusForbidden, "流转部门不属于当前组织")
		}
		if department.Status != model.StatusActive {
			return echo.NewHTTPError(http.StatusConflict, "停用部门不能接收新任务")
		}
	}
	return nil
}

func parseOptionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if value, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return &value, nil
		}
	}
	return nil, echo.NewHTTPError(http.StatusBadRequest, "交期格式应为 YYYY-MM-DD 或 RFC3339")
}

func recordNotFound(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, message)
	}
	return err
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func titleFromRequest(item model.WorkOrder) string {
	if item.Type == TypeProduction {
		return "生产单 - " + item.ProductName
	}
	return "通用任务"
}

func uniqueUint(values []uint) []uint {
	seen := make(map[uint]bool, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func allDepartmentTasksCompleted(tasks []model.DepartmentTask) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Status != DepartmentTaskCompleted {
			return false
		}
	}
	return true
}
