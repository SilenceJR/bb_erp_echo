package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/update"
)

type updaterMetadata struct {
	Version       string `json:"version"`
	ServerVersion string `json:"server_version"`
	ManifestURL   string `json:"manifest_url"`
	UpdateSource  string `json:"update_source"`
	ReleaseDir    string `json:"release_dir"`
}

var errAlreadyUpToDate = errors.New("server is already up to date")

const maxServerPackageSize = int64(512 << 20)

func main() {
	var manifestURL string
	var releaseDir string
	var packagePath string
	var packageSignature string
	var installDir string
	var serviceName string
	var currentVersion string

	flag.StringVar(&manifestURL, "manifest-url", "", "update-manifest.json URL from GitHub, Gitee, or intranet")
	flag.StringVar(&releaseDir, "release-dir", "", "local active update release directory containing update-manifest.json")
	flag.StringVar(&packagePath, "package", "", "local server zip package path")
	flag.StringVar(&packageSignature, "package-signature", "", "base64 Minisign signature for a local server zip package")
	interactive := len(os.Args) == 1
	flag.StringVar(&installDir, "install-dir", executableDirectory(), "server install directory")
	flag.StringVar(&serviceName, "service", "", "optional Windows service name")
	flag.StringVar(&currentVersion, "current-version", "", "current server version")
	flag.Parse()

	output, closeLog, logPath, logErr := openUpgradeLog(installDir)
	if logErr != nil {
		fmt.Fprintln(os.Stderr, "warning: create updater log:", logErr)
	}
	err := runWithProgressSource(manifestURL, releaseDir, packagePath, packageSignature, installDir, serviceName, currentVersion, output)
	if err != nil {
		fmt.Fprintln(output, "upgrade failed:", err)
	} else {
		fmt.Fprintln(output, "upgrade completed")
	}
	if logPath != "" {
		fmt.Fprintln(output, "log file:", logPath)
	}
	closeLog()
	if interactive {
		fmt.Fprintln(os.Stdout, "Press Enter to close this window.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err != nil {
		os.Exit(1)
	}
}

func runWithProgress(manifestURL string, packagePath string, packageSignature string, installDir string, serviceName string, currentVersion string, output io.Writer) error {
	return runWithProgressSource(manifestURL, "", packagePath, packageSignature, installDir, serviceName, currentVersion, output)
}

// runWithProgressSource 执行 HTTP manifest、directory manifest 或显式本地包升级。
// 保留 runWithProgress 作为旧调用方兼容包装，避免改变既有测试和内部集成契约。
func runWithProgressSource(manifestURL string, releaseDir string, packagePath string, packageSignature string, installDir string, serviceName string, currentVersion string, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	manifestURL = strings.TrimSpace(manifestURL)
	releaseDir = strings.TrimSpace(releaseDir)
	packagePath = strings.TrimSpace(packagePath)
	if manifestURL != "" && releaseDir != "" {
		return errors.New("-manifest-url and -release-dir cannot be used together")
	}
	if packagePath != "" && releaseDir != "" {
		return errors.New("-package and -release-dir cannot be used together")
	}
	installDir, err := filepath.Abs(installDir)
	if err != nil {
		return fmt.Errorf("resolve install dir: %w", err)
	}
	releaseLock, err := acquireUpgradeLock(installDir)
	if err != nil {
		return err
	}
	defer releaseLock()
	if packagePath == "" && manifestURL == "" && releaseDir == "" {
		metadata, metadataErr := loadUpdaterMetadata(installDir)
		if metadataErr != nil {
			return fmt.Errorf("no -package, -manifest-url or -release-dir was supplied and installed version metadata is unavailable: %w", metadataErr)
		}
		if metadata.UpdateSource == "directory" {
			releaseDir = metadata.ReleaseDir
		} else {
			manifestURL = metadata.ManifestURL
		}
		if currentVersion == "" {
			currentVersion = metadata.ServerVersion
		}
		if currentVersion == "" {
			currentVersion = metadata.Version
		}
	}
	downloadedPackage := false
	expectedVersion := ""
	if packagePath == "" {
		if releaseDir != "" {
			fmt.Fprintln(output, "[1/6] Reading and verifying the local server package...")
			packagePath, expectedVersion, err = downloadServerPackageFromDirectory(releaseDir, currentVersion, filepath.Join(installDir, "update-public.key"))
		} else {
			fmt.Fprintln(output, "[1/6] Downloading and verifying the server package...")
			packagePath, expectedVersion, err = downloadServerPackage(manifestURL, currentVersion, filepath.Join(installDir, "update-public.key"))
		}
		if err != nil {
			if errors.Is(err, errAlreadyUpToDate) {
				fmt.Fprintln(output, err)
				return nil
			}
			return err
		}
		downloadedPackage = true
	}
	packagePath, err = filepath.Abs(packagePath)
	if err != nil {
		return fmt.Errorf("resolve package path: %w", err)
	}
	if downloadedPackage {
		defer os.Remove(packagePath)
	} else if err := verifyLocalServerPackage(packagePath, packageSignature, filepath.Join(installDir, "update-public.key")); err != nil {
		return err
	}

	fmt.Fprintln(output, "[2/6] Extracting and validating the server package...")
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
	if err := validateServerPackage(sourceDir, expectedVersion); err != nil {
		return err
	}
	if serviceName != "" {
		if err := validateWindowsServiceTarget(serviceName, installDir); err != nil {
			return err
		}
	}

	wasRunning, err := serverRunning(serviceName, installDir)
	if err != nil {
		return fmt.Errorf("inspect current server state: %w", err)
	}
	backupRoot := filepath.Join(installDir, "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return fmt.Errorf("create backup root: %w", err)
	}
	backupDir, err := os.MkdirTemp(backupRoot, time.Now().Format("20060102-150405")+"-*")
	if err != nil {
		return fmt.Errorf("create unique backup directory: %w", err)
	}
	fmt.Fprintln(output, "[3/6] Stopping the current server...")
	if err := stopServer(serviceName, installDir); err != nil {
		return err
	}
	fmt.Fprintln(output, "[4/6] Backing up the current deployment to", backupDir)
	if err := backupServerFiles(installDir, backupDir); err != nil {
		if wasRunning {
			if restartErr := startServer(serviceName, installDir); restartErr != nil {
				return fmt.Errorf("backup failed (%v); previous server restart failed: %w", err, restartErr)
			}
			return fmt.Errorf("backup failed and previous server was restarted: %w", err)
		}
		return err
	}
	fmt.Fprintln(output, "[5/6] Installing the verified server files...")
	if err := replaceServerFiles(sourceDir, installDir); err != nil {
		return recoverFailedUpgrade(serviceName, installDir, backupDir, wasRunning, err)
	}
	fmt.Fprintln(output, "[6/6] Starting and verifying the updated server...")
	if err := startServer(serviceName, installDir); err != nil {
		return recoverFailedUpgrade(serviceName, installDir, backupDir, wasRunning, err)
	}
	return nil
}

func executableDirectory() string {
	executable, err := os.Executable()
	if err == nil {
		if absolute, absErr := filepath.Abs(executable); absErr == nil {
			return filepath.Dir(absolute)
		}
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return workingDir
}

func openUpgradeLog(installDir string) (io.Writer, func(), string, error) {
	absoluteDir, err := filepath.Abs(installDir)
	if err != nil {
		return os.Stdout, func() {}, "", err
	}
	logDir := filepath.Join(absoluteDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return os.Stdout, func() {}, "", err
	}
	file, err := os.CreateTemp(logDir, "server-updater-"+time.Now().Format("20060102-150405")+"-*.log")
	if err != nil {
		return os.Stdout, func() {}, "", err
	}
	logPath := file.Name()
	return io.MultiWriter(os.Stdout, file), func() { _ = file.Close() }, logPath, nil
}

func readUpdaterMetadata(installDir string) (updaterMetadata, error) {
	path := filepath.Join(installDir, "version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return updaterMetadata{}, fmt.Errorf("read %s: %w", path, err)
	}
	var metadata updaterMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return updaterMetadata{}, fmt.Errorf("decode %s: %w", path, err)
	}
	metadata.ManifestURL = strings.TrimSpace(metadata.ManifestURL)
	metadata.UpdateSource = strings.ToLower(strings.TrimSpace(metadata.UpdateSource))
	metadata.ReleaseDir = strings.TrimSpace(metadata.ReleaseDir)
	metadata.ServerVersion = strings.TrimSpace(metadata.ServerVersion)
	metadata.Version = strings.TrimSpace(metadata.Version)
	return metadata, nil
}

func loadUpdaterMetadata(installDir string) (updaterMetadata, error) {
	metadata, err := readUpdaterMetadata(installDir)
	if err != nil {
		return updaterMetadata{}, err
	}
	metadataPath := filepath.Join(installDir, "version.json")
	switch metadata.UpdateSource {
	case "":
		// Existing installations only carry manifest_url. When a package is
		// explicitly configured with release_dir but omits update_source, infer
		// directory mode so the runner remains useful across metadata revisions.
		if metadata.ReleaseDir != "" {
			metadata.UpdateSource = "directory"
			break
		}
		if metadata.ManifestURL == "" {
			return updaterMetadata{}, fmt.Errorf("%s does not contain manifest_url", metadataPath)
		}
	case "http":
		if metadata.ManifestURL == "" {
			return updaterMetadata{}, fmt.Errorf("%s does not contain manifest_url for http update source", metadataPath)
		}
	case "directory":
		if metadata.ReleaseDir == "" {
			return updaterMetadata{}, fmt.Errorf("%s does not contain release_dir for directory update source", metadataPath)
		}
	default:
		return updaterMetadata{}, fmt.Errorf("%s contains unsupported update_source %q", metadataPath, metadata.UpdateSource)
	}
	return metadata, nil
}

func validateServerPackage(sourceDir, expectedVersion string) error {
	for _, name := range []string{"bb-erp-server.exe", "bb-erp-updater.exe", "bb-erp-upgrade-runner.bat"} {
		info, err := os.Stat(filepath.Join(sourceDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("server package contains an invalid %s", name)
		}
	}
	keyPath := filepath.Join(sourceDir, "update-public.key")
	keyInfo, err := os.Stat(keyPath)
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Size() == 0 {
		return fmt.Errorf("server package must contain a non-empty update-public.key")
	}
	if _, err := update.LoadSignedManifestVerifier("", keyPath); err != nil {
		return fmt.Errorf("validate server package update-public.key: %w", err)
	}
	metadata, err := readUpdaterMetadata(sourceDir)
	if err != nil {
		return fmt.Errorf("validate server package version.json: %w", err)
	}
	if metadata.ServerVersion == "" && metadata.Version == "" {
		return fmt.Errorf("validate server package version.json: version is empty")
	}
	packageVersion := metadata.ServerVersion
	if packageVersion == "" {
		packageVersion = metadata.Version
	}
	if expectedVersion != "" && update.CompareVersions(packageVersion, expectedVersion) != 0 {
		return fmt.Errorf("validate server package version.json: package=%s manifest=%s", packageVersion, expectedVersion)
	}
	return nil
}

func downloadServerPackage(manifestURL string, currentVersion string, trustedPublicKeyPath string) (string, string, error) {
	if manifestURL == "" {
		return "", "", fmt.Errorf("either -package or -manifest-url is required")
	}
	manager := update.NewManagerForURL(manifestURL)
	manifest, err := manager.FetchManifest()
	if err != nil {
		return "", "", err
	}
	if currentVersion != "" && update.CompareVersions(manifest.Server.Version, currentVersion) <= 0 {
		return "", "", fmt.Errorf("%w: current=%s latest=%s", errAlreadyUpToDate, currentVersion, manifest.Server.Version)
	}
	if manifest.Server.URL == "" {
		return "", "", fmt.Errorf("manifest server.url is empty")
	}
	if manifest.Server.Size <= 0 || manifest.Server.Size > maxServerPackageSize || !validSHA256(manifest.Server.SHA256) || strings.TrimSpace(manifest.Server.Signature) == "" {
		return "", "", fmt.Errorf("manifest server package must declare a size within 1..%d, SHA-256 and Minisign signature", maxServerPackageSize)
	}
	verifier, err := update.LoadSignedManifestVerifier("", trustedPublicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("load installed trusted update public key: %w", err)
	}
	if verifier == nil {
		return "", "", fmt.Errorf("installed trusted update public key is not configured")
	}
	temporary, err := os.CreateTemp("", "bb-erp-server-windows-*.zip")
	if err != nil {
		return "", "", fmt.Errorf("create temporary server package: %w", err)
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("close temporary server package: %w", err)
	}
	if err := downloadFile(manifest.Server.URL, path, manifest.Server.Size, manifest.Server.SHA256); err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	if err := verifier.VerifyFile(path, manifest.Server.Signature); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("verify server package with installed trusted public key: %w", err)
	}
	return path, manifest.Server.Version, nil
}

// downloadServerPackageFromDirectory 读取 active release 目录中的清单和服务端包。
// 目录源不调用 HTTP 客户端；路径、大小、SHA-256 和 Minisign 均在资源离开
// release 目录前完成校验，后续 runWithProgressSource 仍会执行 ZIP 和版本结构校验。
func downloadServerPackageFromDirectory(releaseDir string, currentVersion string, trustedPublicKeyPath string) (string, string, error) {
	releaseDir = strings.TrimSpace(releaseDir)
	if releaseDir == "" {
		return "", "", errors.New("-release-dir is empty")
	}
	source := update.NewDirectoryManifestSource(releaseDir)
	manifest, err := source.Fetch(context.Background())
	if err != nil {
		return "", "", fmt.Errorf("read local update manifest: %w", err)
	}
	if currentVersion != "" && update.CompareVersions(manifest.Server.Version, currentVersion) <= 0 {
		return "", "", fmt.Errorf("%w: current=%s latest=%s", errAlreadyUpToDate, currentVersion, manifest.Server.Version)
	}
	if manifest.Server.Version == "" {
		return "", "", errors.New("manifest server.version is empty")
	}
	if strings.TrimSpace(manifest.Server.URL) == "" {
		return "", "", errors.New("manifest server.url is empty")
	}
	if manifest.Server.Size <= 0 || manifest.Server.Size > maxServerPackageSize || !validSHA256(manifest.Server.SHA256) || strings.TrimSpace(manifest.Server.Signature) == "" {
		return "", "", fmt.Errorf("manifest server package must declare a size within 1..%d, SHA-256 and Minisign signature", maxServerPackageSize)
	}
	verifier, err := update.LoadSignedManifestVerifier("", trustedPublicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("load installed trusted update public key: %w", err)
	}
	if verifier == nil {
		return "", "", errors.New("installed trusted update public key is not configured")
	}

	// Use the existing directory package store for path confinement and the
	// streaming size/SHA-256 check. Its temporary root is removed after the
	// verified package is copied to the updater-owned temporary file below.
	cacheRoot, err := os.MkdirTemp("", "bb-erp-directory-update-cache-*")
	if err != nil {
		return "", "", fmt.Errorf("create temporary local update cache: %w", err)
	}
	defer os.RemoveAll(cacheRoot)
	store := &update.DirectoryPackageStore{Root: cacheRoot, ReleaseDir: releaseDir}
	resourceName := "server/" + filepath.Base(filepath.FromSlash(manifest.Server.URL))
	cachedPath, _, err := store.Ensure(context.Background(), resourceName, manifest.Server)
	if err != nil {
		return "", "", fmt.Errorf("copy local server package: %w", err)
	}

	temporary, err := os.CreateTemp("", "bb-erp-server-windows-*.zip")
	if err != nil {
		return "", "", fmt.Errorf("create temporary server package: %w", err)
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("close temporary server package: %w", err)
	}
	if err := copyPath(cachedPath, path); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("copy verified local server package: %w", err)
	}
	if err := verifyFileSize(path, manifest.Server.Size); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("verify local server package size: %w", err)
	}
	if err := verifySHA256(path, manifest.Server.SHA256); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("verify local server package SHA-256: %w", err)
	}
	if err := verifier.VerifyFile(path, manifest.Server.Signature); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("verify local server package with installed trusted public key: %w", err)
	}
	return path, manifest.Server.Version, nil
}

func verifyLocalServerPackage(packagePath, signature, trustedPublicKeyPath string) error {
	info, err := os.Stat(packagePath)
	if err != nil {
		return fmt.Errorf("inspect local server package: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxServerPackageSize {
		return fmt.Errorf("local server package size %d is outside the allowed range 1..%d", info.Size(), maxServerPackageSize)
	}
	if strings.TrimSpace(signature) == "" {
		return errors.New("local server package requires -package-signature from the signed release manifest")
	}
	verifier, err := update.LoadSignedManifestVerifier("", trustedPublicKeyPath)
	if err != nil {
		return fmt.Errorf("load installed trusted update public key: %w", err)
	}
	if verifier == nil {
		return errors.New("installed trusted update public key is not configured")
	}
	if err := verifier.VerifyFile(packagePath, signature); err != nil {
		return fmt.Errorf("verify local server package with installed trusted public key: %w", err)
	}
	return nil
}

func validateWindowsServiceTarget(serviceName, installDir string) error {
	target, err := filepath.Abs(filepath.Join(installDir, "bb-erp-server.exe"))
	if err != nil {
		return fmt.Errorf("resolve installed server executable: %w", err)
	}
	const script = `$service=Get-CimInstance Win32_Service | Where-Object { $_.Name -eq $args[0] } | Select-Object -First 1; if (-not $service) { Write-Error "service not found"; exit 2 }; $raw=$service.PathName.Trim(); if ($raw.StartsWith('"')) { $exe=$raw.Split('"')[1] } elseif ($raw -match '^(.*?[.]exe)(?:\s|$)') { $exe=$Matches[1] } else { Write-Error "cannot parse service executable path"; exit 3 }; [IO.Path]::GetFullPath($exe)`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("validate Windows service %q executable: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	actual := strings.TrimSpace(string(output))
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(target)) {
		return fmt.Errorf("Windows service %q points to %q, expected %q", serviceName, actual, target)
	}
	return nil
}

func stopServer(serviceName, installDir string) error {
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
	pids, err := serverProcessIDs(installDir)
	if err != nil {
		return fmt.Errorf("locate server process for install directory: %w", err)
	}
	serviceNames, err := windowsServicesForProcessIDs(pids)
	if err != nil {
		return fmt.Errorf("inspect Windows Service ownership: %w", err)
	}
	if len(serviceNames) > 0 {
		return fmt.Errorf("server process is managed by Windows Service %q; rerun with -service or set BB_ERP_WINDOWS_SERVICE_NAME", strings.Join(serviceNames, ","))
	}
	var killErr error
	for _, pid := range pids {
		output, err := exec.Command("taskkill.exe", "/F", "/T", "/PID", strconv.FormatUint(uint64(pid), 10)).CombinedOutput()
		if err != nil {
			killErr = fmt.Errorf("taskkill PID %d: %w: %s", pid, err, strings.TrimSpace(string(output)))
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		running, err := serverProcessRunning(installDir)
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
				return waitServerReady()
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
	running, err := serverProcessRunning(installDir)
	if err != nil {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		return fmt.Errorf("verify server startup: %w", err)
	}
	if running {
		return waitServerReady()
	}
	if cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
	}
	if wrapperErr != nil {
		return fmt.Errorf("server did not remain running after launcher exited: %w", wrapperErr)
	}
	return fmt.Errorf("server did not remain running after launcher completed")
}

func waitServerReady() error {
	port := strings.TrimSpace(os.Getenv("BB_ERP_HTTP_PORT"))
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	readyURL := "http://127.0.0.1:" + port + "/ready"
	var lastErr error = errors.New("no readiness response")
	for time.Now().Before(deadline) {
		response, err := client.Get(readyURL)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server readiness check %s failed within 45 seconds: %w", readyURL, lastErr)
}

func serverRunning(serviceName, installDir string) (bool, error) {
	if serviceName != "" {
		return serviceState(serviceName, "RUNNING")
	}
	return serverProcessRunning(installDir)
}

func serviceState(serviceName, state string) (bool, error) {
	output, err := exec.Command("sc.exe", "query", serviceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("sc.exe query %q: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToUpper(string(output)), state), nil
}

func serverProcessRunning(installDir string) (bool, error) {
	pids, err := serverProcessIDs(installDir)
	return len(pids) > 0, err
}

func serverProcessIDs(installDir string) ([]uint32, error) {
	target, err := filepath.Abs(filepath.Join(installDir, "bb-erp-server.exe"))
	if err != nil {
		return nil, err
	}
	const script = `$target=[IO.Path]::GetFullPath($args[0]); Get-CimInstance Win32_Process -Filter "Name='bb-erp-server.exe'" | Where-Object { $_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath) -eq $target } | ForEach-Object { $_.ProcessId }`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, target).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query exact server process path: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var pids []uint32
	for _, line := range strings.Fields(string(output)) {
		pid, parseErr := strconv.ParseUint(strings.TrimSpace(line), 10, 32)
		if parseErr != nil {
			return nil, fmt.Errorf("parse server process id %q: %w", line, parseErr)
		}
		pids = append(pids, uint32(pid))
	}
	return pids, nil
}

func windowsServicesForProcessIDs(pids []uint32) ([]string, error) {
	if len(pids) == 0 {
		return nil, nil
	}
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, strconv.FormatUint(uint64(pid), 10))
	}
	const script = `$ids=$args[0].Split(',') | ForEach-Object { [uint32]$_ }; Get-CimInstance Win32_Service | Where-Object { $_.ProcessId -in $ids } | ForEach-Object { $_.Name }`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, strings.Join(values, ",")).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query service process ownership: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.Fields(string(output)), nil
}

func backupServerFiles(installDir string, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	// Keep the rollback snapshot self-contained. Runtime configuration is supplied
	// through environment variables and must be backed up by the deployment system,
	// not copied into an upgrade archive.
	for _, name := range []string{"bb-erp-server.exe", "bb-erp-updater.exe", "bb-erp-upgrade-runner.bat", "bb-erp-verify-update.exe", "激活离线更新.ps1", "web", "data", "static", "updates", "logs", "update-public.key", "version.json"} {
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
	if err := replaceFileSafely(filepath.Join(sourceDir, "version.json"), filepath.Join(installDir, "version.json")); err != nil {
		return fmt.Errorf("replace version.json: %w", err)
	}
	if err := replaceFileSafely(filepath.Join(sourceDir, "bb-erp-updater.exe"), filepath.Join(installDir, "bb-erp-updater.pending.exe")); err != nil {
		return fmt.Errorf("stage next updater executable: %w", err)
	}
	if err := replaceFileSafely(filepath.Join(sourceDir, "bb-erp-upgrade-runner.bat"), filepath.Join(installDir, "bb-erp-upgrade-runner.pending.bat")); err != nil {
		return fmt.Errorf("stage next updater runner: %w", err)
	}
	if err := replaceFileSafely(filepath.Join(sourceDir, "bb-erp-verify-update.exe"), filepath.Join(installDir, "bb-erp-verify-update.exe")); err != nil {
		return fmt.Errorf("replace trusted update verifier: %w", err)
	}
	if err := replaceFileSafely(filepath.Join(sourceDir, "激活离线更新.ps1"), filepath.Join(installDir, "激活离线更新.ps1")); err != nil {
		return fmt.Errorf("replace trusted offline activation script: %w", err)
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
	_ = stopServer(serviceName, installDir)
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
	for _, name := range []string{
		"bb-erp-server.exe",
		"bb-erp-updater.pending.exe", "bb-erp-upgrade-runner.pending.bat",
		"bb-erp-verify-update.exe", "激活离线更新.ps1",
		"update-public.key", "version.json", "web", "data", "static",
	} {
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
	const (
		maxArchiveEntries = 10_000
		maxExpandedBytes  = uint64(1 << 30)
	)
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > maxArchiveEntries {
		return fmt.Errorf("zip contains too many entries: got %d max %d", len(reader.File), maxArchiveEntries)
	}
	var expandedBytes uint64
	for _, item := range reader.File {
		if item.UncompressedSize64 > maxExpandedBytes-expandedBytes {
			return fmt.Errorf("zip expanded size exceeds %d bytes", maxExpandedBytes)
		}
		expandedBytes += item.UncompressedSize64
	}
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

func downloadFile(url string, path string, expectedSize int64, expectedSHA256 string) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	res, err := client.Get(url)
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
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, expectedSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("write package file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close package file: %w", closeErr)
	}
	if written != expectedSize {
		return fmt.Errorf("package size mismatch: got %d want %d", written, expectedSize)
	}
	gotSHA256 := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(gotSHA256, strings.TrimSpace(expectedSHA256)) {
		return fmt.Errorf("sha256 mismatch: got %s", gotSHA256)
	}
	return nil
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
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

func verifyFileSize(path string, want int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() != want {
		return fmt.Errorf("package size mismatch: got %d want %d", info.Size(), want)
	}
	return nil
}
