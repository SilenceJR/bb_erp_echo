package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aead.dev/minisign"
	"bb_erp_echo/internal/config"
	"github.com/labstack/echo/v5"
)

type staticManifestSource struct{ manifest *Manifest }

func (s staticManifestSource) Fetch(context.Context) (*Manifest, error) {
	return cloneManifest(s.manifest), nil
}
func (s staticManifestSource) Location() string { return "memory://signed-manifest" }

func artifactForTest(kind, url string, body []byte) ClientArtifact {
	digest := sha256.Sum256(body)
	return ClientArtifact{
		Kind: kind, URL: url, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body)), Signature: "artifact-signature",
	}
}

func signedEnvelopeForTest(t *testing.T, payload ClientUpdatePayload) (*SignedClientUpdateManifest, SignedManifestVerifier) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate minisign key: %v", err)
	}
	verifier, err := NewMinisignVerifier(public.String())
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return &SignedClientUpdateManifest{
		Payload: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(minisign.Sign(private, raw)),
	}, verifier
}

func TestMinisignVerifierAcceptsTauriPublicKeyEnvelope(t *testing.T) {
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicText, err := public.MarshalText()
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	verifier, err := NewMinisignVerifier(base64.StdEncoding.EncodeToString(publicText))
	if err != nil {
		t.Fatalf("parse Tauri public key envelope: %v", err)
	}
	payload := []byte("signed update payload")
	signature := base64.StdEncoding.EncodeToString(minisign.Sign(private, payload))
	if err := verifier.Verify(payload, signature); err != nil {
		t.Fatalf("verify Tauri signature envelope: %v", err)
	}
}

func TestMinisignVerifierAcceptsPublishedCanonicalPublicKeyEnvelope(t *testing.T) {
	const publishedKey = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IDk4Y2E4ZTFhMmNlOWQxNTcKUldSWDBla3NHbzdLbUpoZnlmNWlwY1p6eEdEaUFiNmlFVVpsNTRIcnV0RmI5NjlFMytNNFlTQVcK"
	if _, err := NewMinisignVerifier(publishedKey); err != nil {
		t.Fatalf("parse published canonical public key envelope: %v", err)
	}
}

func TestLoadSignedManifestVerifierFallsBackToConfiguredFile(t *testing.T) {
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicText, err := public.MarshalText()
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(publicText)), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	verifier, err := LoadSignedManifestVerifier("pub-key", keyFile)
	if err != nil {
		t.Fatalf("load fallback public key file: %v", err)
	}
	payload := []byte("signed update payload")
	signature := base64.StdEncoding.EncodeToString(minisign.Sign(private, payload))
	if err := verifier.Verify(payload, signature); err != nil {
		t.Fatalf("verify with fallback public key file: %v", err)
	}
}

func TestLoadSignedManifestVerifierKeepsValidDirectKeyAuthoritative(t *testing.T) {
	directPublic, directPrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate direct key: %v", err)
	}
	filePublic, filePrivate, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate file key: %v", err)
	}
	fileText, err := filePublic.MarshalText()
	if err != nil {
		t.Fatalf("marshal file public key: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyFile, fileText, 0o600); err != nil {
		t.Fatalf("write file public key: %v", err)
	}
	verifier, err := LoadSignedManifestVerifier(directPublic.String(), keyFile)
	if err != nil {
		t.Fatalf("load direct public key: %v", err)
	}
	payload := []byte("authoritative direct key")
	directSignature := base64.StdEncoding.EncodeToString(minisign.Sign(directPrivate, payload))
	if err := verifier.Verify(payload, directSignature); err != nil {
		t.Fatalf("verify direct signature: %v", err)
	}
	fileSignature := base64.StdEncoding.EncodeToString(minisign.Sign(filePrivate, payload))
	if err := verifier.Verify(payload, fileSignature); err == nil {
		t.Fatal("signature failure must not trigger a switch to the file public key")
	}
}

func TestLoadSignedManifestVerifierRejectsTwoInvalidSources(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyFile, []byte("also-invalid"), 0o600); err != nil {
		t.Fatalf("write invalid public key file: %v", err)
	}
	if _, err := LoadSignedManifestVerifier("invalid", keyFile); err == nil {
		t.Fatal("two invalid public key sources must be rejected")
	}
}

func TestDecodeSignedClientPayloadUsesFullMinisignVerification(t *testing.T) {
	payload := ClientUpdatePayload{
		ProtocolVersion: 2, Version: "1.0.1", Target: clientTargetWindowsX64, LayoutVersion: 1,
		Full: ClientFullArtifacts{
			NSIS:     artifactForTest("nsis", "https://example.test/client.exe", []byte("nsis")),
			Portable: artifactForTest("portable", "https://example.test/client-portable.exe", []byte("portable")),
		},
	}
	envelope, verifier := signedEnvelopeForTest(t, payload)
	decoded, err := DecodeSignedClientPayload(envelope, verifier)
	if err != nil || decoded.Version != payload.Version || decoded.LayoutVersion != 1 {
		t.Fatalf("decode signed payload = %#v, %v", decoded, err)
	}

	tamperedPayload := *envelope
	tamperedPayload.Payload = base64.StdEncoding.EncodeToString([]byte(`{"protocol_version":2}`))
	if _, err := DecodeSignedClientPayload(&tamperedPayload, verifier); err == nil {
		t.Fatal("tampered payload must be rejected")
	}
	tamperedSignature := *envelope
	tamperedSignature.Signature += "\ntrusted comment: altered"
	if _, err := DecodeSignedClientPayload(&tamperedSignature, verifier); err == nil {
		t.Fatal("tampered trusted comment must be rejected")
	}
}

func TestServicePlansDeltaCachesArtifactsAndServesRange(t *testing.T) {
	nsis := []byte("nsis-full-artifact")
	portable := []byte("portable-full-artifact")
	patch := []byte("zstd-patch")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/nsis":
			_, _ = w.Write(nsis)
		case "/portable":
			_, _ = w.Write(portable)
		case "/patch":
			_, _ = w.Write(patch)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	nsisArtifact := artifactForTest("nsis", server.URL+"/nsis", nsis)
	portableArtifact := artifactForTest("portable", server.URL+"/portable", portable)
	deltaArtifact := artifactForTest("delta", server.URL+"/patch", patch)
	deltaArtifact.Algorithm = "zstd-patch-from-v1"
	payload := ClientUpdatePayload{
		ProtocolVersion: 2, Version: "1.0.1", Target: clientTargetWindowsX64, LayoutVersion: 1,
		Full: ClientFullArtifacts{NSIS: nsisArtifact, Portable: portableArtifact},
		Deltas: []ClientDeltaArtifact{{
			ClientArtifact: deltaArtifact, FromVersion: "1.0.0", FromSHA256: stringsOf('a', 64), TargetSHA256: stringsOf('b', 64),
		}},
	}
	envelope, verifier := signedEnvelopeForTest(t, payload)
	artifactStore := &LocalArtifactStore{Root: t.TempDir(), Client: &http.Client{Timeout: time.Second}}
	service := NewServiceWithAllDependencies(config.UpdateConfig{
		Enabled: true, CacheDir: t.TempDir(), ClientVersion: "1.0.0", CheckInterval: time.Hour, DownloadTimeout: time.Second,
	}, "1.0.0", staticManifestSource{manifest: &Manifest{Version: "1.0.1", ClientUpdateV2: envelope}}, noPackageStore{},
		artifactStore, verifier, nil)
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("check v2 update: %v", err)
	}

	plan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", CurrentSHA256: stringsOf('a', 64), Target: clientTargetWindowsX64, InstallMode: "portable",
	})
	if err != nil || !available || plan.Strategy != "delta" || plan.SavedBytes != int64(len(portable)-len(patch)) {
		t.Fatalf("unexpected delta plan: %#v, available=%v, err=%v", plan, available, err)
	}
	if plan.Artifact.DownloadPath != "/api/v1/updates/client/artifacts/"+deltaArtifact.SHA256 {
		t.Fatalf("unsafe artifact download path: %s", plan.Artifact.DownloadPath)
	}
	nsisPlan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", CurrentSHA256: stringsOf('a', 64), Target: clientTargetWindowsX64, InstallMode: "nsis",
	})
	if err != nil || !available || nsisPlan.Strategy != "delta" || nsisPlan.FullFallback.Kind != "nsis" {
		t.Fatalf("unexpected NSIS delta plan: %#v available=%v err=%v", nsisPlan, available, err)
	}
	fullPlan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", CurrentSHA256: stringsOf('c', 64), Target: clientTargetWindowsX64, InstallMode: "portable",
	})
	if err != nil || !available || fullPlan.Strategy != "full" {
		t.Fatalf("unexpected full fallback plan: %#v available=%v err=%v", fullPlan, available, err)
	}

	e := echo.New()
	NewHandlerWithService(&config.Config{}, service).RegisterPublicRoutes(e.Group("/api/v1"))
	var verificationScans atomic.Int32
	artifactStore.verifyFile = func(path, digest string) error {
		verificationScans.Add(1)
		return verifySHA256(path, digest)
	}
	req := httptest.NewRequest(http.MethodGet, plan.Artifact.DownloadPath, nil)
	req.Header.Set("Range", "bytes=0-2")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != string(patch[:3]) || rec.Header().Get("ETag") != `"`+deltaArtifact.SHA256+`"` {
		t.Fatalf("range response code=%d body=%q headers=%v", rec.Code, rec.Body.String(), rec.Header())
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, plan.Artifact.DownloadPath, nil))
	if rec.Code != http.StatusOK || verificationScans.Load() != 0 {
		t.Fatalf("repeated artifact ranges must use verified snapshot: code=%d scans=%d", rec.Code, verificationScans.Load())
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates/client/artifacts/../../etc/passwd", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("path traversal code=%d", rec.Code)
	}

	tauri, available, err := service.TauriClientUpdate(clientTargetWindowsX64, "1.0.0")
	if err != nil || !available || tauri.URL != "/api/v1/updates/client/artifacts/"+nsisArtifact.SHA256 || tauri.Signature != nsisArtifact.Signature {
		t.Fatalf("unexpected tauri update: %#v available=%v err=%v", tauri, available, err)
	}
	rec = httptest.NewRecorder()
	tauriRequest := httptest.NewRequest(http.MethodGet, "/api/v1/updates/client/tauri/windows/x86_64/1.0.0", nil)
	tauriRequest.Host = "erp.lan:8080"
	e.ServeHTTP(rec, tauriRequest)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"url":"http://erp.lan:8080/api/v1/updates/client/artifacts/`) {
		t.Fatalf("tauri handler must return an absolute artifact URL: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOptionalDeltaCacheFailureFallsBackToFull(t *testing.T) {
	nsis, portable := []byte("nsis"), []byte("portable")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/nsis":
			_, _ = w.Write(nsis)
		case "/portable":
			_, _ = w.Write(portable)
		default:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	delta := artifactForTest("delta", server.URL+"/missing-delta", []byte("missing"))
	delta.Algorithm = "zstd-patch-from-v1"
	payload := ClientUpdatePayload{
		ProtocolVersion: 2, Version: "1.0.1", Target: clientTargetWindowsX64, LayoutVersion: 1,
		Full:   ClientFullArtifacts{NSIS: artifactForTest("nsis", server.URL+"/nsis", nsis), Portable: artifactForTest("portable", server.URL+"/portable", portable)},
		Deltas: []ClientDeltaArtifact{{ClientArtifact: delta, FromVersion: "1.0.0", FromSHA256: stringsOf('a', 64), TargetSHA256: stringsOf('b', 64)}},
	}
	envelope, verifier := signedEnvelopeForTest(t, payload)
	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: t.TempDir(), ClientVersion: "1.0.0", CheckInterval: time.Hour}, "1.0.0",
		staticManifestSource{manifest: &Manifest{Version: "1.0.1", ClientUpdateV2: envelope}}, noPackageStore{}, &LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}, verifier, nil)
	status, err := service.Check(context.Background())
	if err != nil || status.ClientDeltaDegraded == "" {
		t.Fatalf("optional delta failure must not fail full update: status=%#v err=%v", status, err)
	}
	plan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{CurrentVersion: "1.0.0", CurrentSHA256: stringsOf('a', 64), Target: clientTargetWindowsX64, InstallMode: "portable"})
	if err != nil || !available || plan.Strategy != "full" || plan.Message == "" {
		t.Fatalf("delta failure fallback = %#v available=%v err=%v", plan, available, err)
	}
}

func TestV2ManifestWithoutConfiguredPublicKeyDoesNotReplaceV1State(t *testing.T) {
	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: t.TempDir(), CheckInterval: time.Hour}, "1.0.0",
		staticManifestSource{manifest: &Manifest{
			Version: "1.0.1",
			ClientUpdateV2: &SignedClientUpdateManifest{
				Payload: base64.StdEncoding.EncodeToString([]byte(`{"protocol_version":2}`)), Signature: "not-a-signature",
			},
		}}, noPackageStore{}, &LocalArtifactStore{Root: t.TempDir(), Client: &http.Client{Timeout: time.Second}}, nil, nil)
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("v2 manifest without a public key must be rejected")
	}
	if status := service.Status(""); status.Manifest != nil || status.LastSuccessAt != nil || status.LastError == "" {
		t.Fatalf("failed v2 check must not publish an unverified state: %#v", status)
	}
}

func TestV2FailureDoesNotReplaceLegacyClientZIP(t *testing.T) {
	oldArchive := zipBytes(t)
	newArchive := append(append([]byte(nil), oldArchive...), []byte("new-release")...)
	newDigest := sha256.Sum256(newArchive)
	var zipRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/client.zip" {
			zipRequests.Add(1)
			_, _ = w.Write(newArchive)
			return
		}
		http.NotFound(w, request)
	}))
	defer server.Close()

	cacheRoot := t.TempDir()
	store := &LocalPackageStore{Root: cacheRoot, Client: server.Client()}
	oldPath := store.Path(clientPackageName)
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatalf("create legacy cache directory: %v", err)
	}
	if err := os.WriteFile(oldPath, oldArchive, 0o600); err != nil {
		t.Fatalf("write old legacy zip: %v", err)
	}

	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: cacheRoot, CheckInterval: time.Hour}, "1.0.0",
		staticManifestSource{manifest: &Manifest{
			Version: "1.0.1",
			Client:  PackageManifest{Version: "1.0.1", URL: server.URL + "/client.zip", SHA256: hex.EncodeToString(newDigest[:]), Size: int64(len(newArchive))},
			ClientUpdateV2: &SignedClientUpdateManifest{
				Payload: base64.StdEncoding.EncodeToString([]byte(`{"protocol_version":2}`)), Signature: "not-a-signature",
			},
		}}, store, &LocalArtifactStore{Root: cacheRoot, Client: server.Client()}, nil, nil)
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("invalid v2 update must fail")
	}
	after, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read legacy zip after failed update: %v", err)
	}
	if !bytes.Equal(after, oldArchive) || zipRequests.Load() != 0 {
		t.Fatalf("failed v2 update changed legacy ZIP=%v requests=%d", !bytes.Equal(after, oldArchive), zipRequests.Load())
	}
}

func TestLocalArtifactStoreCoalescesConcurrentDownloads(t *testing.T) {
	body := []byte("concurrent artifact")
	artifact := artifactForTest("portable", "", body)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	artifact.URL = server.URL
	store := &LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, _, err := store.Ensure(context.Background(), artifact); err != nil {
				t.Errorf("ensure artifact: %v", err)
			}
		}()
	}
	wait.Wait()
	if requests.Load() != 1 {
		t.Fatalf("requests=%d, want one", requests.Load())
	}
}

func TestLocalArtifactStoreVerifiedSnapshotAvoidsRescanAndInvalidatesChange(t *testing.T) {
	body := []byte("verified artifact bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()
	artifact := artifactForTest("portable", server.URL, body)
	var scans atomic.Int32
	store := &LocalArtifactStore{Root: t.TempDir(), Client: server.Client(), verifyFile: func(path, digest string) error {
		scans.Add(1)
		return verifySHA256(path, digest)
	}}
	path, _, err := store.Ensure(context.Background(), artifact)
	if err != nil {
		t.Fatalf("initial artifact ensure: %v", err)
	}
	for range 3 {
		if !store.Cached(artifact) {
			t.Fatal("verified artifact should remain cached")
		}
	}
	if scans.Load() != 0 {
		t.Fatalf("verified snapshot performed %d redundant hash scans", scans.Load())
	}
	if err := os.WriteFile(path, []byte("changed artifact bytes!"), 0o600); err != nil {
		t.Fatalf("modify cached artifact: %v", err)
	}
	changedAt := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, changedAt, changedAt); err != nil {
		t.Fatalf("change cached artifact timestamp: %v", err)
	}
	if store.Cached(artifact) {
		t.Fatal("changed artifact must be invalidated")
	}
	if scans.Load() != 0 {
		t.Fatalf("changed artifact must invalidate by identity instead of rescan: scans=%d", scans.Load())
	}
}

func TestManifestDownloadsEnforceDeclaredHardSizeLimit(t *testing.T) {
	body := []byte("response exceeds declared manifest size")
	digest := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	artifactStore := &LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}
	artifact := ClientArtifact{Kind: "portable", URL: server.URL, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body) - 1), Signature: "signature"}
	if _, _, err := artifactStore.Ensure(context.Background(), artifact); err == nil {
		t.Fatal("oversized artifact response must be rejected")
	}
	if _, ok := artifactStore.Path(artifact.SHA256); ok {
		t.Fatal("oversized artifact must not enter cache")
	}

	packageStore := &LocalPackageStore{Root: t.TempDir(), Client: server.Client()}
	pkg := PackageManifest{URL: server.URL, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body) - 1)}
	if _, _, err := packageStore.Ensure(context.Background(), clientPackageName, pkg); err == nil {
		t.Fatal("oversized legacy ZIP response must be rejected")
	}
	if _, err := os.Stat(packageStore.Path(clientPackageName)); !os.IsNotExist(err) {
		t.Fatalf("oversized legacy ZIP must not enter cache: %v", err)
	}
}

func stringsOf(value rune, count int) string {
	return string(makeRunes(value, count))
}

func makeRunes(value rune, count int) []rune {
	result := make([]rune, count)
	for index := range result {
		result[index] = value
	}
	return result
}
