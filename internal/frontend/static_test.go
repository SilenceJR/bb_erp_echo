package frontend

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir 保存原工作目录并切换到 dir，返回一个恢复函数。
// Go 1.20 没有 t.Chdir，需要手动保存和恢复。
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

// TestResolveDistDirFromNestedWorkingDir 验证从 cmd/server 这类子目录启动时，
// 默认 web/dist 仍然能解析到项目根目录下的前端构建产物。
func TestResolveDistDirFromNestedWorkingDir(t *testing.T) {
	rootDir := t.TempDir()
	distDir := filepath.Join(rootDir, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("create web dist: %v", err)
	}

	nestedDir := filepath.Join(rootDir, "cmd", "server")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}
	chdir(t, nestedDir)

	resolved := resolveDistDir("web/dist")
	// macOS 下 /var 是 /private/var 的符号链接，使用 EvalSymlinks 规范化后再比较。
	resolvedReal, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("eval symlinks for resolved %q: %v", resolved, err)
	}
	wantReal, err := filepath.EvalSymlinks(distDir)
	if err != nil {
		t.Fatalf("eval symlinks for want %q: %v", distDir, err)
	}
	if resolvedReal != wantReal {
		t.Fatalf("resolved dist dir = %q, want %q", resolvedReal, wantReal)
	}
}

// TestResolveDistDirKeepsMissingPath 验证目录不存在时保留原配置，
// 由实际请求阶段返回明确的静态文件不存在错误。
func TestResolveDistDirKeepsMissingPath(t *testing.T) {
	chdir(t, t.TempDir())

	const missingDir = "missing-web/dist"
	if resolved := resolveDistDir(missingDir); resolved != missingDir {
		t.Fatalf("resolved missing dir = %q, want %q", resolved, missingDir)
	}
}
