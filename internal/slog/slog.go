// Package slog provides a minimal structured logging shim compatible with
// the standard log/slog API subset used by this project. It exists so the
// project can compile with Go 1.20 for Windows 7 binary compatibility,
// since log/slog was only introduced in Go 1.21.
package slog

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// HandlerOptions configures a JSONHandler.
type HandlerOptions struct {
	Level Level
}

// Handler is implemented by log record handlers.
type Handler interface {
	Handle(level Level, msg string, args ...any)
}

// Logger emits structured log records through a Handler.
type Logger struct {
	handler Handler
}

// New creates a Logger that writes through h.
func New(h Handler) *Logger {
	return &Logger{handler: h}
}

func (l *Logger) log(level Level, msg string, args ...any) {
	if l == nil || l.handler == nil {
		return
	}
	l.handler.Handle(level, msg, args...)
}

// Debug logs a message at DEBUG level.
func (l *Logger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args...) }

// Info logs a message at INFO level.
func (l *Logger) Info(msg string, args ...any) { l.log(LevelInfo, msg, args...) }

// Warn logs a message at WARN level.
func (l *Logger) Warn(msg string, args ...any) { l.log(LevelWarn, msg, args...) }

// Error logs a message at ERROR level.
func (l *Logger) Error(msg string, args ...any) { l.log(LevelError, msg, args...) }

// JSONHandler writes JSON-encoded log records to an io.Writer.
type JSONHandler struct {
	mu    sync.Mutex
	w     io.Writer
	level Level
}

// NewJSONHandler creates a JSONHandler that writes to w.
func NewJSONHandler(w io.Writer, opts *HandlerOptions) *JSONHandler {
	h := &JSONHandler{w: w}
	if opts != nil {
		h.level = opts.Level
	}
	return h
}

// Handle writes a JSON log record if level passes the filter.
func (h *JSONHandler) Handle(level Level, msg string, args ...any) {
	if level < h.level {
		return
	}
	record := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339Nano),
		"level": levelString(level),
		"msg":   msg,
	}
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		record[key] = args[i+1]
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	_, _ = h.w.Write(append(data, '\n'))
}

func levelString(level Level) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}
