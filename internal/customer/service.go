package customer

import (
	"errors"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
)

var ErrCustomerNotFound = errors.New("customer not found")

// Service 是客户模块对传输层暴露的应用服务接口。
type Service interface {
	List(query pagination.Query) (pagination.Result[model.Customer], error)
	Create(req CreateCustomerRequest) (model.Customer, error)
	Update(id uint, req UpdateCustomerRequest) (model.Customer, error)
	Delete(id uint) error
}

type gormService struct {
	db *gorm.DB
}

var _ Service = (*gormService)(nil)

func NewService(db *gorm.DB) Service {
	return &gormService{db: db}
}

func (s *gormService) List(query pagination.Query) (pagination.Result[model.Customer], error) {
	db := s.db.Model(&model.Customer{})
	db = pagination.ApplyKeyword(db, query.Keyword, "name", "code", "phone", "address")
	return pagination.Page[model.Customer](db, query, "id desc", func(tx *gorm.DB) *gorm.DB {
		return tx.Preload("Contacts.Phones")
	})
}

func (s *gormService) Create(req CreateCustomerRequest) (model.Customer, error) {
	item := model.Customer{Name: req.Name, Code: req.Code, Phone: req.Phone}
	return item, s.db.Create(&item).Error
}

func (s *gormService) Update(id uint, req UpdateCustomerRequest) (model.Customer, error) {
	var item model.Customer
	if err := s.db.First(&item, id).Error; err != nil {
		return item, mapCustomerError(err)
	}
	item.Name = req.Name
	item.Code = req.Code
	item.Phone = req.Phone
	item.Address = req.Address
	return item, s.db.Save(&item).Error
}

func (s *gormService) Delete(id uint) error {
	var item model.Customer
	if err := s.db.First(&item, id).Error; err != nil {
		return mapCustomerError(err)
	}
	return s.db.Delete(&item).Error
}

func mapCustomerError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCustomerNotFound
	}
	return err
}
