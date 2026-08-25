// Package frontend 负责把 Web 管理端构建产物挂载到 Echo。
package frontend

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"bb_erp_echo/internal/config"

	"github.com/labstack/echo/v5"
)

// RegisterStatic 注册 Web 管理端静态文件路由。
//
// 参数说明：
// - e：Echo 实例。
// - cfg：Web 静态文件配置。
//
// 行为说明：
// - /api/* 不会进入前端 fallback，避免错误接口被 index.html 吃掉。
// - 已存在的静态文件会直接返回。
// - 不存在且非 API 的 GET 路径返回 index.html，支持前端路由刷新。
func RegisterStatic(e *echo.Echo, cfg config.WebConfig) {
	if !cfg.Enabled {
		return
	}
	distDir := cfg.DistDir
	if distDir == "" {
		distDir = "web/dist"
	}
	fileSystem := os.DirFS(resolveDistDir(distDir))
	e.GET("/*", spaHandler(fileSystem))
}

// resolveDistDir 解析 Web 构建产物目录。
//
// 参数说明：
// - distDir：配置中的静态文件目录，可以是绝对路径，也可以是相对路径。
//
// 返回说明：
// - 返回可传给 os.DirFS 的目录路径。
//
// 业务说明：
// air 如果从 cmd/server 目录启动，默认 web/dist 会被解析为 cmd/server/web/dist。
// 为了让“后端启动时同时启动 Web 静态托管”更稳定，这里会在当前目录和上级目录中查找 web/dist。
func resolveDistDir(distDir string) string {
	if filepath.IsAbs(distDir) || dirExists(distDir) {
		return distDir
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return distDir
	}
	current := workingDir
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(current, distDir)
		if dirExists(candidate) {
			return candidate
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return distDir
}

// dirExists 判断指定路径是否存在且是目录。
//
// 参数说明：
// - dir：待检查目录。
func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func spaHandler(fileSystem fs.FS) echo.HandlerFunc {
	return func(c *echo.Context) error {
		requestPath := c.Request().URL.Path
		if isBackendPath(requestPath) {
			return echo.NewHTTPError(http.StatusNotFound, "接口不存在")
		}

		fileName := cleanFileName(requestPath)
		if fileName == "." || fileName == "/" {
			fileName = "index.html"
		}

		if err := serveFile(c, fileSystem, fileName); err == nil {
			return nil
		}

		if _, err := fs.Stat(fileSystem, "index.html"); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "Web 管理端静态文件不存在，请先构建 web/dist")
		}
		return serveFile(c, fileSystem, "index.html")
	}
}

// serveFile 从 fs.FS 中读取文件并通过 http.ServeContent 写入响应。
func serveFile(c *echo.Context, fileSystem fs.FS, name string) error {
	f, err := fileSystem.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if stat.IsDir() {
		return echo.NewHTTPError(http.StatusForbidden, "不允许访问目录")
	}

	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(c.Response(), c.Request(), stat.Name(), stat.ModTime(), rs)
		return nil
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	http.ServeContent(c.Response(), c.Request(), stat.Name(), stat.ModTime(), bytes.NewReader(content))
	return nil
}

func cleanFileName(requestPath string) string {
	return path.Clean(strings.TrimPrefix(requestPath, "/"))
}

func isBackendPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/api/") ||
		requestPath == "/api" ||
		requestPath == "/health" ||
		requestPath == "/ready"
}
