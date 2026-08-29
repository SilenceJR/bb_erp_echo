package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bb_erp_echo/internal/update"
)

func main() {
	var manifestURL string
	var packagePath string
	var installDir string
	var serviceName string
	var currentVersion string

	flag.StringVar(&manifestURL, "manifest-url", "", "update-manifest.json URL from GitHub, Gitee, or intranet")
	flag.StringVar(&packagePath, "package", "", "local server zip package path")
	flag.StringVar(&installDir, "install-dir", ".", "server install directory")
	flag.StringVar(&serviceName, "service", "", "optional Windows service name")
	flag.StringVar(&currentVersion, "current-version", "", "current server version")
	flag.Parse()

	if err := run(manifestURL, packagePath, installDir, serviceName, currentVersion); err != nil {
		fmt.Fprintln(os.Stderr, "upgrade failed:", err)
		os.Exit(1)
	}
	fmt.Println("upgrade completed")
}

func run(manifestURL string, packagePath string, installDir string, serviceName string, currentVersion string) error {
	installDir, err := filepath.Abs(installDir)
	if err != nil {
		return fmt.Errorf("resolve install dir: %w", err)
	}
	if packagePath == "" {
		packagePath, err = downloadServerPackage(manifestURL, currentVersion)
		if err != nil {
			return err
		}
	}
	packagePath, err = filepath.Abs(packagePath)
	if err != nil {
		return fmt.Errorf("resolve package path: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "bb-erp-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZip(packagePath, tmpDir); err != nil {
		return err
	}

	sourceDir := filepath.Join(tmpDir, "server")
	if _, err := os.Stat(filepath.Join(sourceDir, "bb-erp-server.exe")); err != nil {
		sourceDir = tmpDir
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "bb-erp-server.exe")); err != nil {
		return fmt.Errorf("server package does not contain bb-erp-server.exe: %w", err)
	}
	if err := validateServerPackage(sourceDir); err != nil {
		return err
	}

	wasRunning, err := serverRunning(serviceName)
	if err != nil {
		return fmt.Errorf("inspect current server state: %w", err)
	}
	if err := stopServer(serviceName); err != nil {
		return err
	}

	backupDir := filepath.Join(installDir, "backups", time.Now().Format("20060102-150405"))
	if err := backupServerFiles(installDir, backupDir); err != nil {
		if wasRunning {
			if restartErr := startServer(serviceName, installDir); restartErr != nil {
				return fmt.Errorf("backup failed (%v); previous server restart failed: %w", err, restartErr)
			}
			return fmt.Errorf("backup failed and previous server was restarted: %w", err)
		}
		return err
	}
	if err := replaceServerFiles(sourceDir, installDir); err != nil {
		return recoverFailedUpgrade(serviceName, installDir, backupDir, wasRunning, err)
	}
	if err := startServer(serviceName, installDir); err != nil {
		return recoverFailedUpgrade(serviceName, installDir, backupDir, wasRunning, err)
	}
	return nil
}

func validateServerPackage(sourceDir string) error {
	serverPath := filepath.Join(sourceDir, "bb-erp-server.exe")
	serverInfo, err := os.Stat(serverPath)
	if err != nil || !serverInfo.Mode().IsRegular() || serverInfo.Size() == 0 {
		return fmt.Errorf("server package contains an invalid bb-erp-server.exe")
	}
	keyPath := filepath.Join(sourceDir, "update-public.key")
	keyInfo, err := os.Stat(keyPath)
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Size() == 0 {
		return fmt.Errorf("server package must contain a non-empty update-public.key")
	}
	if _, err := update.LoadSignedManifestVerifier("", keyPath); err != nil {
		return fmt.Errorf("validate server package update-public.key: %w", err)
	}
	return nil
}

func downloadServerPackage(manifestURL string, currentVersion string) (string, error) {
	if manifestURL == "" {
		return "", fmt.Errorf("either -package or -manifest-url is required")
	}
	manager := update.NewManagerForURL(manifestURL)
	manifest, err := manager.FetchManifest()
	if err != nil {
		return "", err
	}
	if currentVersion != "" && update.CompareVersions(manifest.Server.Version, currentVersion) <= 0 {
		return "", fmt.Errorf("server is already up to date: current=%s latest=%s", currentVersion, manifest.Server.Version)
	}
	if manifest.Server.URL == "" {
		return "", fmt.Errorf("manifest server.url is empty")
	}
	path := filepath.Join(os.TempDir(), "bb-erp-server-windows.zip")
	if err := downloadFile(manifest.Server.URL, path); err != nil {
		return "", err
	}
	if manifest.Server.SHA256 != "" {
		if err := verifySHA256(path, manifest.Server.SHA256); err != nil {
			return "", err
		}
	}
	return path, nil
}

func stopServer(serviceName string) error {
	if serviceName != "" {
		stopErr := exec.Command("sc.exe", "stop", serviceName).Run()
		deadline := time.Now().Add(30 * time.Second)
		for {
			stopped, err := serviceState(serviceName, "STOPPED")
			if err != nil {
				if stopErr != nil {
					return fmt.Errorf("stop service: %v; query service state: %w", stopErr, err)
				}
				return fmt.Errorf("query service state: %w", err)
			}
			if stopped {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("service %q did not stop within 30 seconds", serviceName)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	killErr := exec.Command("taskkill.exe", "/F", "/IM", "bb-erp-server.exe").Run()
	deadline := time.Now().Add(30 * time.Second)
	for {
		running, err := serverProcessRunning()
		if err != nil {
			if killErr != nil {
				return fmt.Errorf("stop server process: %v; query process state: %w", killErr, err)
			}
			return fmt.Errorf("query process state: %w", err)
		}
		if !running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bb-erp-server.exe did not stop within 30 seconds")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func startServer(serviceName string, installDir string) error {
	if serviceName != "" {
		if err := exec.Command("sc.exe", "start", serviceName).Run(); err != nil {
			return err
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			running, err := serviceState(serviceName, "RUNNING")
			if err != nil {
				return err
			}
			if running {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("service %q did not start within 30 seconds", serviceName)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	launcher := filepath.Join(installDir, "启动服务端.bat")
	var cmd *exec.Cmd
	if info, err := os.Stat(launcher); err == nil && info.Mode().IsRegular() {
		cmd = exec.Command("cmd.exe", "/D", "/C", launcher)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect server launcher: %w", err)
	} else {
		cmd = exec.Command(filepath.Join(installDir, "bb-erp-server.exe"))
	}
	cmd.Dir = installDir
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	var wrapperErr error
	select {
	case err := <-exited:
		wrapperErr = err
		<-timer.C
	case <-timer.C:
	}
	running, err := serverProcessRunning()
	if err != nil {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		return fmt.Errorf("verify server startup: %w", err)
	}
	if running {
		return nil
	}
	if cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
	}
	if wrapperErr != nil {
		return fmt.Errorf("server did not remain running after launcher exited: %w", wrapperErr)
	}
	return fmt.Errorf("server did not remain running after launcher completed")
}

func serverRunning(serviceName string) (bool, error) {
	if serviceName != "" {
		return serviceState(serviceName, "RUNNING")
	}
	return serverProcessRunning()
}

func serviceState(serviceName, state string) (bool, error) {
	output, err := exec.Command("sc.exe", "query", serviceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("sc.exe query %q: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToUpper(string(output)), state), nil
}

func serverProcessRunning() (bool, error) {
	output, err := exec.Command("tasklist.exe", "/NH", "/FI", "IMAGENAME eq bb-erp-server.exe").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("tasklist.exe: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToLower(string(output)), "bb-erp-server.exe"), nil
}

func backupServerFiles(installDir string, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	// Keep the rollback snapshot self-contained. Runtime configuration is supplied
	// through environment variables and must be backed up by the deployment system,
	// not copied into an upgrade archive.
	for _, name := range []string{"bb-erp-server.exe", "web", "data", "static", "updates", "logs", "update-public.key"} {
		source := filepath.Join(installDir, name)
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) && name != "bb-erp-server.exe" {
				continue
			}
			return fmt.Errorf("inspect %s for backup: %w", name, err)
		}
		if err := copyPath(source, filepath.Join(backupDir, name)); err != nil {
			return fmt.Errorf("backup %s: %w", name, err)
		}
	}
	return nil
}

func replaceServerFiles(sourceDir string, installDir string) error {
	if err := replaceFileSafely(filepath.Join(sourceDir, "bb-erp-server.exe"), filepath.Join(installDir, "bb-erp-server.exe")); err != nil {
		return fmt.Errorf("replace server exe: %w", err)
	}
	if err := replaceFileSafely(filepath.Join(sourceDir, "update-public.key"), filepath.Join(installDir, "update-public.key")); err != nil {
		return fmt.Errorf("replace update-public.key: %w", err)
	}
	webSource := filepath.Join(sourceDir, "web")
	if _, err := os.Stat(webSource); err == nil {
		if err := replaceDirectorySafely(webSource, filepath.Join(installDir, "web")); err != nil {
			return fmt.Errorf("replace web dist: %w", err)
		}
	}
	for _, name := range []string{"data", "logs", "updates", "backups"} {
		if err := os.MkdirAll(filepath.Join(installDir, name), 0o755); err != nil {
			return fmt.Errorf("ensure %s dir: %w", name, err)
		}
	}
	return nil
}

func recoverFailedUpgrade(serviceName, installDir, backupDir string, restartPrevious bool, cause error) error {
	_ = stopServer(serviceName)
	restoreErr := restoreServerFiles(backupDir, installDir)
	if restoreErr != nil {
		return fmt.Errorf("upgrade failed (%v); restore backup failed: %w", cause, restoreErr)
	}
	if restartPrevious {
		if restartErr := startServer(serviceName, installDir); restartErr != nil {
			return fmt.Errorf("upgrade failed (%v); backup restored but previous server restart failed: %w", cause, restartErr)
		}
		return fmt.Errorf("upgrade failed and previous version was restored and restarted: %w", cause)
	}
	return fmt.Errorf("upgrade failed and previous stopped version was restored: %w", cause)
}

func restoreServerFiles(backupDir, installDir string) error {
	for _, name := range []string{"bb-erp-server.exe", "update-public.key", "web"} {
		source := filepath.Join(backupDir, name)
		target := filepath.Join(installDir, name)
		info, err := os.Stat(source)
		if os.IsNotExist(err) {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove newly installed %s: %w", name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect backup %s: %w", name, err)
		}
		if info.IsDir() {
			err = replaceDirectorySafely(source, target)
		} else {
			err = replaceFileSafely(source, target)
		}
		if err != nil {
			return fmt.Errorf("restore %s: %w", name, err)
		}
	}
	return nil
}

func replaceFileSafely(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".new-*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(info.Mode()); err != nil {
		_ = staged.Close()
		return err
	}
	if _, err := io.Copy(staged, input); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	return swapPreparedPath(stagedPath, target)
}

func replaceDirectorySafely(source, target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stagedPath, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".new-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagedPath)
	if err := copyPath(source, stagedPath); err != nil {
		return err
	}
	return swapPreparedPath(stagedPath, target)
}

func swapPreparedPath(stagedPath, target string) error {
	backupHandle, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".old-*")
	if err != nil {
		return err
	}
	backupPath := backupHandle.Name()
	if err := backupHandle.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	cleanupBackup := true
	defer func() {
		if cleanupBackup {
			_ = os.RemoveAll(backupPath)
		}
	}()
	targetExists := false
	if _, err := os.Stat(target); err == nil {
		targetExists = true
		if err := os.Rename(target, backupPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stagedPath, target); err != nil {
		if targetExists {
			if restoreErr := os.Rename(backupPath, target); restoreErr != nil {
				cleanupBackup = false
				return fmt.Errorf("install prepared path: %v; restore previous path from %q: %w", err, backupPath, restoreErr)
			}
		}
		return err
	}
	return nil
}

func extractZip(path string, dest string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	for _, item := range reader.File {
		target := filepath.Join(dest, item.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe zip path: %s", item.Name)
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, item.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := item.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, source, item.Mode())
		_ = source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyPath(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(target, info.Mode()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	return writeFile(target, input, info.Mode())
}

func writeFile(target string, reader io.Reader, mode os.FileMode) error {
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer output.Close()
	_, err = io.Copy(output, reader)
	return err
}

func downloadFile(url string, path string) error {
	res, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download package: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("download package status %d", res.StatusCode)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create package file: %w", err)
	}
	defer file.Close()
	_, err = io.Copy(file, res.Body)
	return err
}

func verifySHA256(path string, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("sha256 mismatch: got %s", got)
	}
	return nil
}
