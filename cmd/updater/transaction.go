package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	upgradeTransactionSchema = 1
	upgradeTransactionName   = "server-upgrade-transaction.json"
	maxTransactionSize       = int64(64 << 10)

	transactionPreparing = "preparing"
	transactionBackedUp  = "backed_up"
	transactionInstalled = "installed"
	transactionActivated = "activated"
	transactionStarted   = "started"
)

type upgradeTransaction struct {
	SchemaVersion   int    `json:"schema_version"`
	Phase           string `json:"phase"`
	BackupDir       string `json:"backup_dir"`
	DatabasePath    string `json:"database_path"`
	ServiceName     string `json:"service_name,omitempty"`
	WasRunning      bool   `json:"was_running"`
	PreviousVersion string `json:"previous_version"`
	HealthBaseURL   string `json:"health_base_url,omitempty"`
	UpdaterPath     string `json:"updater_path"`
	UpdaterSHA256   string `json:"updater_sha256"`
}

var (
	recoveryServerRunning = serverRunning
	recoveryStopServer    = stopServer
	recoveryStartServer   = startServer
	recoveryWaitForHealth = waitForServerHealth
)

func upgradeTransactionPath(installDir string) string {
	return filepath.Join(installDir, "updates", "pending", upgradeTransactionName)
}

func prepareRecoveryUpdater(installDir string) (string, string, error) {
	source, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve running updater executable: %w", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve running updater path: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe"))
	if err != nil {
		return "", "", fmt.Errorf("resolve recovery updater path: %w", err)
	}
	if !strings.EqualFold(source, target) {
		if err := replaceFileSafely(source, target); err != nil {
			return "", "", fmt.Errorf("prepare persistent recovery updater: %w", err)
		}
	}
	digest, err := sha256File(target)
	if err != nil {
		return "", "", fmt.Errorf("hash recovery updater: %w", err)
	}
	return target, digest, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeUpgradeTransaction(installDir string, transaction upgradeTransaction) error {
	transaction.SchemaVersion = upgradeTransactionSchema
	path := upgradeTransactionPath(installDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create upgrade transaction directory: %w", err)
	}
	encoded, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Errorf("encode upgrade transaction: %w", err)
	}
	encoded = append(encoded, '\n')
	flags := os.O_WRONLY | os.O_APPEND
	if transaction.Phase == transactionPreparing {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open upgrade transaction journal: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("append upgrade transaction journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync upgrade transaction journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close upgrade transaction journal: %w", err)
	}
	return nil
}

func readUpgradeTransaction(installDir string) (*upgradeTransaction, error) {
	path := upgradeTransactionPath(installDir)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open upgrade transaction: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat upgrade transaction: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxTransactionSize {
		return nil, fmt.Errorf("upgrade transaction size %d is invalid", info.Size())
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTransactionSize+1))
	if err != nil {
		return nil, fmt.Errorf("read upgrade transaction: %w", err)
	}
	lines := bytes.Split(data, []byte{'\n'})
	var latest *upgradeTransaction
	for index, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var transaction upgradeTransaction
		decodeErr := decoder.Decode(&transaction)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		valid := decodeErr == nil && errors.Is(trailingErr, io.EOF)
		isFinalPartialLine := index == len(lines)-1 && len(data) > 0 && data[len(data)-1] != '\n'
		if !valid {
			if isFinalPartialLine {
				break
			}
			return nil, fmt.Errorf("decode upgrade transaction journal record %d", index+1)
		}
		copy := transaction
		latest = &copy
	}
	if latest == nil {
		return nil, errors.New("upgrade transaction journal has no complete record")
	}
	return latest, nil
}

func validateUpgradeTransaction(installDir, serviceName, databasePath string, transaction *upgradeTransaction) error {
	if transaction == nil || transaction.SchemaVersion != upgradeTransactionSchema {
		return errors.New("upgrade transaction schema is invalid")
	}
	if !matchesTransactionPhase(transaction.Phase) {
		return fmt.Errorf("upgrade transaction phase %q is invalid", transaction.Phase)
	}
	if strings.TrimSpace(transaction.ServiceName) != strings.TrimSpace(serviceName) {
		return fmt.Errorf("interrupted upgrade service %q does not match requested service %q", transaction.ServiceName, serviceName)
	}
	resolvedDatabase, err := resolveDatabasePath(installDir, transaction.DatabasePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(resolvedDatabase), filepath.Clean(databasePath)) {
		return fmt.Errorf("interrupted upgrade database %q does not match requested database %q", resolvedDatabase, databasePath)
	}
	backupRoot, err := filepath.Abs(filepath.Join(installDir, "backups"))
	if err != nil {
		return err
	}
	backupDir, err := filepath.Abs(filepath.Clean(transaction.BackupDir))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(backupRoot, backupDir)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Dir(relative) != "." {
		return fmt.Errorf("interrupted upgrade backup directory is outside the managed backup root: %q", backupDir)
	}
	transaction.BackupDir = backupDir
	expectedUpdaterPath, err := filepath.Abs(filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe"))
	if err != nil {
		return err
	}
	updaterPath, err := filepath.Abs(filepath.Clean(transaction.UpdaterPath))
	if err != nil || !strings.EqualFold(updaterPath, expectedUpdaterPath) || !regularFileExists(updaterPath) {
		return fmt.Errorf("interrupted upgrade updater is missing: %q", transaction.UpdaterPath)
	}
	if !validSHA256(transaction.UpdaterSHA256) {
		return errors.New("interrupted upgrade updater SHA-256 is invalid")
	}
	if err := verifySHA256(updaterPath, transaction.UpdaterSHA256); err != nil {
		return fmt.Errorf("interrupted upgrade updater integrity check failed: %w", err)
	}
	transaction.UpdaterPath = updaterPath
	return nil
}

func matchesTransactionPhase(phase string) bool {
	switch phase {
	case transactionPreparing, transactionBackedUp, transactionInstalled, transactionActivated, transactionStarted:
		return true
	default:
		return false
	}
}

func recoverInterruptedUpgrade(installDir, serviceName, databasePath string, output io.Writer) error {
	transaction, err := readUpgradeTransaction(installDir)
	if err != nil || transaction == nil {
		return err
	}
	if err := validateUpgradeTransaction(installDir, serviceName, databasePath, transaction); err != nil {
		return err
	}
	if strings.TrimSpace(serviceName) != "" {
		if err := validateWindowsServiceTarget(serviceName, installDir); err != nil {
			return fmt.Errorf("validate Windows service before interrupted upgrade recovery: %w", err)
		}
	}
	if output != nil {
		fmt.Fprintln(output, "recovering interrupted server upgrade from phase", transaction.Phase)
	}
	if transaction.Phase == transactionPreparing {
		if transaction.WasRunning {
			running, err := recoveryServerRunning(serviceName, installDir)
			if err != nil {
				return fmt.Errorf("inspect server during interrupted preparation recovery: %w", err)
			}
			if !running {
				if err := recoveryStartServer(serviceName, installDir); err != nil {
					return fmt.Errorf("restart server after interrupted preparation: %w", err)
				}
			}
			if strings.TrimSpace(transaction.HealthBaseURL) != "" {
				if err := recoveryWaitForHealth(transaction.HealthBaseURL, transaction.PreviousVersion, transaction.PreviousVersion, output); err != nil {
					return fmt.Errorf("verify server after interrupted preparation: %w", err)
				}
			}
		}
		_ = os.RemoveAll(transaction.BackupDir)
	} else {
		if err := recoveryStopServer(serviceName, installDir); err != nil {
			return fmt.Errorf("stop server before interrupted upgrade recovery: %w", err)
		}
		if err := restoreServerFilesWithDatabase(transaction.BackupDir, installDir, databasePath); err != nil {
			return fmt.Errorf("restore interrupted upgrade: %w", err)
		}
		if transaction.WasRunning {
			if err := recoveryStartServer(serviceName, installDir); err != nil {
				return fmt.Errorf("restart restored server: %w", err)
			}
			if strings.TrimSpace(transaction.HealthBaseURL) != "" {
				if err := recoveryWaitForHealth(transaction.HealthBaseURL, transaction.PreviousVersion, transaction.PreviousVersion, output); err != nil {
					return fmt.Errorf("verify restored server after interrupted upgrade: %w", err)
				}
			}
		}
	}
	if err := os.Remove(upgradeTransactionPath(installDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove completed upgrade transaction: %w", err)
	}
	return nil
}
