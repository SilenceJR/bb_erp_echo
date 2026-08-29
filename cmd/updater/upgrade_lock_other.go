//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func acquireUpgradeLock(installDir string) (func(), error) {
	path := filepath.Join(installDir, ".bb-erp-updater.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another server updater is already running for this install directory")
		}
		return nil, fmt.Errorf("create updater lock: %w", err)
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return func() {
		_ = file.Close()
		_ = os.Remove(path)
	}, nil
}
