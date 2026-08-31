package database

import (
	"strings"
	"testing"
	"time"

	"bb_erp_echo/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureEmployeeDepartmentConsistencyBlocksCrossOrganizationRelations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	organizations := []model.Organization{
		{Name: "组织一", Code: "CONSISTENCY-ONE", Status: model.StatusActive},
		{Name: "组织二", Code: "CONSISTENCY-TWO", Status: model.StatusActive},
	}
	if err := db.Create(&organizations).Error; err != nil {
		t.Fatalf("seed organizations: %v", err)
	}
	department := model.Department{OrganizationID: organizations[0].ID, Name: "部门", Code: "CONSISTENCY-DEPT", Status: model.StatusActive}
	employee := model.Employee{OrganizationID: organizations[1].ID, Name: "员工", HireDate: modelDate(2020, 1, 1), BirthDate: modelDate(1990, 1, 1), Status: model.StatusActive}
	if err := db.Create(&department).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("seed employee: %v", err)
	}
	// 关系两端存在，因此外键允许写入；组织边界由启动一致性扫描负责。
	if err := db.Create(&model.EmployeeDepartment{EmployeeID: employee.ID, DepartmentID: department.ID}).Error; err != nil {
		t.Fatalf("seed inconsistent relation: %v", err)
	}
	err = EnsureEmployeeDepartmentConsistency(db)
	if err == nil || !strings.Contains(err.Error(), "cross-organization") || !strings.Contains(err.Error(), "employee ") || !strings.Contains(err.Error(), "department ") {
		t.Fatalf("consistency error = %v, want actionable cross-organization relation", err)
	}
}

func TestEmployeeDepartmentHasForeignKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}); err != nil {
		t.Fatalf("migrate organization models: %v", err)
	}
	var foreignKeys []struct {
		Table string
	}
	rows, err := db.Raw("PRAGMA foreign_key_list('employee_departments')").Rows()
	if err != nil {
		t.Fatalf("list employee department foreign keys: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, sequence int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &sequence, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan employee department foreign key: %v", err)
		}
		foreignKeys = append(foreignKeys, struct{ Table string }{Table: table})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate employee department foreign keys: %v", err)
	}
	seenEmployee, seenDepartment := false, false
	for _, foreignKey := range foreignKeys {
		switch foreignKey.Table {
		case "employees":
			seenEmployee = true
		case "departments":
			seenDepartment = true
		}
	}
	if !seenEmployee || !seenDepartment {
		t.Fatalf("employee_departments foreign keys = %+v, want employees and departments", foreignKeys)
	}
}

func modelDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestInventoryDocumentSchemaCreatesPartialUniqueIdempotencyIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.InventoryDocument{}, &model.InventoryDocumentLine{}); err != nil {
		t.Fatalf("auto migrate inventory document schema: %v", err)
	}

	var index struct {
		Unique  int    `gorm:"column:is_unique"`
		Partial int    `gorm:"column:is_partial"`
		SQL     string `gorm:"column:create_sql"`
	}
	result := db.Raw(`
		SELECT il.[unique] AS is_unique, il.[partial] AS is_partial, COALESCE(sm.sql, '') AS create_sql
		FROM pragma_index_list('inventory_documents') AS il
		LEFT JOIN sqlite_master AS sm ON sm.type = 'index' AND sm.name = il.name
		WHERE il.name = ?
	`, "idx_inventory_documents_idempotency_key").Scan(&index)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("inspect idempotency index: error=%v rows=%d", result.Error, result.RowsAffected)
	}
	if index.Unique != 1 || index.Partial != 1 || !strings.Contains(strings.ToLower(index.SQL), "where") {
		t.Fatalf("idempotency index = %+v, want partial unique index", index)
	}

	first := model.InventoryDocument{Code: "DOC-001", Type: "inbound", Status: "draft", WarehouseID: 1, IdempotencyKey: "same-request"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first idempotent document: %v", err)
	}
	duplicate := model.InventoryDocument{Code: "DOC-002", Type: "inbound", Status: "draft", WarehouseID: 1, IdempotencyKey: "same-request"}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("schema accepted duplicate non-empty idempotency key")
	}
	empty := model.InventoryDocument{Code: "DOC-003", Type: "inbound", Status: "draft", WarehouseID: 1}
	if err := db.Create(&empty).Error; err != nil {
		t.Fatalf("create empty idempotency key document: %v", err)
	}
}
