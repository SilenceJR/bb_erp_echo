package update

import (
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
		Kind: kind, URL: url, SHA256: hex.EncodeToString(digest[:]),
		Size: int64(len(body)), Signature: "artifact-signature",
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
		Payload:   base64.StdEncoding.EncodeToString(raw),
		Signature: base64.StdEncoding.EncodeToString(minisign.Sign(private, raw)),
	}, verifier
}

func fullPayload(nsis, portable ClientArtifact) ClientUpdatePayload {
	return ClientUpdatePayload{
		ProtocolVersion: 2,
		Version:         "1.0.1",
		Target:          clientTargetWindowsX64,
		LayoutVersion:   1,
		Full:            ClientFullArtifacts{NSIS: nsis, Portable: portable},
	}
}

func TestMinisignVerifierAcceptsTauriPublicKeyEnvelope(t *testing.T) {
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicText, err := public.MarshalText()
	if err != nil {
		t.Fatal(err)
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

func TestLoadSignedManifestVerifierUsesConfiguredFileWhenDirectKeyIsInvalid(t *testing.T) {
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicText, err := public.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "update-public.key")
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(publicText)), 0o600); err != nil {
		t.Fatal(err)
	}
	verifier, err := LoadSignedManifestVerifier("invalid", keyFile)
	if err != nil {
		t.Fatalf("load configured public key file: %v", err)
	}
	payload := []byte("signed update payload")
	signature := base64.StdEncoding.EncodeToString(minisign.Sign(private, payload))
	if err := verifier.Verify(payload, signature); err != nil {
		t.Fatalf("verify with configured public key file: %v", err)
	}
}

func TestDecodeSignedClientPayloadRejectsTamperingAndDeltaFields(t *testing.T) {
	payload := fullPayload(
		artifactForTest("nsis", "http://192.168.1.2/nsis.exe", []byte("nsis")),
		artifactForTest("portable", "http://192.168.1.2/client.exe", []byte("portable")),
	)
	envelope, verifier := signedEnvelopeForTest(t, payload)
	decoded, err := DecodeSignedClientPayload(envelope, verifier)
	if err != nil || decoded.Version != payload.Version {
		t.Fatalf("decode signed payload = %#v, %v", decoded, err)
	}

	tampered := *envelope
	tampered.Payload = base64.StdEncoding.EncodeToString([]byte(`{"protocol_version":2}`))
	if _, err := DecodeSignedClientPayload(&tampered, verifier); err == nil {
		t.Fatal("tampered payload must be rejected")
	}

	raw, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["deltas"] = []any{}
	raw, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var strict ClientUpdatePayload
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&strict); err == nil {
		t.Fatal("full-only payload must reject deltas")
	}
}

func TestDecodeSignedClientPayloadRejectsNestedDuplicateKeys(t *testing.T) {
	raw := []byte(`{"protocol_version":2,"version":"1.0.1","target":"windows-x86_64","layout_version":1,"full":{"nsis":{"kind":"nsis","url":"http://192.168.1.2/nsis.exe","size":4,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature":"sig","kind":"nsis"},"portable":{"kind":"portable","url":"http://192.168.1.2/client.exe","size":8,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","signature":"sig"}}}`)
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewMinisignVerifier(public.String())
	if err != nil {
		t.Fatal(err)
	}
	envelope := &SignedClientUpdateManifest{
		Payload:   base64.StdEncoding.EncodeToString(raw),
		Signature: base64.StdEncoding.EncodeToString(minisign.Sign(private, raw)),
	}
	if _, err := DecodeSignedClientPayload(envelope, verifier); err == nil || !strings.Contains(err.Error(), "duplicate JSON") {
		t.Fatalf("nested duplicate key must be rejected, got %v", err)
	}
}

func TestServiceCachesOnlyFullArtifactsAndServesSameOriginReferences(t *testing.T) {
	nsis := []byte("nsis-full-artifact")
	portable := []byte("portable-full-artifact")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/nsis":
			_, _ = w.Write(nsis)
		case "/portable":
			_, _ = w.Write(portable)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	nsisArtifact := artifactForTest("nsis", server.URL+"/nsis", nsis)
	portableArtifact := artifactForTest("portable", server.URL+"/portable", portable)
	envelope, verifier := signedEnvelopeForTest(t, fullPayload(nsisArtifact, portableArtifact))
	artifactStore := &LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}
	service := NewServiceWithAllDependencies(config.UpdateConfig{
		Enabled: true, CacheDir: t.TempDir(), ClientVersion: "1.0.0",
		CheckInterval: time.Hour, DownloadTimeout: time.Second,
	}, "1.0.0", staticManifestSource{manifest: &Manifest{Version: "1.0.1", ClientUpdateV2: envelope}},
		noPackageStore{}, artifactStore, verifier, nil)
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check full-only update: %v", err)
	}
	if !status.ClientFullCached || status.ClientCacheBytes != int64(len(nsis)+len(portable)) {
		t.Fatalf("unexpected full cache status: %#v", status)
	}

	portablePlan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", Target: clientTargetWindowsX64, InstallMode: installModePortable,
	})
	if err != nil || !available || portablePlan.Strategy != "full" || portablePlan.Artifact.Kind != "portable" {
		t.Fatalf("portable full plan=%#v available=%v err=%v", portablePlan, available, err)
	}
	if portablePlan.Artifact.DownloadPath != "/api/v1/updates/client/artifacts/"+portableArtifact.SHA256 {
		t.Fatalf("unsafe download path: %s", portablePlan.Artifact.DownloadPath)
	}
	nsisPlan, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.0", Target: clientTargetWindowsX64, InstallMode: installModeNSIS,
	})
	if err != nil || !available || nsisPlan.Strategy != "full" || nsisPlan.Artifact.Kind != "nsis" {
		t.Fatalf("NSIS full plan=%#v available=%v err=%v", nsisPlan, available, err)
	}
	if _, available, err := service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: "1.0.1", Target: clientTargetWindowsX64, InstallMode: installModeNSIS,
	}); err != nil || available {
		t.Fatalf("same-version plan available=%v err=%v", available, err)
	}
	for name, request := range map[string]ClientUpdatePlanRequest{
		"missing current version":  {Target: clientTargetWindowsX64, InstallMode: installModeNSIS},
		"invalid current version":  {CurrentVersion: "current", Target: clientTargetWindowsX64, InstallMode: installModeNSIS},
		"missing target":           {CurrentVersion: "1.0.0", InstallMode: installModeNSIS},
		"missing install mode":     {CurrentVersion: "1.0.0", Target: clientTargetWindowsX64},
		"removed all-in-one alias": {CurrentVersion: "1.0.0", Target: clientTargetWindowsX64, InstallMode: "all-in-one"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := service.ClientUpdatePlan(request); err == nil {
				t.Fatal("strict full-only plan input must be rejected")
			}
		})
	}

	e := echo.New()
	NewHandlerWithService(&config.Config{}, service).RegisterPublicRoutes(e.Group("/api/v1"))
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, portablePlan.Artifact.DownloadPath, nil)
	request.Header.Set("Range", "bytes=0-2")
	e.ServeHTTP(record, request)
	if record.Code != http.StatusPartialContent || record.Body.String() != string(portable[:3]) {
		t.Fatalf("range code=%d body=%q", record.Code, record.Body.String())
	}

	tauri, available, err := service.TauriClientUpdate(clientTargetWindowsX64, "1.0.0")
	if err != nil || !available || tauri.URL != "/api/v1/updates/client/artifacts/"+nsisArtifact.SHA256 {
		t.Fatalf("tauri response=%#v available=%v err=%v", tauri, available, err)
	}
	record = httptest.NewRecorder()
	tauriRequest := httptest.NewRequest(http.MethodGet, "/api/v1/updates/client/tauri/windows/x86_64/1.0.0", nil)
	tauriRequest.Host = "192.168.1.10:8080"
	e.ServeHTTP(record, tauriRequest)
	if record.Code != http.StatusOK || !strings.Contains(record.Body.String(), `"url":"http://192.168.1.10:8080/api/v1/updates/client/artifacts/`) {
		t.Fatalf("tauri absolute LAN URL code=%d body=%s", record.Code, record.Body.String())
	}
	for _, removedPath := range []string{"/api/v1/updates/client/status", "/api/v1/updates/client/download"} {
		record = httptest.NewRecorder()
		e.ServeHTTP(record, httptest.NewRequest(http.MethodGet, removedPath, nil))
		if record.Code != http.StatusNotFound {
			t.Fatalf("removed legacy path %s status=%d", removedPath, record.Code)
		}
	}
}

func TestFullArtifactFailureDoesNotPublishPartialState(t *testing.T) {
	nsis := []byte("nsis")
	portable := []byte("portable")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/nsis" {
			_, _ = w.Write(nsis)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	payload := fullPayload(
		artifactForTest("nsis", server.URL+"/nsis", nsis),
		artifactForTest("portable", server.URL+"/portable", portable),
	)
	envelope, verifier := signedEnvelopeForTest(t, payload)
	service := NewServiceWithAllDependencies(
		config.UpdateConfig{Enabled: true, CacheDir: t.TempDir(), CheckInterval: time.Hour},
		"1.0.0", staticManifestSource{manifest: &Manifest{Version: "1.0.1", ClientUpdateV2: envelope}},
		noPackageStore{}, &LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}, verifier, nil,
	)
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("missing required portable full artifact must fail")
	}
	status := service.Status("")
	if status.Manifest != nil || status.LastSuccessAt != nil || status.LastError == "" {
		t.Fatalf("partial full-only state published: %#v", status)
	}
}

func TestLocalArtifactStoreCoalescesConcurrentDownloadsAndInvalidatesMutation(t *testing.T) {
	body := []byte("concurrent full artifact")
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
	path, ok := store.Path(artifact.SHA256)
	if !ok {
		t.Fatal("cached artifact missing")
	}
	if err := os.WriteFile(path, []byte("mutated-full-artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedAt := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	if store.Cached(artifact) {
		t.Fatal("mutated artifact must invalidate verified snapshot")
	}
}
