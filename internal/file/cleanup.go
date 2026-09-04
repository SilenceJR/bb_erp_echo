package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bb_erp_echo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QueueCleanupTasks 必须与删除业务记录处于同一数据库事务，确保崩溃后仍可恢复清理。
func QueueCleanupTasks(db *gorm.DB, paths []string) error {
	for _, relative := range paths {
		if strings.TrimSpace(relative) == "" {
			continue
		}
		task := model.FileCleanupTask{StoragePath: filepath.ToSlash(relative)}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "storage_path"}}, DoNothing: true}).Create(&task).Error; err != nil {
			return err
		}
	}
	return nil
}

// CleanupStoredPaths 在数据库事务已持久化任务后清理物理文件并完成任务。
func CleanupStoredPaths(root string, db *gorm.DB, paths []string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	for _, relative := range paths {
		if strings.TrimSpace(relative) == "" {
			continue
		}
		if err := removeStoredPath(root, relative); err != nil {
			_ = db.Model(&model.FileCleanupTask{}).Where("storage_path = ?", filepath.ToSlash(relative)).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_error": err.Error()}).Error
			continue
		}
		_ = db.Where("storage_path = ?", filepath.ToSlash(relative)).Delete(&model.FileCleanupTask{}).Error
	}
}

// RetryPendingCleanups 重试此前提交后未能删除的孤儿文件。
func RetryPendingCleanups(root string, db *gorm.DB) error {
	var lastID uint
	for {
		var tasks []model.FileCleanupTask
		if err := db.Where("id > ?", lastID).Order("id asc").Limit(500).Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		for _, task := range tasks {
			lastID = task.ID
			if err := removeStoredPath(root, task.StoragePath); err != nil {
				if updateErr := db.Model(&task).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_error": err.Error()}).Error; updateErr != nil {
					return updateErr
				}
				continue
			}
			if err := db.Delete(&task).Error; err != nil {
				return err
			}
		}
	}
}

func removeStoredPath(root, relative string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("上传根目录不能为空")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("非法文件路径 %q", relative)
	}
	err := os.Remove(filepath.Join(root, clean))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
