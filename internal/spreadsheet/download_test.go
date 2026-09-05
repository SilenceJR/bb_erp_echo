package spreadsheet

import (
	"net/http"
	"strings"
	"testing"
)

func TestDownloadHeadersUseSafeUTF8Disposition(t *testing.T) {
	headers := make(http.Header)
	DownloadHeaders(headers, "../博邦\r\n资料包.zip", "application/zip", 42)
	if got := headers.Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q", got)
	}
	if got := headers.Get("Content-Length"); got != "42" {
		t.Fatalf("content length = %q", got)
	}
	contentDisposition := headers.Get("Content-Disposition")
	if !strings.HasPrefix(contentDisposition, "attachment; filename=\"") || !strings.Contains(contentDisposition, "filename*=UTF-8''") {
		t.Fatalf("unsafe or incomplete content disposition = %q", contentDisposition)
	}
	if strings.ContainsAny(contentDisposition, "\r\n") || strings.Contains(contentDisposition, "../") {
		t.Fatalf("content disposition contains unsafe input = %q", contentDisposition)
	}
	if headers.Get("Cache-Control") != "no-store" || headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers = %#v", headers)
	}
}

func TestSafeDownloadFilenameUsesBasename(t *testing.T) {
	if got := SafeDownloadFilename(`/tmp/资料.xlsx`); got != "资料.xlsx" {
		t.Fatalf("safe filename = %q", got)
	}
	if got := SafeDownloadFilename("资料\r\n.zip"); got != "资料__.zip" {
		t.Fatalf("control filename = %q", got)
	}
}
