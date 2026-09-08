package mold

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const moldImportModule = "molds"
const MaxPackageSize int64 = 2 << 30
const maxMoldArchiveEntries = 2000
const maxMoldExpandedBytes uint64 = 4 << 30
const maxMoldWorkbookBytes uint64 = 64 << 20
const maxMoldLocationsBytes uint64 = 4 << 20
const maxMoldCorrectionsBytes = 4 << 20
const maxMoldStagedAssets = 5000

var moldColumns = []spreadsheet.Column{
	{Key: "id", Title: "序号", Width: 8, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
	{Key: "mold_number", Title: "模具编号", Width: 22, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "model", Title: "模具型号", Width: 24, Type: spreadsheet.CellTypeText},
	{Key: "mold_type", Title: "模具类型", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "location", Title: "模具位置", Width: 14, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "common_group_no", Title: "共模组号", Width: 16, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "image_count", Title: "图片总数", Width: 12, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
	{Key: "remark", Title: "备注", Width: 36, Type: spreadsheet.CellTypeText},
}

type MoldImportPreviewResult struct {
	Token      string                  `json:"token,omitempty"`
	ExpiresAt  *time.Time              `json:"expires_at,omitempty"`
	Summary    MoldImportSummary       `json:"summary"`
	Errors     []spreadsheet.CellError `json:"errors"`
	Unresolved []MoldImportFile        `json:"unresolved"`
}
type MoldImportSummary struct {
	Molds      int  `json:"molds"`
	Images     int  `json:"images"`
	Drawings   int  `json:"drawings"`
	Locations  int  `json:"locations"`
	Unresolved int  `json:"unresolved"`
	Replaced   bool `json:"replaced"`
}
type MoldImportFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
}
type ImportCorrection struct {
	Codes    []string `json:"codes"`
	Category string   `json:"category"`
}
type MoldImportResult struct {
	Molds       int       `json:"molds"`
	Images      int       `json:"images"`
	Drawings    int       `json:"drawings"`
	CompletedAt time.Time `json:"completed_at"`
}
type packageData struct {
	Rows       []Input
	Locations  []model.MoldLocation
	Images     []packageAsset
	Drawings   []packageAsset
	Unresolved []packageAsset
	Errors     []spreadsheet.CellError
}
type packageAsset struct {
	Entry    *zip.File
	Path     string
	Codes    []string
	Category string
	Name     string
}

// ImportTemplate 下载可直接回导的模具 ZIP 模板。
// @Summary 下载模具导入模板
// @Description 返回 `博邦模具导入模板.zip`，包含单模与共模示例、默认位置字典和 images/drawings 标准目录。
// @Tags mold
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/molds/import-template [get]
func (h *Handler) ImportTemplate(c *echo.Context) error {
	data, err := buildMoldImportTemplate(c.Request().Context())
	if err != nil {
		return err
	}
	return sendMoldDownload(c, moldImportTemplateFilename, "application/zip", bytes.NewReader(data), int64(len(data)))
}

const moldImportTemplateFilename = "博邦模具导入模板.zip"

// buildMoldImportTemplate 生成与正式模具资料包相同目录规范的空模板。
// 模板包含单模和共模示例记录，用户可直接替换 molds.xlsx 内容并向预留目录添加图片、DWG 文件后提交导入。
func buildMoldImportTemplate(ctx context.Context) ([]byte, error) {
	templateMolds := []model.Mold{
		{MoldNumber: "MOLD-001", Model: "示例产品", MoldType: model.MoldTypeSingle},
		{MoldNumber: "MOLD-002", Model: "示例共模 A", MoldType: model.MoldTypeCommon, CommonGroupNo: "TEMPLATE-GROUP"},
		{MoldNumber: "MOLD-003", Model: "示例共模 B", MoldType: model.MoldTypeCommon, CommonGroupNo: "TEMPLATE-GROUP"},
	}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(ctx, spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Title: "博邦模具", Columns: moldColumns,
		Rows: [][]string{
			{"", "MOLD-001", "示例产品", "单模", "A1-1", "", "0", "示例备注"},
			{"", "MOLD-002", "示例共模 A", "共模", "A1-1", "TEMPLATE-GROUP", "0", "共模示例"},
			{"", "MOLD-003", "示例共模 B", "共模", "A1-1", "TEMPLATE-GROUP", "0", "共模示例"},
		}, TotalRows: 3,
	})
	if err != nil {
		return nil, err
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	if err := addZipBytes(zw, "molds.xlsx", xlsx); err != nil {
		return nil, err
	}
	locations, err := json.MarshalIndent(defaultMoldLocations(), "", "  ")
	if err != nil {
		return nil, err
	}
	if err := addZipBytes(zw, "locations.json", append(locations, '\n')); err != nil {
		return nil, err
	}
	if err := addMoldArchiveDirectories(zw, templateMolds); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

// ImportPreview 预览模具 ZIP 全量资料包。
// @Summary 预览模具资料包
// @Tags mold
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "ZIP 资料包"
// @Success 200 {object} MoldImportPreviewResult
// @Router /api/v1/molds/import/preview [post]
func (h *Handler) ImportPreview(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	temp, hash, size, cleanup, err := receivePackage(c)
	if err != nil {
		return err
	}
	defer cleanup()
	data, err := h.readPackage(temp, size, nil)
	if err != nil {
		return err
	}
	preview := MoldImportPreviewResult{Summary: MoldImportSummary{Replaced: true}, Errors: data.Errors, Unresolved: unresolvedFiles(data)}
	preview.Summary = packageSummary(data)
	if len(preview.Errors) == 0 {
		token, tokenHash, tokenErr := newImportToken()
		if tokenErr != nil {
			return tokenErr
		}
		expires := time.Now().Add(30 * time.Minute)
		if err := h.DB.Create(&model.ImportSession{UserID: current.ID, Module: moldImportModule, FileHash: hash, TokenHash: tokenHash, ExpiresAt: expires}).Error; err != nil {
			return err
		}
		preview.Token, preview.ExpiresAt = token, &expires
	}
	return c.JSON(http.StatusOK, preview)
}

// ImportCommit 提交已经预览的模具 ZIP 全量资料包。
// @Summary 提交模具资料包
// @Tags mold
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "ZIP 资料包"
// @Param token formData string true "预览令牌"
// @Param corrections formData string false "JSON 格式的图片人工修正"
// @Success 201 {object} MoldImportResult
// @Router /api/v1/molds/import/commit [post]
func (h *Handler) ImportCommit(c *echo.Context) error {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "缺少预览令牌")
	}
	temp, hash, size, cleanup, err := receivePackage(c)
	if err != nil {
		return err
	}
	defer cleanup()
	correctionsRaw := c.FormValue("corrections")
	if len(correctionsRaw) > maxMoldCorrectionsBytes {
		return echo.NewHTTPError(http.StatusBadRequest, "人工修正参数超过 4 MiB")
	}
	corrections, err := parseImportCorrections(correctionsRaw)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "人工修正参数无效")
	}
	data, err := h.readPackage(temp, size, corrections)
	if err != nil {
		return err
	}
	if len(data.Errors) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "文件校验失败，请重新预览")
	}
	if len(data.Unresolved) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "仍有未修正的图片")
	}
	var session model.ImportSession
	if err := h.DB.Where("token_hash = ? AND user_id = ? AND module = ?", hashToken(token), current.ID, moldImportModule).First(&session).Error; err != nil {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌无效，请重新预览")
	}
	if session.ConsumedAt != nil || time.Now().After(session.ExpiresAt) || session.FileHash != hash {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效、文件不一致或已使用")
	}
	paths, err := h.stageAssets(data)
	if err != nil {
		var validationErr *filemodule.ValidationError
		if errors.As(err, &validationErr) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return err
	}
	unlock := filemodule.LockMoldAssetMutation()
	defer unlock()
	oldPaths, err := h.moldStoredPaths()
	if err != nil {
		cleanupStaged(h.StorageRoot, paths)
		return err
	}
	result := MoldImportResult{Molds: len(data.Rows), Images: len(data.Images), Drawings: len(data.Drawings), CompletedAt: time.Now()}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		consume := tx.Model(&model.ImportSession{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", session.ID, result.CompletedAt).Update("consumed_at", result.CompletedAt)
		if consume.Error != nil || consume.RowsAffected != 1 {
			return errors.New("invalid import token")
		}
		if err := replaceMoldData(tx, data, paths, current.ID); err != nil {
			return err
		}
		return filemodule.QueueCleanupTasks(tx, oldPaths)
	})
	if err != nil {
		cleanupStaged(h.StorageRoot, paths)
		return err
	}
	filemodule.CleanupStoredPaths(h.StorageRoot, h.DB, oldPaths)
	return c.JSON(http.StatusCreated, result)
}

// Export 导出模具全量资料 ZIP。
// @Summary 导出模具资料包
// @Tags mold
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/molds/export [get]
func (h *Handler) Export(c *echo.Context) error {
	if h.DB == nil || h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "模具资料包服务未配置")
	}
	unlock := filemodule.LockMoldAssetMutation()
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()

	var molds []model.Mold
	if err := h.DB.Preload("Location").Order("id asc").Find(&molds).Error; err != nil {
		return err
	}
	var locations []model.MoldLocation
	if err := h.DB.Order("code asc, id asc").Find(&locations).Error; err != nil {
		return err
	}
	rows := make([][]string, 0, len(molds))
	for _, item := range molds {
		var count int64
		if err := h.DB.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Count(&count).Error; err != nil {
			return err
		}
		rows = append(rows, []string{strconv.FormatUint(uint64(item.ID), 10), item.MoldNumber, item.Model, moldTypeLabel(item.MoldType), item.Location.Code, item.CommonGroupNo, strconv.FormatInt(count, 10), item.Remark})
	}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), spreadsheet.SpreadsheetDocument{SheetName: "模具", Title: "博邦模具", Columns: moldColumns, Rows: rows, TotalRows: int64(len(rows))})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "bb-molds-export-*.zip")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	zw := zip.NewWriter(temp)
	if err := addZipBytes(zw, "molds.xlsx", xlsx); err != nil {
		temp.Close()
		return err
	}
	locationData, _ := json.Marshal(locations)
	if err := addZipBytes(zw, "locations.json", locationData); err != nil {
		temp.Close()
		return err
	}
	if err := addMoldArchiveDirectories(zw, molds); err != nil {
		temp.Close()
		return err
	}
	imageByMold := make(map[string][]model.ImageFile, len(molds))
	drawingByMold := make(map[string][]model.MoldDrawing, len(molds))
	for _, item := range molds {
		var images []model.ImageFile
		if err := h.DB.Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Order("category asc, sort_order asc, id asc").Find(&images).Error; err != nil {
			temp.Close()
			return err
		}
		imageByMold[item.MoldNumber] = images
		var drawings []model.MoldDrawing
		if err := h.DB.Where("mold_id = ?", item.ID).Order("id asc").Find(&drawings).Error; err != nil {
			temp.Close()
			return err
		}
		drawingByMold[item.MoldNumber] = drawings
	}
	groups, groupedMolds, err := moldArchiveGroups(molds)
	if err != nil {
		temp.Close()
		return err
	}
	archiveNames := map[string]struct{}{}
	for _, item := range molds {
		if _, grouped := groupedMolds[item.MoldNumber]; grouped {
			continue
		}
		if err := exportMoldAssets(zw, h.StorageRoot, item.MoldNumber, item.MoldNumber, imageByMold[item.MoldNumber], drawingByMold[item.MoldNumber], archiveNames); err != nil {
			temp.Close()
			return err
		}
	}
	for _, group := range groups {
		if err := exportMoldGroupAssets(zw, h.StorageRoot, group, imageByMold, drawingByMold, archiveNames); err != nil {
			temp.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// The ZIP is now independent of stored assets; downloading must not block writes.
	unlock()
	locked = false

	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return sendMoldDownload(c, "博邦模具资料包.zip", "application/zip", f, info.Size())
}

// The caller owns the reader and must keep it open through asset staging.
func (h *Handler) readPackage(f io.ReaderAt, size int64, corrections map[string]ImportCorrection) (packageData, error) {
	zr, err := zip.NewReader(f, size)
	if err != nil {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "ZIP 资料包无效")
	}
	files := map[string]*zip.File{}
	if len(zr.File) > maxMoldArchiveEntries {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("资料包文件数量超过 %d 个，请拆分后导入", maxMoldArchiveEntries))
	}
	var declaredExpandedBytes uint64
	for _, item := range zr.File {
		if item.UncompressedSize64 > maxMoldExpandedBytes-declaredExpandedBytes {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包解压后的文件总量超过 4 GiB，请拆分后导入")
		}
		declaredExpandedBytes += item.UncompressedSize64
		rawName := filepath.ToSlash(item.Name)
		isDirectory := strings.HasSuffix(rawName, "/")
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSuffix(rawName, "/")))
		if isDirectory {
			clean += "/"
		}
		if rawName != clean || clean == "./" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包包含非法路径")
		}
		if _, ok := files[clean]; ok {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包包含重复文件")
		}
		files[clean] = item
	}
	main := files["molds.xlsx"]
	if main == nil {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "缺少 molds.xlsx")
	}
	if main.UncompressedSize64 > maxMoldWorkbookBytes {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "molds.xlsx 超过 64 MiB，请减少数据后重试")
	}
	reader, err := main.Open()
	if err != nil {
		return packageData{}, err
	}
	raw, err := spreadsheet.XLSXReader{}.Read(context.Background(), reader, spreadsheet.ReadOptions{MaxRows: spreadsheet.DefaultMaxRows + 1, MaxColumns: 16})
	reader.Close()
	if err != nil {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "读取 molds.xlsx 失败")
	}
	data := packageData{}
	data.Rows, data.Errors = parseMoldRows(raw)
	data.Locations, err = parseLocations(files["locations.json"])
	if err != nil {
		return packageData{}, err
	}
	locationCodes := map[string]bool{}
	for _, location := range data.Locations {
		locationCodes[location.Code] = true
	}
	for _, row := range data.Rows {
		if !locationCodes[row.LocationCode] {
			data.Errors = append(data.Errors, importError(row.MoldNumber, "模具位置不在 locations.json 中"))
		}
	}
	known := map[string]bool{}
	moldInputs := map[string]Input{}
	for _, row := range data.Rows {
		known[row.MoldNumber] = true
		moldInputs[row.MoldNumber] = row
	}
	for path, item := range files {
		if item.FileInfo().IsDir() || path == "molds.xlsx" || path == "locations.json" {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			data.Errors = append(data.Errors, importError(path, "资料包文件路径不符合模板"))
			continue
		}
		switch parts[0] {
		case "images":
			if err := validateImageEntry(item); err != nil {
				data.Errors = append(data.Errors, importError(path, err.Error()))
				continue
			}
			asset, ok := parseImageAsset(item, parts, known)
			if strings.Contains(parts[1], "+") && !isCanonicalImagePath(parts, known) {
				asset, ok = parseGroupedImageAsset(item, parts, moldInputs)
				if !ok && legacySharedOuterAllowed(parts[1], known, moldInputs) {
					asset, ok = parseImageAsset(item, parts, known)
				}
			}
			if !ok {
				data.Errors = append(data.Errors, importError(path, "图片无法匹配模具编号或图片分组"))
			} else if correction, exists := corrections[path]; exists {
				asset, ok = applyImageCorrection(asset, correction, known)
				if !ok {
					data.Errors = append(data.Errors, importError(path, "图片人工修正无效"))
				} else {
					data.Images = append(data.Images, asset)
				}
			} else if asset.Category == "" || len(asset.Codes) == 0 {
				data.Unresolved = append(data.Unresolved, asset)
			} else {
				data.Images = append(data.Images, asset)
			}
		case "drawings":
			asset, ok := parseDrawingAsset(item, parts, known)
			if strings.Contains(parts[1], "+") && !isCanonicalDrawingPath(parts, known) {
				asset, ok = parseGroupedDrawingAsset(item, parts, moldInputs)
				if !ok && legacySharedOuterAllowed(parts[1], known, moldInputs) {
					asset, ok = parseDrawingAsset(item, parts, known)
				}
			}
			if !ok {
				data.Errors = append(data.Errors, importError(path, "图纸无法匹配模具编号"))
			} else {
				data.Drawings = append(data.Drawings, asset)
			}
		default:
			data.Errors = append(data.Errors, importError(path, "资料包包含未识别文件"))
		}
	}
	sort.SliceStable(data.Images, func(i, j int) bool {
		return naturalAssetLess(data.Images[i].Name, data.Images[j].Name)
	})
	return data, nil
}

func parseMoldRows(raw [][]string) ([]Input, []spreadsheet.CellError) {
	headers := []string{"序号", "模具编号", "模具型号", "模具类型", "模具位置", "共模组号", "图片总数", "备注"}
	header := -1
	for i, row := range raw {
		if len(row) < len(headers) {
			continue
		}
		match := true
		for j, want := range headers {
			if strings.TrimSpace(row[j]) != want {
				match = false
				break
			}
		}
		if match {
			header = i
			break
		}
	}
	if header < 0 {
		return nil, []spreadsheet.CellError{{Row: 0, Column: "表头", Reason: "未找到八列标准表头"}}
	}
	rows := make([]Input, 0)
	errs := make([]spreadsheet.CellError, 0)
	seen := map[string]bool{}
	for i := header + 1; i < len(raw); i++ {
		cells := make([]string, len(headers))
		for j := range cells {
			if j < len(raw[i]) {
				cells[j] = strings.TrimSpace(raw[i][j])
			}
		}
		if cells[1] == "" && cells[2] == "" {
			continue
		}
		typ := cells[3]
		if typ == "共模" {
			typ = model.MoldTypeCommon
		}
		if typ == "单模" {
			typ = model.MoldTypeSingle
		}
		input := Input{MoldNumber: cells[1], Model: cells[2], MoldType: typ, LocationCode: cells[4], CommonGroupNo: cells[5], Remark: cells[7]}
		if input.MoldNumber == "" || input.Model == "" || input.LocationCode == "" {
			errs = append(errs, rowError(i+1, "模具编号/模具型号/模具位置", "不能为空"))
			continue
		}
		if seen[input.MoldNumber] {
			errs = append(errs, rowError(i+1, "模具编号", "重复"))
			continue
		}
		seen[input.MoldNumber] = true
		if err := validateInput(normalizeInput(input)); err != nil {
			errs = append(errs, rowError(i+1, "模具类型/共模组号", err.Error()))
			continue
		}
		rows = append(rows, input)
	}
	return rows, errs
}

func parseLocations(item *zip.File) ([]model.MoldLocation, error) {
	result := defaultMoldLocations()
	if item != nil {
		if item.UncompressedSize64 > maxMoldLocationsBytes {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "locations.json 超过 4 MiB")
		}
		if r, err := item.Open(); err == nil {
			defer r.Close()
			var input []model.MoldLocation
			if err := json.NewDecoder(r).Decode(&input); err != nil {
				return nil, echo.NewHTTPError(http.StatusBadRequest, "读取 locations.json 失败")
			}
			result = input
		} else {
			return nil, err
		}
	}
	if !hasPalletLocation(result) {
		result = append(result, model.MoldLocation{Code: model.MoldLocationPallet, Status: model.MoldLocationActive})
	}
	return result, nil
}

func hasPalletLocation(locations []model.MoldLocation) bool {
	for _, location := range locations {
		if strings.TrimSpace(location.Code) == model.MoldLocationPallet {
			return true
		}
	}
	return false
}

func parseImageAsset(item *zip.File, parts []string, known map[string]bool) (packageAsset, bool) {
	if !validImageEntry(item) {
		return packageAsset{}, false
	}
	if len(parts) >= 4 {
		code, category := parts[1], normalizeCategory(parts[2])
		if known[code] && category != "" {
			return packageAsset{Entry: item, Path: strings.Join(parts, "/"), Codes: []string{code}, Category: category, Name: parts[len(parts)-1]}, true
		}
	}
	name := parts[len(parts)-1]
	category := inferCategory(name)
	knownCodes := make([]string, 0, len(known))
	for code := range known {
		knownCodes = append(knownCodes, code)
	}
	codes := moldNumbersInName(name, knownCodes)
	if len(codes) == 0 {
		return packageAsset{Entry: item, Path: strings.Join(parts, "/"), Name: name}, true
	}
	// 无分组关键词但能匹配模具编号时，按客户资料图的常见命名约定归入产品材料。
	if category == "" {
		category = "product_material"
	}
	sort.Strings(codes)
	return packageAsset{Entry: item, Path: strings.Join(parts, "/"), Codes: codes, Category: category, Name: name}, true
}

func isCanonicalImagePath(parts []string, known map[string]bool) bool {
	return len(parts) >= 4 && known[parts[1]] && normalizeCategory(parts[2]) != ""
}

// parseGroupedImageAsset 解析共模组外层目录，具体编号目录只归属该编号，平铺文件按文件名匹配组内编号。
func parseGroupedImageAsset(item *zip.File, parts []string, known map[string]Input) (packageAsset, bool) {
	if len(parts) < 3 || !validImageEntry(item) {
		return packageAsset{}, false
	}
	members, ok := validSharedMembers(parts[1], known)
	if !ok {
		return packageAsset{}, false
	}
	asset := packageAsset{Entry: item, Path: strings.Join(parts, "/"), Name: parts[len(parts)-1]}
	switch len(parts) {
	case 3: // images/<group>/<file>
		asset.Codes = moldNumbersInName(asset.Name, members)
		asset.Category = inferCategory(asset.Name)
	case 4:
		if parts[2] == "共用" {
			asset.Codes = members
			asset.Category = inferCategory(asset.Name)
		} else if category := normalizeCategory(parts[2]); category != "" {
			asset.Codes = moldNumbersInName(asset.Name, members)
			asset.Category = category
		} else {
			return packageAsset{}, false
		}
	case 5: // images/<group>/<共用|具体编号>/<category>/<file>
		asset.Category = normalizeCategory(parts[3])
		if asset.Category == "" {
			return packageAsset{}, false
		}
		switch parts[2] {
		case "共用":
			asset.Codes = members
		default:
			if !containsString(members, parts[2]) {
				return packageAsset{}, false
			}
			asset.Codes = []string{parts[2]}
		}
	default:
		return packageAsset{}, false
	}
	if asset.Category == "" && len(asset.Codes) > 0 {
		// 共模平铺文件沿用旧资料包的兜底约定：能精确匹配编号但未写分类关键词时，归入产品材料。
		asset.Category = "product_material"
	}
	return asset, true
}

func parseDrawingAsset(item *zip.File, parts []string, known map[string]bool) (packageAsset, bool) {
	if len(parts) >= 3 && known[parts[1]] && allowedDrawingExt(filepath.Ext(parts[len(parts)-1])) {
		return packageAsset{Entry: item, Path: strings.Join(parts, "/"), Codes: []string{parts[1]}, Name: parts[len(parts)-1]}, true
	}
	return packageAsset{}, false
}

func isCanonicalDrawingPath(parts []string, known map[string]bool) bool {
	return len(parts) == 3 && known[parts[1]] && allowedDrawingExt(filepath.Ext(parts[len(parts)-1]))
}

// parseGroupedDrawingAsset 解析共模组图纸目录和平铺图纸。
func parseGroupedDrawingAsset(item *zip.File, parts []string, known map[string]Input) (packageAsset, bool) {
	if len(parts) < 3 || item == nil || item.UncompressedSize64 == 0 || !allowedDrawingExt(filepath.Ext(parts[len(parts)-1])) {
		return packageAsset{}, false
	}
	members, ok := validSharedMembers(parts[1], known)
	if !ok {
		return packageAsset{}, false
	}
	asset := packageAsset{Entry: item, Path: strings.Join(parts, "/"), Name: parts[len(parts)-1]}
	switch len(parts) {
	case 3: // drawings/<group>/<file>
		asset.Codes = moldNumbersInName(asset.Name, members)
	case 4: // drawings/<group>/<共用|具体编号>/<file>
		switch parts[2] {
		case "共用":
			asset.Codes = members
		default:
			if !containsString(members, parts[2]) {
				return packageAsset{}, false
			}
			asset.Codes = []string{parts[2]}
		}
	default:
		return packageAsset{}, false
	}
	return asset, len(asset.Codes) > 0
}

func validSharedMembers(value string, known map[string]Input) ([]string, bool) {
	parts := strings.Split(value, "+")
	if len(parts) < 2 {
		return nil, false
	}
	seen := make(map[string]bool, len(parts))
	var group string
	for _, code := range parts {
		if code == "" || seen[code] {
			return nil, false
		}
		input, ok := known[code]
		if !ok || input.MoldType != model.MoldTypeCommon || strings.TrimSpace(input.CommonGroupNo) == "" {
			return nil, false
		}
		if group == "" {
			group = strings.TrimSpace(input.CommonGroupNo)
		} else if group != strings.TrimSpace(input.CommonGroupNo) {
			return nil, false
		}
		seen[code] = true
	}
	expected := make([]string, 0, len(parts))
	for code, input := range known {
		if input.MoldType == model.MoldTypeCommon && strings.TrimSpace(input.CommonGroupNo) == group {
			expected = append(expected, code)
		}
	}
	sort.SliceStable(expected, func(i, j int) bool { return naturalAssetLess(expected[i], expected[j]) })
	if len(parts) != len(expected) {
		return nil, false
	}
	for _, code := range expected {
		if !seen[code] {
			return nil, false
		}
	}
	// 导出使用稳定自然序，导入则按集合校验，兼容人工整理目录时的任意成员顺序。
	return expected, true
}

func legacySharedOuterAllowed(value string, known map[string]bool, inputs map[string]Input) bool {
	parts := strings.Split(value, "+")
	if len(parts) < 2 {
		return false
	}
	for _, code := range parts {
		if !known[code] {
			return false
		}
		if input, ok := inputs[code]; ok && input.MoldType == model.MoldTypeCommon {
			return false
		}
	}
	return true
}

func moldNumbersInName(name string, members []string) []string {
	// Match longer known numbers first so AB cannot consume part of AB-CD.
	ordered := append([]string(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	lower := strings.ToLower(name)
	used := make([]bool, len(lower))
	codes := make([]string, 0, len(members))
	for _, member := range ordered {
		code := strings.ToLower(member)
		if code == "" {
			continue
		}
		matched := false
		for start := 0; start < len(lower); {
			index := strings.Index(lower[start:], code)
			if index < 0 {
				break
			}
			index += start
			end := index + len(code)
			available := (index == 0 || !isASCIILetterOrDigit(lower[index-1])) && (end == len(lower) || !isASCIILetterOrDigit(lower[end]))
			for i := index; i < end && available; i++ {
				available = !used[i]
			}
			if available {
				matched = true
				for i := index; i < end; i++ {
					used[i] = true
				}
			}
			start = index + 1
		}
		if matched {
			codes = append(codes, member)
		}
	}
	sort.SliceStable(codes, func(i, j int) bool { return naturalAssetLess(codes[i], codes[j]) })
	return codes
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeCategory(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "product_material", "产品材料", "产品图", "材质":
		return "product_material"
	case "supplement", "补充图":
		return "supplement"
	}
	return ""
}

func parseImportCorrections(raw string) (map[string]ImportCorrection, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if len(raw) > maxMoldCorrectionsBytes {
		return nil, errors.New("人工修正参数超过 4 MiB")
	}
	var result map[string]ImportCorrection
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func applyImageCorrection(asset packageAsset, correction ImportCorrection, known map[string]bool) (packageAsset, bool) {
	category := normalizeCategory(correction.Category)
	if category == "" {
		return packageAsset{}, false
	}
	codes := make([]string, 0, len(correction.Codes))
	seen := map[string]bool{}
	for _, code := range correction.Codes {
		code = strings.TrimSpace(code)
		if code == "" || !known[code] || seen[code] {
			return packageAsset{}, false
		}
		seen[code] = true
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return packageAsset{}, false
	}
	sort.Strings(codes)
	asset.Codes, asset.Category = codes, category
	return asset, true
}

func inferCategory(name string) string {
	for _, word := range []string{"产品材料", "产品图", "材质"} {
		if strings.Contains(name, word) {
			return "product_material"
		}
	}
	for _, word := range []string{"前模", "后模", "开模", "尺寸", "局部", "原理图"} {
		if strings.Contains(name, word) {
			return "supplement"
		}
	}
	return ""
}
func allowedDrawingExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".dwg" || ext == ".fdwg"
}
func exportCategory(category string) string {
	if category == "product_material" {
		return "product_material"
	}
	return "supplement"
}
func naturalAssetLess(left, right string) bool {
	left, right = strings.ToLower(left), strings.ToLower(right)
	for i, j := 0, 0; i < len(left) && j < len(right); {
		if isASCIIDigit(left[i]) && isASCIIDigit(right[j]) {
			iStart, jStart := i, j
			for i < len(left) && isASCIIDigit(left[i]) {
				i++
			}
			for j < len(right) && isASCIIDigit(right[j]) {
				j++
			}
			ln, rn := strings.TrimLeft(left[iStart:i], "0"), strings.TrimLeft(right[jStart:j], "0")
			if ln == "" {
				ln = "0"
			}
			if rn == "" {
				rn = "0"
			}
			if len(ln) != len(rn) {
				return len(ln) < len(rn)
			}
			if ln != rn {
				return ln < rn
			}
			continue
		}
		if left[i] != right[j] {
			return left[i] < right[j]
		}
		i++
		j++
	}
	return len(left) < len(right)
}

func isASCIIDigit(value byte) bool { return value >= '0' && value <= '9' }

func validImageEntry(item *zip.File) bool {
	return item != nil && item.UncompressedSize64 > 0 && item.UncompressedSize64 <= uint64(MaxPackageSize) && filemodule.AllowedImageExtension(filepath.Ext(item.Name))
}

func validateImageEntry(item *zip.File) error {
	if !validImageEntry(item) {
		return errors.New("图片为空、超过单文件安全边界或扩展名不受支持")
	}
	return filemodule.ValidateStaticImage(int64(item.UncompressedSize64), filepath.Ext(item.Name), func() (io.ReadCloser, error) {
		return item.Open()
	})
}

type stagedAsset struct {
	Asset       packageAsset
	Path        string
	Size        int64
	PreviewPath string
	PreviewMime string
	PreviewSize int64
}

func (h *Handler) stageAssets(data packageData) ([]stagedAsset, error) {
	staged := make([]stagedAsset, 0, len(data.Images)+len(data.Drawings))
	var stagedBytes int64
	for _, asset := range append(append([]packageAsset{}, data.Images...), data.Drawings...) {
		for _, code := range asset.Codes {
			if len(staged) >= maxMoldStagedAssets {
				cleanupStaged(h.StorageRoot, staged)
				return nil, fmt.Errorf("资料包展开后的图片和图纸超过 %d 个，请拆分后导入", maxMoldStagedAssets)
			}
			ext := strings.ToLower(filepath.Ext(asset.Name))
			prefix := filepath.Join("mold", "import", time.Now().Format("20060102"))
			if asset.Category != "" {
				prefix = filepath.Join(prefix, asset.Category)
			} else {
				prefix = filepath.Join("mold", "drawings", "import", time.Now().Format("20060102"))
			}
			name, err := randomStorageName(ext)
			if err != nil {
				cleanupStaged(h.StorageRoot, staged)
				return nil, err
			}
			relative := filepath.ToSlash(filepath.Join(prefix, name))
			path := filepath.Join(h.StorageRoot, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				cleanupStaged(h.StorageRoot, staged)
				return nil, err
			}
			src, err := asset.Entry.Open()
			if err != nil {
				cleanupStaged(h.StorageRoot, staged)
				return nil, err
			}
			dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				src.Close()
				cleanupStaged(h.StorageRoot, staged)
				return nil, err
			}
			remaining := int64(maxMoldExpandedBytes) - stagedBytes
			if remaining <= 0 {
				src.Close()
				dst.Close()
				_ = os.Remove(path)
				cleanupStaged(h.StorageRoot, staged)
				return nil, errors.New("资料包实际解压文件总量超过 4 GiB，请拆分后导入")
			}
			readLimit := min(MaxPackageSize, remaining) + 1
			written, copyErr := io.Copy(dst, io.LimitReader(src, readLimit))
			src.Close()
			closeErr := dst.Close()
			if copyErr != nil || closeErr != nil || written > MaxPackageSize || written > remaining {
				_ = os.Remove(path)
				cleanupStaged(h.StorageRoot, staged)
				if copyErr != nil {
					return nil, copyErr
				}
				if closeErr != nil {
					return nil, closeErr
				}
				return nil, errors.New("资料包内文件超过大小限制")
			}
			stagedBytes += written
			if stagedBytes > int64(maxMoldExpandedBytes) {
				_ = os.Remove(path)
				cleanupStaged(h.StorageRoot, staged)
				return nil, errors.New("资料包实际解压文件总量超过 4 GiB，请拆分后导入")
			}
			stagedItem := stagedAsset{Asset: asset, Path: relative, Size: written}
			if asset.Category != "" {
				preview, previewMime, previewErr := filemodule.MakeStaticPreviewFile(path, ext)
				if previewErr != nil {
					_ = os.Remove(path)
					cleanupStaged(h.StorageRoot, staged)
					return nil, fmt.Errorf("图片 %s 无法生成静态预览: %w", asset.Name, previewErr)
				}
				stagedBytes += int64(len(preview))
				if stagedBytes > int64(maxMoldExpandedBytes) {
					_ = os.Remove(path)
					cleanupStaged(h.StorageRoot, staged)
					return nil, errors.New("资料包实际解压文件和预览总量超过 4 GiB，请拆分后导入")
				}
				previewName, previewErr := randomStorageName(".jpg")
				if previewErr != nil {
					_ = os.Remove(path)
					cleanupStaged(h.StorageRoot, staged)
					return nil, previewErr
				}
				previewRelative := filepath.ToSlash(filepath.Join(prefix, "preview-"+previewName))
				previewPath := filepath.Join(h.StorageRoot, filepath.FromSlash(previewRelative))
				previewFile, previewErr := os.OpenFile(previewPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
				if previewErr == nil {
					_, previewErr = previewFile.Write(preview)
					if closePreviewErr := previewFile.Close(); previewErr == nil {
						previewErr = closePreviewErr
					}
				}
				if previewErr != nil {
					_ = os.Remove(path)
					_ = os.Remove(previewPath)
					cleanupStaged(h.StorageRoot, staged)
					return nil, fmt.Errorf("保存图片 %s 的静态预览失败: %w", asset.Name, previewErr)
				}
				stagedItem.PreviewPath, stagedItem.PreviewMime, stagedItem.PreviewSize = previewRelative, previewMime, int64(len(preview))
			}
			staged = append(staged, stagedItem)
			staged[len(staged)-1].Asset.Codes = []string{code}
		}
	}
	return staged, nil
}

func replaceMoldData(tx *gorm.DB, data packageData, staged []stagedAsset, uploadedBy uint) error {
	if err := tx.Unscoped().Where("owner_type = ?", "mold").Delete(&model.ImageFile{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("1 = 1").Delete(&model.MoldDrawing{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("1 = 1").Delete(&model.Mold{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("1 = 1").Delete(&model.MoldLocation{}).Error; err != nil {
		return err
	}
	locations := data.Locations
	if len(locations) == 0 {
		locations = defaultMoldLocations()
	}
	locationIDs := map[string]uint{}
	for _, location := range locations {
		location.ID = 0
		location.Status = locationStatus(location.Status)
		if err := tx.Create(&location).Error; err != nil {
			return err
		}
		locationIDs[location.Code] = location.ID
	}
	moldIDs := map[string]uint{}
	for _, row := range data.Rows {
		locationID := locationIDs[row.LocationCode]
		if locationID == 0 {
			return fmt.Errorf("模具 %s 的位置 %s 不存在", row.MoldNumber, row.LocationCode)
		}
		item := model.Mold{MoldNumber: row.MoldNumber, Model: row.Model, MoldType: row.MoldType, LocationID: locationID, CommonGroupNo: row.CommonGroupNo, Remark: row.Remark}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		moldIDs[row.MoldNumber] = item.ID
	}
	imageOrder := map[string]int{}
	for _, asset := range staged {
		for _, code := range asset.Asset.Codes {
			moldID := moldIDs[code]
			if moldID == 0 {
				return fmt.Errorf("图片 %s 的模具不存在", asset.Asset.Name)
			}
			if asset.Asset.Category != "" {
				image := model.ImageFile{OwnerType: "mold", OwnerID: moldID, UploadedBy: uploadedBy, Category: asset.Asset.Category, SortOrder: imageOrder[code+asset.Asset.Category], OriginalName: filepath.Base(asset.Asset.Name), Size: asset.Size, MimeType: filemodule.ImageMIMEForExtension(filepath.Ext(asset.Asset.Name)), Extension: strings.ToLower(filepath.Ext(asset.Asset.Name)), StoragePath: asset.Path, PreviewPath: asset.PreviewPath, PreviewMime: asset.PreviewMime, PreviewSize: asset.PreviewSize}
				imageOrder[code+asset.Asset.Category]++
				if err := tx.Create(&image).Error; err != nil {
					return err
				}
			} else {
				drawing := model.MoldDrawing{MoldID: moldID, UploadedBy: uploadedBy, OriginalName: filepath.Base(asset.Asset.Name), Size: asset.Size, MimeType: "application/octet-stream", Extension: strings.ToLower(filepath.Ext(asset.Asset.Name)), StoragePath: asset.Path}
				if err := tx.Create(&drawing).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (h *Handler) moldStoredPaths() ([]string, error) {
	var images []model.ImageFile
	if err := h.DB.Where("owner_type = ?", "mold").Select("storage_path", "preview_path").Find(&images).Error; err != nil {
		return nil, err
	}
	var drawings []model.MoldDrawing
	if err := h.DB.Select("storage_path").Find(&drawings).Error; err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(images)+len(drawings))
	for _, image := range images {
		paths = append(paths, image.StoragePath)
		if image.PreviewPath != "" {
			paths = append(paths, image.PreviewPath)
		}
	}
	for _, drawing := range drawings {
		paths = append(paths, drawing.StoragePath)
	}
	return paths, nil
}

func locationStatus(value string) string {
	if value == model.MoldLocationDisabled {
		return value
	}
	return model.MoldLocationActive
}
func cleanupStaged(root string, staged []stagedAsset) {
	for _, item := range staged {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(item.Path)))
		if item.PreviewPath != "" {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(item.PreviewPath)))
		}
	}
}

func firstLocation(ids map[string]uint) uint {
	for _, id := range ids {
		return id
	}
	return 0
}
func packageSummary(data packageData) MoldImportSummary {
	return MoldImportSummary{Molds: len(data.Rows), Images: len(data.Images), Drawings: len(data.Drawings), Locations: len(data.Locations), Unresolved: len(data.Unresolved), Replaced: true}
}

func unresolvedFiles(data packageData) []MoldImportFile {
	items := make([]MoldImportFile, 0, len(data.Unresolved))
	for _, item := range data.Unresolved {
		items = append(items, MoldImportFile{Path: item.Path, Name: item.Name})
	}
	return items
}
func receivePackage(c *echo.Context) (*os.File, string, int64, func(), error) {
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return nil, "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "请选择 ZIP 资料包")
	}
	if header.Size <= 0 || header.Size > MaxPackageSize {
		return nil, "", 0, func() {}, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "资料包不能超过 2 GiB")
	}
	src, err := header.Open()
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	temp, err := os.CreateTemp("", "bb-molds-import-*.zip")
	if err != nil {
		src.Close()
		return nil, "", 0, func() {}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(temp, io.LimitReader(io.TeeReader(src, hash), MaxPackageSize+1))
	src.Close()
	name := temp.Name()
	cleanup := func() { temp.Close(); os.Remove(name) }
	if copyErr != nil || written > MaxPackageSize {
		cleanup()
		if copyErr != nil {
			return nil, "", 0, func() {}, copyErr
		}
		return nil, "", 0, func() {}, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "资料包不能超过 2 GiB")
	}
	return temp, hex.EncodeToString(hash.Sum(nil)), written, cleanup, nil
}
func addZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
func addZipDirectory(zw *zip.Writer, name string) error {
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}
	_, err := zw.Create(name)
	return err
}

type moldArchiveGroup struct {
	Outer   string
	Members []model.Mold
}

type archiveImageKey struct {
	Category string
	Name     string
}

func moldArchiveGroups(molds []model.Mold) ([]moldArchiveGroup, map[string]struct{}, error) {
	byGroup := map[string][]model.Mold{}
	for _, item := range molds {
		if err := validateArchiveSegment(item.MoldNumber); err != nil {
			return nil, nil, err
		}
		if item.MoldType == model.MoldTypeCommon && strings.TrimSpace(item.CommonGroupNo) != "" {
			key := strings.TrimSpace(item.CommonGroupNo)
			byGroup[key] = append(byGroup[key], item)
		}
	}
	groupNames := make([]string, 0, len(byGroup))
	for name := range byGroup {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)
	groups := make([]moldArchiveGroup, 0, len(groupNames))
	grouped := map[string]struct{}{}
	for _, name := range groupNames {
		members := byGroup[name]
		if len(members) < 2 {
			continue
		}
		// "A+B" is a valid historical mold number. It cannot safely be used
		// inside a '+'-joined group folder, so keep such groups in the legacy
		// per-mold layout and let the exact mold-number parser handle them.
		ambiguous := false
		for _, member := range members {
			if strings.Contains(member.MoldNumber, "+") {
				ambiguous = true
				break
			}
		}
		if ambiguous {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].MoldNumber == members[j].MoldNumber {
				return members[i].ID < members[j].ID
			}
			return naturalAssetLess(members[i].MoldNumber, members[j].MoldNumber)
		})
		numbers := make([]string, 0, len(members))
		for _, member := range members {
			numbers = append(numbers, member.MoldNumber)
			grouped[member.MoldNumber] = struct{}{}
		}
		outer := strings.Join(numbers, "+")
		if err := validateArchiveSegment(outer); err != nil {
			return nil, nil, err
		}
		groups = append(groups, moldArchiveGroup{Outer: outer, Members: members})
	}
	return groups, grouped, nil
}

func addMoldArchiveDirectories(zw *zip.Writer, molds []model.Mold) error {
	groups, _, err := moldArchiveGroups(molds)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, directory := range []string{"images/", "drawings/"} {
		if err := addUniqueZipDirectory(zw, directory, seen); err != nil {
			return err
		}
	}
	groupedFolders := map[string]string{}
	for _, group := range groups {
		for _, member := range group.Members {
			groupedFolders[member.MoldNumber] = filepath.ToSlash(filepath.Join(group.Outer, member.MoldNumber))
		}
		for _, directory := range []string{
			filepath.ToSlash(filepath.Join("images", group.Outer, "共用")) + "/",
			filepath.ToSlash(filepath.Join("images", group.Outer, "共用", "product_material")) + "/",
			filepath.ToSlash(filepath.Join("images", group.Outer, "共用", "supplement")) + "/",
			filepath.ToSlash(filepath.Join("drawings", group.Outer, "共用")) + "/",
		} {
			if err := addUniqueZipDirectory(zw, directory, seen); err != nil {
				return err
			}
		}
	}
	for _, item := range molds {
		folder := item.MoldNumber
		if groupedFolder, ok := groupedFolders[item.MoldNumber]; ok {
			folder = groupedFolder
		}
		for _, directory := range []string{
			filepath.ToSlash(filepath.Join("images", folder)) + "/",
			filepath.ToSlash(filepath.Join("images", folder, "product_material")) + "/",
			filepath.ToSlash(filepath.Join("images", folder, "supplement")) + "/",
			filepath.ToSlash(filepath.Join("drawings", folder)) + "/",
		} {
			if err := addUniqueZipDirectory(zw, directory, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func exportMoldAssets(zw *zip.Writer, root, folder, moldNumber string, images []model.ImageFile, drawings []model.MoldDrawing, used map[string]struct{}) error {
	for _, image := range images {
		if err := exportImageAsset(zw, root, filepath.Join("images", folder, exportCategory(image.Category)), image, used); err != nil {
			return err
		}
	}
	for _, drawing := range drawings {
		if err := exportDrawingAsset(zw, root, filepath.Join("drawings", folder), drawing, used); err != nil {
			return err
		}
	}
	return nil
}

func exportMoldGroupAssets(zw *zip.Writer, root string, group moldArchiveGroup, imageByMold map[string][]model.ImageFile, drawingByMold map[string][]model.MoldDrawing, used map[string]struct{}) error {
	imageGroups := map[archiveImageKey]map[string][]model.ImageFile{}
	for _, member := range group.Members {
		for _, image := range imageByMold[member.MoldNumber] {
			key := archiveImageKey{Category: exportCategory(image.Category), Name: archiveFileName(image.OriginalName)}
			if imageGroups[key] == nil {
				imageGroups[key] = map[string][]model.ImageFile{}
			}
			imageGroups[key][member.MoldNumber] = append(imageGroups[key][member.MoldNumber], image)
		}
	}
	imageKeys := make([]archiveImageKey, 0, len(imageGroups))
	for key := range imageGroups {
		imageKeys = append(imageKeys, key)
	}
	sort.SliceStable(imageKeys, func(i, j int) bool {
		if imageKeys[i].Category == imageKeys[j].Category {
			return naturalAssetLess(imageKeys[i].Name, imageKeys[j].Name)
		}
		return imageKeys[i].Category < imageKeys[j].Category
	})
	for _, key := range imageKeys {
		members := imageGroups[key]
		if shared, ok := sharedImageAsset(root, group.Members, members); ok {
			if err := exportImageAsset(zw, root, filepath.Join("images", group.Outer, "共用", key.Category), shared, used); err != nil {
				return err
			}
			continue
		}
		for _, member := range group.Members {
			for _, image := range members[member.MoldNumber] {
				if err := exportImageAsset(zw, root, filepath.Join("images", group.Outer, member.MoldNumber, key.Category), image, used); err != nil {
					return err
				}
			}
		}
	}

	drawingGroups := map[string]map[string][]model.MoldDrawing{}
	for _, member := range group.Members {
		for _, drawing := range drawingByMold[member.MoldNumber] {
			name := archiveFileName(drawing.OriginalName)
			if drawingGroups[name] == nil {
				drawingGroups[name] = map[string][]model.MoldDrawing{}
			}
			drawingGroups[name][member.MoldNumber] = append(drawingGroups[name][member.MoldNumber], drawing)
		}
	}
	drawingNames := make([]string, 0, len(drawingGroups))
	for name := range drawingGroups {
		drawingNames = append(drawingNames, name)
	}
	sort.SliceStable(drawingNames, func(i, j int) bool { return naturalAssetLess(drawingNames[i], drawingNames[j]) })
	for _, name := range drawingNames {
		members := drawingGroups[name]
		if shared, ok := sharedDrawingAsset(root, group.Members, members); ok {
			if err := exportDrawingAsset(zw, root, filepath.Join("drawings", group.Outer, "共用"), shared, used); err != nil {
				return err
			}
			continue
		}
		for _, member := range group.Members {
			for _, drawing := range members[member.MoldNumber] {
				if err := exportDrawingAsset(zw, root, filepath.Join("drawings", group.Outer, member.MoldNumber), drawing, used); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func sharedImageAsset(root string, members []model.Mold, assets map[string][]model.ImageFile) (model.ImageFile, bool) {
	if len(assets) != len(members) {
		return model.ImageFile{}, false
	}
	var first model.ImageFile
	for i, member := range members {
		items := assets[member.MoldNumber]
		if len(items) != 1 {
			return model.ImageFile{}, false
		}
		if i == 0 {
			first = items[0]
			continue
		}
		equal, err := storedFilesEqual(root, first.StoragePath, items[0].StoragePath)
		if err != nil || !equal {
			return model.ImageFile{}, false
		}
	}
	return first, true
}

func sharedDrawingAsset(root string, members []model.Mold, assets map[string][]model.MoldDrawing) (model.MoldDrawing, bool) {
	if len(assets) != len(members) {
		return model.MoldDrawing{}, false
	}
	var first model.MoldDrawing
	for i, member := range members {
		items := assets[member.MoldNumber]
		if len(items) != 1 {
			return model.MoldDrawing{}, false
		}
		if i == 0 {
			first = items[0]
			continue
		}
		equal, err := storedFilesEqual(root, first.StoragePath, items[0].StoragePath)
		if err != nil || !equal {
			return model.MoldDrawing{}, false
		}
	}
	return first, true
}

func exportImageAsset(zw *zip.Writer, root, directory string, image model.ImageFile, used map[string]struct{}) error {
	name := uniqueArchiveFileName(archiveFileName(image.OriginalName), image.ID, directory, used)
	return addStoredFile(zw, root, filepath.ToSlash(filepath.Join(directory, name)), image.StoragePath)
}

func exportDrawingAsset(zw *zip.Writer, root, directory string, drawing model.MoldDrawing, used map[string]struct{}) error {
	name := uniqueArchiveFileName(archiveFileName(drawing.OriginalName), drawing.ID, directory, used)
	return addStoredFile(zw, root, filepath.ToSlash(filepath.Join(directory, name)), drawing.StoragePath)
}

func storedFilesEqual(root string, left, right string) (bool, error) {
	leftHash, err := storedFileHash(root, left)
	if err != nil {
		return false, err
	}
	rightHash, err := storedFileHash(root, right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func storedFileHash(root, relative string) ([sha256.Size]byte, error) {
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	hashValue := sha256.New()
	if _, err := io.Copy(hashValue, file); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], hashValue.Sum(nil))
	return result, nil
}

func validateArchiveSegment(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("模具编号包含非法资料包路径字符")
	}
	return nil
}

func archiveFileName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

func uniqueArchiveFileName(name string, id uint, directory string, used map[string]struct{}) string {
	name = archiveFileName(name)
	path := filepath.ToSlash(filepath.Join(directory, name))
	if _, exists := used[path]; !exists {
		used[path] = struct{}{}
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for suffix := 1; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, id, ext)
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d-%d%s", stem, id, suffix, ext)
		}
		path = filepath.ToSlash(filepath.Join(directory, candidate))
		if _, exists := used[path]; !exists {
			used[path] = struct{}{}
			return candidate
		}
	}
}

func addUniqueZipDirectory(zw *zip.Writer, name string, seen map[string]struct{}) error {
	if _, exists := seen[name]; exists {
		return nil
	}
	seen[name] = struct{}{}
	return addZipDirectory(zw, name)
}
func addStoredFile(zw *zip.Writer, root, archive, relative string) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(filepath.ToSlash(archive))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
func randomStorageName(ext string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw) + ext, nil
}
func newImportToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashToken(token), nil
}
func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func rowError(row int, column, reason string) spreadsheet.CellError {
	return spreadsheet.CellError{Row: row, Column: column, Reason: reason}
}
func importError(path, reason string) spreadsheet.CellError {
	return spreadsheet.CellError{Row: 0, Column: "文件", Value: path, Reason: reason}
}
func moldTypeLabel(value string) string {
	if value == model.MoldTypeCommon {
		return "共模"
	}
	return "单模"
}
func sendMoldDownload(c *echo.Context, name, contentType string, reader io.Reader, size int64) error {
	spreadsheet.DownloadHeaders(c.Response().Header(), name, contentType, size)
	return c.Stream(http.StatusOK, contentType, reader)
}
