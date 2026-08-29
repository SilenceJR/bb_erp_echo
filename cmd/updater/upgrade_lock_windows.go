//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func acquireUpgradeLock(installDir string) (func(), error) {
	absolute, err := filepath.Abs(installDir)
	if err != nil {
		return nil, fmt.Errorf("resolve updater lock directory: %w", err)
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute))))
	// Global namespace keeps different interactive/RDP sessions from upgrading
	// the same installation concurrently.
	name, err := windows.UTF16PtrFromString(`Global\BBERPServerUpdater-` + hex.EncodeToString(digest[:16]))
	if err != nil {
		return nil, fmt.Errorf("create updater mutex name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, fmt.Errorf("create updater mutex: %w", err)
	}
	result, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("acquire updater mutex: %w", err)
	}
	if result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(handle)
		return nil, errorsNewUpgradeInProgress()
	}
	return func() {
		_ = windows.ReleaseMutex(handle)
		_ = windows.CloseHandle(handle)
	}, nil
}

func errorsNewUpgradeInProgress() error {
	return fmt.Errorf("another server updater is already running for this install directory")
}
