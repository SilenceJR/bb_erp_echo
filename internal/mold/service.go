package mold

import (
	"errors"
	"strings"

	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
)

var (
	ErrMoldNotFound          = errors.New("mold not found")
	ErrMoldNumberConflict    = errors.New("mold number already exists")
	ErrMoldInvalidType       = errors.New("invalid mold type")
	ErrMoldGroupRequired     = errors.New("common group number required")
	ErrMoldGroupForbidden    = errors.New("single mold cannot have common group number")
	ErrMoldLocationRequired  = errors.New("mold location required")
	ErrMoldLocationNotFound  = errors.New("mold location not found")
	ErrMoldLocationDisabled  = errors.New("mold location disabled")
	ErrMoldLocationInUse     = errors.New("mold location is in use")
	ErrMoldSelectionRequired = errors.New("mold selection required")
)

type Input struct {
	MoldNumber    string `json:"mold_number" validate:"required"`
	Model         string `json:"model" validate:"required"`
	MoldType      string `json:"mold_type" validate:"required,oneof=single common"`
	LocationID    uint   `json:"location_id" validate:"required"`
	LocationCode  string `json:"-"`
	CommonGroupNo string `json:"common_group_no"`
	Remark        string `json:"remark"`
}

type ListFilter struct {
	Type       string
	LocationID uint
	GroupNo    string
}

type MoldResponse struct {
	model.Mold
	ImageCount   int64 `json:"image_count"`
	DrawingCount int64 `json:"drawing_count"`
}

// MoldPageResponse 是模具列表分页响应的 Swagger 具体类型。
type MoldPageResponse struct {
	Items    []MoldResponse `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Keyword  string         `json:"keyword,omitempty"`
}

type LocationInput struct {
	Code string `json:"code" validate:"required"`
}

type LocationStatusInput struct {
	Status string `json:"status" validate:"required,oneof=active disabled"`
}

type BulkMoveInput struct {
	MoldIDs    []uint `json:"mold_ids" validate:"required,min=1"`
	LocationID uint   `json:"location_id" validate:"required"`
}

type Service interface {
	List(query pagination.Query, filter ListFilter) (pagination.Result[MoldResponse], error)
	Get(id uint) (MoldResponse, error)
	Create(input Input) (model.Mold, error)
	Update(id uint, input Input) (model.Mold, error)
	Delete(id uint) error
	Locations(includeDisabled bool) ([]model.MoldLocation, error)
	CreateLocation(input LocationInput) (model.MoldLocation, error)
	UpdateLocation(id uint, input LocationStatusInput) (model.MoldLocation, error)
	BulkMove(input BulkMoveInput) error
}

type gormService struct {
	db          *gorm.DB
	storageRoot string
}

var _ Service = (*gormService)(nil)

func NewService(db *gorm.DB) Service { return &gormService{db: db} }

func NewServiceWithStorage(db *gorm.DB, storageRoot string) Service {
	return &gormService{db: db, storageRoot: storageRoot}
}

// SeedLocations 保证全新数据库具备最小固定位置字典。
func SeedLocations(db *gorm.DB) error {
	for _, code := range []string{"A1-1", "B1-1"} {
		item := model.MoldLocation{Code: code, Status: model.MoldLocationActive}
		if err := db.Where("code = ?", code).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *gormService) List(query pagination.Query, filter ListFilter) (pagination.Result[MoldResponse], error) {
	db := s.db.Model(&model.Mold{}).Preload("Location")
	if filter.Type != "" {
		db = db.Where("mold_type = ?", filter.Type)
	}
	if filter.LocationID != 0 {
		db = db.Where("location_id = ?", filter.LocationID)
	}
	if filter.GroupNo != "" {
		db = db.Where("common_group_no LIKE ?", "%"+strings.TrimSpace(filter.GroupNo)+"%")
	}
	db = pagination.ApplyKeyword(db, query.Keyword, "mold_number", "model", "mold_type", "common_group_no", "remark")
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return pagination.Result[MoldResponse]{}, err
	}
	var molds []model.Mold
	if err := db.Order("id desc").Offset(query.Offset).Limit(query.PageSize).Find(&molds).Error; err != nil {
		return pagination.Result[MoldResponse]{}, err
	}
	items, err := s.withCounts(molds)
	if err != nil {
		return pagination.Result[MoldResponse]{}, err
	}
	return pagination.Result[MoldResponse]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Keyword: query.Keyword}, nil
}

func (s *gormService) Get(id uint) (MoldResponse, error) {
	var item model.Mold
	if err := s.db.Preload("Location").First(&item, id).Error; err != nil {
		return MoldResponse{}, mapMoldError(err)
	}
	items, err := s.withCounts([]model.Mold{item})
	if err != nil {
		return MoldResponse{}, err
	}
	return items[0], nil
}

func (s *gormService) withCounts(molds []model.Mold) ([]MoldResponse, error) {
	items := make([]MoldResponse, len(molds))
	for i, item := range molds {
		items[i].Mold = item
		if err := s.db.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Count(&items[i].ImageCount).Error; err != nil {
			return nil, err
		}
		if err := s.db.Model(&model.MoldDrawing{}).Where("mold_id = ?", item.ID).Count(&items[i].DrawingCount).Error; err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *gormService) Create(input Input) (model.Mold, error) {
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return model.Mold{}, err
	}
	if err := s.validateLocation(input.LocationID, false); err != nil {
		return model.Mold{}, err
	}
	item := model.Mold{MoldNumber: input.MoldNumber, Model: input.Model, MoldType: input.MoldType, LocationID: input.LocationID, CommonGroupNo: input.CommonGroupNo, Remark: input.Remark}
	if err := s.db.Create(&item).Error; err != nil {
		return model.Mold{}, mapMoldError(err)
	}
	return item, nil
}

func (s *gormService) Update(id uint, input Input) (model.Mold, error) {
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return model.Mold{}, err
	}
	if err := s.validateLocation(input.LocationID, false); err != nil {
		return model.Mold{}, err
	}
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return model.Mold{}, mapMoldError(err)
	}
	item.MoldNumber, item.Model, item.MoldType = input.MoldNumber, input.Model, input.MoldType
	item.LocationID, item.CommonGroupNo, item.Remark = input.LocationID, input.CommonGroupNo, input.Remark
	if err := s.db.Save(&item).Error; err != nil {
		return model.Mold{}, mapMoldError(err)
	}
	return item, nil
}

func (s *gormService) Delete(id uint) error {
	unlock := filemodule.LockMoldAssetMutation()
	defer unlock()
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return mapMoldError(err)
	}
	var images []model.ImageFile
	var drawings []model.MoldDrawing
	if err := s.db.Where("owner_type = ? AND owner_id = ?", "mold", id).Find(&images).Error; err != nil {
		return err
	}
	if err := s.db.Where("mold_id = ?", id).Find(&drawings).Error; err != nil {
		return err
	}
	paths := make([]string, 0, len(images)*2+len(drawings))
	for _, asset := range images {
		paths = append(paths, asset.StoragePath)
		if asset.PreviewPath != "" {
			paths = append(paths, asset.PreviewPath)
		}
	}
	for _, asset := range drawings {
		paths = append(paths, asset.StoragePath)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("owner_type = ? AND owner_id = ?", "mold", id).Delete(&model.ImageFile{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("mold_id = ?", id).Delete(&model.MoldDrawing{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&item).Error; err != nil {
			return err
		}
		return filemodule.QueueCleanupTasks(tx, paths)
	}); err != nil {
		return err
	}
	filemodule.CleanupStoredPaths(s.storageRoot, s.db, paths)
	return nil
}

func (s *gormService) Locations(includeDisabled bool) ([]model.MoldLocation, error) {
	db := s.db.Model(&model.MoldLocation{})
	if !includeDisabled {
		db = db.Where("status = ?", model.MoldLocationActive)
	}
	var items []model.MoldLocation
	return items, db.Order("code asc, id asc").Find(&items).Error
}

func (s *gormService) CreateLocation(input LocationInput) (model.MoldLocation, error) {
	item := model.MoldLocation{Code: strings.TrimSpace(input.Code), Status: model.MoldLocationActive}
	if item.Code == "" {
		return item, ErrMoldLocationRequired
	}
	if err := s.db.Create(&item).Error; err != nil {
		return item, err
	}
	return item, nil
}

func (s *gormService) UpdateLocation(id uint, input LocationStatusInput) (model.MoldLocation, error) {
	var item model.MoldLocation
	if err := s.db.First(&item, id).Error; err != nil {
		return item, mapMoldError(err)
	}
	if input.Status == model.MoldLocationDisabled {
		var count int64
		if err := s.db.Model(&model.Mold{}).Where("location_id = ?", id).Count(&count).Error; err != nil {
			return item, err
		}
		if count > 0 {
			return item, ErrMoldLocationInUse
		}
	}
	item.Status = input.Status
	return item, s.db.Save(&item).Error
}

func (s *gormService) BulkMove(input BulkMoveInput) error {
	if len(input.MoldIDs) == 0 {
		return ErrMoldSelectionRequired
	}
	if err := s.validateLocation(input.LocationID, false); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Mold{}).Where("id IN ?", input.MoldIDs).Update("location_id", input.LocationID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(uniqueIDs(input.MoldIDs))) {
			return ErrMoldNotFound
		}
		return nil
	})
}

func (s *gormService) validateLocation(id uint, includeDisabled bool) error {
	if id == 0 {
		return ErrMoldLocationRequired
	}
	var item model.MoldLocation
	if err := s.db.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMoldLocationNotFound
		}
		return err
	}
	if !includeDisabled && item.Status != model.MoldLocationActive {
		return ErrMoldLocationDisabled
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.MoldNumber, input.Model, input.MoldType = strings.TrimSpace(input.MoldNumber), strings.TrimSpace(input.Model), strings.ToLower(strings.TrimSpace(input.MoldType))
	input.CommonGroupNo, input.Remark = strings.TrimSpace(input.CommonGroupNo), strings.TrimSpace(input.Remark)
	return input
}

func validateInput(input Input) error {
	if input.MoldNumber == "" || input.Model == "" {
		return errors.New("模具编号和模具型号不能为空")
	}
	if input.MoldType != model.MoldTypeSingle && input.MoldType != model.MoldTypeCommon {
		return ErrMoldInvalidType
	}
	if input.MoldType == model.MoldTypeCommon && input.CommonGroupNo == "" {
		return ErrMoldGroupRequired
	}
	if input.MoldType == model.MoldTypeSingle && input.CommonGroupNo != "" {
		return ErrMoldGroupForbidden
	}
	return nil
}

func uniqueIDs(ids []uint) []uint {
	seen := map[uint]struct{}{}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result
}

func mapMoldError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMoldNotFound
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrMoldNumberConflict
	}
	return err
}
