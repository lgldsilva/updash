package updater

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

// agentOpenCode is the catalog name of the OpenCode agent item (see
// scanner.agentCatalog). It owns its own update path (`opencode upgrade`), so
// after that command runs we validate the binary is actually healthy.
const agentOpenCode = "OpenCode"

// opencodeHealthTimeout caps the post-update `opencode --version` probe.
const opencodeHealthTimeout = 30 * time.Second

// opencodeReinstallHint is the actionable recovery command surfaced when a
// post-update health check fails (broken launcher stub / missing binary).
const opencodeReinstallHint = "npm install -g --allow-scripts=opencode-ai opencode-ai@latest"

// ensureOpenCodeHealthy validates that `opencode upgrade` left a working
// launcher. An update can report success while the wrapper's postinstall was
// skipped (npm allowScripts) or the binary was replaced by a placeholder stub,
// leaving every later `opencode` invocation broken. Detect that and fail
// explicitly with an actionable message instead of advertising success.
//
// Mirrors ensureClaudeNativeBinary for Claude Code, but opencode-ai ships a
// precompiled binary so there is no postinstall to re-run: we only verify.
func ensureOpenCodeHealthy(ctx context.Context, item *model.Item, res *Result) *Result {
	// If the update itself failed, keep that failure.
	if !res.Success {
		return res
	}

	ver, verErr := openCodeVersionProbe(ctx)
	binPath, binOK := openCodeBinaryOK()
	if verErr == nil && ver != "" && binOK {
		res.Success = true
		res.Error = ""
		item.Status = model.StatusDone
		if res.Output != "" {
			res.Output += "\n"
		}
		res.Output += fmt.Sprintf("updash: opencode healthy (%s at %s)", ver, binPath)
		return res
	}

	// Update reported success but opencode is broken. Fail explicitly.
	res.Success = false
	item.Status = model.StatusError
	detail := openCodeHealthDetail(ver, verErr, binPath, binOK)
	res.Error = detail + " — launcher may be a broken stub; reinstall with `" + opencodeReinstallHint + "`"
	if res.Output != "" {
		res.Output += "\n"
	}
	res.Output += detail
	item.Log = res.Output
	return res
}

func openCodeHealthDetail(ver string, verErr error, binPath string, binOK bool) string {
	switch {
	case verErr != nil:
		return fmt.Sprintf("opencode --version failed: %v", verErr)
	case ver == "":
		return "opencode --version returned no version"
	case !binOK:
		if binPath == "" {
			return "opencode launcher not found on PATH"
		}
		return "opencode launcher not executable: " + binPath
	default:
		return ""
	}
}

// openCodeVersionProbe runs `opencode --version` and returns the trimmed
// stdout (empty on failure). Uses outputRunner so tests can stub the probe.
func openCodeVersionProbe(ctx context.Context) (string, error) {
	pctx, cancel := context.WithTimeout(ctx, opencodeHealthTimeout)
	defer cancel()
	out, err := outputRunner(pctx, "opencode", "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// openCodeBinaryOK resolves the opencode launcher and reports whether it exists
// and is executable (not a broken stub / missing file). Uses lookPath + os.Stat
// so tests can stub both.
func openCodeBinaryOK() (path string, ok bool) {
	p, err := lookPath("opencode")
	if err != nil || p == "" {
		return "", false
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return p, false
	}
	return p, info.Mode()&0o100 != 0 // any execute bit set
}
