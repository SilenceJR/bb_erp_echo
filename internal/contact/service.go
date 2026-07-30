package contact

import (
	"errors"

	"bb_erp_echo/internal/model"

	"gorm.io/gorm"
)

var (
	ErrContactNotFound      = errors.New("contact not found")
	ErrCustomerNotFound     = errors.New("customer not found")
	ErrMultiplePrimaryPhone = errors.New("only one primary phone is allowed")
)

// Service 封装联系人及电话明细的一致性规则。
type Service interface {
	List() ([]model.Contact, error)
	Get(id uint) (model.Contact, error)
	Create(req CreateContactRequest) (model.Contact, error)
	Update(id uint, req UpdateContactRequest) (model.Contact, error)
	Delete(id uint) error
}

type gormService struct {
	db *gorm.DB
}

var _ Service = (*gormService)(nil)

func NewService(db *gorm.DB) Service {
	return &gormService{db: db}
}

func (s *gormService) List() ([]model.Contact, error) {
	var items []model.Contact
	err := s.db.Order("id desc").Preload("Phones").Find(&items).Error
	return items, err
}

func (s *gormService) Get(id uint) (model.Contact, error) {
	var item model.Contact
	err := s.db.Preload("Phones").First(&item, id).Error
	return item, mapContactError(err)
}

func (s *gormService) Create(req CreateContactRequest) (model.Contact, error) {
	if err := validatePhones(req.Phones); err != nil {
		return model.Contact{}, err
	}
	item := model.Contact{CustomerID: req.CustomerID, Name: req.Name, Phones: contactPhones(req.Phones)}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureCustomer(tx, req.CustomerID); err != nil {
			return err
		}
		return tx.Create(&item).Error
	})
	return item, err
}

func (s *gormService) Update(id uint, req UpdateContactRequest) (model.Contact, error) {
	if err := validatePhones(req.Phones); err != nil {
		return model.Contact{}, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureCustomer(tx, req.CustomerID); err != nil {
			return err
		}
		var item model.Contact
		if err := tx.First(&item, id).Error; err != nil {
			return mapContactError(err)
		}
		item.CustomerID = req.CustomerID
		item.Name = req.Name
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		// 电话是联系人内部值对象，整体替换时无需保留软删除占位数据。
		if err := tx.Unscoped().Where("contact_id = ?", id).Delete(&model.ContactPhone{}).Error; err != nil {
			return err
		}
		for _, phone := range contactPhones(req.Phones) {
			phone.ContactID = id
			if err := tx.Create(&phone).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Contact{}, err
	}
	return s.Get(id)
}

func (s *gormService) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var item model.Contact
		if err := tx.First(&item, id).Error; err != nil {
			return mapContactError(err)
		}
		if err := tx.Where("contact_id = ?", id).Delete(&model.ContactPhone{}).Error; err != nil {
			return err
		}
		return tx.Delete(&item).Error
	})
}

func ensureCustomer(db *gorm.DB, id uint) error {
	var count int64
	if err := db.Model(&model.Customer{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrCustomerNotFound
	}
	return nil
}

func validatePhones(phones []ContactPhoneRequest) error {
	primaryCount := 0
	for _, phone := range phones {
		if phone.Primary {
			primaryCount++
		}
	}
	if primaryCount > 1 {
		return ErrMultiplePrimaryPhone
	}
	return nil
}

func contactPhones(requests []ContactPhoneRequest) []model.ContactPhone {
	result := make([]model.ContactPhone, 0, len(requests))
	for _, item := range requests {
		result = append(result, model.ContactPhone{Phone: item.Phone, Label: item.Label, Primary: item.Primary})
	}
	return result
}

func mapContactError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrContactNotFound
	}
	return err
}
