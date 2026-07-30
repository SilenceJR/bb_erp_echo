package warehouse

import (
	"net/http"

	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const (
	tabProduct            = "product"
	tabProductionMaterial = "production_material"
	tabRegularProduct     = "regular_product"
	tabDailySupply        = "daily_supply"
)

// CatalogItemInput 是仓库标签页下统一的物品录入请求。
type CatalogItemInput struct {
	Tab         string `json:"tab" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Code        string `json:"code" validate:"required"`
	Unit        string `json:"unit"`
	Spec        string `json:"spec"`
	SafetyStock int64  `json:"safety_stock"`
	DefaultCost int64  `json:"default_cost"`
}

// CatalogItem 是仓库标签页统一返回结构。
type CatalogItem struct {
	ID          uint   `json:"id"`
	ItemType    string `json:"item_type"`
	Tab         string `json:"tab"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Unit        string `json:"unit"`
	Spec        string `json:"spec"`
	SafetyStock int64  `json:"safety_stock"`
	DefaultCost int64  `json:"default_cost,omitempty"`
	Status      string `json:"status"`
	Quantity    int64  `json:"quantity"`
	AvgCost     int64  `json:"avg_cost,omitempty"`
	Amount      int64  `json:"amount,omitempty"`
}

// CatalogTabSpec 用策略方式描述每个仓库标签对应的数据表和固定分类。
type CatalogTabSpec struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	ItemType string `json:"item_type"`
	Category string `json:"category"`
}

var catalogTabs = []CatalogTabSpec{
	{Key: tabProduct, Title: "产品", ItemType: "product", Category: "产品"},
	{Key: tabProductionMaterial, Title: "生产物资", ItemType: "material", Category: "生产物资"},
	{Key: tabRegularProduct, Title: "常规产品", ItemType: "material", Category: "常规产品"},
	{Key: tabDailySupply, Title: "生活物资", ItemType: "material", Category: "生活物资"},
}

func catalogSpec(tab string) (CatalogTabSpec, bool) {
	for _, spec := range catalogTabs {
		if spec.Key == tab {
			return spec, true
		}
	}
	return CatalogTabSpec{}, false
}

func listCatalogItems(db *gorm.DB, tab string) ([]CatalogItem, error) {
	spec, ok := catalogSpec(tab)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "无效仓库标签")
	}
	if spec.ItemType == "product" {
		var products []model.Product
		if err := db.Order("id desc").Find(&products).Error; err != nil {
			return nil, err
		}
		items := make([]CatalogItem, 0, len(products))
		for _, product := range products {
			items = append(items, CatalogItem{
				ID: product.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category,
				Name: product.Name, Code: product.Code, Unit: product.Unit, Spec: product.Spec,
				SafetyStock: product.SafetyStock, DefaultCost: product.DefaultCost, Status: product.Status,
			})
		}
		return items, nil
	}
	var materials []model.Material
	if err := db.Where("category = ?", spec.Category).Order("id desc").Find(&materials).Error; err != nil {
		return nil, err
	}
	items := make([]CatalogItem, 0, len(materials))
	for _, material := range materials {
		items = append(items, CatalogItem{
			ID: material.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category,
			Name: material.Name, Code: material.Code, Unit: material.Unit, Spec: material.Spec,
			SafetyStock: material.SafetyStock, DefaultCost: material.DefaultCost, Status: material.Status,
		})
	}
	return items, nil
}

func createCatalogItem(db *gorm.DB, input CatalogItemInput) (CatalogItem, error) {
	spec, ok := catalogSpec(input.Tab)
	if !ok {
		return CatalogItem{}, echo.NewHTTPError(http.StatusBadRequest, "无效仓库标签")
	}
	if input.Unit == "" {
		input.Unit = "个"
	}
	if spec.ItemType == "product" {
		product := model.Product{
			Name: input.Name, Code: input.Code, Unit: input.Unit, Spec: input.Spec,
			SafetyStock: input.SafetyStock, DefaultCost: input.DefaultCost, Status: model.StatusActive,
		}
		if err := db.Create(&product).Error; err != nil {
			return CatalogItem{}, err
		}
		return CatalogItem{ID: product.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category, Name: product.Name, Code: product.Code, Unit: product.Unit, Spec: product.Spec, SafetyStock: product.SafetyStock, DefaultCost: product.DefaultCost, Status: product.Status}, nil
	}
	material := model.Material{
		Name: input.Name, Code: input.Code, Category: spec.Category, Unit: input.Unit, Spec: input.Spec,
		SafetyStock: input.SafetyStock, DefaultCost: input.DefaultCost, Status: model.StatusActive,
	}
	if err := db.Create(&material).Error; err != nil {
		return CatalogItem{}, err
	}
	return CatalogItem{ID: material.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category, Name: material.Name, Code: material.Code, Unit: material.Unit, Spec: material.Spec, SafetyStock: material.SafetyStock, DefaultCost: material.DefaultCost, Status: material.Status}, nil
}
