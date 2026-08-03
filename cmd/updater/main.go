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

	if err := stopServer(serviceName); err != nil {
		return err
	}

	backupDir := filepath.Join(installDir, "backups", time.Now().Format("20060102-150405"))
	if err := backupServerFiles(installDir, backupDir); err != nil {
		return err
	}
	if err := replaceServerFiles(sourceDir, installDir); err != nil {
		return err
	}
	if err := startServer(serviceName, installDir); err != nil {
		return err
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
		_ = exec.Command("sc.exe", "stop", serviceName).Run()
		time.Sleep(3 * time.Second)
		return nil
	}
	_ = exec.Command("taskkill.exe", "/F", "/IM", "bb-erp-server.exe").Run()
	time.Sleep(2 * time.Second)
	return nil
}

func startServer(serviceName string, installDir string) error {
	if serviceName != "" {
		return exec.Command("sc.exe", "start", serviceName).Run()
	}
	cmd := exec.Command(filepath.Join(installDir, "bb-erp-server.exe"))
	cmd.Dir = installDir
	return cmd.Start()
}

func backupServerFiles(installDir string, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	for _, name := range []string{"bb-erp-server.exe", "web"} {
		source := filepath.Join(installDir, name)
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if err := copyPath(source, filepath.Join(backupDir, name)); err != nil {
			return fmt.Errorf("backup %s: %w", name, err)
		}
	}
	return nil
}

func replaceServerFiles(sourceDir string, installDir string) error {
	if err := copyPath(filepath.Join(sourceDir, "bb-erp-server.exe"), filepath.Join(installDir, "bb-erp-server.exe")); err != nil {
		return fmt.Errorf("replace server exe: %w", err)
	}
	webSource := filepath.Join(sourceDir, "web")
	if _, err := os.Stat(webSource); err == nil {
		if err := os.RemoveAll(filepath.Join(installDir, "web")); err != nil {
			return fmt.Errorf("remove old web dist: %w", err)
		}
		if err := copyPath(webSource, filepath.Join(installDir, "web")); err != nil {
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
