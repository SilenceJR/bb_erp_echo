package model

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCustomerProfileReferencesAreRestrictForeignKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := db.AutoMigrate(
		&CustomerCode{}, &CustomerProfile{},
		&InventoryDocument{}, &InventoryDocumentLine{},
		&WorkOrder{}, &DepartmentTask{}, &WorkOrderFlowLog{},
		&Mold{}, &MoldEvent{},
	); err != nil {
		t.Fatalf("auto migrate customer references: %v", err)
	}

	for _, table := range []string{"inventory_documents", "work_orders", "molds"} {
		assertCustomerProfileForeignKey(t, db, table)
	}

	code := CustomerCode{Code: "BB-901"}
	if err := db.Create(&code).Error; err != nil {
		t.Fatalf("create customer code: %v", err)
	}
	profiles := []CustomerProfile{
		{CustomerCodeID: code.ID, IsDefault: true},
		{CustomerCodeID: code.ID},
		{CustomerCodeID: code.ID},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("create customer profiles: %v", err)
	}

	if err := db.Create(&InventoryDocument{Code: "INV-CUSTOMER-FK", Type: "outbound", Status: "draft", WarehouseID: 1, CustomerID: &profiles[0].ID}).Error; err != nil {
		t.Fatalf("create inventory document reference: %v", err)
	}
	if err := db.Create(&WorkOrder{Code: "WO-CUSTOMER-FK", Title: "客户外键", CustomerID: &profiles[1].ID}).Error; err != nil {
		t.Fatalf("create work order reference: %v", err)
	}
	if err := db.Create(&Mold{Code: "MOLD-CUSTOMER-FK", Name: "客户外键模具", CustomerID: &profiles[2].ID}).Error; err != nil {
		t.Fatalf("create mold reference: %v", err)
	}

	for _, profile := range profiles {
		if err := db.Delete(&profile).Error; err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			t.Fatalf("delete referenced profile %d error = %v, want foreign-key restriction", profile.ID, err)
		}
		var count int64
		if err := db.Model(&CustomerProfile{}).Where("id = ?", profile.ID).Count(&count).Error; err != nil {
			t.Fatalf("verify referenced profile %d: %v", profile.ID, err)
		}
		if count != 1 {
			t.Fatalf("referenced profile %d disappeared after failed delete", profile.ID)
		}
	}
}

func assertCustomerProfileForeignKey(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	rows, err := db.Raw("PRAGMA foreign_key_list('" + table + "')").Rows()
	if err != nil {
		t.Fatalf("list %s foreign keys: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, sequence int
		var referencedTable, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &sequence, &referencedTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan %s foreign key: %v", table, err)
		}
		if referencedTable == "customer_profiles" && from == "customer_id" && to == "id" {
			if strings.ToUpper(onDelete) != "RESTRICT" {
				t.Fatalf("%s customer foreign key on delete = %q, want RESTRICT", table, onDelete)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s foreign keys: %v", table, err)
	}
	t.Fatalf("%s has no customer_id -> customer_profiles(id) foreign key", table)
}
