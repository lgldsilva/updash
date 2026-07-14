package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// mockCommands stores canned responses for execCommand.

type mockResult struct {
	stdout []byte
	err    error
}

// mockKey builds a key from the command arguments.
func mockKey(name string, args []string) string {
	return name + " " + strings.Join(args, " ")
}

// setMock registers a canned response for a command.

var mockData = struct {
	data map[string]mockResult
}{data: make(map[string]mockResult)}

func setMock(name string, args []string, stdout string, err error) {
	mockData.data[mockKey(name, args)] = mockResult{[]byte(stdout), err}
}

// clearMocks removes all canned responses.

// mockExecCommand is the test replacement for execCommand.
func mockExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	key := mockKey(name, args)
	if res, ok := mockData.data[key]; ok {
		return res.stdout, res.err
	}
	// Fall back to real execution for LookPath etc.
	return realExecCommand(ctx, name, args...)
}

// realExecCommand is the original implementation.
var realExecCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return defaultExecCommand(ctx, name, args...)
}

// defaultExecCommand is the real os/exec implementation, saved at init.
var defaultExecCommand func(ctx context.Context, name string, args ...string) ([]byte, error)

func init() {
	defaultExecCommand = execCommand
}

// enableMocks replaces execCommand with the mock.
func enableMocks() {
	execCommand = mockExecCommand
}

// disableMocks restores the real implementation.
func disableMocks() {
	execCommand = defaultExecCommand
}

// TestMain is not used — we call enable/disable in each test.

// --- Brew Scanner ---

const sampleBrewOutdated = `{
	"formulae": [
		{"name": "btop", "installed_versions": ["1.3.0"], "current_version": "1.5.0", "pinned": false}
	],
	"casks": [
		{"name": "vlc", "installed_versions": ["3.0.18"], "current_version": "3.0.23", "pinned": false}
	]
}`

func TestBrewScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("brew", []string{"outdated", "--greedy", "--json=v2"}, sampleBrewOutdated, nil)

	src := &BrewSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "btop" || items[0].Status != model.StatusOutdated {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[1].Name != "vlc" || items[1].Status != model.StatusOutdated {
		t.Errorf("item[1] = %+v", items[1])
	}
}

func TestBrewScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("brew", []string{"outdated", "--greedy", "--json=v2"}, `{"formulae":[],"casks":[]}`, nil)

	src := &BrewSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected 1 OK item, got %+v", items)
	}
}

func TestBrewScan_Error(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("brew", []string{"outdated", "--greedy", "--json=v2"}, "", errors.New("brew not found"))

	src := &BrewSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan should handle error internally, got: %v", err)
	}
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Errorf("expected 1 error item, got %+v", items)
	}
}

// --- MAS Scanner ---

const masOutdatedOutput = `1234567890  Xcode (16.0 -> 16.1)
497799835  Pixelmator Pro (3.5.0 -> 3.6.0)`

func TestMASScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("mas", []string{"outdated"}, masOutdatedOutput, nil)

	src := &MASource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "Xcode" || items[0].AvailableVer != "16.1" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestMASScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("mas", []string{"outdated"}, "", nil)

	src := &MASource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- NPM Scanner ---

const npmOutdatedJSON = `{
	"@charmland/crush": {"current": "0.79.1", "wanted": "0.84.1", "latest": "0.84.1"},
	"npm": {"current": "11.17.0", "wanted": "12.0.1", "latest": "12.0.1"}
}`

func TestNpmScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"outdated", "-g", "--json"}, npmOutdatedJSON, nil)

	src := &NpmSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "@charmland/crush" || items[0].AvailableVer != "0.84.1" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestNpmScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"outdated", "-g", "--json"}, `{}`, nil)

	src := &NpmSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Winget Scanner ---

const wingetOutdatedJSON = `{"upgrades": [
	{"name": "7zip", "id": "7zip.7zip", "version": "23.01", "newVersion": "24.01"},
	{"name": "Git", "id": "Git.Git", "version": "2.42.0", "newVersion": "2.45.0"}
]}`

func TestWingetScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("winget", []string{"upgrade", "--json"}, wingetOutdatedJSON, nil)

	src := &WingetSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "7zip" || items[0].AvailableVer != "24.01" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

// --- Choco Scanner ---

const chocoOutdatedOutput = `chocolatey|1.2.0|2.0.0|false
git|2.42.0|2.45.0|false`

func TestChocoScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("choco", []string{"outdated", "--no-color", "--limit-output"}, chocoOutdatedOutput, nil)

	src := &ChocoSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "chocolatey" || items[0].CurrentVer != "1.2.0" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestChocoScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("choco", []string{"outdated", "--no-color", "--limit-output"}, "", nil)

	src := &ChocoSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Scoop Scanner ---

const scoopOutdatedOutput = `WARN  Scoop is out of date.
Updates are available for the following packages:
    aria2: 1.36.0-1 (latest: 1.37.0-1)
    git: 2.42.0.windows.1 (latest: 2.45.0.windows.1)`

func TestScoopScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("scoop", []string{"status"}, scoopOutdatedOutput, nil)

	src := &ScoopSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "aria2" || items[0].AvailableVer != "1.37.0-1" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestScoopScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("scoop", []string{"status"}, "Everything is ok!", nil)

	src := &ScoopSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Apt Scanner ---

const aptOutdatedOutput = `Listing...
libssl3/stable 3.1.0 amd64 [upgradable from: 3.0.13]
git/stable 2.45.0 amd64 [upgradable from: 2.43.0]`

func TestAptScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("sudo", []string{"apt-get", "update"}, "", nil)
	setMock("apt", []string{"list", "--upgradable", "-q"}, aptOutdatedOutput, nil)

	src := &AptSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "libssl3" || items[0].AvailableVer != "3.1.0" {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[1].Name != "git" || items[1].CurrentVer != "2.43.0" {
		t.Errorf("item[1] = %+v", items[1])
	}
}

func TestAptScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("sudo", []string{"apt-get", "update"}, "", nil)
	setMock("apt", []string{"list", "--upgradable", "-q"}, "Listing...\n", nil)

	src := &AptSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Snap Scanner ---

func TestSnapScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `Name    Version   Rev   Tracking  Publisher  Notes
core22  2024-01   1234  latest/stable  canonical✓  -
firefox 123.0    5678  latest/stable  mozilla✓    -`

	setMock("snap", []string{"refresh", "--list"}, out, nil)

	src := &SnapSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "core22" || items[0].AvailableVer != "2024-01" {
		t.Errorf("item[0] = %+v", items[0])
	}
}

// --- Flatpak Scanner ---

func TestFlatpakScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `Updates:
1.    org.gnome.Platform     3.38     4.0      stable`

	setMock("flatpak", []string{"update", "--dry-run"}, out, nil)

	src := &FlatpakSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

func TestFlatpakScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("flatpak", []string{"update", "--dry-run"}, "Nothing to do.", nil)

	src := &FlatpakSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Rustup Scanner ---

func TestRustupScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `rustc 1.84.0-x86_64-unknown-linux-gnu is out of date`

	setMock("rustup", []string{"check"}, out, nil)

	src := &RustupSource{}
	// Need HasRustup=true in platform, but the source doesn't check it
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
	if items[0].Status != model.StatusOutdated {
		t.Errorf("expected outdated, got %v", items[0].Status)
	}
}

func TestRustupScan_UpToDate(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `rustc 1.84.0-x86_64-unknown-linux-gnu is up to date`

	setMock("rustup", []string{"check"}, out, nil)

	src := &RustupSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Goup Scanner (Go tools) ---

func TestGoScan_GupOutdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("gup", []string{"update", "--dry-run"}, "btop -> 1.5.0\neza -> 0.20.0", nil)

	src := &GoSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasGup: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "btop" || items[0].Status != model.StatusOutdated {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestGoScan_GupUpToDate(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("gup", []string{"update", "--dry-run"}, "Nothing to update", nil)

	src := &GoSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasGup: true})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Docker Scanner ---

func TestDockerScan_OK(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("docker", []string{"info", "--format", "{{.ServerVersion}}"}, "24.0.7", nil)

	dfOut := `TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          19        1         5.5GB     4.4GB (79%)
Containers      2         0         32kB      32kB (100%)
Local Volumes   7         2         607MB     472MB (77%)
Build Cache     145       0         16GB      15GB`
	setMock("docker", []string{"system", "df"}, dfOut, nil)

	src := &DockerSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

func TestDockerScan_DaemonNotRunning(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("docker", []string{"info", "--format", "{{.ServerVersion}}"}, "", errors.New("daemon not running"))

	src := &DockerSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || !strings.Contains(items[0].CurrentVer, "not running") {
		t.Errorf("expected daemon-not-running, got %+v", items)
	}
}

// --- PACMAN Scanner ---

func TestPacmanScan_Outdated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("yay", []string{"-Qua"}, "core/btop 1.3.0 -> 1.5.0\ncore/git 2.42.0 -> 2.45.0", nil)

	src := &PacmanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestPacmanScan_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("yay", []string{"-Qua"}, "", nil)

	src := &PacmanSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- PIPX Scanner ---

func TestPipxScan_Success(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `{"venvs": {"poetry": {"metadata": {"item_version": "1.8.0"}}}}`
	setMock("pipx", []string{"list", "--json"}, out, nil)

	src := &PipxSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

// --- Cleanup Scanners ---

func TestBrewCleanSource(t *testing.T) {
	// Create a temp cache directory
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	_ = os.MkdirAll(cacheDir, 0755)

	// Override HOME to point to our temp dir
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// The scanner looks for ~/Library/Caches/Homebrew on macOS
	// and ~/.cache/Homebrew on Linux
	macCache := filepath.Join(tmpDir, "Library", "Caches", "Homebrew")
	_ = os.MkdirAll(macCache, 0755)

	// Mock du command
	enableMocks()
	defer disableMocks()
	setMock("du", []string{"-sh", macCache}, "13G\t"+macCache, nil)

	src := &BrewCleanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{OS: "darwin"})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
	t.Logf("brew-cache: %s reclaimable=%s", items[0].CurrentVer, items[0].Reclaimable)
}

// --- NVM / OMZ ---

func TestNvmScan(t *testing.T) {
	tmpDir := t.TempDir()
	nvmDir := filepath.Join(tmpDir, ".nvm")
	_ = os.MkdirAll(nvmDir, 0755)

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	src := &NvmSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].CurrentVer != "installed" {
		t.Errorf("expected installed, got %+v", items)
	}
}

func TestOmzScan(t *testing.T) {
	tmpDir := t.TempDir()
	omzDir := filepath.Join(tmpDir, ".oh-my-zsh")
	_ = os.MkdirAll(omzDir, 0755)

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	src := &OmzSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].CurrentVer != "installed" {
		t.Errorf("expected installed, got %+v", items)
	}
}

// --- Agent Source (with mock) ---

func TestAgentScan(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Mock version commands for agents
	setMock("claude", []string{"--version"}, "2.1.209 (Claude Code)", nil)
	setMock("opencode", []string{"--version"}, "1.17.20", nil)
	setMock("gemini", []string{"--version"}, "0.46.0", nil)

	src := &AgentSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should not be empty (at minimum the "none installed" item)
	if len(items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

// Test that mockExecCommand falls back to real execution for unregistered commands.
func TestMockFallback(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// "go" should work if Go is installed (it is, since we're running tests)
	_, err := mockExecCommand(context.Background(), "go", "version")
	if err != nil {
		t.Logf("note: mock fallback to real exec failed: %v (may be expected in sandbox)", err)
	}
}

// --- SDKMAN Source with temp dir (without execMock) ---

func TestSDKMANSourceScan_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	candidatesDir := filepath.Join(tmpDir, ".sdkman", "candidates")
	_ = os.MkdirAll(filepath.Join(candidatesDir, "java"), 0755)
	_ = os.MkdirAll(filepath.Join(candidatesDir, "gradle"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "java", "11.0.25-tem"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "java", "21.0.7-tem"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "java", "21.0.5-tem"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "java", "current"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "gradle", "8.14.4"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "gradle", "8.14.1"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "gradle", "9.4.1"), 0755)
	_ = os.Mkdir(filepath.Join(candidatesDir, "gradle", "current"), 0755)

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	src := &SDKMANSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) < 4 {
		t.Fatalf("expected at least 4 items, got %d", len(items))
	}

	// Count cleanup candidates
	cleanCount := 0
	for _, it := range items {
		if it.Status == model.StatusCleanCandidate {
			cleanCount++
		}
	}
	if cleanCount < 2 {
		t.Errorf("expected at least 2 cleanup candidates, got %d", cleanCount)
	}
}

// --- RunAll with real detection ---

func TestRunAll_DoesNotPanic(t *testing.T) {
	// Ensure RunAll doesn't panic with any platform config
	platforms := []model.PlatformInfo{
		{OS: "linux"},
		{OS: "darwin"},
		{OS: "windows"},
		{OS: "linux", HasApt: true, HasSnap: true, HasDocker: true, HasNpm: true},
	}

	for _, p := range platforms {
		results := RunAll(context.Background(), p, false)
		_ = results // should not panic
	}
}

// --- GoSource without gup (ls GOPATH/bin) ---

func TestGoScan_NoGup(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Mock go env GOPATH
	setMock("go", []string{"env", "GOPATH"}, "/tmp/test-gopath", nil)
	// Mock ls GOPATH/bin
	setMock("ls", []string{"/tmp/test-gopath/bin"}, "gopls\ngocognit\nlefthook\n", nil)

	src := &GoSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasGo: true, HasGup: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestGoScan_NoGup_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("go", []string{"env", "GOPATH"}, "/tmp/test-gopath", nil)
	// Empty ls output
	setMock("ls", []string{"/tmp/test-gopath/bin"}, "", nil)

	src := &GoSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasGo: true, HasGup: false})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Pacman without yay ---

func TestPacmanScan_NoYay(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("pacman", []string{"-Qu"}, "btop 1.3.0 -> 1.5.0\ngit 2.42.0 -> 2.45.0", nil)

	src := &PacmanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasPacman: true, HasYay: false})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "btop" || items[0].Status != model.StatusOutdated {
		t.Errorf("item[0] = %+v", items[0])
	}
}

func TestPacmanScan_NoYay_Empty(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("pacman", []string{"-Qu"}, "", nil)

	src := &PacmanSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasPacman: true, HasYay: false})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

// --- Flatpak fallback ---

func TestFlatpakScan_Fallback(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Output that doesn't match the structured parser
	setMock("flatpak", []string{"update", "--dry-run"}, "Updates available.\n  1. some.app 1.0 2.0 stable\n", nil)

	src := &FlatpakSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

// --- Agent Source ---

func TestAgentScan_SomeInstalled(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Only mock a subset of agents — others won't be found via LookPath
	// which falls through to real exec, so they'll be skipped
	setMock("claude", []string{"--version"}, "2.1.209 (Claude Code)", nil)
	setMock("opencode", []string{"--version"}, "1.17.20", nil)

	src := &AgentSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should work (won't crash)
	_ = items
}

// --- Cargo Source ---

func TestCargoScan_NoCargoInstallUpdate(t *testing.T) {
	src := &CargoSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasCargo: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

// --- SDKMAN edge cases ---

func TestSDKMANScan_NoDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	src := &SDKMANSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item (error), got %d", len(items))
	}
}

// --- Npm JSON parse error ---

func TestNpmScan_ParseError(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"outdated", "-g", "--json"}, "not valid json", nil)

	src := &NpmSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK on parse error, got %+v", items)
	}
}

// --- Winget fallback text parsing ---

func TestWingetScan_TextFallback(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Return nothing from --json (simulating failure), the text fallback runs
	// But we also register the fallback mock
	// The JSON call should fail
	jsonErr := errors.New("winget json failed")
	setMock("winget", []string{"upgrade", "--json"}, "error", jsonErr)
	// The text fallback
	out := `Name   Id          Version    Available   Source
7zip   7zip.7zip   23.01      24.01       winget
git    Git.Git     2.42.0     2.45.0      winget`
	setMock("winget", []string{"upgrade"}, out, nil)

	src := &WingetSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

// --- Docker edge cases ---

func TestDockerScan_Error(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("docker", []string{"info", "--format", "{{.ServerVersion}}"}, "24.0.7", nil)
	setMock("docker", []string{"system", "df"}, "", errors.New("df failed"))

	src := &DockerSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{})
	if len(items) < 1 {
		t.Fatal("expected at least 1 item")
	}
}

// --- EnabledSources with cleanup ---

func TestEnabledSources_WithCleanup(t *testing.T) {
	plat := model.PlatformInfo{
		OS: "darwin", Distro: "macos",
		HasBrew: true, HasMAS: true,
		HasApt: false, HasDocker: true, HasGo: true, HasNpm: true,
		HasSDKMAN: true, HasSnap: false,
	}

	srcs := enabledSources(plat, true)
	// Should include cleanup sources like BrewCleanSource
	foundBrewClean := false
	for _, s := range srcs {
		if _, ok := s.(*BrewCleanSource); ok {
			foundBrewClean = true
		}
	}
	if !foundBrewClean {
		t.Error("expected BrewCleanSource in cleanup sources")
	}
}

// --- RunAll with cleanup ---

func TestRunAll_WithCleanup(t *testing.T) {
	// Limit to one platform to avoid long execution
	plat := model.PlatformInfo{OS: "darwin", HasBrew: true}
	results := RunAll(context.Background(), plat, true)
	// Should return both regular and cleanup sources
	// BrewCleanSource needs du command which may not be mocked — just ensure no panic
	_ = results
}

// Ensure runtime is referenced (for build constraint awareness).
var _ = runtime.GOOS
