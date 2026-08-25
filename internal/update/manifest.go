// Package update 提供版本清单读取、更新包缓存和更新状态调度能力。
package update

import (
	"archive/zip"
	"context"
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
	"sync"
	"time"

	"bb_erp_echo/internal/config"

	"golang.org/x/mod/semver"
)

const clientPackageName = "bb-erp-client-windows.zip"

// Manifest 是 Gitee、GitHub、对象存储或内网静态服务提供的更新清单。
type Manifest struct {
	Version     string          `json:"version"`
	PublishedAt time.Time       `json:"published_at,omitempty"`
	Notes       string          `json:"notes,omitempty"`
	Server      PackageManifest `json:"server"`
	Client      PackageManifest `json:"client"`
	AllInOne    PackageManifest `json:"all_in_one,omitempty"`
	Updater     PackageManifest `json:"updater,omitempty"`
}

// PackageManifest 描述一个可下载升级包。
type PackageManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// ClientUpdateStatus 是服务端暴露给 Tauri 客户端的兼容升级状态。
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

// ComponentStatus 描述服务端或桌面客户端的版本状态。
type ComponentStatus struct {
	CurrentVersion string           `json:"current_version"`
	LatestVersion  string           `json:"latest_version,omitempty"`
	Available      bool             `json:"available"`
	DownloadURL    string           `json:"download_url,omitempty"`
	Size           int64            `json:"size,omitempty"`
	SHA256         string           `json:"sha256,omitempty"`
	Manifest       *PackageManifest `json:"manifest,omitempty"`
}

// ClientComponentStatus 在组件状态上增加服务端缓存信息。
type ClientComponentStatus struct {
	ComponentStatus
	Cached       bool   `json:"cached"`
	FileName     string `json:"file_name,omitempty"`
	DownloadPath string `json:"download_path,omitempty"`
}

// SystemUpdateStatus 是管理员更新页面使用的完整状态。
type SystemUpdateStatus struct {
	Enabled         bool                  `json:"enabled"`
	ManifestURL     string                `json:"manifest_url"`
	Reachable       bool                  `json:"reachable"`
	Checking        bool                  `json:"checking"`
	CheckInterval   string                `json:"check_interval"`
	IntervalSeconds int64                 `json:"interval_seconds"`
	LastAttemptAt   *time.Time            `json:"last_attempt_at,omitempty"`
	LastSuccessAt   *time.Time            `json:"last_success_at,omitempty"`
	NextCheckAt     *time.Time            `json:"next_check_at,omitempty"`
	LastError       string                `json:"last_error,omitempty"`
	Manifest        *Manifest             `json:"manifest,omitempty"`
	Server          ComponentStatus       `json:"server"`
	Client          ClientComponentStatus `json:"client"`
}

// ManifestSource 抽象更新清单来源，后续可替换为 OSS、COS、MinIO 或内网服务。
type ManifestSource interface {
	Fetch(context.Context) (*Manifest, error)
	Location() string
}

// PackageStore 抽象升级包缓存，默认实现写入本机文件系统。
type PackageStore interface {
	Ensure(context.Context, string, PackageManifest) (path string, reused bool, err error)
	Cached(string, PackageManifest) bool
	Path(string) string
}

// UpdateService 定义更新检查、状态查询和周期调度契约。
type UpdateService interface {
	Check(context.Context) (SystemUpdateStatus, error)
	Status(string) SystemUpdateStatus
	ClientStatus(string) ClientUpdateStatus
	CachedClientPackagePath() string
	Start(context.Context)
}

// HTTPManifestSource 从普通 HTTPS JSON 地址读取清单，不依赖特定托管平台 API。
type HTTPManifestSource struct {
	URL    string
	Client *http.Client
}

// Fetch 读取并解析清单，标准 HTTP 客户端自动跟随 302 跳转。
func (s *HTTPManifestSource) Fetch(ctx context.Context) (*Manifest, error) {
	if strings.TrimSpace(s.URL) == "" {
		return nil, errors.New("update manifest url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch manifest status %d", res.StatusCode)
	}
	var manifest Manifest
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Server.Version == "" && manifest.Client.Version == "" {
		return nil, errors.New("manifest contains no package version")
	}
	return &manifest, nil
}

// Location 返回当前清单地址。
func (s *HTTPManifestSource) Location() string { return s.URL }

// LocalPackageStore 使用临时文件、校验和重命名缓存升级包。
type LocalPackageStore struct {
	Root   string
	Client *http.Client
}

// Path 返回缓存包路径。
func (s *LocalPackageStore) Path(name string) string {
	return filepath.Join(s.Root, "client", filepath.Base(name))
}

// Cached 判断现有文件是否与清单一致且是有效 ZIP。
func (s *LocalPackageStore) Cached(name string, pkg PackageManifest) bool {
	path := s.Path(name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	if pkg.Size > 0 && info.Size() != pkg.Size {
		return false
	}
	if pkg.SHA256 != "" && verifySHA256(path, pkg.SHA256) != nil {
		return false
	}
	return validateZip(path) == nil
}

// Ensure 复用已校验缓存；否则下载到同目录临时文件，验证后原子替换。
func (s *LocalPackageStore) Ensure(ctx context.Context, name string, pkg PackageManifest) (string, bool, error) {
	if s.Cached(name, pkg) {
		return s.Path(name), true, nil
	}
	if strings.TrimSpace(pkg.URL) == "" {
		return "", false, errors.New("client package url is empty")
	}
	path := s.Path(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, fmt.Errorf("create update cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return "", false, fmt.Errorf("create package temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", false, fmt.Errorf("close package temporary file: %w", err)
	}
	defer os.Remove(tmpPath)
	if err := downloadFile(ctx, s.Client, pkg.URL, tmpPath); err != nil {
		return "", false, err
	}
	if pkg.Size > 0 {
		info, err := os.Stat(tmpPath)
		if err != nil || info.Size() != pkg.Size {
			return "", false, errors.New("package size mismatch")
		}
	}
	if pkg.SHA256 != "" {
		if err := verifySHA256(tmpPath, pkg.SHA256); err != nil {
			return "", false, err
		}
	}
	if err := validateZip(tmpPath); err != nil {
		return "", false, err
	}
	if err := replaceCachedFile(tmpPath, path); err != nil {
		return "", false, fmt.Errorf("store client package: %w", err)
	}
	return path, false, nil
}

type checkCall struct {
	done   chan struct{}
	status SystemUpdateStatus
	err    error
}

// Service 是 UpdateService 的默认线程安全实现。
type Service struct {
	cfg           config.UpdateConfig
	serverVersion string
	source        ManifestSource
	store         PackageStore

	mu            sync.RWMutex
	manifest      *Manifest
	lastAttemptAt *time.Time
	lastSuccessAt *time.Time
	nextCheckAt   *time.Time
	lastError     string
	reachable     bool
	checking      bool
	inflight      *checkCall
	startOnce     sync.Once
}

// NewService 使用普通 HTTP 清单源与本地文件缓存创建更新服务。
func NewService(cfg config.UpdateConfig, serverVersion string) *Service {
	if cfg.ManifestTimeout <= 0 {
		cfg.ManifestTimeout = 20 * time.Second
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = 10 * time.Minute
	}
	manifestClient := &http.Client{Timeout: cfg.ManifestTimeout}
	downloadClient := &http.Client{Timeout: cfg.DownloadTimeout}
	return NewServiceWithDependencies(cfg, serverVersion,
		&HTTPManifestSource{URL: cfg.ManifestURL, Client: manifestClient},
		&LocalPackageStore{Root: cfg.CacheDir, Client: downloadClient},
	)
}

// NewServiceWithDependencies 允许测试和未来存储实现注入依赖。
func NewServiceWithDependencies(cfg config.UpdateConfig, serverVersion string, source ManifestSource, store PackageStore) *Service {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 6 * time.Hour
	}
	return &Service{cfg: cfg, serverVersion: serverVersion, source: source, store: store}
}

// Check 合并并发检查：后续调用等待同一次远端请求和缓存任务。
func (s *Service) Check(ctx context.Context) (SystemUpdateStatus, error) {
	s.mu.Lock()
	if s.inflight != nil {
		call := s.inflight
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return s.Status(""), ctx.Err()
		case <-call.done:
			return call.status, call.err
		}
	}
	call := &checkCall{done: make(chan struct{})}
	s.inflight = call
	now := time.Now()
	s.lastAttemptAt = &now
	s.checking = true
	s.mu.Unlock()

	err := s.performCheck(ctx)

	s.mu.Lock()
	s.checking = false
	if err != nil {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
		s.reachable = true
		success := time.Now()
		s.lastSuccessAt = &success
	}
	if s.cfg.Enabled {
		next := time.Now().Add(s.cfg.CheckInterval)
		s.nextCheckAt = &next
	} else {
		s.nextCheckAt = nil
	}
	call.status = s.statusLocked("")
	call.err = err
	s.inflight = nil
	close(call.done)
	s.mu.Unlock()
	return call.status, err
}

func (s *Service) performCheck(ctx context.Context) error {
	if !s.cfg.Enabled {
		return errors.New("update check is disabled")
	}
	manifest, err := s.source.Fetch(ctx)
	if err != nil {
		s.mu.Lock()
		s.reachable = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.reachable = true
	s.mu.Unlock()
	if shouldCacheClient(manifest.Client.Version, s.cfg.ClientVersion) && manifest.Client.URL != "" {
		if _, _, err := s.store.Ensure(ctx, clientPackageName, manifest.Client); err != nil {
			return fmt.Errorf("cache client package: %w", err)
		}
	}
	s.mu.Lock()
	s.manifest = cloneManifest(manifest)
	s.mu.Unlock()
	return nil
}

// Status 返回上一次成功结果；currentClientVersion 为空时使用服务端随包版本。
func (s *Service) Status(currentClientVersion string) SystemUpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked(currentClientVersion)
}

func (s *Service) statusLocked(currentClientVersion string) SystemUpdateStatus {
	if currentClientVersion == "" {
		currentClientVersion = s.cfg.ClientVersion
	}
	status := SystemUpdateStatus{
		Enabled:         s.cfg.Enabled,
		ManifestURL:     s.source.Location(),
		Reachable:       s.reachable,
		Checking:        s.checking,
		CheckInterval:   formatDuration(s.cfg.CheckInterval),
		IntervalSeconds: int64(s.cfg.CheckInterval / time.Second),
		LastAttemptAt:   cloneTime(s.lastAttemptAt),
		LastSuccessAt:   cloneTime(s.lastSuccessAt),
		NextCheckAt:     cloneTime(s.nextCheckAt),
		LastError:       s.lastError,
		Manifest:        cloneManifest(s.manifest),
		Server:          ComponentStatus{CurrentVersion: s.serverVersion},
		Client: ClientComponentStatus{ComponentStatus: ComponentStatus{
			CurrentVersion: currentClientVersion,
		}},
	}
	if s.manifest == nil {
		return status
	}
	serverPkg := s.manifest.Server
	clientPkg := s.manifest.Client
	status.Server.LatestVersion = serverPkg.Version
	status.Server.Available = CompareVersions(serverPkg.Version, s.serverVersion) > 0
	status.Server.DownloadURL = serverPkg.URL
	status.Server.Size = serverPkg.Size
	status.Server.SHA256 = serverPkg.SHA256
	status.Server.Manifest = clonePackage(&serverPkg)
	status.Client.LatestVersion = clientPkg.Version
	status.Client.Available = CompareVersions(clientPkg.Version, currentClientVersion) > 0
	status.Client.Size = clientPkg.Size
	status.Client.SHA256 = clientPkg.SHA256
	status.Client.Manifest = clonePackage(&clientPkg)
	if s.store.Cached(clientPackageName, clientPkg) {
		status.Client.Cached = true
		status.Client.FileName = clientPackageName
		status.Client.DownloadPath = "/api/v1/updates/client/download"
		status.Client.DownloadURL = status.Client.DownloadPath
	}
	return status
}

// ClientStatus 返回兼容客户端接口，并按 Tauri 传入的真实安装版本比较。
func (s *Service) ClientStatus(currentVersion string) ClientUpdateStatus {
	status := s.Status(currentVersion)
	client := status.Client
	return ClientUpdateStatus{
		CurrentVersion: client.CurrentVersion,
		LatestVersion:  client.LatestVersion,
		Available:      client.Available,
		Cached:         client.Cached,
		FileName:       client.FileName,
		DownloadPath:   client.DownloadPath,
		CheckedAt:      status.LastSuccessAt,
		Manifest:       client.Manifest,
		Message:        status.LastError,
	}
}

// CachedClientPackagePath 返回客户端缓存包路径。
func (s *Service) CachedClientPackagePath() string { return s.store.Path(clientPackageName) }

// Start 异步执行启动检查，并按配置周期检查；上下文取消即停止。
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		go func() {
			if !s.cfg.Enabled {
				return
			}
			for {
				_, _ = s.Check(ctx)
				timer := time.NewTimer(s.cfg.CheckInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	})
}

// Manager 保留命令行升级器既有的清单读取接口。
type Manager struct {
	Config config.UpdateConfig
	Source ManifestSource
}

// NewManager 创建兼容更新管理器。
func NewManager(cfg config.UpdateConfig) *Manager {
	if cfg.ManifestTimeout <= 0 {
		cfg.ManifestTimeout = 20 * time.Second
	}
	return &Manager{Config: cfg, Source: &HTTPManifestSource{
		URL: cfg.ManifestURL, Client: &http.Client{Timeout: cfg.ManifestTimeout},
	}}
}

// NewManagerForURL 创建只用于命令行检查的更新管理器。
func NewManagerForURL(manifestURL string) *Manager {
	return NewManager(config.UpdateConfig{Enabled: true, ManifestURL: manifestURL, ManifestTimeout: 20 * time.Second})
}

// FetchManifest 从配置的 URL 读取更新清单。
func (m *Manager) FetchManifest() (*Manifest, error) {
	if !m.Config.Enabled {
		return nil, errors.New("update check is disabled")
	}
	return m.Source.Fetch(context.Background())
}

func shouldCacheClient(latest, current string) bool {
	if normalizeVersion(latest) == "" {
		return false
	}
	if normalizeVersion(current) == "" {
		return true
	}
	return CompareVersions(latest, current) >= 0
}

func downloadFile(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	if _, err := io.Copy(file, res.Body); err != nil {
		_ = file.Close()
		return fmt.Errorf("write package file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync package file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close package file: %w", err)
	}
	return nil
}

func replaceCachedFile(tmpPath, path string) error {
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	// Windows 不能直接覆盖已存在目标，使用可回滚的同目录备份完成替换。
	backupPath := path + ".previous"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func verifySHA256(path, want string) error {
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

// CompareVersions 按标准 SemVer 比较版本，接受可选的 v 前缀和预发布版本。
func CompareVersions(left, right string) int {
	left = normalizeVersion(left)
	right = normalizeVersion(right)
	if left == "" && right == "" {
		return 0
	}
	if left == "" {
		return -1
	}
	if right == "" {
		return 1
	}
	return semver.Compare(left, right)
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if !semver.IsValid(value) {
		return ""
	}
	return value
}

func cloneManifest(manifest *Manifest) *Manifest {
	if manifest == nil {
		return nil
	}
	copy := *manifest
	return &copy
}

func clonePackage(pkg *PackageManifest) *PackageManifest {
	if pkg == nil {
		return nil
	}
	copy := *pkg
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func formatDuration(value time.Duration) string {
	if value > 0 && value%time.Hour == 0 {
		return fmt.Sprintf("%dh", value/time.Hour)
	}
	if value > 0 && value%time.Minute == 0 {
		return fmt.Sprintf("%dm", value/time.Minute)
	}
	return value.String()
}

// RuntimePackageSuffix 返回当前平台产物后缀，便于 manifest 生成时复用。
func RuntimePackageSuffix() string { return runtime.GOOS + "-" + runtime.GOARCH }
