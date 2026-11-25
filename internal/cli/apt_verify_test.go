package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// End-to-end regression for the apt verify report: the fast scan lists one
// package, the plan runs `apt-get update` and installs only that name, and
// the verify rescan lists three (two were published after the scan). The
// real AptSource parser and the real updater planner/executor run against
// fixture binaries that are the ONLY entries on PATH, so no host apt or sudo
// is reachable: the fake sudo never executes its argument, it only records it.
func TestRunUpdate_AptPackagesAppearingAfterScanAreNotFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apt fixtures are shell scripts")
	}
	restoreHooks(t)
	fakeScan(nil, nil)
	dir, state, log := writeFakeAptToolchain(t)
	plat := model.PlatformInfo{OS: "linux", Distro: "ubuntu"}
	scans := 0
	runScannerAll = func(ctx context.Context, _ model.PlatformInfo, _ bool) []*model.SourceSummary {
		scans++
		items, err := (&scanner.AptSource{}).Scan(ctx, plat)
		if err != nil {
			t.Fatalf("fake apt scan: %v", err)
		}
		return []*model.SourceSummary{{Category: model.CatApt, Label: "apt", Icon: "🐧", Items: items}}
	}
	withRealUpdater(t)
	canElevateNP = func(context.Context) bool { return true }
	dropInheritedUpdateEnv(t)
	t.Setenv("PATH", dir)

	var ok, fail int
	var err error
	out := captureStdout(t, func() {
		ok, fail, err = RunUpdate(context.Background(), Config{})
	})
	if err != nil || ExitCode(err) != 0 {
		t.Fatalf("ok=%d fail=%d err=%v\n%s", ok, fail, err, out)
	}
	if ok != 1 || fail != 0 || scans != 2 {
		t.Fatalf("ok=%d fail=%d scans=%d\n%s", ok, fail, scans, out)
	}

	calls := readLines(t, log)
	if !containsLine(calls, "apt-get update") || !containsLine(calls, "apt-get install --only-upgrade -y docker-ce") {
		t.Fatalf("plan did not run the scanned name only:\n%s", strings.Join(calls, "\n"))
	}
	for _, c := range calls {
		if strings.Contains(c, "ghostty") || strings.Contains(c, "sqlite3") {
			t.Fatalf("unscanned package reached the install command: %q", c)
		}
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("fake apt was not consulted: %v", err)
	}
	for _, want := range []string{"2 not attempted", "no planned item is still outdated", "Not attempted this run", "ghostty", "sqlite3", "run again to pick them up"} {
		if !strings.Contains(out, want) {
			t.Fatalf("verify report missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"✘", "item(s) still outdated", "ainda desatualizado"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("verify report leaked %q:\n%s", unwanted, out)
		}
	}
}

// writeFakeAptToolchain puts apt, apt-get and sudo fixtures in one directory.
// apt answers `list --upgradable` with docker-ce on the first call only (the
// plan installs it) and with ghostty + sqlite3 from the second call on —
// packages published after the scan, which the plan never received. The
// call counter lives in a state file. apt-get and sudo only append their
// argv to the log and exit 0. The scripts use shell builtins alone because
// the fixture directory is the whole PATH.
func writeFakeAptToolchain(t *testing.T) (dir, state, log string) {
	t.Helper()
	dir = t.TempDir()
	state = filepath.Join(dir, "apt-calls")
	log = filepath.Join(dir, "calls")
	writeScript(t, filepath.Join(dir, "apt"), "#!/bin/sh\n"+
		"case \"$1 $2\" in\n"+
		"\"list --upgradable\")\n"+
		"n=0; if [ -f "+shQuote(state)+" ]; then read n < "+shQuote(state)+"; fi\n"+
		"n=$((n+1)); printf '%s\\n' \"$n\" > "+shQuote(state)+"\n"+
		"echo 'Listing...'\n"+
		"if [ \"$n\" -eq 1 ]; then echo 'docker-ce/stable 5:27 amd64 [upgradable from: 5:26]'; fi\n"+
		"if [ \"$n\" -ge 2 ]; then\n"+
		"  echo 'ghostty/stable 1.1 amd64 [upgradable from: 1.0]'\n"+
		"  echo 'sqlite3/stable 3.50 amd64 [upgradable from: 3.49]'\n"+
		"fi ;;\n"+
		"*) echo unexpected >&2; exit 9 ;;\n"+
		"esac\n")
	writeScript(t, filepath.Join(dir, "apt-get"), recordArgsScript(log))
	writeScript(t, filepath.Join(dir, "sudo"), recordArgsScript(log))
	return dir, state, log
}

func recordArgsScript(log string) string {
	return "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shQuote(log) + "\nexit 0\n"
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fixture log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}
