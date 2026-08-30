package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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

func zipBytes(t *testing.T) []byte {
	return zipBytesWithComment(t, "")
}

func zipBytesWithComment(t *testing.T, comment string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("README.txt")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := file.Write([]byte("bb erp update")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	writer.SetComment(comment)
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func serverPackageZipBytes(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{
		"bb-erp-server.exe", "bb-erp-updater.exe", "bb-erp-upgrade-runner.bat", "update-public.key", "version.json",
	} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create server package entry %s: %v", name, err)
		}
		contents := []byte("payload")
		if name == "version.json" {
			contents = []byte(`{"version":"2.0.0","server_version":"2.0.0"}`)
		}
		if _, err := file.Write(contents); err != nil {
			t.Fatalf("write server package entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close server package: %v", err)
	}
	return buffer.Bytes()
}

func signedPackageForTest(t *testing.T, payload []byte) (string, SignedManifestVerifier) {
	t.Helper()
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate package signing key: %v", err)
	}
	verifier, err := NewMinisignVerifier(public.String())
	if err != nil {
		t.Fatalf("create package verifier: %v", err)
	}
	reader := minisign.NewReader(bytes.NewReader(payload))
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("hash signed package: %v", err)
	}
	return base64.StdEncoding.EncodeToString(reader.Sign(private)), verifier
}

func TestServiceChecksRedirectCachesAndReusesPackage(t *testing.T) {
	archive := zipBytes(t)
	digest := sha256.Sum256(archive)
	var manifestRequests atomic.Int32
	var packageRequests atomic.Int32
	var serverPackageRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			http.Redirect(w, r, "/stable/update-manifest.json", http.StatusFound)
		case "/stable/update-manifest.json":
			manifestRequests.Add(1)
			_ = json.NewEncoder(w).Encode(Manifest{
				Version: "2.0.0",
				Server: PackageManifest{
					Version: "2.0.0", URL: server.URL + "/server.zip",
					SHA256: hex.EncodeToString(digest[:]), Size: int64(len(archive)),
				},
				Client: PackageManifest{
					Version: "2.0.0", URL: server.URL + "/client.zip",
					SHA256: hex.EncodeToString(digest[:]), Size: int64(len(archive)),
				},
			})
		case "/server.zip":
			serverPackageRequests.Add(1)
			_, _ = w.Write(archive)
		case "/client.zip":
			packageRequests.Add(1)
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.UpdateConfig{
		Enabled: true, ManifestURL: server.URL + "/manifest", CacheDir: t.TempDir(),
		ClientVersion: "1.0.0", CheckInterval: time.Hour,
		ManifestTimeout: time.Second, DownloadTimeout: time.Second,
	}
	service := NewService(cfg, "1.0.0")
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	if !status.Reachable || !status.Server.Available || !status.Client.Available || !status.Client.Cached {
		t.Fatalf("unexpected first status: %+v", status)
	}
	if status.Server.DownloadURL != serverDownloadPath || status.Server.DownloadPath != serverDownloadPath ||
		status.Server.FileName != "server.zip" || status.Client.DownloadURL != "/api/v1/system/updates/client/download" || status.CheckInterval != "1h" {
		t.Fatalf("unexpected local download/interval fields: %+v", status)
	}
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("second check: %v", err)
	}
	if manifestRequests.Load() != 2 || packageRequests.Load() != 1 || serverPackageRequests.Load() != 0 {
		t.Fatalf("requests manifest=%d client=%d server=%d", manifestRequests.Load(), packageRequests.Load(), serverPackageRequests.Load())
	}
	if service.ClientStatus("2.0.0").Available {
		t.Fatal("same installed client version should not report update")
	}
	if !service.ClientStatus("1.5.0").Available {
		t.Fatal("older installed client should report update")
	}
}

func TestDownloadServerPackageUsesReadPermissionAndAttachment(t *testing.T) {
	archive := serverPackageZipBytes(t)
	digest := sha256.Sum256(archive)
	signature, verifier := signedPackageForTest(t, archive)
	var upstreamRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamRequests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	cacheRoot := t.TempDir()
	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: cacheRoot}, "1.0.0",
		staticManifestSource{manifest: &Manifest{Server: PackageManifest{
			Version: "2.0.0", URL: server.URL + "/server.zip", SHA256: hex.EncodeToString(digest[:]), Size: int64(len(archive)), Signature: signature,
		}}},
		&LocalPackageStore{Root: cacheRoot, Client: server.Client()},
		&LocalArtifactStore{Root: cacheRoot, Client: server.Client()}, verifier, nil)
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("check server package manifest: %v", err)
	}

	var gotObject, gotAction string
	allowed := false
	require := func(object, action string) echo.MiddlewareFunc {
		gotObject, gotAction = object, action
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				if !allowed {
					return echo.NewHTTPError(http.StatusForbidden, "forbidden")
				}
				return next(c)
			}
		}
	}
	e := echo.New()
	NewHandlerWithService(&config.Config{}, service).RegisterSystemRoutes(e.Group("/api/v1/system"), require)

	record := httptest.NewRecorder()
	e.ServeHTTP(record, httptest.NewRequest(http.MethodGet, serverDownloadPath, nil))
	if record.Code != http.StatusForbidden {
		t.Fatalf("download without read permission status=%d body=%s", record.Code, record.Body.String())
	}
	if gotObject != "/api/v1/system/updates" || gotAction != "read" {
		t.Fatalf("download permission=%q/%q", gotObject, gotAction)
	}

	allowed = true
	recorders := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	start := make(chan struct{})
	var downloads sync.WaitGroup
	for _, concurrentRecord := range recorders {
		downloads.Add(1)
		go func(recorder *httptest.ResponseRecorder) {
			defer downloads.Done()
			<-start
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, serverDownloadPath, nil))
		}(concurrentRecord)
	}
	close(start)
	downloads.Wait()
	for _, concurrentRecord := range recorders {
		if concurrentRecord.Code != http.StatusOK || !bytes.Equal(concurrentRecord.Body.Bytes(), archive) {
			t.Fatalf("download response status=%d body-size=%d", concurrentRecord.Code, concurrentRecord.Body.Len())
		}
	}
	if got := recorders[0].Header().Get(echo.HeaderContentDisposition); !strings.Contains(got, "attachment") || !strings.Contains(got, "server.zip") {
		t.Fatalf("content disposition=%q", got)
	}
	if upstreamRequests.Load() != 1 {
		t.Fatalf("concurrent package requests were not coalesced: upstream=%d", upstreamRequests.Load())
	}
	record = httptest.NewRecorder()
	e.ServeHTTP(record, httptest.NewRequest(http.MethodGet, serverDownloadPath, nil))
	if record.Code != http.StatusOK || upstreamRequests.Load() != 1 {
		t.Fatalf("verified package cache was not reused: status=%d upstream=%d", record.Code, upstreamRequests.Load())
	}
}

func TestDownloadServerPackageRejectsInvalidZipWithoutCaching(t *testing.T) {
	body := []byte("not a zip archive")
	digest := sha256.Sum256(body)
	signature, verifier := signedPackageForTest(t, body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cacheRoot := t.TempDir()
	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: cacheRoot}, "1.0.0",
		staticManifestSource{manifest: &Manifest{Version: "2.0.0", Server: PackageManifest{
			Version: "2.0.0", URL: server.URL + "/server.zip", SHA256: hex.EncodeToString(digest[:]), Size: int64(len(body)), Signature: signature,
		}}},
		&LocalPackageStore{Root: cacheRoot, Client: server.Client()},
		&LocalArtifactStore{Root: cacheRoot, Client: server.Client()}, verifier, nil)
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("server package is downloaded only on demand: %v", err)
	}
	if status := service.Status(""); status.Manifest == nil || status.LastSuccessAt == nil || status.Server.DownloadPath != serverDownloadPath {
		t.Fatalf("valid manifest was not published before package download: %+v", status)
	}
	e := echo.New()
	NewHandlerWithService(&config.Config{}, service).RegisterSystemRoutes(e.Group("/api/v1/system"), func(_, _ string) echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	})
	record := httptest.NewRecorder()
	e.ServeHTTP(record, httptest.NewRequest(http.MethodGet, serverDownloadPath, nil))
	if record.Code != http.StatusBadGateway || !strings.Contains(record.Body.String(), "下载或校验服务端升级包失败") {
		t.Fatalf("invalid package response status=%d body=%s", record.Code, record.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "server", "server.zip")); !os.IsNotExist(err) {
		t.Fatalf("invalid server package entered cache: %v", err)
	}
}

func TestServicePreservesSuccessfulStateAfterPackageFailure(t *testing.T) {
	archive := zipBytes(t)
	digest := sha256.Sum256(archive)
	var broken atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/client.zip" {
			_, _ = w.Write(archive)
			return
		}
		version := "2.0.0"
		hash := hex.EncodeToString(digest[:])
		if broken.Load() {
			version = "3.0.0"
			hash = "bad-sha"
		}
		_ = json.NewEncoder(w).Encode(Manifest{
			Server: PackageManifest{Version: version},
			Client: PackageManifest{Version: version, URL: server.URL + "/client.zip", SHA256: hash, Size: int64(len(archive))},
		})
	}))
	defer server.Close()

	service := NewService(config.UpdateConfig{
		Enabled: true, ManifestURL: server.URL, CacheDir: filepath.Join(t.TempDir(), "cache"),
		ClientVersion: "1.0.0", CheckInterval: time.Hour,
		ManifestTimeout: time.Second, DownloadTimeout: time.Second,
	}, "1.0.0")
	first, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("successful check: %v", err)
	}
	broken.Store(true)
	failed, err := service.Check(context.Background())
	if err == nil {
		t.Fatal("bad hash should fail")
	}
	if failed.Server.LatestVersion != first.Server.LatestVersion || failed.LastSuccessAt == nil {
		t.Fatalf("last successful state was not preserved: %+v", failed)
	}
	if failed.LastError == "" || !failed.Reachable {
		t.Fatalf("failure details/connectivity missing: %+v", failed)
	}
}

func TestServiceCachesSameVersionClientForOlderInstalledClients(t *testing.T) {
	archive := zipBytes(t)
	digest := sha256.Sum256(archive)
	var packageRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/client.zip" {
			packageRequests.Add(1)
			_, _ = w.Write(archive)
			return
		}
		_ = json.NewEncoder(w).Encode(Manifest{
			Server: PackageManifest{Version: "1.2.3"},
			Client: PackageManifest{
				Version: "1.2.3", URL: server.URL + "/client.zip",
				SHA256: hex.EncodeToString(digest[:]), Size: int64(len(archive)),
			},
		})
	}))
	defer server.Close()
	service := NewService(config.UpdateConfig{
		Enabled: true, ManifestURL: server.URL, CacheDir: t.TempDir(),
		ClientVersion: "1.2.3", CheckInterval: time.Hour,
		ManifestTimeout: time.Second, DownloadTimeout: time.Second,
	}, "1.2.3")
	status, err := service.Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if status.Client.Available || !status.Client.Cached || packageRequests.Load() != 1 {
		t.Fatalf("same release client must be cached without reporting local update: %+v requests=%d", status.Client, packageRequests.Load())
	}
	oldClient := service.ClientStatus("1.2.2")
	if !oldClient.Available || !oldClient.Cached {
		t.Fatalf("older installed client cannot use cached package: %+v", oldClient)
	}
}

type countingSource struct {
	count atomic.Int32
	delay time.Duration
}

func (s *countingSource) Fetch(ctx context.Context) (*Manifest, error) {
	s.count.Add(1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
		return &Manifest{Server: PackageManifest{Version: "1.0.0"}}, nil
	}
}

func (s *countingSource) Location() string { return "memory://manifest" }

type noPackageStore struct{}

func (noPackageStore) Ensure(context.Context, string, PackageManifest) (string, bool, error) {
	return "", false, errors.New("unexpected package download")
}
func (noPackageStore) Cached(string, PackageManifest) bool { return false }
func (noPackageStore) Path(string) string                  { return "" }

func TestConcurrentChecksAreCoalesced(t *testing.T) {
	source := &countingSource{delay: 50 * time.Millisecond}
	service := NewServiceWithDependencies(config.UpdateConfig{
		Enabled: true, ClientVersion: "1.0.0", CheckInterval: time.Hour,
	}, "1.0.0", source, noPackageStore{})
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			if _, err := service.Check(context.Background()); err != nil {
				t.Errorf("check: %v", err)
			}
		}()
	}
	close(start)
	wait.Wait()
	if source.count.Load() != 1 {
		t.Fatalf("source fetch count = %d", source.count.Load())
	}
}

func TestStartChecksImmediatelyAndPeriodically(t *testing.T) {
	source := &countingSource{}
	service := NewServiceWithDependencies(config.UpdateConfig{
		Enabled: true, ClientVersion: "1.0.0", CheckInterval: 20 * time.Millisecond,
	}, "1.0.0", source, noPackageStore{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.Start(ctx)
	deadline := time.Now().Add(time.Second)
	for source.count.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if source.count.Load() < 2 {
		t.Fatalf("scheduled fetch count = %d", source.count.Load())
	}
}

func TestHTTPManifestSourceRejectsInvalidJSONAndTimesOut(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer server.Close()
		source := &HTTPManifestSource{URL: server.URL, Client: &http.Client{Timeout: time.Second}}
		if _, err := source.Fetch(context.Background()); err == nil {
			t.Fatal("invalid JSON should fail")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"server":{"version":"1.0.0"}}`))
		}))
		defer server.Close()
		source := &HTTPManifestSource{URL: server.URL, Client: &http.Client{Timeout: 10 * time.Millisecond}}
		if _, err := source.Fetch(context.Background()); err == nil {
			t.Fatal("timeout should fail")
		}
	})
}

func TestLocalPackageStoreRejectsInvalidZip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not a zip archive"))
	}))
	defer server.Close()
	store := &LocalPackageStore{Root: t.TempDir(), Client: &http.Client{Timeout: time.Second}}
	if _, _, err := store.Ensure(context.Background(), clientPackageName, PackageManifest{URL: server.URL}); err == nil {
		t.Fatal("invalid ZIP should not be cached")
	}
	if store.Cached(clientPackageName, PackageManifest{}) {
		t.Fatal("invalid ZIP must not be reported as cached")
	}
}

func TestLocalPackageStoreVerifiedSnapshotAvoidsRepeatedHashScans(t *testing.T) {
	archive := zipBytes(t)
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	var scans atomic.Int32
	store := &LocalPackageStore{Root: t.TempDir(), Client: server.Client(), verifyFile: func(path, want string) error {
		scans.Add(1)
		return verifySHA256(path, want)
	}}
	pkg := PackageManifest{URL: server.URL, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(archive))}
	service := NewServiceWithAllDependencies(config.UpdateConfig{Enabled: true, CacheDir: t.TempDir(), ClientVersion: "1.0.0", CheckInterval: time.Hour}, "1.0.0",
		staticManifestSource{manifest: &Manifest{Version: "1.0.1", Client: pkg}}, store,
		&LocalArtifactStore{Root: t.TempDir(), Client: server.Client()}, nil, nil)
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("check legacy package: %v", err)
	}
	for range 3 {
		if status := service.Status(""); !status.Client.Cached {
			t.Fatal("verified legacy package should remain cached during repeated status requests")
		}
	}
	if scans.Load() != 0 {
		t.Fatalf("legacy status path performed %d redundant SHA scans", scans.Load())
	}
}

func TestLocalPackageStoreSnapshotIncludesDeclaredDigest(t *testing.T) {
	first := zipBytesWithComment(t, "first")
	second := zipBytesWithComment(t, "other")
	if len(first) != len(second) {
		t.Fatalf("test archives must have identical sizes: %d != %d", len(first), len(second))
	}
	firstDigest := sha256.Sum256(first)
	secondDigest := sha256.Sum256(second)
	var serveSecond atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if serveSecond.Load() {
			_, _ = w.Write(second)
			return
		}
		_, _ = w.Write(first)
	}))
	defer server.Close()
	store := &LocalPackageStore{Root: t.TempDir(), Client: server.Client()}
	firstPkg := PackageManifest{URL: server.URL, SHA256: hex.EncodeToString(firstDigest[:]), Size: int64(len(first))}
	if _, _, err := store.Ensure(context.Background(), clientPackageName, firstPkg); err != nil {
		t.Fatalf("cache first package: %v", err)
	}
	serveSecond.Store(true)
	secondPkg := PackageManifest{URL: server.URL, SHA256: hex.EncodeToString(secondDigest[:]), Size: int64(len(second))}
	if store.Cached(clientPackageName, secondPkg) {
		t.Fatal("same-name, same-size package with a new digest must not hit old snapshot")
	}
	if _, reused, err := store.Ensure(context.Background(), clientPackageName, secondPkg); err != nil || reused {
		t.Fatalf("new digest must download replacement: reused=%v err=%v", reused, err)
	}
	if requests.Load() != 2 || !store.Cached(clientPackageName, secondPkg) {
		t.Fatalf("new digest cache result requests=%d cached=%v", requests.Load(), store.Cached(clientPackageName, secondPkg))
	}
}

func TestCompareVersionsUsesSemVerPrereleaseRules(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"v1.2.3", "1.2.2", 1},
		{"1.2.3-beta.2", "1.2.3-beta.1", 1},
		{"1.2.3-beta.1", "1.2.3", -1},
		{"1.2.3", "v1.2.3", 0},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.left, tc.right); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}
