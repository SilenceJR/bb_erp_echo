package warehouse

import (
	"net/http"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

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
	Tab                string `json:"tab" validate:"required"`
	Name               string `json:"name" validate:"required"`
	Code               string `json:"code" validate:"required"`
	Unit               string `json:"unit"`
	Spec               string `json:"spec"`
	SafetyStock        int64  `json:"safety_stock"`
	DefaultCost        int64  `json:"default_cost"`
	OperatorEmployeeID uint   `json:"operator_employee_id"`
	operatorSnapshot   model.OperatorSnapshot
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

func listCatalogItems(db *gorm.DB, tab string, pageQuery pagination.Query) (pagination.Result[CatalogItem], error) {
	spec, ok := catalogSpec(tab)
	if !ok {
		return pagination.Result[CatalogItem]{}, echo.NewHTTPError(http.StatusBadRequest, "无效仓库标签")
	}
	if spec.ItemType == "product" {
		query := db.Model(&model.Product{})
		query = pagination.ApplyKeyword(query, pageQuery.Keyword, "name", "code", "unit", "spec", "status")
		result, err := pagination.Page[model.Product](query, pageQuery, "id desc", nil)
		if err != nil {
			return pagination.Result[CatalogItem]{}, err
		}
		items := make([]CatalogItem, 0, len(result.Items))
		for _, product := range result.Items {
			items = append(items, CatalogItem{
				ID: product.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category,
				Name: product.Name, Code: product.Code, Unit: product.Unit, Spec: product.Spec,
				SafetyStock: product.SafetyStock, DefaultCost: product.DefaultCost, Status: product.Status,
			})
		}
		return pagination.Result[CatalogItem]{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, Keyword: result.Keyword}, nil
	}
	query := db.Model(&model.Material{}).Where("category = ?", spec.Category)
	query = pagination.ApplyKeyword(query, pageQuery.Keyword, "name", "code", "category", "unit", "spec", "status")
	result, err := pagination.Page[model.Material](query, pageQuery, "id desc", nil)
	if err != nil {
		return pagination.Result[CatalogItem]{}, err
	}
	items := make([]CatalogItem, 0, len(result.Items))
	for _, material := range result.Items {
		items = append(items, CatalogItem{
			ID: material.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category,
			Name: material.Name, Code: material.Code, Unit: material.Unit, Spec: material.Spec,
			SafetyStock: material.SafetyStock, DefaultCost: material.DefaultCost, Status: material.Status,
		})
	}
	return pagination.Result[CatalogItem]{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, Keyword: result.Keyword}, nil
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
			OperatorSnapshot: input.operatorSnapshot,
		}
		if err := db.Create(&product).Error; err != nil {
			return CatalogItem{}, err
		}
		return CatalogItem{ID: product.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category, Name: product.Name, Code: product.Code, Unit: product.Unit, Spec: product.Spec, SafetyStock: product.SafetyStock, DefaultCost: product.DefaultCost, Status: product.Status}, nil
	}
	material := model.Material{
		Name: input.Name, Code: input.Code, Category: spec.Category, Unit: input.Unit, Spec: input.Spec,
		SafetyStock: input.SafetyStock, DefaultCost: input.DefaultCost, Status: model.StatusActive,
		OperatorSnapshot: input.operatorSnapshot,
	}
	if err := db.Create(&material).Error; err != nil {
		return CatalogItem{}, err
	}
	return CatalogItem{ID: material.ID, ItemType: spec.ItemType, Tab: spec.Key, Category: spec.Category, Name: material.Name, Code: material.Code, Unit: material.Unit, Spec: material.Spec, SafetyStock: material.SafetyStock, DefaultCost: material.DefaultCost, Status: material.Status}, nil
}
