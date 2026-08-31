package mold

import (
	"errors"
	"strings"
	"time"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
)

const (
	statusInStock     = "in_stock"
	statusLoaned      = "loaned"
	statusRepairing   = "repairing"
	statusMaintenance = "maintenance"
	statusScrapped    = "scrapped"

	eventCreate      = "create"
	eventLoan        = "loan"
	eventReturn      = "return"
	eventRepair      = "repair"
	eventMaintenance = "maintenance"
)

var (
	ErrMoldNotFound                 = errors.New("mold not found")
	ErrMoldStatusConflict           = errors.New("mold status conflict")
	ErrMoldReturnLocationRequired   = errors.New("mold return location required")
	ErrMoldMaintenanceCycleRequired = errors.New("mold maintenance cycle required")
	ErrCustomerProfileNotFound      = errors.New("customer profile not found")
)

type Transition struct {
	Status       string
	EventType    string
	Location     string
	Counterparty string
	HandlerName  string
	Reason       string
	Description  string
}

type MaintenanceCommand struct {
	Location             string
	HandlerName          string
	Description          string
	MaintenanceCycleDays int
	Completed            bool
}

// Service 是模具台账与状态流转的应用服务接口。
type Service interface {
	List(query pagination.Query, status string) (pagination.Result[model.Mold], error)
	Get(id uint) (model.Mold, error)
	Create(req moldRequest) (model.Mold, error)
	Update(id uint, req moldRequest) (model.Mold, error)
	Delete(id uint) error
	Transition(id uint, command Transition) (model.Mold, error)
	Maintain(id uint, command MaintenanceCommand) (model.Mold, error)
}

type gormService struct {
	db  *gorm.DB
	now func() time.Time
}

var _ Service = (*gormService)(nil)

func NewService(db *gorm.DB) Service {
	return &gormService{db: db, now: time.Now}
}

func (s *gormService) List(query pagination.Query, status string) (pagination.Result[model.Mold], error) {
	db := s.db.Model(&model.Mold{})
	if status != "" {
		db = db.Where("status = ?", status)
	}
	db = pagination.ApplyKeyword(db, query.Keyword, "code", "name", "mold_material", "steel", "size", "manufacturer", "owner", "storage_location", "current_location", "status", "remark")
	return pagination.Page[model.Mold](db, query, "id desc", nil)
}

func (s *gormService) Get(id uint) (model.Mold, error) {
	var item model.Mold
	err := s.db.Preload("Events", func(db *gorm.DB) *gorm.DB {
		return db.Order("id desc")
	}).First(&item, id).Error
	return item, mapMoldError(err)
}

func (s *gormService) Create(req moldRequest) (model.Mold, error) {
	item := moldFromRequest(req)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := validateCustomerProfile(tx, item.CustomerID); err != nil {
			return err
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return s.createEvent(tx, item, eventCreate, "", item.Status, item.CurrentLocation, "", "", "新建模具档案", "")
	})
	return item, err
}

func (s *gormService) Update(id uint, req moldRequest) (model.Mold, error) {
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return item, mapMoldError(err)
	}
	beforeStatus := item.Status
	applyMoldRequest(&item, req)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := validateCustomerProfile(tx, item.CustomerID); err != nil {
			return err
		}
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		if beforeStatus != item.Status {
			return s.createEvent(tx, item, "status_change", beforeStatus, item.Status, item.CurrentLocation, "", "", "更新模具状态", "")
		}
		return nil
	})
	return item, err
}

func validateCustomerProfile(db *gorm.DB, id *uint) error {
	if id == nil {
		return nil
	}
	var profile model.CustomerProfile
	if err := db.Select("id").First(&profile, *id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCustomerProfileNotFound
		}
		return err
	}
	return nil
}

func (s *gormService) Delete(id uint) error {
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return mapMoldError(err)
	}
	return s.db.Delete(&item).Error
}

func (s *gormService) Transition(id uint, command Transition) (model.Mold, error) {
	if command.EventType == eventReturn {
		command.Location = strings.TrimSpace(command.Location)
		if command.Location == "" {
			return model.Mold{}, ErrMoldReturnLocationRequired
		}
	}
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return item, mapMoldError(err)
	}
	expectedStatus, ok := transitionSourceStatus(command.EventType, command.Status)
	if !ok || item.Status != expectedStatus {
		return item, ErrMoldStatusConflict
	}
	beforeStatus := item.Status
	item.Status = command.Status
	if command.Location != "" {
		item.CurrentLocation = command.Location
	}
	now := s.now()
	if command.EventType == eventRepair && command.Status == statusInStock {
		item.LastRepairAt = &now
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": item.Status}
		if command.Location != "" {
			updates["current_location"] = item.CurrentLocation
		}
		if item.LastRepairAt != nil {
			updates["last_repair_at"] = item.LastRepairAt
		}
		result := tx.Model(&model.Mold{}).Where("id = ? AND status = ?", item.ID, beforeStatus).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMoldStatusConflict
		}
		return s.createEvent(tx, item, command.EventType, beforeStatus, item.Status, item.CurrentLocation, command.Counterparty, command.HandlerName, command.Reason, command.Description)
	})
	return item, err
}

func (s *gormService) Maintain(id uint, command MaintenanceCommand) (model.Mold, error) {
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return item, mapMoldError(err)
	}
	expectedStatus := statusInStock
	if command.Completed {
		expectedStatus = statusMaintenance
	}
	if item.Status != expectedStatus {
		return item, ErrMoldStatusConflict
	}
	if command.Completed && command.MaintenanceCycleDays <= 0 && item.MaintenanceCycleDays <= 0 {
		return item, ErrMoldMaintenanceCycleRequired
	}
	beforeStatus := item.Status
	item.Status = statusMaintenance
	if command.Completed {
		item.Status = statusInStock
	}
	if command.Location != "" {
		item.CurrentLocation = command.Location
	}
	if command.MaintenanceCycleDays > 0 {
		item.MaintenanceCycleDays = command.MaintenanceCycleDays
	}
	if command.Completed {
		now := s.now()
		item.LastMaintenanceAt = &now
		if item.MaintenanceCycleDays > 0 {
			next := now.AddDate(0, 0, item.MaintenanceCycleDays)
			item.NextMaintenanceAt = &next
		}
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": item.Status}
		if command.Location != "" {
			updates["current_location"] = item.CurrentLocation
		}
		if command.MaintenanceCycleDays > 0 {
			updates["maintenance_cycle_days"] = item.MaintenanceCycleDays
		}
		if item.LastMaintenanceAt != nil {
			updates["last_maintenance_at"] = item.LastMaintenanceAt
		}
		if item.NextMaintenanceAt != nil {
			updates["next_maintenance_at"] = item.NextMaintenanceAt
		}
		result := tx.Model(&model.Mold{}).Where("id = ? AND status = ?", item.ID, beforeStatus).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMoldStatusConflict
		}
		return s.createEvent(tx, item, eventMaintenance, beforeStatus, item.Status, item.CurrentLocation, "", command.HandlerName, "模具保养", command.Description)
	})
	return item, err
}

func transitionSourceStatus(eventType, nextStatus string) (string, bool) {
	switch eventType {
	case eventLoan:
		return statusInStock, nextStatus == statusLoaned
	case eventReturn:
		return statusLoaned, nextStatus == statusInStock
	case eventRepair:
		switch nextStatus {
		case statusRepairing:
			return statusInStock, true
		case statusInStock:
			return statusRepairing, true
		}
	}
	return "", false
}

func (s *gormService) createEvent(tx *gorm.DB, item model.Mold, eventType string, before string, after string, location string, counterparty string, handlerName string, reason string, description string) error {
	now := s.now()
	event := model.MoldEvent{
		MoldID: item.ID, Type: eventType, StatusBefore: before, StatusAfter: after,
		Location: location, Counterparty: counterparty, HandlerName: handlerName,
		Reason: reason, Description: description, StartedAt: &now,
	}
	if eventType == eventReturn || eventType == eventMaintenance || (eventType == eventRepair && after == statusInStock) {
		event.FinishedAt = &now
	}
	return tx.Create(&event).Error
}

func mapMoldError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMoldNotFound
	}
	return err
}

func moldFromRequest(req moldRequest) model.Mold {
	item := model.Mold{}
	applyMoldRequest(&item, req)
	return item
}

func applyMoldRequest(item *model.Mold, req moldRequest) {
	item.Code = req.Code
	item.Name = req.Name
	item.CustomerID = req.CustomerID
	item.ProductID = req.ProductID
	item.CavityCount = req.CavityCount
	item.MoldMaterial = req.MoldMaterial
	item.Steel = req.Steel
	item.Size = req.Size
	item.WeightGram = req.WeightGram
	item.Manufacturer = req.Manufacturer
	item.Owner = req.Owner
	item.StorageLocation = req.StorageLocation
	item.CurrentLocation = req.CurrentLocation
	item.MaintenanceCycleDays = req.MaintenanceCycleDays
	item.Remark = req.Remark
	if req.Status != "" {
		item.Status = req.Status
	}
	if item.Status == "" {
		item.Status = statusInStock
	}
	if item.CavityCount <= 0 {
		item.CavityCount = 1
	}
	if item.CurrentLocation == "" {
		item.CurrentLocation = item.StorageLocation
	}
}
