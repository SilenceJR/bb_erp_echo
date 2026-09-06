package servertray

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WriteBootstrapError(logDir string, err error) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = filepath.Join(executableDir(), "logs")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create bootstrap log directory: %w", err)
	}
	file, openErr := os.OpenFile(filepath.Join(logDir, "bootstrap-error.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if openErr != nil {
		return fmt.Errorf("open bootstrap error log: %w", openErr)
	}
	_, writeErr := fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339), strings.ReplaceAll(err.Error(), "\n", " "))
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write bootstrap error log: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close bootstrap error log: %w", closeErr)
	}
	return nil
}
