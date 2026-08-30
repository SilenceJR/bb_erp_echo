package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeTransactionFixture(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestRecoverInterruptedPreparingRemovesJournalAndIncompleteBackup(t *testing.T) {
	installDir := t.TempDir()
	backupDir := filepath.Join(installDir, "backups", "preparing")
	databasePath := filepath.Join(installDir, "data", "erp.db")
	updaterPath := filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe")
	writeTransactionFixture(t, filepath.Join(backupDir, "partial"), "partial")
	writeTransactionFixture(t, updaterPath, "updater")
	updaterSHA256, err := sha256File(updaterPath)
	if err != nil {
		t.Fatalf("hash updater: %v", err)
	}
	if err := writeUpgradeTransaction(installDir, upgradeTransaction{
		Phase: transactionPreparing, BackupDir: backupDir, DatabasePath: databasePath,
		UpdaterPath: updaterPath, UpdaterSHA256: updaterSHA256,
	}); err != nil {
		t.Fatalf("write transaction: %v", err)
	}
	if err := recoverInterruptedUpgrade(installDir, "", databasePath, io.Discard); err != nil {
		t.Fatalf("recover preparing transaction: %v", err)
	}
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("incomplete backup still exists: %v", err)
	}
	if _, err := os.Stat(upgradeTransactionPath(installDir)); !os.IsNotExist(err) {
		t.Fatalf("transaction journal still exists: %v", err)
	}
}

func TestRecoverInterruptedInstalledRestoresRollbackSnapshot(t *testing.T) {
	installDir := t.TempDir()
	backupDir := filepath.Join(installDir, "backups", "installed")
	databasePath := filepath.Join(installDir, "data", "erp.db")
	updaterPath := filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe")
	writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-server.exe"), "old-server")
	writeTransactionFixture(t, filepath.Join(installDir, "version.json"), `{"version":"1.0.0"}`)
	writeTransactionFixture(t, filepath.Join(installDir, "updates", "stable", "update-manifest.json"), `{"version":"1.0.0"}`)
	writeTransactionFixture(t, databasePath, "old-database")
	writeTransactionFixture(t, updaterPath, "updater")
	updaterSHA256, err := sha256File(updaterPath)
	if err != nil {
		t.Fatalf("hash updater: %v", err)
	}
	if err := backupServerFilesWithDatabase(installDir, backupDir, databasePath); err != nil {
		t.Fatalf("create rollback snapshot: %v", err)
	}
	writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-server.exe"), "new-server")
	writeTransactionFixture(t, filepath.Join(installDir, "version.json"), `{"version":"1.0.1"}`)
	writeTransactionFixture(t, filepath.Join(installDir, "updates", "stable", "update-manifest.json"), `{"version":"1.0.1"}`)
	writeTransactionFixture(t, databasePath, "new-database")
	transaction := upgradeTransaction{
		Phase: transactionPreparing, BackupDir: backupDir, DatabasePath: databasePath,
		PreviousVersion: "1.0.0", UpdaterPath: updaterPath, UpdaterSHA256: updaterSHA256,
	}
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		t.Fatalf("write preparing transaction: %v", err)
	}
	transaction.Phase = transactionInstalled
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		t.Fatalf("write transaction: %v", err)
	}
	previousStop := recoveryStopServer
	recoveryStopServer = func(string, string) error { return nil }
	t.Cleanup(func() { recoveryStopServer = previousStop })
	if err := recoverInterruptedUpgrade(installDir, "", databasePath, io.Discard); err != nil {
		t.Fatalf("recover installed transaction: %v", err)
	}
	for path, want := range map[string]string{
		filepath.Join(installDir, "bb-erp-server.exe"): "old-server",
		filepath.Join(installDir, "version.json"):      `{"version":"1.0.0"}`,
		databasePath: "old-database",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("restored %s = %q, err=%v; want %q", path, got, err, want)
		}
	}
}

func TestValidateUpgradeTransactionRejectsBackupOutsideManagedRoot(t *testing.T) {
	installDir := t.TempDir()
	databasePath := filepath.Join(installDir, "data", "erp.db")
	updaterPath := filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe")
	writeTransactionFixture(t, updaterPath, "updater")
	updaterSHA256, err := sha256File(updaterPath)
	if err != nil {
		t.Fatalf("hash updater: %v", err)
	}
	transaction := &upgradeTransaction{
		SchemaVersion: upgradeTransactionSchema, Phase: transactionBackedUp,
		BackupDir: t.TempDir(), DatabasePath: databasePath,
		UpdaterPath: updaterPath, UpdaterSHA256: updaterSHA256,
	}
	if err := validateUpgradeTransaction(installDir, "", databasePath, transaction); err == nil {
		t.Fatal("backup directory outside installDir/backups must be rejected")
	}
}

func TestReadUpgradeTransactionKeepsLastDurablePhaseAfterPartialAppend(t *testing.T) {
	installDir := t.TempDir()
	transaction := upgradeTransaction{Phase: transactionPreparing, BackupDir: "backup", DatabasePath: "database"}
	if err := writeUpgradeTransaction(installDir, transaction); err != nil {
		t.Fatalf("write preparing phase: %v", err)
	}
	journal, err := os.OpenFile(upgradeTransactionPath(installDir), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("open journal for interrupted append: %v", err)
	}
	if _, err := journal.WriteString(`{"schema_version":1,"phase":"installed"`); err != nil {
		_ = journal.Close()
		t.Fatalf("append partial phase: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("close partial journal: %v", err)
	}
	got, err := readUpgradeTransaction(installDir)
	if err != nil {
		t.Fatalf("read journal with interrupted final append: %v", err)
	}
	if got.Phase != transactionPreparing {
		t.Fatalf("phase = %q, want last durable phase %q", got.Phase, transactionPreparing)
	}
}

func TestRecoverInterruptedPhasesRestoresDatabaseSidecarsAndIsIdempotent(t *testing.T) {
	for _, phase := range []string{transactionBackedUp, transactionInstalled, transactionActivated, transactionStarted} {
		t.Run(phase, func(t *testing.T) {
			installDir := t.TempDir()
			backupDir := filepath.Join(installDir, "backups", phase)
			databasePath := filepath.Join(installDir, "data", "erp.db")
			updaterPath := filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe")
			writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-server.exe"), "old-server")
			writeTransactionFixture(t, databasePath, "old-db")
			writeTransactionFixture(t, databasePath+"-wal", "old-wal")
			writeTransactionFixture(t, databasePath+"-shm", "old-shm")
			writeTransactionFixture(t, updaterPath, "updater")
			if err := backupServerFilesWithDatabase(installDir, backupDir, databasePath); err != nil {
				t.Fatalf("backup: %v", err)
			}
			writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-server.exe"), "new-server")
			writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-updater.pending.exe"), "new-updater")
			writeTransactionFixture(t, databasePath, "new-db")
			writeTransactionFixture(t, databasePath+"-wal", "new-wal")
			writeTransactionFixture(t, databasePath+"-shm", "new-shm")
			updaterSHA256, err := sha256File(updaterPath)
			if err != nil {
				t.Fatalf("hash updater: %v", err)
			}
			transaction := upgradeTransaction{
				Phase: transactionPreparing, BackupDir: backupDir, DatabasePath: databasePath,
				PreviousVersion: "1.0.0", UpdaterPath: updaterPath, UpdaterSHA256: updaterSHA256,
			}
			if err := writeUpgradeTransaction(installDir, transaction); err != nil {
				t.Fatalf("write preparing phase: %v", err)
			}
			transaction.Phase = phase
			if err := writeUpgradeTransaction(installDir, transaction); err != nil {
				t.Fatalf("write %s phase: %v", phase, err)
			}
			stopCalls := 0
			previousStop := recoveryStopServer
			recoveryStopServer = func(string, string) error { stopCalls++; return nil }
			t.Cleanup(func() { recoveryStopServer = previousStop })
			if err := recoverInterruptedUpgrade(installDir, "", databasePath, io.Discard); err != nil {
				t.Fatalf("recover %s: %v", phase, err)
			}
			for path, want := range map[string]string{
				filepath.Join(installDir, "bb-erp-server.exe"): "old-server",
				databasePath: "old-db", databasePath + "-wal": "old-wal", databasePath + "-shm": "old-shm",
			} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("restored %s = %q, err=%v; want %q", path, got, err, want)
				}
			}
			if _, err := os.Stat(filepath.Join(installDir, "bb-erp-updater.pending.exe")); !os.IsNotExist(err) {
				t.Fatalf("new-only pending updater remains: %v", err)
			}
			if err := recoverInterruptedUpgrade(installDir, "", databasePath, io.Discard); err != nil {
				t.Fatalf("second recovery must be idempotent: %v", err)
			}
			if stopCalls != 1 {
				t.Fatalf("stop calls = %d, want 1 across both recovery calls", stopCalls)
			}
		})
	}
}

func TestRecoverInterruptedFirstInstallRestoresOrRemovesProvisionedDatabase(t *testing.T) {
	for _, provisioned := range []bool{false, true} {
		name := "without_database"
		if provisioned {
			name = "with_database"
		}
		t.Run(name, func(t *testing.T) {
			installDir := t.TempDir()
			backupDir := filepath.Join(installDir, "backups", name)
			databasePath := filepath.Join(installDir, "data", "erp.db")
			updaterPath := filepath.Join(installDir, "updates", "recovery", "bb-erp-updater.exe")
			writeTransactionFixture(t, updaterPath, "updater")
			if provisioned {
				writeTransactionFixture(t, databasePath, "provisioned-db")
			}
			if err := backupServerFilesWithDatabaseMode(installDir, backupDir, databasePath, false, false); err != nil {
				t.Fatalf("first-install backup: %v", err)
			}
			writeTransactionFixture(t, filepath.Join(installDir, "bb-erp-server.exe"), "new-server")
			writeTransactionFixture(t, databasePath, "new-db")
			updaterSHA256, err := sha256File(updaterPath)
			if err != nil {
				t.Fatalf("hash updater: %v", err)
			}
			transaction := upgradeTransaction{
				Phase: transactionPreparing, BackupDir: backupDir, DatabasePath: databasePath,
				UpdaterPath: updaterPath, UpdaterSHA256: updaterSHA256,
			}
			if err := writeUpgradeTransaction(installDir, transaction); err != nil {
				t.Fatalf("write preparing: %v", err)
			}
			transaction.Phase = transactionBackedUp
			if err := writeUpgradeTransaction(installDir, transaction); err != nil {
				t.Fatalf("write backed-up: %v", err)
			}
			previousStop := recoveryStopServer
			recoveryStopServer = func(string, string) error { return nil }
			t.Cleanup(func() { recoveryStopServer = previousStop })
			if err := recoverInterruptedUpgrade(installDir, "", databasePath, io.Discard); err != nil {
				t.Fatalf("recover first install: %v", err)
			}
			if _, err := os.Stat(filepath.Join(installDir, "bb-erp-server.exe")); !os.IsNotExist(err) {
				t.Fatalf("new server remains: %v", err)
			}
			got, err := os.ReadFile(databasePath)
			if provisioned {
				if err != nil || string(got) != "provisioned-db" {
					t.Fatalf("database = %q, err=%v", got, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("new database was not removed: %v", err)
			}
		})
	}
}
