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

func TestBackupServerFilesRequiresExistingServerExecutable(t *testing.T) {
	if err := backupServerFiles(t.TempDir(), filepath.Join(t.TempDir(), "backup")); err == nil {
		t.Fatal("backup without the installed server executable must fail")
	}
}

// TestReplaceServerFilesRefreshesVerificationKey 验证服务端升级会替换公钥和启动文件，但保留业务数据。
func TestReplaceServerFilesRefreshesVerificationKey(t *testing.T) {
	sourceDir := t.TempDir()
	installDir := t.TempDir()
	newFiles := map[string]string{
		"bb-erp-server.exe": "new-server",
		"update-public.key": "new-public-key",
		"web/index.html":    "new-web",
	}
	for name, contents := range newFiles {
		path := filepath.Join(sourceDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create source parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	customFiles := map[string]string{
		"version.json": `{"version":"custom"}`,
		"启动服务端.bat":    "set BB_ERP_HTTP_PORT=9080",
	}
	for name, contents := range customFiles {
		if err := os.WriteFile(filepath.Join(installDir, name), []byte(contents), 0o640); err != nil {
			t.Fatalf("write custom %s: %v", name, err)
		}
	}
	preservedFiles := map[string]string{
		"data/erp.db":              "business-data",
		"static/uploads/image.png": "uploaded-image",
		"logs/app.log":             "runtime-log",
		"updates/client.patch":     "cached-update",
	}
	for name, contents := range preservedFiles {
		path := filepath.Join(installDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create preserved parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
			t.Fatalf("write preserved %s: %v", name, err)
		}
	}

	if err := replaceServerFiles(sourceDir, installDir); err != nil {
		t.Fatalf("replace server files: %v", err)
	}
	for name, want := range newFiles {
		got, err := os.ReadFile(filepath.Join(installDir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read installed %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("installed %s = %q, want %q", name, got, want)
		}
	}
	for name, want := range preservedFiles {
		got, err := os.ReadFile(filepath.Join(installDir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read preserved %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("preserved %s = %q, want %q", name, got, want)
		}
	}
	for name, want := range customFiles {
		got, err := os.ReadFile(filepath.Join(installDir, name))
		if err != nil || string(got) != want {
			t.Errorf("custom deployment file %s changed: %q, %v", name, got, err)
		}
	}
}

func TestValidateServerPackageRequiresValidPublicKey(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "bb-erp-server.exe"), []byte("server"), 0o700); err != nil {
		t.Fatalf("write server: %v", err)
	}
	if err := validateServerPackage(sourceDir); err == nil {
		t.Fatal("package without update-public.key must be rejected")
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "update-public.key"), []byte("invalid"), 0o600); err != nil {
		t.Fatalf("write invalid public key: %v", err)
	}
	if err := validateServerPackage(sourceDir); err == nil {
		t.Fatal("package with invalid update-public.key must be rejected")
	}
	const publishedKey = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IDk4Y2E4ZTFhMmNlOWQxNTcKUldSWDBla3NHbzdLbUpoZnlmNWlwY1p6eEdEaUFiNmlFVVpsNTRIcnV0RmI5NjlFMytNNFlTQVcK"
	if err := os.WriteFile(filepath.Join(sourceDir, "update-public.key"), []byte(publishedKey), 0o600); err != nil {
		t.Fatalf("write published public key: %v", err)
	}
	if err := validateServerPackage(sourceDir); err != nil {
		t.Fatalf("validate published server package: %v", err)
	}
}

func TestRestoreServerFilesReturnsReplacedFilesToBackup(t *testing.T) {
	backupDir := t.TempDir()
	installDir := t.TempDir()
	backupFiles := map[string]string{
		"bb-erp-server.exe": "old-server",
		"update-public.key": "old-key",
		"web/index.html":    "old-web",
	}
	for name, contents := range backupFiles {
		path := filepath.Join(backupDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create backup parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
			t.Fatalf("write backup %s: %v", name, err)
		}
	}
	for name := range backupFiles {
		path := filepath.Join(installDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create install parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("partial-new-content"), 0o640); err != nil {
			t.Fatalf("write installed %s: %v", name, err)
		}
	}

	if err := restoreServerFiles(backupDir, installDir); err != nil {
		t.Fatalf("restore server files: %v", err)
	}
	for name, want := range backupFiles {
		got, err := os.ReadFile(filepath.Join(installDir, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Errorf("restored %s = %q, want %q, err=%v", name, got, want, err)
		}
	}
}

func TestRestoreServerFilesRemovesPathsAbsentFromBackup(t *testing.T) {
	backupDir := t.TempDir()
	installDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, "bb-erp-server.exe"), []byte("old-server"), 0o700); err != nil {
		t.Fatalf("write backed up server: %v", err)
	}
	for name := range map[string]struct{}{
		"bb-erp-server.exe": {},
		"update-public.key": {},
		"web/index.html":    {},
	} {
		path := filepath.Join(installDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create installed parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("new-content"), 0o640); err != nil {
			t.Fatalf("write installed %s: %v", name, err)
		}
	}

	if err := restoreServerFiles(backupDir, installDir); err != nil {
		t.Fatalf("restore server files: %v", err)
	}
	server, err := os.ReadFile(filepath.Join(installDir, "bb-erp-server.exe"))
	if err != nil || string(server) != "old-server" {
		t.Fatalf("server was not restored: %q, %v", server, err)
	}
	for _, name := range []string{"update-public.key", "web"} {
		if _, err := os.Stat(filepath.Join(installDir, name)); !os.IsNotExist(err) {
			t.Errorf("new path %s should be removed when absent from backup: %v", name, err)
		}
	}
}
