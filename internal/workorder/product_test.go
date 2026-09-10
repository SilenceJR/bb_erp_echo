package workorder

import (
	"testing"

	"bb_erp_echo/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLoadActiveProductUsesProductModel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:workorder-product-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Product{}); err != nil {
		t.Fatal(err)
	}
	product := model.Product{ProductModel: "P-400", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	handler := &Handler{DB: db}
	loaded, err := handler.loadActiveProductDB(db, &product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProductModel != "P-400" {
		t.Fatalf("product model = %q", loaded.ProductModel)
	}
	if err := db.Model(&product).Update("status", model.StatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := handler.loadActiveProductDB(db, &product.ID); err == nil {
		t.Fatal("expected disabled product error")
	}
}
