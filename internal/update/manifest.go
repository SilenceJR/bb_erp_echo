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
	clientPackageName    = "bb-erp-client-windows.zip"
	serverPackageName    = "bb-erp-server-windows.zip"
	serverDownloadPath   = "/api/v1/system/updates/server/download"
	maxServerPackageSize = int64(512 << 20)
)

// ErrServerPackageUnavailable 表示尚无成功检查确认的服务端升级包。
var ErrServerPackageUnavailable = errors.New("暂无可下载的服务端升级包，请先执行更新检查")

// Manifest 是 Gitee、GitHub、对象存储或内网静态服务提供的更新清单。
type Manifest struct {
	Version        string                      `json:"version"`
	PublishedAt    time.Time                   `json:"published_at,omitempty"`
	Notes          string                      `json:"notes,omitempty"`
	Server         PackageManifest             `json:"server"`
	Client         PackageManifest             `json:"client"`
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
	ClientDeltaCached     bool                  `json:"client_delta_cached"`
	ClientDeltaFrom       string                `json:"client_delta_from_version,omitempty"`
	ClientCacheBytes      int64                 `json:"client_cache_bytes"`
	ClientDeltaDegraded   string                `json:"client_delta_degraded,omitempty"`
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

	mu         sync.Mutex
	verified   map[string]verifiedFileSnapshot
	verifyFile func(path, digest string) error
}

// Path 返回缓存包路径。普通名称进入 client 目录，server/ 前缀进入 server 目录。
func (s *LocalPackageStore) Path(name string) string {
	cacheDir, fileName := packageCacheLocation(name)
	return filepath.Join(s.Root, cacheDir, fileName)
}

// Cached 判断现有文件是否与清单一致且是有效 ZIP。
func (s *LocalPackageStore) Cached(name string, pkg PackageManifest) bool {
	path := s.Path(name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
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
	planner       UpdatePlanner
	verifierErr   error

	mu              sync.RWMutex
	serverPackageMu sync.Mutex
	manifest        *Manifest
	clientPayload   *ClientUpdatePayload
	artifacts       map[string]ClientArtifact
	deltaDegraded   string
	lastAttemptAt   *time.Time
	lastSuccessAt   *time.Time
	nextCheckAt     *time.Time
	lastError       string
	reachable       bool
	checking        bool
	inflight        *checkCall
	startOnce       sync.Once
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
	verifier, verifierErr := LoadSignedManifestVerifier(cfg.SigningPublicKey, cfg.SigningPublicKeyFile)
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
	return NewServiceWithAllDependencies(cfg, serverVersion, source, store,
		&LocalArtifactStore{Root: cfg.CacheDir, Client: &http.Client{Timeout: cfg.DownloadTimeout}}, nil, nil)
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
		artifactStore = &LocalArtifactStore{Root: cfg.CacheDir, Client: &http.Client{Timeout: cfg.DownloadTimeout}}
	}
	return &Service{
		cfg: cfg, serverVersion: serverVersion, source: source, store: store,
		artifactStore: artifactStore, verifier: verifier, planner: PreviousVersionUpdatePlanner{}, verifierErr: verifierErr,
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
	// v2 的验签和必需完整资源缓存必须先完成。若失败，不能覆盖旧客户端仍在使用的兼容 ZIP。
	payload, artifacts, deltaDegraded, err := s.cacheV2ClientArtifacts(ctx, manifest)
	if err != nil {
		return err
	}
	// 保持 v1 ZIP 缓存和下载接口，兼容已发布的 rc.3 及更旧客户端。
	// 只有 v2 已成功准备后才能替换该恢复资产；内存状态则在两者都成功后才提交。
	if manifest.Client.URL != "" {
		if _, _, err := s.store.Ensure(ctx, clientPackageName, manifest.Client); err != nil {
			return fmt.Errorf("cache client package: %w", err)
		}
	}
	s.mu.Lock()
	s.manifest = cloneManifest(manifest)
	s.clientPayload = cloneClientUpdatePayload(payload)
	s.artifacts = artifacts
	s.deltaDegraded = deltaDegraded
	s.mu.Unlock()
	return nil
}

// cacheV2ClientArtifacts 验证受签名 payload，并缓存完整包。差分缓存失败只降级为完整包。
func (s *Service) cacheV2ClientArtifacts(ctx context.Context, manifest *Manifest) (*ClientUpdatePayload, map[string]ClientArtifact, string, error) {
	if manifest == nil || manifest.ClientUpdateV2 == nil {
		return nil, nil, "", nil
	}
	if s.verifierErr != nil {
		return nil, nil, "", fmt.Errorf("load client update signing public key: %w", s.verifierErr)
	}
	payload, err := DecodeSignedClientPayload(manifest.ClientUpdateV2, s.verifier)
	if err != nil {
		return nil, nil, "", err
	}
	if manifest.Version != "" && CompareVersions(manifest.Version, payload.Version) != 0 {
		return nil, nil, "", errors.New("signed client update version does not match manifest version")
	}
	if manifest.Client.Version != "" && CompareVersions(manifest.Client.Version, payload.Version) != 0 {
		return nil, nil, "", errors.New("signed client update version does not match legacy client version")
	}
	artifacts := make(map[string]ClientArtifact, 2+len(payload.Deltas))
	for _, artifact := range []ClientArtifact{payload.Full.NSIS, payload.Full.Portable} {
		if _, _, err := s.artifactStore.Ensure(ctx, artifact); err != nil {
			return nil, nil, "", fmt.Errorf("cache required %s artifact: %w", artifact.Kind, err)
		}
		artifacts[strings.ToLower(artifact.SHA256)] = artifact
	}

	var degraded []string
	for _, delta := range payload.Deltas {
		if _, _, err := s.artifactStore.Ensure(ctx, delta.ClientArtifact); err != nil {
			degraded = append(degraded, fmt.Sprintf("%s: %v", delta.FromVersion, err))
			continue
		}
		artifacts[strings.ToLower(delta.SHA256)] = delta.ClientArtifact
	}
	return payload, artifacts, strings.Join(degraded, "; "), nil
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
		ClientDeltaDegraded: s.deltaDegraded,
	}
	if s.manifest == nil {
		return status
	}
	if s.clientPayload != nil {
		status.ClientProtocolVersion = s.clientPayload.ProtocolVersion
		status.ClientFullCached = s.artifactStore.Cached(s.clientPayload.Full.NSIS) &&
			s.artifactStore.Cached(s.clientPayload.Full.Portable)
		for _, delta := range s.clientPayload.Deltas {
			if status.ClientDeltaFrom == "" {
				status.ClientDeltaFrom = delta.FromVersion
			}
			if s.artifactStore.Cached(delta.ClientArtifact) {
				status.ClientDeltaCached = true
			}
		}
		for _, artifact := range s.artifacts {
			if s.artifactStore.Cached(artifact) {
				status.ClientCacheBytes += artifact.Size
			}
		}
	}
	serverPkg := s.manifest.Server
	clientPkg := s.manifest.Client
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
		status.ClientCacheBytes += clientPkg.Size
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

// ServerPackage 按当前成功清单下载或复用服务端包，返回前验证大小、SHA-256 和 ZIP 结构。
func (s *Service) ServerPackage(ctx context.Context) (path string, fileName string, err error) {
	s.serverPackageMu.Lock()
	defer s.serverPackageMu.Unlock()

	s.mu.RLock()
	manifest := cloneManifest(s.manifest)
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
	cacheName := serverPackageCacheName(fileName)
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

// ClientUpdatePlan 为 Windows x86_64 桌面端选择已缓存的差分或完整更新。
func (s *Service) ClientUpdatePlan(request ClientUpdatePlanRequest) (ClientUpdatePlan, bool, error) {
	request.Target = strings.TrimSpace(strings.ToLower(request.Target))
	if request.Target == "" {
		request.Target = clientTargetWindowsX64
	}
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
	deltaDegraded := s.deltaDegraded
	artifacts := cloneArtifacts(s.artifacts)
	s.mu.RUnlock()
	if payload == nil || manifest == nil || CompareVersions(payload.Version, request.CurrentVersion) <= 0 {
		return ClientUpdatePlan{}, false, nil
	}
	request.InstallMode = installMode
	full, selectedDelta := s.planner.Select(payload, request, func(artifact ClientArtifact) bool {
		_, declared := artifacts[strings.ToLower(artifact.SHA256)]
		return declared && s.artifactStore.Cached(artifact)
	})
	if _, ok := artifacts[strings.ToLower(full.SHA256)]; !ok || !s.artifactStore.Cached(full) {
		return ClientUpdatePlan{}, false, errors.New("required full client update artifact is not cached")
	}
	fullPlan := clientPlanArtifact(full, nil)
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
		FullFallback:    fullPlan,
		Message:         deltaDegraded,
	}
	if selectedDelta != nil {
		plan.Strategy = "delta"
		plan.DownloadSize = selectedDelta.Size
		plan.SavedBytes = max(full.Size-selectedDelta.Size, 0)
		plan.Artifact = clientPlanArtifact(selectedDelta.ClientArtifact, selectedDelta)
	}
	return plan, true, nil
}

// TauriClientUpdate 返回官方 updater 使用的完整 NSIS 更新，不参与差分选择。
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

func serverPackageCacheName(fileName string) string {
	return "server/" + fileName
}

func packageCacheLocation(name string) (cacheDir, fileName string) {
	normalized := filepath.ToSlash(strings.TrimSpace(name))
	cacheDir = "client"
	fallback := clientPackageName
	if normalized == "server" || strings.HasPrefix(normalized, "server/") {
		cacheDir = "server"
		fallback = serverPackageName
		normalized = strings.TrimPrefix(normalized, "server/")
	}
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
	case "", installModeNSIS:
		return installModeNSIS, nil
	case installModePortable, "all-in-one", "all_in_one":
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
	copy.Deltas = append([]ClientDeltaArtifact(nil), payload.Deltas...)
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
