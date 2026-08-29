package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Bounded downloads ─────────────────────────────────────────────────────

func TestHTTPGetLimited_rejectsOversizedBody(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	if _, err := httpGetLimited(context.Background(), srv.Client(), srv.URL, "", 1024); err == nil {
		t.Fatal("expected oversized response to fail closed")
	}
}

func TestHTTPGetLimited_rejectsOversizedContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("A"), 999999))
	}))
	defer srv.Close()

	if _, err := httpGetLimited(context.Background(), srv.Client(), srv.URL, "", 1024); err == nil {
		t.Fatal("expected declared oversized length to fail closed")
	}
}

func TestHTTPGetLimited_allowsBodyAtLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	body, err := httpGetLimited(context.Background(), srv.Client(), srv.URL, "", 1024)
	if err != nil {
		t.Fatalf("body exactly at the limit must be accepted: %v", err)
	}
	if len(body) != 1024 {
		t.Fatalf("got %d bytes, want 1024", len(body))
	}
}

func TestDownloadReleaseBinary_rejectsOversizedArchive(t *testing.T) {
	withArchiveLimit(t, 512)

	archive := bytes.Repeat([]byte("A"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = fmt.Fprint(w, "deadbeef  x\n")
			return
		}
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	_, err := downloadReleaseBinary(context.Background(), srv.Client(), srv.URL, "v1.0.0", "linux", "amd64", "")
	if err == nil {
		t.Fatal("expected oversized archive download to fail closed")
	}
	if !strings.Contains(err.Error(), "download archive") {
		t.Fatalf("expected download failure, got %v", err)
	}
}

// A truncated download must never reach the binary replacement path: the
// checksum of the partial body cannot match the manifest.
func TestDownloadReleaseBinary_truncatedArchiveFailsChecksum(t *testing.T) {
	full := makeTarGz(t, map[string][]byte{"updash": {0x7f, 'E', 'L', 'F', 1, 2, 3, 4}})
	h := sha256.Sum256(full)
	checksum := hex.EncodeToString(h[:]) + "  updash_1.0.0_linux_amd64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = fmt.Fprint(w, checksum)
			return
		}
		_, _ = w.Write(full[:len(full)/2])
	}))
	defer srv.Close()

	_, err := downloadReleaseBinary(context.Background(), srv.Client(), srv.URL, "v1.0.0", "linux", "amd64", "")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("truncated archive must fail the checksum gate, got %v", err)
	}
}

// ── Redirect trust ────────────────────────────────────────────────────────

func TestCheckRedirect_rejectsHTTPSDowngrade(t *testing.T) {
	from := mustRequest(t, "https://example.test/a")
	to := mustRequest(t, "http://example.test/b")
	if err := checkRedirect(to, []*http.Request{from}); err == nil {
		t.Fatal("https -> http redirect must be refused")
	}
}

func TestCheckRedirect_allowsHTTPSHop(t *testing.T) {
	from := mustRequest(t, "https://example.test/a")
	to := mustRequest(t, "https://cdn.example.test/b")
	if err := checkRedirect(to, []*http.Request{from}); err != nil {
		t.Fatalf("https -> https redirect must be allowed: %v", err)
	}
}

func TestCheckRedirect_boundsRedirectChain(t *testing.T) {
	var via []*http.Request
	for i := 0; i < maxRedirects; i++ {
		via = append(via, mustRequest(t, "https://example.test/a"))
	}
	if err := checkRedirect(mustRequest(t, "https://example.test/b"), via); err == nil {
		t.Fatalf("chain longer than %d hops must be refused", maxRedirects)
	}
}

func TestHTTPClient_usesRedirectPolicy(t *testing.T) {
	if httpClient(Config{}).CheckRedirect == nil {
		t.Fatal("upgrade http client must install a redirect policy")
	}
}

func mustRequest(t *testing.T, raw string) *http.Request {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Request{URL: u}
}

// ── Bounded / validated extraction ────────────────────────────────────────

func TestExtractFromTarGz_rejectsDecompressionBomb(t *testing.T) {
	withExtractLimit(t, 1024)

	archive := makeTarGz(t, map[string][]byte{"updash": bytes.Repeat([]byte{0x7f}, 8192)})
	if _, err := extractFromTarGz(archive); err == nil {
		t.Fatal("tar bomb must be refused by the decompression budget")
	}
}

func TestExtractFromZip_rejectsDecompressionBomb(t *testing.T) {
	withExtractLimit(t, 1024)

	archive := makeZip(t, map[string][]byte{"updash.exe": bytes.Repeat([]byte{'M'}, 8192)})
	if _, err := extractFromZip(archive); err == nil {
		t.Fatal("zip bomb must be refused by the decompression budget")
	}
}

func TestExtractFromTarGz_rejectsTraversalMember(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"../../updash": {0x7f, 'E', 'L', 'F', 1}})
	if _, err := extractFromTarGz(archive); err == nil {
		t.Fatal("traversal member name must fail closed")
	}
}

func TestExtractFromZip_rejectsAbsoluteMember(t *testing.T) {
	archive := makeZip(t, map[string][]byte{"/etc/updash": {'M', 'Z', 0, 0}})
	if _, err := extractFromZip(archive); err == nil {
		t.Fatal("absolute member name must fail closed")
	}
}

func TestSafeArchiveMemberName(t *testing.T) {
	bad := []string{"", "..", "../updash", "a/../../updash", "/updash", `\updash`, `C:\updash`, `..\updash`}
	for _, name := range bad {
		if err := safeArchiveMemberName(name); err == nil {
			t.Errorf("safeArchiveMemberName(%q) = nil, want error", name)
		}
	}
	for _, name := range []string{"updash", "updash_1.0.0_linux_amd64/updash", "./updash"} {
		if err := safeArchiveMemberName(name); err != nil {
			t.Errorf("safeArchiveMemberName(%q) = %v, want nil", name, err)
		}
	}
}

// A tar entry that declares a symlink out of the archive must never be
// installed as the release binary.
func TestExtractFromTarGz_ignoresSymlinkMember(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "updash", Typeflag: tar.TypeSymlink, Linkname: "/bin/sh", Mode: 0o777,
	}); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()

	if _, err := extractFromTarGz(buf.Bytes()); err == nil {
		t.Fatal("symlink-only archive must not yield a binary")
	}
}

func TestPickReleaseBinary_rejectsNonExecutableNamedMember(t *testing.T) {
	_, err := pickReleaseBinary([]archiveMember{
		{name: "updash", data: []byte("#!/bin/sh\nrm -rf /\n")},
	})
	if err == nil {
		t.Fatal("a member named updash that is not an executable image must be refused")
	}
}

// ── Checksum manifest parsing ─────────────────────────────────────────────

func TestFindChecksum_rejectsMalformedDigest(t *testing.T) {
	_, err := findChecksum([]byte("not-a-sha  updash_1.0.0_linux_amd64.tar.gz\n"), "updash_1.0.0_linux_amd64.tar.gz")
	if err == nil {
		t.Fatal("malformed digest must fail closed")
	}
}

func TestFindChecksum_rejectsConflictingEntries(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("b", 64)
	manifest := a + "  updash_1.0.0_linux_amd64.tar.gz\n" + b + "  updash_1.0.0_linux_amd64.tar.gz\n"
	if _, err := findChecksum([]byte(manifest), "updash_1.0.0_linux_amd64.tar.gz"); err == nil {
		t.Fatal("conflicting checksum entries must fail closed")
	}
}

func TestFindChecksum_normalizesCase(t *testing.T) {
	want := strings.Repeat("a", 64)
	got, err := findChecksum([]byte(strings.ToUpper(want)+"  updash.tar.gz\n"), "updash.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDownloadReleaseBinary_malformedChecksumFailsClosed(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"updash": {0x7f, 'E', 'L', 'F', 1}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = fmt.Fprint(w, "zzzz  updash_1.0.0_linux_amd64.tar.gz\n")
			return
		}
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	if _, err := downloadReleaseBinary(context.Background(), srv.Client(), srv.URL, "v1.0.0", "linux", "amd64", ""); err == nil {
		t.Fatal("malformed checksum manifest must not install anything")
	}
}

// ── Atomic, non-widening binary replacement ───────────────────────────────

func TestReplaceRunningBinaryWithOS_preservesPermissions(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "updash")
	if err := os.WriteFile(self, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	stubSelfUpdateDepsForReplace(t, self)

	if err := replaceRunningBinaryWithOS([]byte("new"), "linux"); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	fi, err := os.Stat(self)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Fatalf("permissions widened: got %o, want 700", got)
	}
}

// A pre-planted symlink at the staging path must not redirect the write: the
// staged file has to be created exclusively.
func TestReplaceRunningBinaryWithOS_doesNotFollowStagedSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(dir, "updash")
	if err := os.WriteFile(self, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".updash.upgrade.tmp")); err != nil {
		t.Fatal(err)
	}
	stubSelfUpdateDepsForReplace(t, self)

	if err := replaceRunningBinaryWithOS([]byte("new"), "linux"); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	victim, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(victim) != "untouched" {
		t.Fatalf("staged write followed a symlink out of the install dir: %q", victim)
	}
	fi, err := os.Lstat(self)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("installed binary must be a regular file, not a symlink")
	}
	got, err := os.ReadFile(self)
	if err != nil || string(got) != "new" {
		t.Fatalf("self = %q err=%v", got, err)
	}
}

func TestReplaceRunningBinaryWithOS_rejectsEmptyPayload(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "updash")
	if err := os.WriteFile(self, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubSelfUpdateDepsForReplace(t, self)

	if err := replaceRunningBinaryWithOS(nil, "linux"); err == nil {
		t.Fatal("empty payload must never truncate the running binary")
	}
	got, err := os.ReadFile(self)
	if err != nil || string(got) != "old" {
		t.Fatalf("running binary was damaged: %q err=%v", got, err)
	}
}

// A failed rename must leave the current binary untouched and remove the
// staged file instead of leaving debris in the install directory.
func TestReplaceRunningBinaryWithOS_failedRenameKeepsCurrentBinary(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "updash")
	if err := os.Mkdir(self, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(self, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubSelfUpdateDepsForReplace(t, self)

	if err := replaceRunningBinaryWithOS([]byte("new"), "linux"); err == nil {
		t.Fatal("expected rename over a non-empty directory to fail")
	}
	if _, err := os.Stat(filepath.Join(self, "keep")); err != nil {
		t.Fatalf("current install was damaged: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".updash.upgrade") {
			t.Fatalf("staged file %q left behind after failure", e.Name())
		}
	}
}

// ── test seams ────────────────────────────────────────────────────────────

func withArchiveLimit(t *testing.T, limit int64) {
	t.Helper()
	prev := maxArchiveBytes
	t.Cleanup(func() { maxArchiveBytes = prev })
	maxArchiveBytes = limit
}

func withExtractLimit(t *testing.T, limit int64) {
	t.Helper()
	prev := maxExtractedBytes
	t.Cleanup(func() { maxExtractedBytes = prev })
	maxExtractedBytes = limit
}

// ── error branches of the staging helpers ─────────────────────────────────

func TestCurrentBinaryMode_fallbackWhenUnreadable(t *testing.T) {
	if got := currentBinaryMode(filepath.Join(t.TempDir(), "absent")); got != 0o755 {
		t.Fatalf("got %o, want 755 fallback", got)
	}
}

func TestCurrentBinaryMode_forcesExecutableBit(t *testing.T) {
	self := filepath.Join(t.TempDir(), "updash")
	if err := os.WriteFile(self, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := currentBinaryMode(self); got != 0o700 {
		t.Fatalf("got %o, want 700", got)
	}
}

func TestStageBinaryContents_writeError(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "staged-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stageBinaryContents(f, []byte("new"), 0o755); err == nil {
		t.Fatal("expected write on a closed file to fail")
	}
}

func TestWriteStagedBinary_createError(t *testing.T) {
	if _, err := writeStagedBinary(filepath.Join(t.TempDir(), "absent"), []byte("new"), 0o755); err == nil {
		t.Fatal("expected staging in a missing directory to fail")
	}
}

func TestReplaceBinaryAt_refusesEmptyDestination(t *testing.T) {
	if err := ReplaceBinaryAt([]byte("payload"), "", "linux"); err == nil {
		t.Fatal("empty destination must fail closed")
	}
}

func TestReplaceBinaryAt_refusesEmptyPayload(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "updash")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceBinaryAt(nil, dest, "linux"); err == nil {
		t.Fatal("empty payload must fail closed")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "old" {
		t.Fatalf("destination must be untouched, got %q err=%v", got, err)
	}
}

func TestReplaceBinaryAt_brokenSymlinkFailsClosed(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "updash")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), dest); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceBinaryAt([]byte("payload"), dest, "linux"); err == nil {
		t.Fatal("a dangling destination symlink must fail closed")
	}
}

func TestReplaceBinaryAt_unreadableDestinationFailsClosed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based lstat errors are not reliable as root")
	}
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o750) })
	dest := filepath.Join(blocked, "updash")
	if err := ReplaceBinaryAt([]byte("payload"), dest, "linux"); err == nil {
		t.Fatal("an uninspectable destination must fail closed")
	}
}

func TestReplaceBinaryAt_firstInstall(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "bin", "updash")
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceBinaryAt([]byte("payload"), dest, "linux"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "payload" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestReadArchiveMember_readError(t *testing.T) {
	budget := int64(1024)
	if _, err := readArchiveMember(errReader{}, "updash", &budget); err == nil {
		t.Fatal("expected read failure to propagate")
	}
}

func TestReadArchiveMember_consumesBudget(t *testing.T) {
	budget := int64(10)
	if _, err := readArchiveMember(strings.NewReader("1234"), "a", &budget); err != nil {
		t.Fatal(err)
	}
	if budget != 6 {
		t.Fatalf("budget = %d, want 6", budget)
	}
	if _, err := readArchiveMember(strings.NewReader("1234567"), "b", &budget); err == nil {
		t.Fatal("expected the shared budget to be exhausted")
	}
}
