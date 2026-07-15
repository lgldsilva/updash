// Package upgrade self-updates the updash binary from a Gitea release.
// The release URL can be overridden with UPDASH_UPDATE_API / UPDASH_UPDATE_URL
// env vars, making it compatible with any Gitea or GitHub releases host.
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Defaults — homelab Gitea. Override via env vars.
const (
	DefaultUpdateAPI = "https://github.com/api/v1/repos/lgldsilva/updash"
	DefaultUpdateDL  = "https://github.com/lgldsilva/updash/releases/download"
)

// Config holds upgrade configuration, sourced from env vars.
type Config struct {
	API       string // Gitea API base URL for releases
	Download  string // Download URL prefix for release assets
	Token     string // Optional token for private repos
	CAFile    string // Optional custom CA cert path (self-signed homelab)
	CheckOnly bool   // Only check, don't install
	Version   string // Specific version to install (default: latest)
}

// EffectiveConfig reads configuration from environment variables.
func EffectiveConfig() Config {
	return Config{
		API:      envOr("UPDASH_UPDATE_API", DefaultUpdateAPI),
		Download: envOr("UPDASH_UPDATE_URL", DefaultUpdateDL),
		Token:    os.Getenv("UPDASH_UPDATE_TOKEN"),
		CAFile:   os.Getenv("UPDASH_TLS_CA_CERT"),
	}
}

// Check queries the release API and returns whether an update is available.
func Check(ctx context.Context, cfg Config, currentVersion string) (string, bool, error) {
	tag, err := resolveTag(ctx, cfg, cfg.Version)
	if err != nil {
		return "", false, fmt.Errorf("resolve version: %w", err)
	}
	if cfg.CheckOnly && cfg.Version == "" {
		return tag, !sameVersion(currentVersion, tag), nil
	}
	return tag, !sameVersion(currentVersion, tag), nil
}

// Run performs a full upgrade: check, download, verify, install.
func Run(ctx context.Context, cfg Config, currentVersion string) error {
	hc := httpClient(cfg)

	tag, err := resolveTag(ctx, cfg, cfg.Version)
	if err != nil {
		return fmt.Errorf("resolve version: %w", err)
	}

	fmt.Printf("current: %s\nlatest:  %s\n", currentVersion, tag)

	if cfg.CheckOnly {
		if sameVersion(currentVersion, tag) {
			fmt.Println("up to date.")
		} else {
			fmt.Println("an update is available — run `updash upgrade`.")
		}
		return nil
	}

	if cfg.Version == "" && sameVersion(currentVersion, tag) {
		fmt.Println("already up to date.")
		return nil
	}

	return install(ctx, hc, cfg, tag)
}

// install downloads, verifies, and atomically replaces the running binary.
func install(ctx context.Context, hc *http.Client, cfg Config, tag string) error {
	bin, err := downloadReleaseBinary(ctx, hc, cfg.Download, tag, runtime.GOOS, runtime.GOARCH, cfg.Token)
	if err != nil {
		return err
	}
	if err := replaceRunningBinary(bin); err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	fmt.Printf("upgraded to %s\n", tag)
	return nil
}

// --- Tag resolution ---

func resolveTag(ctx context.Context, cfg Config, want string) (string, error) {
	if want != "" {
		return want, nil
	}
	return fetchLatestTag(ctx, httpClient(cfg), cfg.API, cfg.Token)
}

func fetchLatestTag(ctx context.Context, hc *http.Client, apiURL, token string) (string, error) {
	base := strings.TrimRight(apiURL, "/")

	// Try /releases/latest (GitHub-compatible)
	if tag, err := fetchLatestFromEndpoint(ctx, hc, base+"/releases/latest", token); err == nil {
		return tag, nil
	} else if !isHTTP404(err) {
		return "", err
	}

	// Fallback: list all releases (Gitea-style)
	return fetchLatestFromList(ctx, hc, base+"/releases?limit=50", token)
}

func fetchLatestFromEndpoint(ctx context.Context, hc *http.Client, url, token string) (string, error) {
	body, err := httpGet(ctx, hc, url, token)
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("parse release: %w", err)
	}
	if rel.TagName == "" {
		return "", errors.New("no tag_name in latest release")
	}
	return rel.TagName, nil
}

func fetchLatestFromList(ctx context.Context, hc *http.Client, url, token string) (string, error) {
	body, err := httpGet(ctx, hc, url, token)
	if err != nil {
		return "", err
	}
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("parse release list: %w", err)
	}
	var tags []string
	for _, r := range releases {
		if r.Draft || r.TagName == "" {
			continue
		}
		tags = append(tags, r.TagName)
	}
	if len(tags) == 0 {
		return "", errors.New("no published releases found")
	}
	sort.Slice(tags, func(i, j int) bool {
		return compareTags(tags[i], tags[j]) < 0
	})
	return tags[len(tags)-1], nil
}

// --- Download & verify ---

func downloadReleaseBinary(ctx context.Context, hc *http.Client, dlURL, tag, goos, goarch, token string) ([]byte, error) {
	archName := archiveName(tag, goos, goarch)
	archiveURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(dlURL, "/"), tag, archName)
	fmt.Printf("download: %s\n", archiveURL)

	// Download archive
	archiveData, err := httpGet(ctx, hc, archiveURL, token)
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}

	// Download checksum
	checksumsURL := fmt.Sprintf("%s/%s/checksums.txt", strings.TrimRight(dlURL, "/"), tag)
	checksumsData, err := httpGet(ctx, hc, checksumsURL, token)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}

	// Verify SHA-256
	expectedHash := findChecksum(checksumsData, archName)
	if expectedHash != "" {
		got := sha256.Sum256(archiveData)
		gotHex := hex.EncodeToString(got[:])
		if gotHex != expectedHash {
			return nil, fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedHash, gotHex)
		}
		fmt.Println("checksum: verified")
	}

	// Extract binary from archive
	bin, err := extractBinary(archiveData, archName, goos)
	if err != nil {
		return nil, fmt.Errorf("extract binary: %w", err)
	}
	return bin, nil
}

func archiveName(tag, goos, goarch string) string {
	// GoReleaser name_template uses .Version (no "v"); release tags keep the prefix.
	ver := strings.TrimPrefix(tag, "v")
	switch goos {
	case "windows":
		return fmt.Sprintf("updash_%s_%s_%s.zip", ver, goos, goarch)
	default:
		return fmt.Sprintf("updash_%s_%s_%s.tar.gz", ver, goos, goarch)
	}
}

func findChecksum(checksums []byte, filename string) string {
	for _, line := range strings.Split(string(checksums), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "  "+filename) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[0]
			}
		}
	}
	return ""
}

func extractBinary(data []byte, archName, goos string) ([]byte, error) {
	if strings.HasSuffix(archName, ".zip") {
		return extractFromZip(data)
	}
	return extractFromTarGz(data)
}

func extractFromTarGz(data []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	_ = gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Accept the first regular file
		bin, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", hdr.Name, err)
		}
		return bin, nil
	}
	return nil, errors.New("no files found in archive")
}

func extractFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		defer func() { _ = r.Close() }()
		bin, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		return bin, nil
	}
	return nil, errors.New("no files found in archive")
}

// --- Binary replacement ---

func replaceRunningBinary(newBin []byte) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve self path: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}
	dir := filepath.Dir(self)
	tmp := filepath.Join(dir, ".updash.upgrade.tmp")

	// #nosec G306 — binary needs executable permission
	if err := os.WriteFile(tmp, newBin, 0755); err != nil {
		return fmt.Errorf("write temp binary: %w", err)
	}
	if err := os.Rename(tmp, self); err != nil {
		_ = os.Remove(tmp) // clean up
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// --- Version comparison ---

var tagVersionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

type tagVer struct{ major, minor, patch int }

func (v tagVer) less(o tagVer) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

func parseTag(tag string) (tagVer, bool) {
	m := tagVersionRE.FindStringSubmatch(tag)
	if m == nil {
		return tagVer{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return tagVer{major: maj, minor: min, patch: pat}, true
}

func compareTags(a, b string) int {
	av, aok := parseTag(a)
	bv, bok := parseTag(b)
	if aok && bok {
		if av.less(bv) {
			return -1
		}
		if bv.less(av) {
			return 1
		}
		return 0
	}
	return strings.Compare(a, b)
}

func sameVersion(current, tag string) bool {
	if current == "" || tag == "" {
		return false
	}
	// Strip leading 'v' from both for comparison
	current = strings.TrimPrefix(current, "v")
	tag = strings.TrimPrefix(tag, "v")
	return current == tag
}

// --- HTTP helpers ---

type httpError struct {
	StatusCode int
	URL        string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.StatusCode)
}

func isHTTP404(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.StatusCode == 404
}

func httpClient(cfg Config) *http.Client {
	return &http.Client{
		Timeout:   120 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfigFor(cfg)},
	}
}

// tlsConfigFor builds TLS settings (TLS 1.2+). Self-signed hosts use CAFile
// (UPDASH_TLS_CA_CERT) — verification is never disabled.
func tlsConfigFor(cfg Config) *tls.Config {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile == "" {
		return tlsCfg
	}
	pem, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return tlsCfg
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(pem)
	tlsCfg.RootCAs = pool
	return tlsCfg
}

func httpGet(ctx context.Context, hc *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, &httpError{StatusCode: resp.StatusCode, URL: url}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
