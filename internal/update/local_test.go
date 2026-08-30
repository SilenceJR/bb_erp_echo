package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aead.dev/minisign"
	"bb_erp_echo/internal/config"
)

func localV3Artifact(kind string, body []byte, private minisign.PrivateKey) ClientArtifact {
	digest := sha256.Sum256(body)
	reader := minisign.NewReader(bytes.NewReader(body))
	_, _ = io.Copy(io.Discard, reader)
	return ClientArtifact{
		Kind: kind, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body)),
		Signature: base64.StdEncoding.EncodeToString(reader.Sign(private)),
	}
}

func localV3Envelope(t *testing.T, payload ClientUpdatePayload, private minisign.PrivateKey) *SignedClientUpdateManifest {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal local v3 payload: %v", err)
	}
	return &SignedClientUpdateManifest{
		Payload:   base64.StdEncoding.EncodeToString(raw),
		Signature: base64.StdEncoding.EncodeToString(minisign.Sign(private, raw)),
	}
}

func TestLocalManifestSourceReadsStableManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates", "stable", "update-manifest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"1.0.1"}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	source := NewLocalManifestSource(path)
	manifest, err := source.Fetch(context.Background())
	if err != nil || manifest.Version != "1.0.1" || source.Location() != path {
		t.Fatalf("local manifest = %#v, err=%v, location=%q", manifest, err, source.Location())
	}
	if err := os.WriteFile(path, []byte(`{"version":"1.0.1"} {"version":"1.0.2"}`), 0o600); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	if _, err := source.Fetch(context.Background()); err == nil {
		t.Fatal("manifest with multiple JSON values must be rejected")
	}
}

func TestResolveManifestSourcePathUsesExecutableDirectoryForRelativePath(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "server", "bb-erp-server.exe")
	got := resolveManifestSourcePath(filepath.Join("updates", "stable", "update-manifest.json"), executable)
	want := filepath.Join(filepath.Dir(executable), "updates", "stable", "update-manifest.json")
	if got != want {
		t.Fatalf("resolved manifest path = %q, want %q", got, want)
	}
}

func TestLocalV3ServiceReadsContentAddressedArtifactsWithoutURL(t *testing.T) {
	updatesRoot := filepath.Join(t.TempDir(), "updates")
	manifestPath := filepath.Join(updatesRoot, "stable", "update-manifest.json")
	artifactRoot := filepath.Join(updatesRoot, "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create artifact root: %v", err)
	}
	nsis, portable := []byte("nsis local package"), []byte("portable local package")
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate local v3 signing key: %v", err)
	}
	nsisArtifact := localV3Artifact("nsis", nsis, private)
	portableArtifact := localV3Artifact("portable", portable, private)
	for _, artifactBody := range [][]byte{nsis, portable} {
		digest := sha256.Sum256(artifactBody)
		if err := os.WriteFile(filepath.Join(artifactRoot, hex.EncodeToString(digest[:])), artifactBody, 0o600); err != nil {
			t.Fatalf("write local artifact: %v", err)
		}
	}
	payload := ClientUpdatePayload{
		ProtocolVersion: ClientUpdateProtocolV3,
		Version:         "1.0.1",
		Target:          clientTargetWindowsX64,
		LayoutVersion:   1,
		Full:            ClientFullArtifacts{NSIS: nsisArtifact, Portable: portableArtifact},
	}
	envelope := localV3Envelope(t, payload, private)
	manifest := Manifest{
		Version:        "1.0.1",
		Client:         PackageManifest{Version: "1.0.1"},
		ClientUpdateV3: envelope,
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create stable directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatalf("write stable manifest: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyPath, []byte(public.String()), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	service := NewService(config.UpdateConfig{
		Enabled: true, ManifestFile: manifestPath, CacheDir: filepath.Join(t.TempDir(), "cache"),
		ClientVersion: "1.0.0", CheckInterval: time.Hour, SigningPublicKeyFile: keyPath,
	}, "1.0.0")
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check local v3 manifest: %v", err)
	}
	if status.ClientProtocolVersion != ClientUpdateProtocolV3 || !status.ClientFullCached || !status.Client.Available {
		t.Fatalf("unexpected local v3 status: %+v", status)
	}
	plan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", Target: clientTargetWindowsX64, InstallMode: installModeNSIS,
	})
	if err != nil || !available || plan.ProtocolVersion != ClientUpdateProtocolV3 || plan.Strategy != "full" || plan.Artifact.Kind != "nsis" {
		t.Fatalf("unexpected local v3 plan: %#v, available=%v, err=%v", plan, available, err)
	}
	if plan.Artifact.DownloadPath != "/api/v1/updates/client/artifacts/"+nsisArtifact.SHA256 {
		t.Fatalf("unexpected local artifact path: %q", plan.Artifact.DownloadPath)
	}
	if plan.Artifact.Size != int64(len(nsis)) || plan.Artifact.Signature != nsisArtifact.Signature {
		t.Fatalf("local artifact metadata was not retained: %+v", plan.Artifact)
	}
	if _, err := os.Stat(filepath.Join(updatesRoot, "artifacts", nsisArtifact.SHA256)); err != nil {
		t.Fatalf("local nsis artifact missing: %v", err)
	}
}

func TestValidateLocalClientArtifactsRejectsChangedArtifact(t *testing.T) {
	root := t.TempDir()
	nsis := []byte("nsis")
	portable := []byte("portable")
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate local v3 signing key: %v", err)
	}
	payload := ClientUpdatePayload{
		ProtocolVersion: ClientUpdateProtocolV3, Version: "1.0.1", Target: clientTargetWindowsX64, LayoutVersion: 1,
		Full: ClientFullArtifacts{NSIS: localV3Artifact("nsis", nsis, private), Portable: localV3Artifact("portable", portable, private)},
	}
	envelope := localV3Envelope(t, payload, private)
	publicVerifier, err := NewMinisignVerifier(public.String())
	if err != nil {
		t.Fatalf("create verifier: %v", err)
	}
	artifactsDir := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("create artifacts directory: %v", err)
	}
	for _, artifactBody := range [][]byte{nsis, portable} {
		digest := sha256.Sum256(artifactBody)
		if err := os.WriteFile(filepath.Join(artifactsDir, hex.EncodeToString(digest[:])), artifactBody, 0o600); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}
	if _, err := ValidateLocalClientArtifacts(&Manifest{Version: "1.0.1", ClientUpdateV3: envelope}, root, publicVerifier); err != nil {
		t.Fatalf("valid local artifacts rejected: %v", err)
	}
	badSignatureManifest := Manifest{Version: "1.0.1", ClientUpdateV3: envelope}
	badPayload := payload
	badReader := minisign.NewReader(strings.NewReader("different artifact"))
	_, _ = io.Copy(io.Discard, badReader)
	badPayload.Full.Portable.Signature = base64.StdEncoding.EncodeToString(badReader.Sign(private))
	badSignatureManifest.ClientUpdateV3 = localV3Envelope(t, badPayload, private)
	if _, err := ValidateLocalClientArtifacts(&badSignatureManifest, root, publicVerifier); err == nil {
		t.Fatal("artifact with a mismatched Minisign signature must be rejected")
	}
	digest := strings.TrimSpace(payload.Full.NSIS.SHA256)
	if err := os.WriteFile(filepath.Join(artifactsDir, digest), []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper artifact: %v", err)
	}
	if _, err := ValidateLocalClientArtifacts(&Manifest{Version: "1.0.1", ClientUpdateV3: envelope}, root, publicVerifier); err == nil {
		t.Fatal("changed local artifact must be rejected")
	}
}
