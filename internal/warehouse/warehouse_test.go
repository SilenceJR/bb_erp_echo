package warehouse

import (
	"testing"

	"bb_erp_echo/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplyDeltaMaintainsQuantityAndLedger(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:warehouse-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	models := []any{&model.Product{}, &model.Warehouse{}, &model.Location{}, &model.InventoryDocument{}, &model.InventoryDocumentLine{}, &model.InventoryBalance{}, &model.InventoryLedger{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	product := model.Product{ProductModel: "P-300", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	warehouse := model.Warehouse{Name: "主仓库", Code: model.DefaultWarehouseCode, Status: model.StatusActive}
	if err := db.Create(&warehouse).Error; err != nil {
		t.Fatal(err)
	}
	location := model.Location{WarehouseID: warehouse.ID, Code: "A1-1", Name: "A1-1", Status: model.StatusActive}
	if err := db.Create(&location).Error; err != nil {
		t.Fatal(err)
	}
	document := model.InventoryDocument{Code: "STK-1", Type: "inbound", Status: "posted", WarehouseID: warehouse.ID, BusinessType: "inbound", Reason: "test"}
	if err := db.Create(&document).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db)
	if err := handler.applyDelta(db, product, location.ID, 10000, &document, "test"); err != nil {
		t.Fatal(err)
	}
	var balance model.InventoryBalance
	if err := db.Where("item_type = ? AND item_id = ? AND location_id = ?", itemTypeProduct, product.ID, location.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.Quantity != 10000 {
		t.Fatalf("quantity = %d", balance.Quantity)
	}
	if err := handler.applyDelta(db, product, location.ID, -2500, &document, "test out"); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&balance, balance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if balance.Quantity != 7500 {
		t.Fatalf("quantity after outbound = %d", balance.Quantity)
	}
	var ledgers int64
	if err := db.Model(&model.InventoryLedger{}).Where("item_type = ? AND item_id = ?", itemTypeProduct, product.ID).Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 2 {
		t.Fatalf("ledger count = %d", ledgers)
	}
}
