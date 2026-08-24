// Package logger 提供 ERP 后台文件化结构日志系统。
package logger

import (
	"fmt"
	"io"
	"os"

	"bb_erp_echo/internal/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bb_erp_echo/internal/config"
)

const (
	appKind    = "app"
	accessKind = "access"
	errorKind  = "error"
)

// System 是应用日志系统。
//
// 字段说明：
// - App：应用启动、关闭和业务信息日志。
// - Access：HTTP 访问日志。
// - Error：统一错误处理和系统错误日志。
type System struct {
	App    *slog.Logger
	Access *slog.Logger
	Error  *slog.Logger

	writers []*DailyFileWriter
}

// New 创建日志系统。
//
// 参数说明：
// - cfg：日志配置，包含级别、目录、控制台输出和保留天数。
//
// 返回说明：返回三类日志器；目录创建、旧日志清理或文件打开失败时返回错误。
func New(cfg config.LogConfig) (*System, error) {
	if cfg.Dir == "" {
		cfg.Dir = "logs"
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := Cleanup(cfg.Dir, cfg.RetentionDays, time.Now()); err != nil {
		return nil, err
	}

	level := parseLevel(cfg.Level)
	system := &System{}
	for _, kind := range []string{appKind, accessKind, errorKind} {
		writer := NewDailyFileWriter(cfg.Dir, kind, time.Now)
		system.writers = append(system.writers, writer)

		output := io.Writer(writer)
		if cfg.Console {
			output = io.MultiWriter(os.Stdout, writer)
		}
		logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))

		switch kind {
		case appKind:
			system.App = logger
		case accessKind:
			system.Access = logger
		case errorKind:
			system.Error = logger
		}
	}

	return system, nil
}

// Close 关闭日志文件句柄。
//
// 参数说明：无。
// 返回说明：返回关闭过程中遇到的第一个错误。
func (s *System) Close() error {
	var firstErr error
	for _, writer := range s.writers {
		if err := writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DailyFileWriter 是按日期自动切分的文件 writer。
//
// 写入规则：
// - 每次写入时检查当前日期。
// - 日期变化时关闭旧文件并打开新文件。
// - 文件名格式为 kind-YYYY-MM-DD.log。
type DailyFileWriter struct {
	dir     string
	kind    string
	now     func() time.Time
	mu      sync.Mutex
	date    string
	current *os.File
}

// NewDailyFileWriter 创建按天切分的文件 writer。
//
// 参数说明：
// - dir：日志目录。
// - kind：日志分类，例如 app、access、error。
// - now：当前时间函数，测试中可注入固定时间。
func NewDailyFileWriter(dir string, kind string, now func() time.Time) *DailyFileWriter {
	return &DailyFileWriter{dir: dir, kind: kind, now: now}
}

// Write 写入日志内容。
//
// 参数说明：
// - p：slog JSON handler 生成的一行日志内容。
func (w *DailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateLocked(); err != nil {
		return 0, err
	}
	return w.current.Write(p)
}

// Close 关闭当前打开的日志文件。
func (w *DailyFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.current == nil {
		return nil
	}
	err := w.current.Close()
	w.current = nil
	return err
}

func (w *DailyFileWriter) rotateLocked() error {
	date := w.now().Format(time.DateOnly)
	if w.current != nil && w.date == date {
		return nil
	}
	if w.current != nil {
		if err := w.current.Close(); err != nil {
			return err
		}
		w.current = nil
	}

	path := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.kind, date))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.current = file
	w.date = date
	return nil
}

// Cleanup 清理超过保留天数的历史日志文件。
//
// 参数说明：
// - dir：日志目录。
// - retentionDays：保留天数。
// - now：当前时间，用于测试中固定判断基准。
//
// 返回说明：只删除 app/access/error 且符合日期命名的 .log 文件。
func Cleanup(dir string, retentionDays int, now time.Time) error {
	if retentionDays <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}

	// 以日期为单位清理，避免当天中午启动时误删保留边界日凌晨的日志。
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := today.AddDate(0, 0, -retentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		kind, date, ok := parseLogName(entry.Name())
		if !ok || !knownKind(kind) {
			continue
		}
		logDate, err := time.Parse(time.DateOnly, date)
		if err != nil {
			continue
		}
		if logDate.Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return fmt.Errorf("remove old log file %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseLogName(name string) (string, string, bool) {
	if !strings.HasSuffix(name, ".log") {
		return "", "", false
	}
	withoutExt := strings.TrimSuffix(name, ".log")
	parts := strings.SplitN(withoutExt, "-", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func knownKind(kind string) bool {
	return kind == appKind || kind == accessKind || kind == errorKind
}
