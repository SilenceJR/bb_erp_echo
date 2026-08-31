package spreadsheet

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

// XLSReader 使用 BIFF8/OLE 读取旧版 .xls 工作簿的首个工作表。
type XLSReader struct{}

// Read 读取首个工作表并将单元格统一为字符串。
func (XLSReader) Read(ctx context.Context, reader io.Reader, options ReadOptions) ([][]string, error) {
	options = options.withDefaults()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read xls: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := ValidateSignature(".xls", data); err != nil {
		return nil, err
	}
	workbook, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return nil, fmt.Errorf("open xls workbook: %w", err)
	}
	if workbook.NumSheets() == 0 || workbook.GetSheet(0) == nil {
		return nil, fmt.Errorf("xls workbook has no worksheets")
	}
	sheet := workbook.GetSheet(0)
	if int(sheet.MaxRow)+1 > options.MaxRows {
		return nil, ErrTooManyRows
	}
	rows := make([][]string, 0, int(sheet.MaxRow)+1)
	for rowIndex := 0; rowIndex <= int(sheet.MaxRow); rowIndex++ {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		row := sheet.Row(rowIndex)
		if row == nil {
			rows = append(rows, []string{})
			continue
		}
		lastColumn := row.LastCol()
		if int(lastColumn)+1 > options.MaxColumns {
			return nil, ErrTooManyColumns
		}
		values := make([]string, int(lastColumn)+1)
		for columnIndex := 0; columnIndex <= int(lastColumn); columnIndex++ {
			values[columnIndex] = row.Col(columnIndex)
		}
		rows = append(rows, values)
	}
	return rows, nil
}

// XLSXReader 使用 Excelize 读取 OOXML .xlsx 工作簿的首个工作表。
type XLSXReader struct{}

// Read 读取首个工作表并将单元格统一为字符串。
func (XLSXReader) Read(ctx context.Context, reader io.Reader, options ReadOptions) ([][]string, error) {
	options = options.withDefaults()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read xlsx: %w", err)
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := ValidateSignature(".xlsx", data); err != nil {
		return nil, err
	}
	// 上传文件本身限制为 10 MiB；同时限制 ZIP 解压后的总体积和单个 XML，
	// 防止体积很小的压缩炸弹耗尽服务端内存或磁盘。
	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    128 * 1024 * 1024,
		UnzipXMLSizeLimit: 32 * 1024 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("open xlsx workbook: %w", err)
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx workbook has no worksheets")
	}
	stream, err := workbook.Rows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("open xlsx worksheet: %w", err)
	}
	// Rows 是 Excelize 的 SAX 迭代器。必须在发现上限超出时立即结束，
	// 不能先把整个工作表读入内存再校验；defer 确保所有提前返回路径都关闭
	// 工作表临时文件。
	defer func() { _ = stream.Close() }()
	rows := make([][]string, 0, min(options.MaxRows, 256))
	for stream.Next() {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if len(rows) >= options.MaxRows {
			return nil, ErrTooManyRows
		}
		row, err := stream.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("read xlsx worksheet row: %w", err)
		}
		if len(row) > options.MaxColumns {
			return nil, ErrTooManyColumns
		}
		rows = append(rows, row)
	}
	if err := stream.Error(); err != nil {
		return nil, fmt.Errorf("read xlsx worksheet: %w", err)
	}
	return rows, nil
}

// XLSXWriter 输出带标题、列宽、边框和文本电话格式的 .xlsx 工作簿。
type XLSXWriter struct{}

func (XLSXWriter) Extension() string { return ".xlsx" }
func (XLSXWriter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

// Write 将标准文档写出为 Excelize 工作簿。
func (XLSXWriter) Write(ctx context.Context, document SpreadsheetDocument) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if len(document.Columns) == 0 {
		return nil, fmt.Errorf("spreadsheet document has no columns")
	}
	sheetName := strings.TrimSpace(document.SheetName)
	if sheetName == "" {
		sheetName = "客户资料"
	}
	workbook := excelize.NewFile()
	defer workbook.Close()
	defaultSheet := workbook.GetSheetList()[0]
	if defaultSheet != sheetName {
		if err := workbook.SetSheetName(defaultSheet, sheetName); err != nil {
			return nil, fmt.Errorf("rename worksheet: %w", err)
		}
	}
	workbook.SetActiveSheet(0)

	titleRow, headerRow, firstDataRow := 1, 2, 3
	if title := strings.TrimSpace(document.Title); title != "" {
		if err := workbook.SetCellValue(sheetName, "A1", title); err != nil {
			return nil, err
		}
		lastCell, err := excelize.CoordinatesToCellName(len(document.Columns), titleRow)
		if err != nil {
			return nil, err
		}
		if err := workbook.MergeCell(sheetName, "A1", lastCell); err != nil {
			return nil, err
		}
	} else {
		headerRow, firstDataRow = 1, 2
	}

	for index, column := range document.Columns {
		cell, err := excelize.CoordinatesToCellName(index+1, headerRow)
		if err != nil {
			return nil, err
		}
		if err := workbook.SetCellValue(sheetName, cell, column.Title); err != nil {
			return nil, err
		}
		columnName, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		width := column.Width
		if width <= 0 {
			width = 12
		}
		if err := workbook.SetColWidth(sheetName, columnName, columnName, width); err != nil {
			return nil, err
		}
	}

	headerStyle, bodyStyle, titleStyle, textStyle, centerBodyStyle, centerTextStyle, err := xlsxStyles(workbook)
	if err != nil {
		return nil, err
	}
	lastColumn, err := excelize.ColumnNumberToName(len(document.Columns))
	if err != nil {
		return nil, err
	}
	if err := workbook.SetCellStyle(sheetName, fmt.Sprintf("A%d", headerRow), fmt.Sprintf("%s%d", lastColumn, headerRow), headerStyle); err != nil {
		return nil, err
	}
	if document.Title != "" {
		if err := workbook.SetCellStyle(sheetName, "A1", fmt.Sprintf("%s1", lastColumn), titleStyle); err != nil {
			return nil, err
		}
	}
	for rowIndex, row := range document.Rows {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		for columnIndex, column := range document.Columns {
			value := ""
			if columnIndex < len(row) {
				value = row[columnIndex]
			}
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, firstDataRow+rowIndex)
			if err != nil {
				return nil, err
			}
			if column.Type == CellTypeNumber {
				if number, parseErr := parseNumber(value); parseErr == nil {
					if err := workbook.SetCellValue(sheetName, cell, number); err != nil {
						return nil, err
					}
				} else if err := workbook.SetCellStr(sheetName, cell, value); err != nil {
					return nil, err
				}
			} else if err := workbook.SetCellStr(sheetName, cell, value); err != nil {
				return nil, err
			}
		}
		if err := workbook.SetCellStyle(sheetName, fmt.Sprintf("A%d", firstDataRow+rowIndex), fmt.Sprintf("%s%d", lastColumn, firstDataRow+rowIndex), bodyStyle); err != nil {
			return nil, err
		}
		for columnIndex, column := range document.Columns {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, firstDataRow+rowIndex)
			if err != nil {
				return nil, err
			}
			style := bodyStyle
			if column.Type == CellTypeText {
				style = textStyle
			}
			if column.Alignment == "center" {
				style = centerBodyStyle
				if column.Type == CellTypeText {
					style = centerTextStyle
				}
			}
			if style != bodyStyle {
				if err := workbook.SetCellStyle(sheetName, cell, cell, style); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := workbook.SetRowHeight(sheetName, headerRow, 20); err != nil {
		return nil, err
	}
	if document.Title != "" {
		if err := workbook.SetRowHeight(sheetName, titleRow, 32); err != nil {
			return nil, err
		}
	}
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buffer.Bytes(), nil
}

func xlsxStyles(workbook *excelize.File) (header, body, title, text, centerBody, centerText int, err error) {
	border := []excelize.Border{
		{Type: "left", Color: "B7C1CE", Style: 1},
		{Type: "top", Color: "B7C1CE", Style: 1},
		{Type: "bottom", Color: "B7C1CE", Style: 1},
		{Type: "right", Color: "B7C1CE", Style: 1},
	}
	header, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DCE6F1"}},
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "1F2937"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return
	}
	body, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Font:      &excelize.Font{Size: 10, Color: "1F2937"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return
	}
	title, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF"}},
		Font:      &excelize.Font{Bold: true, Size: 16, Color: "1F2937"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return
	}
	text, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Font:      &excelize.Font{Size: 10, Color: "1F2937"},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		NumFmt:    49,
	})
	if err != nil {
		return
	}
	centerBody, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Font:      &excelize.Font{Size: 10, Color: "1F2937"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return
	}
	centerText, err = workbook.NewStyle(&excelize.Style{
		Border:    border,
		Font:      &excelize.Font{Size: 10, Color: "1F2937"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		NumFmt:    49,
	})
	return
}

func parseNumber(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty number")
	}
	if value, err := strconvParseInt(raw); err == nil {
		return value, nil
	}
	return strconvParseFloat(raw)
}

func strconvParseInt(raw string) (int64, error) {
	var value int64
	if _, err := fmt.Sscan(raw, &value); err != nil || fmt.Sprint(value) != raw {
		return 0, fmt.Errorf("invalid integer")
	}
	return value, nil
}

func strconvParseFloat(raw string) (float64, error) {
	var value float64
	if _, err := fmt.Sscan(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// isOLESignature 保留为共享层内部的可测试小函数，避免依赖字符串转换判断二进制头。
func isOLESignature(data []byte) bool {
	return len(data) >= 8 && binary.LittleEndian.Uint64(data[:8]) == 0xe11ab1a1e011cfd0
}
