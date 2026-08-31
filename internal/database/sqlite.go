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
