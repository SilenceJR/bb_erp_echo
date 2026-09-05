package update

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb_erp_echo/internal/config"
)

func TestDirectoryManifestSourceLoadsManifestWithoutHTTP(t *testing.T) {
	releaseDir := t.TempDir()
	archive := zipBytes(t)
	resourcePath := filepath.Join(releaseDir, "packages", "server.zip")
	if err := os.MkdirAll(filepath.Dir(resourcePath), 0o755); err != nil {
		t.Fatalf("create release package directory: %v", err)
	}
	if err := os.WriteFile(resourcePath, archive, 0o600); err != nil {
		t.Fatalf("write release package: %v", err)
	}
	digest := sha256.Sum256(archive)
	manifest := Manifest{Version: "2.0.0", Server: PackageManifest{
		Version: "2.0.0", URL: "packages/server.zip", Size: int64(len(archive)), SHA256: hex.EncodeToString(digest[:]),
	}}
	writeDirectoryManifest(t, releaseDir, manifest)

	source := NewDirectoryManifestSource(releaseDir)
	loaded, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch local manifest: %v", err)
	}
	if loaded.Version != "2.0.0" || loaded.Server.URL != "packages/server.zip" {
		t.Fatalf("unexpected local manifest: %+v", loaded)
	}
	if !strings.HasPrefix(source.Location(), "directory://") {
		t.Fatalf("unexpected directory source location: %q", source.Location())
	}
}

func TestDirectoryManifestSourceRequiresMatchingActivationMarker(t *testing.T) {
	releaseDir := t.TempDir()
	manifest := Manifest{Version: "2.0.0", Server: PackageManifest{Version: "2.0.0"}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "update-manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "not activated") {
		t.Fatalf("missing activation marker error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, ".release-ready"), []byte("1.9.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched activation marker error=%v", err)
	}
}

func TestDirectoryManifestSourceRejectsUnsafeResourcePaths(t *testing.T) {
	releaseDir := t.TempDir()
	archive := zipBytes(t)
	if err := os.WriteFile(filepath.Join(releaseDir, "server.zip"), archive, 0o600); err != nil {
		t.Fatalf("write release package: %v", err)
	}
	digest := sha256.Sum256(archive)
	unsafe := []string{
		"/server.zip",
		"../server.zip",
		"packages/../../server.zip",
		"https://example.invalid/server.zip",
		"file:///server.zip",
		"C:/server.zip",
		"C%3A/server.zip",
		"server.zip%3Aalternate",
		`packages\\server.zip`,
		"%2e%2e/server.zip",
	}
	for _, resource := range unsafe {
		t.Run(resource, func(t *testing.T) {
			writeDirectoryManifest(t, releaseDir, Manifest{Version: "2.0.0", Server: PackageManifest{
				Version: "2.0.0", URL: resource, Size: int64(len(archive)), SHA256: hex.EncodeToString(digest[:]),
			}})
			if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err == nil {
				t.Fatalf("unsafe resource path %q was accepted", resource)
			}
		})
	}
}

func TestDirectoryManifestSourceRejectsResourceSymlink(t *testing.T) {
	releaseDir := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "server.zip")
	if err := os.WriteFile(outside, zipBytes(t), 0o600); err != nil {
		t.Fatalf("write outside package: %v", err)
	}
	link := filepath.Join(releaseDir, "server.zip")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	contents, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside package: %v", err)
	}
	digest := sha256.Sum256(contents)
	writeDirectoryManifest(t, releaseDir, Manifest{Version: "2.0.0", Server: PackageManifest{
		Version: "2.0.0", URL: "server.zip", Size: int64(len(contents)), SHA256: hex.EncodeToString(digest[:]),
	}})
	if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("resource symlink was not rejected: %v", err)
	}
}

func TestDirectoryManifestSourceValidatesSignedPayloadResourcePaths(t *testing.T) {
	releaseDir := t.TempDir()
	for _, name := range []string{"client/nsis.exe", "client/portable.exe"} {
		path := filepath.Join(releaseDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create client resource directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatalf("write client resource: %v", err)
		}
	}
	payload, err := json.Marshal(map[string]any{
		"full": map[string]any{
			"nsis":     map[string]any{"url": "client/nsis.exe"},
			"portable": map[string]any{"url": "client/portable.exe"},
		},
	})
	if err != nil {
		t.Fatalf("marshal signed payload: %v", err)
	}
	writeDirectoryManifest(t, releaseDir, Manifest{
		Version:        "2.0.0",
		ClientUpdateV2: &SignedClientUpdateManifest{Payload: base64.StdEncoding.EncodeToString(payload), Signature: "not-checked-by-source"},
	})
	if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err != nil {
		t.Fatalf("safe signed payload resources were rejected: %v", err)
	}

	payload, err = json.Marshal(map[string]any{
		"full": map[string]any{
			"nsis":     map[string]any{"url": "../outside.exe"},
			"portable": map[string]any{"url": "client/portable.exe"},
		},
	})
	if err != nil {
		t.Fatalf("marshal unsafe payload: %v", err)
	}
	writeDirectoryManifest(t, releaseDir, Manifest{
		Version:        "2.0.0",
		ClientUpdateV2: &SignedClientUpdateManifest{Payload: base64.StdEncoding.EncodeToString(payload), Signature: "not-checked-by-source"},
	})
	if _, err := NewDirectoryManifestSource(releaseDir).Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "parent traversal") {
		t.Fatalf("unsafe signed payload resource was not rejected: %v", err)
	}
}

func TestDirectoryServiceCopiesAndVerifiesServerPackageLocally(t *testing.T) {
	releaseDir := t.TempDir()
	cacheDir := t.TempDir()
	archive := serverPackageZipBytes(t)
	serverPath := filepath.Join(releaseDir, "server.zip")
	if err := os.WriteFile(serverPath, archive, 0o600); err != nil {
		t.Fatalf("write release server package: %v", err)
	}
	digest := sha256.Sum256(archive)
	signature, verifier := signedPackageForTest(t, archive)
	writeDirectoryManifest(t, releaseDir, Manifest{Version: "2.0.0", Server: PackageManifest{
		Version: "2.0.0", URL: "server.zip", Size: int64(len(archive)), SHA256: hex.EncodeToString(digest[:]), Signature: signature,
	}})

	cfg := config.UpdateConfig{
		Enabled: true, Source: "directory", ReleaseDir: releaseDir, CacheDir: cacheDir,
		ClientVersion: "1.0.0", CheckInterval: time.Hour,
	}
	service := NewServiceWithAllDependencies(cfg, "1.0.0", NewDirectoryManifestSource(releaseDir),
		&DirectoryPackageStore{Root: cacheDir, ReleaseDir: releaseDir},
		&DirectoryArtifactStore{Root: cacheDir, ReleaseDir: releaseDir}, verifier, nil)
	if _, err := service.Check(context.Background()); err != nil {
		t.Fatalf("check local update source: %v", err)
	}
	path, name, err := service.ServerPackage(context.Background())
	if err != nil {
		t.Fatalf("copy and verify local server package: %v", err)
	}
	if name != "server.zip" || filepath.Clean(path) != filepath.Join(cacheDir, "server", hex.EncodeToString(digest[:])[:16]+"-server.zip") {
		t.Fatalf("unexpected cached local package path=%q name=%q", path, name)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cached local package: %v", err)
	}
	if string(got) != string(archive) {
		t.Fatal("cached local package differs from release resource")
	}
}

func TestDirectoryArtifactStoreDoesNotUseHTTPClient(t *testing.T) {
	releaseDir := t.TempDir()
	cacheDir := t.TempDir()
	contents := []byte("client artifact")
	resourcePath := filepath.Join(releaseDir, "client.exe")
	if err := os.WriteFile(resourcePath, contents, 0o600); err != nil {
		t.Fatalf("write client artifact: %v", err)
	}
	digest := sha256.Sum256(contents)
	store := &DirectoryArtifactStore{
		Root: cacheDir, ReleaseDir: releaseDir,
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("HTTP must not be used by directory artifact store")
		})},
	}
	artifact := ClientArtifact{Kind: "portable", URL: "client.exe", SHA256: hex.EncodeToString(digest[:]), Size: int64(len(contents)), Signature: "signature"}
	path, reused, err := store.Ensure(context.Background(), artifact)
	if err != nil {
		t.Fatalf("copy local client artifact: %v", err)
	}
	if reused {
		t.Fatal("first local artifact copy was reported as reused")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("cached local artifact=%q err=%v", got, err)
	}
}

func TestNewServiceWithDependenciesUsesDirectoryArtifactStore(t *testing.T) {
	releaseDir := t.TempDir()
	cfg := config.UpdateConfig{
		Enabled: true, Source: config.UpdateSourceDirectory, ReleaseDir: releaseDir,
		CacheDir: t.TempDir(), DownloadTimeout: time.Minute,
	}
	service := NewServiceWithDependencies(cfg, "1.0.0", NewDirectoryManifestSource(releaseDir), &DirectoryPackageStore{
		Root: cfg.CacheDir, ReleaseDir: releaseDir,
	})
	artifactStore, ok := service.artifactStore.(*DirectoryArtifactStore)
	if !ok {
		t.Fatalf("directory service dependencies selected %T instead of DirectoryArtifactStore", service.artifactStore)
	}
	if artifactStore.ReleaseDir != releaseDir {
		t.Fatalf("directory artifact store release dir=%q, want %q", artifactStore.ReleaseDir, releaseDir)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func writeDirectoryManifest(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal directory manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "update-manifest.json"), raw, 0o600); err != nil {
		t.Fatalf("write directory manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".release-ready"), []byte(manifest.Version), 0o600); err != nil {
		t.Fatalf("write directory activation marker: %v", err)
	}
}
