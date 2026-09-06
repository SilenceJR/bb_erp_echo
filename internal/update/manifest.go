// Package update implements the fixed, LAN-only portable client update
// contract. The server never fetches a remote manifest and never upgrades
// itself; an administrator places a signed manifest and executable beside it.
package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/shared/jsonstrict"
)

const (
	maxManifestSize = 2 << 20
	// DefaultClientCacheDir is relative to the server executable directory.
	DefaultClientCacheDir = "updates/client-cache"
	clientManifestName    = "client-update.json"
	clientArtifactsPath   = "/api/v1/client-updates/artifacts/"
)

var (
	// ErrClientUpdateNotPublished means the fixed client directory has no valid
	// published manifest yet. It is intentionally safe to expose to clients.
	ErrClientUpdateNotPublished = errors.New("client update is not published")
	// ErrClientUpdateInvalid means a manifest or executable was found but did not
	// pass strict parsing, signature, size or hash validation.
	ErrClientUpdateInvalid = errors.New("client update is invalid")
	// ErrInvalidClientVersion means the caller supplied a non-SemVer version.
	ErrInvalidClientVersion      = errors.New("current client version must be a valid semver")
	errClientUpdateSourceChanged = errors.New("client update source changed during verification")
)

// UpdateService is the minimal public service used by the HTTP handler. A
// check refreshes the local source every time; no periodic background worker
// or administrator update permission is involved.
type UpdateService interface {
	CheckClientUpdate(context.Context, string) (ClientUpdatePlan, bool, error)
	ClientArtifact(string) (path string, artifact ClientArtifact, ok bool)
}

type clientSnapshot struct {
	payload         ClientUpdatePayload
	envelope        SignedClientUpdateManifest
	cachePath       string
	checkedAt       time.Time
	manifestSize    int64
	manifestModTime time.Time
}

type checkCall struct {
	done     chan struct{}
	snapshot *clientSnapshot
	err      error
}

// Service reads the fixed sibling client directory, verifies a signed update,
// and publishes a content-addressed cache object before committing a new
// snapshot. The in-flight pointer coalesces concurrent requests.
type Service struct {
	cfg           config.UpdateConfig
	serverVersion string
	clientDir     string
	cacheDir      string
	verifier      SignedManifestVerifier
	verifierErr   error

	mu             sync.RWMutex
	snapshot       *clientSnapshot
	inflight       *checkCall
	lastFailureKey string
	lastFailureErr error
}

// NewService constructs the production fixed-directory service. Relative
// ClientDir and CacheDir values resolve from the server executable directory.
func NewService(cfg config.UpdateConfig, serverVersion string) *Service {
	verifier, verifierErr := LoadSignedManifestVerifier(cfg.SigningPublicKey, cfg.SigningPublicKeyFile)
	return NewServiceWithVerifier(cfg, serverVersion, verifier, verifierErr)
}

// NewServiceWithVerifier is injectable for tests and alternate trusted-key
// providers. It does not change the fixed directory or wire contract.
func NewServiceWithVerifier(cfg config.UpdateConfig, serverVersion string, verifier SignedManifestVerifier, verifierErr error) *Service {
	base := executableDir()
	clientDir := strings.TrimSpace(cfg.ClientDir)
	if clientDir == "" {
		clientDir = filepath.Join(base, "..", "client")
	} else if !filepath.IsAbs(clientDir) {
		clientDir = filepath.Join(base, clientDir)
	}
	cacheDir := strings.TrimSpace(cfg.CacheDir)
	if cacheDir == "" {
		cacheDir = DefaultClientCacheDir
	}
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(base, cacheDir)
	}
	return &Service{
		cfg: cfg, serverVersion: serverVersion,
		clientDir: filepath.Clean(clientDir), cacheDir: filepath.Clean(cacheDir),
		verifier: verifier, verifierErr: verifierErr,
	}
}

func executableDir() string {
	if executable, err := os.Executable(); err == nil && strings.TrimSpace(executable) != "" {
		return filepath.Dir(filepath.Clean(executable))
	}
	if working, err := os.Getwd(); err == nil {
		return working
	}
	return "."
}

// CheckClientUpdate refreshes the fixed source and returns a plan only when
// latest version is strictly newer than current. A failed refresh never
// replaces an earlier verified snapshot.
func (s *Service) CheckClientUpdate(ctx context.Context, currentVersion string) (ClientUpdatePlan, bool, error) {
	currentVersion = strings.TrimSpace(currentVersion)
	if normalizeVersion(currentVersion) == "" {
		return ClientUpdatePlan{}, false, ErrInvalidClientVersion
	}
	snapshot, err := s.refresh(ctx)
	if err != nil {
		return ClientUpdatePlan{}, false, err
	}
	if snapshot == nil || CompareVersions(snapshot.payload.Version, currentVersion) <= 0 {
		return ClientUpdatePlan{}, false, nil
	}
	return s.planFor(snapshot, currentVersion), true, nil
}

func (s *Service) planFor(snapshot *clientSnapshot, currentVersion string) ClientUpdatePlan {
	artifact := snapshot.payload.Artifact
	digest := strings.ToLower(artifact.SHA256)
	return ClientUpdatePlan{
		ProtocolVersion: ClientUpdateProtocolVersion,
		CurrentVersion:  currentVersion,
		LatestVersion:   snapshot.payload.Version,
		Target:          snapshot.payload.Target,
		Strategy:        "full",
		DownloadSize:    artifact.Size,
		SignedPayload:   snapshot.envelope.Payload,
		Signature:       snapshot.envelope.Signature,
		Artifact: ClientUpdatePlanArtifact{
			Kind:         artifact.Kind,
			Size:         artifact.Size,
			SHA256:       digest,
			Signature:    artifact.Signature,
			DownloadPath: clientArtifactsPath + digest,
		},
	}
}

func (s *Service) refresh(ctx context.Context) (*clientSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// A stable manifest means the already committed content-addressed snapshot
	// is still authoritative. This cheap metadata check prevents anonymous check
	// requests from repeatedly hashing and signature-verifying a large EXE.
	manifestKey := sourceManifestKey(filepath.Join(s.clientDir, clientManifestName))
	s.mu.RLock()
	currentSnapshot := cloneSnapshot(s.snapshot)
	s.mu.RUnlock()
	if currentSnapshot != nil {
		if info, err := os.Stat(filepath.Join(s.clientDir, clientManifestName)); err == nil &&
			info.Mode().IsRegular() && info.Size() == currentSnapshot.manifestSize &&
			info.ModTime().Equal(currentSnapshot.manifestModTime) {
			return currentSnapshot, nil
		}
	}

	s.mu.Lock()
	if s.inflight != nil {
		call := s.inflight
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-call.done:
			if call.err != nil {
				return nil, call.err
			}
			return cloneSnapshot(call.snapshot), nil
		}
	}
	if manifestKey != "" && manifestKey == s.lastFailureKey && s.lastFailureErr != nil {
		failure := s.lastFailureErr
		s.mu.Unlock()
		if currentSnapshot != nil {
			return currentSnapshot, nil
		}
		return currentSnapshot, failure
	}
	call := &checkCall{done: make(chan struct{})}
	s.inflight = call
	s.mu.Unlock()

	// Refresh is shared service work, so a caller disconnect must not cancel and
	// poison the publication attempt for every other client. Each caller can
	// still stop waiting through its own request context.
	go func() {
		refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		snapshot, err := s.loadAndCache(refreshCtx)
		s.mu.Lock()
		previousSnapshot := cloneSnapshot(s.snapshot)
		if err == nil {
			s.snapshot = snapshot
			s.lastFailureKey = ""
			s.lastFailureErr = nil
		} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errClientUpdateSourceChanged) {
			s.lastFailureKey = manifestKey
			s.lastFailureErr = err
		}
		call.snapshot = cloneSnapshot(s.snapshot)
		call.err = err
		s.inflight = nil
		close(call.done)
		s.mu.Unlock()
		if err != nil && previousSnapshot != nil {
			slog.Warn("client update refresh failed; serving previous verified snapshot", "error", err)
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		if call.err != nil {
			if call.snapshot != nil {
				return cloneSnapshot(call.snapshot), nil
			}
			return nil, call.err
		}
		return cloneSnapshot(call.snapshot), nil
	}
}

func sourceManifestKey(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "missing:" + err.Error()
	}
	return fmt.Sprintf("%d:%d:%d", info.Size(), info.ModTime().UnixNano(), info.Mode())
}

func (s *Service) loadAndCache(ctx context.Context) (*clientSnapshot, error) {
	if s.verifierErr != nil {
		return nil, fmt.Errorf("%w: load signing public key: %v", ErrClientUpdateInvalid, s.verifierErr)
	}
	if s.verifier == nil {
		return nil, fmt.Errorf("%w: signing public key is not configured", ErrClientUpdateNotPublished)
	}
	manifestPath, err := regularFile(filepath.Join(s.clientDir, clientManifestName), s.clientDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrClientUpdateNotPublished
		}
		return nil, fmt.Errorf("%w: locate client update manifest: %v", ErrClientUpdateInvalid, err)
	}
	manifestRaw, err := readBoundedFile(manifestPath, maxManifestSize)
	if err != nil {
		return nil, fmt.Errorf("%w: read client update manifest: %v", ErrClientUpdateInvalid, err)
	}
	envelope, err := parseSignedClientManifest(manifestRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClientUpdateInvalid, err)
	}
	payload, err := DecodeSignedClientPayload(envelope, s.verifier)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClientUpdateInvalid, err)
	}
	artifactPath, err := regularFile(filepath.Join(s.clientDir, clientArtifactFileName), s.clientDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s is missing", ErrClientUpdateNotPublished, clientArtifactFileName)
		}
		return nil, fmt.Errorf("%w: locate client executable: %v", ErrClientUpdateInvalid, err)
	}
	cachePath, err := s.ensureCachedArtifact(ctx, artifactPath, payload.Artifact)
	if err != nil {
		return nil, fmt.Errorf("%w: verify client executable: %v", ErrClientUpdateInvalid, err)
	}
	// The manifest is published last. Re-read it after the EXE copy so a
	// concurrent deployment can never commit payload M1 with metadata from M2.
	manifestAfter, err := readBoundedFile(manifestPath, maxManifestSize)
	if err != nil {
		return nil, fmt.Errorf("%w: re-read client update manifest: %v", errClientUpdateSourceChanged, err)
	}
	if !bytes.Equal(manifestRaw, manifestAfter) {
		return nil, errClientUpdateSourceChanged
	}
	manifestInfo, err := os.Stat(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("%w: stat client update manifest: %v", ErrClientUpdateInvalid, err)
	}
	return &clientSnapshot{
		payload:         *payload,
		envelope:        *envelope,
		cachePath:       cachePath,
		checkedAt:       time.Now(),
		manifestSize:    manifestInfo.Size(),
		manifestModTime: manifestInfo.ModTime(),
	}, nil
}

func parseSignedClientManifest(raw []byte) (*SignedClientUpdateManifest, error) {
	if err := jsonstrict.RejectDuplicateKeys(raw); err != nil {
		return nil, err
	}
	var envelope SignedClientUpdateManifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON content is not allowed")
	}
	if err := validateEnvelope(&envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return contents, nil
}

// regularFile resolves a path below root while rejecting symlinks at every
// component. This avoids a reparse-point/junction escape on Windows as well as
// ordinary Unix symlink traversal.
func regularFile(path, root string) (string, error) {
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", errors.New("client directory is not a real directory")
	}
	target := filepath.Clean(path)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("client resource escapes client directory")
	}
	current := root
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("client resource contains symlink")
		}
		if current != target && !info.IsDir() {
			return "", errors.New("client resource parent is not a directory")
		}
		if current == target && (!info.Mode().IsRegular() || info.Size() <= 0) {
			return "", errors.New("client resource is not a non-empty regular file")
		}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	resolvedRel, err := filepath.Rel(filepath.Clean(resolvedRoot), filepath.Clean(resolvedTarget))
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRel) {
		return "", errors.New("client resource escapes client directory")
	}
	return target, nil
}

func (s *Service) ensureCachedArtifact(ctx context.Context, sourcePath string, artifact ClientArtifact) (string, error) {
	if err := validateClientArtifact(artifact); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory: %w", err)
	}
	if info, err := os.Lstat(s.cacheDir); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return "", fmt.Errorf("stat cache directory: %w", err)
		}
		return "", errors.New("cache directory must not be a symlink")
	}
	digest := strings.ToLower(artifact.SHA256)
	cachePath := filepath.Join(s.cacheDir, digest)
	if validCachedArtifact(cachePath, artifact, s.verifier) {
		// Still validate the newly placed source below: an administrator may have
		// replaced the file while keeping the same manifest name.
		if err := verifyArtifactFile(sourcePath, artifact, s.verifier); err != nil {
			return "", err
		}
		return cachePath, nil
	}
	if err := verifyArtifactFile(sourcePath, artifact, s.verifier); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.cacheDir, ".client-update-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create cache temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := copyFileContext(ctx, tmp, sourcePath); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync cached executable: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close cached executable: %w", err)
	}
	if err := verifyArtifactFile(tmpPath, artifact, s.verifier); err != nil {
		return "", fmt.Errorf("verify cached executable: %w", err)
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		if validCachedArtifact(cachePath, artifact, s.verifier) {
			return cachePath, nil
		}
		return "", fmt.Errorf("publish cached executable: %w", err)
	}
	return cachePath, nil
}

func validCachedArtifact(path string, artifact ClientArtifact, verifier SignedManifestVerifier) bool {
	if _, err := regularFile(path, filepath.Dir(path)); err != nil {
		return false
	}
	digest, size, err := fileSHA256(path)
	if err != nil || size != artifact.Size || !strings.EqualFold(digest, artifact.SHA256) {
		return false
	}
	return verifier != nil && verifier.VerifyFile(path, artifact.Signature) == nil
}

func verifyArtifactFile(path string, artifact ClientArtifact, verifier SignedManifestVerifier) error {
	digest, size, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("hash executable: %w", err)
	}
	if size != artifact.Size {
		return fmt.Errorf("executable size %d does not match manifest size %d", size, artifact.Size)
	}
	if !strings.EqualFold(digest, artifact.SHA256) {
		return errors.New("executable sha256 does not match manifest")
	}
	if verifier == nil {
		return errors.New("signing public key is not configured")
	}
	if err := verifier.VerifyFile(path, artifact.Signature); err != nil {
		return fmt.Errorf("verify executable signature: %w", err)
	}
	return nil
}

func copyFileContext(ctx context.Context, dst *os.File, sourcePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open client executable: %w", err)
	}
	defer src.Close()
	buffer := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := src.Read(buffer)
		if count > 0 {
			if _, err := dst.Write(buffer[:count]); err != nil {
				return fmt.Errorf("write cached executable: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read client executable: %w", readErr)
		}
	}
}

// ClientArtifact returns only the currently committed, verified artifact.
func (s *Service) ClientArtifact(digest string) (string, ClientArtifact, bool) {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if !isSHA256(digest) {
		return "", ClientArtifact{}, false
	}
	s.mu.RLock()
	snapshot := cloneSnapshot(s.snapshot)
	s.mu.RUnlock()
	if snapshot == nil || !strings.EqualFold(snapshot.payload.Artifact.SHA256, digest) {
		return "", ClientArtifact{}, false
	}
	// cachePath belongs to an immutable, content-addressed snapshot that was
	// fully verified before the pointer was committed. Do not re-hash it for
	// every anonymous Range request.
	return snapshot.cachePath, snapshot.payload.Artifact, true
}

func cloneSnapshot(snapshot *clientSnapshot) *clientSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	return &clone
}
