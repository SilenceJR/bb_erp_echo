package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const inventoryDocumentIdempotencyIndex = "idx_inventory_documents_idempotency_key"

// EnsureEmployeeDepartmentConsistency 检查员工和部门关系中的组织边界。
//
// EmployeeDepartment 使用外键保证两端记录存在，但 SQLite 不支持用跨表
// CHECK 约束表达组织一致性。启动迁移因此显式扫描既有关系；发现历史脏数据
// 时阻断启动并报告关系 ID，要求管理员先修复，而不是继续写入更多数据。
func EnsureEmployeeDepartmentConsistency(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var invalid []struct {
			ID           uint `gorm:"column:relation_id"`
			EmployeeID   uint `gorm:"column:employee_id"`
			DepartmentID uint `gorm:"column:department_id"`
		}
		if err := tx.Raw(`
			SELECT employee_departments.id AS relation_id,
			       employee_departments.employee_id,
			       employee_departments.department_id
			FROM employee_departments
			JOIN employees ON employees.id = employee_departments.employee_id
			JOIN departments ON departments.id = employee_departments.department_id
			WHERE employees.organization_id <> departments.organization_id
			ORDER BY employee_departments.id
		`).Scan(&invalid).Error; err != nil {
			return fmt.Errorf("inspect employee department organization consistency: %w", err)
		}
		if len(invalid) == 0 {
			return nil
		}
		parts := make([]string, 0, len(invalid))
		for _, relation := range invalid {
			parts = append(parts, fmt.Sprintf("relation %d (employee %d, department %d)", relation.ID, relation.EmployeeID, relation.DepartmentID))
		}
		return fmt.Errorf("employee department migration blocked: cross-organization relations found (%s); remove or correct these relations, then restart", strings.Join(parts, "; "))
	})
}

// EnsureInventoryDocumentIdempotencyIndex 将库存单据幂等键升级为安全的部分唯一索引。
//
// 旧版本可能已经创建了同名普通索引；GORM AutoMigrate 不会改变已有索引定义，
// 所以启动迁移必须显式检查并重建。迁移在删除旧索引前检查所有非空重复值，
// 发现重复时原样阻断并报告 key/数量，避免静默丢弃或覆盖历史数据。
//
// 该函数只处理 SQLite。新库可在 AutoMigrate 前调用（此时表尚不存在会跳过），
// 旧库应在 AutoMigrate 前后各调用一次：前置调用可以在 GORM 尝试创建唯一索引
// 前报告重复 key，后置调用负责新表/新字段落地后的最终索引校验。
func EnsureInventoryDocumentIdempotencyIndex(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		ready, err := sqliteTableHasColumn(tx, "inventory_documents", "idempotency_key")
		if err != nil {
			return fmt.Errorf("inspect inventory document table: %w", err)
		}
		if !ready {
			return nil
		}
		var duplicates []struct {
			Key   string `gorm:"column:idempotency_key"`
			Count int64  `gorm:"column:duplicate_count"`
		}
		if err := tx.Raw(`
			SELECT idempotency_key, COUNT(*) AS duplicate_count
			FROM inventory_documents
			WHERE idempotency_key <> ''
			GROUP BY idempotency_key
			HAVING COUNT(*) > 1
		`).Scan(&duplicates).Error; err != nil {
			return fmt.Errorf("inspect inventory document idempotency keys: %w", err)
		}
		if len(duplicates) > 0 {
			parts := make([]string, 0, len(duplicates))
			for _, duplicate := range duplicates {
				parts = append(parts, fmt.Sprintf("key %q has %d rows", duplicate.Key, duplicate.Count))
			}
			return fmt.Errorf("inventory document idempotency index migration blocked: duplicate non-empty idempotency keys (%s); resolve duplicates manually, then restart", strings.Join(parts, "; "))
		}

		index, exists, err := sqliteIndexDefinition(tx, inventoryDocumentIdempotencyIndex)
		if err != nil {
			return err
		}
		if exists && index.Unique == 1 && index.Partial == 1 && isExpectedInventoryDocumentIdempotencyIndex(index.SQL) {
			return nil
		}
		if exists {
			if err := tx.Exec(`DROP INDEX ` + quoteSQLiteIdentifier(inventoryDocumentIdempotencyIndex)).Error; err != nil {
				return fmt.Errorf("drop legacy inventory document idempotency index: %w", err)
			}
		}
		if err := tx.Exec(`CREATE UNIQUE INDEX ` + quoteSQLiteIdentifier(inventoryDocumentIdempotencyIndex) + ` ON inventory_documents (idempotency_key) WHERE idempotency_key <> ''`).Error; err != nil {
			return fmt.Errorf("create inventory document idempotency index: %w", err)
		}
		return nil
	})
}

func sqliteTableHasColumn(db *gorm.DB, tableName, columnName string) (bool, error) {
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, tableName, columnName).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func isExpectedInventoryDocumentIdempotencyIndex(createSQL string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(createSQL), " "))
	normalized = strings.NewReplacer(`"`, "", "`", "", "[", "", "]", "").Replace(normalized)
	normalized = strings.TrimSuffix(normalized, ";")
	where := strings.Index(normalized, " where ")
	if where < 0 {
		return false
	}
	predicate := strings.TrimSpace(normalized[where+len(" where "):])
	return strings.Contains(normalized, " on inventory_documents (idempotency_key) ") && predicate == "idempotency_key <> ''"
}

type sqliteIndex struct {
	Unique  int
	Partial int
	SQL     string
}

func sqliteIndexDefinition(db *gorm.DB, name string) (sqliteIndex, bool, error) {
	var row struct {
		Unique  int    `gorm:"column:is_unique"`
		Partial int    `gorm:"column:is_partial"`
		SQL     string `gorm:"column:create_sql"`
	}
	result := db.Raw(`
		SELECT il.[unique] AS is_unique, il.[partial] AS is_partial, COALESCE(sm.sql, '') AS create_sql
		FROM pragma_index_list('inventory_documents') AS il
		LEFT JOIN sqlite_master AS sm ON sm.type = 'index' AND sm.name = il.name
		WHERE il.name = ?
	`, name).Scan(&row)
	if result.Error != nil {
		return sqliteIndex{}, false, fmt.Errorf("inspect inventory document idempotency index: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return sqliteIndex{}, false, nil
	}
	return sqliteIndex{Unique: row.Unique, Partial: row.Partial, SQL: row.SQL}, true, nil
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// Open 打开 SQLite 数据库并应用 ERP 后台所需的连接参数。
//
// 参数说明：
// - path：SQLite 数据库文件路径，例如 bb_erp.sqlite3。
//
// 返回说明：
// - 返回已连接的 GORM DB。
// - 当数据库打开、连接池获取、Ping 或 PRAGMA 初始化失败时返回错误。
func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", path)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql database: %w", err)
	}

	// SQLite 更适合少量连接串行写入；WAL 提升读写并发，但仍避免打开过多写连接。
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// PRAGMA 同时写入 DSN 和启动校验，保证不同驱动行为下配置都能落地。
	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if err := sqlDB.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("ping sqlite database: %w", err)
		}
		if err := db.Exec(statement).Error; err != nil {
			return nil, fmt.Errorf("apply sqlite pragma %q: %w", statement, err)
		}
	}

	return db, nil
}
