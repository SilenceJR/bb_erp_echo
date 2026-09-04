package customer

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/gorm"
)

var (
	ErrNotFound           = errors.New("customer record not found")
	ErrCodeConflict       = errors.New("customer code already exists")
	ErrCodeHasProfiles    = errors.New("customer code has profiles")
	ErrProfileReferenced  = errors.New("customer profile is referenced")
	ErrReplacementNeeded  = errors.New("replacement default profile required")
	ErrInvalidReplacement = errors.New("invalid replacement profile")
)

var codePattern = regexp.MustCompile(`(?i)^(?:BB-)?([0-9]+)$`)

type ProfileInput struct {
	CustomerCodeID uint   `json:"customer_code_id" validate:"required"`
	ShortName      string `json:"short_name"`
	Name           string `json:"name"`
	Address        string `json:"address"`
	Phone          string `json:"phone"`
	ContactName    string `json:"contact_name"`
	ContactPhone   string `json:"contact_phone"`
	Salesperson    string `json:"salesperson"`
}

type ProfileUpdate struct {
	ShortName    string `json:"short_name"`
	Name         string `json:"name"`
	Address      string `json:"address"`
	Phone        string `json:"phone"`
	ContactName  string `json:"contact_name"`
	ContactPhone string `json:"contact_phone"`
	Salesperson  string `json:"salesperson"`
}

type ProfileResponse struct {
	model.CustomerProfile
	Code string `json:"code"`
}

type CodeResponse struct {
	model.CustomerCode
	ProfileCount   int64            `json:"profile_count"`
	DefaultProfile *ProfileResponse `json:"default_profile,omitempty"`
}

type OptionResponse struct {
	ID        uint   `json:"id"`
	Code      string `json:"code"`
	ShortName string `json:"short_name"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

// CodePage 和 ProfilePage 仅用于稳定描述分页 OpenAPI 契约。
type CodePage struct {
	Items    []CodeResponse `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	Keyword  string         `json:"keyword,omitempty"`
}

type ProfilePage struct {
	Items    []ProfileResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Keyword  string            `json:"keyword,omitempty"`
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func NormalizeCode(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	m := codePattern.FindStringSubmatch(raw)
	if len(m) != 2 {
		return "", fmt.Errorf("客户编码必须为 BB- 加正整数")
	}
	n, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil || n == 0 {
		return "", fmt.Errorf("客户编码数字必须大于 0")
	}
	digits := strconv.FormatUint(n, 10)
	if len(digits) < 3 {
		digits = strings.Repeat("0", 3-len(digits)) + digits
	}
	return "BB-" + digits, nil
}

func (s *Service) NextCode() (string, error) {
	var codes []string
	if err := s.db.Model(&model.CustomerCode{}).Pluck("code", &codes).Error; err != nil {
		return "", err
	}
	var max uint64
	for _, code := range codes {
		m := codePattern.FindStringSubmatch(code)
		if len(m) != 2 {
			continue
		}
		n, err := strconv.ParseUint(m[1], 10, 64)
		if err == nil && n > max {
			max = n
		}
	}
	return NormalizeCode(strconv.FormatUint(max+1, 10))
}

func (s *Service) CreateCode(raw string) (CodeResponse, error) {
	var result CodeResponse
	// 自动编号允许有限重试，覆盖多个并发请求读取到相同建议值的情况。
	for attempt := 0; attempt < 16; attempt++ {
		code := raw
		var err error
		if strings.TrimSpace(code) == "" {
			code, err = s.NextCode()
		} else {
			code, err = NormalizeCode(code)
		}
		if err != nil {
			return result, err
		}
		item := model.CustomerCode{Code: code}
		if err = s.db.Create(&item).Error; err == nil {
			result.CustomerCode = item
			return result, nil
		} else if !isUniqueError(err) || strings.TrimSpace(raw) != "" {
			if isUniqueError(err) {
				return result, ErrCodeConflict
			}
			return result, err
		}
	}
	return result, ErrCodeConflict
}

func (s *Service) UpdateCode(id uint, raw string) (CodeResponse, error) {
	var item model.CustomerCode
	if err := s.db.First(&item, id).Error; err != nil {
		return CodeResponse{}, mapNotFound(err)
	}
	code, err := NormalizeCode(raw)
	if err != nil {
		return CodeResponse{}, err
	}
	item.Code = code
	if err = s.db.Save(&item).Error; err != nil {
		if isUniqueError(err) {
			return CodeResponse{}, ErrCodeConflict
		}
		return CodeResponse{}, err
	}
	return s.getCodeResponse(item.ID)
}

func (s *Service) DeleteCode(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var item model.CustomerCode
		if err := tx.First(&item, id).Error; err != nil {
			return mapNotFound(err)
		}
		var count int64
		if err := tx.Model(&model.CustomerProfile{}).Where("customer_code_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCodeHasProfiles
		}
		return tx.Delete(&item).Error
	})
}

func (s *Service) ListCodes(query pagination.Query, filter string) (pagination.Result[CodeResponse], error) {
	db := s.db.Model(&model.CustomerCode{})
	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("customer_codes.code LIKE ? OR EXISTS (SELECT 1 FROM customer_profiles p WHERE p.customer_code_id = customer_codes.id AND (p.short_name LIKE ? OR p.name LIKE ? OR p.address LIKE ? OR p.phone LIKE ? OR p.contact_name LIKE ? OR p.contact_phone LIKE ? OR p.salesperson LIKE ?))", like, like, like, like, like, like, like, like)
	}
	switch filter {
	case "multiple":
		db = db.Where("(SELECT COUNT(1) FROM customer_profiles p WHERE p.customer_code_id = customer_codes.id) > 1")
	case "empty":
		db = db.Where("NOT EXISTS (SELECT 1 FROM customer_profiles p WHERE p.customer_code_id = customer_codes.id)")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return pagination.Result[CodeResponse]{}, err
	}
	var codes []model.CustomerCode
	if err := db.Preload("Profiles", func(tx *gorm.DB) *gorm.DB { return tx.Order("is_default desc, id asc") }).Order("CAST(SUBSTR(customer_codes.code, 4) AS INTEGER) asc").Offset(query.Offset).Limit(query.PageSize).Find(&codes).Error; err != nil {
		return pagination.Result[CodeResponse]{}, err
	}
	items := make([]CodeResponse, 0, len(codes))
	for _, code := range codes {
		items = append(items, buildCodeResponse(code))
	}
	return pagination.Result[CodeResponse]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Keyword: query.Keyword}, nil
}

func buildCodeResponse(code model.CustomerCode) CodeResponse {
	r := CodeResponse{CustomerCode: code, ProfileCount: int64(len(code.Profiles))}
	for _, p := range code.Profiles {
		if p.IsDefault {
			v := ProfileResponse{CustomerProfile: p, Code: code.Code}
			r.DefaultProfile = &v
			break
		}
	}
	return r
}

func (s *Service) getCodeResponse(id uint) (CodeResponse, error) {
	var item model.CustomerCode
	if err := s.db.Preload("Profiles", func(tx *gorm.DB) *gorm.DB { return tx.Order("is_default desc, id asc") }).First(&item, id).Error; err != nil {
		return CodeResponse{}, mapNotFound(err)
	}
	return buildCodeResponse(item), nil
}

func trimInput(in ProfileInput) ProfileInput {
	in.ShortName = strings.TrimSpace(in.ShortName)
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	in.Phone = strings.TrimSpace(in.Phone)
	in.ContactName = strings.TrimSpace(in.ContactName)
	in.ContactPhone = strings.TrimSpace(in.ContactPhone)
	in.Salesperson = strings.TrimSpace(in.Salesperson)
	return in
}

func (s *Service) CreateProfile(in ProfileInput) (ProfileResponse, error) {
	in = trimInput(in)
	var out ProfileResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var code model.CustomerCode
		if err := tx.First(&code, in.CustomerCodeID).Error; err != nil {
			return mapNotFound(err)
		}
		var count int64
		if err := tx.Model(&model.CustomerProfile{}).Where("customer_code_id = ?", code.ID).Count(&count).Error; err != nil {
			return err
		}
		item := model.CustomerProfile{CustomerCodeID: code.ID, ShortName: in.ShortName, Name: in.Name, Address: in.Address, Phone: in.Phone, ContactName: in.ContactName, ContactPhone: in.ContactPhone, Salesperson: in.Salesperson, IsDefault: count == 0}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		out = ProfileResponse{CustomerProfile: item, Code: code.Code}
		return nil
	})
	return out, err
}

func (s *Service) GetProfile(id uint) (ProfileResponse, error) {
	var item model.CustomerProfile
	if err := s.db.Preload("CustomerCode").First(&item, id).Error; err != nil {
		return ProfileResponse{}, mapNotFound(err)
	}
	return ProfileResponse{CustomerProfile: item, Code: item.CustomerCode.Code}, nil
}

func (s *Service) UpdateProfile(id uint, in ProfileUpdate) (ProfileResponse, error) {
	var item model.CustomerProfile
	if err := s.db.Preload("CustomerCode").First(&item, id).Error; err != nil {
		return ProfileResponse{}, mapNotFound(err)
	}
	v := trimInput(ProfileInput{ShortName: in.ShortName, Name: in.Name, Address: in.Address, Phone: in.Phone, ContactName: in.ContactName, ContactPhone: in.ContactPhone, Salesperson: in.Salesperson})
	item.ShortName, item.Name, item.Address, item.Phone = v.ShortName, v.Name, v.Address, v.Phone
	item.ContactName, item.ContactPhone, item.Salesperson = v.ContactName, v.ContactPhone, v.Salesperson
	if err := s.db.Save(&item).Error; err != nil {
		return ProfileResponse{}, err
	}
	return ProfileResponse{CustomerProfile: item, Code: item.CustomerCode.Code}, nil
}

func (s *Service) SetDefault(id uint) (ProfileResponse, error) {
	var out ProfileResponse
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var item model.CustomerProfile
		if err := tx.Preload("CustomerCode").First(&item, id).Error; err != nil {
			return mapNotFound(err)
		}
		if err := tx.Model(&model.CustomerProfile{}).Where("customer_code_id = ?", item.CustomerCodeID).Update("is_default", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CustomerProfile{}).Where("id = ?", item.ID).Update("is_default", true).Error; err != nil {
			return err
		}
		item.IsDefault = true
		out = ProfileResponse{CustomerProfile: item, Code: item.CustomerCode.Code}
		return nil
	})
	return out, err
}

func (s *Service) DeleteProfile(id uint, replacementID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var item model.CustomerProfile
		if err := tx.First(&item, id).Error; err != nil {
			return mapNotFound(err)
		}
		if referenced, err := profileReferenced(tx, id); err != nil {
			return err
		} else if referenced {
			return ErrProfileReferenced
		}
		var count int64
		if err := tx.Model(&model.CustomerProfile{}).Where("customer_code_id = ?", item.CustomerCodeID).Count(&count).Error; err != nil {
			return err
		}
		if item.IsDefault && count > 1 {
			if replacementID == 0 {
				return ErrReplacementNeeded
			}
			var replacement model.CustomerProfile
			if err := tx.First(&replacement, replacementID).Error; err != nil || replacement.CustomerCodeID != item.CustomerCodeID || replacement.ID == item.ID {
				return ErrInvalidReplacement
			}
			// 条件唯一索引不允许两个默认值短暂共存，因此先撤销旧默认，
			// 再指定替代资料；整个切换和删除仍在同一事务内完成。
			if err := tx.Model(&model.CustomerProfile{}).Where("id = ?", item.ID).Update("is_default", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.CustomerProfile{}).Where("id = ?", replacement.ID).Update("is_default", true).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&item).Error
	})
}

func profileReferenced(db *gorm.DB, id uint) (bool, error) {
	checks := []any{&model.InventoryDocument{}, &model.WorkOrder{}}
	for _, table := range checks {
		// 客户模块单元测试可以只迁移本模块表；完整应用中新业务表存在时
		// 才执行引用检查。
		if !db.Migrator().HasTable(table) {
			continue
		}
		var count int64
		// 历史业务记录即使已软删除仍然保留客户资料引用，因此按 Unscoped
		// 检查，避免物理删除客户资料后破坏历史展示。
		if err := db.Unscoped().Model(table).Where("customer_id = ?", id).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) ListProfiles(query pagination.Query) (pagination.Result[ProfileResponse], error) {
	db := s.db.Model(&model.CustomerProfile{}).Joins("JOIN customer_codes ON customer_codes.id = customer_profiles.customer_code_id")
	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("customer_codes.code LIKE ? OR customer_profiles.short_name LIKE ? OR customer_profiles.name LIKE ? OR customer_profiles.address LIKE ? OR customer_profiles.phone LIKE ? OR customer_profiles.contact_name LIKE ? OR customer_profiles.contact_phone LIKE ? OR customer_profiles.salesperson LIKE ?", like, like, like, like, like, like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return pagination.Result[ProfileResponse]{}, err
	}
	var items []model.CustomerProfile
	if err := db.Preload("CustomerCode").Order("CAST(SUBSTR(customer_codes.code, 4) AS INTEGER) asc, customer_profiles.is_default desc, customer_profiles.id asc").Offset(query.Offset).Limit(query.PageSize).Find(&items).Error; err != nil {
		return pagination.Result[ProfileResponse]{}, err
	}
	out := make([]ProfileResponse, 0, len(items))
	for _, item := range items {
		out = append(out, ProfileResponse{CustomerProfile: item, Code: item.CustomerCode.Code})
	}
	return pagination.Result[ProfileResponse]{Items: out, Total: total, Page: query.Page, PageSize: query.PageSize, Keyword: query.Keyword}, nil
}

func (s *Service) Options(keyword string) ([]OptionResponse, error) {
	db := s.db.Model(&model.CustomerProfile{}).Joins("JOIN customer_codes ON customer_codes.id = customer_profiles.customer_code_id")
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("customer_codes.code LIKE ? OR customer_profiles.short_name LIKE ? OR customer_profiles.name LIKE ?", like, like, like)
	}
	var rows []struct {
		ID                    uint
		Code, ShortName, Name string
		IsDefault             bool
	}
	if err := db.Select("customer_profiles.id, customer_codes.code, customer_profiles.short_name, customer_profiles.name, customer_profiles.is_default").Order("CAST(SUBSTR(customer_codes.code, 4) AS INTEGER) asc, customer_profiles.is_default desc").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OptionResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, OptionResponse{ID: r.ID, Code: r.Code, ShortName: r.ShortName, Name: r.Name, IsDefault: r.IsDefault})
	}
	return out, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
func isUniqueError(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate")
}
