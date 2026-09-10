package mold

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

const (
	moldImportModule           = "molds"
	MaxPackageSize      int64  = 2 << 30
	maxMoldWorkbookSize uint64 = 64 << 20
	maxMoldAssetSize    uint64 = 512 << 20
)

var moldColumns = []spreadsheet.Column{
	{Key: "sequence", Title: "序号", Width: 10, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "product_model", Title: "产品型号", Width: 24, Type: spreadsheet.CellTypeText},
	{Key: "mold_type", Title: "模具类型", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "cavity_count", Title: "模穴数", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "location", Title: "模具位置", Width: 14, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "common_group_no", Title: "共模组号", Width: 16, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "image_count", Title: "图片数量", Width: 12, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
	{Key: "remark", Title: "备注", Width: 36, Type: spreadsheet.CellTypeText},
}

type MoldImportPreviewResult struct {
	Token     string                  `json:"token,omitempty"`
	ExpiresAt *time.Time              `json:"expires_at,omitempty"`
	Summary   MoldImportSummary       `json:"summary"`
	Errors    []spreadsheet.CellError `json:"errors"`
}

type MoldImportSummary struct {
	Molds           int `json:"molds"`
	Images          int `json:"images"`
	Drawings        int `json:"drawings"`
	Locations       int `json:"locations"`
	ProductsCreated int `json:"products_created"`
}

type MoldImportResult struct {
	Molds           int       `json:"molds"`
	Images          int       `json:"images"`
	Drawings        int       `json:"drawings"`
	ProductsCreated int       `json:"products_created"`
	CompletedAt     time.Time `json:"completed_at"`
}

type moldPackageRow struct {
	Sequence     string
	ProductModel string
	MoldType     string
	CavityCount  string
	LocationCode string
	GroupNo      string
	Remark       string
}

type stagedMoldAsset struct {
	ProductModel string
	Sequence     string
	OriginalName string
	Extension    string
	MIME         string
	Size         int64
	TempPath     string
	StoragePath  string
	PreviewPath  string
	PreviewMIME  string
	PreviewSize  int64
	Drawing      bool
}

// ImportTemplate 下载模具 ZIP 模板。
// @Summary 下载模具导入模板
// @Tags mold
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/molds/import-template [get]
func (h *Handler) ImportTemplate(c *echo.Context) error {
	rows := [][]string{{"001", "示例产品型号", "单模", "1*1", "A1-1", "", "0", "示例备注"}}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), spreadsheet.SpreadsheetDocument{SheetName: "模具", Title: "博邦模具", Columns: moldColumns, Rows: rows, TotalRows: 1})
	if err != nil {
		return err
	}
	locations, err := json.MarshalIndent(defaultMoldLocations(), "", "  ")
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	if err := addMoldZipBytes(zw, "molds.xlsx", xlsx); err != nil {
		return err
	}
	if err := addMoldZipBytes(zw, "locations.json", append(locations, '\n')); err != nil {
		return err
	}
	if _, err := zw.Create("示例产品型号/001/"); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return sendMoldDownload(c, "博邦模具导入模板.zip", "application/zip", bytes.NewReader(archive.Bytes()), int64(archive.Len()))
}

// ImportPreview 预览模具资料包。
// @Summary 预览模具资料包
// @Tags mold
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "模具 ZIP"
// @Success 200 {object} MoldImportPreviewResult
// @Router /api/v1/molds/import/preview [post]
func (h *Handler) ImportPreview(c *echo.Context) error {
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择模具资料包")
	}
	temp, hash, size, cleanup, err := receiveMoldPackage(header)
	if err != nil {
		return err
	}
	defer cleanup()
	rows, locations, images, drawings, errs, err := h.readMoldPackage(c.Request().Context(), temp, size)
	if err != nil {
		return err
	}
	created, err := h.countMissingProducts(rows)
	if err != nil {
		return err
	}
	result := MoldImportPreviewResult{Summary: MoldImportSummary{Molds: len(rows), Images: len(images), Drawings: len(drawings), Locations: len(locations), ProductsCreated: created}, Errors: errs}
	if len(errs) == 0 {
		token, tokenHash, err := newMoldToken()
		if err != nil {
			return err
		}
		expires := time.Now().Add(30 * time.Minute)
		if err := h.DB.Create(&model.ImportSession{UserID: currentMoldUserID(c), Module: moldImportModule, FileHash: hash, TokenHash: tokenHash, ExpiresAt: expires}).Error; err != nil {
			return err
		}
		result.Token, result.ExpiresAt = token, &expires
	}
	return c.JSON(http.StatusOK, result)
}

// ImportCommit 提交模具资料包。
// @Summary 提交模具资料包
// @Tags mold
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "模具 ZIP"
// @Param token formData string true "预览令牌"
// @Success 201 {object} MoldImportResult
// @Router /api/v1/molds/import/commit [post]
func (h *Handler) ImportCommit(c *echo.Context) error {
	if h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "模具文件服务未配置")
	}
	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "缺少预览令牌")
	}
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择模具资料包")
	}
	temp, hash, size, cleanup, err := receiveMoldPackage(header)
	if err != nil {
		return err
	}
	defer cleanup()
	rows, locations, images, drawings, errs, err := h.readMoldPackage(c.Request().Context(), temp, size)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "文件校验失败，请重新预览")
	}
	var session model.ImportSession
	if err := h.DB.Where("token_hash = ? AND user_id = ? AND module = ?", hashMoldToken(token), currentMoldUserID(c), moldImportModule).First(&session).Error; err != nil {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌无效，请重新预览")
	}
	if session.ConsumedAt != nil || time.Now().After(session.ExpiresAt) || session.FileHash != hash {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效、文件不一致或已使用")
	}
	unlock := filemodule.LockMoldAssetMutation()
	defer unlock()
	staged, err := h.stageMoldAssets(rows, temp)
	if err != nil {
		cleanupStagedMoldAssets(h.StorageRoot, staged)
		return err
	}
	oldPaths, err := h.moldStoredPaths()
	if err != nil {
		cleanupStagedMoldAssets(h.StorageRoot, staged)
		return err
	}
	result := MoldImportResult{Images: len(images), Drawings: len(drawings), CompletedAt: time.Now()}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		consume := tx.Model(&model.ImportSession{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", session.ID, result.CompletedAt).Update("consumed_at", result.CompletedAt)
		if consume.Error != nil || consume.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效或已使用")
		}
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
		locationIDs := map[string]uint{}
		for _, location := range locations {
			if strings.TrimSpace(location.Code) == "" {
				continue
			}
			item := model.MoldLocation{Code: strings.TrimSpace(location.Code), Status: model.StatusActive}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			locationIDs[item.Code] = item.ID
		}
		productIDs := map[string]uint{}
		for _, row := range rows {
			if productIDs[row.ProductModel] != 0 {
				continue
			}
			var product model.Product
			err := tx.Where("product_model = ?", row.ProductModel).First(&product).Error
			if err == gorm.ErrRecordNotFound {
				product = model.Product{ProductModel: row.ProductModel, Status: model.StatusActive}
				if err := tx.Create(&product).Error; err != nil {
					return err
				}
				result.ProductsCreated++
			} else if err != nil {
				return err
			}
			productIDs[row.ProductModel] = product.ID
		}
		moldIDs := map[string]uint{}
		for _, row := range rows {
			locationID := locationIDs[row.LocationCode]
			if locationID == 0 {
				return fmt.Errorf("模具 %s/%s 的位置 %s 不存在", row.ProductModel, row.Sequence, row.LocationCode)
			}
			item := model.Mold{ProductID: productIDs[row.ProductModel], MoldType: row.MoldType, CavityCount: row.CavityCount, LocationID: locationID, CommonGroupNo: row.GroupNo, Remark: row.Remark}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			moldIDs[row.ProductModel+"/"+row.Sequence] = item.ID
		}
		for _, asset := range staged {
			moldID := moldIDs[asset.ProductModel+"/"+asset.Sequence]
			if moldID == 0 {
				continue
			}
			if asset.Drawing {
				drawing := model.MoldDrawing{MoldID: moldID, UploadedBy: currentMoldUserID(c), OriginalName: asset.OriginalName, Size: asset.Size, MimeType: "application/octet-stream", Extension: asset.Extension, StoragePath: asset.StoragePath}
				if err := tx.Create(&drawing).Error; err != nil {
					return err
				}
				continue
			}
			image := model.ImageFile{OwnerType: "mold", OwnerID: moldID, UploadedBy: currentMoldUserID(c), OriginalName: asset.OriginalName, Size: asset.Size, MimeType: asset.MIME, Extension: asset.Extension, StoragePath: asset.StoragePath, PreviewPath: asset.PreviewPath, PreviewMime: asset.PreviewMIME, PreviewSize: asset.PreviewSize}
			if err := tx.Create(&image).Error; err != nil {
				return err
			}
		}
		result.Molds = len(rows)
		return filemodule.QueueCleanupTasks(tx, oldPaths)
	})
	if err != nil {
		cleanupStagedMoldAssets(h.StorageRoot, staged)
		return err
	}
	filemodule.CleanupStoredPaths(h.StorageRoot, h.DB, oldPaths)
	cleanupMoldAssetTemps(staged)
	return c.JSON(http.StatusCreated, result)
}

// Export 导出模具资料包。
// @Summary 导出模具资料包
// @Tags mold
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/molds/export [get]
func (h *Handler) Export(c *echo.Context) error {
	if h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "模具文件服务未配置")
	}
	var molds []model.Mold
	if err := h.DB.Preload("Product").Preload("Location").Order("product_id asc, id asc").Find(&molds).Error; err != nil {
		return err
	}
	var locations []model.MoldLocation
	if err := h.DB.Order("code asc, id asc").Find(&locations).Error; err != nil {
		return err
	}
	sequenceByModel := map[string]int{}
	rowByMold := map[uint]string{}
	rows := make([][]string, 0, len(molds))
	for _, item := range molds {
		sequenceByModel[item.Product.ProductModel]++
		seq := fmt.Sprintf("%03d", sequenceByModel[item.Product.ProductModel])
		rowByMold[item.ID] = seq
		var imageCount, drawingCount int64
		_ = h.DB.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Count(&imageCount).Error
		_ = h.DB.Model(&model.MoldDrawing{}).Where("mold_id = ?", item.ID).Count(&drawingCount).Error
		rows = append(rows, []string{seq, item.Product.ProductModel, moldTypeLabel(item.MoldType), item.CavityCount, item.Location.Code, item.CommonGroupNo, strconv.FormatInt(imageCount, 10), item.Remark})
	}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), spreadsheet.SpreadsheetDocument{SheetName: "模具", Title: "博邦模具", Columns: moldColumns, Rows: rows, TotalRows: int64(len(rows))})
	if err != nil {
		return err
	}
	locationData, _ := json.MarshalIndent(locations, "", "  ")
	temp, err := os.CreateTemp("", "bb-molds-export-*.zip")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	zw := zip.NewWriter(temp)
	if err := addMoldZipBytes(zw, "molds.xlsx", xlsx); err != nil {
		temp.Close()
		return err
	}
	if err := addMoldZipBytes(zw, "locations.json", append(locationData, '\n')); err != nil {
		temp.Close()
		return err
	}
	for _, item := range molds {
		dir := filepath.ToSlash(filepath.Join(item.Product.ProductModel, rowByMold[item.ID]))
		if _, err := zw.Create(dir + "/"); err != nil {
			temp.Close()
			return err
		}
		var images []model.ImageFile
		if err := h.DB.Where("owner_type = ? AND owner_id = ?", "mold", item.ID).Order("sort_order asc, id asc").Find(&images).Error; err != nil {
			temp.Close()
			return err
		}
		for _, image := range images {
			if err := addMoldStoredFile(zw, h.StorageRoot, filepath.ToSlash(filepath.Join(dir, uniqueMoldArchiveName(image.OriginalName, image.ID))), image.StoragePath); err != nil {
				temp.Close()
				return err
			}
		}
		var drawings []model.MoldDrawing
		if err := h.DB.Where("mold_id = ?", item.ID).Find(&drawings).Error; err != nil {
			temp.Close()
			return err
		}
		for _, drawing := range drawings {
			if err := addMoldStoredFile(zw, h.StorageRoot, filepath.ToSlash(filepath.Join(dir, uniqueMoldArchiveName(drawing.OriginalName, drawing.ID))), drawing.StoragePath); err != nil {
				temp.Close()
				return err
			}
		}
	}
	if err := zw.Close(); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		temp.Close()
		return err
	}
	info, err := temp.Stat()
	if err != nil {
		temp.Close()
		return err
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="bobang-molds.zip"`)
	return c.Stream(http.StatusOK, "application/zip", io.LimitReader(temp, info.Size()))
}

func (h *Handler) readMoldPackage(ctx context.Context, path string, size int64) ([]moldPackageRow, []model.MoldLocation, []string, []string, []spreadsheet.CellError, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "ZIP 资料包无法打开")
	}
	defer reader.Close()
	var workbook *zip.File
	var locationFile *zip.File
	images := []string{}
	drawings := []string{}
	dirs := map[string]bool{}
	for _, item := range reader.File {
		name := strings.TrimPrefix(filepath.ToSlash(item.Name), "./")
		if item.FileInfo().IsDir() {
			parts := strings.Split(strings.Trim(name, "/"), "/")
			if len(parts) == 2 {
				dirs[parts[0]+"/"+parts[1]] = true
			}
			continue
		}
		switch name {
		case "molds.xlsx":
			workbook = item
			continue
		case "locations.json":
			locationFile = item
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) >= 3 {
			dirs[parts[0]+"/"+parts[1]] = true
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".dwg" || ext == ".fdwg" {
			drawings = append(drawings, name)
		} else if filemodule.AllowedImageExtension(ext) {
			images = append(images, name)
		}
	}
	if workbook == nil {
		return nil, nil, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "缺少 molds.xlsx")
	}
	if workbook.UncompressedSize64 > maxMoldWorkbookSize {
		return nil, nil, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "molds.xlsx 超过 64 MiB")
	}
	rc, err := workbook.Open()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	sheetRows, err := spreadsheet.XLSXReader{}.Read(ctx, rc, spreadsheet.ReadOptions{})
	rc.Close()
	if err != nil {
		return nil, nil, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "读取 molds.xlsx 失败")
	}
	rows, errs := decodeMoldRows(sheetRows, dirs)
	locations := defaultMoldLocations()
	if locationFile != nil {
		lr, err := locationFile.Open()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		var input []model.MoldLocation
		if err := json.NewDecoder(io.LimitReader(lr, 4<<20)).Decode(&input); err != nil {
			lr.Close()
			return nil, nil, nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "locations.json 无效")
		}
		lr.Close()
		if len(input) > 0 {
			locations = input
		}
	}
	sort.Strings(images)
	sort.Strings(drawings)
	return rows, locations, images, drawings, errs, nil
}

func decodeMoldRows(sheetRows [][]string, dirs map[string]bool) ([]moldPackageRow, []spreadsheet.CellError) {
	if len(sheetRows) == 0 {
		return nil, []spreadsheet.CellError{{Row: 1, Column: "文件", Reason: "工作簿为空"}}
	}
	headerIndex := 0
	if len(sheetRows) > 1 && strings.TrimSpace(moldCellAt(sheetRows[1], 0)) == "序号" {
		headerIndex = 1
	}
	header := sheetRows[headerIndex]
	index := map[string]int{}
	for i, value := range header {
		index[strings.TrimSpace(value)] = i
	}
	required := []string{"序号", "产品型号", "模具类型", "模穴数", "模具位置", "共模组号", "备注"}
	for _, name := range required {
		if _, ok := index[name]; !ok {
			return nil, []spreadsheet.CellError{{Row: headerIndex + 1, Column: name, Reason: "缺少必需列"}}
		}
	}
	rows := []moldPackageRow{}
	errs := []spreadsheet.CellError{}
	seen := map[string]bool{}
	for rowIndex := headerIndex + 1; rowIndex < len(sheetRows); rowIndex++ {
		row := sheetRows[rowIndex]
		productModel := strings.TrimSpace(moldCellAt(row, index["产品型号"]))
		if productModel == "" {
			continue
		}
		sequence := strings.TrimSpace(moldCellAt(row, index["序号"]))
		if sequence == "" {
			sequence = fmt.Sprintf("%03d", rowIndex-headerIndex)
		}
		if strings.ContainsAny(productModel, `/\\`) || strings.ContainsAny(sequence, `/\\`) {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "产品型号/序号", Reason: "不能包含斜杠"})
			continue
		}
		key := productModel + "/" + sequence
		if seen[key] {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "序号", Value: sequence, Reason: "同一产品型号下序号重复"})
			continue
		}
		seen[key] = true
		moldType := strings.TrimSpace(moldCellAt(row, index["模具类型"]))
		switch moldType {
		case "单模", "single":
			moldType = model.MoldTypeSingle
		case "共模", "common":
			moldType = model.MoldTypeCommon
		default:
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "模具类型", Value: moldType, Reason: "只能填写单模或共模"})
			continue
		}
		group := strings.TrimSpace(moldCellAt(row, index["共模组号"]))
		if moldType == model.MoldTypeCommon && group == "" {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "共模组号", Reason: "共模必须填写"})
			continue
		}
		rows = append(rows, moldPackageRow{Sequence: sequence, ProductModel: productModel, MoldType: moldType, CavityCount: strings.TrimSpace(moldCellAt(row, index["模穴数"])), LocationCode: strings.TrimSpace(moldCellAt(row, index["模具位置"])), GroupNo: group, Remark: strings.TrimSpace(moldCellAt(row, index["备注"]))})
	}
	for dir := range dirs {
		if !seen[dir] {
			errs = append(errs, spreadsheet.CellError{Row: 0, Column: "目录", Value: dir, Reason: "目录没有对应的模具行"})
		}
	}
	return rows, errs
}

func (h *Handler) countMissingProducts(rows []moldPackageRow) (int, error) {
	seen := map[string]bool{}
	count := 0
	for _, row := range rows {
		if seen[row.ProductModel] {
			continue
		}
		seen[row.ProductModel] = true
		var value int64
		if err := h.DB.Model(&model.Product{}).Where("product_model = ?", row.ProductModel).Count(&value).Error; err != nil {
			return 0, err
		}
		if value == 0 {
			count++
		}
	}
	return count, nil
}

func (h *Handler) stageMoldAssets(rows []moldPackageRow, packagePath string) ([]stagedMoldAsset, error) {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := reader.File
	known := map[string]bool{}
	for _, row := range rows {
		known[row.ProductModel+"/"+row.Sequence] = true
	}
	staged := []stagedMoldAsset{}
	for _, entry := range entries {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(entry.Name), "./")
		parts := strings.Split(name, "/")
		if len(parts) < 3 || !known[parts[0]+"/"+parts[1]] {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		drawing := ext == ".dwg" || ext == ".fdwg"
		if !drawing && !filemodule.AllowedImageExtension(ext) {
			continue
		}
		if entry.UncompressedSize64 > maxMoldAssetSize {
			return staged, echo.NewHTTPError(http.StatusBadRequest, "单个模具资料超过 512 MiB")
		}
		src, err := entry.Open()
		if err != nil {
			return staged, err
		}
		temp, err := os.CreateTemp("", "bb-mold-import-*"+ext)
		if err != nil {
			src.Close()
			return staged, err
		}
		written, copyErr := io.Copy(temp, io.LimitReader(src, int64(maxMoldAssetSize)+1))
		closeErr := temp.Close()
		src.Close()
		if copyErr != nil || closeErr != nil || uint64(written) > maxMoldAssetSize {
			os.Remove(temp.Name())
			return staged, echo.NewHTTPError(http.StatusBadRequest, "模具资料读取失败或超过 512 MiB")
		}
		now := time.Now()
		randomName, err := randomMoldName(ext)
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		asset := stagedMoldAsset{ProductModel: parts[0], Sequence: parts[1], OriginalName: filepath.Base(name), Extension: ext, Size: written, TempPath: temp.Name(), Drawing: drawing}
		if drawing {
			asset.StoragePath = filepath.ToSlash(filepath.Join("mold", "drawings", now.Format("2006"), now.Format("01"), randomName))
		} else {
			if err := filemodule.ValidateStaticImage(written, ext, func() (io.ReadCloser, error) { return os.Open(temp.Name()) }); err != nil {
				os.Remove(temp.Name())
				return staged, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("图片 %s 校验失败：%s", name, err.Error()))
			}
			asset.StoragePath = filepath.ToSlash(filepath.Join("mold", now.Format("2006"), now.Format("01"), randomName))
			preview, previewMIME, err := filemodule.MakeStaticPreviewFile(temp.Name(), ext)
			if err != nil {
				os.Remove(temp.Name())
				return staged, err
			}
			previewRandom, err := randomMoldName(".jpg")
			if err != nil {
				os.Remove(temp.Name())
				return staged, err
			}
			asset.PreviewPath = filepath.ToSlash(filepath.Join("mold", now.Format("2006"), now.Format("01"), "preview-"+previewRandom))
			asset.PreviewMIME = previewMIME
			asset.PreviewSize = int64(len(preview))
			asset.MIME = filemodule.ImageMIMEForExtension(ext)
			if err := writeMoldAsset(temp.Name(), h.StorageRoot, asset.StoragePath, nil); err != nil {
				os.Remove(temp.Name())
				return staged, err
			}
			previewPath, _ := moldStoragePath(h.StorageRoot, asset.PreviewPath)
			if err := os.MkdirAll(filepath.Dir(previewPath), 0o755); err != nil {
				os.Remove(temp.Name())
				return staged, err
			}
			if err := os.WriteFile(previewPath, preview, 0o644); err != nil {
				os.Remove(temp.Name())
				return staged, err
			}
		}
		staged = append(staged, asset)
	}
	return staged, nil
}

func (h *Handler) moldStoredPaths() ([]string, error) {
	var images []model.ImageFile
	if err := h.DB.Where("owner_type = ?", "mold").Find(&images).Error; err != nil {
		return nil, err
	}
	var drawings []model.MoldDrawing
	if err := h.DB.Find(&drawings).Error; err != nil {
		return nil, err
	}
	paths := []string{}
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

func receiveMoldPackage(header *multipart.FileHeader) (string, string, int64, func(), error) {
	if header == nil || header.Size <= 0 {
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "文件不能为空")
	}
	if header.Size > MaxPackageSize {
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "模具资料包超过 2 GiB")
	}
	src, err := header.Open()
	if err != nil {
		return "", "", 0, func() {}, err
	}
	defer src.Close()
	temp, err := os.CreateTemp("", "bb-molds-import-*.zip")
	if err != nil {
		return "", "", 0, func() {}, err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(src, MaxPackageSize+1))
	closeErr := temp.Close()
	if err != nil || closeErr != nil || written > MaxPackageSize {
		os.Remove(temp.Name())
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "模具资料包读取失败或超过 2 GiB")
	}
	return temp.Name(), hex.EncodeToString(hash.Sum(nil)), written, func() { _ = os.Remove(temp.Name()) }, nil
}

func writeMoldAsset(temp, root, relative string, _ []byte) error {
	dstPath, err := moldStoragePath(root, relative)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	src, err := os.Open(temp)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func cleanupMoldAssetTemps(items []stagedMoldAsset) {
	for _, item := range items {
		_ = os.Remove(item.TempPath)
	}
}

func cleanupStagedMoldAssets(root string, items []stagedMoldAsset) {
	for _, item := range items {
		_ = os.Remove(item.TempPath)
		if path, err := moldStoragePath(root, item.StoragePath); err == nil {
			_ = os.Remove(path)
		}
		if item.PreviewPath != "" {
			if path, err := moldStoragePath(root, item.PreviewPath); err == nil {
				_ = os.Remove(path)
			}
		}
	}
}

func moldStoragePath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	normalized := filepath.ToSlash(clean)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !strings.HasPrefix(normalized, "mold/") {
		return "", fmt.Errorf("非法模具文件路径")
	}
	return filepath.Join(root, clean), nil
}

func addMoldZipBytes(zw *zip.Writer, name string, data []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}
func addMoldStoredFile(zw *zip.Writer, root, archiveName, relative string) error {
	path, err := moldStoragePath(root, relative)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writer, err := zw.Create(archiveName)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, f)
	return err
}
func randomMoldName(ext string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw) + ext, nil
}
func newMoldToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashMoldToken(token), nil
}
func hashMoldToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func currentMoldUserID(c *echo.Context) uint {
	if current := auth.GetCurrentUser(c); current != nil {
		return current.ID
	}
	return 0
}
func moldCellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}
func uniqueMoldArchiveName(name string, id uint) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." {
		base = "asset"
	}
	ext := filepath.Ext(base)
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(base, ext), id, ext)
}
func sendMoldDownload(c *echo.Context, name, contentType string, reader io.Reader, size int64) error {
	spreadsheet.DownloadHeaders(c.Response().Header(), name, contentType, size)
	return c.Stream(http.StatusOK, contentType, reader)
}
func moldTypeLabel(value string) string {
	if value == model.MoldTypeCommon {
		return "共模"
	}
	return "单模"
}
