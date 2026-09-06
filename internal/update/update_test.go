package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bb_erp_echo/internal/config"

	"github.com/labstack/echo/v5"
)

type testVerifier struct {
	verifyCount     atomic.Int32
	verifyFileCount atomic.Int32
	verifyErr       error
	fileErr         error
	started         chan struct{}
	release         chan struct{}
	once            sync.Once
}

func (v *testVerifier) Verify([]byte, string) error {
	v.verifyCount.Add(1)
	if v.started != nil {
		v.once.Do(func() { close(v.started) })
	}
	if v.release != nil {
		<-v.release
	}
	return v.verifyErr
}

func (v *testVerifier) VerifyFile(string, string) error {
	v.verifyFileCount.Add(1)
	return v.fileErr
}

func TestClientUpdatePlanRefreshesFixedDirectoryAndServesRange(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("portable client executable")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	payload := ClientUpdatePayload{
		Version: "2.0.0", Target: clientTargetWindowsX64,
		Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature},
	}
	writeSignedManifest(t, clientDir, payload)
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", &testVerifier{}, nil)
	plan, available, err := service.CheckClientUpdate(context.Background(), "1.0.0")
	if err != nil || !available {
		t.Fatalf("check update: available=%v err=%v", available, err)
	}
	if plan.ProtocolVersion != ClientUpdateProtocolVersion || plan.Target != clientTargetWindowsX64 || plan.Strategy != "full" || plan.DownloadSize != int64(len(exe)) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Artifact.DownloadPath != clientArtifactsPath+strings.ToLower(payload.Artifact.SHA256) || plan.Artifact.Kind != clientArtifactKindPortable {
		t.Fatalf("unexpected artifact plan: %+v", plan.Artifact)
	}
	cachePath, _, ok := service.ClientArtifact(payload.Artifact.SHA256)
	if !ok || cachePath == filepath.Join(clientDir, clientArtifactFileName) {
		t.Fatalf("artifact was not cached by content address: path=%q ok=%v", cachePath, ok)
	}

	e := echo.New()
	NewHandlerWithService(&config.Config{App: config.AppConfig{Name: "ERP", Version: "1.0.0"}}, service).RegisterPublicRoutes(e.Group("/api/v1"))
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/client-updates/artifacts/"+payload.Artifact.SHA256, nil)
	request.Header.Set("Range", "bytes=0-7")
	e.ServeHTTP(record, request)
	if record.Code != http.StatusPartialContent || record.Body.String() != string(exe[:8]) {
		t.Fatalf("range download code=%d body=%q", record.Code, record.Body.String())
	}
	if record.Header().Get("ETag") != `"`+payload.Artifact.SHA256+`"` {
		t.Fatalf("etag=%q", record.Header().Get("ETag"))
	}
}

func TestClientUpdateRejectsOlderVersionAndInvalidCurrentVersion(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("client")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "1.0.1", Target: clientTargetWindowsX64, Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}})
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", &testVerifier{}, nil)
	if _, available, err := service.CheckClientUpdate(context.Background(), "1.0.1"); err != nil || available {
		t.Fatalf("older/equal client should have no update: available=%v err=%v", available, err)
	}
	if _, available, err := service.CheckClientUpdate(context.Background(), "not-semver"); !errors.Is(err, ErrInvalidClientVersion) || available {
		t.Fatalf("invalid current version result: available=%v err=%v", available, err)
	}
}

func TestFailedRefreshKeepsPreviouslyVerifiedSnapshot(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("stable client")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "2.0.0", Target: clientTargetWindowsX64, Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}})
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", &testVerifier{}, nil)
	if _, available, err := service.CheckClientUpdate(context.Background(), "1.0.0"); err != nil || !available {
		t.Fatalf("initial update: available=%v err=%v", available, err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, clientManifestName), []byte(`{"payload":"bad","signature":"bad"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, available, err := service.CheckClientUpdate(context.Background(), "1.0.0")
	if err != nil || !available || plan.LatestVersion != "2.0.0" {
		t.Fatalf("invalid refresh did not retain old plan: plan=%+v available=%v err=%v", plan, available, err)
	}
	if _, _, ok := service.ClientArtifact(hex.EncodeToString(digest[:])); !ok {
		t.Fatal("failed refresh must retain old verified snapshot")
	}
}

func TestConcurrentChecksCoalesce(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("concurrent client")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "2.0.0", Target: clientTargetWindowsX64, Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}})
	verifier := &testVerifier{started: make(chan struct{}), release: make(chan struct{})}
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", verifier, nil)
	results := make(chan error, 2)
	go func() { _, _, err := service.CheckClientUpdate(context.Background(), "1.0.0"); results <- err }()
	select {
	case <-verifier.started:
	case <-time.After(time.Second):
		t.Fatal("first check did not start")
	}
	go func() { _, _, err := service.CheckClientUpdate(context.Background(), "1.0.0"); results <- err }()
	close(verifier.release)
	if first, second := <-results, <-results; first != nil || second != nil {
		t.Fatalf("coalesced checks errors: %v %v", first, second)
	}
	if count := verifier.verifyCount.Load(); count != 1 {
		t.Fatalf("manifest was verified %d times, want 1", count)
	}
}

func TestSequentialChecksReuseVerifiedStableSnapshot(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("stable sequential client")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "2.0.0", Target: clientTargetWindowsX64, Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}})
	verifier := &testVerifier{}
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", verifier, nil)
	for range 3 {
		if _, available, err := service.CheckClientUpdate(context.Background(), "1.0.0"); err != nil || !available {
			t.Fatalf("check update: available=%v err=%v", available, err)
		}
	}
	if got := verifier.verifyCount.Load(); got != 1 {
		t.Fatalf("stable manifest verified %d times, want 1", got)
	}
	before := verifier.verifyFileCount.Load()
	for range 3 {
		if _, _, ok := service.ClientArtifact(hex.EncodeToString(digest[:])); !ok {
			t.Fatal("committed artifact unavailable")
		}
	}
	if got := verifier.verifyFileCount.Load(); got != before {
		t.Fatalf("artifact downloads triggered %d extra signature checks", got-before)
	}
}

func TestCanceledLeaderDoesNotPoisonSharedRefresh(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := bytes.Repeat([]byte("portable"), 128*1024)
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "2.0.0", Target: clientTargetWindowsX64, Artifact: ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}})
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", &testVerifier{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := service.CheckClientUpdate(ctx, "1.0.0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled leader error=%v", err)
	}
	if _, available, err := service.CheckClientUpdate(context.Background(), "1.0.0"); err != nil || !available {
		t.Fatalf("subsequent check was poisoned: available=%v err=%v", available, err)
	}
}

func TestManifestChangedDuringVerificationIsNeverCommittedAsOldPayload(t *testing.T) {
	clientDir := t.TempDir()
	cacheDir := t.TempDir()
	exe := []byte("atomic publish client")
	digest := sha256.Sum256(exe)
	if err := os.WriteFile(filepath.Join(clientDir, clientArtifactFileName), exe, 0o644); err != nil {
		t.Fatal(err)
	}
	artifact := ClientArtifact{Kind: clientArtifactKindPortable, Size: int64(len(exe)), SHA256: hex.EncodeToString(digest[:]), Signature: testFileSignature}
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "2.0.0", Target: clientTargetWindowsX64, Artifact: artifact})
	verifier := &testVerifier{started: make(chan struct{}), release: make(chan struct{})}
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: clientDir, CacheDir: cacheDir}, "1.0.0", verifier, nil)
	result := make(chan error, 1)
	go func() { _, _, err := service.CheckClientUpdate(context.Background(), "1.0.0"); result <- err }()
	<-verifier.started
	writeSignedManifest(t, clientDir, ClientUpdatePayload{Version: "3.0.0", Target: clientTargetWindowsX64, Artifact: artifact})
	close(verifier.release)
	if err := <-result; !errors.Is(err, errClientUpdateSourceChanged) {
		t.Fatalf("mid-refresh replacement error=%v", err)
	}
	plan, available, err := service.CheckClientUpdate(context.Background(), "1.0.0")
	if err != nil || !available || plan.LatestVersion != "3.0.0" {
		t.Fatalf("next check did not publish M2: plan=%+v available=%v err=%v", plan, available, err)
	}
}

func TestMissingManifestReturnsNotPublished(t *testing.T) {
	service := NewServiceWithVerifier(config.UpdateConfig{ClientDir: t.TempDir(), CacheDir: t.TempDir()}, "1.0.0", &testVerifier{}, nil)
	if _, _, err := service.CheckClientUpdate(context.Background(), "1.0.0"); !errors.Is(err, ErrClientUpdateNotPublished) {
		t.Fatalf("missing manifest error=%v", err)
	}
}

func writeSignedManifest(t *testing.T, dir string, payload ClientUpdatePayload) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope := SignedClientUpdateManifest{Payload: base64.StdEncoding.EncodeToString(raw), Signature: testManifestSignature}
	contents, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, clientManifestName), contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	testManifestSignature = "bWFuaWZlc3Q="
	testFileSignature     = "ZmlsZQ=="
)
