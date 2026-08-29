package scanner

import (
	"errors"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestParseDnfCheckUpdate(t *testing.T) {
	out := `
Last metadata expiration check: 0:12:34 ago.
kernel.x86_64                     6.9.7-200.fc40           updates
bash.noarch                       5.2.26-3.fc40            updates
Security: kernel is a security update
glibc.i686                        2.39-22.fc40             updates
`
	items := ParseDnfCheckUpdate(out)
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	if items[0].Name != "kernel" || items[0].AvailableVer != "6.9.7-200.fc40" {
		t.Errorf("bad first item: %+v", items[0])
	}
	if items[1].Name != "bash" {
		t.Errorf("arch suffix not stripped: %q", items[1].Name)
	}
	if items[2].Name != "glibc" {
		t.Errorf("i686 arch not stripped: %q", items[2].Name)
	}
	for _, it := range items {
		if it.Status.String() != "outdated" {
			t.Errorf("item %s: want outdated, got %s", it.Name, it.Status)
		}
	}
}

func TestParseDnfCheckUpdateEmpty(t *testing.T) {
	if items := ParseDnfCheckUpdate(""); len(items) != 0 {
		t.Fatalf("want 0 items, got %d", len(items))
	}
	if items := ParseDnfCheckUpdate("Last metadata expiration check: 0:00:01 ago.\n"); len(items) != 0 {
		t.Fatalf("header lines must be skipped, got %d items", len(items))
	}
}

func TestParseZypperListUpdates(t *testing.T) {
	out := `Loading repository data...
Reading installed packages...
S | Repository      | Name       | Current Version | Available Version | Arch
--+-----------------+------------+-----------------+-------------------+-----
v | openSUSE-oss    | bash       | 5.1.8-1.1       | 5.1.8-2.1         | x86_64
v | openSUSE-Update | kernel     | 6.5-1.1         | 6.9-2.1           | x86_64
i | some-repo       | other      | 1.0-1.1         | 1.1-1.1           | x86_64
`
	items := ParseZypperListUpdates(out)
	if len(items) != 2 {
		t.Fatalf("want 2 items (only v rows), got %d", len(items))
	}
	if items[0].Name != "bash" || items[0].CurrentVer != "5.1.8-1.1" || items[0].AvailableVer != "5.1.8-2.1" {
		t.Errorf("bad first item: %+v", items[0])
	}
	if items[1].Name != "kernel" {
		t.Errorf("bad second item: %+v", items[1])
	}
}

func TestParseZypperListUpdatesEmpty(t *testing.T) {
	if items := ParseZypperListUpdates("No updates found.\n"); len(items) != 0 {
		t.Fatalf("want 0 items, got %d", len(items))
	}
}

func TestZypperScanFailureWithOutputIsError(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("zypper", []string{"--quiet", "--non-interactive", "list-updates"}, "warning: repository unavailable", errors.New("zypper failed"))

	items, _ := (&ZypperSource{}).Scan(t.Context(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("expected error for failed zypper command, got %+v", items)
	}
}

func TestParseApkVersion(t *testing.T) {
	out := `Installed:                                Available:
openssl-3.1.4-r5                        < openssl-3.1.5-r0
zlib-1.2.13-r1                          < zlib-1.3-r0
`
	items := ParseApkVersion(out)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "openssl" || items[0].CurrentVer != "3.1.4-r5" || items[0].AvailableVer != "3.1.5-r0" {
		t.Errorf("bad first item: %+v", items[0])
	}
	if items[1].Name != "zlib" || items[1].AvailableVer != "1.3-r0" {
		t.Errorf("bad second item: %+v", items[1])
	}
}

func TestApkSplitNameVer(t *testing.T) {
	cases := []struct{ in, name, ver string }{
		{"musl-1.2.5-r0", "musl", "1.2.5-r0"},
		{"libssl3-3.1.4-r5", "libssl3", "3.1.4-r5"},
		{"ca-certificates-20240226-r0", "ca-certificates", "20240226-r0"},
		{"weird", "weird", ""},
	}
	for _, c := range cases {
		name, ver := apkSplitNameVer(c.in)
		if name != c.name || ver != c.ver {
			t.Errorf("apkSplitNameVer(%q) = (%q,%q), want (%q,%q)", c.in, name, ver, c.name, c.ver)
		}
	}
}

func TestApkScanFailureWithOutputIsError(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("apk", []string{"version", "-l", "<"}, "WARNING: database is locked", errors.New("apk failed"))

	items, _ := (&ApkSource{}).Scan(t.Context(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("expected error for failed apk command, got %+v", items)
	}
}

func TestIsExitCode(t *testing.T) {
	if isExitCode(nil, 100) {
		t.Error("nil error must not match")
	}
}
