// Package upgrade self-updates the updash binary from a GitHub release.
// The release URL can be overridden with UPDASH_UPDATE_API / UPDASH_UPDATE_URL
// env vars, making it compatible with any GitHub-compatible releases host.
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
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Defaults — public GitHub. Override via env vars for private mirrors.
const (
	DefaultUpdateAPI = "https://api.github.com/repos/lgldsilva/updash"
	DefaultUpdateDL  = "https://github.com/lgldsilva/updash/releases/download"
	versionPrefix    = "v"
	pathSeparator    = "/"
	// maxRedirects bounds the redirect chain a release download may follow.
	maxRedirects = 5
)

// Transfer/decompression budgets. A release archive for a single Go CLI is a
// few megabytes; these ceilings leave generous headroom while keeping a
// hostile or broken host from streaming an unbounded body into memory or
// expanding a compression bomb. They are vars so hermetic tests can shrink
// them instead of generating multi-megabyte fixtures.
//
// Because tests mutate these package-level vars (see withArchiveLimit /
// withExtractLimit in hardening_test.go), tests in this package MUST NOT call
// t.Parallel(): a parallel test would observe another test's shrunken budget.
var (
	maxAPIResponseBytes = int64(8 << 20)   // 8 MiB  — release JSON
	maxChecksumsBytes   = int64(1 << 20)   // 1 MiB  — checksums.txt
	maxArchiveBytes     = int64(64 << 20)  // 64 MiB — tar.gz/zip asset
	maxExtractedBytes   = int64(192 << 20) // 192 MiB — total decompressed
)

// Config holds upgrade configuration, sourced from env vars.
type Config struct {
	API       string // GitHub API base URL for releases
	Download  string // Download URL prefix for release assets
	Token     string // Optional token for private repos
	CAFile    string // Optional custom CA cert path (self-signed host)
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
	return tag, isNewer(currentVersion, tag), nil
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
		if !isNewer(currentVersion, tag) {
			fmt.Println("up to date.")
		} else {
			fmt.Println("an update is available — run `updash upgrade`.")
		}
		return nil
	}

	// Auto-latest upgrades are forward-only: a dev/snapshot build newer than
	// the latest stable release must not be downgraded. An explicit
	// --version still installs exactly what was asked.
	if cfg.Version == "" && !isNewer(currentVersion, tag) {
		fmt.Println("already up to date.")
		return nil
	}
	if !canSelfUpdate() {
		return errors.New("this installation is managed by a package manager; update it with that package manager (set UPDASH_ALLOW_SELF_UPDATE=1 only for a deliberately self-managed install)")
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
	base := strings.TrimRight(apiURL, pathSeparator)

	// Try /releases/latest (GitHub-compatible)
	if tag, err := fetchLatestFromEndpoint(ctx, hc, base+"/releases/latest", token); err == nil {
		return tag, nil
	} else if !isHTTP404(err) {
		return "", err
	}

	// Fallback: list all releases (paginated, GitHub-compatible)
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
		// Match /releases/latest semantics: drafts and prereleases are not
		// candidates for "latest" — a fallback host must not auto-install
		// an -rc build.
		if r.Draft || r.Prerelease || r.TagName == "" {
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
	archiveURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(dlURL, pathSeparator), tag, archName)
	fmt.Printf("download: %s\n", archiveURL)

	// Download archive (bounded: a hostile host must not be able to stream an
	// unbounded body into memory).
	archiveData, err := httpGetLimited(ctx, hc, archiveURL, token, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}

	// Download checksum
	checksumsURL := fmt.Sprintf("%s/%s/checksums.txt", strings.TrimRight(dlURL, pathSeparator), tag)
	checksumsData, err := httpGetLimited(ctx, hc, checksumsURL, token, maxChecksumsBytes)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}

	// Verify SHA-256. A missing, malformed, or ambiguous entry must fail
	// closed: installing an unverified release would turn a broken checksum
	// manifest into a supply-chain bypass.
	expectedHash, err := findChecksum(checksumsData, archName)
	if err != nil {
		return nil, fmt.Errorf("verify checksums: %w", err)
	}
	if expectedHash == "" {
		return nil, fmt.Errorf("no checksum entry for %s", archName)
	}
	got := sha256.Sum256(archiveData)
	gotHex := hex.EncodeToString(got[:])
	if gotHex != expectedHash {
		return nil, fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedHash, gotHex)
	}
	fmt.Println("checksum: verified")

	// Extract binary from archive
	bin, err := extractBinary(archiveData, archName, goos)
	if err != nil {
		return nil, fmt.Errorf("extract binary: %w", err)
	}
	return bin, nil
}

func archiveName(tag, goos, goarch string) string {
	// GoReleaser name_template uses .Version (no "v"); release tags keep the prefix.
	ver := strings.TrimPrefix(tag, versionPrefix)
	switch goos {
	case "windows":
		return fmt.Sprintf("updash_%s_%s_%s.zip", ver, goos, goarch)
	default:
		return fmt.Sprintf("updash_%s_%s_%s.tar.gz", ver, goos, goarch)
	}
}

var sha256HexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// findChecksum returns the lowercase SHA-256 digest published for filename.
// A digest that is not a well-formed SHA-256, or two entries for the same
// file that disagree, are hard errors: silently taking the first line would
// let a tampered manifest choose which digest the client trusts.
func findChecksum(checksums []byte, filename string) (string, error) {
	var found string
	for _, line := range strings.Split(string(checksums), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "  "+filename) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			// Defensive: a suffix match implies a digest and a filename, but
			// the parse must not depend on that implicit invariant.
			continue
		}
		hash := strings.ToLower(parts[0])
		if !sha256HexRE.MatchString(hash) {
			return "", fmt.Errorf("malformed sha256 digest for %s", filename)
		}
		if found != "" && found != hash {
			return "", fmt.Errorf("conflicting checksum entries for %s", filename)
		}
		found = hash
	}
	return found, nil
}

func extractBinary(data []byte, archName, goos string) ([]byte, error) {
	if strings.HasSuffix(archName, ".zip") {
		return extractFromZip(data)
	}
	return extractFromTarGz(data)
}

// archiveMember is one regular file found inside a release archive.
type archiveMember struct {
	name string
	data []byte
}

func extractFromTarGz(data []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	var members []archiveMember
	budget := maxExtractedBytes
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if err := safeArchiveMemberName(hdr.Name); err != nil {
			return nil, err
		}
		// Only regular files are candidates: symlinks/hardlinks in a release
		// archive would point outside the archive by definition.
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, err := readArchiveMember(tr, hdr.Name, &budget)
		if err != nil {
			return nil, err
		}
		members = append(members, archiveMember{name: hdr.Name, data: body})
	}
	return pickReleaseBinary(members)
}

func extractFromZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	var members []archiveMember
	budget := maxExtractedBytes
	for _, f := range zr.File {
		if err := safeArchiveMemberName(f.Name); err != nil {
			return nil, err
		}
		if f.FileInfo().IsDir() || !f.FileInfo().Mode().IsRegular() {
			continue
		}
		body, err := readZipMember(f, &budget)
		if err != nil {
			return nil, err
		}
		members = append(members, archiveMember{name: f.Name, data: body})
	}
	return pickReleaseBinary(members)
}

func readZipMember(f *zip.File, budget *int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer func() { _ = r.Close() }()
	return readArchiveMember(r, f.Name, budget)
}

// safeArchiveMemberName rejects members that escape the archive root. Nothing
// from the archive is written to disk by path, but an entry named
// "../../updash" would still let a crafted release masquerade as the release
// binary once the name is reduced to its base.
func safeArchiveMemberName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("archive contains an unnamed member")
	}
	if len(name) >= 2 && name[1] == ':' {
		return fmt.Errorf("absolute path in archive: %q", name)
	}
	clean := path.Clean(strings.ReplaceAll(name, `\`, pathSeparator))
	if strings.HasPrefix(clean, pathSeparator) {
		return fmt.Errorf("absolute path in archive: %q", name)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path traversal in archive: %q", name)
	}
	return nil
}

// readArchiveMember reads one member against a shared decompression budget
// instead of trusting the size the archive declares (zip/tar bomb defense).
func readArchiveMember(r io.Reader, name string, budget *int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, *budget+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > *budget {
		return nil, fmt.Errorf("archive exceeds the %d byte decompression budget at %s", maxExtractedBytes, name)
	}
	*budget -= int64(len(data))
	return data, nil
}

// pickReleaseBinary selects the updash executable from release archive members.
// Prefer an explicit "updash" / "updash.exe" name; never install README/LICENSE
// (GoReleaser often packs docs before the binary — that used to brick --upgrade).
func pickReleaseBinary(members []archiveMember) ([]byte, error) {
	if len(members) == 0 {
		return nil, errors.New("no files found in archive")
	}
	var fallback []byte
	for _, m := range members {
		base := filepath.Base(m.name)
		if isReleaseBinaryName(base) {
			// The name alone is not proof: refuse to install a script or text
			// file that merely claims to be the release binary.
			if !looksLikeExecutable(m.data) {
				return nil, fmt.Errorf("archive member %q is not an executable image", m.name)
			}
			return m.data, nil
		}
		if isSkippableArchiveFile(base) {
			continue
		}
		if looksLikeExecutable(m.data) && fallback == nil {
			fallback = m.data
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("no updash binary found in archive (only docs/text?)")
}

func isReleaseBinaryName(base string) bool {
	switch strings.ToLower(base) {
	case "updash", "updash.exe":
		return true
	default:
		return false
	}
}

func isSkippableArchiveFile(base string) bool {
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".md"),
		strings.HasSuffix(lower, ".txt"),
		strings.HasSuffix(lower, ".rst"),
		lower == "license", lower == "licence", lower == "copying",
		lower == "changelog", lower == "notice", lower == "authors":
		return true
	default:
		return false
	}
}

// looksLikeExecutable checks common binary magic numbers (ELF / Mach-O / PE).
func looksLikeExecutable(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// ELF
	if data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return true
	}
	// Mach-O 64/32 (big/little endian)
	if (data[0] == 0xfe && data[1] == 0xed && data[2] == 0xfa) ||
		(data[0] == 0xcf && data[1] == 0xfa && data[2] == 0xed && data[3] == 0xfe) ||
		(data[0] == 0xce && data[1] == 0xfa && data[2] == 0xed && data[3] == 0xfe) ||
		(data[0] == 0xca && data[1] == 0xfe && data[2] == 0xba && data[3] == 0xbe) {
		return true
	}
	// PE / DOS MZ
	if data[0] == 'M' && data[1] == 'Z' {
		return true
	}
	return false
}

// --- Binary replacement ---

func replaceRunningBinary(newBin []byte) error {
	return replaceRunningBinaryWithOS(newBin, runtime.GOOS)
}

func replaceRunningBinaryWithOS(newBin []byte, goos string) error {
	// An empty payload would truncate a working installation into a
	// zero-byte file that can never self-heal.
	if len(newBin) == 0 {
		return errors.New("refusing to install an empty binary")
	}
	self, err := osExecutable()
	if err != nil {
		return fmt.Errorf("resolve self path: %w", err)
	}
	self, err = evalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}
	dir := filepath.Dir(self)
	old := self + ".old"

	// Stage next to the destination (same filesystem => the rename is atomic)
	// and only then swap. The running binary is never truncated in place.
	tmp, err := writeStagedBinary(dir, newBin, currentBinaryMode(self))
	if err != nil {
		return err
	}

	if err := os.Rename(tmp, self); err != nil {
		if goos != "windows" {
			_ = os.Remove(tmp)
			return fmt.Errorf("replace binary: %w", err)
		}
		// On Windows the running executable is locked. Rename it out of the
		// way and move the new binary into place. The .old file is left for
		// CleanupOldBinary() to remove on the next startup.
		if err := performWindowsReplace(tmp, self, old); err != nil {
			return err
		}
	}
	return nil
}

// currentBinaryMode preserves the permissions of the installed binary: an
// install deliberately restricted to 0700 must not be widened by a self
// update. 0755 is the fallback when the current mode cannot be read; the
// executable bit is always forced on.
func currentBinaryMode(self string) os.FileMode {
	fi, err := os.Stat(self)
	if err != nil {
		return 0o755
	}
	return fi.Mode().Perm() | 0o100
}

// writeStagedBinary writes the verified payload into a fresh, unpredictable
// file in dir. os.CreateTemp uses O_CREATE|O_EXCL, so a pre-planted symlink
// at a guessable staging path cannot redirect the write outside dir. The
// content is fsynced before the caller may rename it over the running binary,
// so a crash mid-upgrade cannot leave a half-written executable in place.
func writeStagedBinary(dir string, newBin []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".updash.upgrade-*")
	if err != nil {
		return "", fmt.Errorf("write temp binary: %w", err)
	}
	tmp := f.Name()
	if err := stageBinaryContents(f, newBin, mode); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close temp binary: %w", err)
	}
	return tmp, nil
}

func stageBinaryContents(f *os.File, newBin []byte, mode os.FileMode) error {
	if _, err := f.Write(newBin); err != nil {
		return fmt.Errorf("write temp binary: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flush temp binary: %w", err)
	}
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp binary: %w", err)
	}
	return nil
}

func performWindowsReplace(tmp, self, old string) error {
	_ = os.Remove(old)
	if renErr := os.Rename(self, old); renErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace binary (stage old): %w", renErr)
	}
	if mvErr := os.Rename(tmp, self); mvErr != nil {
		// Best-effort rollback.
		rollbackWindowsReplace(old, self, tmp)
		return fmt.Errorf("replace binary (stage new): %w", mvErr)
	}
	return nil
}

func rollbackWindowsReplace(old, self, tmp string) {
	_ = os.Rename(old, self)
	_ = os.Remove(tmp)
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
	// Strip the leading version prefix from both for comparison.
	current = strings.TrimPrefix(current, versionPrefix)
	tag = strings.TrimPrefix(tag, versionPrefix)
	return current == tag
}

// isNewer reports whether tag is strictly newer than current by semver.
// Unparseable versions (dev builds, dirty git describes) never count as
// older — the startup auto-upgrade must not replace a newer snapshot with
// an older stable release. Pre-release suffixes parse to their base
// version, so an -rc of the current version is not "newer" either.
func isNewer(current, tag string) bool {
	cv, cok := parseTag(current)
	tv, tok := parseTag(tag)
	if !cok || !tok {
		return false
	}
	if tv.less(cv) {
		return false
	}
	return cv.less(tv)
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
		Timeout:       120 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: tlsConfigFor(cfg)},
		CheckRedirect: checkRedirect,
	}
}

// checkRedirect keeps a release download on a verified transport: bounded hop
// count, and never a downgrade from https to a plaintext scheme (which would
// expose the artifact and any Authorization header to a network attacker).
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	prev := via[len(via)-1]
	if prev.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect from https to %s", req.URL.Scheme)
	}
	return nil
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
	pool, poolErr := x509.SystemCertPool()
	systemPoolAvailable := poolErr == nil && pool != nil
	if !systemPoolAvailable {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		fmt.Fprintln(os.Stderr, "warning: custom CA file contains no certificates; using system certificate pool")
		// A nil RootCAs tells crypto/tls to use the platform's system pool.
		if systemPoolAvailable {
			tlsCfg.RootCAs = pool
		}
		return tlsCfg
	}
	tlsCfg.RootCAs = pool
	return tlsCfg
}

// httpGet reads a release-metadata response under the API size budget.
func httpGet(ctx context.Context, hc *http.Client, url, token string) ([]byte, error) {
	return httpGetLimited(ctx, hc, url, token, maxAPIResponseBytes)
}

// httpGetLimited fails closed when the response is larger than limit, both by
// the declared Content-Length and by what is actually streamed.
func httpGetLimited(ctx context.Context, hc *http.Client, url, token string, limit int64) ([]byte, error) {
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
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("GET %s: declared %d bytes, over the %d byte limit", url, resp.ContentLength, limit)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("GET %s: response exceeds the %d byte limit", url, limit)
	}
	return body, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
