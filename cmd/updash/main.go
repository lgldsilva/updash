package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/cli"
	"github.com/lgldsilva/updash/internal/config"
	"github.com/lgldsilva/updash/internal/tui"
	"github.com/lgldsilva/updash/internal/upgrade"
)

// Injected at build time via ldflags: -X main.version=<tag>
var version = "dev"

// Bubble Tea model wrapper.
type bubbleModel struct {
	state   *tui.State
	program *tea.Program
}

func main() {
	mode, cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "✘ %v\n", err)
		os.Exit(2)
	}
	ctx := context.Background()
	startupRes := runStartup(ctx, mode, cfg)
	if code := runMode(ctx, mode, cfg, startupRes); code != 0 {
		os.Exit(code)
	}
}

// runMode dispatches CLI/TUI modes. Returns process exit code (0 = ok).
func runMode(ctx context.Context, mode string, cfg cli.Config, startupRes upgrade.StartupResult) int {
	switch mode {
	case "check":
		return exitOnErr(cli.RunCheck(ctx))
	case "update":
		return exitOnUpdateClean(cli.RunUpdate(ctx, cfg))
	case "clean":
		return exitOnUpdateClean(cli.RunClean(ctx, cfg))
	case "all":
		return exitOnErr(cli.RunAll(ctx, cfg))
	case "help":
		printHelp()
	case "version":
		fmt.Println("updash", upgrade.FormatBuild(version))
	case "env-defaults":
		fmt.Print(config.EnvDefaults())
	case "upgrade":
		return exitUpgrade(ctx, false)
	case "check-upgrade":
		return exitUpgrade(ctx, true)
	case "update-self":
		updateSelf()
	default: // tui
		runTUI(startupRes)
	}
	return 0
}

func exitOnErr(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

func exitOnUpdateClean(_ int, fail int, err error) int {
	if err != nil || fail > 0 {
		return 1
	}
	return 0
}

func exitUpgrade(ctx context.Context, checkOnly bool) int {
	c := upgrade.EffectiveConfig()
	c.CheckOnly = checkOnly
	if err := upgrade.Run(ctx, c, version); err != nil {
		if checkOnly {
			fmt.Fprintf(os.Stderr, "✘ %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "✘ upgrade: %v\n", err)
		}
		return 1
	}
	return 0
}

func runStartup(ctx context.Context, mode string, cfg cli.Config) upgrade.StartupResult {
	res := upgrade.StartupResult{Current: version}
	if upgrade.ModeSkipsStartupUpgrade(mode) {
		return res
	}
	uCfg := upgrade.EffectiveConfig()
	auto := upgrade.ShouldAutoUpgrade(version, cfg.SkipAutoUpgrade)
	out, err := upgrade.Startup(ctx, uCfg, version, auto)
	if err == nil {
		return out
	}
	// Startup prints banner on check/install errors; continue with current binary.
	if out.Current != "" {
		return out
	}
	return res
}

func parseArgs(args []string) (mode string, cfg cli.Config, err error) {
	cfg.Verbose = true
	mode = "tui"

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--check", "-c":
			mode = "check"
		case "--update":
			mode = "update"
		case "--clean":
			mode = "clean"
		case "--all", "-a":
			mode = "all"
		case "--help", "-h":
			mode = "help"
		case "--version", "-v":
			mode = "version"
		case "--env-defaults":
			mode = "env-defaults"
		case "--update-self", "-u":
			mode = "update-self"
		case "--upgrade":
			mode = "upgrade"
		case "--check-upgrade":
			mode = "check-upgrade"
		case "--dry-run":
			cfg.DryRun = true
		case "--only":
			if i+1 >= len(args) {
				return "", cfg, fmt.Errorf("--only requires a category name")
			}
			i++
			cfg.Only = args[i]
		case "--quiet", "-q":
			cfg.Verbose = false
		case "--verbose":
			cfg.Verbose = true
		case "--skip-password":
			cfg.SkipPassword = true
		case "--skip-auto-upgrade":
			cfg.SkipAutoUpgrade = true
		case "--strict":
			cfg.Strict = true
		default:
			return "", cfg, fmt.Errorf("unknown argument: %s (try --help)", arg)
		}
	}
	return mode, cfg, nil
}

func runTUI(startupRes upgrade.StartupResult) {
	state := tui.NewWithVersion(startupRes.Current, startupRes.Latest)
	m := &bubbleModel{state: state}

	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	m.state.SetProgram(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func (m *bubbleModel) Init() tea.Cmd {
	return tea.Batch(m.state.HandleAction(tui.KeyRefresh), tui.TickCmd())
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	case tui.TickMsg:
		return m.onTick()
	case tui.ScanSourceDoneMsg:
		return m.onScanSourceDone(msg)
	case tui.ScanFinishedMsg:
		return m.onScanFinished(msg)
	case tui.ErrMsg:
		return m.onErr(msg)
	case tui.UpdateBatchDoneMsg:
		return m.onUpdateBatch(msg)
	case tui.UpdateAllDoneMsg:
		return m.onUpdateAllDone(msg)
	case tui.CleanBatchDoneMsg:
		return m.onCleanBatch(msg)
	case tui.CleanAllDoneMsg:
		return m.onCleanAllDone(msg)
	case tui.OutputLineMsg:
		return m.onOutputLine(msg)
	case tui.ElevRequiredMsg:
		return m.onElevRequired(msg)
	case tui.PasswordResultMsg:
		return m.onPasswordResult(msg)
	default:
		return m, nil
	}
}

func (m *bubbleModel) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.state.Width = msg.Width
	m.state.Height = msg.Height
	m.state.LogWindowSize(msg.Width, msg.Height)
	return m, nil
}

func (m *bubbleModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.state.HandleKey(msg.String())
	switch action {
	case tui.KeyQuit:
		m.state.Cancel()
		m.state.ClearElevation()
		return m, tea.Quit
	case tui.KeyConfirm:
		return m, m.state.ConsumeConfirmCmd(m.program)
	case tui.KeyCancel:
		return m, nil
	default:
		cmd := m.state.HandleAction(action)
		if cmd == nil && m.state.NeedsSpinner() {
			return m, tui.TickCmd()
		}
		return m, cmd
	}
}

func (m *bubbleModel) onTick() (tea.Model, tea.Cmd) {
	if m.state.NeedsSpinner() {
		m.state.AdvanceSpinner()
		return m, tui.TickCmd()
	}
	return m, nil
}

func (m *bubbleModel) onScanSourceDone(msg tui.ScanSourceDoneMsg) (tea.Model, tea.Cmd) {
	if msg.IsCleanup {
		m.state.CleanItems = tui.MergeSummary(m.state.CleanItems, msg.Summary)
	} else {
		m.state.Summaries = tui.MergeSummary(m.state.Summaries, msg.Summary)
	}
	m.state.ScanDone++
	m.state.OperationLabel = msg.Summary.Label
	return m, tui.TickCmd()
}

func (m *bubbleModel) onScanFinished(msg tui.ScanFinishedMsg) (tea.Model, tea.Cmd) {
	m.state.Scanning = false
	m.state.OperationLabel = ""
	m.state.ClampCursor()
	elapsed := ""
	if msg.Elapsed > 0 {
		elapsed = fmt.Sprintf(" (%s)", msg.Elapsed)
	}
	if errs := m.state.TotalScanErrors(); errs > 0 {
		m.state.LogScanErrors()
		m.state.LastSummary = fmt.Sprintf("⚠ Scan done — %d error(s), see Logs tab", errs)
		m.state.AddLog(fmt.Sprintf("Scan complete: %d outdated, %d cleanable, %d error(s)%s",
			m.state.TotalOutdated(), m.state.TotalCleanable(), errs, elapsed), false)
	} else {
		m.state.LastSummary = ""
		m.state.AddLog(fmt.Sprintf("Scan complete: %d outdated, %d cleanable%s",
			m.state.TotalOutdated(), m.state.TotalCleanable(), elapsed), true)
	}
	return m, nil
}

func (m *bubbleModel) onErr(msg tui.ErrMsg) (tea.Model, tea.Cmd) {
	m.state.Error = msg.Error.Error()
	m.state.Scanning = false
	return m, nil
}

func truncateErr(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func (m *bubbleModel) onUpdateBatch(msg tui.UpdateBatchDoneMsg) (tea.Model, tea.Cmd) {
	m.state.UpdateDone = msg.Done
	m.state.UpdateTotal = msg.Total
	if msg.Results == nil && msg.Category != "" {
		m.state.OperationLabel = msg.Category
		m.state.AddLog(fmt.Sprintf("⟳ %s: updating...", msg.Category), true)
		return m, tui.TickCmd()
	}
	for _, r := range msg.Results {
		if r.Success {
			m.state.AddLog(fmt.Sprintf("✓ %s: updated", r.Item.Name), true)
			continue
		}
		m.state.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, truncateErr(r.Error)), false)
	}
	return m, nil
}

func (m *bubbleModel) onUpdateAllDone(msg tui.UpdateAllDoneMsg) (tea.Model, tea.Cmd) {
	m.state.Updating = false
	m.state.OperationLabel = ""
	m.state.LastSummary = fmt.Sprintf("✓ Update done: %d ok, %d failed of %d",
		msg.Success, msg.Failed, msg.Total)
	m.state.AddLog(fmt.Sprintf("Update complete: %d ok, %d failed of %d",
		msg.Success, msg.Failed, msg.Total), msg.Failed == 0)
	m.state.ClampCursor()
	return m, tui.TickCmd()
}

func (m *bubbleModel) onCleanBatch(msg tui.CleanBatchDoneMsg) (tea.Model, tea.Cmd) {
	m.state.CleanDone = msg.Done
	m.state.CleanTotal = msg.Total
	if msg.Results == nil && msg.Category != "" {
		m.state.OperationLabel = msg.Category
		m.state.AddLog(fmt.Sprintf("⟳ %s: cleaning...", msg.Category), true)
		return m, tui.TickCmd()
	}
	for _, r := range msg.Results {
		if !r.Success {
			m.state.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, truncateErr(r.Error)), false)
			continue
		}
		if r.BytesFreed > 0 {
			m.state.AddLog(fmt.Sprintf("✓ %s: freed %s", r.Item.Name, cleaner.FormatBytes(r.BytesFreed)), true)
		} else {
			m.state.AddLog(fmt.Sprintf("✓ %s: nothing to remove", r.Item.Name), true)
		}
	}
	return m, nil
}

func (m *bubbleModel) onCleanAllDone(msg tui.CleanAllDoneMsg) (tea.Model, tea.Cmd) {
	m.state.Cleaning = false
	m.state.OperationLabel = ""
	if msg.BytesFreed > 0 {
		freed := cleaner.FormatBytes(msg.BytesFreed)
		m.state.LastSummary = fmt.Sprintf("✓ Cleanup complete — %s freed", freed)
		m.state.AddLog(fmt.Sprintf("Cleanup complete — %s freed", freed), true)
	} else {
		m.state.LastSummary = "✓ Cleanup complete — nothing to remove"
		m.state.AddLog("Cleanup complete — nothing to remove", true)
	}
	return m, nil
}

func (m *bubbleModel) onOutputLine(msg tui.OutputLineMsg) (tea.Model, tea.Cmd) {
	line := msg.Line
	if len(line) > 72 {
		line = line[:72] + "…"
	}
	m.state.OperationLabel = line
	return m, tui.TickCmd()
}

func (m *bubbleModel) onElevRequired(msg tui.ElevRequiredMsg) (tea.Model, tea.Cmd) {
	m.state.ShowPassword = true
	m.state.PasswordInput = ""
	m.state.PasswordError = ""
	if msg.Reason != "" {
		m.state.LastSummary = msg.Reason
	}
	return m, tui.TickCmd()
}

func (m *bubbleModel) onPasswordResult(msg tui.PasswordResultMsg) (tea.Model, tea.Cmd) {
	if msg.OK {
		return m, m.state.HandlePasswordOK(msg.Session, m.program)
	}
	m.state.PasswordError = msg.Error
	m.state.ShowPassword = true
	return m, nil
}

func (m *bubbleModel) View() string {
	return m.state.Render()
}

func printHelp() {
	fmt.Print(`updash — System Update Dashboard  (macOS / Linux / Windows)

Usage:
  updash                      Interactive TUI dashboard
  updash --check, -c          Scan and show outdated packages
  updash --update             Update outdated packages (CLI, live output)
  updash --clean              Run cleanup operations (CLI)
  updash --all, -a            Update + clean everything
  updash --upgrade            Self-update from latest Gitea release
  updash --check-upgrade      Check for self-update without installing
  updash --version, -v        Show version
  updash --env-defaults       Print UPDASH_* retention vars (effective values)
  updash --update-self, -u    Update updash via git pull + rebuild (dev)
  updash --help, -h           Show this help

Options (CLI modes):
  --only <category>           Limit to one source (brew, mas, npm, docker, …)
  --dry-run                   Show what would run without executing
  --quiet, -q                 Hide command output (errors still shown)
  --verbose                   Force live command output (default on TTY)
  --skip-password             Skip updates that need sudo (no macOS dialog)
  --skip-auto-upgrade         Skip release self-update on startup
  --strict                    Exit non-zero if anything stays outdated

Docker cleanup age defaults to 336h (14d). Override with UPDASH_DOCKER_IMAGE_MAX_AGE,
UPDASH_DOCKER_BUILDER_MAX_AGE, UPDASH_DOCKER_CONTAINER_MAX_AGE (e.g. 168h for 7d).
See --env-defaults for the full list.

On startup (TUI and --check/--update/--clean/--all), updash prints its build
version and checks the Gitea release API. When a newer release exists, it
downloads, verifies, and reinstalls itself before scanning.

Examples:
  updash --check
  updash --all                Scan + update + clean (macOS password dialog when needed)
  updash --update --only brew
  updash --clean --dry-run
  updash --clean --only brew
  updash --clean --only docker
  updash --all

Package managers by platform:
  macOS:   brew, mas, npm, pipx, Go, Rust, SDKMAN, Docker, AI agents
  Linux:   apt, pacman/yay, flatpak, snap, brew, npm, pipx, Go, Rust,
           SDKMAN, Docker, AI agents
  Windows: winget, choco, scoop, npm, pipx, Go, Rust, Docker, AI agents
`)
}

const (
	// Binary name used for build output and install path under repo/home.
	updashBinary = "/updash"
)

func updateSelf() {
	home, _ := os.UserHomeDir()
	repoDir := home + "/.config/updash"
	binOut := repoDir + updashBinary

	fmt.Println("📦 Updating updash itself...")

	cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠ git pull failed (not a git repo?): %v\n", err)
	}

	build := exec.Command("go", "build", "-o", binOut, repoDir+"/cmd/updash/")
	build.Dir = repoDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Printf("✘ Build failed: %v\n", err)
		os.Exit(1)
	}

	installDir := home + "/.local/bin"
	if err := os.MkdirAll(installDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create install dir: %v\n", err)
		os.Exit(1)
	}
	copyCmd := exec.Command("cp", binOut, installDir+updashBinary)
	if err := copyCmd.Run(); err != nil {
		fmt.Printf("✘ Install failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ updash updated!")
}
