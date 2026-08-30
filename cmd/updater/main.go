package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	pathpkg "path"
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
	ManifestFile  string `json:"manifest_file"`
}

var errAlreadyUpToDate = errors.New("server is already up to date")

const maxServerPackageSize = int64(512 << 20)

func main() {
	var manifestURL string
	var packagePath string
	var packageSignature string
	var candidateManifest string
	var installDir string
	var serviceName string
	var currentVersion string
	var healthBaseURL string
	var databasePath string
	var trustedPublicKey string
	var recoverInterrupted bool

	flag.StringVar(&manifestURL, "manifest-url", "", "legacy update-manifest.json URL")
	flag.StringVar(&packagePath, "package", "", "local server zip package path")
	flag.StringVar(&packageSignature, "package-signature", "", "base64 Minisign signature for a local server zip package")
	flag.StringVar(&candidateManifest, "candidate-manifest", "", "independent local candidate update-manifest.json")
	interactive := len(os.Args) == 1
	flag.StringVar(&installDir, "install-dir", executableDirectory(), "server install directory")
	flag.StringVar(&serviceName, "service", "", "optional Windows service name")
	flag.StringVar(&currentVersion, "current-version", "", "current server version")
	flag.StringVar(&healthBaseURL, "health-base-url", "", "base URL used to verify /ready, /api/v1/version and client plan after restart")
	flag.StringVar(&databasePath, "database-path", "", "SQLite database path; relative paths are resolved from the install directory")
	flag.StringVar(&trustedPublicKey, "trusted-public-key", "", "external Minisign public-key file used to bootstrap a first installation")
	flag.BoolVar(&recoverInterrupted, "recover-interrupted", false, "recover an interrupted local upgrade transaction and exit")
	flag.Parse()

	output, closeLog, logPath, logErr := openUpgradeLog(installDir)
	if logErr != nil {
		fmt.Fprintln(os.Stderr, "warning: create updater log:", logErr)
	}
	var err error
	if recoverInterrupted {
		err = runInterruptedRecovery(installDir, serviceName, databasePath, output)
	} else {
		err = runWithProgress(manifestURL, packagePath, packageSignature, candidateManifest, installDir, serviceName, currentVersion, healthBaseURL, databasePath, output, trustedPublicKey)
	}
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

func runInterruptedRecovery(installDir, serviceName, databasePath string, output io.Writer) error {
	installDir, err := filepath.Abs(installDir)
	if err != nil {
		return fmt.Errorf("resolve install dir: %w", err)
	}
	releaseLock, err := acquireUpgradeLock(installDir)
	if err != nil {
		return err
	}
	defer releaseLock()
	databasePath, err = resolveDatabasePath(installDir, databasePath)
	if err != nil {
		return err
	}
	return recoverInterruptedUpgrade(installDir, serviceName, databasePath, output)
}

func runWithProgress(manifestURL string, packagePath string, packageSignature string, candidateManifest string, installDir string, serviceName string, currentVersion string, healthBaseURL string, databasePath string, output io.Writer, trustedPublicKey ...string) error {
	if output == nil {
		output = io.Discard
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
	databasePath, err = resolveDatabasePath(installDir, databasePath)
	if err != nil {
		return err
	}
	if err := recoverInterruptedUpgrade(installDir, serviceName, databasePath, output); err != nil {
		return err
	}
	trustedPublicKeyPath, err := resolveTrustedPublicKeyPath(installDir, trustedPublicKey...)
	if err != nil {
		return err
	}
	if packagePath == "" && manifestURL == "" && candidateManifest == "" {
		metadata, metadataErr := loadUpdaterMetadata(installDir)
		if metadataErr != nil {
			return fmt.Errorf("no -package, -candidate-manifest, or -manifest-url was supplied and installed version metadata is unavailable: %w", metadataErr)
		}
		manifestURL = metadata.ManifestURL
		if metadata.ManifestFile != "" {
			candidateManifest = metadata.ManifestFile
			if !filepath.IsAbs(candidateManifest) {
				candidateManifest = filepath.Join(installDir, candidateManifest)
			}
		}
		if currentVersion == "" {
			currentVersion = metadata.ServerVersion
		}
		if currentVersion == "" {
			currentVersion = metadata.Version
		}
	}
	if currentVersion == "" {
		// Explicit candidate/package invocations still need the previous version
		// for rollback health checks. A malformed or missing metadata file is not
		// fatal here; package validation below remains authoritative.
		if metadata, metadataErr := readUpdaterMetadata(installDir); metadataErr == nil {
			currentVersion = metadata.ServerVersion
			if currentVersion == "" {
				currentVersion = metadata.Version
			}
		}
	}
	downloadedPackage := false
	expectedVersion := ""
	var candidate *candidateRelease
	if strings.TrimSpace(candidateManifest) != "" {
		candidate, err = loadCandidateReleaseWithPublicKey(candidateManifest, installDir, trustedPublicKeyPath)
		if err != nil {
			return err
		}
		expectedVersion = candidate.Manifest.Server.Version
		if currentVersion != "" && update.CompareVersions(expectedVersion, currentVersion) <= 0 {
			return fmt.Errorf("%w: current=%s latest=%s", errAlreadyUpToDate, currentVersion, expectedVersion)
		}
	}
	if packagePath == "" {
		if candidate != nil {
			return errors.New("-candidate-manifest requires a local -package")
		}
		fmt.Fprintln(output, "[1/6] Downloading and verifying the server package...")
		packagePath, expectedVersion, err = downloadServerPackage(manifestURL, currentVersion, trustedPublicKeyPath)
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
	}
	packageInfo, err := os.Stat(packagePath)
	if err != nil || !packageInfo.Mode().IsRegular() || packageInfo.Size() <= 0 || packageInfo.Size() > maxServerPackageSize {
		return fmt.Errorf("server package is missing or outside the allowed size 1..%d: %s", maxServerPackageSize, packagePath)
	}
	tmpDir, err := os.MkdirTemp("", "bb-erp-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	privatePackagePath := filepath.Join(tmpDir, "verified-server-package.zip")
	if err := copyPath(packagePath, privatePackagePath); err != nil {
		return fmt.Errorf("copy server package into private staging: %w", err)
	}
	packagePath = privatePackagePath
	if !downloadedPackage {
		if err := verifyLocalServerPackage(packagePath, packageSignature, trustedPublicKeyPath); err != nil {
			return err
		}
	}
	if candidate != nil {
		if err := verifyCandidateServerPackage(packagePath, candidate.Manifest, trustedPublicKeyPath); err != nil {
			return err
		}
	}

	fmt.Fprintln(output, "[2/6] Extracting and validating the server package...")
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o700); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	if err := extractZip(packagePath, extractDir); err != nil {
		return err
	}
	if err := rejectEmbeddedManifest(extractDir); err != nil {
		return err
	}

	sourceDir, err := findServerPackageSourceDir(extractDir)
	if err != nil {
		return err
	}
	validatePackage := validateServerPackage
	if candidate != nil {
		validatePackage = validateServerPackageForCandidate
	}
	if err := validatePackage(sourceDir, expectedVersion); err != nil {
		return err
	}
	if err := requireMatchingPublicKeys(trustedPublicKeyPath, filepath.Join(sourceDir, "update-public.key")); err != nil {
		return fmt.Errorf("server package update-public.key is not the trusted key: %w", err)
	}
	if serviceName != "" {
		if err := validateWindowsServiceTarget(serviceName, installDir); err != nil {
			return err
		}
	}
	recoveryUpdaterPath, recoveryUpdaterSHA256, err := prepareRecoveryUpdater(installDir)
	if err != nil {
		return err
	}

	wasRunning, err := serverRunning(serviceName, installDir)
	if err != nil {
		return fmt.Errorf("inspect current server state: %w", err)
	}
	previousServerInstalled, err := existingServerExecutable(installDir)
	if err != nil {
		return err
	}
	backupRoot := filepath.Join(installDir, "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return fmt.Errorf("create backup root: %w", err)
	}
	backupDir, err := os.MkdirTemp(backupRoot, time.Now().Format("20060102-150405")+"-*")
	if err != nil {
		return fmt.Errorf("create unique backup directory: %w", err)
	}
	transaction := upgradeTransaction{
		Phase: transactionPreparing, BackupDir: backupDir, DatabasePath: databasePath,
		ServiceName: serviceName, WasRunning: wasRunning, PreviousVersion: currentVersion,
		HealthBaseURL: healthBaseURL, UpdaterPath: recoveryUpdaterPath, UpdaterSHA256: recoveryUpdaterSHA256,
	}
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		return err
	}
	fmt.Fprintln(output, "[3/6] Stopping the current server...")
	if err := stopServer(serviceName, installDir); err != nil {
		return err
	}
	fmt.Fprintln(output, "[4/6] Backing up the current deployment to", backupDir)
	// A first installation has no old executable to roll back to. Capture an
	// empty/minimal snapshot in that case, while still preserving a pre-existing
	// database if one was provisioned separately. Existing deployments require
	// the complete rollback set and database snapshot.
	if err := backupServerFilesWithDatabaseMode(installDir, backupDir, databasePath, previousServerInstalled, previousServerInstalled); err != nil {
		if wasRunning {
			if restartErr := startServer(serviceName, installDir); restartErr != nil {
				return fmt.Errorf("backup failed (%v); previous server restart failed: %w", err, restartErr)
			}
			return fmt.Errorf("backup failed and previous server was restarted: %w", err)
		}
		return err
	}
	transaction.Phase = transactionBackedUp
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
	}
	fmt.Fprintln(output, "[5/6] Installing the verified server files...")
	replaceFiles := replaceServerFiles
	if candidate != nil {
		replaceFiles = replaceServerFilesForCandidate
	}
	if err := replaceFiles(sourceDir, installDir); err != nil {
		return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
	}
	transaction.Phase = transactionInstalled
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
	}
	if candidate != nil {
		if err := activateCandidateManifest(candidate, installDir); err != nil {
			return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
		}
		transaction.Phase = transactionActivated
		if err := writeUpgradeTransaction(installDir, transaction); err != nil {
			return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
		}
	}
	fmt.Fprintln(output, "[6/6] Starting and verifying the updated server...")
	if err := startServer(serviceName, installDir); err != nil {
		return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
	}
	transaction.Phase = transactionStarted
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
	}
	if strings.TrimSpace(healthBaseURL) != "" {
		if err := waitForServerHealth(healthBaseURL, expectedVersion, currentVersion, output); err != nil {
			return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath, wasRunning, currentVersion, currentVersion, healthBaseURL, err)
		}
	}
	if err := os.Remove(upgradeTransactionPath(installDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove completed upgrade transaction: %w", err)
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
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var metadata updaterMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return updaterMetadata{}, fmt.Errorf("decode %s: %w", path, err)
	}
	metadata.ManifestURL = strings.TrimSpace(metadata.ManifestURL)
	metadata.ManifestFile = strings.TrimSpace(metadata.ManifestFile)
	metadata.ServerVersion = strings.TrimSpace(metadata.ServerVersion)
	metadata.Version = strings.TrimSpace(metadata.Version)
	return metadata, nil
}

func loadUpdaterMetadata(installDir string) (updaterMetadata, error) {
	metadata, err := readUpdaterMetadata(installDir)
	if err != nil {
		return updaterMetadata{}, err
	}
	if metadata.ManifestURL == "" && metadata.ManifestFile == "" {
		return updaterMetadata{}, fmt.Errorf("%s does not contain manifest_url or manifest_file", filepath.Join(installDir, "version.json"))
	}
	return metadata, nil
}

func validateServerPackage(sourceDir, expectedVersion string) error {
	for _, name := range []string{"update-manifest.json", "updates/stable/update-manifest.json"} {
		if _, err := os.Stat(filepath.Join(sourceDir, filepath.FromSlash(name))); err == nil {
			return fmt.Errorf("server package must not embed %s; pass it separately with -candidate-manifest", name)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect embedded %s: %w", name, err)
		}
	}
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

func validateServerPackageForCandidate(sourceDir, expectedVersion string) error {
	if err := validateServerPackage(sourceDir, expectedVersion); err != nil {
		return err
	}
	launcher := filepath.Join(sourceDir, "启动服务端.bat")
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("candidate server package must contain a non-empty 启动服务端.bat")
	}
	return nil
}

func rejectEmbeddedManifest(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(entry.Name(), "update-manifest.json") {
			return fmt.Errorf("server package must not embed update-manifest.json at %s; pass it separately with -candidate-manifest", strings.TrimPrefix(path, root+string(os.PathSeparator)))
		}
		return nil
	})
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
	running, err := serverProcessRunning(installDir)
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

const defaultDatabasePath = "data/erp.db"

func resolveDatabasePath(installDir, configured string) (string, error) {
	installDir, err := filepath.Abs(filepath.Clean(installDir))
	if err != nil {
		return "", fmt.Errorf("resolve install directory for database: %w", err)
	}
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = defaultDatabasePath
	}
	if !filepath.IsAbs(configured) {
		configured = filepath.Join(installDir, configured)
	}
	databasePath, err := filepath.Abs(filepath.Clean(configured))
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	if filepath.Base(databasePath) == "." || filepath.Base(databasePath) == string(filepath.Separator) {
		return "", fmt.Errorf("database path must name a file: %q", configured)
	}
	return databasePath, nil
}

func existingServerExecutable(installDir string) (bool, error) {
	path := filepath.Join(installDir, "bb-erp-server.exe")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect installed server executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return false, fmt.Errorf("installed server executable is not a non-empty regular file: %s", path)
	}
	return true, nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func resolveTrustedPublicKeyPath(installDir string, configured ...string) (string, error) {
	installedPath := filepath.Join(installDir, "update-public.key")
	externalPath := ""
	if len(configured) > 0 {
		externalPath = strings.TrimSpace(configured[0])
	}
	if externalPath != "" {
		if !filepath.IsAbs(externalPath) {
			externalPath = filepath.Join(installDir, externalPath)
		}
		var err error
		externalPath, err = filepath.Abs(filepath.Clean(externalPath))
		if err != nil {
			return "", fmt.Errorf("resolve external trusted public key: %w", err)
		}
		if !regularFileExists(externalPath) {
			return "", fmt.Errorf("external trusted public key is missing or not a regular file: %s", externalPath)
		}
	}
	if regularFileExists(installedPath) {
		if externalPath != "" {
			if err := requireMatchingPublicKeys(installedPath, externalPath); err != nil {
				return "", err
			}
		}
		return installedPath, nil
	}
	if externalPath == "" {
		return "", errors.New("installed update-public.key is unavailable; first installation requires -trusted-public-key")
	}
	return externalPath, nil
}

func requireMatchingPublicKeys(firstPath, secondPath string) error {
	first, err := os.ReadFile(firstPath)
	if err != nil {
		return fmt.Errorf("read trusted public key %s: %w", firstPath, err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		return fmt.Errorf("read external trusted public key %s: %w", secondPath, err)
	}
	firstCanonical, err := update.CanonicalMinisignPublicKey(string(first))
	if err != nil {
		return fmt.Errorf("parse trusted public key %s: %w", firstPath, err)
	}
	secondCanonical, err := update.CanonicalMinisignPublicKey(string(second))
	if err != nil {
		return fmt.Errorf("parse external trusted public key %s: %w", secondPath, err)
	}
	if firstCanonical != secondCanonical {
		return fmt.Errorf("external trusted public key %s does not match installed update-public.key", secondPath)
	}
	return nil
}

func findServerPackageSourceDir(root string) (string, error) {
	for _, candidate := range []string{filepath.Join(root, "server"), root} {
		if regularFileExists(filepath.Join(candidate, "bb-erp-server.exe")) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("server package does not contain bb-erp-server.exe: %s", root)
}

func backupServerFiles(installDir string, backupDir string) error {
	databasePath, err := resolveDatabasePath(installDir, "")
	if err != nil {
		return err
	}
	return backupServerFilesWithDatabase(installDir, backupDir, databasePath)
}

// backupServerFilesWithDatabase captures the minimum rollback set after the
// server has stopped. It intentionally excludes logs, uploads and the large
// updates/{artifacts,releases,pending} tree; only the stable manifest and the
// SQLite database (including WAL/SHM sidecars) participate in the transaction.
func backupServerFilesWithDatabase(installDir, backupDir, databasePath string) error {
	return backupServerFilesWithDatabaseMode(installDir, backupDir, databasePath, true, true)
}

// backupServerFilesWithDatabaseMode permits a first-install snapshot. When
// requireServer is false, absent old runtime files are expected and are not
// copied; an already provisioned database is still copied when present. Once
// an installed server exists, both the executable and database are mandatory
// so a later rollback cannot silently become a partial restore.
func backupServerFilesWithDatabaseMode(installDir, backupDir, databasePath string, requireServer, requireDatabase bool) error {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	databasePath, err := resolveDatabasePath(installDir, databasePath)
	if err != nil {
		return err
	}
	for _, name := range []string{
		"bb-erp-server.exe", "启动服务端.bat", "web", "update-public.key", "version.json",
		"updates/stable/update-manifest.json",
	} {
		source := filepath.Join(installDir, filepath.FromSlash(name))
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) && (!requireServer || name != "bb-erp-server.exe") {
				continue
			}
			return fmt.Errorf("inspect %s for backup: %w", name, err)
		}
		if err := copyPath(source, filepath.Join(backupDir, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("backup %s: %w", name, err)
		}
	}
	databaseBackupRoot, err := databaseBackupRoot(installDir, backupDir, databasePath)
	if err != nil {
		return err
	}
	if err := backupDatabaseFilesMode(databasePath, databaseBackupRoot, requireDatabase); err != nil {
		return err
	}
	return nil
}

func databaseBackupRoot(installDir, backupDir, databasePath string) (string, error) {
	installDir, err := filepath.Abs(filepath.Clean(installDir))
	if err != nil {
		return "", err
	}
	databasePath, err = filepath.Abs(filepath.Clean(databasePath))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(installDir, databasePath)
	if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
		return filepath.Join(backupDir, relative), nil
	}
	base := filepath.Base(databasePath)
	if base == "." || base == "" || base == string(filepath.Separator) {
		return "", fmt.Errorf("database path must name a file: %q", databasePath)
	}
	return filepath.Join(backupDir, "database", base), nil
}

func backupDatabaseFiles(databasePath, backupPath string) error {
	return backupDatabaseFilesMode(databasePath, backupPath, true)
}

func backupDatabaseFilesMode(databasePath, backupPath string, required bool) error {
	mainInfo, err := os.Stat(databasePath)
	if err != nil {
		if !required && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect SQLite database %s: %w", databasePath, err)
	}
	if mainInfo.IsDir() || !mainInfo.Mode().IsRegular() {
		return fmt.Errorf("SQLite database path is not a regular file: %s", databasePath)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := databasePath + suffix
		info, statErr := os.Stat(source)
		if statErr != nil {
			if suffix != "" && os.IsNotExist(statErr) {
				continue
			}
			return fmt.Errorf("inspect SQLite file %s: %w", source, statErr)
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return fmt.Errorf("SQLite file is not a regular file: %s", source)
		}
		if err := copyPath(source, backupPath+suffix); err != nil {
			return fmt.Errorf("backup SQLite file %s: %w", source, err)
		}
	}
	return nil
}

func replaceServerFiles(sourceDir string, installDir string) error {
	return replaceServerFilesInternal(sourceDir, installDir, false)
}

// replaceServerFilesForCandidate installs the packaged launcher along with
// runtime files. Candidate releases must carry 启动服务端.bat so ordinary
// process deployments retain the production environment (manifest path,
// update settings and HTTP bind configuration) after an upgrade. Windows
// Service deployments validate the file but start the service directly.
func replaceServerFilesForCandidate(sourceDir, installDir string) error {
	return replaceServerFilesInternal(sourceDir, installDir, true)
}

func replaceServerFilesInternal(sourceDir string, installDir string, replaceLauncher bool) error {
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
	if replaceLauncher {
		if err := replaceFileSafely(filepath.Join(sourceDir, "启动服务端.bat"), filepath.Join(installDir, "启动服务端.bat")); err != nil {
			return fmt.Errorf("replace server launcher: %w", err)
		}
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
	return recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, "", restartPrevious, "", "", "", cause)
}

func recoverFailedUpgradeWithHealth(serviceName, installDir, backupDir, databasePath string, restartPrevious bool, previousVersion, currentVersion, healthBaseURL string, cause error) error {
	if stopErr := stopServer(serviceName, installDir); stopErr != nil {
		return fmt.Errorf("upgrade failed (%v); stop failed before rollback: %w", cause, stopErr)
	}
	restoreErr := restoreServerFilesWithDatabase(backupDir, installDir, databasePath)
	if restoreErr != nil {
		return fmt.Errorf("upgrade failed (%v); restore backup failed: %w", cause, restoreErr)
	}
	if restartPrevious {
		if restartErr := startServer(serviceName, installDir); restartErr != nil {
			return fmt.Errorf("upgrade failed (%v); backup restored but previous server restart failed: %w", cause, restartErr)
		}
		if strings.TrimSpace(healthBaseURL) != "" {
			if healthErr := waitForServerHealth(healthBaseURL, previousVersion, currentVersion, nil); healthErr != nil {
				return fmt.Errorf("upgrade failed (%v); previous version was restored but health verification failed: %w", cause, healthErr)
			}
		}
		if err := os.Remove(upgradeTransactionPath(installDir)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("upgrade failed (%v); previous version was restored but transaction journal cleanup failed: %w", cause, err)
		}
		return fmt.Errorf("upgrade failed and previous version was restored and restarted: %w", cause)
	}
	if err := os.Remove(upgradeTransactionPath(installDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("upgrade failed (%v); previous stopped version was restored but transaction journal cleanup failed: %w", cause, err)
	}
	return fmt.Errorf("upgrade failed and previous stopped version was restored: %w", cause)
}

func restoreServerFiles(backupDir, installDir string) error {
	databasePath, err := resolveDatabasePath(installDir, "")
	if err != nil {
		return err
	}
	return restoreServerFilesWithDatabase(backupDir, installDir, databasePath)
}

func restoreServerFilesWithDatabase(backupDir, installDir, databasePath string) error {
	for _, name := range []string{
		"bb-erp-server.exe",
		"bb-erp-updater.pending.exe", "bb-erp-upgrade-runner.pending.bat",
		"update-public.key", "version.json", "启动服务端.bat", "web",
		"updates/stable/update-manifest.json",
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
	databasePath, err := resolveDatabasePath(installDir, databasePath)
	if err != nil {
		return err
	}
	databaseRoot, err := databaseBackupRoot(installDir, backupDir, databasePath)
	if err != nil {
		return err
	}
	// A snapshot without the old server executable is the explicit first-
	// install snapshot. In that case an absent database backup means the new
	// database must be removed on rollback; older snapshots from an existing
	// deployment may legitimately predate database snapshots and are preserved.
	_, serverBackupErr := os.Stat(filepath.Join(backupDir, "bb-erp-server.exe"))
	removeMissingDatabase := os.IsNotExist(serverBackupErr)
	if err := restoreDatabaseFiles(databaseRoot, databasePath, removeMissingDatabase); err != nil {
		return err
	}
	return nil
}

func restoreDatabaseFiles(backupPath, databasePath string, removeMissing ...bool) error {
	mainBackup := backupPath
	if _, err := os.Stat(mainBackup); os.IsNotExist(err) {
		if len(removeMissing) > 0 && removeMissing[0] {
			for _, suffix := range []string{"", "-wal", "-shm"} {
				if err := os.Remove(databasePath + suffix); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove new SQLite file %s: %w", databasePath+suffix, err)
				}
			}
			return nil
		}
		// Older rollback snapshots did not include an explicit database copy.
		// Keep their behavior for manual recovery, while every new transaction
		// always has the main database and therefore takes this path.
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect backed up SQLite database %s: %w", mainBackup, err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := mainBackup + suffix
		target := databasePath + suffix
		info, statErr := os.Stat(source)
		if statErr != nil && os.IsNotExist(statErr) {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove current SQLite sidecar %s: %w", target, err)
			}
			continue
		} else if statErr != nil {
			return fmt.Errorf("inspect backed up SQLite file %s: %w", source, statErr)
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return fmt.Errorf("backed up SQLite file is not a regular file: %s", source)
		}
		if err := replaceFileSafely(source, target); err != nil {
			return fmt.Errorf("restore SQLite file %s: %w", target, err)
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
		// ZIP names always use slash separators, even when the updater runs on
		// Windows. Normalize before filepath.Join so a crafted `..\\` entry
		// cannot evade the traversal check on Unix tests or on the production
		// Windows host. The same check also rejects absolute and NUL-containing
		// names before any destination path is created.
		archiveName := strings.ReplaceAll(item.Name, "\\", "/")
		cleanName := pathpkg.Clean(archiveName)
		if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || pathpkg.IsAbs(cleanName) || strings.IndexByte(cleanName, 0) >= 0 {
			return fmt.Errorf("unsafe zip path: %s", item.Name)
		}
		if item.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip contains symbolic link: %s", item.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(cleanName))
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
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	hash := sha256.New()
	if err := writeFile(target, io.TeeReader(input, hash), info.Mode()); err != nil {
		return err
	}
	if err := verifyFileSize(target, info.Size()); err != nil {
		return fmt.Errorf("verify durable backup size: %w", err)
	}
	if err := verifySHA256(target, hex.EncodeToString(hash.Sum(nil))); err != nil {
		return fmt.Errorf("verify durable backup content: %w", err)
	}
	return nil
}

func writeFile(target string, reader io.Reader, mode os.FileMode) error {
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, reader); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
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
	if copyErr != nil {
		_ = file.Close()
		return fmt.Errorf("write package file: %w", copyErr)
	}
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close()
		return fmt.Errorf("sync package file: %w", syncErr)
	}
	closeErr := file.Close()
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
