package update

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"aead.dev/minisign"
)

const (
	clientTargetWindowsX64 = "windows-x86_64"
	installModeNSIS        = "nsis"
	installModePortable    = "portable"
)

// SignedClientUpdateManifest 是 v1 manifest 中可选的 v2 客户端更新签名信封。
// Payload 必须是原始 JSON 字节的 base64 编码，Signature 使用 Minisign detached signature。
type SignedClientUpdateManifest struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// ClientUpdatePayload 是经 Minisign 签名的客户端更新描述。
type ClientUpdatePayload struct {
	ProtocolVersion int                   `json:"protocol_version"`
	Version         string                `json:"version"`
	Target          string                `json:"target"`
	LayoutVersion   int                   `json:"layout_version"`
	Full            ClientFullArtifacts   `json:"full"`
	Deltas          []ClientDeltaArtifact `json:"deltas,omitempty"`
}

// ClientFullArtifacts 描述两种完整客户端载荷。
type ClientFullArtifacts struct {
	NSIS     ClientArtifact `json:"nsis"`
	Portable ClientArtifact `json:"portable"`
}

// ClientArtifact 描述受签名 payload 保护的二进制资源。
// URL 只在服务端下载远端资源时使用，响应永远只暴露 DownloadPath。
type ClientArtifact struct {
	Kind      string `json:"kind"`
	Algorithm string `json:"algorithm,omitempty"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

// ClientDeltaArtifact 是从一个精确版本和 EXE 哈希生成的差分资源。
type ClientDeltaArtifact struct {
	ClientArtifact
	FromVersion  string `json:"from_version"`
	FromSHA256   string `json:"from_sha256"`
	TargetSHA256 string `json:"target_sha256"`
}

// ClientUpdatePlanRequest 是桌面端请求更新策略时传入的当前安装信息。
type ClientUpdatePlanRequest struct {
	CurrentVersion string
	CurrentSHA256  string
	Target         string
	InstallMode    string
}

// ClientUpdatePlanArtifact 是 API 返回的安全资源引用，不包含远端 URL 或本机路径。
type ClientUpdatePlanArtifact struct {
	Kind         string `json:"kind"`
	Algorithm    string `json:"algorithm,omitempty"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	Signature    string `json:"signature"`
	DownloadPath string `json:"download_path"`
	FromVersion  string `json:"from_version,omitempty"`
	FromSHA256   string `json:"from_sha256,omitempty"`
	TargetSHA256 string `json:"target_sha256,omitempty"`
}

// ClientUpdatePlan 是桌面端增量/完整更新统一策略响应。
type ClientUpdatePlan struct {
	ProtocolVersion int                      `json:"protocol_version"`
	CurrentVersion  string                   `json:"current_version"`
	LatestVersion   string                   `json:"latest_version"`
	Target          string                   `json:"target"`
	InstallMode     string                   `json:"install_mode"`
	Strategy        string                   `json:"strategy"`
	DownloadSize    int64                    `json:"download_size"`
	FullSize        int64                    `json:"full_size"`
	SavedBytes      int64                    `json:"saved_bytes"`
	SignedPayload   string                   `json:"signed_payload"`
	Signature       string                   `json:"signature"`
	Artifact        ClientUpdatePlanArtifact `json:"artifact"`
	FullFallback    ClientUpdatePlanArtifact `json:"full_fallback"`
	Message         string                   `json:"message,omitempty"`
}

// TauriUpdateResponse 符合 tauri-plugin-updater 的更新响应格式。
type TauriUpdateResponse struct {
	Version   string `json:"version"`
	PubDate   string `json:"pub_date,omitempty"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

// SignedManifestVerifier 验证 v2 签名 payload。接口允许测试和未来密钥托管实现替换。
type SignedManifestVerifier interface {
	Verify(payload []byte, signature string) error
}

// UpdatePlanner isolates update-strategy selection from transport, signature
// verification and storage. A future rollout can replace this policy (for
// example with multi-hop deltas) without changing handlers or cache code.
type UpdatePlanner interface {
	Select(payload *ClientUpdatePayload, request ClientUpdatePlanRequest, cached func(ClientArtifact) bool) (full ClientArtifact, delta *ClientDeltaArtifact)
}

// PreviousVersionUpdatePlanner selects a delta only for an exact version and
// executable hash match; every other case intentionally returns a full update.
type PreviousVersionUpdatePlanner struct{}

func (PreviousVersionUpdatePlanner) Select(payload *ClientUpdatePayload, request ClientUpdatePlanRequest, cached func(ClientArtifact) bool) (ClientArtifact, *ClientDeltaArtifact) {
	full := payload.Full.NSIS
	if request.InstallMode == installModePortable {
		full = payload.Full.Portable
	}
	if !isSHA256(request.CurrentSHA256) {
		return full, nil
	}
	for index := range payload.Deltas {
		delta := &payload.Deltas[index]
		if CompareVersions(delta.FromVersion, request.CurrentVersion) == 0 &&
			strings.EqualFold(delta.FromSHA256, request.CurrentSHA256) && cached(delta.ClientArtifact) {
			return full, delta
		}
	}
	return full, nil
}

// MinisignVerifier 使用 Minisign Ed25519 公钥验证 detached signature 的原始 payload 签名。
type MinisignVerifier struct {
	public minisign.PublicKey
}

// NewMinisignVerifier 从标准 Minisign 公钥文本创建验证器。
func NewMinisignVerifier(publicKey string) (*MinisignVerifier, error) {
	var key minisign.PublicKey
	keyText := []byte(strings.TrimSpace(publicKey))
	if err := key.UnmarshalText(keyText); err != nil {
		// `tauri signer generate` stores the complete two-line Minisign
		// public-key text as a single base64 envelope. Keep that official
		// representation in CI because tauri-plugin-updater expects it.
		decoded, decodeErr := base64.StdEncoding.DecodeString(string(keyText))
		if decodeErr != nil || key.UnmarshalText(decoded) != nil {
			return nil, fmt.Errorf("parse minisign public key: %w", err)
		}
	}
	return &MinisignVerifier{public: key}, nil
}

// Verify 使用完整 Minisign 规则验证 payload、密钥 ID 和可信注释签名。
func (v *MinisignVerifier) Verify(payload []byte, signature string) error {
	if v == nil {
		return errors.New("minisign verifier is not configured")
	}
	// Tauri's updater JSON carries the complete four-line Minisign signature
	// file as one base64 string. Decode that transport envelope before passing
	// the textual signature to the Minisign verifier.
	signatureText, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return errors.New("minisign signature is not valid base64")
	}
	if !minisign.Verify(v.public, payload, signatureText) {
		return errors.New("minisign signature verification failed")
	}
	return nil
}

// LoadSignedManifestVerifier 按直接公钥优先、文件路径次之加载验证器；缺失公钥返回 nil，v1 不受影响。
func LoadSignedManifestVerifier(publicKey, publicKeyFile string) (SignedManifestVerifier, error) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" && strings.TrimSpace(publicKeyFile) != "" {
		contents, err := os.ReadFile(filepath.Clean(publicKeyFile))
		if err != nil {
			return nil, fmt.Errorf("read update signing public key file: %w", err)
		}
		publicKey = string(contents)
	}
	if publicKey == "" {
		return nil, nil
	}
	return NewMinisignVerifier(publicKey)
}

// DecodeSignedClientPayload 对 base64 payload 解码、验签并校验 v2 契约。
func DecodeSignedClientPayload(envelope *SignedClientUpdateManifest, verifier SignedManifestVerifier) (*ClientUpdatePayload, error) {
	if envelope == nil {
		return nil, nil
	}
	if verifier == nil {
		return nil, errors.New("v2 client update requires a configured minisign public key")
	}
	raw, err := decodeBase64Payload(envelope.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode signed client payload: %w", err)
	}
	if err := verifier.Verify(raw, envelope.Signature); err != nil {
		return nil, fmt.Errorf("verify signed client payload: %w", err)
	}
	var payload ClientUpdatePayload
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode client update payload: %w", err)
	}
	if err := validateClientUpdatePayload(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func decodeBase64Payload(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("signed payload is empty")
	}
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
		return raw, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func validateClientUpdatePayload(payload *ClientUpdatePayload) error {
	if payload == nil || payload.ProtocolVersion != 2 {
		return errors.New("unsupported client update protocol version")
	}
	if normalizeVersion(payload.Version) == "" {
		return errors.New("client update payload version is invalid")
	}
	if payload.Target != clientTargetWindowsX64 {
		return fmt.Errorf("unsupported client update target %q", payload.Target)
	}
	if payload.LayoutVersion <= 0 {
		return errors.New("client update layout version is required")
	}
	if err := validateClientArtifact(payload.Full.NSIS, "nsis"); err != nil {
		return fmt.Errorf("invalid full nsis artifact: %w", err)
	}
	if err := validateClientArtifact(payload.Full.Portable, "portable"); err != nil {
		return fmt.Errorf("invalid full portable artifact: %w", err)
	}
	for index, delta := range payload.Deltas {
		if err := validateClientArtifact(delta.ClientArtifact, "delta"); err != nil {
			return fmt.Errorf("invalid delta %d: %w", index, err)
		}
		if strings.TrimSpace(delta.Algorithm) != "zstd-patch-from-v1" {
			return fmt.Errorf("invalid delta %d algorithm", index)
		}
		if normalizeVersion(delta.FromVersion) == "" || !isSHA256(delta.FromSHA256) || !isSHA256(delta.TargetSHA256) {
			return fmt.Errorf("invalid delta %d source or target hash", index)
		}
	}
	return nil
}

func validateClientArtifact(artifact ClientArtifact, expectedKind string) error {
	if strings.TrimSpace(artifact.Kind) != expectedKind {
		return fmt.Errorf("kind must be %q", expectedKind)
	}
	if strings.TrimSpace(artifact.URL) == "" || !isSHA256(artifact.SHA256) || artifact.Size <= 0 || strings.TrimSpace(artifact.Signature) == "" {
		return errors.New("url, sha256, positive size and signature are required")
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256HexLength {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

const sha256HexLength = 64

// ArtifactStore 将经验证签名清单引用的资源按 SHA-256 内容寻址缓存。
type ArtifactStore interface {
	Ensure(context.Context, ClientArtifact) (path string, reused bool, err error)
	Cached(ClientArtifact) bool
	Path(sha256 string) (string, bool)
}

type artifactCall struct {
	done   chan struct{}
	path   string
	reused bool
	err    error
}

// verifiedFileSnapshot 是一次完整 SHA-256 校验后记录的不可变文件身份。
// 对外分发仅接受当前 manifest 白名单中的 SHA，并在热路径用它避免重复扫描大文件。
type verifiedFileSnapshot struct {
	path   string
	digest string
	size   int64
	info   os.FileInfo
}

func (s verifiedFileSnapshot) matches(path, digest string, info os.FileInfo) bool {
	if s.path != path || !strings.EqualFold(s.digest, digest) || info == nil || s.size != info.Size() || !s.info.ModTime().Equal(info.ModTime()) {
		return false
	}
	// SameFile 在支持文件 ID/inode 的平台上额外防止原子替换后保留相同长度和时间戳的情况。
	// 不支持文件身份的文件系统仍使用路径、长度、修改时间作为保守快照条件。
	if s.info.Sys() != nil && info.Sys() != nil {
		return os.SameFile(s.info, info)
	}
	return true
}

// LocalArtifactStore 使用 SHA-256 文件名，避免调用方控制文件路径。
type LocalArtifactStore struct {
	Root   string
	Client *http.Client

	mu         sync.Mutex
	inflight   map[string]*artifactCall
	verified   map[string]verifiedFileSnapshot
	verifyFile func(path, digest string) error
}

// Path 仅接受完整 SHA-256，绝不接受文件名或目录片段。
func (s *LocalArtifactStore) Path(digest string) (string, bool) {
	if !isSHA256(digest) {
		return "", false
	}
	path := filepath.Join(s.Root, "artifacts", strings.ToLower(digest))
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir() && info.Size() > 0
}

// Cached 校验内容地址对象的长度和哈希。
func (s *LocalArtifactStore) Cached(artifact ClientArtifact) bool {
	path, ok := s.Path(artifact.SHA256)
	if !ok {
		s.invalidate(artifact.SHA256)
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != artifact.Size {
		s.invalidate(artifact.SHA256)
		return false
	}

	digest := strings.ToLower(artifact.SHA256)
	s.mu.Lock()
	snapshot, verified := s.verified[digest]
	if verified {
		if snapshot.matches(path, digest, info) {
			s.mu.Unlock()
			return true
		}
		// 资源在校验后发生变化时立即失效，不在公开请求热路径重新扫描整个文件。
		delete(s.verified, digest)
		s.mu.Unlock()
		return false
	}
	// 没有快照仅发生在旧缓存、进程重启或第一次受控 Ensure 前；完整哈希校验只做一次。
	verify := s.verifyFile
	if verify == nil {
		verify = verifySHA256
	}
	err = verify(path, artifact.SHA256)
	if err == nil {
		s.rememberVerifiedLocked(digest, path, info)
	}
	s.mu.Unlock()
	return err == nil
}

// Ensure 合并同一 SHA 的并发下载，以临时文件和原子替换写入缓存。
func (s *LocalArtifactStore) Ensure(ctx context.Context, artifact ClientArtifact) (string, bool, error) {
	if !isSHA256(artifact.SHA256) || strings.TrimSpace(artifact.URL) == "" || artifact.Size <= 0 {
		return "", false, errors.New("invalid content-addressed artifact")
	}
	if s.Cached(artifact) {
		path, _ := s.Path(artifact.SHA256)
		return path, true, nil
	}
	key := strings.ToLower(artifact.SHA256)
	s.mu.Lock()
	if s.inflight == nil {
		s.inflight = make(map[string]*artifactCall)
	}
	if call := s.inflight[key]; call != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-call.done:
			return call.path, call.reused, call.err
		}
	}
	call := &artifactCall{done: make(chan struct{})}
	s.inflight[key] = call
	s.mu.Unlock()

	call.path, call.reused, call.err = s.ensure(ctx, artifact)
	s.mu.Lock()
	delete(s.inflight, key)
	close(call.done)
	s.mu.Unlock()
	return call.path, call.reused, call.err
}

func (s *LocalArtifactStore) ensure(ctx context.Context, artifact ClientArtifact) (string, bool, error) {
	if s.Cached(artifact) {
		path, _ := s.Path(artifact.SHA256)
		return path, true, nil
	}
	path, _ := s.Path(artifact.SHA256)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, fmt.Errorf("create artifact cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-*.tmp")
	if err != nil {
		return "", false, fmt.Errorf("create artifact temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", false, fmt.Errorf("close artifact temporary file: %w", err)
	}
	defer os.Remove(tmpPath)
	if err := downloadVerifiedFile(ctx, s.Client, artifact.URL, tmpPath, artifact.Size, artifact.SHA256); err != nil {
		return "", false, err
	}
	info, err := os.Stat(tmpPath)
	if err != nil || info.Size() != artifact.Size {
		return "", false, errors.New("artifact size mismatch")
	}
	if err := replaceCachedFile(tmpPath, path); err != nil {
		return "", false, fmt.Errorf("store artifact: %w", err)
	}
	info, err = os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("stat stored artifact: %w", err)
	}
	s.mu.Lock()
	s.rememberVerifiedLocked(strings.ToLower(artifact.SHA256), path, info)
	s.mu.Unlock()
	return path, false, nil
}

func (s *LocalArtifactStore) invalidate(digest string) {
	s.mu.Lock()
	delete(s.verified, strings.ToLower(digest))
	s.mu.Unlock()
}

func (s *LocalArtifactStore) rememberVerifiedLocked(digest, path string, info os.FileInfo) {
	if s.verified == nil {
		s.verified = make(map[string]verifiedFileSnapshot)
	}
	s.verified[digest] = verifiedFileSnapshot{path: path, digest: digest, size: info.Size(), info: info}
}

func clientPlanArtifact(artifact ClientArtifact, delta *ClientDeltaArtifact) ClientUpdatePlanArtifact {
	result := ClientUpdatePlanArtifact{
		Kind:         artifact.Kind,
		Algorithm:    artifact.Algorithm,
		SHA256:       strings.ToLower(artifact.SHA256),
		Size:         artifact.Size,
		Signature:    artifact.Signature,
		DownloadPath: "/api/v1/updates/client/artifacts/" + strings.ToLower(artifact.SHA256),
	}
	if delta != nil {
		result.FromVersion = delta.FromVersion
		result.FromSHA256 = strings.ToLower(delta.FromSHA256)
		result.TargetSHA256 = strings.ToLower(delta.TargetSHA256)
	}
	return result
}
