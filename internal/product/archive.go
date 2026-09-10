package product

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	productImportModule           = "products"
	MaxProductPackageSize  int64  = 2 << 30
	maxProductWorkbookSize uint64 = 64 << 20
	maxProductEntrySize    uint64 = 512 << 20
)

var productColumns = []spreadsheet.Column{
	{Key: "id", Title: "序号", Width: 8, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
	{Key: "product_model", Title: "产品型号", Width: 24, Type: spreadsheet.CellTypeText},
	{Key: "customer_model", Title: "客户型号", Width: 20, Type: spreadsheet.CellTypeText},
	{Key: "material", Title: "产品材料", Width: 16, Type: spreadsheet.CellTypeText},
	{Key: "ink_required", Title: "是否刷墨", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "status", Title: "状态", Width: 12, Type: spreadsheet.CellTypeText, Alignment: "center"},
	{Key: "image_count", Title: "图片数量", Width: 12, Type: spreadsheet.CellTypeNumber, Alignment: "center"},
}

type ProductImportPreview struct {
	Token     string                  `json:"token,omitempty"`
	ExpiresAt *time.Time              `json:"expires_at,omitempty"`
	Summary   ProductImportSummary    `json:"summary"`
	Errors    []spreadsheet.CellError `json:"errors"`
}

type ProductImportSummary struct {
	Products int `json:"products"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Images   int `json:"images"`
}

type ProductImportResult struct {
	Created     int       `json:"created"`
	Updated     int       `json:"updated"`
	Images      int       `json:"images"`
	CompletedAt time.Time `json:"completed_at"`
}

type productPackageRow struct {
	Product model.Product
	HasDir  bool
}

type stagedProductImage struct {
	ProductModel string
	OriginalName string
	Extension    string
	MIME         string
	Size         int64
	TempPath     string
	StoragePath  string
	PreviewPath  string
	PreviewMIME  string
	PreviewSize  int64
}

// ImportTemplate 下载产品资料 ZIP 模板。
// @Summary 下载产品资料模板
// @Tags product
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/products/import-template [get]
func (h *Handler) ImportTemplate(c *echo.Context) error {
	document := spreadsheet.SpreadsheetDocument{SheetName: "产品资料", Title: "博邦产品资料", Columns: productColumns, Rows: [][]string{{"", "示例产品型号", "客户型号", "ABS 白", "否", "启用", "0"}}, TotalRows: 1}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), document)
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	if err := addProductZipBytes(zw, "products.xlsx", xlsx); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return sendProductDownload(c, "博邦产品资料导入模板.zip", "application/zip", bytes.NewReader(archive.Bytes()), int64(archive.Len()))
}

// ImportPreview 预览产品资料 ZIP。
// @Summary 预览产品资料包
// @Tags product
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "产品资料 ZIP"
// @Success 200 {object} ProductImportPreview
// @Router /api/v1/products/import/preview [post]
func (h *Handler) ImportPreview(c *echo.Context) error {
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择产品资料 ZIP")
	}
	temp, hash, size, cleanup, err := receiveProductPackage(header)
	if err != nil {
		return err
	}
	defer cleanup()
	rows, images, errs, err := h.readProductPackage(c.Request().Context(), temp, size)
	if err != nil {
		return err
	}
	result := ProductImportPreview{Summary: ProductImportSummary{Products: len(rows), Images: len(images)}, Errors: errs}
	for _, row := range rows {
		var count int64
		if err := h.DB.Model(&model.Product{}).Where("product_model = ?", row.Product.ProductModel).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			result.Summary.Updated++
		} else {
			result.Summary.Created++
		}
	}
	if len(errs) == 0 {
		token, tokenHash, err := newProductToken()
		if err != nil {
			return err
		}
		expires := time.Now().Add(30 * time.Minute)
		if err := h.DB.Create(&model.ImportSession{UserID: currentUserID(c), Module: productImportModule, FileHash: hash, TokenHash: tokenHash, ExpiresAt: expires}).Error; err != nil {
			return err
		}
		result.Token, result.ExpiresAt = token, &expires
	}
	return c.JSON(http.StatusOK, result)
}

// ImportCommit 提交产品资料 ZIP。
// @Summary 提交产品资料包
// @Tags product
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "产品资料 ZIP"
// @Param token formData string true "预览令牌"
// @Success 201 {object} ProductImportResult
// @Router /api/v1/products/import/commit [post]
func (h *Handler) ImportCommit(c *echo.Context) error {
	if h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "产品资料文件服务未配置")
	}
	token := strings.TrimSpace(c.FormValue("token"))
	if token == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "缺少预览令牌")
	}
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择产品资料 ZIP")
	}
	temp, hash, size, cleanup, err := receiveProductPackage(header)
	if err != nil {
		return err
	}
	defer cleanup()
	rows, _, errs, err := h.readProductPackage(c.Request().Context(), temp, size)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "文件校验失败，请重新预览")
	}
	var session model.ImportSession
	if err := h.DB.Where("token_hash = ? AND user_id = ? AND module = ?", hashProductToken(token), currentUserID(c), productImportModule).First(&session).Error; err != nil {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌无效，请重新预览")
	}
	if session.ConsumedAt != nil || time.Now().After(session.ExpiresAt) || session.FileHash != hash {
		return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效、文件不一致或已使用")
	}
	unlock := lockProductAssetMutation()
	defer unlock()
	staged, err := h.stageProductImages(rows, temp)
	if err != nil {
		cleanupStagedProductImages(h.StorageRoot, staged)
		return productArchiveHTTPError(err)
	}
	result := ProductImportResult{Images: len(staged), CompletedAt: time.Now()}
	oldPaths := []string{}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		consume := tx.Model(&model.ImportSession{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", session.ID, result.CompletedAt).Update("consumed_at", result.CompletedAt)
		if consume.Error != nil || consume.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "预览令牌已失效或已使用")
		}
		byModel := make(map[string]productPackageRow, len(rows))
		for _, row := range rows {
			byModel[row.Product.ProductModel] = row
		}
		for _, row := range rows {
			var current model.Product
			err := tx.Where("product_model = ?", row.Product.ProductModel).First(&current).Error
			if err == nil {
				current.CustomerModel = row.Product.CustomerModel
				current.Material = row.Product.Material
				current.InkRequired = row.Product.InkRequired
				current.Status = row.Product.Status
				if err := tx.Save(&current).Error; err != nil {
					return err
				}
				result.Updated++
			} else if errorsIsNotFound(err) {
				if err := tx.Create(&row.Product).Error; err != nil {
					return err
				}
				current = row.Product
				result.Created++
			} else {
				return err
			}
			if !row.HasDir {
				continue
			}
			var old []model.ImageFile
			if err := tx.Where("owner_type = ? AND owner_id = ?", "product", current.ID).Find(&old).Error; err != nil {
				return err
			}
			for _, asset := range old {
				oldPaths = append(oldPaths, asset.StoragePath)
				if asset.PreviewPath != "" {
					oldPaths = append(oldPaths, asset.PreviewPath)
				}
			}
			if err := tx.Unscoped().Where("owner_type = ? AND owner_id = ?", "product", current.ID).Delete(&model.ImageFile{}).Error; err != nil {
				return err
			}
		}
		for index := range staged {
			asset := &staged[index]
			row, ok := byModel[asset.ProductModel]
			if !ok {
				return fmt.Errorf("图片 %s 的产品型号不存在", asset.OriginalName)
			}
			var item model.Product
			if err := tx.Where("product_model = ?", row.Product.ProductModel).First(&item).Error; err != nil {
				return err
			}
			image := model.ImageFile{OwnerType: "product", OwnerID: item.ID, UploadedBy: currentUserID(c), OriginalName: asset.OriginalName, Size: asset.Size, MimeType: asset.MIME, Extension: asset.Extension, StoragePath: asset.StoragePath, PreviewPath: asset.PreviewPath, PreviewMime: asset.PreviewMIME, PreviewSize: asset.PreviewSize, SortOrder: index}
			if err := tx.Create(&image).Error; err != nil {
				return err
			}
		}
		return queueImageCleanup(tx, oldPaths)
	})
	if err != nil {
		cleanupStagedProductImages(h.StorageRoot, staged)
		return err
	}
	cleanupImagePaths(h.StorageRoot, h.DB, oldPaths)
	cleanupProductImageTemps(staged)
	return c.JSON(http.StatusCreated, result)
}

// Export 导出产品资料 ZIP。
// @Summary 导出产品资料包
// @Tags product
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /api/v1/products/export [get]
func (h *Handler) Export(c *echo.Context) error {
	if h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "产品资料文件服务未配置")
	}
	var products []model.Product
	if err := h.DB.Order("product_model asc, id asc").Find(&products).Error; err != nil {
		return err
	}
	rows := make([][]string, 0, len(products))
	imageByProduct := make(map[uint][]model.ImageFile, len(products))
	for _, item := range products {
		var images []model.ImageFile
		if err := h.DB.Where("owner_type = ? AND owner_id = ?", "product", item.ID).Order("sort_order asc, id asc").Find(&images).Error; err != nil {
			return err
		}
		imageByProduct[item.ID] = images
		rows = append(rows, []string{strconv.FormatUint(uint64(item.ID), 10), item.ProductModel, item.CustomerModel, item.Material, boolLabel(item.InkRequired), statusLabel(item.Status), strconv.Itoa(len(images))})
	}
	xlsx, err := spreadsheet.XLSXWriter{}.Write(c.Request().Context(), spreadsheet.SpreadsheetDocument{SheetName: "产品资料", Title: "博邦产品资料", Columns: productColumns, Rows: rows, TotalRows: int64(len(rows))})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "bb-products-export-*.zip")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	zw := zip.NewWriter(temp)
	if err := addProductZipBytes(zw, "products.xlsx", xlsx); err != nil {
		temp.Close()
		return err
	}
	for _, item := range products {
		if err := addProductZipDirectory(zw, item.ProductModel); err != nil {
			temp.Close()
			return err
		}
		for _, image := range imageByProduct[item.ID] {
			archiveName := filepath.ToSlash(filepath.Join(item.ProductModel, uniqueArchiveName(image.OriginalName, image.ID)))
			if err := addProductStoredFile(zw, h.StorageRoot, archiveName, image.StoragePath); err != nil {
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
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="bobang-products.zip"`)
	return c.Stream(http.StatusOK, "application/zip", io.LimitReader(temp, info.Size()))
}

func (h *Handler) readProductPackage(ctx context.Context, path string, size int64) ([]productPackageRow, []string, []spreadsheet.CellError, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "ZIP 资料包无法打开")
	}
	defer reader.Close()
	var workbook *zip.File
	images := make([]string, 0)
	dirs := map[string]bool{}
	for _, item := range reader.File {
		name := strings.TrimPrefix(filepath.ToSlash(item.Name), "./")
		if item.FileInfo().IsDir() {
			parts := strings.Split(strings.Trim(name, "/"), "/")
			if len(parts) > 0 && parts[0] != "" {
				dirs[parts[0]] = true
			}
			continue
		}
		if name == "products.xlsx" {
			workbook = item
			continue
		}
		if !strings.Contains(name, "/") {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if strings.TrimSpace(parts[0]) == "" {
			continue
		}
		dirs[parts[0]] = true
		if filemodule.AllowedImageExtension(filepath.Ext(name)) {
			if item.UncompressedSize64 > maxProductEntrySize {
				return nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "单张图片超过 512 MiB")
			}
			images = append(images, name)
		}
	}
	if workbook == nil {
		return nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "缺少 products.xlsx")
	}
	if workbook.UncompressedSize64 > maxProductWorkbookSize {
		return nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "products.xlsx 超过 64 MiB")
	}
	rc, err := workbook.Open()
	if err != nil {
		return nil, nil, nil, err
	}
	defer rc.Close()
	sheetRows, err := spreadsheet.XLSXReader{}.Read(ctx, rc, spreadsheet.ReadOptions{})
	if err != nil {
		return nil, nil, nil, echo.NewHTTPError(http.StatusBadRequest, "读取 products.xlsx 失败")
	}
	rows, errs := decodeProductRows(sheetRows, dirs)
	sort.Strings(images)
	return rows, images, errs, nil
}

func decodeProductRows(sheetRows [][]string, dirs map[string]bool) ([]productPackageRow, []spreadsheet.CellError) {
	if len(sheetRows) == 0 {
		return nil, []spreadsheet.CellError{{Row: 1, Column: "文件", Reason: "工作簿为空"}}
	}
	headerIndex := 0
	if len(sheetRows) > 1 && strings.TrimSpace(cellAt(sheetRows[0], 0)) == "序号" {
		headerIndex = 0
	} else if len(sheetRows) > 1 && strings.TrimSpace(cellAt(sheetRows[1], 0)) == "序号" {
		headerIndex = 1
	}
	header := sheetRows[headerIndex]
	index := map[string]int{}
	for i, value := range header {
		index[strings.TrimSpace(value)] = i
	}
	required := []string{"产品型号", "客户型号", "产品材料", "是否刷墨", "状态"}
	for _, name := range required {
		if _, ok := index[name]; !ok {
			return nil, []spreadsheet.CellError{{Row: headerIndex + 1, Column: name, Reason: "缺少必需列"}}
		}
	}
	rows := make([]productPackageRow, 0)
	errs := make([]spreadsheet.CellError, 0)
	seen := map[string]bool{}
	for rowIndex := headerIndex + 1; rowIndex < len(sheetRows); rowIndex++ {
		row := sheetRows[rowIndex]
		if strings.TrimSpace(cellAt(row, index["产品型号"])) == "" && strings.TrimSpace(cellAt(row, index["客户型号"])) == "" {
			continue
		}
		productModel := strings.TrimSpace(cellAt(row, index["产品型号"]))
		if productModel == "" {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "产品型号", Reason: "不能为空"})
			continue
		}
		if strings.ContainsAny(productModel, `/\\`) {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "产品型号", Value: productModel, Reason: "不能包含斜杠"})
			continue
		}
		if seen[productModel] {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "产品型号", Value: productModel, Reason: "文件内重复"})
			continue
		}
		seen[productModel] = true
		status := strings.TrimSpace(cellAt(row, index["状态"]))
		if status == "" {
			status = model.StatusActive
		}
		switch status {
		case "启用":
			status = model.StatusActive
		case "停用":
			status = model.StatusDisabled
		}
		if status != model.StatusActive && status != model.StatusDisabled {
			errs = append(errs, spreadsheet.CellError{Row: rowIndex + 1, Column: "状态", Value: status, Reason: "只能填写启用或停用"})
			continue
		}
		rows = append(rows, productPackageRow{
			Product: model.Product{ProductModel: productModel, CustomerModel: strings.TrimSpace(cellAt(row, index["客户型号"])), Material: strings.TrimSpace(cellAt(row, index["产品材料"])), InkRequired: parseBool(cellAt(row, index["是否刷墨"])), Status: status},
			HasDir:  dirs[productModel],
		})
	}
	for directory := range dirs {
		if !seen[directory] {
			errs = append(errs, spreadsheet.CellError{Row: 0, Column: "目录", Value: directory, Reason: "目录没有对应的产品型号行"})
		}
	}
	return rows, errs
}

func (h *Handler) stageProductImages(rows []productPackageRow, packagePath string) ([]stagedProductImage, error) {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := reader.File
	known := map[string]bool{}
	for _, row := range rows {
		known[row.Product.ProductModel] = true
	}
	staged := make([]stagedProductImage, 0, len(entries))
	for _, entry := range entries {
		if entry.FileInfo().IsDir() || !filemodule.AllowedImageExtension(filepath.Ext(entry.Name)) {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(entry.Name), "./")
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || !known[parts[0]] {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		f, err := entry.Open()
		if err != nil {
			return staged, err
		}
		temp, err := os.CreateTemp("", "bb-product-import-*"+ext)
		if err != nil {
			f.Close()
			return staged, err
		}
		written, copyErr := io.Copy(temp, io.LimitReader(f, int64(maxProductEntrySize)+1))
		closeErr := temp.Close()
		f.Close()
		if copyErr != nil || closeErr != nil || uint64(written) > maxProductEntrySize {
			os.Remove(temp.Name())
			return staged, echo.NewHTTPError(http.StatusBadRequest, "产品图片读取失败或超过 512 MiB")
		}
		if err := filemodule.ValidateStaticImage(written, ext, func() (io.ReadCloser, error) { return os.Open(temp.Name()) }); err != nil {
			os.Remove(temp.Name())
			return staged, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("图片 %s 校验失败：%s", entry.Name, err.Error()))
		}
		now := time.Now()
		randomName, err := randomProductFile(ext)
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		relative := filepath.ToSlash(filepath.Join("product", now.Format("2006"), now.Format("01"), randomName))
		preview, previewMIME, err := filemodule.MakeStaticPreviewFile(temp.Name(), ext)
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		previewRandom, err := randomProductFile(".jpg")
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		previewRelative := filepath.ToSlash(filepath.Join("product", now.Format("2006"), now.Format("01"), "preview-"+previewRandom))
		storagePath, err := productStoragePath(h.StorageRoot, relative)
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		previewPath, err := productStoragePath(h.StorageRoot, previewRelative)
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		src, err := os.Open(temp.Name())
		if err != nil {
			os.Remove(temp.Name())
			return staged, err
		}
		dst, err := os.OpenFile(storagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, err = io.Copy(dst, src)
			if closeErr := dst.Close(); err == nil {
				err = closeErr
			}
		}
		src.Close()
		if err != nil {
			os.Remove(temp.Name())
			os.Remove(storagePath)
			return staged, err
		}
		if err := os.WriteFile(previewPath, preview, 0o644); err != nil {
			os.Remove(temp.Name())
			os.Remove(storagePath)
			return staged, err
		}
		staged = append(staged, stagedProductImage{ProductModel: parts[0], OriginalName: filepath.Base(name), Extension: ext, MIME: filemodule.ImageMIMEForExtension(ext), Size: written, TempPath: temp.Name(), StoragePath: relative, PreviewPath: previewRelative, PreviewMIME: previewMIME, PreviewSize: int64(len(preview))})
	}
	return staged, nil
}

func cleanupProductImageTemps(items []stagedProductImage) {
	for _, item := range items {
		_ = os.Remove(item.TempPath)
	}
}

func cleanupStagedProductImages(root string, items []stagedProductImage) {
	for _, item := range items {
		_ = os.Remove(item.TempPath)
		if path, err := productStoragePath(root, item.StoragePath); err == nil {
			_ = os.Remove(path)
		}
		if path, err := productStoragePath(root, item.PreviewPath); err == nil {
			_ = os.Remove(path)
		}
	}
}

func cleanupImagePaths(root string, db *gorm.DB, paths []string) {
	filemodule.CleanupStoredPaths(root, db, paths)
}

func receiveProductPackage(header *multipart.FileHeader) (string, string, int64, func(), error) {
	if header == nil || header.Size <= 0 {
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "文件不能为空")
	}
	if header.Size > MaxProductPackageSize {
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "产品资料包超过 2 GiB")
	}
	src, err := header.Open()
	if err != nil {
		return "", "", 0, func() {}, err
	}
	defer src.Close()
	temp, err := os.CreateTemp("", "bb-products-import-*.zip")
	if err != nil {
		return "", "", 0, func() {}, err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(src, MaxProductPackageSize+1))
	closeErr := temp.Close()
	if err != nil || closeErr != nil || written > MaxProductPackageSize {
		os.Remove(temp.Name())
		return "", "", 0, func() {}, echo.NewHTTPError(http.StatusBadRequest, "产品资料包读取失败或超过 2 GiB")
	}
	return temp.Name(), hex.EncodeToString(hash.Sum(nil)), written, func() { _ = os.Remove(temp.Name()) }, nil
}

func productArchiveHTTPError(err error) error { return err }

func currentUserID(c *echo.Context) uint {
	if current := auth.GetCurrentUser(c); current != nil {
		return current.ID
	}
	return 0
}

func addProductZipBytes(zw *zip.Writer, name string, data []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func addProductZipDirectory(zw *zip.Writer, name string) error {
	_, err := zw.Create(strings.TrimSuffix(name, "/") + "/")
	return err
}

func addProductStoredFile(zw *zip.Writer, root, archiveName, relative string) error {
	path, err := productStoragePath(root, relative)
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

func productStoragePath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !strings.HasPrefix(filepath.ToSlash(clean), "product/") {
		return "", fmt.Errorf("非法产品文件路径")
	}
	return filepath.Join(root, clean), nil
}

func randomProductFile(ext string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw) + ext, nil
}

func newProductToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashProductToken(token), nil
}

func hashProductToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "是", "true", "1", "刷墨":
		return true
	default:
		return false
	}
}

func boolLabel(value bool) string {
	if value {
		return "是"
	}
	return "否"
}
func statusLabel(value string) string {
	if value == model.StatusDisabled {
		return "停用"
	}
	return "启用"
}
func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}
func errorsIsNotFound(err error) bool { return err != nil && err == gorm.ErrRecordNotFound }

func uniqueArchiveName(name string, id uint) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." {
		base = "image"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s-%d%s", stem, id, ext)
}

func sendProductDownload(c *echo.Context, name, contentType string, reader io.Reader, size int64) error {
	spreadsheet.DownloadHeaders(c.Response().Header(), name, contentType, size)
	return c.Stream(http.StatusOK, contentType, reader)
}
