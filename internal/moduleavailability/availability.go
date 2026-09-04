// Package moduleavailability provides a single, public error and middleware
// for modules whose database schema is intentionally deferred during a
// staged ERP migration.
package moduleavailability

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const (
	// Code is the stable API error code for a deferred module schema.
	Code = "module_not_initialized"
)

// Requirement identifies one table required by a module endpoint. Name is
// deliberately supplied by the caller so the public error remains stable
// across GORM naming-strategy changes.
type Requirement struct {
	Model any
	Name  string
}

// Error is returned when one or more required module tables are absent.
// It implements the shared business-error contract without importing the
// response package (which keeps this package usable by handlers and tests).
type Error struct {
	Module       string
	MissingTable []string
}

func (e *Error) Error() string {
	if e == nil {
		return Code
	}
	if len(e.MissingTable) == 0 {
		return fmt.Sprintf("module %s is not initialized", e.Module)
	}
	return fmt.Sprintf("module %s is not initialized; missing tables: %s", e.Module, strings.Join(e.MissingTable, ", "))
}

// Code returns the stable API error code.
func (e *Error) Code() string { return Code }

// StatusCode returns HTTP 503 because the service is healthy but this module
// is intentionally unavailable until its later schema redesign is delivered.
func (e *Error) StatusCode() int { return http.StatusServiceUnavailable }

// PublicMessage returns a safe, user-facing message with no SQL details.
func (e *Error) PublicMessage() string {
	if e == nil || strings.TrimSpace(e.Module) == "" {
		return "当前模块的数据结构尚未初始化，请稍后再试"
	}
	return fmt.Sprintf("%s模块的数据结构尚未初始化，待后续重构完成后再使用", e.Module)
}

// Check returns nil when every required table exists, or a stable 503 error
// when any required table is missing. Database inspection itself is kept
// separate from endpoint queries so callers never issue a doomed SQL query.
func Check(db *gorm.DB, module string, requirements ...Requirement) error {
	if db == nil {
		return &Error{Module: module}
	}
	missing := make([]string, 0, len(requirements))
	seen := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		name := strings.TrimSpace(requirement.Name)
		if name == "" {
			name = "unknown"
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if requirement.Model == nil || !db.Migrator().HasTable(requirement.Model) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &Error{Module: module, MissingTable: missing}
}

// Middleware checks deferred module tables before invoking the endpoint.
func Middleware(db *gorm.DB, module string, requirements ...Requirement) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := Check(db, module, requirements...); err != nil {
				return err
			}
			return next(c)
		}
	}
}

// Is reports whether err is a module-not-initialized error.
func Is(err error) bool {
	var target *Error
	return errors.As(err, &target)
}
