package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ClientUpdateProtocolV3 is the LAN release payload contract. It contains
	// only complete NSIS and Portable artifacts; deltas are intentionally not
	// published by the Windows Server 2016/Windows 10 release flow.
	ClientUpdateProtocolV3 = 3
	clientUpdateProtocolV2 = 2
	maxManifestSize        = int64(2 << 20)
)

// LocalManifestSource reads the stable manifest from the server installation
// directory. The path is configuration-controlled and never derived from an
// HTTP request.
type LocalManifestSource struct {
	Path string
}

// NewLocalManifestSource creates a local stable-manifest source.
func NewLocalManifestSource(manifestPath string) *LocalManifestSource {
	return &LocalManifestSource{Path: resolveManifestSourcePath(manifestPath, "")}
}

// resolveManifestSourcePath makes a relative configured path stable across
// process launchers. Windows Services normally start with a system directory
// as their working directory, so resolving against cwd would make the default
// updates/stable/update-manifest.json point at the wrong volume. The server
// executable directory is the installation root for both service and ordinary
// process deployments.
//
// executablePath is injectable for tests; an empty value asks os.Executable.
func resolveManifestSourcePath(manifestPath, executablePath string) string {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" || filepath.IsAbs(manifestPath) {
		return filepath.Clean(manifestPath)
	}
	if strings.TrimSpace(executablePath) == "" {
		executablePath, _ = os.Executable()
	}
	if executablePath != "" {
		if absolute, err := filepath.Abs(executablePath); err == nil {
			return filepath.Join(filepath.Dir(absolute), filepath.Clean(manifestPath))
		}
	}
	// This is only a development fallback (os.Executable can fail in unusual
	// test hosts); keep it deterministic rather than silently using a service's
	// arbitrary cwd.
	return filepath.Clean(manifestPath)
}

// Location returns the configured manifest path for status and diagnostics.
func (s *LocalManifestSource) Location() string {
	if s == nil {
		return ""
	}
	return s.Path
}

// Fetch reads and strictly decodes the stable manifest. A bounded read avoids
// allowing a damaged file to consume unbounded memory.
func (s *LocalManifestSource) Fetch(ctx context.Context) (*Manifest, error) {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("local update manifest file is empty")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	manifest, err := readManifestFile(s.Path)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

// ReadManifestFile reads a manifest for release/updater validation.
func ReadManifestFile(manifestPath string) (*Manifest, error) {
	return readManifestFile(manifestPath)
}

func readManifestFile(manifestPath string) (*Manifest, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return nil, errors.New("local update manifest file is empty")
	}
	file, err := os.Open(filepath.Clean(manifestPath))
	if err != nil {
		return nil, fmt.Errorf("open local update manifest %q: %w", manifestPath, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat local update manifest %q: %w", manifestPath, err)
	}
	if info.IsDir() || info.Size() <= 0 || info.Size() > maxManifestSize {
		return nil, fmt.Errorf("local update manifest size %d is outside the allowed range 1..%d", info.Size(), maxManifestSize)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("read local update manifest: %w", err)
	}
	if int64(len(data)) > maxManifestSize {
		return nil, fmt.Errorf("local update manifest exceeds %d bytes", maxManifestSize)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := json.NewDecoder(bytes.NewReader(data))
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode local update manifest: %w", err)
	}
	// Reject a second JSON value or arbitrary trailing content. This keeps the
	// atomically published file unambiguous and catches truncated/concatenated
	// release artifacts early.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("local update manifest contains multiple JSON values")
		}
		return nil, fmt.Errorf("decode trailing local update manifest data: %w", err)
	}
	if manifest.Version == "" && manifest.Server.Version == "" && manifest.Client.Version == "" && manifest.SignedClientUpdate() == nil {
		return nil, errors.New("manifest contains no package version")
	}
	return &manifest, nil
}

// LocalUpdatesRoot derives the updates directory containing stable/ and
// artifacts/ from a manifest path. It is exported for the updater and tests.
func LocalUpdatesRoot(manifestPath string) (string, error) {
	return localUpdatesRoot(manifestPath)
}

func localUpdatesRoot(manifestPath string) (string, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return "", errors.New("manifest path is empty")
	}
	absolute, err := filepath.Abs(filepath.Clean(manifestPath))
	if err != nil {
		return "", fmt.Errorf("resolve manifest path: %w", err)
	}
	// stable/update-manifest.json is the only supported stable location. Do not
	// use an arbitrary parent supplied by a caller as an artifact root.
	if !strings.EqualFold(filepath.Base(absolute), "update-manifest.json") || !strings.EqualFold(filepath.Base(filepath.Dir(absolute)), "stable") {
		return "", fmt.Errorf("manifest path must end in stable%cupdate-manifest.json", os.PathSeparator)
	}
	return filepath.Dir(filepath.Dir(absolute)), nil
}

// ValidateLocalClientArtifacts verifies both required v3 full packages in the
// content-addressed artifacts directory. It does not trust URL fields from a
// manifest; every file is resolved from its lower-case SHA-256 name.
func ValidateLocalClientArtifacts(manifest *Manifest, updatesRoot string, verifier SignedManifestVerifier) (*ClientUpdatePayload, error) {
	if manifest == nil {
		return nil, errors.New("manifest is nil")
	}
	envelope := manifest.SignedClientUpdate()
	if envelope == nil {
		return nil, errors.New("manifest does not contain a signed client update payload")
	}
	payload, err := DecodeSignedClientPayload(envelope, verifier)
	if err != nil {
		return nil, err
	}
	if payload.ProtocolVersion != ClientUpdateProtocolV3 {
		return nil, fmt.Errorf("local client update requires protocol version %d, got %d", ClientUpdateProtocolV3, payload.ProtocolVersion)
	}
	if len(payload.Deltas) != 0 {
		return nil, errors.New("local protocol v3 payload must not contain delta artifacts")
	}
	if manifest.Version != "" && CompareVersions(manifest.Version, payload.Version) != 0 {
		return nil, errors.New("signed client update version does not match manifest version")
	}
	if manifest.Client.Version != "" && CompareVersions(manifest.Client.Version, payload.Version) != 0 {
		return nil, errors.New("signed client update version does not match legacy client version")
	}
	root, err := filepath.Abs(filepath.Clean(updatesRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve local updates root: %w", err)
	}
	for _, artifact := range []ClientArtifact{payload.Full.NSIS, payload.Full.Portable} {
		if err := verifyLocalArtifact(root, artifact); err != nil {
			return nil, fmt.Errorf("verify local %s artifact: %w", artifact.Kind, err)
		}
		artifactPath := filepath.Join(root, "artifacts", strings.ToLower(strings.TrimSpace(artifact.SHA256)))
		if err := verifier.VerifyFile(artifactPath, artifact.Signature); err != nil {
			return nil, fmt.Errorf("verify local %s artifact Minisign signature: %w", artifact.Kind, err)
		}
	}
	return payload, nil
}

func verifyLocalArtifact(updatesRoot string, artifact ClientArtifact) error {
	if err := validateClientArtifactPolicy(artifact, artifact.Kind, false); err != nil {
		return err
	}
	digest := strings.ToLower(strings.TrimSpace(artifact.SHA256))
	artifactRoot := filepath.Join(updatesRoot, "artifacts")
	artifactRootInfo, err := os.Lstat(artifactRoot)
	if err != nil {
		return fmt.Errorf("inspect artifact directory: %w", err)
	}
	if artifactRootInfo.Mode()&os.ModeSymlink != 0 || !artifactRootInfo.IsDir() {
		return errors.New("artifact directory is not a regular directory")
	}
	path := filepath.Join(artifactRoot, digest)
	if filepath.Dir(path) != artifactRoot || filepath.Base(path) != digest {
		return errors.New("artifact path is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("artifact file is missing: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != artifact.Size {
		return fmt.Errorf("artifact file size/type mismatch: got %d want %d", info.Size(), artifact.Size)
	}
	if err := verifySHA256(path, digest); err != nil {
		return err
	}
	return nil
}
