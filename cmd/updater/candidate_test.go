package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
	"bb_erp_echo/internal/update"
)

func clientRecoveryZIPFixture(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for name, content := range map[string]string{
		"client/bb_erp_client.exe":          "portable",
		"client/bb-erp-portable.json":       `{"layout_version":1}`,
		"installer/bb-erp-client-setup.exe": "installer",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create recovery ZIP entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write recovery ZIP entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close recovery ZIP: %v", err)
	}
	return body.Bytes()
}

func TestInstallStableManifestReplacesOnlyStableManifest(t *testing.T) {
	installDir := t.TempDir()
	stablePath := filepath.Join(installDir, "updates", "stable", "update-manifest.json")
	candidatePath := filepath.Join(t.TempDir(), "pending-manifest.json")
	if err := os.MkdirAll(filepath.Dir(stablePath), 0o755); err != nil {
		t.Fatalf("create stable directory: %v", err)
	}
	if err := os.WriteFile(stablePath, []byte(`{"version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write old stable manifest: %v", err)
	}
	if err := os.WriteFile(candidatePath, []byte(`{"version":"1.0.1"}`), 0o600); err != nil {
		t.Fatalf("write candidate manifest: %v", err)
	}
	if err := installStableManifest(candidatePath, installDir); err != nil {
		t.Fatalf("install stable manifest: %v", err)
	}
	got, err := os.ReadFile(stablePath)
	if err != nil || string(got) != `{"version":"1.0.1"}` {
		t.Fatalf("stable manifest = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(candidatePath); err != nil || string(got) != `{"version":"1.0.1"}` {
		t.Fatalf("candidate manifest should remain independently readable: %q, err=%v", got, err)
	}
}

func TestLoadCandidateReleaseValidatesLocalV3Artifacts(t *testing.T) {
	installDir := t.TempDir()
	updatesRoot := filepath.Join(installDir, "updates")
	artifactDir := filepath.Join(updatesRoot, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "update-public.key"), []byte(public.String()), 0o600); err != nil {
		t.Fatalf("write trusted public key: %v", err)
	}
	makeArtifact := func(kind string, body []byte) update.ClientArtifact {
		digest := sha256.Sum256(body)
		if err := os.WriteFile(filepath.Join(artifactDir, hex.EncodeToString(digest[:])), body, 0o600); err != nil {
			t.Fatalf("write %s artifact: %v", kind, err)
		}
		reader := minisign.NewReader(bytes.NewReader(body))
		_, _ = io.Copy(io.Discard, reader)
		return update.ClientArtifact{
			Kind: kind, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body)),
			Signature: base64.StdEncoding.EncodeToString(reader.Sign(private)),
		}
	}
	payload := update.ClientUpdatePayload{
		ProtocolVersion: update.ClientUpdateProtocolV3,
		Version:         "1.0.1",
		Target:          "windows-x86_64",
		LayoutVersion:   1,
		Full: update.ClientFullArtifacts{
			NSIS: makeArtifact("nsis", []byte("nsis")), Portable: makeArtifact("portable", []byte("portable")),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	envelope := &update.SignedClientUpdateManifest{
		Payload: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(minisign.Sign(private, raw)),
	}
	recoveryBody := clientRecoveryZIPFixture(t)
	recoveryDigest := sha256.Sum256(recoveryBody)
	recoveryReader := minisign.NewReader(bytes.NewReader(recoveryBody))
	_, _ = io.Copy(io.Discard, recoveryReader)
	candidate := update.Manifest{
		Version: "1.0.1", Server: update.PackageManifest{Version: "1.0.1", Size: 1, SHA256: strings.Repeat("a", 64), Signature: "server-signature"},
		Client:         update.PackageManifest{Version: "1.0.1", Size: int64(len(recoveryBody)), SHA256: hex.EncodeToString(recoveryDigest[:]), Signature: base64.StdEncoding.EncodeToString(recoveryReader.Sign(private))},
		ClientUpdateV3: envelope,
	}
	candidateRoot := t.TempDir()
	candidatePath := filepath.Join(candidateRoot, "candidate.json")
	if err := os.WriteFile(filepath.Join(candidateRoot, "bb-erp-client-windows.zip"), recoveryBody, 0o600); err != nil {
		t.Fatalf("write recovery ZIP: %v", err)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	if err := os.WriteFile(candidatePath, encoded, 0o600); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	release, err := loadCandidateRelease(candidatePath, installDir)
	if err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
	if release.Manifest.Version != "1.0.1" || release.Payload.ProtocolVersion != update.ClientUpdateProtocolV3 {
		t.Fatalf("unexpected candidate release: %+v", release)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, payload.Full.NSIS.SHA256), []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper candidate artifact: %v", err)
	}
	if _, err := loadCandidateRelease(candidatePath, installDir); err == nil {
		t.Fatal("tampered candidate artifact must be rejected")
	}
}

func TestHealthChecksRequireReadyVersionAndFullV3Plan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			_, _ = w.Write([]byte(`{"status":"ready"}`))
		case "/api/v1/version":
			_, _ = w.Write([]byte(`{"server_version":"1.0.1"}`))
		case "/api/v1/updates/client/plan":
			_, _ = w.Write([]byte(`{"protocol_version":3,"latest_version":"1.0.1","target":"windows-x86_64","install_mode":"portable","strategy":"full","artifact":{"kind":"portable","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1,"signature":"signed-artifact","download_path":"/api/v1/updates/client/artifacts/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"full_fallback":{"kind":"portable","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1,"signature":"signed-artifact","download_path":"/api/v1/updates/client/artifacts/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, err := parseHealthBaseURL(server.URL + "/")
	if err != nil {
		t.Fatalf("parse health URL: %v", err)
	}
	client := server.Client()
	if err := checkReady(client, base); err != nil {
		t.Fatalf("ready check: %v", err)
	}
	if err := checkVersion(client, base, "1.0.1"); err != nil {
		t.Fatalf("version check: %v", err)
	}
	if err := checkClientPlan(client, base, "1.0.0", "1.0.1"); err != nil {
		t.Fatalf("client plan check: %v", err)
	}
	if _, err := parseHealthBaseURL("file:///tmp/server"); err == nil {
		t.Fatal("non-HTTP health URL must be rejected")
	}
}

func TestHealthClientPlanAcceptsNoUpdateOnlyForRestoredVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base, err := parseHealthBaseURL(server.URL)
	if err != nil {
		t.Fatalf("parse health URL: %v", err)
	}
	if err := checkClientPlan(server.Client(), base, "1.0.0", "1.0.0"); err != nil {
		t.Fatalf("restored version 204 must pass: %v", err)
	}
	if err := checkClientPlan(server.Client(), base, "1.0.0", "1.0.1"); err == nil {
		t.Fatal("candidate version 204 must fail")
	}
}
