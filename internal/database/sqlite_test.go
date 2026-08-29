package database

import (
	"strings"
	"testing"
	"time"

	"bb_erp_echo/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newLegacyInventoryDocumentDB(t *testing.T, indexSQL string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE inventory_documents (id INTEGER PRIMARY KEY AUTOINCREMENT, idempotency_key TEXT NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatalf("create legacy inventory_documents table: %v", err)
	}
	if strings.TrimSpace(indexSQL) != "" {
		if err := db.Exec(indexSQL).Error; err != nil {
			t.Fatalf("create legacy index: %v", err)
		}
	}
	return db
}

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

func TestEnsureInventoryDocumentIdempotencyIndexUpgradesLegacyOrdinaryIndexAndIsIdempotent(t *testing.T) {
	db := newLegacyInventoryDocumentDB(t, `CREATE INDEX idx_inventory_documents_idempotency_key ON inventory_documents (idempotency_key)`)
	// 真实启动顺序先执行 GORM AutoMigrate；已有同名普通索引必须仍由显式
	// 升级步骤接管，而不能依赖 AutoMigrate 猜测索引谓词。
	if err := db.AutoMigrate(&model.InventoryDocument{}, &model.InventoryDocumentLine{}); err != nil {
		t.Fatalf("auto migrate legacy inventory document table: %v", err)
	}

	if err := EnsureInventoryDocumentIdempotencyIndex(db); err != nil {
		t.Fatalf("upgrade legacy index: %v", err)
	}
	assertPartialUniqueInventoryDocumentIndex(t, db)
	if err := EnsureInventoryDocumentIdempotencyIndex(db); err != nil {
		t.Fatalf("repeat index upgrade: %v", err)
	}
	assertPartialUniqueInventoryDocumentIndex(t, db)
}

func TestEnsureInventoryDocumentIdempotencyIndexBlocksDuplicateLegacyData(t *testing.T) {
	db := newLegacyInventoryDocumentDB(t, `CREATE INDEX idx_inventory_documents_idempotency_key ON inventory_documents (idempotency_key)`)
	if err := db.Exec(`INSERT INTO inventory_documents (idempotency_key) VALUES (?), (?)`, "legacy-key", "legacy-key").Error; err != nil {
		t.Fatalf("seed duplicate legacy keys: %v", err)
	}

	err := EnsureInventoryDocumentIdempotencyIndex(db)
	if err == nil {
		t.Fatal("duplicate legacy keys should block migration")
	}
	message := err.Error()
	if !strings.Contains(message, `"legacy-key"`) || !strings.Contains(message, "2 rows") || !strings.Contains(message, "resolve duplicates manually") {
		t.Fatalf("migration error = %q, want key, count and remediation", message)
	}
	index, exists, indexErr := sqliteIndexDefinition(db, inventoryDocumentIdempotencyIndex)
	if indexErr != nil {
		t.Fatalf("inspect preserved legacy index: %v", indexErr)
	}
	if !exists || index.Unique != 0 || index.Partial != 0 {
		t.Fatalf("duplicate-blocked migration changed legacy index: exists=%v index=%+v", exists, index)
	}
}

func TestEnsureInventoryDocumentIdempotencyIndexBlocksDuplicatesBeforeAutoMigrate(t *testing.T) {
	db := newLegacyInventoryDocumentDB(t, "")
	if err := db.Exec(`INSERT INTO inventory_documents (idempotency_key) VALUES (?), (?)`, "preflight-key", "preflight-key").Error; err != nil {
		t.Fatalf("seed duplicate preflight keys: %v", err)
	}
	if err := EnsureInventoryDocumentIdempotencyIndex(db); err == nil || !strings.Contains(err.Error(), `"preflight-key"`) || !strings.Contains(err.Error(), "2 rows") {
		t.Fatalf("preflight duplicate error = %v, want key and count", err)
	}
}

func TestEnsureInventoryDocumentIdempotencyIndexAcceptsExistingTargetIndex(t *testing.T) {
	db := newLegacyInventoryDocumentDB(t, `CREATE UNIQUE INDEX idx_inventory_documents_idempotency_key ON inventory_documents (idempotency_key) WHERE idempotency_key <> ''`)
	if err := EnsureInventoryDocumentIdempotencyIndex(db); err != nil {
		t.Fatalf("existing target index should be accepted: %v", err)
	}
	assertPartialUniqueInventoryDocumentIndex(t, db)
}

func assertPartialUniqueInventoryDocumentIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	index, exists, err := sqliteIndexDefinition(db, inventoryDocumentIdempotencyIndex)
	if err != nil {
		t.Fatalf("inspect idempotency index: %v", err)
	}
	if !exists {
		t.Fatalf("idempotency index does not exist")
	}
	if index.Unique != 1 || index.Partial != 1 {
		t.Fatalf("idempotency index metadata = %+v, want unique partial", index)
	}
	if !strings.Contains(strings.ToLower(index.SQL), "where") || !strings.Contains(strings.ToLower(index.SQL), "idempotency_key") {
		t.Fatalf("idempotency index SQL = %q, want predicate", index.SQL)
	}
}
