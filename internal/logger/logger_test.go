package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb_erp_echo/internal/config"
)

// TestNewCreatesLogFiles 验证日志目录不存在时会自动创建，并且三类日志写入对应文件。
func TestNewCreatesLogFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	system, err := New(config.LogConfig{
		Level:         "debug",
		Dir:           dir,
		Console:       false,
		RetentionDays: 30,
	})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	t.Cleanup(func() {
		_ = system.Close()
	})

	system.App.Info("app message")
	system.Access.Info("access message")
	system.Error.Error("error message")

	date := time.Now().Format(time.DateOnly)
	assertLogContains(t, filepath.Join(dir, "app-"+date+".log"), "app message")
	assertLogContains(t, filepath.Join(dir, "access-"+date+".log"), "access message")
	assertLogContains(t, filepath.Join(dir, "error-"+date+".log"), "error message")
}

// TestConsoleOutputDoesNotBlockFileWrite 验证开启控制台输出时，文件写入仍然正常。
func TestConsoleOutputDoesNotBlockFileWrite(t *testing.T) {
	dir := t.TempDir()
	system, err := New(config.LogConfig{
		Level:         "info",
		Dir:           dir,
		Console:       true,
		RetentionDays: 30,
	})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	t.Cleanup(func() {
		_ = system.Close()
	})

	system.App.Info("console and file")

	date := time.Now().Format(time.DateOnly)
	assertLogContains(t, filepath.Join(dir, "app-"+date+".log"), "console and file")
}

// TestDailyFileWriterRotatesByDate 验证同一个 writer 会按日期写入不同文件。
func TestDailyFileWriterRotatesByDate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.Local)
	writer := NewDailyFileWriter(dir, "app", func() time.Time {
		return now
	})
	t.Cleanup(func() {
		_ = writer.Close()
	})

	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	now = now.AddDate(0, 0, 1)
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("write second: %v", err)
	}

	assertLogContains(t, filepath.Join(dir, "app-2026-07-27.log"), "first")
	assertLogContains(t, filepath.Join(dir, "app-2026-07-28.log"), "second")
}

// TestCleanupOnlyRemovesExpiredKnownLogFiles 验证清理逻辑只删除过期且符合命名规则的日志。
func TestCleanupOnlyRemovesExpiredKnownLogFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"app-2026-06-01.log":    "delete",
		"access-2026-07-10.log": "keep",
		"error-2026-06-01.log":  "delete",
		"audit-2026-06-01.log":  "keep unknown kind",
		"app-not-a-date.log":    "keep invalid date",
		"random-2026-06-01.txt": "keep invalid suffix",
		"app-2026-07-01.log":    "keep boundary",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.Local)
	if err := Cleanup(dir, 30, now); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	assertMissing(t, filepath.Join(dir, "app-2026-06-01.log"))
	assertMissing(t, filepath.Join(dir, "error-2026-06-01.log"))
	assertExists(t, filepath.Join(dir, "access-2026-07-10.log"))
	assertExists(t, filepath.Join(dir, "audit-2026-06-01.log"))
	assertExists(t, filepath.Join(dir, "app-not-a-date.log"))
	assertExists(t, filepath.Join(dir, "random-2026-06-01.txt"))
	assertExists(t, filepath.Join(dir, "app-2026-07-01.log"))
}

func assertLogContains(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s does not contain %q: %s", path, want, string(content))
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, stat err=%v", path, err)
	}
}
