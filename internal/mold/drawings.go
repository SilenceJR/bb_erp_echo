package mold

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const MaxDrawingSize int64 = 512 << 20

var ErrInvalidDrawing = errors.New("仅支持 DWG 或 FDWG 文件")

// ListDrawings 查询模具 DWG/FDWG 文件。
// @Summary 查询模具图纸
// @Tags mold
// @Security BearerAuth
// @Produce json
// @Param id path int true "模具 ID"
// @Success 200 {array} model.MoldDrawing
// @Router /api/v1/molds/{id}/drawings [get]
func (h *Handler) ListDrawings(c *echo.Context) error {
	if h.DB == nil {
		return echo.NewHTTPError(http.StatusNotImplemented, "图纸服务未配置")
	}
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	if err := h.ensureMold(id); err != nil {
		return err
	}
	var items []model.MoldDrawing
	if err := h.DB.Where("mold_id = ?", id).Order("id asc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// UploadDrawing 上传模具 DWG/FDWG 文件。
// @Summary 上传模具图纸
// @Tags mold
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "模具 ID"
// @Param file formData file true "DWG 或 FDWG 文件"
// @Success 201 {object} model.MoldDrawing
// @Router /api/v1/molds/{id}/drawings [post]
func (h *Handler) UploadDrawing(c *echo.Context) error {
	if h.DB == nil || h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "图纸服务未配置")
	}
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	if err := h.ensureMold(id); err != nil {
		return err
	}
	header, err := c.FormFile("file")
	if err != nil || header == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请选择 DWG 文件")
	}
	item, err := saveDrawing(h.StorageRoot, h.DB, header, id, auth.GetCurrentUser(c).ID)
	if err != nil {
		return drawingHTTPError(err)
	}
	return c.JSON(http.StatusCreated, item)
}

// DrawingContent 下载模具 DWG/FDWG 文件。
// @Summary 下载模具图纸
// @Tags mold
// @Security BearerAuth
// @Produce application/octet-stream
// @Param id path int true "模具 ID"
// @Param drawing_id path int true "图纸 ID"
// @Success 200 {file} binary
// @Router /api/v1/molds/{id}/drawings/{drawing_id}/content [get]
func (h *Handler) DrawingContent(c *echo.Context) error {
	if h.DB == nil || h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "图纸服务未配置")
	}
	moldID, err := request.ParamID(c)
	if err != nil {
		return err
	}
	drawingID, err := strconv.ParseUint(c.Param("drawing_id"), 10, 64)
	if err != nil || drawingID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "图纸 ID 无效")
	}
	var item model.MoldDrawing
	if err := h.DB.Where("id = ? AND mold_id = ?", drawingID, moldID).First(&item).Error; err != nil {
		return drawingHTTPError(err)
	}
	path, err := drawingPath(h.StorageRoot, item.StoragePath)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return echo.NewHTTPError(http.StatusNotFound, "图纸文件不存在")
		}
		return err
	}
	defer f.Close()
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+safeFilename(item.OriginalName)+`"`)
	c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(item.Size, 10))
	return c.Stream(http.StatusOK, item.MimeType, f)
}

// DeleteDrawing 删除模具 DWG/FDWG 文件。
// @Summary 删除模具图纸
// @Tags mold
// @Security BearerAuth
// @Param id path int true "模具 ID"
// @Param drawing_id path int true "图纸 ID"
// @Success 204
// @Router /api/v1/molds/{id}/drawings/{drawing_id} [delete]
func (h *Handler) DeleteDrawing(c *echo.Context) error {
	if h.DB == nil || h.StorageRoot == "" {
		return echo.NewHTTPError(http.StatusNotImplemented, "图纸服务未配置")
	}
	moldID, err := request.ParamID(c)
	if err != nil {
		return err
	}
	drawingID, err := strconv.ParseUint(c.Param("drawing_id"), 10, 64)
	if err != nil || drawingID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "图纸 ID 无效")
	}
	var item model.MoldDrawing
	if err := h.DB.Where("id = ? AND mold_id = ?", drawingID, moldID).First(&item).Error; err != nil {
		return drawingHTTPError(err)
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error { return tx.Unscoped().Delete(&item).Error }); err != nil {
		return err
	}
	path, err := drawingPath(h.StorageRoot, item.StoragePath)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func saveDrawing(root string, db *gorm.DB, header *multipart.FileHeader, moldID, uploadedBy uint) (model.MoldDrawing, error) {
	if header.Size <= 0 || header.Size > MaxDrawingSize {
		return model.MoldDrawing{}, fmt.Errorf("图纸大小不能超过 %d MiB", MaxDrawingSize>>20)
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".dwg" && ext != ".fdwg" {
		return model.MoldDrawing{}, ErrInvalidDrawing
	}
	src, err := header.Open()
	if err != nil {
		return model.MoldDrawing{}, err
	}
	defer src.Close()
	dir := filepath.Join(root, "mold", "drawings", time.Now().Format("2006"), time.Now().Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return model.MoldDrawing{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return model.MoldDrawing{}, err
	}
	relative := filepath.ToSlash(filepath.Join("mold", "drawings", time.Now().Format("2006"), time.Now().Format("01"), fmt.Sprintf("%x%s", random, ext)))
	path, err := drawingPath(root, relative)
	if err != nil {
		return model.MoldDrawing{}, err
	}
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return model.MoldDrawing{}, err
	}
	written, copyErr := io.Copy(dst, io.LimitReader(src, MaxDrawingSize+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil || written > MaxDrawingSize {
		_ = os.Remove(path)
		if copyErr != nil {
			return model.MoldDrawing{}, copyErr
		}
		if closeErr != nil {
			return model.MoldDrawing{}, closeErr
		}
		return model.MoldDrawing{}, fmt.Errorf("图纸大小不能超过 %d MiB", MaxDrawingSize>>20)
	}
	item := model.MoldDrawing{MoldID: moldID, UploadedBy: uploadedBy, OriginalName: filepath.Base(header.Filename), Size: written, MimeType: "application/octet-stream", Extension: ext, StoragePath: relative}
	if err := db.Create(&item).Error; err != nil {
		_ = os.Remove(path)
		return model.MoldDrawing{}, err
	}
	return item, nil
}

func (h *Handler) ensureMold(id uint) error {
	var item model.Mold
	if err := h.DB.First(&item, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
	} else if err != nil {
		return err
	}
	return nil
}

func drawingPath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !strings.HasPrefix(filepath.ToSlash(clean), "mold/drawings/") {
		return "", errors.New("非法图纸路径")
	}
	return filepath.Join(root, clean), nil
}

func safeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, `"`, "'")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	return name
}

func drawingHTTPError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "图纸不存在")
	}
	if errors.Is(err, ErrInvalidDrawing) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return err
}
