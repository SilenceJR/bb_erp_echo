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
	"unicode/utf8"

	"bb_erp_echo/internal/auth"
	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
	"golang.org/x/text/encoding/simplifiedchinese"
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
	Path         string                  `json:"path"`
	Name         string                  `json:"name"`
	Kind         string                  `json:"kind"`
	AllowedCodes []string                `json:"allowed_codes"`
	AllowedMolds []MoldImportAllowedMold `json:"allowed_molds"`
}

// MoldImportAllowedMold is the user-facing candidate for an unresolved asset.
// The code remains the stable MoldNumber accepted by corrections.codes while
// Model is the relationship-archive directory/file identity displayed by the
// client.
type MoldImportAllowedMold struct {
	Code  string `json:"code"`
	Model string `json:"model"`
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
	// AssetMolds records the mold numbers whose archive directory was present
	// in the package.  It is intentionally independent from Images/Drawings:
	// an empty directory is still an explicit replacement request, while a
	// mold without a directory keeps its existing assets during a full import.
	AssetMolds map[string]bool
}
type packageAsset struct {
	Entry        *zip.File
	Path         string
	Codes        []string
	AllowedCodes []string
	AllowedMolds []MoldImportAllowedMold
	Category     string
	Name         string
	Kind         string
}

// ImportTemplate 下载可直接回导的模具 ZIP 模板。
// @Summary 下载模具导入模板
// @Description 返回 `博邦模具导入模板.zip`，包含单模与共模示例、默认位置字典和扁平模具资料目录。
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
// @Param corrections formData string false "JSON 格式的图片或图纸人工修正"
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
		return echo.NewHTTPError(http.StatusBadRequest, "仍有未修正的图片或图纸")
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
	// A full workbook replacement does not imply that every asset is being
	// replaced.  A mold row without an archive directory keeps its existing
	// image/DWG rows; exclude those paths from the post-commit cleanup queue.
	preservedPaths, err := h.moldPreservedPaths(data)
	if err != nil {
		cleanupStaged(h.StorageRoot, paths)
		return err
	}
	oldPaths = subtractStoredPaths(oldPaths, preservedPaths)
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
		// Flat archive directories and file prefixes use the mold Model.  The
		// maps above remain keyed by MoldNumber because that is the database
		// owner key and the public correction contract.
		if err := exportMoldAssets(zw, h.StorageRoot, item.Model, item.Model, imageByMold[item.MoldNumber], drawingByMold[item.MoldNumber], archiveNames); err != nil {
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
	if len(zr.File) > maxMoldArchiveEntries {
		return packageData{}, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("资料包文件数量超过 %d 个，请拆分后导入", maxMoldArchiveEntries))
	}
	var declaredExpandedBytes uint64
	for _, item := range zr.File {
		if err := normalizeMoldZipEntryName(item); err != nil {
			return packageData{}, err
		}
		if item.UncompressedSize64 > maxMoldExpandedBytes-declaredExpandedBytes {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包解压后的文件总量超过 4 GiB，请拆分后导入")
		}
		declaredExpandedBytes += item.UncompressedSize64
		rawName := filepath.ToSlash(item.Name)
		if strings.Contains(rawName, "\\") {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包包含非法路径")
		}
		isDirectory := strings.HasSuffix(rawName, "/")
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSuffix(rawName, "/")))
		if isDirectory {
			clean += "/"
		}
		if rawName != clean || clean == "./" || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return packageData{}, echo.NewHTTPError(http.StatusBadRequest, "资料包包含非法路径")
		}
	}
	files, err := normalizeMoldPackageFiles(zr)
	if err != nil {
		return packageData{}, err
	}
	main := files["molds.xlsx"]
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
	data := packageData{AssetMolds: map[string]bool{}}
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
	// Relationship archive directories are keyed by Model.  Keep the parsed
	// MoldNumber in each Input because database replacement and correction
	// payloads continue to use the stable MoldNumber identity.
	moldInputsByModel := map[string]Input{}
	for _, row := range data.Rows {
		known[row.MoldNumber] = true
		moldInputsByModel[row.Model] = row
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.SliceStable(paths, func(i, j int) bool { return naturalAssetLess(paths[i], paths[j]) })
	for _, path := range paths {
		item := files[path]
		if path == "molds.xlsx" || path == "locations.json" {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
		markMoldModelAssetScope(path, item.FileInfo().IsDir(), moldInputsByModel, data.AssetMolds)
		if item.FileInfo().IsDir() {
			continue
		}
		if len(parts) < 2 {
			data.Errors = append(data.Errors, importError(path, "资料包文件路径不符合模板"))
			continue
		}
		if len(parts) != 2 {
			data.Errors = append(data.Errors, importError(path, "资料包文件路径不符合扁平模具目录"))
			continue
		}
		ext := strings.ToLower(filepath.Ext(parts[1]))
		if filemodule.AllowedImageExtension(ext) {
			asset, ok, ambiguous := parseFlatImageAsset(item, parts, moldInputsByModel)
			if ambiguous {
				data.Errors = append(data.Errors, importError(path, "资料包目录同时匹配单个模具型号和共模组，存在歧义"))
				continue
			}
			if !ok {
				data.Errors = append(data.Errors, importError(path, "图片无法匹配模具型号或共模目录"))
				continue
			}
			if validationErr := validateImageEntry(item); validationErr != nil {
				data.Errors = append(data.Errors, importError(path, validationErr.Error()))
				continue
			}
			if correction, exists := corrections[path]; exists {
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
			continue
		}
		if allowedDrawingExt(ext) {
			asset, ok, ambiguous := parseFlatDrawingAsset(item, parts, moldInputsByModel)
			if ambiguous || !ok {
				data.Errors = append(data.Errors, importError(path, "图纸无法匹配模具型号或共模目录"))
			} else if correction, exists := corrections[path]; exists {
				asset, ok = applyDrawingCorrection(asset, correction, known)
				if !ok {
					data.Errors = append(data.Errors, importError(path, "图纸人工修正无效"))
				} else {
					data.Drawings = append(data.Drawings, asset)
				}
			} else if len(asset.Codes) == 0 {
				data.Unresolved = append(data.Unresolved, asset)
			} else {
				data.Drawings = append(data.Drawings, asset)
			}
			continue
		}
		data.Errors = append(data.Errors, importError(path, "资料包包含未识别文件"))
	}
	resolveAmbiguousModelAssetScopes(&data, moldInputsByModel, files)
	sort.SliceStable(data.Images, func(i, j int) bool {
		return naturalAssetLess(data.Images[i].Name, data.Images[j].Name)
	})
	return data, nil
}

// resolveAmbiguousModelAssetScopes handles the flat relationship tree. The
// folder key is a Model, while the selected scope is recorded as MoldNumbers
// so replacement and cleanup remain number-based.
func resolveAmbiguousModelAssetScopes(data *packageData, inputs map[string]Input, files map[string]*zip.File) {
	if data == nil {
		return
	}
	type choice struct {
		mode    string
		members []string
	}
	choices := map[string]choice{}
	pending := map[string]bool{}
	for _, asset := range data.Unresolved {
		parts := strings.SplitN(asset.Path, "/", 2)
		if len(parts) == 2 {
			pending[parts[0]] = true
		}
	}
	assets := append(append([]packageAsset(nil), data.Images...), data.Drawings...)
	for _, asset := range assets {
		parts := strings.SplitN(asset.Path, "/", 2)
		if len(parts) != 2 {
			continue
		}
		folder := parts[0]
		exact, exactOK := inputs[folder]
		members, groupOK := validSharedModelMembers(folder, inputs)
		if !exactOK || !groupOK {
			continue
		}
		mode := "group"
		if containsString(asset.Codes, exact.MoldNumber) {
			mode = "single"
		}
		current := choices[folder]
		if current.mode != "" && current.mode != mode {
			data.Errors = append(data.Errors, importError(folder, "同一歧义目录中的资料不能混合选择单模和共模归属"))
			continue
		}
		choices[folder] = choice{mode: mode, members: members}
	}
	for folder, selected := range choices {
		if selected.mode == "single" {
			if input, ok := inputs[folder]; ok {
				data.AssetMolds[input.MoldNumber] = true
			}
			continue
		}
		for _, member := range selected.members {
			data.AssetMolds[member] = true
		}
	}
	reported := map[string]bool{}
	for path := range files {
		parts := strings.SplitN(strings.TrimSuffix(path, "/"), "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		folder := parts[0]
		if reported[folder] {
			continue
		}
		if _, exactOK := inputs[folder]; !exactOK {
			continue
		}
		if _, groupOK := validSharedModelMembers(folder, inputs); !groupOK || choices[folder].mode != "" || pending[folder] {
			continue
		}
		data.Errors = append(data.Errors, importError(folder, "含 + 的空资料目录同时匹配单模和共模，无法确认覆盖范围，请添加资料后在预览中确认或调整目录"))
		reported[folder] = true
	}
}

// normalizeMoldZipEntryName accepts the legacy GBK file names produced by the
// Windows archive used as the business reference. UTF-8 names remain untouched.
func normalizeMoldZipEntryName(item *zip.File) error {
	// Some macOS ZIP writers leave the UTF-8 flag unset even though the name
	// bytes are already valid UTF-8. Preserve valid UTF-8 regardless of that
	// metadata bit; only legacy invalid byte sequences should be decoded as GBK.
	if item == nil || utf8.ValidString(item.Name) {
		if item != nil {
			item.NonUTF8 = false
		}
		return nil
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes([]byte(item.Name))
	if err != nil || !utf8.Valid(decoded) {
		return echo.NewHTTPError(http.StatusBadRequest, "资料包文件名编码不支持，请使用 UTF-8 或 GBK ZIP")
	}
	item.Name = string(decoded)
	item.NonUTF8 = false
	return nil
}

// normalizeMoldPackageFiles strips the one optional packaging directory used
// by file managers when a folder is compressed (for example 001/...).  The
// business archive itself must still have molds.xlsx at its root.  Returning a
// normalized map also makes correction paths stable between preview and
// commit, regardless of whether the user compressed the containing folder.
func normalizeMoldPackageFiles(zr *zip.Reader) (map[string]*zip.File, error) {
	rawFiles := make(map[string]*zip.File, len(zr.File))
	for _, item := range zr.File {
		rawName := filepath.ToSlash(item.Name)
		isDirectory := strings.HasSuffix(rawName, "/")
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSuffix(rawName, "/")))
		if isDirectory {
			clean += "/"
		}
		if isIgnorableMoldPackagePath(clean) {
			continue
		}
		if _, exists := rawFiles[clean]; exists {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包包含重复文件")
		}
		rawFiles[clean] = item
	}

	mainPath := ""
	if rawFiles["molds.xlsx"] != nil {
		mainPath = "molds.xlsx"
	} else {
		candidates := make([]string, 0, 1)
		for path, item := range rawFiles {
			if item.FileInfo().IsDir() || strings.ToLower(filepath.Base(path)) != "molds.xlsx" {
				continue
			}
			candidates = append(candidates, path)
		}
		if len(candidates) == 0 {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "缺少 molds.xlsx")
		}
		if len(candidates) != 1 {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包包含多个 molds.xlsx")
		}
		mainPath = candidates[0]
	}

	prefix := ""
	if mainPath != "molds.xlsx" {
		prefix = strings.TrimSuffix(mainPath, "molds.xlsx")
		if !strings.HasSuffix(prefix, "/") {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包包含非法 molds.xlsx 路径")
		}
		wrapper := strings.TrimSuffix(prefix, "/")
		if wrapper == "" || strings.Contains(wrapper, "/") {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包只支持一层包装目录")
		}
	}

	files := make(map[string]*zip.File, len(rawFiles))
	for path, item := range rawFiles {
		if prefix != "" {
			if path == prefix {
				continue
			}
			if !strings.HasPrefix(path, prefix) {
				return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包包含包装目录外的文件")
			}
			path = strings.TrimPrefix(path, prefix)
		}
		if path == "" {
			continue
		}
		if _, exists := files[path]; exists {
			return nil, echo.NewHTTPError(http.StatusBadRequest, "资料包包含重复文件")
		}
		files[path] = item
	}
	if files["molds.xlsx"] == nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "缺少 molds.xlsx")
	}
	return files, nil
}

func isIgnorableMoldPackagePath(path string) bool {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for _, part := range parts {
		if strings.EqualFold(part, "__MACOSX") {
			return true
		}
	}
	base := filepath.Base(strings.TrimSuffix(path, "/"))
	switch {
	case strings.EqualFold(base, ".DS_Store"):
		return true
	case strings.EqualFold(base, "Thumbs.db"):
		return true
	case strings.EqualFold(base, "desktop.ini"):
		return true
	}
	return false
}

// markMoldModelAssetScope records replacement scopes for the current flat
// relationship archive.  Its folder and group keys are Models, but the scope
// map remains keyed by MoldNumber for the database replacement transaction.
func markMoldModelAssetScope(path string, directory bool, inputs map[string]Input, scopes map[string]bool) {
	if scopes == nil {
		return
	}
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	folder := parts[0]
	if input, ok := inputs[folder]; ok {
		// A single model containing '+' can collide with the exact common-group
		// folder.  Wait for the preview correction to select one interpretation.
		if _, groupOK := validSharedModelMembers(folder, inputs); groupOK {
			return
		}
		scopes[input.MoldNumber] = true
		return
	}
	if members, ok := validSharedModelMembers(folder, inputs); ok {
		for _, member := range members {
			scopes[member] = true
		}
	}
	_ = directory
}

// parseFlatImageAsset parses the current archive format, where files are
// directly below either a mold-model directory or an exact common-group
// directory.  A valid group with no exact member in its filename is returned
// as an unresolved asset so the preview can offer manual correction.  Codes
// and AllowedCodes always contain MoldNumbers even though matching uses Model.
func parseFlatImageAsset(item *zip.File, parts []string, known map[string]Input) (packageAsset, bool, bool) {
	if len(parts) != 2 || !validImageEntry(item) {
		return packageAsset{}, false, false
	}
	folder, name := parts[0], parts[1]
	asset := packageAsset{Entry: item, Path: strings.Join(parts, "/"), Name: name, Category: inferCategory(name), Kind: "image"}
	if input, ok := known[folder]; ok {
		if members, groupOK := validSharedModelMembers(folder, known); groupOK {
			asset.AllowedCodes = append([]string{input.MoldNumber}, members...)
			asset.AllowedMolds = allowedMoldsForCodes(asset.AllowedCodes, known)
			return asset, true, false
		}
		asset.Codes = []string{input.MoldNumber}
		asset.AllowedCodes = asset.Codes
		asset.AllowedMolds = allowedMoldsForCodes(asset.AllowedCodes, known)
		return asset, true, false
	}
	members, ok := validSharedModelMembers(folder, known)
	if !ok {
		return packageAsset{}, false, false
	}
	asset.AllowedCodes = members
	asset.AllowedMolds = allowedMoldsForCodes(members, known)
	asset.Codes, _ = moldNumbersInModelNameResult(name, members, known)
	return asset, true, false
}

func parseFlatDrawingAsset(item *zip.File, parts []string, known map[string]Input) (packageAsset, bool, bool) {
	if len(parts) != 2 || item == nil || item.UncompressedSize64 == 0 || !allowedDrawingExt(filepath.Ext(parts[1])) {
		return packageAsset{}, false, false
	}
	folder, name := parts[0], parts[1]
	asset := packageAsset{Entry: item, Path: strings.Join(parts, "/"), Name: name, Kind: "drawing"}
	if input, ok := known[folder]; ok {
		if members, groupOK := validSharedModelMembers(folder, known); groupOK {
			asset.AllowedCodes = append([]string{input.MoldNumber}, members...)
			asset.AllowedMolds = allowedMoldsForCodes(asset.AllowedCodes, known)
			return asset, true, false
		}
		asset.Codes = []string{input.MoldNumber}
		asset.AllowedCodes = asset.Codes
		asset.AllowedMolds = allowedMoldsForCodes(asset.AllowedCodes, known)
		return asset, true, false
	}
	members, ok := validSharedModelMembers(folder, known)
	if !ok {
		return packageAsset{}, false, false
	}
	asset.AllowedCodes = members
	asset.AllowedMolds = allowedMoldsForCodes(members, known)
	asset.Codes, _ = moldNumbersInModelNameResult(name, members, known)
	return asset, true, false
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
	seenModels := map[string]bool{}
	for i := header + 1; i < len(raw); i++ {
		cells := make([]string, len(headers))
		offset := 0
		// Manually edited workbooks may omit the blank serial-number cell A.
		// Excelize then returns the populated row from column B, so restore the
		// leading blank when the third returned value is clearly the mold type.
		if len(raw[i]) >= 4 && isMoldTypeLabel(raw[i][2]) && !isMoldTypeLabel(raw[i][3]) {
			offset = 1
		}
		for j, value := range raw[i] {
			if j+offset < len(cells) {
				cells[j+offset] = strings.TrimSpace(value)
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
		input := normalizeInput(Input{MoldNumber: cells[1], Model: cells[2], MoldType: typ, LocationCode: cells[4], CommonGroupNo: cells[5], Remark: cells[7]})
		if input.MoldNumber == "" || input.Model == "" || input.LocationCode == "" {
			errs = append(errs, rowError(i+1, "模具编号/模具型号/模具位置", "不能为空"))
			continue
		}
		if err := validateArchiveSegment(input.MoldNumber); err != nil {
			errs = append(errs, rowError(i+1, "模具编号", err.Error()))
			continue
		}
		if err := validateArchiveSegment(input.Model); err != nil {
			errs = append(errs, rowError(i+1, "模具型号", err.Error()))
			continue
		}
		if seen[input.MoldNumber] {
			errs = append(errs, rowError(i+1, "模具编号", "重复"))
			continue
		}
		seen[input.MoldNumber] = true
		if seenModels[input.Model] {
			errs = append(errs, rowError(i+1, "模具型号", "重复，无法唯一匹配资料目录"))
			continue
		}
		seenModels[input.Model] = true
		if err := validateInput(normalizeInput(input)); err != nil {
			errs = append(errs, rowError(i+1, "模具类型/共模组号", err.Error()))
			continue
		}
		rows = append(rows, input)
	}
	return rows, errs
}

func isMoldTypeLabel(value string) bool {
	value = strings.TrimSpace(value)
	return value == "单模" || value == "共模" || value == model.MoldTypeSingle || value == model.MoldTypeCommon
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

// validSharedModelMembers validates a flat relationship folder whose members
// are Models and returns the corresponding MoldNumbers.  Keeping the result
// in number space is important: packageAsset is consumed by the replacement
// transaction and correction API, both of which intentionally use numbers.
func validSharedModelMembers(value string, known map[string]Input) ([]string, bool) {
	parts := strings.Split(value, "+")
	if len(parts) < 2 {
		return nil, false
	}
	seen := make(map[string]bool, len(parts))
	var group string
	for _, modelName := range parts {
		if modelName == "" || seen[modelName] {
			return nil, false
		}
		input, ok := known[modelName]
		if !ok || input.MoldType != model.MoldTypeCommon || strings.TrimSpace(input.CommonGroupNo) == "" {
			return nil, false
		}
		if group == "" {
			group = strings.TrimSpace(input.CommonGroupNo)
		} else if group != strings.TrimSpace(input.CommonGroupNo) {
			return nil, false
		}
		seen[modelName] = true
	}
	expectedModels := make([]string, 0, len(parts))
	for modelName, input := range known {
		if input.MoldType == model.MoldTypeCommon && strings.TrimSpace(input.CommonGroupNo) == group {
			expectedModels = append(expectedModels, modelName)
		}
	}
	sort.SliceStable(expectedModels, func(i, j int) bool { return naturalAssetLess(expectedModels[i], expectedModels[j]) })
	if len(parts) != len(expectedModels) {
		return nil, false
	}
	for _, modelName := range expectedModels {
		if !seen[modelName] {
			return nil, false
		}
	}
	numbers := make([]string, 0, len(expectedModels))
	for _, modelName := range expectedModels {
		numbers = append(numbers, known[modelName].MoldNumber)
	}
	sort.SliceStable(numbers, func(i, j int) bool { return naturalAssetLess(numbers[i], numbers[j]) })
	return numbers, true
}

func moldNumbersInName(name string, members []string) []string {
	// Match longer known identifiers first so AB cannot consume part of AB-CD.
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

// moldNumbersInModelName matches a flat archive file name against Models and
// translates every matched model back to its MoldNumber. This keeps the
// relationship archive's model-directory contract separate from the stable
// MoldNumber identity used by storage and replacement.
func moldNumbersInModelName(name string, members []string, known map[string]Input) []string {
	numbers, _ := moldNumbersInModelNameResult(name, members, known)
	return numbers
}

// moldNumbersInModelNameResult matches a flat common-model asset against the
// models in its directory and translates the selected models back to their
// stable MoldNumbers.  The boolean is true when a non-exact alias maps to
// overlapping real models and therefore must be confirmed in the preview.
func moldNumbersInModelNameResult(name string, members []string, known map[string]Input) ([]string, bool) {
	// validSharedModelMembers returns MoldNumbers for the package contract.
	// Translate those numbers back to model keys before matching the file name.
	models := make([]string, 0, len(members))
	for modelName, input := range known {
		if containsString(members, input.MoldNumber) {
			models = append(models, modelName)
		}
	}
	matchedModels, ambiguous := matchModelNamesInName(name, models)
	if ambiguous {
		return nil, true
	}
	numbers := make([]string, 0, len(matchedModels))
	for _, modelName := range matchedModels {
		if input, ok := known[modelName]; ok {
			numbers = append(numbers, input.MoldNumber)
		}
	}
	sort.SliceStable(numbers, func(i, j int) bool { return naturalAssetLess(numbers[i], numbers[j]) })
	return numbers, false
}

// allowedMoldsForCodes keeps the wire-compatible allowed_codes list while
// supplying model labels for manual selection.  Iterating codes (rather than
// the map) makes preview order deterministic and keeps both lists aligned.
func allowedMoldsForCodes(codes []string, known map[string]Input) []MoldImportAllowedMold {
	result := make([]MoldImportAllowedMold, 0, len(codes))
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if code == "" || seen[code] {
			continue
		}
		for _, input := range known {
			if input.MoldNumber != code {
				continue
			}
			result = append(result, MoldImportAllowedMold{Code: code, Model: input.Model})
			seen[code] = true
			break
		}
	}
	return result
}

type modelNameOccurrence struct {
	Model string
	Start int
	End   int
	Exact bool
}

// matchModelNamesInName applies exact matching first, then a deliberately
// narrow alias rule.  Hyphens are ignored, and a single S may be inserted at
// the end of an alphabetic prefix immediately before the numeric portion (or
// removed from that position).  It does not remove arbitrary S characters:
// models such as S-123 remain distinct from 123.
func matchModelNamesInName(name string, models []string) ([]string, bool) {
	ordered := uniqueNonEmptyModelNames(models)
	if len(ordered) == 0 {
		return nil, false
	}

	occurrences := make([]modelNameOccurrence, 0, len(ordered))
	seen := make(map[modelNameOccurrence]bool)
	lowerName := strings.ToLower(name)
	for _, modelName := range ordered {
		for _, span := range findLiteralModelOccurrences(lowerName, strings.ToLower(modelName)) {
			occurrence := modelNameOccurrence{Model: modelName, Start: span.Start, End: span.End, Exact: true}
			seen[occurrence] = true
			occurrences = append(occurrences, occurrence)
		}
		for _, key := range modelMatchKeys(modelName) {
			for _, span := range findCompactModelOccurrences(lowerName, key) {
				alias := modelNameOccurrence{Model: modelName, Start: span.Start, End: span.End}
				if _, ok := findOccurrence(seen, alias); ok {
					// The literal occurrence is already represented as exact; an
					// alias occurrence at the same span is a duplicate too.
					continue
				}
				seen[alias] = true
				occurrences = append(occurrences, alias)
			}
		}
	}
	if len(occurrences) == 0 {
		return nil, false
	}

	sort.SliceStable(occurrences, func(i, j int) bool {
		if occurrences[i].Start != occurrences[j].Start {
			return occurrences[i].Start < occurrences[j].Start
		}
		if occurrences[i].End != occurrences[j].End {
			return occurrences[i].End > occurrences[j].End
		}
		if occurrences[i].Exact != occurrences[j].Exact {
			return occurrences[i].Exact
		}
		return naturalAssetLess(occurrences[i].Model, occurrences[j].Model)
	})

	selected := make(map[string]bool)
	ambiguous := false
	group := make([]modelNameOccurrence, 0, len(occurrences))
	groupEnd := -1
	flush := func() {
		if len(group) == 0 {
			return
		}
		exact := make([]modelNameOccurrence, 0, len(group))
		aliases := make([]modelNameOccurrence, 0, len(group))
		for _, occurrence := range group {
			if occurrence.Exact {
				exact = append(exact, occurrence)
			} else {
				aliases = append(aliases, occurrence)
			}
		}
		if len(exact) > 0 {
			// A literal match wins over aliases occupying the same span.  Within
			// literals, retain the existing longest-token behavior (AB-CD wins
			// over AB in AB-CD).
			sort.SliceStable(exact, func(i, j int) bool {
				left, right := exact[i].End-exact[i].Start, exact[j].End-exact[j].Start
				if left != right {
					return left > right
				}
				return naturalAssetLess(exact[i].Model, exact[j].Model)
			})
			chosen := make([]modelNameOccurrence, 0, len(exact))
			for _, occurrence := range exact {
				overlaps := false
				for _, prior := range chosen {
					if modelSpansOverlap(occurrence, prior) {
						overlaps = true
						break
					}
				}
				if !overlaps {
					chosen = append(chosen, occurrence)
					selected[occurrence.Model] = true
				}
			}
		} else {
			candidates := make(map[string]bool)
			for _, occurrence := range aliases {
				candidates[occurrence.Model] = true
			}
			if len(candidates) > 1 {
				ambiguous = true
			} else {
				for candidate := range candidates {
					selected[candidate] = true
				}
			}
		}
		group = group[:0]
		groupEnd = -1
	}
	for _, occurrence := range occurrences {
		if len(group) == 0 || occurrence.Start >= groupEnd {
			flush()
		}
		group = append(group, occurrence)
		if occurrence.End > groupEnd {
			groupEnd = occurrence.End
		}
	}
	flush()

	result := make([]string, 0, len(selected))
	for modelName := range selected {
		result = append(result, modelName)
	}
	sort.SliceStable(result, func(i, j int) bool { return naturalAssetLess(result[i], result[j]) })
	return result, ambiguous
}

func uniqueNonEmptyModelNames(models []string) []string {
	seen := make(map[string]bool, len(models))
	result := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || seen[modelName] {
			continue
		}
		seen[modelName] = true
		result = append(result, modelName)
	}
	sort.SliceStable(result, func(i, j int) bool { return naturalAssetLess(result[i], result[j]) })
	return result
}

func findOccurrence(seen map[modelNameOccurrence]bool, wanted modelNameOccurrence) (modelNameOccurrence, bool) {
	for occurrence := range seen {
		if occurrence.Model == wanted.Model && occurrence.Start == wanted.Start && occurrence.End == wanted.End {
			return occurrence, true
		}
	}
	return modelNameOccurrence{}, false
}

func findLiteralModelOccurrences(value, needle string) []modelNameOccurrenceSpan {
	if needle == "" {
		return nil
	}
	result := make([]modelNameOccurrenceSpan, 0, 1)
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], needle)
		if index < 0 {
			break
		}
		index += offset
		end := index + len(needle)
		if modelNameBoundary(value, index, end) {
			result = append(result, modelNameOccurrenceSpan{Start: index, End: end})
		}
		offset = index + 1
	}
	return result
}

type modelNameOccurrenceSpan struct {
	Start int
	End   int
}

func findCompactModelOccurrences(value, needle string) []modelNameOccurrenceSpan {
	if needle == "" {
		return nil
	}
	compact, positions := compactModelText(value)
	result := make([]modelNameOccurrenceSpan, 0, 1)
	for offset := 0; offset < len(compact); {
		index := strings.Index(compact[offset:], needle)
		if index < 0 {
			break
		}
		index += offset
		end := index + len(needle)
		if end <= len(positions) {
			startByte := positions[index]
			endByte := positions[end-1] + 1
			if modelNameBoundary(value, startByte, endByte) {
				result = append(result, modelNameOccurrenceSpan{Start: startByte, End: endByte})
			}
		}
		offset = index + 1
	}
	return result
}

func compactModelText(value string) (string, []int) {
	var builder strings.Builder
	positions := make([]int, 0, len(value))
	for index := 0; index < len(value); index++ {
		if value[index] == '-' {
			continue
		}
		builder.WriteByte(value[index])
		positions = append(positions, index)
	}
	return builder.String(), positions
}

func modelNameBoundary(value string, start, end int) bool {
	return (start == 0 || !isASCIILetterOrDigit(value[start-1])) && (end == len(value) || !isASCIILetterOrDigit(value[end]))
}

func modelSpansOverlap(left, right modelNameOccurrence) bool {
	return left.Start < right.End && right.Start < left.End
}

func modelMatchKeys(modelName string) []string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	compact := strings.ReplaceAll(modelName, "-", "")
	keys := make([]string, 0, 3)
	appendKey := func(value string) {
		if value == "" {
			return
		}
		for _, existing := range keys {
			if existing == value {
				return
			}
		}
		keys = append(keys, value)
	}
	appendKey(compact)
	prefix, suffix, ok := splitModelPrefixSuffix(compact)
	if !ok {
		return keys
	}
	if strings.HasSuffix(prefix, "s") {
		base := strings.TrimSuffix(prefix, "s")
		if len(base) >= 2 {
			appendKey(base + suffix)
		}
	} else if len(prefix) >= 2 {
		appendKey(prefix + "s" + suffix)
	}
	return keys
}

func splitModelPrefixSuffix(value string) (string, string, bool) {
	firstDigit := -1
	for index := 0; index < len(value); index++ {
		if value[index] >= '0' && value[index] <= '9' {
			firstDigit = index
			break
		}
	}
	if firstDigit < 2 {
		return "", "", false
	}
	for index := 0; index < firstDigit; index++ {
		if !((value[index] >= 'a' && value[index] <= 'z') || (value[index] >= 'A' && value[index] <= 'Z')) {
			return "", "", false
		}
	}
	for index := firstDigit; index < len(value); index++ {
		if !isASCIILetterOrDigit(value[index]) {
			return "", "", false
		}
	}
	return value[:firstDigit], value[firstDigit:], true
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
	case "product_material", "产品材料", "产品材料图片", "产品图", "材质":
		return "product_material"
	case "supplement", "补充图", "补充图片", "模具图":
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
	codes, ok := correctedAssetCodes(asset, correction.Codes, known)
	if !ok {
		return packageAsset{}, false
	}
	asset.Codes, asset.Category = codes, category
	return asset, true
}

func applyDrawingCorrection(asset packageAsset, correction ImportCorrection, known map[string]bool) (packageAsset, bool) {
	codes, ok := correctedAssetCodes(asset, correction.Codes, known)
	if !ok {
		return packageAsset{}, false
	}
	asset.Codes = codes
	return asset, true
}

func correctedAssetCodes(asset packageAsset, correctionCodes []string, known map[string]bool) ([]string, bool) {
	codes := make([]string, 0, len(correctionCodes))
	seen := map[string]bool{}
	for _, code := range correctionCodes {
		code = strings.TrimSpace(code)
		if code == "" || !known[code] || seen[code] || (len(asset.AllowedCodes) > 0 && !containsString(asset.AllowedCodes, code)) {
			return nil, false
		}
		seen[code] = true
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return nil, false
	}
	if len(asset.AllowedCodes) >= 3 && strings.Contains(asset.AllowedCodes[0], "+") && containsString(codes, asset.AllowedCodes[0]) && len(codes) != 1 {
		return nil, false
	}
	sort.Strings(codes)
	return codes, true
}

func inferCategory(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.Contains(stem, "产品刷墨图") || hasPositiveImageSequenceSuffix(stem) {
		return "product_material"
	}
	// The relationship-archive contract intentionally treats every other
	// valid image as a mold image.  This includes front/rear/local/size and
	// otherwise unnamed pictures; only ownership ambiguity remains unresolved.
	return "supplement"
}

func hasPositiveImageSequenceSuffix(stem string) bool {
	index := strings.LastIndexByte(stem, '-')
	if index < 0 || index == len(stem)-1 {
		return false
	}
	digits := stem[index+1:]
	nonZero := false
	for i := 0; i < len(digits); i++ {
		if !isASCIIDigit(digits[i]) {
			return false
		}
		if digits[i] != '0' {
			nonZero = true
		}
	}
	return nonZero
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
	// Snapshot the old owners before replacing the workbook.  Assets belonging
	// to a mold that remains in the workbook and has no archive directory are
	// migrated to the new row; every other asset is removed transactionally.
	var oldMolds []model.Mold
	if err := tx.Unscoped().Find(&oldMolds).Error; err != nil {
		return err
	}
	oldByNumber := make(map[string]model.Mold, len(oldMolds))
	for _, item := range oldMolds {
		oldByNumber[item.MoldNumber] = item
	}
	preserveOldIDs := make(map[uint]struct{})
	for _, row := range data.Rows {
		if !data.AssetMolds[row.MoldNumber] {
			if old, ok := oldByNumber[row.MoldNumber]; ok {
				preserveOldIDs[old.ID] = struct{}{}
			}
		}
	}
	var oldImages []model.ImageFile
	if err := tx.Unscoped().Where("owner_type = ?", "mold").Find(&oldImages).Error; err != nil {
		return err
	}
	for _, image := range oldImages {
		if _, preserve := preserveOldIDs[image.OwnerID]; preserve {
			continue
		}
		if err := tx.Unscoped().Delete(&image).Error; err != nil {
			return err
		}
	}
	var oldDrawings []model.MoldDrawing
	if err := tx.Unscoped().Find(&oldDrawings).Error; err != nil {
		return err
	}
	for _, drawing := range oldDrawings {
		if _, preserve := preserveOldIDs[drawing.MoldID]; preserve {
			continue
		}
		if err := tx.Unscoped().Delete(&drawing).Error; err != nil {
			return err
		}
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
		location := location
		location.ID = 0
		location.DeletedAt = gorm.DeletedAt{}
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
	// Repoint retained rows only after the replacement molds exist.  Updating
	// by primary key keeps their original storage paths and metadata intact.
	for _, old := range oldMolds {
		if _, preserve := preserveOldIDs[old.ID]; !preserve {
			continue
		}
		newID, ok := moldIDs[old.MoldNumber]
		if !ok {
			continue
		}
		if err := tx.Unscoped().Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", old.ID).Update("owner_id", newID).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Model(&model.MoldDrawing{}).Where("mold_id = ?", old.ID).Update("mold_id", newID).Error; err != nil {
			return err
		}
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

func (h *Handler) moldPreservedPaths(data packageData) ([]string, error) {
	if h.DB == nil || len(data.Rows) == 0 {
		return nil, nil
	}
	numbers := make([]string, 0, len(data.Rows))
	for _, row := range data.Rows {
		if !data.AssetMolds[row.MoldNumber] {
			numbers = append(numbers, row.MoldNumber)
		}
	}
	if len(numbers) == 0 {
		return nil, nil
	}
	var molds []model.Mold
	if err := h.DB.Where("mold_number IN ?", numbers).Find(&molds).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(molds))
	for _, item := range molds {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	paths := make([]string, 0)
	var images []model.ImageFile
	if err := h.DB.Where("owner_type = ? AND owner_id IN ?", "mold", ids).Select("storage_path", "preview_path").Find(&images).Error; err != nil {
		return nil, err
	}
	for _, image := range images {
		paths = append(paths, image.StoragePath)
		if image.PreviewPath != "" {
			paths = append(paths, image.PreviewPath)
		}
	}
	var drawings []model.MoldDrawing
	if err := h.DB.Where("mold_id IN ?", ids).Select("storage_path").Find(&drawings).Error; err != nil {
		return nil, err
	}
	for _, drawing := range drawings {
		paths = append(paths, drawing.StoragePath)
	}
	return paths, nil
}

func subtractStoredPaths(paths, excluded []string) []string {
	if len(excluded) == 0 {
		return paths
	}
	keep := make(map[string]struct{}, len(excluded))
	for _, path := range excluded {
		keep[path] = struct{}{}
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, found := keep[path]; !found {
			result = append(result, path)
		}
	}
	return result
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
		items = append(items, MoldImportFile{
			Path:         item.Path,
			Name:         item.Name,
			Kind:         item.Kind,
			AllowedCodes: append([]string(nil), item.AllowedCodes...),
			AllowedMolds: append([]MoldImportAllowedMold(nil), item.AllowedMolds...),
		})
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

func moldArchiveKey(item model.Mold) string {
	if modelName := strings.TrimSpace(item.Model); modelName != "" {
		return modelName
	}
	// Tests and a few historical records may not have populated Model.  Keep
	// the fallback local to archive generation; real imported/API rows still
	// require Model and therefore always take the model-based path.
	return strings.TrimSpace(item.MoldNumber)
}

func moldArchiveGroups(molds []model.Mold) ([]moldArchiveGroup, map[string]struct{}, error) {
	byGroup := map[string][]model.Mold{}
	knownModels := make(map[string]string, len(molds))
	for _, item := range molds {
		if err := validateArchiveSegment(item.MoldNumber); err != nil {
			return nil, nil, err
		}
		archiveKey := moldArchiveKey(item)
		if err := validateArchiveSegment(archiveKey); err != nil {
			return nil, nil, fmt.Errorf("模具型号包含非法资料包路径字符: %s", archiveKey)
		}
		if previous, exists := knownModels[archiveKey]; exists && previous != item.MoldNumber {
			return nil, nil, fmt.Errorf("模具型号重复，无法生成资料包目录: %s", archiveKey)
		}
		knownModels[archiveKey] = item.MoldNumber
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
		// A '+' in a model has the same ambiguity as a '+' in a mold number: it
		// could be a single-model directory or a joined group path. Keep such
		// groups out of the flat layout so export never creates an ambiguous
		// directory; the records remain exportable through their own model paths.
		ambiguous := false
		for _, member := range members {
			if strings.Contains(moldArchiveKey(member), "+") {
				ambiguous = true
				break
			}
		}
		if ambiguous {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			left, right := moldArchiveKey(members[i]), moldArchiveKey(members[j])
			if left == right {
				return members[i].ID < members[j].ID
			}
			return naturalAssetLess(left, right)
		})
		models := make([]string, 0, len(members))
		for _, member := range members {
			models = append(models, moldArchiveKey(member))
		}
		outer := strings.Join(models, "+")
		// A historical single mold may itself be named "A+B".  Emitting the
		// common group at that same path would make the flat archive ambiguous
		// and could silently associate assets with the wrong record.
		if _, collision := knownModels[outer]; collision {
			belongsToGroup := false
			for _, member := range members {
				if moldArchiveKey(member) == outer {
					belongsToGroup = true
					break
				}
			}
			if !belongsToGroup {
				continue
			}
		}
		if err := validateArchiveSegment(outer); err != nil {
			return nil, nil, err
		}
		for _, member := range members {
			grouped[member.MoldNumber] = struct{}{}
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
	for _, group := range groups {
		if err := addUniqueZipDirectory(zw, filepath.ToSlash(group.Outer)+"/", seen); err != nil {
			return err
		}
	}
	grouped := make(map[string]struct{})
	for _, group := range groups {
		for _, member := range group.Members {
			grouped[member.MoldNumber] = struct{}{}
		}
	}
	for _, item := range molds {
		if _, ok := grouped[item.MoldNumber]; ok {
			continue
		}
		if err := addUniqueZipDirectory(zw, filepath.ToSlash(moldArchiveKey(item))+"/", seen); err != nil {
			return err
		}
	}
	return nil
}

func exportMoldAssets(zw *zip.Writer, root, folder, moldNumber string, images []model.ImageFile, drawings []model.MoldDrawing, used map[string]struct{}) error {
	productSequence := 1
	for _, image := range images {
		name := archiveImageOutputName(image, moldNumber, productSequence, false)
		if image.Category == "product_material" && !isProductInkImage(image.OriginalName) {
			productSequence++
		}
		if err := exportImageAssetNamed(zw, root, folder, image, name, used); err != nil {
			return err
		}
	}
	for _, drawing := range drawings {
		if err := exportDrawingAssetNamed(zw, root, folder, drawing, archiveFileName(drawing.OriginalName), used); err != nil {
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
	productSequence := make(map[string]int, len(group.Members))
	sharedProductSequence := 1
	for _, member := range group.Members {
		productSequence[member.MoldNumber] = 1
	}
	for _, key := range imageKeys {
		members := imageGroups[key]
		if shared, ok := sharedImageAsset(root, group.Members, members); ok {
			name := archiveImageOutputName(shared, group.Outer, sharedProductSequence, true)
			if shared.Category == "product_material" && !isProductInkImage(shared.OriginalName) {
				sharedProductSequence++
			}
			if err := exportImageAssetNamed(zw, root, group.Outer, shared, name, used); err != nil {
				return err
			}
			continue
		}
		for _, member := range group.Members {
			for _, image := range members[member.MoldNumber] {
				memberKey := moldArchiveKey(member)
				name := archiveImageOutputName(image, memberKey, productSequence[member.MoldNumber], true)
				fallbackLabel := "模具图"
				if image.Category == "product_material" {
					fallbackLabel = "产品刷墨图"
				}
				name = memberSpecificArchiveName(memberKey, name, group.Members, fallbackLabel)
				if image.Category == "product_material" && !isProductInkImage(image.OriginalName) {
					productSequence[member.MoldNumber]++
				}
				if err := exportImageAssetNamed(zw, root, group.Outer, image, name, used); err != nil {
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
			if err := exportDrawingAssetNamed(zw, root, group.Outer, shared, prefixArchiveName(group.Outer, archiveFileName(shared.OriginalName)), used); err != nil {
				return err
			}
			continue
		}
		for _, member := range group.Members {
			for _, drawing := range members[member.MoldNumber] {
				name := memberSpecificArchiveName(moldArchiveKey(member), archiveFileName(drawing.OriginalName), group.Members, "图纸")
				if err := exportDrawingAssetNamed(zw, root, group.Outer, drawing, name, used); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func memberSpecificArchiveName(member, name string, group []model.Mold, fallbackLabel string) string {
	codes := make([]string, 0, len(group))
	for _, item := range group {
		codes = append(codes, moldArchiveKey(item))
	}
	for _, matched := range moldNumbersInName(name, codes) {
		if matched != member {
			ext := filepath.Ext(name)
			return member + "-" + fallbackLabel + ext
		}
	}
	return prefixArchiveName(member, name)
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

func exportImageAssetNamed(zw *zip.Writer, root, directory string, image model.ImageFile, name string, used map[string]struct{}) error {
	name = uniqueArchiveFileName(name, image.ID, directory, used)
	return addStoredFile(zw, root, filepath.ToSlash(filepath.Join(directory, name)), image.StoragePath)
}

func exportDrawingAsset(zw *zip.Writer, root, directory string, drawing model.MoldDrawing, used map[string]struct{}) error {
	name := uniqueArchiveFileName(archiveFileName(drawing.OriginalName), drawing.ID, directory, used)
	return addStoredFile(zw, root, filepath.ToSlash(filepath.Join(directory, name)), drawing.StoragePath)
}

func exportDrawingAssetNamed(zw *zip.Writer, root, directory string, drawing model.MoldDrawing, name string, used map[string]struct{}) error {
	name = uniqueArchiveFileName(name, drawing.ID, directory, used)
	return addStoredFile(zw, root, filepath.ToSlash(filepath.Join(directory, name)), drawing.StoragePath)
}

func archiveImageOutputName(image model.ImageFile, prefix string, sequence int, grouped bool) string {
	original := archiveFileName(image.OriginalName)
	ext := filepath.Ext(original)
	if image.Category == "product_material" {
		if isProductInkImage(original) {
			if grouped {
				return prefixArchiveName(prefix, original)
			}
			return original
		}
		return fmt.Sprintf("%s-%d%s", prefix, sequence, ext)
	}
	if strings.Contains(strings.TrimSuffix(original, ext), "产品刷墨图") {
		original = strings.ReplaceAll(strings.TrimSuffix(original, ext), "产品刷墨图", "模具图") + ext
	}
	if hasPositiveImageSequenceSuffix(strings.TrimSuffix(original, ext)) {
		original = strings.TrimSuffix(original, ext) + "-模具图" + ext
	}
	if grouped {
		return prefixArchiveName(prefix, original)
	}
	return original
}

func isProductInkImage(name string) bool {
	name = archiveFileName(name)
	return strings.Contains(strings.TrimSuffix(name, filepath.Ext(name)), "产品刷墨图")
}

func prefixArchiveName(prefix, name string) string {
	prefix = archiveFileName(prefix)
	name = archiveFileName(name)
	if prefix == "" {
		return name
	}
	lowerPrefix, lowerName := strings.ToLower(prefix), strings.ToLower(name)
	if strings.HasPrefix(lowerName, lowerPrefix) {
		if len(lowerName) == len(lowerPrefix) || !isASCIILetterOrDigit(lowerName[len(lowerPrefix)]) {
			return name
		}
	}
	return prefix + "-" + name
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
		candidate := fmt.Sprintf("%s-副本%d%s", stem, id, ext)
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-副本%d_%d%s", stem, id, suffix, ext)
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
