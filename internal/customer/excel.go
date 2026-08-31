package customer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const importModule = "customers"

var customerColumns = []spreadsheet.Column{
	{Key: "id", Title: "序号", Width: 6, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
	{Key: "code", Title: "客户编码", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "short_name", Title: "客户简称", Width: 14, Type: spreadsheet.CellTypeText},
	{Key: "name", Title: "客户名称", Width: 30, Type: spreadsheet.CellTypeText},
	{Key: "address", Title: "地址", Width: 60, Type: spreadsheet.CellTypeText},
	{Key: "phone", Title: "电话", Width: 18, Type: spreadsheet.CellTypeText},
	{Key: "contact_name", Title: "联系人", Width: 14, Type: spreadsheet.CellTypeText},
	{Key: "contact_phone", Title: "联系人电话", Width: 18, Type: spreadsheet.CellTypeText},
	{Key: "salesperson", Title: "业务员", Width: 14, Type: spreadsheet.CellTypeText},
}

type importRow struct {
	Row                                                                           int
	Code, ShortName, Name, Address, Phone, ContactName, ContactPhone, Salesperson string
}
type ImportSummary struct {
	TotalRows          int `json:"total_rows"`
	NewCodes           int `json:"new_codes"`
	NewProfiles        int `json:"new_profiles"`
	MultipleCodeGroups int `json:"multiple_code_groups"`
}
type ImportPreview struct {
	Token     string                  `json:"token,omitempty"`
	ExpiresAt *time.Time              `json:"expires_at,omitempty"`
	Summary   ImportSummary           `json:"summary"`
	Errors    []spreadsheet.CellError `json:"errors"`
}
type ImportResult struct {
	ImportedCodes    int       `json:"imported_codes"`
	ImportedProfiles int       `json:"imported_profiles"`
	CompletedAt      time.Time `json:"completed_at"`
}

func (h *Handler) registerExcelRoutes(g *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	g.GET("/import-template", h.importTemplate, require("/api/v1/customers", "read"))
	g.POST("/import/preview", h.importPreview, require("/api/v1/customers/import", "import"))
	g.POST("/import/commit", h.importCommit, require("/api/v1/customers/import", "import"))
	g.GET("/export/preview", h.exportPreview, require("/api/v1/customers", "read"))
	g.GET("/export", h.exportFile, require("/api/v1/customers", "read"))
}

// @Summary 下载客户资料导入模板
// @Tags 客户
// @Security BearerAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success 200 {file} binary
// @Router /api/v1/customers/import-template [get]
func (h *Handler) importTemplate(c *echo.Context) error {
	doc := spreadsheet.SpreadsheetDocument{SheetName: "客户资料", Title: "东莞博邦光电科技有限公司 客户资料", Columns: customerColumns, Rows: [][]string{{"", "BB-001", "示例简称", "示例客户名称", "示例地址", "0755-12345678", "张先生", "13800000000", "业务员"}}, TotalRows: 1}
	data, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), doc)
	if err != nil {
		return err
	}
	return sendXLSX(c, "客户资料导入模板.xlsx", data)
}

// @Summary 校验客户 Excel 导入
// @Tags 客户
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "客户资料 .xls 或 .xlsx"
// @Success 200 {object} ImportPreview
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/customers/import/preview [post]
func (h *Handler) importPreview(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	header, data, err := readUpload(c)
	if err != nil {
		return err
	}
	rows, errs := parseCustomerWorkbook(c.Request().Context(), header.Filename, data)
	summary, moreErrs, err := h.previewRows(rows)
	if err != nil {
		return err
	}
	errs = append(errs, moreErrs...)
	result := ImportPreview{Summary: summary, Errors: errs}
	if len(errs) == 0 {
		token, tokenHash, err := newToken()
		if err != nil {
			return err
		}
		expires := time.Now().Add(30 * time.Minute)
		session := model.ImportSession{UserID: current.ID, Module: importModule, FileHash: hashBytes(data), TokenHash: tokenHash, ExpiresAt: expires}
		if err = h.DB.Create(&session).Error; err != nil {
			return err
		}
		result.Token = token
		result.ExpiresAt = &expires
	}
	return c.JSON(http.StatusOK, result)
}

// @Summary 确认客户 Excel 导入
// @Tags 客户
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "与预览相同的文件"
// @Param token formData string true "30 分钟一次性预览令牌"
// @Success 201 {object} ImportResult
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/customers/import/commit [post]
func (h *Handler) importCommit(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	header, data, err := readUpload(c)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "缺少预览令牌")
	}
	rows, errs := parseCustomerWorkbook(c.Request().Context(), header.Filename, data)
	if len(errs) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "文件校验失败，请重新预览")
	}
	var result ImportResult
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		// 确认时必须在最终写事务内重新判断数据库重复，避免预览后数据
		// 变化与实际插入之间出现竞态窗口。
		_, currentErrors, snapshot, err := h.previewRowsDBWithSnapshot(tx, rows)
		if err != nil {
			return err
		}
		if len(currentErrors) > 0 {
			return errors.New("import data changed")
		}
		var session model.ImportSession
		if err := tx.Where("token_hash = ? AND user_id = ? AND module = ?", hashToken(token), current.ID, importModule).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("invalid import token")
			}
			return err
		}
		if session.ConsumedAt != nil || time.Now().After(session.ExpiresAt) || session.FileHash != hashBytes(data) {
			return errors.New("expired or mismatched import token")
		}
		now := time.Now()
		consume := tx.Model(&model.ImportSession{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", session.ID, now).
			Update("consumed_at", now)
		if consume.Error != nil {
			return consume.Error
		}
		if consume.RowsAffected != 1 {
			return errors.New("invalid import token")
		}
		createdCodes, err := persistImportRows(tx, rows, snapshot)
		if err != nil {
			return err
		}
		result = ImportResult{ImportedCodes: createdCodes, ImportedProfiles: len(rows), CompletedAt: now}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "data changed") {
			return echo.NewHTTPError(http.StatusConflict, "数据已变化，请重新预览")
		}
		if strings.Contains(err.Error(), "token") {
			return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效、文件不一致或已使用，请重新校验")
		}
		return err
	}
	return c.JSON(http.StatusCreated, result)
}

// @Summary 预览客户资料 XLSX 导出
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Param scope query string true "current 或 all"
// @Param q query string false "关键词"
// @Param filter query string false "all、multiple 或 empty"
// @Param page query int false "预览页码"
// @Param page_size query int false "50 或 100"
// @Success 200 {object} spreadsheet.SpreadsheetDocument
// @Router /api/v1/customers/export/preview [get]
func (h *Handler) exportPreview(c *echo.Context) error {
	doc, err := h.buildExportDocument(c, true)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, doc)
}

// @Summary 导出客户资料 XLSX
// @Tags 客户
// @Security BearerAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param scope query string true "current 或 all"
// @Param q query string false "关键词"
// @Param filter query string false "all、multiple 或 empty"
// @Success 200 {file} binary
// @Failure 422 {object} ErrorResponse
// @Router /api/v1/customers/export [get]
func (h *Handler) exportFile(c *echo.Context) error {
	doc, err := h.buildExportDocument(c, false)
	if err != nil {
		return err
	}
	if doc.TotalRows == 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "当前范围没有可导出的客户资料")
	}
	data, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), doc)
	if err != nil {
		return err
	}
	return sendXLSX(c, "客户资料.xlsx", data)
}

func (h *Handler) buildExportDocument(c *echo.Context, preview bool) (spreadsheet.SpreadsheetDocument, error) {
	scope := strings.TrimSpace(c.QueryParam("scope"))
	if scope == "" {
		scope = "current"
	}
	if scope != "current" && scope != "all" {
		return spreadsheet.SpreadsheetDocument{}, echo.NewHTTPError(http.StatusBadRequest, "scope 仅支持 current 或 all")
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	filter := strings.TrimSpace(c.QueryParam("filter"))
	if scope == "all" {
		q = ""
		filter = ""
	}
	db := h.DB.Model(&model.CustomerProfile{}).Joins("JOIN customer_codes ON customer_codes.id = customer_profiles.customer_code_id")
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("customer_codes.code LIKE ? OR customer_profiles.short_name LIKE ? OR customer_profiles.name LIKE ? OR customer_profiles.address LIKE ? OR customer_profiles.phone LIKE ? OR customer_profiles.contact_name LIKE ? OR customer_profiles.contact_phone LIKE ? OR customer_profiles.salesperson LIKE ?", like, like, like, like, like, like, like, like)
	}
	if filter == "multiple" {
		db = db.Where("(SELECT COUNT(1) FROM customer_profiles p WHERE p.customer_code_id = customer_profiles.customer_code_id) > 1")
	} else if filter == "empty" {
		return spreadsheet.SpreadsheetDocument{SheetName: "客户资料", Title: "东莞博邦光电科技有限公司 客户资料", Columns: customerColumns, Rows: [][]string{}, Page: 1, PageSize: 50, Empty: true}, nil
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return spreadsheet.SpreadsheetDocument{}, err
	}
	page, size := 1, 50
	if preview {
		if v, _ := strconv.Atoi(c.QueryParam("page")); v > 0 {
			page = v
		}
		if v, _ := strconv.Atoi(c.QueryParam("page_size")); v == 100 {
			size = 100
		}
		db = db.Offset((page - 1) * size).Limit(size)
	}
	var rows []struct {
		ID                                                                            uint
		Code, ShortName, Name, Address, Phone, ContactName, ContactPhone, Salesperson string
	}
	if err := db.Select("customer_profiles.id,customer_codes.code,customer_profiles.short_name,customer_profiles.name,customer_profiles.address,customer_profiles.phone,customer_profiles.contact_name,customer_profiles.contact_phone,customer_profiles.salesperson").Order("CAST(SUBSTR(customer_codes.code, 4) AS INTEGER) asc, customer_profiles.is_default desc, customer_profiles.id asc").Scan(&rows).Error; err != nil {
		return spreadsheet.SpreadsheetDocument{}, err
	}
	values := make([][]string, 0, len(rows))
	for _, r := range rows {
		values = append(values, []string{strconv.FormatUint(uint64(r.ID), 10), r.Code, r.ShortName, r.Name, r.Address, r.Phone, r.ContactName, r.ContactPhone, r.Salesperson})
	}
	return spreadsheet.SpreadsheetDocument{SheetName: "客户资料", Title: "东莞博邦光电科技有限公司 客户资料", Columns: customerColumns, Rows: values, TotalRows: total, Page: page, PageSize: size, Empty: total == 0, HasMore: int64(page*size) < total}, nil
}

func readUpload(c *echo.Context) (*multipart.FileHeader, []byte, error) {
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "请选择 Excel 文件")
	}
	if header.Size > spreadsheet.MaxFileSize {
		return nil, nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "文件不能超过 10 MiB")
	}
	f, err := header.Open()
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, spreadsheet.MaxFileSize+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > spreadsheet.MaxFileSize {
		return nil, nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "文件不能超过 10 MiB")
	}
	return header, data, nil
}

func parseCustomerWorkbook(ctx context.Context, filename string, data []byte) ([]importRow, []spreadsheet.CellError) {
	ext := strings.ToLower(filepath.Ext(filename))
	if err := spreadsheet.ValidateSignature(ext, data); err != nil {
		return nil, []spreadsheet.CellError{{Row: 0, Column: "文件", Value: filename, Reason: "文件扩展名与内容格式不匹配"}}
	}
	reader, err := spreadsheet.DefaultRegistry().Reader(filename)
	if err != nil {
		return nil, []spreadsheet.CellError{{Row: 0, Column: "文件", Value: filename, Reason: "仅支持 .xls 和 .xlsx"}}
	}
	raw, err := reader.Read(ctx, bytes.NewReader(data), spreadsheet.ReadOptions{MaxRows: spreadsheet.DefaultMaxRows + 2, MaxColumns: 16})
	if err != nil {
		return nil, []spreadsheet.CellError{{Row: 0, Column: "文件", Value: filename, Reason: err.Error()}}
	}
	headers := []string{"序号", "客户编码", "客户简称", "客户名称", "地址", "电话", "联系人", "联系人电话", "业务员"}
	headerIndex := -1
	for i := 0; i < len(raw) && i < 10; i++ {
		if len(raw[i]) >= len(headers) {
			ok := true
			for j, h := range headers {
				if strings.TrimSpace(raw[i][j]) != h {
					ok = false
					break
				}
			}
			if ok {
				headerIndex = i
				break
			}
		}
	}
	if headerIndex < 0 {
		return nil, []spreadsheet.CellError{{Row: 0, Column: "表头", Reason: "未找到九列标准表头"}}
	}
	rows := make([]importRow, 0)
	errs := make([]spreadsheet.CellError, 0)
	for i := headerIndex + 1; i < len(raw); i++ {
		r := raw[i]
		cells := make([]string, 9)
		for j := 0; j < 9 && j < len(r); j++ {
			cells[j] = strings.TrimSpace(r[j])
		}
		allEmpty := true
		for _, v := range cells[1:] {
			if v != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}
		code, err := NormalizeCode(cells[1])
		if err != nil {
			errs = append(errs, spreadsheet.CellError{Row: i + 1, Column: "客户编码", Value: cells[1], Reason: err.Error()})
			continue
		}
		rows = append(rows, importRow{Row: i + 1, Code: code, ShortName: cells[2], Name: cells[3], Address: cells[4], Phone: cells[5], ContactName: cells[6], ContactPhone: cells[7], Salesperson: cells[8]})
	}
	if len(rows) > spreadsheet.DefaultMaxRows {
		errs = append(errs, spreadsheet.CellError{Row: 0, Column: "文件", Reason: "数据不能超过 10000 条"})
	}
	return rows, errs
}

func (h *Handler) previewRows(rows []importRow) (ImportSummary, []spreadsheet.CellError, error) {
	return h.previewRowsDB(h.DB, rows)
}

func (h *Handler) previewRowsDB(db *gorm.DB, rows []importRow) (ImportSummary, []spreadsheet.CellError, error) {
	summary, errs, _, err := h.previewRowsDBWithSnapshot(db, rows)
	return summary, errs, err
}

const importDBBatchSize = 500

// importCodeState 是一次导入中涉及的客户编码数据库快照。预览和提交都基于
// 同一批量快照计算默认资料，避免在每一行上反复 First/Count。
type importCodeState struct {
	ID           uint
	ProfileCount int
}

type importProfileSnapshot struct {
	CustomerCodeID uint   `gorm:"column:customer_code_id"`
	ShortName      string `gorm:"column:short_name"`
	Name           string `gorm:"column:name"`
	Address        string `gorm:"column:address"`
	Phone          string `gorm:"column:phone"`
	ContactName    string `gorm:"column:contact_name"`
	ContactPhone   string `gorm:"column:contact_phone"`
	Salesperson    string `gorm:"column:salesperson"`
}

type importDatabaseSnapshot struct {
	Codes       map[string]importCodeState
	ProfileKeys map[string]struct{}
}

// previewRowsDBWithSnapshot 一次批量加载所有涉及的编码和现有资料，然后在
// 内存中完成文件内重复判断、数据库重复判断和默认资料计算。codes/profile
// 查询按固定批次切分，兼容 SQLite 的变量数量上限，同时避免逐编码/逐资料
// 的查询放大。
func (h *Handler) previewRowsDBWithSnapshot(db *gorm.DB, rows []importRow) (ImportSummary, []spreadsheet.CellError, importDatabaseSnapshot, error) {
	s := ImportSummary{TotalRows: len(rows)}
	errs := []spreadsheet.CellError{}
	groups := map[string]int{}
	seen := map[string]int{}
	invalidRows := map[int]struct{}{}
	codeOrder := make([]string, 0)
	for _, r := range rows {
		if _, ok := groups[r.Code]; !ok {
			codeOrder = append(codeOrder, r.Code)
		}
		groups[r.Code]++
		key := rowKey(r)
		if first, ok := seen[key]; ok {
			errs = append(errs, spreadsheet.CellError{Row: r.Row, Column: "客户资料", Reason: fmt.Sprintf("与第 %d 行完全重复", first)})
			invalidRows[r.Row] = struct{}{}
		} else {
			seen[key] = r.Row
		}
	}
	snapshot, err := loadImportDatabaseSnapshot(db, codeOrder)
	if err != nil {
		return s, nil, importDatabaseSnapshot{}, err
	}
	for code, count := range groups {
		if count > 1 {
			s.MultipleCodeGroups++
		}
		if _, ok := snapshot.Codes[code]; !ok {
			s.NewCodes++
		}
	}
	for _, r := range rows {
		if _, ok := snapshot.ProfileKeys[rowKey(r)]; ok {
			errs = append(errs, spreadsheet.CellError{Row: r.Row, Column: "客户资料", Reason: "数据库中已存在完全相同的资料"})
			invalidRows[r.Row] = struct{}{}
		}
	}
	s.NewProfiles = len(rows) - len(invalidRows)
	return s, errs, snapshot, nil
}

func loadImportDatabaseSnapshot(db *gorm.DB, codeOrder []string) (importDatabaseSnapshot, error) {
	snapshot := importDatabaseSnapshot{
		Codes:       make(map[string]importCodeState, len(codeOrder)),
		ProfileKeys: make(map[string]struct{}),
	}
	if len(codeOrder) == 0 {
		return snapshot, nil
	}
	codeIDs := make(map[uint]string, len(codeOrder))
	for start := 0; start < len(codeOrder); start += importDBBatchSize {
		end := start + importDBBatchSize
		if end > len(codeOrder) {
			end = len(codeOrder)
		}
		var codes []model.CustomerCode
		if err := db.Where("code IN ?", codeOrder[start:end]).Find(&codes).Error; err != nil {
			return importDatabaseSnapshot{}, err
		}
		for _, code := range codes {
			snapshot.Codes[code.Code] = importCodeState{ID: code.ID}
			codeIDs[code.ID] = code.Code
		}
	}
	if len(codeIDs) == 0 {
		return snapshot, nil
	}
	ids := make([]uint, 0, len(codeIDs))
	for id := range codeIDs {
		ids = append(ids, id)
	}
	for start := 0; start < len(ids); start += importDBBatchSize {
		end := start + importDBBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		var profiles []importProfileSnapshot
		if err := db.Model(&model.CustomerProfile{}).
			Select("customer_code_id, short_name, name, address, phone, contact_name, contact_phone, salesperson").
			Where("customer_code_id IN ?", ids[start:end]).
			Scan(&profiles).Error; err != nil {
			return importDatabaseSnapshot{}, err
		}
		for _, profile := range profiles {
			code, ok := codeIDs[profile.CustomerCodeID]
			if !ok {
				continue
			}
			state := snapshot.Codes[code]
			state.ProfileCount++
			snapshot.Codes[code] = state
			snapshot.ProfileKeys[rowKey(importRow{
				Code: code, ShortName: profile.ShortName, Name: profile.Name,
				Address: profile.Address, Phone: profile.Phone,
				ContactName: profile.ContactName, ContactPhone: profile.ContactPhone,
				Salesperson: profile.Salesperson,
			})] = struct{}{}
		}
	}
	return snapshot, nil
}

// persistImportRows 在同一个调用方事务内先批量创建缺失编码，再批量创建
// 资料。新编码的第一条资料成为默认，已有编码的 ProfileCount 从数据库快照
// 开始，因此已有默认资料不会被覆盖；同编码多行会保留为多条资料。
func persistImportRows(db *gorm.DB, rows []importRow, snapshot importDatabaseSnapshot) (int, error) {
	newCodes := make([]model.CustomerCode, 0)
	for _, row := range rows {
		if _, ok := snapshot.Codes[row.Code]; ok {
			continue
		}
		newCodes = append(newCodes, model.CustomerCode{Code: row.Code})
		snapshot.Codes[row.Code] = importCodeState{}
	}
	if len(newCodes) > 0 {
		if err := db.CreateInBatches(&newCodes, importDBBatchSize).Error; err != nil {
			return 0, err
		}
		for _, code := range newCodes {
			state := snapshot.Codes[code.Code]
			state.ID = code.ID
			snapshot.Codes[code.Code] = state
		}
	}
	profiles := make([]model.CustomerProfile, 0, len(rows))
	for _, row := range rows {
		state, ok := snapshot.Codes[row.Code]
		if !ok || state.ID == 0 {
			return 0, fmt.Errorf("customer code %q was not loaded", row.Code)
		}
		profiles = append(profiles, model.CustomerProfile{
			CustomerCodeID: state.ID,
			ShortName:      row.ShortName,
			Name:           row.Name,
			Address:        row.Address,
			Phone:          row.Phone,
			ContactName:    row.ContactName,
			ContactPhone:   row.ContactPhone,
			Salesperson:    row.Salesperson,
			IsDefault:      state.ProfileCount == 0,
		})
		state.ProfileCount++
		snapshot.Codes[row.Code] = state
	}
	if len(profiles) > 0 {
		if err := db.CreateInBatches(&profiles, importDBBatchSize).Error; err != nil {
			return 0, err
		}
	}
	return len(newCodes), nil
}

func rowKey(r importRow) string {
	return strings.Join([]string{r.Code, r.ShortName, r.Name, r.Address, r.Phone, r.ContactName, r.ContactPhone, r.Salesperson}, "\x1f")
}
func newToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	t := hex.EncodeToString(b)
	return t, hashToken(t), nil
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func sendXLSX(c *echo.Context, name string, data []byte) error {
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(name)))
	return c.Blob(http.StatusOK, spreadsheet.XLSXWriter{}.ContentType(), data)
}
