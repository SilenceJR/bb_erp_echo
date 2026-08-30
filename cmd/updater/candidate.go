package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bb_erp_echo/internal/update"
)

// candidateRelease is deliberately kept outside the server ZIP. The server
// ZIP contains executable/runtime files; the candidate manifest is activated
// only after those files and all local client artifacts have passed validation.
type candidateRelease struct {
	Path        string
	Manifest    *update.Manifest
	UpdatesRoot string
	Payload     *update.ClientUpdatePayload
}

func loadCandidateRelease(manifestPath, installDir string) (*candidateRelease, error) {
	installDir, err := filepath.Abs(filepath.Clean(installDir))
	if err != nil {
		return nil, fmt.Errorf("resolve install directory: %w", err)
	}
	return loadCandidateReleaseWithPublicKey(manifestPath, installDir, filepath.Join(installDir, "update-public.key"))
}

func loadCandidateReleaseWithPublicKey(manifestPath, installDir, publicKeyPath string) (*candidateRelease, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return nil, errors.New("candidate manifest path is empty")
	}
	var err error
	installDir, err = filepath.Abs(filepath.Clean(installDir))
	if err != nil {
		return nil, fmt.Errorf("resolve install directory: %w", err)
	}
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(installDir, filepath.Clean(manifestPath))
	}
	manifest, err := update.ReadManifestFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read candidate manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" || update.CompareVersions(manifest.Version, "0.0.0") < 0 {
		return nil, errors.New("candidate manifest version is invalid")
	}
	if strings.TrimSpace(manifest.Server.Version) == "" || update.CompareVersions(manifest.Server.Version, manifest.Version) != 0 {
		return nil, errors.New("candidate server version does not match manifest version")
	}
	if manifest.Server.Size <= 0 || manifest.Server.Size > maxServerPackageSize || !validSHA256(manifest.Server.SHA256) || strings.TrimSpace(manifest.Server.Signature) == "" {
		return nil, fmt.Errorf("candidate server package must declare a size within 1..%d, SHA-256 and Minisign signature", maxServerPackageSize)
	}
	updatesRoot, err := filepath.Abs(filepath.Join(installDir, "updates"))
	if err != nil {
		return nil, fmt.Errorf("resolve updates root: %w", err)
	}
	verifier, err := update.LoadSignedManifestVerifier("", publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load installed trusted update public key: %w", err)
	}
	if verifier == nil {
		return nil, errors.New("installed trusted update public key is not configured")
	}
	if err := verifyCandidateClientRecoveryPackage(manifestPath, manifest, verifier); err != nil {
		return nil, err
	}
	payload, err := update.ValidateLocalClientArtifacts(manifest, updatesRoot, verifier)
	if err != nil {
		return nil, fmt.Errorf("validate candidate local client artifacts: %w", err)
	}
	return &candidateRelease{Path: manifestPath, Manifest: manifest, UpdatesRoot: updatesRoot, Payload: payload}, nil
}

func verifyCandidateClientRecoveryPackage(manifestPath string, manifest *update.Manifest, verifier update.SignedManifestVerifier) error {
	if manifest == nil || manifest.Client.Size <= 0 || !validSHA256(manifest.Client.SHA256) || strings.TrimSpace(manifest.Client.Signature) == "" {
		return errors.New("candidate client recovery ZIP must declare size, SHA-256 and Minisign signature")
	}
	path := filepath.Join(filepath.Dir(manifestPath), "bb-erp-client-windows.zip")
	if err := verifyFileSize(path, manifest.Client.Size); err != nil {
		return fmt.Errorf("verify candidate client recovery ZIP size: %w", err)
	}
	if err := verifySHA256(path, manifest.Client.SHA256); err != nil {
		return fmt.Errorf("verify candidate client recovery ZIP SHA-256: %w", err)
	}
	if err := verifier.VerifyFile(path, manifest.Client.Signature); err != nil {
		return fmt.Errorf("verify candidate client recovery ZIP Minisign signature: %w", err)
	}
	temporary, err := os.MkdirTemp("", "bb-erp-client-recovery-verify-*")
	if err != nil {
		return fmt.Errorf("create client recovery ZIP validation directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := extractZip(path, temporary); err != nil {
		return fmt.Errorf("validate candidate client recovery ZIP paths: %w", err)
	}
	for _, required := range []string{
		"client/bb_erp_client.exe", "client/bb-erp-portable.json", "installer/bb-erp-client-setup.exe",
	} {
		if !regularFileExists(filepath.Join(temporary, filepath.FromSlash(required))) {
			return fmt.Errorf("candidate client recovery ZIP is missing %s", required)
		}
	}
	return nil
}

func verifyCandidateServerPackage(packagePath string, manifest *update.Manifest, trustedPublicKeyPath string) error {
	if manifest == nil {
		return errors.New("candidate manifest is missing")
	}
	info, err := os.Stat(packagePath)
	if err != nil {
		return fmt.Errorf("inspect candidate server package: %w", err)
	}
	server := manifest.Server
	if info.IsDir() || info.Size() != server.Size {
		return fmt.Errorf("candidate server package size mismatch: got %d want %d", info.Size(), server.Size)
	}
	if err := verifySHA256(packagePath, server.SHA256); err != nil {
		return fmt.Errorf("verify candidate server package SHA-256: %w", err)
	}
	verifier, err := update.LoadSignedManifestVerifier("", trustedPublicKeyPath)
	if err != nil {
		return fmt.Errorf("load installed trusted update public key: %w", err)
	}
	if verifier == nil {
		return errors.New("installed trusted update public key is not configured")
	}
	if err := verifier.VerifyFile(packagePath, server.Signature); err != nil {
		return fmt.Errorf("verify candidate server package signature: %w", err)
	}
	return nil
}

func installStableManifest(candidatePath, installDir string) error {
	candidatePath = strings.TrimSpace(candidatePath)
	if candidatePath == "" {
		return errors.New("candidate manifest path is empty")
	}
	if _, err := update.ReadManifestFile(candidatePath); err != nil {
		return fmt.Errorf("revalidate candidate manifest before activation: %w", err)
	}
	stableDir := filepath.Join(installDir, "updates", "stable")
	if err := os.MkdirAll(stableDir, 0o755); err != nil {
		return fmt.Errorf("create stable manifest directory: %w", err)
	}
	target := filepath.Join(stableDir, "update-manifest.json")
	if err := replaceFileSafely(candidatePath, target); err != nil {
		return fmt.Errorf("activate stable manifest: %w", err)
	}
	return nil
}

// activateCandidateManifest re-reads and re-verifies the candidate immediately
// before activation. This closes the window between the initial candidate
// validation and the end of file replacement, while keeping the candidate
// manifest outside the server ZIP and preserving its independent release copy.
func activateCandidateManifest(candidate *candidateRelease, installDir string) error {
	if candidate == nil || candidate.Manifest == nil {
		return errors.New("candidate release is missing")
	}
	manifest, err := update.ReadManifestFile(candidate.Path)
	if err != nil {
		return fmt.Errorf("revalidate candidate manifest before activation: %w", err)
	}
	if !sameCandidateIdentity(candidate.Manifest, manifest) {
		return errors.New("candidate manifest changed after initial validation")
	}
	verifier, err := update.LoadSignedManifestVerifier("", filepath.Join(installDir, "update-public.key"))
	if err != nil {
		return fmt.Errorf("load update public key before manifest activation: %w", err)
	}
	if verifier == nil {
		return errors.New("installed trusted update public key is not configured")
	}
	updatesRoot, err := filepath.Abs(filepath.Join(installDir, "updates"))
	if err != nil {
		return fmt.Errorf("resolve updates root before manifest activation: %w", err)
	}
	if _, err := update.ValidateLocalClientArtifacts(manifest, updatesRoot, verifier); err != nil {
		return fmt.Errorf("revalidate local client artifacts before manifest activation: %w", err)
	}
	if err := verifyCandidateClientRecoveryPackage(candidate.Path, manifest, verifier); err != nil {
		return fmt.Errorf("revalidate client recovery ZIP before manifest activation: %w", err)
	}
	return installStableManifest(candidate.Path, installDir)
}

func sameCandidateIdentity(left, right *update.Manifest) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Version != right.Version || left.Server.Version != right.Server.Version ||
		left.Server.Size != right.Server.Size || !strings.EqualFold(left.Server.SHA256, right.Server.SHA256) ||
		left.Server.Signature != right.Server.Signature || left.Client.Version != right.Client.Version ||
		left.Client.Size != right.Client.Size || !strings.EqualFold(left.Client.SHA256, right.Client.SHA256) ||
		left.Client.Signature != right.Client.Signature {
		return false
	}
	leftClient := left.SignedClientUpdate()
	rightClient := right.SignedClientUpdate()
	if leftClient == nil || rightClient == nil {
		return leftClient == rightClient
	}
	return leftClient.Payload == rightClient.Payload && leftClient.Signature == rightClient.Signature
}

func waitForServerHealth(baseURL, expectedVersion, currentVersion string, output io.Writer) error {
	base, err := parseHealthBaseURL(baseURL)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	if currentVersion == "" {
		currentVersion = "0.0.0"
	}
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := checkReady(client, base); err != nil {
			lastErr = err
		} else if err := checkVersion(client, base, expectedVersion); err != nil {
			lastErr = err
		} else if err := checkClientPlan(client, base, currentVersion, expectedVersion); err != nil {
			lastErr = err
		} else {
			if output != nil {
				fmt.Fprintln(output, "server health, version, and client plan verified")
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("server health verification timed out")
	}
	return fmt.Errorf("verify updated server health: %w", lastErr)
}

func parseHealthBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("health base URL must be an HTTP(S) URL with a host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func healthURL(base *url.URL, suffix string) string {
	copy := *base
	copy.Path = strings.TrimRight(base.Path, "/") + suffix
	return copy.String()
}

func checkReady(client *http.Client, base *url.URL) error {
	res, err := client.Get(healthURL(base, "/ready"))
	if err != nil {
		return fmt.Errorf("request /ready: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("/ready returned HTTP %d", res.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := readJSONResponse(res, &body); err != nil {
		return fmt.Errorf("decode /ready: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(body.Status), "ready") {
		return fmt.Errorf("/ready status is %q", body.Status)
	}
	return nil
}

func checkVersion(client *http.Client, base *url.URL, expectedVersion string) error {
	res, err := client.Get(healthURL(base, "/api/v1/version"))
	if err != nil {
		return fmt.Errorf("request /api/v1/version: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("/api/v1/version returned HTTP %d", res.StatusCode)
	}
	var body struct {
		ServerVersion string `json:"server_version"`
	}
	if err := readJSONResponse(res, &body); err != nil {
		return fmt.Errorf("decode /api/v1/version: %w", err)
	}
	if update.CompareVersions(body.ServerVersion, expectedVersion) != 0 {
		return fmt.Errorf("server version is %q, want %q", body.ServerVersion, expectedVersion)
	}
	return nil
}

func checkClientPlan(client *http.Client, base *url.URL, currentVersion, expectedVersion string) error {
	planURL, err := url.Parse(healthURL(base, "/api/v1/updates/client/plan"))
	if err != nil {
		return err
	}
	query := planURL.Query()
	query.Set("current_version", currentVersion)
	query.Set("target", "windows-x86_64")
	query.Set("install_mode", "portable")
	planURL.RawQuery = query.Encode()
	res, err := client.Get(planURL.String())
	if err != nil {
		return fmt.Errorf("request client plan: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent && update.CompareVersions(currentVersion, expectedVersion) == 0 {
		// A restored stable release correctly reports no client update when the
		// probe already identifies itself as that same version.
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("client plan returned HTTP %d", res.StatusCode)
	}
	var body update.ClientUpdatePlan
	if err := readJSONResponse(res, &body); err != nil {
		return fmt.Errorf("decode client plan: %w", err)
	}
	if body.ProtocolVersion != update.ClientUpdateProtocolV3 {
		return fmt.Errorf("client plan protocol is %d, want %d", body.ProtocolVersion, update.ClientUpdateProtocolV3)
	}
	if update.CompareVersions(body.LatestVersion, expectedVersion) != 0 || body.Target != "windows-x86_64" || body.InstallMode != "portable" || body.Strategy != "full" {
		return fmt.Errorf("client plan latest=%q strategy=%q does not match full release %q", body.LatestVersion, body.Strategy, expectedVersion)
	}
	if err := validateHealthPlanArtifact(body.Artifact, "portable"); err != nil {
		return fmt.Errorf("client plan artifact: %w", err)
	}
	if err := validateHealthPlanArtifact(body.FullFallback, "portable"); err != nil {
		return fmt.Errorf("client plan full fallback: %w", err)
	}
	if !strings.EqualFold(body.Artifact.SHA256, body.FullFallback.SHA256) || body.Artifact.Size != body.FullFallback.Size || body.Artifact.Signature != body.FullFallback.Signature {
		return errors.New("client plan artifact does not match its full fallback")
	}
	return nil
}

func validateHealthPlanArtifact(artifact update.ClientUpdatePlanArtifact, expectedKind string) error {
	if artifact.Kind != expectedKind || artifact.Size <= 0 || !validSHA256(artifact.SHA256) || strings.TrimSpace(artifact.Signature) == "" {
		return errors.New("client plan does not contain a valid full portable artifact")
	}
	wantPath := "/api/v1/updates/client/artifacts/" + strings.ToLower(artifact.SHA256)
	if artifact.DownloadPath != wantPath {
		return fmt.Errorf("client plan artifact path %q does not match its SHA-256", artifact.DownloadPath)
	}
	return nil
}

func readJSONResponse(res *http.Response, target any) error {
	if res == nil || res.Body == nil {
		return errors.New("empty HTTP response")
	}
	decoder := jsonDecoder(io.LimitReader(res.Body, 2<<20))
	return decoder(target)
}

// jsonDecoder is a tiny indirection to keep response parsing bounded while
// allowing tests to replace no global state.
func jsonDecoder(reader io.Reader) func(any) error {
	return func(target any) error {
		return decodeJSON(reader, target)
	}
}

func decodeJSON(reader io.Reader, target any) error {
	// Kept in this file so the updater has one bounded JSON parsing path.
	return json.NewDecoder(reader).Decode(target)
}
