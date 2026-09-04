package file

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

const (
	maxPreviewPixels    int64 = 32_000_000
	maxPreviewDimension       = 2560
	maxWASMSourceBytes  int64 = 128 << 20
	maxSVGSourceBytes   int64 = 8 << 20
	previewWaitTimeout        = 2 * time.Minute
)

var previewSlots = make(chan struct{}, 2)

type imageSource struct {
	size int64
	open func() (io.ReadCloser, error)
}

func makeStaticPreview(header *multipart.FileHeader, ext string) ([]byte, string, error) {
	if header == nil {
		return nil, "", validationError("文件不能为空")
	}
	return makeStaticPreviewSource(imageSource{size: header.Size, open: func() (io.ReadCloser, error) {
		return header.Open()
	}}, ext)
}

// MakeStaticPreviewFile 为受信任的内部导入流程生成与上传接口一致的静态预览。
func MakeStaticPreviewFile(path, ext string) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	return makeStaticPreviewSource(imageSource{size: info.Size(), open: func() (io.ReadCloser, error) { return os.Open(path) }}, strings.ToLower(ext))
}

// ValidateStaticImage 验证可重复打开的图片源能否按上传规则生成静态预览。
func ValidateStaticImage(size int64, ext string, open func() (io.ReadCloser, error)) error {
	_, _, err := makeStaticPreviewSource(imageSource{size: size, open: open}, strings.ToLower(ext))
	return err
}

func makeStaticPreviewSource(source imageSource, ext string) ([]byte, string, error) {
	select {
	case previewSlots <- struct{}{}:
		defer func() { <-previewSlots }()
	case <-time.After(previewWaitTimeout):
		return nil, "", validationError("图片转换任务繁忙，请稍后分批重试")
	}
	if ext == ".svg" {
		return makeSVGPreview(source)
	}
	if (ext == ".heic" || ext == ".heif" || ext == ".avif") && source.size > maxWASMSourceBytes {
		return nil, "", validationError("HEIC、HEIF、AVIF 原图超过 128 MiB，当前服务器无法安全生成预览")
	}

	cfg, err := decodeImageConfig(source, ext)
	if err != nil {
		return nil, "", validationError("图片内容无法按文件格式解析，文件可能已损坏或扩展名不正确")
	}
	if err := validatePixelSize(cfg.Width, cfg.Height); err != nil {
		return nil, "", err
	}
	img, err := decodeImage(source, ext)
	if err != nil {
		return nil, "", validationError("图片内容解码失败，文件可能已损坏、采用了暂不兼容的编码，或扩展名不正确")
	}
	if ext == ".jpg" || ext == ".jpeg" || ext == ".jfif" {
		img = orientImage(img, jpegOrientation(source))
	}
	return encodePreview(img)
}

func decodeImageConfig(source imageSource, ext string) (image.Config, error) {
	src, err := source.open()
	if err != nil {
		return image.Config{}, err
	}
	defer src.Close()
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		return jpeg.DecodeConfig(src)
	case ".png":
		return png.DecodeConfig(src)
	case ".gif":
		return gif.DecodeConfig(src)
	case ".webp":
		return webp.DecodeConfig(src)
	case ".bmp":
		return bmp.DecodeConfig(src)
	case ".tif", ".tiff":
		return tiff.DecodeConfig(src)
	case ".heic", ".heif":
		return heic.DecodeConfig(src)
	case ".avif":
		return avif.DecodeConfig(src)
	default:
		return image.Config{}, fmt.Errorf("unsupported extension %q", ext)
	}
}

func decodeImage(source imageSource, ext string) (image.Image, error) {
	src, err := source.open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		return jpeg.Decode(src)
	case ".png":
		return png.Decode(src)
	case ".gif":
		return gif.Decode(src)
	case ".webp":
		return webp.Decode(src)
	case ".bmp":
		return bmp.Decode(src)
	case ".tif", ".tiff":
		return tiff.Decode(src)
	case ".heic", ".heif":
		return heic.Decode(src)
	case ".avif":
		return avif.Decode(src, avif.Options{AutoRotate: true})
	default:
		return nil, fmt.Errorf("unsupported extension %q", ext)
	}
}

func validatePixelSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return validationError("图片像素尺寸无效")
	}
	if int64(width) > maxPreviewPixels/int64(height) {
		return validationError("图片像素总量超过 3200 万，服务器无法安全生成预览；请降低分辨率后重试")
	}
	return nil
}

func encodePreview(src image.Image) ([]byte, string, error) {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if err := validatePixelSize(width, height); err != nil {
		return nil, "", err
	}
	targetWidth, targetHeight := previewDimensions(width, height)
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", fmt.Errorf("生成静态预览失败: %w", err)
	}
	return output.Bytes(), "image/jpeg", nil
}

func previewDimensions(width, height int) (int, int) {
	if width <= maxPreviewDimension && height <= maxPreviewDimension {
		return width, height
	}
	scale := math.Min(float64(maxPreviewDimension)/float64(width), float64(maxPreviewDimension)/float64(height))
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

func makeSVGPreview(source imageSource) ([]byte, string, error) {
	if source.size > maxSVGSourceBytes {
		return nil, "", validationError("SVG 文件超过 8 MiB，服务器无法安全生成预览；请简化矢量内容后重试")
	}
	src, err := source.open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	icon, err := oksvg.ReadIconStream(io.LimitReader(src, maxSVGSourceBytes+1), oksvg.WarnErrorMode)
	if err != nil {
		return nil, "", validationError("SVG 内容无法解析，文件可能已损坏或包含不兼容元素")
	}
	width, height := int(math.Ceil(icon.ViewBox.W)), int(math.Ceil(icon.ViewBox.H))
	if err := validatePixelSize(width, height); err != nil {
		return nil, "", err
	}
	targetWidth, targetHeight := previewDimensions(width, height)
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	icon.SetTarget(0, 0, float64(targetWidth), float64(targetHeight))
	icon.Draw(rasterx.NewDasher(targetWidth, targetHeight, rasterx.NewScannerGV(targetWidth, targetHeight, dst, dst.Bounds())), 1)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", fmt.Errorf("生成 SVG 静态预览失败: %w", err)
	}
	return output.Bytes(), "image/jpeg", nil
}

func jpegOrientation(source imageSource) int {
	src, err := source.open()
	if err != nil {
		return 1
	}
	defer src.Close()
	metadata, err := exif.Decode(src)
	if err != nil {
		return 1
	}
	tag, err := metadata.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	value, err := tag.Int(0)
	if err != nil || value < 1 || value > 8 {
		return 1
	}
	return value
}

type orientedImage struct {
	source      image.Image
	orientation int
	bounds      image.Rectangle
}

func orientImage(source image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return source
	}
	b := source.Bounds()
	w, h := b.Dx(), b.Dy()
	if orientation >= 5 {
		w, h = h, w
	}
	return &orientedImage{source: source, orientation: orientation, bounds: image.Rect(0, 0, w, h)}
}

func (o *orientedImage) ColorModel() color.Model { return o.source.ColorModel() }
func (o *orientedImage) Bounds() image.Rectangle { return o.bounds }
func (o *orientedImage) At(x, y int) color.Color {
	b := o.source.Bounds()
	w, h := b.Dx(), b.Dy()
	var sx, sy int
	switch o.orientation {
	case 2:
		sx, sy = w-1-x, y
	case 3:
		sx, sy = w-1-x, h-1-y
	case 4:
		sx, sy = x, h-1-y
	case 5:
		sx, sy = y, x
	case 6:
		sx, sy = y, h-1-x
	case 7:
		sx, sy = w-1-y, h-1-x
	case 8:
		sx, sy = w-1-y, x
	default:
		sx, sy = x, y
	}
	return o.source.At(b.Min.X+sx, b.Min.Y+sy)
}
