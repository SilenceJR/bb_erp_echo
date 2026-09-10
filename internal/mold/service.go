package mold

import (
	"errors"
	"fmt"
	"strings"

	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrMoldNotFound          = errors.New("mold not found")
	ErrMoldInvalidType       = errors.New("invalid mold type")
	ErrMoldGroupRequired     = errors.New("common group number required")
	ErrMoldGroupForbidden    = errors.New("single mold cannot have common group number")
	ErrMoldLocationRequired  = errors.New("mold location required")
	ErrMoldLocationNotFound  = errors.New("mold location not found")
	ErrMoldLocationDisabled  = errors.New("mold location disabled")
	ErrMoldLocationInUse     = errors.New("mold location is in use")
	ErrMoldSelectionRequired = errors.New("mold selection required")
	ErrMoldLocationZone      = errors.New("mold location zone invalid")
	ErrMoldLocationRange     = errors.New("mold location range invalid")
	ErrProductRequired       = errors.New("product required")
	ErrProductNotFound       = errors.New("product not found")
	ErrProductDisabled       = errors.New("product disabled")
)

type Input struct {
	ProductID     uint   `json:"product_id" validate:"required"`
	MoldType      string `json:"mold_type" validate:"required,oneof=single common"`
	CavityCount   string `json:"cavity_count" validate:"required,max=60"`
	LocationID    uint   `json:"location_id" validate:"required"`
	CommonGroupNo string `json:"common_group_no"`
	Remark        string `json:"remark"`
}

type ListFilter struct {
	ProductModel string
	Type         string
	LocationID   uint
	GroupNo      string
}

type MoldResponse struct {
	model.Mold
	ProductModel string `json:"product_model"`
	ImageCount   int64  `json:"image_count"`
	DrawingCount int64  `json:"drawing_count"`
}

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

type BulkLocationInput struct {
	Zone    string `json:"zone" validate:"required"`
	Rows    int    `json:"rows"`
	Columns int    `json:"columns"`
}

type BulkLocationResult struct {
	Created int `json:"created"`
}

type BulkMoveInput struct {
	MoldIDs    []uint `json:"mold_ids" validate:"required,min=1"`
	LocationID uint   `json:"location_id" validate:"required"`
}

type gormService struct {
	db          *gorm.DB
	storageRoot string
}

func NewService(db *gorm.DB) *gormService { return &gormService{db: db} }
func NewServiceWithStorage(db *gorm.DB, storageRoot string) *gormService {
	return &gormService{db: db, storageRoot: storageRoot}
}

// SeedLocations 补齐默认货架和卡板位置，但不重启用已有停用位置。
func SeedLocations(db *gorm.DB) error {
	locations := defaultMoldLocations()
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "code"}}, DoNothing: true}).Create(&locations).Error
	})
}

func defaultMoldLocations() []model.MoldLocation {
	locations := make([]model.MoldLocation, 0, 101)
	for _, zone := range []string{"A", "B", "C", "D"} {
		maxRow := 6
		if zone == "A" {
			maxRow = 7
		}
		for row := 1; row <= maxRow; row++ {
			for column := 1; column <= 4; column++ {
				locations = append(locations, model.MoldLocation{Code: fmt.Sprintf("%s%d-%d", zone, row, column), Status: model.MoldLocationActive})
			}
		}
	}
	return append(locations, model.MoldLocation{Code: model.MoldLocationPallet, Status: model.MoldLocationActive})
}

func (s *gormService) List(query pagination.Query, filter ListFilter) (pagination.Result[MoldResponse], error) {
	db := s.db.Model(&model.Mold{}).Joins("JOIN products ON products.id = molds.product_id AND products.deleted_at IS NULL")
	if query.Keyword != "" {
		db = pagination.ApplyKeyword(db, query.Keyword, "products.product_model", "molds.remark", "molds.common_group_no")
	}
	if filter.ProductModel != "" {
		db = db.Where("products.product_model = ?", strings.TrimSpace(filter.ProductModel))
	}
	if filter.Type != "" {
		db = db.Where("molds.mold_type = ?", filter.Type)
	}
	if filter.LocationID != 0 {
		db = db.Where("molds.location_id = ?", filter.LocationID)
	}
	if filter.GroupNo != "" {
		db = db.Where("molds.common_group_no = ?", strings.TrimSpace(filter.GroupNo))
	}
	result, err := pagination.Page[model.Mold](db.Preload("Product").Preload("Location"), query, "molds.id desc", nil)
	if err != nil {
		return pagination.Result[MoldResponse]{}, err
	}
	items := make([]MoldResponse, 0, len(result.Items))
	for _, item := range result.Items {
		response := MoldResponse{Mold: item, ProductModel: item.Product.ProductModel}
		if err := s.db.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Count(&response.ImageCount).Error; err != nil {
			return pagination.Result[MoldResponse]{}, err
		}
		if err := s.db.Model(&model.MoldDrawing{}).Where("mold_id = ?", item.ID).Count(&response.DrawingCount).Error; err != nil {
			return pagination.Result[MoldResponse]{}, err
		}
		items = append(items, response)
	}
	return pagination.Result[MoldResponse]{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, Keyword: result.Keyword}, nil
}

func (s *gormService) Get(id uint) (MoldResponse, error) {
	var item model.Mold
	if err := s.db.Preload("Product").Preload("Location").First(&item, id).Error; err != nil {
		return MoldResponse{}, mapMoldError(err)
	}
	response := MoldResponse{Mold: item, ProductModel: item.Product.ProductModel}
	if err := s.db.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Count(&response.ImageCount).Error; err != nil {
		return MoldResponse{}, err
	}
	if err := s.db.Model(&model.MoldDrawing{}).Where("mold_id = ?", item.ID).Count(&response.DrawingCount).Error; err != nil {
		return MoldResponse{}, err
	}
	return response, nil
}

func (s *gormService) Create(input Input) (model.Mold, error) {
	input = normalizeInput(input)
	if err := s.validateInput(input); err != nil {
		return model.Mold{}, err
	}
	if _, err := s.validateProduct(input.ProductID); err != nil {
		return model.Mold{}, err
	}
	if err := s.validateLocation(input.LocationID, false); err != nil {
		return model.Mold{}, err
	}
	item := model.Mold{ProductID: input.ProductID, MoldType: input.MoldType, CavityCount: input.CavityCount, LocationID: input.LocationID, CommonGroupNo: input.CommonGroupNo, Remark: input.Remark}
	if err := s.db.Create(&item).Error; err != nil {
		return item, err
	}
	return item, nil
}

func (s *gormService) Update(id uint, input Input) (model.Mold, error) {
	unlock := filemodule.LockMoldAssetMutation()
	defer unlock()
	input = normalizeInput(input)
	if err := s.validateInput(input); err != nil {
		return model.Mold{}, err
	}
	if _, err := s.validateProduct(input.ProductID); err != nil {
		return model.Mold{}, err
	}
	if err := s.validateLocation(input.LocationID, false); err != nil {
		return model.Mold{}, err
	}
	var item model.Mold
	if err := s.db.First(&item, id).Error; err != nil {
		return model.Mold{}, mapMoldError(err)
	}
	item.ProductID, item.MoldType, item.CavityCount = input.ProductID, input.MoldType, input.CavityCount
	item.LocationID, item.CommonGroupNo, item.Remark = input.LocationID, input.CommonGroupNo, input.Remark
	return item, s.db.Save(&item).Error
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
	return item, s.db.Create(&item).Error
}

func (s *gormService) BulkCreateLocations(input BulkLocationInput) (BulkLocationResult, error) {
	input.Zone = strings.ToUpper(strings.TrimSpace(input.Zone))
	if !validLocationZone(input.Zone) {
		return BulkLocationResult{}, ErrMoldLocationZone
	}
	if input.Rows < 1 || input.Rows > 100 || input.Columns < 1 || input.Columns > 100 {
		return BulkLocationResult{}, ErrMoldLocationRange
	}
	locations := make([]model.MoldLocation, 0, input.Rows*input.Columns)
	for row := 1; row <= input.Rows; row++ {
		for column := 1; column <= input.Columns; column++ {
			locations = append(locations, model.MoldLocation{Code: fmt.Sprintf("%s%d-%d", input.Zone, row, column), Status: model.MoldLocationActive})
		}
	}
	var result BulkLocationResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "code"}}, DoNothing: true}).CreateInBatches(&locations, 500)
		if created.Error != nil {
			return created.Error
		}
		result.Created = int(created.RowsAffected)
		return nil
	})
	return result, err
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
		result := tx.Model(&model.Mold{}).Where("id IN ?", uniqueIDs(input.MoldIDs)).Update("location_id", input.LocationID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(uniqueIDs(input.MoldIDs))) {
			return ErrMoldNotFound
		}
		return nil
	})
}

func (s *gormService) validateInput(input Input) error {
	if input.ProductID == 0 {
		return ErrProductRequired
	}
	if input.MoldType != model.MoldTypeSingle && input.MoldType != model.MoldTypeCommon {
		return ErrMoldInvalidType
	}
	if input.CavityCount == "" {
		return errors.New("模穴数不能为空")
	}
	if input.MoldType == model.MoldTypeCommon && input.CommonGroupNo == "" {
		return ErrMoldGroupRequired
	}
	if input.MoldType == model.MoldTypeSingle && input.CommonGroupNo != "" {
		return ErrMoldGroupForbidden
	}
	return nil
}

func (s *gormService) validateProduct(id uint) (model.Product, error) {
	var item model.Product
	if err := s.db.First(&item, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrProductNotFound
	} else if err != nil {
		return item, err
	}
	if item.Status != model.StatusActive {
		return item, ErrProductDisabled
	}
	return item, nil
}

func (s *gormService) validateLocation(id uint, includeDisabled bool) error {
	if id == 0 {
		return ErrMoldLocationRequired
	}
	var item model.MoldLocation
	if err := s.db.First(&item, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMoldLocationNotFound
	} else if err != nil {
		return err
	}
	if !includeDisabled && item.Status != model.MoldLocationActive {
		return ErrMoldLocationDisabled
	}
	return nil
}

func normalizeInput(input Input) Input {
	input.CavityCount = strings.TrimSpace(input.CavityCount)
	input.CommonGroupNo = strings.TrimSpace(input.CommonGroupNo)
	input.Remark = strings.TrimSpace(input.Remark)
	return input
}

func mapMoldError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMoldNotFound
	}
	return err
}

func validLocationZone(value string) bool {
	if len(value) < 1 || len(value) > 8 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func uniqueIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
