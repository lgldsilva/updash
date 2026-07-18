package upgrade

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const envSkipAutoUpgrade = "UPDASH_SKIP_AUTO_UPGRADE"

// StartupResult summarizes version info and optional self-update.
type StartupResult struct {
	Current string
	Latest  string
	Updated bool
	Note    string // short status: "up to date", "upgraded", error hint
}

// FormatBuild returns version plus OS/arch (e.g. "841d04d (linux/amd64)").
func FormatBuild(version string) string {
	v := version
	if v == "" {
		v = "dev"
	}
	return fmt.Sprintf("%s (%s/%s)", v, runtime.GOOS, runtime.GOARCH)
}

// ShouldAutoUpgrade reports whether startup should try to install a newer release.
func ShouldAutoUpgrade(version string, skipFlag bool) bool {
	if skipFlag || os.Getenv(envSkipAutoUpgrade) == "1" {
		return false
	}
	return true
}

// selfUpdateAllowed reports whether the binary lives in the user-owned install
// location used by install.sh. System and package-manager installs must remain
// immutable so that their manager owns updates and integrity metadata.
func selfUpdateAllowed(executable, home string) bool {
	userBin := filepath.Join(home, ".local", "bin")
	rel, err := filepath.Rel(userBin, executable)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canSelfUpdate() bool {
	if os.Getenv("UPDASH_ALLOW_SELF_UPDATE") == "1" {
		return true
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return selfUpdateAllowed(executable, home)
}

// Startup prints the build line, optionally upgrades, and may re-exec the binary.
func Startup(ctx context.Context, cfg Config, current string, auto bool) (StartupResult, error) {
	res := StartupResult{Current: current}

	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	latest, avail, err := Check(checkCtx, cfg, current)
	res.Latest = latest

	if err != nil {
		res.Note = "upgrade check failed"
		PrintBanner(res)
		return res, err
	}

	if !avail {
		res.Note = "up to date"
		PrintBanner(res)
		return res, nil
	}

	if !auto {
		res.Note = "update available"
		PrintBanner(res)
		return res, nil
	}
	if !canSelfUpdate() {
		res.Note = "package-managed"
		PrintBanner(res)
		return res, nil
	}

	fmt.Printf("↑ update available: %s → %s\n", FormatBuild(current), latest)
	fmt.Println("↓ downloading and installing…")

	installCtx, installCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer installCancel()

	if err := install(installCtx, httpClient(cfg), cfg, latest); err != nil {
		res.Note = "upgrade failed"
		PrintBanner(res)
		return res, err
	}

	res.Updated = true
	res.Note = "upgraded"
	res.Current = latest
	fmt.Printf("✓ upgraded to %s — restarting\n", latest)

	if err := Reexec(); err != nil {
		return res, fmt.Errorf("restart after upgrade: %w", err)
	}
	return res, nil
}

// PrintBanner writes the one-line build/status header to stdout.
func PrintBanner(res StartupResult) {
	line := "updash " + FormatBuild(res.Current)
	switch {
	case res.Updated:
		line += fmt.Sprintf(" · %s", res.Latest)
	case res.Latest != "" && sameVersion(res.Current, res.Latest):
		line += fmt.Sprintf(" · %s · up to date", res.Latest)
	case res.Latest != "":
		line += fmt.Sprintf(" · latest %s", res.Latest)
		if res.Note == "update available" {
			line += " — run: updash --upgrade"
		}
	}
	switch res.Note {
	case "upgrade check failed":
		line += " · upgrade check skipped"
	case "upgrade failed":
		line += " · upgrade failed"
	case "package-managed":
		line += " · update with your package manager"
	}
	fmt.Println(line)
}

// Reexec runs the same binary with the same arguments and exits the current process.
func Reexec() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	// Re-exec the absolute path of this binary with the original argv; args are
	// not shell-interpolated (execve), so this is not shell injection.
	cmd := exec.Command(self, os.Args[1:]...) // #nosec G702 -- re-exec self, not user shell
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// ModeSkipsStartupUpgrade reports CLI modes that must not auto-upgrade or banner before work.
func ModeSkipsStartupUpgrade(mode string) bool {
	switch mode {
	case "version", "help", "upgrade", "check-upgrade", "update-self", "env-defaults":
		return true
	default:
		return false
	}
}

// ModeShowsStartupBanner reports whether to print the build banner on launch.
func ModeShowsStartupBanner(mode string) bool {
	return !ModeSkipsStartupUpgrade(mode) || mode == "version"
}

// NormalizeVersion strips a leading v for display comparisons.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
