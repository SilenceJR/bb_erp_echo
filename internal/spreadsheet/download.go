package spreadsheet

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// DownloadHeaders 为模板和导出文件提供统一的 HTTP 下载响应边界。
//
// 文件名只允许使用路径的最后一段，并过滤控制字符，避免把用户可控内容
// 直接放入 Content-Disposition。UTF-8 文件名通过 filename* 传递，同时保留
// 一个 ASCII fallback，兼容较旧的浏览器和 WebView。
func DownloadHeaders(headers http.Header, name, contentType string, size int64) {
	if headers == nil {
		return
	}
	filename := SafeDownloadFilename(name)
	headers.Set("Content-Disposition", ContentDisposition(filename))
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	if size >= 0 {
		headers.Set("Content-Length", formatContentLength(size))
	}
	// 模板与业务资料包均由用户主动下载，不应被缓存或作为脚本内容解析。
	headers.Set("Cache-Control", "no-store")
	headers.Set("X-Content-Type-Options", "nosniff")
}

// SafeDownloadFilename 将下载名限制为安全的最后路径段。
func SafeDownloadFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == ".." || name == "" {
		return "download"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsControl(r), r == '"', r == '\\':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}

// ContentDisposition 生成 attachment 响应头，UTF-8 文件名使用 RFC 5987。
func ContentDisposition(filename string) string {
	filename = SafeDownloadFilename(filename)
	fallback := asciiFallback(filename)
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + url.PathEscape(filename)
}

func asciiFallback(filename string) string {
	var b strings.Builder
	for _, r := range filename {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "download"
	}
	return b.String()
}

func formatContentLength(size int64) string {
	return strconv.FormatInt(size, 10)
}
