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
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v4"
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
	ProductName         string `json:"product_name"`
	PlannedQuantity     int64  `json:"planned_quantity"`
	Unit                string `json:"unit"`
	DueAt               string `json:"due_at"`
	Priority            string `json:"priority"`
	Description         string `json:"description"`
	TargetDepartmentIDs []uint `json:"target_department_ids"`
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

type urgentRequest struct {
	Urgent bool `json:"urgent"`
}

type completeRequest struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}

type partialCompleteRequest struct {
	CompletedQuantity int64  `json:"completed_quantity"`
	Remark            string `json:"remark"`
}

type remarkRequest struct {
	Remark string `json:"remark"`
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
func (h *Handler) List(c echo.Context) error {
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
// @Tags workorder
// @Security BearerAuth
// @Param body body createRequest true "任务单创建参数"
// @Success 201 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/workorder [post]
func (h *Handler) Create(c echo.Context) error {
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
	current := auth.GetCurrentUser(c)
	item := model.WorkOrder{
		Code:            strings.TrimSpace(req.Code),
		Title:           strings.TrimSpace(req.Title),
		Type:            defaultString(req.Type, TypeProduction),
		Status:          StatusDraft,
		Priority:        defaultString(req.Priority, PriorityNormal),
		CustomerID:      req.CustomerID,
		ProductName:     strings.TrimSpace(req.ProductName),
		PlannedQuantity: req.PlannedQuantity,
		Unit:            strings.TrimSpace(req.Unit),
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
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
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

// Dispatch 派发任务，目标部门自动进入已收到状态。
// @Summary 派发任务单
// @Tags workorder
// @Security BearerAuth
// @Param id path int true "任务单 ID"
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/dispatch [post]
func (h *Handler) Dispatch(c echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.Preload("DepartmentTasks").First(&item, id).Error; err != nil {
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
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		item.Status = StatusProcessing
		item.DispatchedAt = &now
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		for _, task := range item.DepartmentTasks {
			taskBefore := task.Status
			task.Status = DepartmentTaskReceived
			task.AcceptedAt = &now
			if err := tx.Save(&task).Error; err != nil {
				return err
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
func (h *Handler) Pause(c echo.Context) error {
	var req reasonRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "暂停必须填写原因")
	}
	return h.changeWorkOrderStatus(c, StatusPaused, "pause", req.Reason, func(item model.WorkOrder) error {
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
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/{id}/resume [post]
func (h *Handler) Resume(c echo.Context) error {
	return h.changeWorkOrderStatus(c, StatusProcessing, "resume", "", func(item model.WorkOrder) error {
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
func (h *Handler) SetUrgent(c echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req urgentRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.First(&item, id).Error; err != nil {
		return recordNotFound(err, "任务单不存在")
	}
	before := item.Priority
	if req.Urgent {
		item.Priority = PriorityUrgent
	} else {
		item.Priority = PriorityNormal
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
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
func (h *Handler) Complete(c echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req completeRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.Preload("DepartmentTasks").First(&item, id).Error; err != nil {
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
	item.Status = targetStatus
	item.CompletedAt = &now
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
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
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/department-tasks/{id}/start [post]
func (h *Handler) StartDepartmentTask(c echo.Context) error {
	var req remarkRequest
	_ = c.Bind(&req)
	return h.updateDepartmentTask(c, "department_start", func(item model.WorkOrder, task *model.DepartmentTask) error {
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
func (h *Handler) PartialCompleteDepartmentTask(c echo.Context) error {
	var req partialCompleteRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.updateDepartmentTask(c, "department_partial_complete", func(item model.WorkOrder, task *model.DepartmentTask) error {
		if req.CompletedQuantity <= 0 || req.CompletedQuantity >= task.PlannedQuantity {
			return echo.NewHTTPError(http.StatusBadRequest, "部分完成数量必须大于 0 且小于计划数量")
		}
		if req.CompletedQuantity < task.CompletedQuantity {
			return echo.NewHTTPError(http.StatusBadRequest, "已完成数量不能小于当前已完成数量")
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
// @Success 200 {object} model.WorkOrder
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/workorder/department-tasks/{id}/complete [post]
func (h *Handler) CompleteDepartmentTask(c echo.Context) error {
	var req remarkRequest
	_ = c.Bind(&req)
	return h.updateDepartmentTask(c, "department_complete", func(item model.WorkOrder, task *model.DepartmentTask) error {
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
func (h *Handler) ListLogs(c echo.Context) error {
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

func (h *Handler) changeWorkOrderStatus(c echo.Context, targetStatus string, action string, reason string, validate func(model.WorkOrder) error) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.WorkOrder
	if err := h.DB.First(&item, id).Error; err != nil {
		return recordNotFound(err, "任务单不存在")
	}
	if err := validate(item); err != nil {
		return err
	}
	before := item.Status
	item.Status = targetStatus
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		return h.createFlowLog(tx, c, item.ID, nil, nil, action, before, item.Status, 0, item.PlannedQuantity, reason, "")
	}); err != nil {
		return err
	}
	return h.respondWorkOrder(c, item.ID)
}

func (h *Handler) updateDepartmentTask(c echo.Context, action string, mutate func(model.WorkOrder, *model.DepartmentTask) error) error {
	taskID, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var task model.DepartmentTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		return recordNotFound(err, "部门子任务不存在")
	}
	var item model.WorkOrder
	if err := h.DB.Preload("DepartmentTasks").First(&item, task.WorkOrderID).Error; err != nil {
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
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&task).Error; err != nil {
			return err
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
				item.Status = StatusPendingClose
				if err := tx.Save(&item).Error; err != nil {
					return err
				}
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

func (h *Handler) ensureDepartmentTaskAccess(c echo.Context, departmentID uint) error {
	current := auth.GetCurrentUser(c)
	if current == nil || current.DepartmentID == nil {
		return nil
	}
	if *current.DepartmentID == departmentID {
		return nil
	}
	return echo.NewHTTPError(http.StatusForbidden, "不能操作其他部门任务")
}

func (h *Handler) respondWorkOrder(c echo.Context, id uint) error {
	var item model.WorkOrder
	if err := h.DB.Preload("DepartmentTasks", func(tx *gorm.DB) *gorm.DB { return tx.Order("id asc") }).First(&item, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

func (h *Handler) createFlowLog(tx *gorm.DB, c echo.Context, workOrderID uint, taskID *uint, departmentID *uint, action string, before string, after string, quantityBefore int64, quantityAfter int64, reason string, remark string) error {
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
		if strings.TrimSpace(req.ProductName) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "生产单必须填写产品")
		}
		if req.PlannedQuantity <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "生产单计划数量必须大于 0")
		}
		if strings.TrimSpace(req.Unit) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "生产单必须填写单位")
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
