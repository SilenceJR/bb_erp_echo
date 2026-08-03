// Package update 提供版本清单读取、更新包缓存和客户端下载能力。
package update

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"bb_erp_echo/internal/config"
)

const clientPackageName = "bb-erp-client-windows.zip"

// Manifest 是 GitHub、Gitee 或内网静态服务提供的更新清单。
type Manifest struct {
	Version     string          `json:"version"`
	PublishedAt time.Time       `json:"published_at,omitempty"`
	Notes       string          `json:"notes,omitempty"`
	Server      PackageManifest `json:"server"`
	Client      PackageManifest `json:"client"`
	AllInOne    PackageManifest `json:"all_in_one,omitempty"`
}

// PackageManifest 描述一个可下载升级包。
type PackageManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// ClientUpdateStatus 是服务端暴露给 Tauri 客户端的升级状态。
type ClientUpdateStatus struct {
	CurrentVersion string           `json:"current_version"`
	LatestVersion  string           `json:"latest_version,omitempty"`
	Available      bool             `json:"available"`
	Cached         bool             `json:"cached"`
	FileName       string           `json:"file_name,omitempty"`
	DownloadPath   string           `json:"download_path,omitempty"`
	CheckedAt      *time.Time       `json:"checked_at,omitempty"`
	Manifest       *PackageManifest `json:"manifest,omitempty"`
	Message        string           `json:"message,omitempty"`
}

// Manager 管理远端清单检查和本机更新包缓存。
type Manager struct {
	Config config.UpdateConfig
	Client *http.Client
}

// NewManager 创建更新管理器。
func NewManager(cfg config.UpdateConfig) *Manager {
	return &Manager{
		Config: cfg,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

// NewManagerForURL 创建只用于命令行检查的更新管理器。
func NewManagerForURL(manifestURL string) *Manager {
	return NewManager(config.UpdateConfig{
		Enabled:     true,
		ManifestURL: manifestURL,
		CacheDir:    "updates",
	})
}

// FetchManifest 从配置的 URL 读取更新清单。
func (m *Manager) FetchManifest() (*Manifest, error) {
	if !m.Config.Enabled {
		return nil, errors.New("update check is disabled")
	}
	if strings.TrimSpace(m.Config.ManifestURL) == "" {
		return nil, errors.New("update manifest url is empty")
	}

	req, err := http.NewRequest(http.MethodGet, m.Config.ManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch manifest status %d", res.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(res.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return &manifest, nil
}

// CheckAndCacheClientUpdate 检查远端客户端包，并在有新版本时下载到服务端缓存。
func (m *Manager) CheckAndCacheClientUpdate() (ClientUpdateStatus, error) {
	manifest, err := m.FetchManifest()
	if err != nil {
		return ClientUpdateStatus{CurrentVersion: m.Config.ClientVersion, Message: err.Error()}, err
	}

	status := m.StatusFromManifest(manifest)
	if !status.Available || manifest.Client.URL == "" {
		return status, nil
	}

	cachePath, err := m.downloadClientPackage(manifest.Client)
	if err != nil {
		status.Message = err.Error()
		return status, err
	}
	status.Cached = true
	status.FileName = filepath.Base(cachePath)
	status.DownloadPath = "/api/v1/updates/client/download"
	return status, nil
}

// StatusFromManifest 根据远端清单计算客户端升级状态。
func (m *Manager) StatusFromManifest(manifest *Manifest) ClientUpdateStatus {
	now := time.Now()
	status := ClientUpdateStatus{
		CurrentVersion: m.Config.ClientVersion,
		LatestVersion:  manifest.Client.Version,
		Available:      CompareVersions(manifest.Client.Version, m.Config.ClientVersion) > 0,
		CheckedAt:      &now,
		Manifest:       &manifest.Client,
	}
	if m.cachedClientPackageExists() {
		status.Cached = true
		status.FileName = clientPackageName
		status.DownloadPath = "/api/v1/updates/client/download"
	}
	return status
}

// CachedClientStatus 返回本机已缓存的客户端升级包状态。
func (m *Manager) CachedClientStatus() ClientUpdateStatus {
	status := ClientUpdateStatus{CurrentVersion: m.Config.ClientVersion}
	if m.cachedClientPackageExists() {
		status.Available = true
		status.Cached = true
		status.FileName = clientPackageName
		status.DownloadPath = "/api/v1/updates/client/download"
	}
	return status
}

// CachedClientPackagePath 返回本机缓存客户端包路径。
func (m *Manager) CachedClientPackagePath() string {
	return filepath.Join(m.Config.CacheDir, "client", clientPackageName)
}

func (m *Manager) downloadClientPackage(pkg PackageManifest) (string, error) {
	if pkg.URL == "" {
		return "", errors.New("client package url is empty")
	}
	if err := os.MkdirAll(filepath.Dir(m.CachedClientPackagePath()), 0o755); err != nil {
		return "", fmt.Errorf("create update cache directory: %w", err)
	}

	tmpPath := m.CachedClientPackagePath() + ".tmp"
	if err := downloadFile(m.Client, pkg.URL, tmpPath); err != nil {
		return "", err
	}
	if pkg.SHA256 != "" {
		if err := verifySHA256(tmpPath, pkg.SHA256); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
	}
	if err := validateZip(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, m.CachedClientPackagePath()); err != nil {
		return "", fmt.Errorf("store client package: %w", err)
	}
	return m.CachedClientPackagePath(), nil
}

func (m *Manager) cachedClientPackageExists() bool {
	info, err := os.Stat(m.CachedClientPackagePath())
	return err == nil && !info.IsDir() && info.Size() > 0
}

func downloadFile(client *http.Client, url string, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download package: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("download package status %d", res.StatusCode)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create package file: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, res.Body); err != nil {
		return fmt.Errorf("write package file: %w", err)
	}
	return nil
}

func verifySHA256(path string, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open package for hash: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash package: %w", err)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("package sha256 mismatch: got %s", got)
	}
	return nil
}

func validateZip(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("validate client zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("client zip is empty")
	}
	return nil
}

// CompareVersions 比较常见 v1.2.3 版本号。
func CompareVersions(left string, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	for i := 0; i < len(leftParts) || i < len(rightParts); i++ {
		var l, r int
		if i < len(leftParts) {
			l = leftParts[i]
		}
		if i < len(rightParts) {
			r = rightParts[i]
		}
		if l > r {
			return 1
		}
		if l < r {
			return -1
		}
	}
	return 0
}

func versionParts(value string) []int {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		var part int
		for _, r := range field {
			if r < '0' || r > '9' {
				break
			}
			part = part*10 + int(r-'0')
		}
		parts = append(parts, part)
	}
	return parts
}

// RuntimePackageSuffix 返回当前平台产物后缀，便于 manifest 生成时复用。
func RuntimePackageSuffix() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}
