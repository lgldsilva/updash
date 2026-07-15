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

	switch mode {
	case "check":
		if err := cli.RunCheck(ctx); err != nil {
			os.Exit(1)
		}
		return
	case "update":
		_, fail, err := cli.RunUpdate(ctx, cfg)
		if err != nil {
			os.Exit(1)
		}
		if fail > 0 {
			os.Exit(1)
		}
		return
	case "clean":
		_, fail, err := cli.RunClean(ctx, cfg)
		if err != nil {
			os.Exit(1)
		}
		if fail > 0 {
			os.Exit(1)
		}
		return
	case "all":
		if err := cli.RunAll(ctx, cfg); err != nil {
			os.Exit(1)
		}
		return
	case "help":
		printHelp()
		return
	case "version":
		fmt.Println("updash", upgrade.FormatBuild(version))
		return
	case "env-defaults":
		fmt.Print(config.EnvDefaults())
		return
	case "upgrade":
		c := upgrade.EffectiveConfig()
		if err := upgrade.Run(ctx, c, version); err != nil {
			fmt.Fprintf(os.Stderr, "✘ upgrade: %v\n", err)
			os.Exit(1)
		}
		return
	case "check-upgrade":
		c := upgrade.EffectiveConfig()
		c.CheckOnly = true
		if err := upgrade.Run(ctx, c, version); err != nil {
			fmt.Fprintf(os.Stderr, "✘ %v\n", err)
			os.Exit(1)
		}
		return
	case "update-self":
		updateSelf()
		return
	case "tui":
		runTUI(startupRes)
	}
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
		m.state.Width = msg.Width
		m.state.Height = msg.Height
		m.state.LogWindowSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		action := m.state.HandleKey(msg.String())
		switch action {
		case tui.KeyQuit:
			m.state.Cancel()
			m.state.ClearElevation()
			return m, tea.Quit
		case tui.KeyConfirm:
			cmd := m.state.ConsumeConfirmCmd(m.program)
			return m, cmd
		case tui.KeyCancel:
			return m, nil
		default:
			cmd := m.state.HandleAction(action)
			if cmd == nil && m.state.NeedsSpinner() {
				return m, tui.TickCmd()
			}
			return m, cmd
		}

	case tui.TickMsg:
		if m.state.NeedsSpinner() {
			m.state.AdvanceSpinner()
			return m, tui.TickCmd()
		}
		return m, nil

	case tui.ScanSourceDoneMsg:
		if msg.IsCleanup {
			m.state.CleanItems = tui.MergeSummary(m.state.CleanItems, msg.Summary)
		} else {
			m.state.Summaries = tui.MergeSummary(m.state.Summaries, msg.Summary)
		}
		m.state.ScanDone++
		m.state.OperationLabel = msg.Summary.Label
		return m, tui.TickCmd()

	case tui.ScanFinishedMsg:
		m.state.Scanning = false
		m.state.OperationLabel = ""
		m.state.ClampCursor()
		elapsed := ""
		if msg.Elapsed > 0 {
			elapsed = fmt.Sprintf(" (%s)", msg.Elapsed)
		}
		scanErrs := m.state.TotalScanErrors()
		if scanErrs > 0 {
			m.state.LogScanErrors()
			m.state.LastSummary = fmt.Sprintf("⚠ Scan done — %d error(s), see Logs tab", scanErrs)
			m.state.AddLog(
				fmt.Sprintf("Scan complete: %d outdated, %d cleanable, %d error(s)%s",
					m.state.TotalOutdated(), m.state.TotalCleanable(), scanErrs, elapsed), false,
			)
		} else {
			m.state.LastSummary = ""
			m.state.AddLog(
				fmt.Sprintf("Scan complete: %d outdated, %d cleanable%s",
					m.state.TotalOutdated(), m.state.TotalCleanable(), elapsed), true,
			)
		}
		return m, nil

	case tui.ErrMsg:
		m.state.Error = msg.Error.Error()
		m.state.Scanning = false
		return m, nil

	case tui.UpdateBatchDoneMsg:
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
			} else {
				errMsg := r.Error
				if len(errMsg) > 120 {
					errMsg = errMsg[:120] + "..."
				}
				m.state.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, errMsg), false)
			}
		}
		return m, nil

	case tui.UpdateAllDoneMsg:
		m.state.Updating = false
		m.state.OperationLabel = ""
		m.state.LastSummary = fmt.Sprintf("✓ Update done: %d ok, %d failed of %d",
			msg.Success, msg.Failed, msg.Total)
		m.state.AddLog(fmt.Sprintf("Update complete: %d ok, %d failed of %d",
			msg.Success, msg.Failed, msg.Total), msg.Failed == 0)
		m.state.ClampCursor()
		return m, tui.TickCmd()

	case tui.CleanBatchDoneMsg:
		m.state.CleanDone = msg.Done
		m.state.CleanTotal = msg.Total
		if msg.Results == nil && msg.Category != "" {
			m.state.OperationLabel = msg.Category
			m.state.AddLog(fmt.Sprintf("⟳ %s: cleaning...", msg.Category), true)
			return m, tui.TickCmd()
		}
		for _, r := range msg.Results {
			if r.Success {
				if r.BytesFreed > 0 {
					m.state.AddLog(fmt.Sprintf("✓ %s: freed %s", r.Item.Name, cleaner.FormatBytes(r.BytesFreed)), true)
				} else {
					m.state.AddLog(fmt.Sprintf("✓ %s: nothing to remove", r.Item.Name), true)
				}
			} else {
				errMsg := r.Error
				if len(errMsg) > 120 {
					errMsg = errMsg[:120] + "..."
				}
				m.state.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, errMsg), false)
			}
		}
		return m, nil

	case tui.CleanAllDoneMsg:
		m.state.Cleaning = false
		m.state.OperationLabel = ""
		if msg.BytesFreed > 0 {
			m.state.LastSummary = fmt.Sprintf("✓ Cleanup complete — %s freed", cleaner.FormatBytes(msg.BytesFreed))
			m.state.AddLog(fmt.Sprintf("Cleanup complete — %s freed", cleaner.FormatBytes(msg.BytesFreed)), true)
		} else {
			m.state.LastSummary = "✓ Cleanup complete — nothing to remove"
			m.state.AddLog("Cleanup complete — nothing to remove", true)
		}
		return m, nil

	case tui.OutputLineMsg:
		line := msg.Line
		if len(line) > 72 {
			line = line[:72] + "…"
		}
		m.state.OperationLabel = line
		return m, tui.TickCmd()

	case tui.ElevRequiredMsg:
		m.state.ShowPassword = true
		m.state.PasswordInput = ""
		m.state.PasswordError = ""
		if msg.Reason != "" {
			m.state.LastSummary = msg.Reason
		}
		return m, tui.TickCmd()

	case tui.PasswordResultMsg:
		if msg.OK {
			cmd := m.state.HandlePasswordOK(msg.Session, m.program)
			return m, cmd
		}
		m.state.PasswordError = msg.Error
		m.state.ShowPassword = true
		return m, nil
	}

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

func updateSelf() {
	home, _ := os.UserHomeDir()
	repoDir := home + "/.config/updash"

	fmt.Println("📦 Updating updash itself...")

	cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠ git pull failed (not a git repo?): %v\n", err)
	}

	build := exec.Command("go", "build", "-o", repoDir+"/updash", repoDir+"/cmd/updash/")
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
	copyCmd := exec.Command("cp", repoDir+"/updash", installDir+"/updash")
	if err := copyCmd.Run(); err != nil {
		fmt.Printf("✘ Install failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ updash updated!")
}
