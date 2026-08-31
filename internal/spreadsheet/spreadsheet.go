// Package spreadsheet 提供可扩展的 Excel/电子表格读写抽象。
//
// 领域模块只依赖这里定义的文档、Schema、导入/导出处理器和格式注册表，
// 不直接耦合具体文件格式。新增资料类型时可以复用这些能力，而无需复制
// 文件大小、工作表、列类型和样式处理代码。
package spreadsheet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	// DefaultMaxRows 是读入文件的默认最大行数。
	DefaultMaxRows = 10000
	// DefaultMaxColumns 是单个工作表允许的最大列数。
	DefaultMaxColumns = 128
	// MaxFileSize 是 HTTP 导入接口采用的最大文件大小。
	MaxFileSize int64 = 10 * 1024 * 1024

	CellTypeText   = "text"
	CellTypeNumber = "number"
	CellTypeDate   = "date"
	CellTypeBool   = "bool"
)

var (
	// ErrUnsupportedFormat 表示当前注册表没有对应扩展名的适配器。
	ErrUnsupportedFormat = errors.New("unsupported spreadsheet format")
	// ErrInvalidSignature 表示扩展名与文件头不匹配。
	ErrInvalidSignature = errors.New("invalid spreadsheet file signature")
	// ErrTooManyRows 表示工作表超过导入安全上限。
	ErrTooManyRows = errors.New("spreadsheet has too many rows")
	// ErrTooManyColumns 表示工作表超过导入安全上限。
	ErrTooManyColumns = errors.New("spreadsheet has too many columns")
)

// Column 描述导入/导出工作表中的一列。
type Column struct {
	Key       string  `json:"key"`
	Title     string  `json:"title"`
	Width     float64 `json:"width"`
	Type      string  `json:"type"`
	Alignment string  `json:"alignment,omitempty"`
}

// SpreadsheetDocument 是领域导出处理器产生的标准文档模型。
// Rows 中每个字符串位置对应 Columns 中同位置的列；列类型由 Columns.Type
// 指定，使 JSON 预览和 XLSX 写出共用完全相同的数据转换结果。
type SpreadsheetDocument struct {
	SheetName string     `json:"sheet_name"`
	Title     string     `json:"title,omitempty"`
	Columns   []Column   `json:"columns"`
	Rows      [][]string `json:"rows"`
	TotalRows int64      `json:"total_rows"`
	Page      int        `json:"page,omitempty"`
	PageSize  int        `json:"page_size,omitempty"`
	Empty     bool       `json:"empty"`
	HasMore   bool       `json:"has_more"`
}

// ReadOptions 控制工作簿读取上限。
type ReadOptions struct {
	MaxRows    int
	MaxColumns int
}

func (o ReadOptions) withDefaults() ReadOptions {
	if o.MaxRows <= 0 {
		o.MaxRows = DefaultMaxRows
	}
	if o.MaxColumns <= 0 {
		o.MaxColumns = DefaultMaxColumns
	}
	return o
}

// WorkbookReader 是具体文件格式的读取策略。
type WorkbookReader interface {
	Read(context.Context, io.Reader, ReadOptions) ([][]string, error)
}

// WorkbookWriter 是具体文件格式的写出策略。
type WorkbookWriter interface {
	Write(context.Context, SpreadsheetDocument) ([]byte, error)
	Extension() string
	ContentType() string
}

// SheetSchema 描述领域类型与表格行之间的双向转换。
type SheetSchema[T any] interface {
	Columns() []Column
	Decode(row []string, rowNumber int) (T, []CellError)
	Encode(value T) []string
}

// ImportHandler 封装某种资料的领域导入预览与提交行为。
// Preview 的返回值由领域模块定义，通常包含逐行错误和规范化记录；Commit
// 必须在调用方提供的事务内完成原子写入。
type ImportHandler[T any] interface {
	Preview(context.Context, [][]string) (any, error)
	Commit(context.Context, any) error
}

// ExportOptions 是通用导出处理器的分页和筛选参数。
type ExportOptions struct {
	Scope    string
	Keyword  string
	Filter   string
	Page     int
	PageSize int
}

// ExportHandler 把领域查询结果转换为标准文档。
type ExportHandler[T any] interface {
	BuildDocument(context.Context, ExportOptions) (SpreadsheetDocument, error)
}

// CellError 是 Schema 解码时的标准化单元格错误。
type CellError struct {
	Row    int    `json:"row"`
	Column string `json:"column"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

func (e CellError) Error() string {
	if e.Column == "" {
		return fmt.Sprintf("第 %d 行：%s", e.Row, e.Reason)
	}
	return fmt.Sprintf("第 %d 行 %s：%s", e.Row, e.Column, e.Reason)
}

// FormatRegistry 将文件扩展名路由到对应读写适配器。
type FormatRegistry struct {
	readers map[string]WorkbookReader
	writers map[string]WorkbookWriter
}

// NewFormatRegistry 创建空格式注册表。
func NewFormatRegistry() *FormatRegistry {
	return &FormatRegistry{readers: make(map[string]WorkbookReader), writers: make(map[string]WorkbookWriter)}
}

// NewRegistry 是 NewFormatRegistry 的简短别名。
func NewRegistry() *FormatRegistry { return NewFormatRegistry() }

// RegisterReader 注册一个扩展名读取适配器。
func (r *FormatRegistry) RegisterReader(extension string, reader WorkbookReader) {
	if r == nil {
		return
	}
	if r.readers == nil {
		r.readers = make(map[string]WorkbookReader)
	}
	r.readers[normalizeExtension(extension)] = reader
}

// RegisterWriter 注册一个扩展名写出适配器。
func (r *FormatRegistry) RegisterWriter(extension string, writer WorkbookWriter) {
	if r == nil {
		return
	}
	if r.writers == nil {
		r.writers = make(map[string]WorkbookWriter)
	}
	r.writers[normalizeExtension(extension)] = writer
}

// Register 同时注册读写策略；writer 可以为 nil，适用于只读格式。
func (r *FormatRegistry) Register(extension string, reader WorkbookReader, writer WorkbookWriter) {
	r.RegisterReader(extension, reader)
	if writer != nil {
		r.RegisterWriter(extension, writer)
	}
}

// Reader 根据文件名扩展名返回读取策略。
func (r *FormatRegistry) Reader(filename string) (WorkbookReader, error) {
	if r == nil {
		return nil, ErrUnsupportedFormat
	}
	reader, ok := r.readers[normalizeExtension(filepath.Ext(filename))]
	if !ok || reader == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Ext(filename))
	}
	return reader, nil
}

// Writer 根据文件名扩展名返回写出策略。
func (r *FormatRegistry) Writer(filename string) (WorkbookWriter, error) {
	if r == nil {
		return nil, ErrUnsupportedFormat
	}
	writer, ok := r.writers[normalizeExtension(filepath.Ext(filename))]
	if !ok || writer == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Ext(filename))
	}
	return writer, nil
}

// DefaultRegistry 返回内置 .xls/.xlsx 适配器注册表。
func DefaultRegistry() *FormatRegistry {
	r := NewFormatRegistry()
	r.Register(".xls", XLSReader{}, nil)
	r.Register(".xlsx", XLSXReader{}, XLSXWriter{})
	return r
}

func normalizeExtension(extension string) string {
	extension = strings.TrimSpace(strings.ToLower(extension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return extension
}

// ValidateSignature 检查扩展名对应的 OLE 或 ZIP 文件头。
func ValidateSignature(extension string, data []byte) error {
	extension = normalizeExtension(extension)
	switch extension {
	case ".xls":
		if len(data) >= 8 && string(data[:8]) == string([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
			return nil
		}
	case ".xlsx":
		if len(data) >= 4 && string(data[:4]) == "PK\x03\x04" {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrInvalidSignature, extension)
}
