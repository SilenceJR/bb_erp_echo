package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
	"bb_erp_echo/internal/update"
)

// TestBackupServerFilesIncludesRuntimeState 验证升级回滚快照包含数据库、上传文件、更新缓存和公钥文件。
func TestBackupServerFilesIncludesRuntimeState(t *testing.T) {
	installDir := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	files := map[string]string{
		"bb-erp-server.exe":         "server",
		"bb-erp-updater.exe":        "updater",
		"bb-erp-upgrade-runner.bat": "runner",
		"web/index.html":            "web",
		"data/erp.db":               "sqlite",
		"static/uploads/image.png":  "upload",
		"updates/client.zip":        "update-cache",
		"logs/app.log":              "log",
		"update-public.key":         "public-key",
		"version.json":              `{"version":"1.0.0","server_version":"1.0.0","manifest_url":"https://example.com/update-manifest.json"}`,
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
		"bb-erp-server.exe":         "new-server",
		"bb-erp-updater.exe":        "new-updater",
		"bb-erp-upgrade-runner.bat": "new-runner",
		"update-public.key":         "new-public-key",
		"web/index.html":            "new-web",
		"version.json":              `{"version":"2.0.0","server_version":"2.0.0","manifest_url":"https://example.com/update-manifest.json"}`,
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
		"启动服务端.bat": "set BB_ERP_HTTP_PORT=9080",
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
		if name == "bb-erp-updater.exe" || name == "bb-erp-upgrade-runner.bat" {
			continue
		}
		got, err := os.ReadFile(filepath.Join(installDir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read installed %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("installed %s = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"bb-erp-updater.pending.exe":        "new-updater",
		"bb-erp-upgrade-runner.pending.bat": "new-runner",
	} {
		got, err := os.ReadFile(filepath.Join(installDir, name))
		if err != nil || string(got) != want {
			t.Errorf("staged updater file %s = %q, want %q, err=%v", name, got, want, err)
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
	for name, contents := range map[string]string{"bb-erp-updater.exe": "updater", "bb-erp-upgrade-runner.bat": "runner"} {
		if err := os.WriteFile(filepath.Join(sourceDir, name), []byte(contents), 0o700); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := validateServerPackage(sourceDir, ""); err == nil {
		t.Fatal("package without update-public.key must be rejected")
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "update-public.key"), []byte("invalid"), 0o600); err != nil {
		t.Fatalf("write invalid public key: %v", err)
	}
	if err := validateServerPackage(sourceDir, ""); err == nil {
		t.Fatal("package with invalid update-public.key must be rejected")
	}
	const publishedKey = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IDk4Y2E4ZTFhMmNlOWQxNTcKUldSWDBla3NHbzdLbUpoZnlmNWlwY1p6eEdEaUFiNmlFVVpsNTRIcnV0RmI5NjlFMytNNFlTQVcK"
	if err := os.WriteFile(filepath.Join(sourceDir, "update-public.key"), []byte(publishedKey), 0o600); err != nil {
		t.Fatalf("write published public key: %v", err)
	}
	// 显式 -package 升级兼容尚未携带 manifest_url 的历史正式包。
	if err := os.WriteFile(filepath.Join(sourceDir, "version.json"), []byte(`{"version":"0.0.8","server_version":"0.0.8"}`), 0o600); err != nil {
		t.Fatalf("write version metadata: %v", err)
	}
	if err := validateServerPackage(sourceDir, "0.0.8"); err != nil {
		t.Fatalf("validate published server package: %v", err)
	}
	if err := validateServerPackage(sourceDir, "0.0.9"); err == nil {
		t.Fatal("package metadata version must match the manifest version")
	}
}

func TestRestoreServerFilesReturnsReplacedFilesToBackup(t *testing.T) {
	backupDir := t.TempDir()
	installDir := t.TempDir()
	backupFiles := map[string]string{
		"bb-erp-server.exe": "old-server",
		"update-public.key": "old-key",
		"web/index.html":    "old-web",
		"version.json":      "old-version",
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
		"bb-erp-server.exe":                 {},
		"bb-erp-updater.pending.exe":        {},
		"bb-erp-upgrade-runner.pending.bat": {},
		"update-public.key":                 {},
		"web/index.html":                    {},
		"version.json":                      {},
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
	for _, name := range []string{"bb-erp-updater.pending.exe", "bb-erp-upgrade-runner.pending.bat", "update-public.key", "version.json", "web"} {
		if _, err := os.Stat(filepath.Join(installDir, name)); !os.IsNotExist(err) {
			t.Errorf("new path %s should be removed when absent from backup: %v", name, err)
		}
	}
}

func TestLoadUpdaterMetadataUsesInstalledManifestAndVersion(t *testing.T) {
	installDir := t.TempDir()
	contents := `{"version":"0.0.8","server_version":" 0.0.8 ","manifest_url":" https://example.com/update-manifest.json "}`
	if err := os.WriteFile(filepath.Join(installDir, "version.json"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write version metadata: %v", err)
	}
	metadata, err := loadUpdaterMetadata(installDir)
	if err != nil {
		t.Fatalf("load updater metadata: %v", err)
	}
	if metadata.ServerVersion != "0.0.8" || metadata.ManifestURL != "https://example.com/update-manifest.json" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func TestLoadUpdaterMetadataRequiresManifestURL(t *testing.T) {
	installDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(installDir, "version.json"), []byte(`{"server_version":"0.0.8"}`), 0o600); err != nil {
		t.Fatalf("write version metadata: %v", err)
	}
	if _, err := loadUpdaterMetadata(installDir); err == nil {
		t.Fatal("metadata without manifest_url must fail")
	}
}

func TestOpenUpgradeLogPersistsOutput(t *testing.T) {
	writer, closeLog, path, err := openUpgradeLog(t.TempDir())
	if err != nil {
		t.Fatalf("open updater log: %v", err)
	}
	if _, err := writer.Write([]byte("diagnostic details\n")); err != nil {
		t.Fatalf("write updater log: %v", err)
	}
	closeLog()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "diagnostic details\n" {
		t.Fatalf("persisted updater log = %q, err=%v", contents, err)
	}
}

func TestAcquireUpgradeLockRejectsConcurrentUpdater(t *testing.T) {
	installDir := t.TempDir()
	release, err := acquireUpgradeLock(installDir)
	if err != nil {
		t.Fatalf("acquire first updater lock: %v", err)
	}
	if _, err := acquireUpgradeLock(installDir); err == nil {
		release()
		t.Fatal("concurrent updater lock must be rejected")
	}
	release()
	secondRelease, err := acquireUpgradeLock(installDir)
	if err != nil {
		t.Fatalf("reacquire updater lock after release: %v", err)
	}
	secondRelease()
}

func TestVerifyFileSizeRejectsMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.zip")
	if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
		t.Fatalf("write package: %v", err)
	}
	if err := verifyFileSize(path, 4); err != nil {
		t.Fatalf("matching size rejected: %v", err)
	}
	if err := verifyFileSize(path, 5); err == nil {
		t.Fatal("mismatched size must fail")
	}
}

func TestValidSHA256RequiresExactHexDigest(t *testing.T) {
	if !validSHA256(strings.Repeat("a", 64)) {
		t.Fatal("valid SHA-256 was rejected")
	}
	for _, value := range []string{"", strings.Repeat("a", 63), strings.Repeat("z", 64)} {
		if validSHA256(value) {
			t.Fatalf("invalid SHA-256 was accepted: %q", value)
		}
	}
}

func TestDownloadServerPackageUsesInstalledTrustedKey(t *testing.T) {
	payload := []byte("signed-server-package")
	digest := sha256.Sum256(payload)
	trustedPublic, trustedPrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate trusted key: %v", err)
	}
	_, attackerPrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyPath, []byte(trustedPublic.String()), 0o600); err != nil {
		t.Fatalf("write trusted key: %v", err)
	}

	signature := signFileForTest(t, attackerPrivate, payload)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/package.zip" {
			_, _ = response.Write(payload)
			return
		}
		_ = json.NewEncoder(response).Encode(update.Manifest{Server: update.PackageManifest{
			Version: "0.0.9", URL: server.URL + "/package.zip", Size: int64(len(payload)),
			SHA256: hex.EncodeToString(digest[:]), Signature: signature,
		}})
	}))
	defer server.Close()

	if _, _, err := downloadServerPackage(server.URL, "0.0.8", keyPath); err == nil || !strings.Contains(err.Error(), "installed trusted public key") {
		t.Fatalf("attacker signature was not rejected: %v", err)
	}
	signature = signFileForTest(t, trustedPrivate, payload)
	packagePath, version, err := downloadServerPackage(server.URL, "0.0.8", keyPath)
	if err != nil {
		t.Fatalf("download trusted server package: %v", err)
	}
	defer os.Remove(packagePath)
	if version != "0.0.9" {
		t.Fatalf("downloaded package version=%q", version)
	}
}

func TestVerifyLocalServerPackageRequiresTrustedSignature(t *testing.T) {
	payload := []byte("local-server-package")
	packagePath := filepath.Join(t.TempDir(), "server.zip")
	if err := os.WriteFile(packagePath, payload, 0o600); err != nil {
		t.Fatalf("write local package: %v", err)
	}
	trustedPublic, trustedPrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate trusted key: %v", err)
	}
	_, attackerPrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyPath, []byte(trustedPublic.String()), 0o600); err != nil {
		t.Fatalf("write trusted key: %v", err)
	}
	if err := verifyLocalServerPackage(packagePath, "", keyPath); err == nil || !strings.Contains(err.Error(), "package-signature") {
		t.Fatalf("missing signature was not rejected: %v", err)
	}
	if err := verifyLocalServerPackage(packagePath, signFileForTest(t, attackerPrivate, payload), keyPath); err == nil {
		t.Fatal("attacker signature was not rejected")
	}
	if err := verifyLocalServerPackage(packagePath, signFileForTest(t, trustedPrivate, payload), keyPath); err != nil {
		t.Fatalf("trusted local signature was rejected: %v", err)
	}
}

func TestDownloadServerPackageRejectsOversizedManifest(t *testing.T) {
	public, _, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyPath, []byte(public.String()), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(response).Encode(update.Manifest{Server: update.PackageManifest{
			Version: "0.0.9", URL: "https://example.invalid/server.zip", Size: maxServerPackageSize + 1,
			SHA256: strings.Repeat("a", 64), Signature: "signed",
		}})
	}))
	defer server.Close()
	if _, _, err := downloadServerPackage(server.URL, "0.0.8", keyPath); err == nil || !strings.Contains(err.Error(), "1..") {
		t.Fatalf("oversized package manifest was not rejected: %v", err)
	}
}

func signFileForTest(t *testing.T, private minisign.PrivateKey, payload []byte) string {
	t.Helper()
	reader := minisign.NewReader(bytes.NewReader(payload))
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("hash signed file: %v", err)
	}
	return base64.StdEncoding.EncodeToString(reader.Sign(private))
}
