package product

import (
	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"

	"gorm.io/gorm"
)

func lockProductAssetMutation() func() { return filemodule.LockProductAssetMutation() }

func imagePaths(images []model.ImageFile) []string {
	paths := make([]string, 0, len(images)*2)
	for _, asset := range images {
		paths = append(paths, asset.StoragePath)
		if asset.PreviewPath != "" {
			paths = append(paths, asset.PreviewPath)
		}
	}
	return paths
}

func queueImageCleanup(tx *gorm.DB, paths []string) error {
	return filemodule.QueueCleanupTasks(tx, paths)
}
