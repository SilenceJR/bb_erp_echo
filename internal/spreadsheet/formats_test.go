package spreadsheet

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestXLSXReaderStopsAtRowLimitWhileStreaming(t *testing.T) {
	document := SpreadsheetDocument{
		SheetName: "Sheet1",
		Columns:   []Column{{Key: "value", Title: "值", Type: CellTypeText}},
		Rows:      [][]string{{"1"}, {"2"}, {"3"}},
	}
	data, err := (XLSXWriter{}).Write(context.Background(), document)
	if err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	_, err = (XLSXReader{}).Read(context.Background(), bytes.NewReader(data), ReadOptions{MaxRows: 2, MaxColumns: 4})
	if !errors.Is(err, ErrTooManyRows) {
		t.Fatalf("read error = %v, want ErrTooManyRows", err)
	}
}

func TestXLSXReaderStopsAtColumnLimitWhileStreaming(t *testing.T) {
	document := SpreadsheetDocument{
		SheetName: "Sheet1",
		Columns: []Column{
			{Key: "first", Title: "第一列", Type: CellTypeText},
			{Key: "second", Title: "第二列", Type: CellTypeText},
			{Key: "third", Title: "第三列", Type: CellTypeText},
		},
		Rows: [][]string{{"1", "2", "3"}},
	}
	data, err := (XLSXWriter{}).Write(context.Background(), document)
	if err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	_, err = (XLSXReader{}).Read(context.Background(), bytes.NewReader(data), ReadOptions{MaxRows: 8, MaxColumns: 2})
	if !errors.Is(err, ErrTooManyColumns) {
		t.Fatalf("read error = %v, want ErrTooManyColumns", err)
	}
}
