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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bb_erp_echo/internal/config"
	"golang.org/x/mod/semver"
)

const (
	serverPackageName    = "bb-erp-server-windows.zip"
	serverDownloadPath   = "/api/v1/system/updates/server/download"
	maxManifestSize      = 2 << 20
	maxServerPackageSize = int64(512 << 20)
)

// ErrServerPackageUnavailable 表示尚无成功检查确认的服务端升级包。
var ErrServerPackageUnavailable = errors.New("暂无可下载的服务端升级包，请先执行更新检查")

// Manifest 是发布端或内网静态服务提供的当前更新清单。
type Manifest struct {
	Version        string                      `json:"version"`
	PublishedAt    time.Time                   `json:"published_at,omitempty"`
	Notes          string                      `json:"notes,omitempty"`
	Server         PackageManifest             `json:"server"`
	AllInOne       PackageManifest             `json:"all_in_one,omitempty"`
	Updater        PackageManifest             `json:"updater,omitempty"`
	ClientUpdateV2 *SignedClientUpdateManifest `json:"client_update_v2,omitempty"`
}

// PackageManifest 描述一个可下载升级包。
type PackageManifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// ComponentStatus 描述服务端或桌面客户端的版本状态。
type ComponentStatus struct {
	CurrentVersion string           `json:"current_version"`
	LatestVersion  string           `json:"latest_version,omitempty"`
	Available      bool             `json:"available"`
	DownloadURL    string           `json:"download_url,omitempty"`
	DownloadPath   string           `json:"download_path,omitempty"`
	FileName       string           `json:"file_name,omitempty"`
	Size           int64            `json:"size,omitempty"`
	SHA256         string           `json:"sha256,omitempty"`
	Manifest       *PackageManifest `json:"manifest,omitempty"`
}

// ClientComponentStatus 在组件状态上增加服务端缓存信息。
type ClientComponentStatus struct {
	ComponentStatus
	Cached bool `json:"cached"`
}

// SystemUpdateStatus 是管理员更新页面使用的完整状态。
type SystemUpdateStatus struct {
	Enabled               bool                  `json:"enabled"`
	ManifestURL           string                `json:"manifest_url"`
	Reachable             bool                  `json:"reachable"`
	Checking              bool                  `json:"checking"`
	CheckInterval         string                `json:"check_interval"`
	IntervalSeconds       int64                 `json:"interval_seconds"`
	LastAttemptAt         *time.Time            `json:"last_attempt_at,omitempty"`
	LastSuccessAt         *time.Time            `json:"last_success_at,omitempty"`
	NextCheckAt           *time.Time            `json:"next_check_at,omitempty"`
	LastError             string                `json:"last_error,omitempty"`
	Manifest              *Manifest             `json:"manifest,omitempty"`
	Server                ComponentStatus       `json:"server"`
	Client                ClientComponentStatus `json:"client"`
	ClientProtocolVersion int                   `json:"client_protocol_version,omitempty"`
	ClientFullCached      bool                  `json:"client_full_cached"`
	ClientCacheBytes      int64                 `json:"client_cache_bytes"`
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
	ServerPackage(context.Context) (path string, fileName string, err error)
	ClientUpdatePlan(ClientUpdatePlanRequest) (ClientUpdatePlan, bool, error)
	TauriClientUpdate(target, currentVersion string) (TauriUpdateResponse, bool, error)
	ClientArtifact(string) (path string, artifact ClientArtifact, ok bool)
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
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(raw) > maxManifestSize {
		return nil, fmt.Errorf("read manifest: response exceeds %d bytes", maxManifestSize)
	}
	return parseManifest(raw)
}

// Location 返回当前清单地址。
func (s *HTTPManifestSource) Location() string { return s.URL }

// LocalPackageStore 使用临时文件、校验和重命名缓存服务端升级包。
type LocalPackageStore struct {
	Root       string
	Client     *http.Client
	ReleaseDir string

	mu         sync.Mutex
	verified   map[string]verifiedFileSnapshot
	verifyFile func(path, digest string) error
}

// Path 返回服务端升级包缓存路径。
func (s *LocalPackageStore) Path(name string) string {
	cacheDir, fileName := packageCacheLocation(name)
	return filepath.Join(s.Root, cacheDir, fileName)
}

// Cached 判断现有文件是否与清单一致且是有效 ZIP。
func (s *LocalPackageStore) Cached(name string, pkg PackageManifest) bool {
	path := s.Path(name)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() == 0 {
		s.invalidate(name)
		return false
	}
	if pkg.Size <= 0 || !isSHA256(pkg.SHA256) || info.Size() != pkg.Size {
		s.invalidate(name)
		return false
	}
	key := path
	digest := strings.ToLower(pkg.SHA256)
	s.mu.Lock()
	snapshot, verified := s.verified[key]
	if verified {
		if snapshot.matches(path, digest, info) {
			s.mu.Unlock()
			return validateZip(path) == nil
		}
		delete(s.verified, key)
		s.mu.Unlock()
		return false
	}
	verify := s.verifyFile
	if verify == nil {
		verify = verifySHA256
	}
	err = verify(path, pkg.SHA256)
	if err == nil && validateZip(path) == nil {
		s.rememberVerifiedLocked(key, digest, path, info)
		s.mu.Unlock()
		return true
	}
	s.mu.Unlock()
	return false
}

// Ensure 复用已校验缓存；否则下载到同目录临时文件，验证后原子替换。
func (s *LocalPackageStore) Ensure(ctx context.Context, name string, pkg PackageManifest) (string, bool, error) {
	if s.Cached(name, pkg) {
		return s.Path(name), true, nil
	}
	if strings.TrimSpace(s.ReleaseDir) != "" {
		return s.ensureFromDirectory(ctx, name, pkg)
	}
	if strings.TrimSpace(pkg.URL) == "" || pkg.Size <= 0 || !isSHA256(pkg.SHA256) {
		return "", false, errors.New("update package must declare url, positive size and sha256")
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
	if err := downloadVerifiedFile(ctx, s.Client, pkg.URL, tmpPath, pkg.Size, pkg.SHA256); err != nil {
		return "", false, err
	}
	if err := validateZip(tmpPath); err != nil {
		return "", false, err
	}
	if err := replaceCachedFile(tmpPath, path); err != nil {
		return "", false, fmt.Errorf("store update package: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("stat stored client package: %w", err)
	}
	s.mu.Lock()
	s.rememberVerifiedLocked(path, strings.ToLower(pkg.SHA256), path, info)
	s.mu.Unlock()
	return path, false, nil
}

// ensureFromDirectory 从已激活发布目录复制服务端升级包，完全绕过 HTTP 客户端。
func (s *LocalPackageStore) ensureFromDirectory(ctx context.Context, name string, pkg PackageManifest) (string, bool, error) {
	if strings.TrimSpace(pkg.URL) == "" || pkg.Size <= 0 || !isSHA256(pkg.SHA256) {
		return "", false, errors.New("local update package must declare relative url, positive size and sha256")
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
	if err := copyVerifiedReleaseFile(ctx, s.ReleaseDir, pkg.URL, tmpPath, pkg.Size, pkg.SHA256); err != nil {
		return "", false, err
	}
	if err := validateZip(tmpPath); err != nil {
		return "", false, err
	}
	if err := replaceCachedFile(tmpPath, path); err != nil {
		return "", false, fmt.Errorf("store local update package: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("stat stored local update package: %w", err)
	}
	s.mu.Lock()
	s.rememberVerifiedLocked(path, strings.ToLower(pkg.SHA256), path, info)
	s.mu.Unlock()
	return path, false, nil
}

func (s *LocalPackageStore) invalidate(name string) {
	s.mu.Lock()
	delete(s.verified, s.Path(name))
	s.mu.Unlock()
}

func (s *LocalPackageStore) rememberVerifiedLocked(key, digest, path string, info os.FileInfo) {
	if s.verified == nil {
		s.verified = make(map[string]verifiedFileSnapshot)
	}
	s.verified[key] = verifiedFileSnapshot{path: path, digest: digest, size: info.Size(), info: info}
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
	artifactStore ArtifactStore
	verifier      SignedManifestVerifier
	verifierErr   error

	mu              sync.RWMutex
	serverPackageMu sync.Mutex
	manifest        *Manifest
	clientPayload   *ClientUpdatePayload
	artifacts       map[string]ClientArtifact
	lastAttemptAt   *time.Time
	lastSuccessAt   *time.Time
	nextCheckAt     *time.Time
	lastError       string
	reachable       bool
	checking        bool
	inflight        *checkCall
	startOnce       sync.Once
}

// NewService 根据配置选择 HTTP 或本地 directory 清单源，并创建对应缓存实现。
func NewService(cfg config.UpdateConfig, serverVersion string) *Service {
	if cfg.ManifestTimeout <= 0 {
		cfg.ManifestTimeout = 20 * time.Second
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = 10 * time.Minute
	}
	manifestClient := &http.Client{Timeout: cfg.ManifestTimeout}
	downloadClient := &http.Client{Timeout: cfg.DownloadTimeout}
	verifier, verifierErr := LoadSignedManifestVerifier(cfg.SigningPublicKey, cfg.SigningPublicKeyFile)
	if strings.EqualFold(strings.TrimSpace(cfg.Source), config.UpdateSourceDirectory) {
		return NewServiceWithAllDependencies(cfg, serverVersion,
			NewDirectoryManifestSource(cfg.ReleaseDir),
			&DirectoryPackageStore{Root: cfg.CacheDir, ReleaseDir: cfg.ReleaseDir},
			&DirectoryArtifactStore{Root: cfg.CacheDir, ReleaseDir: cfg.ReleaseDir},
			verifier,
			verifierErr,
		)
	}
	return NewServiceWithAllDependencies(cfg, serverVersion,
		&HTTPManifestSource{URL: cfg.ManifestURL, Client: manifestClient},
		&LocalPackageStore{Root: cfg.CacheDir, Client: downloadClient},
		&LocalArtifactStore{Root: cfg.CacheDir, Client: downloadClient},
		verifier,
		verifierErr,
	)
}

// NewServiceWithDependencies 允许测试和未来存储实现注入依赖。
func NewServiceWithDependencies(cfg config.UpdateConfig, serverVersion string, source ManifestSource, store PackageStore) *Service {
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = 10 * time.Minute
	}
	var artifactStore ArtifactStore
	if strings.EqualFold(strings.TrimSpace(cfg.Source), config.UpdateSourceDirectory) {
		artifactStore = &DirectoryArtifactStore{Root: cfg.CacheDir, ReleaseDir: cfg.ReleaseDir}
	} else {
		artifactStore = &LocalArtifactStore{Root: cfg.CacheDir, Client: &http.Client{Timeout: cfg.DownloadTimeout}}
	}
	return NewServiceWithAllDependencies(cfg, serverVersion, source, store, artifactStore, nil, nil)
}

// NewServiceWithAllDependencies 允许测试或宿主注入 v2 签名验证和内容寻址缓存实现。
func NewServiceWithAllDependencies(cfg config.UpdateConfig, serverVersion string, source ManifestSource, store PackageStore, artifactStore ArtifactStore, verifier SignedManifestVerifier, verifierErr error) *Service {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 6 * time.Hour
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = 10 * time.Minute
	}
	if artifactStore == nil {
		if strings.EqualFold(strings.TrimSpace(cfg.Source), config.UpdateSourceDirectory) {
			artifactStore = &DirectoryArtifactStore{Root: cfg.CacheDir, ReleaseDir: cfg.ReleaseDir}
		} else {
			artifactStore = &LocalArtifactStore{Root: cfg.CacheDir, Client: &http.Client{Timeout: cfg.DownloadTimeout}}
		}
	}
	return &Service{
		cfg: cfg, serverVersion: serverVersion, source: source, store: store,
		artifactStore: artifactStore, verifier: verifier, verifierErr: verifierErr,
	}
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
	// 签名与两个完整资源必须全部验证并缓存成功，才提交新的内存状态。
	payload, artifacts, err := s.cacheFullClientArtifacts(ctx, manifest)
	if err != nil {
		return err
	}
	// 本地目录由人工搬运，只有服务端整包也完成大小、摘要、签名和 ZIP
	// 结构校验后，才允许替换上一份成功清单。
	if _, localDirectory := s.source.(*DirectoryManifestSource); localDirectory {
		s.serverPackageMu.Lock()
		_, _, serverErr := s.ensureServerPackage(ctx, manifest)
		s.serverPackageMu.Unlock()
		if serverErr != nil {
			return serverErr
		}
	}
	s.mu.Lock()
	s.manifest = cloneManifest(manifest)
	s.clientPayload = cloneClientUpdatePayload(payload)
	s.artifacts = artifacts
	s.mu.Unlock()
	return nil
}

// cacheFullClientArtifacts 验证受签名 payload，并缓存 NSIS 与便携完整包。
func (s *Service) cacheFullClientArtifacts(ctx context.Context, manifest *Manifest) (*ClientUpdatePayload, map[string]ClientArtifact, error) {
	if manifest == nil || manifest.ClientUpdateV2 == nil {
		return nil, nil, nil
	}
	if s.verifierErr != nil {
		return nil, nil, fmt.Errorf("load client update signing public key: %w", s.verifierErr)
	}
	payload, err := DecodeSignedClientPayload(manifest.ClientUpdateV2, s.verifier)
	if err != nil {
		return nil, nil, err
	}
	if manifest.Version != "" && CompareVersions(manifest.Version, payload.Version) != 0 {
		return nil, nil, errors.New("signed client update version does not match manifest version")
	}
	artifacts := make(map[string]ClientArtifact, 2)
	for _, artifact := range []ClientArtifact{payload.Full.NSIS, payload.Full.Portable} {
		if _, _, err := s.artifactStore.Ensure(ctx, artifact); err != nil {
			return nil, nil, fmt.Errorf("cache required %s artifact: %w", artifact.Kind, err)
		}
		artifacts[strings.ToLower(artifact.SHA256)] = artifact
	}
	return payload, artifacts, nil
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
	if s.clientPayload != nil {
		status.ClientProtocolVersion = s.clientPayload.ProtocolVersion
		status.ClientFullCached = s.artifactStore.Cached(s.clientPayload.Full.NSIS) &&
			s.artifactStore.Cached(s.clientPayload.Full.Portable)
		for _, artifact := range s.artifacts {
			if s.artifactStore.Cached(artifact) {
				status.ClientCacheBytes += artifact.Size
			}
		}
	}
	serverPkg := s.manifest.Server
	status.Server.LatestVersion = serverPkg.Version
	status.Server.Available = CompareVersions(serverPkg.Version, s.serverVersion) > 0
	status.Server.Size = serverPkg.Size
	status.Server.SHA256 = serverPkg.SHA256
	status.Server.Manifest = clonePackage(&serverPkg)
	if serverFileName := packageFileName(serverPkg.URL); strings.TrimSpace(serverPkg.URL) != "" {
		status.Server.FileName = serverFileName
		status.Server.DownloadPath = serverDownloadPath
		status.Server.DownloadURL = serverDownloadPath
	}
	if s.clientPayload != nil {
		status.Client.LatestVersion = s.clientPayload.Version
		status.Client.Available = CompareVersions(s.clientPayload.Version, currentClientVersion) > 0
		status.Client.Cached = status.ClientFullCached
		status.Client.Size = s.clientPayload.Full.NSIS.Size
		status.Client.SHA256 = s.clientPayload.Full.NSIS.SHA256
	}
	return status
}

// ServerPackage 按当前成功清单下载或复用服务端包，返回前验证大小、SHA-256 和 ZIP 结构。
func (s *Service) ServerPackage(ctx context.Context) (path string, fileName string, err error) {
	s.serverPackageMu.Lock()
	defer s.serverPackageMu.Unlock()

	s.mu.RLock()
	manifest := cloneManifest(s.manifest)
	s.mu.RUnlock()
	return s.ensureServerPackage(ctx, manifest)
}

func (s *Service) ensureServerPackage(ctx context.Context, manifest *Manifest) (path string, fileName string, err error) {
	s.mu.RLock()
	verifier := s.verifier
	verifierErr := s.verifierErr
	s.mu.RUnlock()
	if manifest == nil || strings.TrimSpace(manifest.Server.URL) == "" {
		return "", "", ErrServerPackageUnavailable
	}
	if verifierErr != nil {
		return "", "", fmt.Errorf("加载服务端升级验签公钥失败：%w", verifierErr)
	}
	if verifier == nil || strings.TrimSpace(manifest.Server.Signature) == "" {
		return "", "", errors.New("服务端升级包缺少可信 Minisign 签名或验签公钥")
	}
	if manifest.Server.Size <= 0 || manifest.Server.Size > maxServerPackageSize {
		return "", "", fmt.Errorf("服务端升级包大小 %d 超出允许范围 1..%d", manifest.Server.Size, maxServerPackageSize)
	}
	fileName = packageFileName(manifest.Server.URL)
	cacheName := serverPackageCacheName(fileName, manifest.Server.SHA256)
	path, _, err = s.store.Ensure(ctx, cacheName, manifest.Server)
	if err != nil {
		return "", "", fmt.Errorf("下载或校验服务端升级包失败：%w", err)
	}
	if verifyErr := verifier.VerifyFile(path, manifest.Server.Signature); verifyErr != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("服务端升级包 Minisign 签名校验失败：%w", verifyErr)
	}
	if validateErr := validateServerPackageArchive(path, manifest.Server.Version); validateErr != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("服务端升级包结构校验失败：%w", validateErr)
	}
	return path, fileName, nil
}

// ClientUpdatePlan 返回 Windows x86_64 桌面端已缓存的完整更新。
func (s *Service) ClientUpdatePlan(request ClientUpdatePlanRequest) (ClientUpdatePlan, bool, error) {
	if normalizeVersion(request.CurrentVersion) == "" {
		return ClientUpdatePlan{}, false, errors.New("current_version must be a valid SemVer")
	}
	request.Target = strings.TrimSpace(strings.ToLower(request.Target))
	if request.Target != clientTargetWindowsX64 {
		return ClientUpdatePlan{}, false, fmt.Errorf("unsupported update target %q", request.Target)
	}
	installMode, err := normalizeInstallMode(request.InstallMode)
	if err != nil {
		return ClientUpdatePlan{}, false, err
	}

	s.mu.RLock()
	payload := cloneClientUpdatePayload(s.clientPayload)
	manifest := cloneManifest(s.manifest)
	artifacts := cloneArtifacts(s.artifacts)
	s.mu.RUnlock()
	if payload == nil || manifest == nil || CompareVersions(payload.Version, request.CurrentVersion) <= 0 {
		return ClientUpdatePlan{}, false, nil
	}
	request.InstallMode = installMode
	full := payload.Full.NSIS
	if installMode == installModePortable {
		full = payload.Full.Portable
	}
	if _, ok := artifacts[strings.ToLower(full.SHA256)]; !ok || !s.artifactStore.Cached(full) {
		return ClientUpdatePlan{}, false, errors.New("required full client update artifact is not cached")
	}
	fullPlan := clientPlanArtifact(full)
	plan := ClientUpdatePlan{
		ProtocolVersion: payload.ProtocolVersion,
		CurrentVersion:  request.CurrentVersion,
		LatestVersion:   payload.Version,
		Target:          payload.Target,
		InstallMode:     installMode,
		Strategy:        "full",
		DownloadSize:    full.Size,
		FullSize:        full.Size,
		SignedPayload:   manifest.ClientUpdateV2.Payload,
		Signature:       manifest.ClientUpdateV2.Signature,
		Artifact:        fullPlan,
	}
	return plan, true, nil
}

// TauriClientUpdate 返回 Tauri updater 使用的完整 NSIS 更新。
func (s *Service) TauriClientUpdate(target, currentVersion string) (TauriUpdateResponse, bool, error) {
	target = strings.TrimSpace(strings.ToLower(target))
	if target != clientTargetWindowsX64 {
		return TauriUpdateResponse{}, false, fmt.Errorf("unsupported update target %q", target)
	}
	plan, available, err := s.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: currentVersion, Target: target, InstallMode: installModeNSIS,
	})
	if err != nil || !available {
		return TauriUpdateResponse{}, available, err
	}
	s.mu.RLock()
	manifest := cloneManifest(s.manifest)
	s.mu.RUnlock()
	response := TauriUpdateResponse{
		Version:   plan.LatestVersion,
		URL:       plan.Artifact.DownloadPath,
		Signature: plan.Artifact.Signature,
	}
	if manifest != nil && !manifest.PublishedAt.IsZero() {
		response.PubDate = manifest.PublishedAt.UTC().Format(time.RFC3339)
	}
	return response, true, nil
}

// ClientArtifact 按内容哈希查找当前已验证 manifest 中的缓存资源。
func (s *Service) ClientArtifact(digest string) (string, ClientArtifact, bool) {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if !isSHA256(digest) {
		return "", ClientArtifact{}, false
	}
	s.mu.RLock()
	artifact, ok := s.artifacts[digest]
	s.mu.RUnlock()
	if !ok || !s.artifactStore.Cached(artifact) {
		return "", ClientArtifact{}, false
	}
	path, ok := s.artifactStore.Path(digest)
	return path, artifact, ok
}

func serverPackageCacheName(fileName, sha256 string) string {
	digest := strings.ToLower(strings.TrimSpace(sha256))
	if len(digest) > 16 {
		digest = digest[:16]
	}
	return "server/" + digest + "-" + fileName
}

func packageCacheLocation(name string) (cacheDir, fileName string) {
	normalized := filepath.ToSlash(strings.TrimSpace(name))
	cacheDir = "server"
	fallback := serverPackageName
	normalized = strings.TrimPrefix(normalized, "server/")
	fileName = path.Base(normalized)
	if fileName == "." || fileName == ".." || fileName == "" {
		fileName = fallback
	}
	return cacheDir, fileName
}

func packageFileName(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return serverPackageName
	}
	fileName := path.Base(parsed.Path)
	fileName, err = url.PathUnescape(fileName)
	if err != nil || fileName == "." || fileName == ".." || fileName == "" ||
		strings.ContainsAny(fileName, `/\\`) || strings.IndexFunc(fileName, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) >= 0 {
		return serverPackageName
	}
	return fileName
}

func normalizeInstallMode(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case installModeNSIS:
		return installModeNSIS, nil
	case installModePortable:
		return installModePortable, nil
	default:
		return "", fmt.Errorf("unsupported install mode %q", value)
	}
}

func appendPlanMessage(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

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

// NewManager 创建更新管理器。
func NewManager(cfg config.UpdateConfig) *Manager {
	if cfg.ManifestTimeout <= 0 {
		cfg.ManifestTimeout = 20 * time.Second
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Source), config.UpdateSourceDirectory) {
		return &Manager{Config: cfg, Source: NewDirectoryManifestSource(cfg.ReleaseDir)}
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

// downloadVerifiedFile 用 manifest 声明的精确大小限制响应体，并在写入时完成 SHA-256 校验。
// 它是更新资源进入磁盘前唯一的网络下载入口，避免恶意服务端无限流式写满磁盘。
func downloadVerifiedFile(ctx context.Context, client *http.Client, url, path string, size int64, wantSHA256 string) error {
	if size <= 0 || !isSHA256(wantSHA256) {
		return errors.New("download requires a positive manifest size and sha256")
	}
	if client == nil {
		client = http.DefaultClient
	}
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
	hash := sha256.New()
	// 读取 size+1 字节可以区分精确匹配和超长响应，不依赖不可信 Content-Length。
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, size+1))
	if closeErr := file.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("write package file: %w", copyErr)
	}
	if written > size {
		return errors.New("download exceeds manifest size limit")
	}
	if written != size {
		return errors.New("download size mismatch")
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, strings.TrimSpace(wantSHA256)) {
		return fmt.Errorf("package sha256 mismatch: got %s", got)
	}
	file, err = os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("reopen package file for sync: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync package file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close package file after sync: %w", err)
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
		return fmt.Errorf("validate update zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("update zip is empty")
	}
	return nil
}

func validateServerPackageArchive(archivePath, expectedVersion string) error {
	const (
		maxEntries      = 10_000
		maxExpandedSize = uint64(1 << 30)
	)
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if len(reader.File) > maxEntries {
		return fmt.Errorf("too many ZIP entries: got %d max %d", len(reader.File), maxEntries)
	}
	required := map[string]bool{
		"bb-erp-server.exe":         false,
		"bb-erp-updater.exe":        false,
		"bb-erp-upgrade-runner.bat": false,
		"bb-erp-verify-update.exe":  false,
		"激活离线更新.ps1":                false,
		"update-public.key":         false,
		"version.json":              false,
	}
	packageVersion := ""
	var expanded uint64
	for _, entry := range reader.File {
		name := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if name == "." || name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return fmt.Errorf("unsafe ZIP path %q", entry.Name)
		}
		if entry.UncompressedSize64 > maxExpandedSize-expanded {
			return fmt.Errorf("expanded ZIP size exceeds %d bytes", maxExpandedSize)
		}
		expanded += entry.UncompressedSize64
		if _, ok := required[name]; ok && !entry.FileInfo().IsDir() && entry.UncompressedSize64 > 0 {
			required[name] = true
		}
		if name == "version.json" && !entry.FileInfo().IsDir() {
			if entry.UncompressedSize64 > 1<<20 {
				return errors.New("version.json exceeds 1 MiB")
			}
			file, openErr := entry.Open()
			if openErr != nil {
				return fmt.Errorf("open version.json: %w", openErr)
			}
			var metadata struct {
				Version       string `json:"version"`
				ServerVersion string `json:"server_version"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&metadata)
			_ = file.Close()
			if decodeErr != nil {
				return fmt.Errorf("decode version.json: %w", decodeErr)
			}
			packageVersion = strings.TrimSpace(metadata.ServerVersion)
			if packageVersion == "" {
				packageVersion = strings.TrimSpace(metadata.Version)
			}
		}
	}
	for name, present := range required {
		if !present {
			return fmt.Errorf("required package entry %s is missing or empty", name)
		}
	}
	if packageVersion == "" || CompareVersions(packageVersion, expectedVersion) != 0 {
		return fmt.Errorf("package version %q does not match manifest version %q", packageVersion, expectedVersion)
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
	if manifest.ClientUpdateV2 != nil {
		envelope := *manifest.ClientUpdateV2
		copy.ClientUpdateV2 = &envelope
	}
	return &copy
}

func cloneClientUpdatePayload(payload *ClientUpdatePayload) *ClientUpdatePayload {
	if payload == nil {
		return nil
	}
	copy := *payload
	return &copy
}

func cloneArtifacts(source map[string]ClientArtifact) map[string]ClientArtifact {
	if len(source) == 0 {
		return nil
	}
	copy := make(map[string]ClientArtifact, len(source))
	for digest, artifact := range source {
		copy[digest] = artifact
	}
	return copy
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
