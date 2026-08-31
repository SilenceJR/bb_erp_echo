package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCustomerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:customer-%s?mode=memory&cache=shared&_foreign_keys=1", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.CustomerCode{}, &model.CustomerProfile{}, &model.ImportSession{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

func TestNormalizeCode(t *testing.T) {
	cases := map[string]string{"1": "BB-001", "BB-1": "BB-001", "bb-001": "BB-001", "BB-9999": "BB-9999"}
	for input, want := range cases {
		got, err := NormalizeCode(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeCode(%q)=(%q,%v), want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"0", "BB-000", "A-001", "BB--1", ""} {
		if _, err := NormalizeCode(input); err == nil {
			t.Fatalf("NormalizeCode(%q) should fail", input)
		}
	}
}

func TestCodeGenerationDoesNotFillGapsAndDetectsConflict(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	for _, code := range []string{"BB-001", "BB-003"} {
		if _, err := s.CreateCode(code); err != nil {
			t.Fatal(err)
		}
	}
	if next, err := s.NextCode(); err != nil || next != "BB-004" {
		t.Fatalf("next=(%q,%v)", next, err)
	}
	if _, err := s.CreateCode("3"); !errors.Is(err, ErrCodeConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestConcurrentAutomaticCodeGeneration(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	const count = 8
	codes := make(chan string, count)
	errorsCh := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, err := s.CreateCode("")
			if err != nil {
				errorsCh <- err
				return
			}
			codes <- item.Code
		}()
	}
	wg.Wait()
	close(codes)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent create: %v", err)
	}
	got := make([]string, 0, count)
	for code := range codes {
		got = append(got, code)
	}
	sort.Strings(got)
	for i := 0; i < count; i++ {
		want := fmt.Sprintf("BB-%03d", i+1)
		if got[i] != want {
			t.Fatalf("codes=%v", got)
		}
	}
}

func TestProfileDefaultInvariantAndReplacementDelete(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	code, err := s.CreateCode("9")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateProfile(ProfileInput{CustomerCodeID: code.ID, ShortName: "甲"})
	if err != nil || !first.IsDefault {
		t.Fatalf("first=(%+v,%v)", first, err)
	}
	second, err := s.CreateProfile(ProfileInput{CustomerCodeID: code.ID, ShortName: "乙"})
	if err != nil || second.IsDefault {
		t.Fatalf("second=(%+v,%v)", second, err)
	}
	if err = s.DeleteProfile(first.ID, 0); !errors.Is(err, ErrReplacementNeeded) {
		t.Fatalf("missing replacement error=%v", err)
	}
	if err = s.DeleteProfile(first.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProfile(second.ID)
	if err != nil || !got.IsDefault {
		t.Fatalf("replacement=(%+v,%v)", got, err)
	}
	if err = s.DeleteProfile(second.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteCode(code.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSetDefaultLeavesExactlyOneDefault(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	code, _ := s.CreateCode("12")
	first, _ := s.CreateProfile(ProfileInput{CustomerCodeID: code.ID})
	second, _ := s.CreateProfile(ProfileInput{CustomerCodeID: code.ID})
	if _, err := s.SetDefault(second.ID); err != nil {
		t.Fatal(err)
	}
	var defaults int64
	if err := db.Model(&model.CustomerProfile{}).Where("customer_code_id=? AND is_default=1", code.ID).Count(&defaults).Error; err != nil {
		t.Fatal(err)
	}
	if defaults != 1 {
		t.Fatalf("defaults=%d", defaults)
	}
	got, _ := s.GetProfile(first.ID)
	if got.IsDefault {
		t.Fatal("old profile remains default")
	}
}

func TestDeleteReferencedProfileReturnsConflict(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	code, _ := s.CreateCode("22")
	profile, _ := s.CreateProfile(ProfileInput{CustomerCodeID: code.ID})
	if err := db.Exec("CREATE TABLE inventory_documents (id integer primary key, customer_id integer)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO inventory_documents(id,customer_id) VALUES(1,?)", profile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile(profile.ID, 0); !errors.Is(err, ErrProfileReferenced) {
		t.Fatalf("error=%v", err)
	}
}

func TestGroupedFiltersAndEmptyFields(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	empty, _ := s.CreateCode("30")
	multiple, _ := s.CreateCode("31")
	_, _ = s.CreateProfile(ProfileInput{CustomerCodeID: multiple.ID})
	_, _ = s.CreateProfile(ProfileInput{CustomerCodeID: multiple.ID, ContactPhone: "00123"})
	result, err := s.ListCodes(pagination.Query{Page: 1, PageSize: 20}, "multiple")
	if err != nil || result.Total != 1 || result.Items[0].ID != multiple.ID {
		t.Fatalf("multiple=%+v err=%v", result, err)
	}
	result, err = s.ListCodes(pagination.Query{Page: 1, PageSize: 20}, "empty")
	if err != nil || result.Total != 1 || result.Items[0].ID != empty.ID {
		t.Fatalf("empty=%+v err=%v", result, err)
	}
}

func TestCustomerWorkbookRoundTripKeepsPhoneAsText(t *testing.T) {
	doc := spreadsheet.SpreadsheetDocument{
		SheetName: "客户资料", Title: "客户资料", Columns: customerColumns,
		Rows: [][]string{{"1", "BB-009", "简称", "名称", "地址", "0755-001", "联系人", "0013800", "业务员"}}, TotalRows: 1,
	}
	data, err := (spreadsheet.XLSXWriter{}).Write(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	rows, cellErrors := parseCustomerWorkbook(context.Background(), "客户资料.xlsx", data)
	if len(cellErrors) != 0 || len(rows) != 1 {
		t.Fatalf("rows=%+v errors=%+v", rows, cellErrors)
	}
	if rows[0].Phone != "0755-001" || rows[0].ContactPhone != "0013800" {
		t.Fatalf("phone values changed: %+v", rows[0])
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	if value, _ := book.GetCellValue("客户资料", "F3", excelize.Options{RawCellValue: true}); value != "0755-001" {
		t.Fatalf("raw phone=%q", value)
	}
	phoneStyleID, err := book.GetCellStyle("客户资料", "F3")
	if err != nil {
		t.Fatal(err)
	}
	phoneStyle, err := book.GetStyle(phoneStyleID)
	if err != nil || phoneStyle.NumFmt != 49 || len(phoneStyle.Border) != 4 {
		t.Fatalf("phone style=%+v err=%v", phoneStyle, err)
	}
	codeStyleID, err := book.GetCellStyle("客户资料", "B3")
	if err != nil {
		t.Fatal(err)
	}
	codeStyle, err := book.GetStyle(codeStyleID)
	if err != nil || codeStyle.Alignment == nil || codeStyle.Alignment.Horizontal != "center" {
		t.Fatalf("code style=%+v err=%v", codeStyle, err)
	}
}

func TestExportPreviewAndDownloadShareDocumentDefinition(t *testing.T) {
	db := newCustomerTestDB(t)
	s := NewService(db)
	code, _ := s.CreateCode("9")
	_, _ = s.CreateProfile(ProfileInput{CustomerCodeID: code.ID, Name: "客户", Phone: "0755-001", ContactPhone: "0013800"})
	h := NewHandler(db)
	e := echo.New()
	previewContext := e.NewContext(httptest.NewRequest(http.MethodGet, "/?scope=current&page=1&page_size=50", nil), httptest.NewRecorder())
	preview, err := h.buildExportDocument(previewContext, true)
	if err != nil {
		t.Fatal(err)
	}
	downloadContext := e.NewContext(httptest.NewRequest(http.MethodGet, "/?scope=current", nil), httptest.NewRecorder())
	download, err := h.buildExportDocument(downloadContext, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview.Columns, download.Columns) || !reflect.DeepEqual(preview.Rows, download.Rows) {
		t.Fatalf("preview and download differ:\npreview=%+v\ndownload=%+v", preview, download)
	}
	if preview.TotalRows != 1 || preview.Empty || preview.HasMore {
		t.Fatalf("preview metadata=%+v", preview)
	}
}

func TestPreviewRejectsDuplicateRowsAtomically(t *testing.T) {
	db := newCustomerTestDB(t)
	h := NewHandler(db)
	rows := []importRow{
		{Row: 2, Code: "BB-001", Name: "同一资料"},
		{Row: 3, Code: "BB-001", Name: "同一资料"},
	}
	summary, cellErrors, err := h.previewRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	if summary.MultipleCodeGroups != 1 || len(cellErrors) != 1 {
		t.Fatalf("summary=%+v errors=%+v", summary, cellErrors)
	}
}

func TestReferenceXLSWhenAvailable(t *testing.T) {
	data, err := os.ReadFile("../../博邦/客户资料.xls")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("参考客户资料.xls 未放入工作区")
	}
	if err != nil {
		t.Fatal(err)
	}
	rows, cellErrors := parseCustomerWorkbook(context.Background(), "客户资料.xls", data)
	if len(cellErrors) != 0 {
		t.Fatalf("reference xls errors=%+v", cellErrors)
	}
	if len(rows) == 0 || rows[0].Code != "BB-001" {
		t.Fatalf("reference xls rows=%d first=%+v", len(rows), rows[0])
	}
}

func TestImportPreviewCommitAndReplay(t *testing.T) {
	db := newCustomerTestDB(t)
	h := NewHandler(db)
	doc := spreadsheet.SpreadsheetDocument{
		SheetName: "客户资料", Title: "客户资料", Columns: customerColumns,
		Rows: [][]string{{"", "BB-101", "甲", "客户甲", "", "010-001", "张三", "00138", "业务员"}}, TotalRows: 1,
	}
	data, err := (spreadsheet.XLSXWriter{}).Write(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: 77, Username: "importer"}
	previewRec := performImport(t, h.importPreview, current, data, "")
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRec.Code, previewRec.Body.String())
	}
	var preview ImportPreview
	if err := json.Unmarshal(previewRec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Token == "" || len(preview.Errors) != 0 {
		t.Fatalf("preview=%+v", preview)
	}
	wrongUserRec := performImport(t, h.importCommit, &auth.CurrentUser{ID: 78, Username: "other"}, data, preview.Token)
	if wrongUserRec.Code != http.StatusConflict {
		t.Fatalf("wrong user status=%d body=%s", wrongUserRec.Code, wrongUserRec.Body.String())
	}
	commitRec := performImport(t, h.importCommit, current, data, preview.Token)
	if commitRec.Code != http.StatusCreated {
		t.Fatalf("commit status=%d body=%s", commitRec.Code, commitRec.Body.String())
	}
	replayRec := performImport(t, h.importCommit, current, data, preview.Token)
	if replayRec.Code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%s", replayRec.Code, replayRec.Body.String())
	}
	var profiles int64
	if err := db.Model(&model.CustomerProfile{}).Count(&profiles).Error; err != nil || profiles != 1 {
		t.Fatalf("profiles=%d err=%v", profiles, err)
	}
}

func TestImportBatchKeepsExistingDefaultAndAssignsNewCodeDefaultOnce(t *testing.T) {
	db := newCustomerTestDB(t)
	h := NewHandler(db)
	s := NewService(db)
	existingCode, err := s.CreateCode("201")
	if err != nil {
		t.Fatal(err)
	}
	existingDefault, err := s.CreateProfile(ProfileInput{CustomerCodeID: existingCode.ID, ShortName: "原默认"})
	if err != nil {
		t.Fatal(err)
	}

	doc := spreadsheet.SpreadsheetDocument{
		SheetName: "客户资料", Title: "客户资料", Columns: customerColumns,
		Rows: [][]string{
			{"", "BB-201", "追加资料", "已有编码资料", "", "010-201", "联系人一", "001201", "业务员"},
			{"", "BB-202", "资料一", "新客户一", "", "010-202", "联系人二", "001202", "业务员"},
			{"", "BB-202", "资料二", "新客户二", "", "010-203", "联系人三", "001203", "业务员"},
		},
		TotalRows: 3,
	}
	data, err := (spreadsheet.XLSXWriter{}).Write(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: 88, Username: "batch-importer"}
	previewRec := performImport(t, h.importPreview, current, data, "")
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRec.Code, previewRec.Body.String())
	}
	var preview ImportPreview
	if err := json.Unmarshal(previewRec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Token == "" || len(preview.Errors) != 0 || preview.Summary.NewCodes != 1 || preview.Summary.NewProfiles != 3 || preview.Summary.MultipleCodeGroups != 1 {
		t.Fatalf("preview=%+v", preview)
	}
	commitRec := performImport(t, h.importCommit, current, data, preview.Token)
	if commitRec.Code != http.StatusCreated {
		t.Fatalf("commit status=%d body=%s", commitRec.Code, commitRec.Body.String())
	}

	var profiles []model.CustomerProfile
	if err := db.Where("customer_code_id = ?", existingCode.ID).Order("id").Find(&profiles).Error; err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || !profiles[0].IsDefault || profiles[1].IsDefault || profiles[0].ID != existingDefault.ID {
		t.Fatalf("existing code profiles=%+v, want original default plus one non-default", profiles)
	}
	var newCode model.CustomerCode
	if err := db.Where("code = ?", "BB-202").First(&newCode).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("customer_code_id = ?", newCode.ID).Order("id").Find(&profiles).Error; err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || !profiles[0].IsDefault || profiles[1].IsDefault {
		t.Fatalf("new code profiles=%+v, want exactly first default", profiles)
	}
}

func performImport(t *testing.T, handler echo.HandlerFunc, current *auth.CurrentUser, file []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "客户资料.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(file); err != nil {
		t.Fatal(err)
	}
	if token != "" {
		if err = writer.WriteField("token", token); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserKey, current)
	if err = handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}
