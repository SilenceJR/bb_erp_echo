package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBackupServerFilesIncludesRuntimeState 验证升级回滚快照包含数据库、上传文件、更新缓存和公钥文件。
func TestBackupServerFilesIncludesRuntimeState(t *testing.T) {
	installDir := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	files := map[string]string{
		"bb-erp-server.exe":        "server",
		"web/index.html":           "web",
		"data/erp.db":              "sqlite",
		"static/uploads/image.png": "upload",
		"updates/client.zip":       "update-cache",
		"logs/app.log":             "log",
		"update-public.key":        "public-key",
	}
	for name, contents := range files {
		path := filepath.Join(installDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create %s parent: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := backupServerFiles(installDir, backupDir); err != nil {
		t.Fatalf("backup server files: %v", err)
	}

	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(backupDir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read backed up %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("backed up %s = %q, want %q", name, got, want)
		}
	}
}
