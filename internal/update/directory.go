package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"bb_erp_echo/internal/shared/jsonstrict"
)

// DirectoryManifestSource 从已激活的本地发布目录读取更新清单。
//
// URL 字段在此模式下不是 URL，而是相对于 Root 的普通文件路径。所有
// 资源在读取前都会经过路径和文件类型检查，因此该来源不会创建网络请求。
type DirectoryManifestSource struct {
	Root string
}

// DirectoryPackageStore 是 directory 更新源使用的本地包缓存实现。
// 它保留 LocalPackageStore 的缓存和校验行为，但从 ReleaseDir 原子复制资源，
// 而不是调用 HTTP 客户端下载。
type DirectoryPackageStore = LocalPackageStore

// DirectoryArtifactStore 是 directory 更新源使用的本地客户端资源缓存实现。
type DirectoryArtifactStore = LocalArtifactStore

// NewDirectoryManifestSource 创建本地目录清单来源。
func NewDirectoryManifestSource(root string) *DirectoryManifestSource {
	return &DirectoryManifestSource{Root: root}
}

// Fetch 读取并严格解析 active 目录下的 update-manifest.json。
func (s *DirectoryManifestSource) Fetch(ctx context.Context) (*Manifest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := releaseRoot(s.Root)
	if err != nil {
		return nil, err
	}
	readyPath, err := resolveReleaseFile(root, ".release-ready")
	if err != nil {
		return nil, fmt.Errorf("local update release is not activated: %w", err)
	}
	readyVersion, err := os.ReadFile(readyPath)
	if err != nil {
		return nil, fmt.Errorf("read local update activation marker: %w", err)
	}
	manifestPath, err := resolveReleaseFile(root, "update-manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read local update manifest: %w", err)
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("open local update manifest: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxManifestSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read local update manifest: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close local update manifest: %w", closeErr)
	}
	if len(raw) > maxManifestSize {
		return nil, fmt.Errorf("read manifest: response exceeds %d bytes", maxManifestSize)
	}
	manifest, err := parseManifest(raw)
	if err != nil {
		return nil, err
	}
	if err := validateDirectoryManifest(root, manifest); err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(readyVersion)) != strings.TrimSpace(manifest.Version) {
		return nil, errors.New("local update activation marker version does not match manifest version")
	}
	return manifest, nil
}

// Location 返回不含公网 URL 的来源标识。
func (s *DirectoryManifestSource) Location() string {
	root := strings.TrimSpace(s.Root)
	if root == "" {
		return "directory://"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "directory://"
	}
	return "directory://" + filepath.ToSlash(filepath.Clean(abs))
}

// parseManifest 在 HTTP 和 directory 来源之间共享 JSON 契约校验。
func parseManifest(raw []byte) (*Manifest, error) {
	if err := jsonstrict.RejectDuplicateKeys(raw); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode manifest: trailing JSON content is not allowed")
	}
	if manifest.Server.Version == "" && manifest.ClientUpdateV2 == nil {
		return nil, errors.New("manifest contains no package version")
	}
	return &manifest, nil
}

// validateDirectoryManifest 验证清单及签名 payload 中的资源引用均为已存在的
// release 文件。签名 payload 的真实性仍由 Service 使用 Minisign 再次校验；这里
// 只提前阻止恶意路径和不完整发布目录进入后续缓存流程。
func validateDirectoryManifest(root string, manifest *Manifest) error {
	if manifest == nil {
		return errors.New("local update manifest is empty")
	}
	packages := []struct {
		name string
		url  string
	}{
		{name: "server", url: manifest.Server.URL},
		{name: "all_in_one", url: manifest.AllInOne.URL},
		{name: "updater", url: manifest.Updater.URL},
	}
	for _, pkg := range packages {
		if strings.TrimSpace(pkg.url) == "" {
			continue
		}
		if _, err := resolveReleaseFile(root, pkg.url); err != nil {
			return fmt.Errorf("local update manifest %s resource is invalid: %w", pkg.name, err)
		}
	}
	if manifest.ClientUpdateV2 == nil {
		return nil
	}
	urls, err := signedPayloadResourceURLs(manifest.ClientUpdateV2)
	if err != nil {
		return fmt.Errorf("local update manifest client payload is invalid: %w", err)
	}
	for _, resource := range urls {
		if strings.TrimSpace(resource.url) == "" {
			continue
		}
		if _, err := resolveReleaseFile(root, resource.url); err != nil {
			return fmt.Errorf("local update manifest %s resource is invalid: %w", resource.name, err)
		}
	}
	return nil
}

type signedPayloadResource struct {
	name string
	url  string
}

// signedPayloadResourceURLs 提取 payload 中的资源路径以执行目录边界检查。
// Minisign 验签不在此处重复执行，仍由 cacheFullClientArtifacts 负责。
func signedPayloadResourceURLs(envelope *SignedClientUpdateManifest) ([]signedPayloadResource, error) {
	raw, err := decodeBase64Payload(envelope.Payload)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Full struct {
			NSIS struct {
				URL string `json:"url"`
			} `json:"nsis"`
			Portable struct {
				URL string `json:"url"`
			} `json:"portable"`
		} `json:"full"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON content is not allowed")
	}
	return []signedPayloadResource{
		{name: "client full nsis", url: payload.Full.NSIS.URL},
		{name: "client full portable", url: payload.Full.Portable.URL},
	}, nil
}

// releaseRoot 解析并检查 directory 更新源根目录。根目录本身不能是符号链接，
// 以免管理员配置的边界在运行时悄然指向另一处目录。
func releaseRoot(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("update release directory is empty")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve update release directory: %w", err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("stat update release directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("update release directory must not be a symlink")
	}
	if !info.IsDir() {
		return "", errors.New("update release directory is not a directory")
	}
	return abs, nil
}

// validateRelativeResourcePath 将 manifest 中的相对引用约束为普通相对路径。
// URL scheme、host、query、fragment、反斜杠和任何 .. 段全部拒绝，避免 URL/Windows
// 路径语义不一致造成目录逃逸。
func validateRelativeResourcePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("resource path is empty")
	}
	if strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("resource path contains control characters")
	}
	if strings.Contains(raw, "\\") {
		return "", errors.New("resource path must use slash-separated relative paths")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse resource path: %w", err)
	}
	if parsed.IsAbs() || parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("resource path must be relative: %q", raw)
	}
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", fmt.Errorf("decode resource path: %w", err)
	}
	if decoded == "" || strings.ContainsAny(decoded, "\\:\x00\r\n") || strings.HasPrefix(decoded, "/") || filepath.IsAbs(decoded) || path.IsAbs(decoded) {
		return "", fmt.Errorf("resource path must be relative: %q", raw)
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == ".." {
			return "", fmt.Errorf("resource path contains parent traversal: %q", raw)
		}
	}
	clean := path.Clean(decoded)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("resource path escapes release directory: %q", raw)
	}
	return clean, nil
}

// resolveReleaseFile 解析一个 directory 资源，并逐级拒绝符号链接、非目录父级、
// 目录外路径和非普通文件。再次使用 EvalSymlinks 检查实际路径，覆盖 Windows
// junction/reparse point 等可能无法由 ModeSymlink 完整表达的情况。
func resolveReleaseFile(root, raw string) (string, error) {
	root, err := releaseRoot(root)
	if err != nil {
		return "", err
	}
	relative, err := validateRelativeResourcePath(raw)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("resource path escapes release directory: %q", raw)
	}
	current := root
	segments := strings.Split(relative, "/")
	for index, segment := range segments {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", fmt.Errorf("resource %q is unavailable: %w", raw, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("resource path contains symlink: %q", raw)
		}
		if index < len(segments)-1 {
			if !info.IsDir() {
				return "", fmt.Errorf("resource parent is not a directory: %q", raw)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("resource is not a regular file: %q", raw)
		}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve update release directory: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve resource path: %w", err)
	}
	resolvedRel, err := filepath.Rel(filepath.Clean(resolvedRoot), filepath.Clean(resolvedTarget))
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRel) {
		return "", fmt.Errorf("resource path escapes release directory: %q", raw)
	}
	return target, nil
}

// copyVerifiedReleaseFile 原子缓存 directory 资源，并在写入过程中完成大小和
// SHA-256 校验。调用者负责在成功后执行 ZIP/Minisign 等资源类型校验。
func copyVerifiedReleaseFile(ctx context.Context, releaseDir, resourcePath, destination string, size int64, wantSHA256 string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if size <= 0 || !isSHA256(wantSHA256) {
		return errors.New("local update resource requires a positive size and sha256")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sourcePath, err := resolveReleaseFile(releaseDir, resourcePath)
	if err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open local update resource: %w", err)
	}
	defer source.Close()
	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat local update resource: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() || sourceInfo.Size() != size {
		return fmt.Errorf("local update resource size mismatch: got %d want %d", sourceInfo.Size(), size)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}
	destinationFile, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create cached update resource: %w", err)
	}
	hash := sha256.New()
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(source, size+1)}
	written, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), reader)
	if closeErr := destinationFile.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("copy local update resource: %w", copyErr)
	}
	if written != size {
		return fmt.Errorf("local update resource size mismatch: got %d want %d", written, size)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(wantSHA256)) {
		return fmt.Errorf("local update resource sha256 mismatch: got %s", got)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("reopen cached update resource for sync: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync cached update resource: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close cached update resource: %w", err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
