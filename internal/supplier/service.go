package supplier

import (
	"errors"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("supplier not found")

// Service 是供应商应用服务接口，便于替换存储实现或注入测试替身。
type Service interface {
	List(query pagination.Query) (pagination.Result[model.Supplier], error)
	Create(request supplierRequest) (model.Supplier, error)
	Update(id uint, request supplierRequest) (model.Supplier, error)
}

type gormService struct {
	db *gorm.DB
}

var _ Service = (*gormService)(nil)

func NewService(db *gorm.DB) Service {
	return &gormService{db: db}
}

func (service *gormService) List(query pagination.Query) (pagination.Result[model.Supplier], error) {
	db := service.db.Model(&model.Supplier{})
	db = pagination.ApplyKeyword(db, query.Keyword, "name", "code", "contact", "phone", "address", "status")
	return pagination.Page[model.Supplier](db, query, "id desc", nil)
}

func (service *gormService) Create(request supplierRequest) (model.Supplier, error) {
	status := request.Status
	if status == "" {
		status = model.StatusActive
	}
	item := model.Supplier{Name: request.Name, Code: request.Code, Contact: request.Contact, Phone: request.Phone, Address: request.Address, Status: status}
	return item, service.db.Create(&item).Error
}

func (service *gormService) Update(id uint, request supplierRequest) (model.Supplier, error) {
	var item model.Supplier
	if err := service.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, ErrNotFound
		}
		return item, err
	}
	item.Name, item.Code, item.Contact, item.Phone, item.Address = request.Name, request.Code, request.Contact, request.Phone, request.Address
	if request.Status != "" {
		item.Status = request.Status
	}
	return item, service.db.Save(&item).Error
}
