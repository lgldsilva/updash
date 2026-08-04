package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

// agentClaudeCode is the catalog name of the Claude Code agent item.
const agentClaudeCode = "Claude Code"

// claudeStubMarker is the telltale printed by the @anthropic-ai/claude-code
// wrapper stub when the platform-native binary was never placed.
const claudeStubMarker = "native binary not installed"

// claudeInstallScript is the wrapper package's postinstall, relative to the
// global node_modules root.
var claudeInstallScript = filepath.Join("@anthropic-ai", "claude-code", "install.cjs")

// containsStubMarker reports whether CLI output carries the missing-binary
// stub message.
func containsStubMarker(output string) bool {
	return strings.Contains(output, claudeStubMarker)
}

// claudeHealth probes `claude --version` and reports whether the failure (if
// any) is the missing-native-binary stub problem.
func claudeHealth(ctx context.Context) (healthy bool, stubIssue bool) {
	hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(hctx, "claude", "--version").CombinedOutput()
	if err == nil {
		return true, false
	}
	return false, containsStubMarker(string(out))
}

// npmGlobalRoot returns the global node_modules root (`npm root -g`).
func npmGlobalRoot(ctx context.Context) (string, error) {
	nctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(nctx, "npm", "root", "-g").Output()
	if err != nil {
		return "", fmt.Errorf("npm root -g failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// repairClaudeNativeBinary re-runs the @anthropic-ai/claude-code postinstall
// so the platform-native binary is placed over the wrapper's placeholder
// stub. Returns ok plus a diagnostic message.
func repairClaudeNativeBinary(ctx context.Context) (bool, string) {
	root, err := npmGlobalRoot(ctx)
	if err != nil {
		return false, err.Error()
	}
	install := filepath.Join(root, claudeInstallScript)
	if _, err := os.Stat(install); err != nil {
		return false, "claude-code install script not found at " + install
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(rctx, "node", install).CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return false, fmt.Sprintf("postinstall repair failed: %v: %s", err, trimmed)
	}
	return true, trimmed
}

// ensureClaudeNativeBinary self-heals Claude Code after an update.
//
// npm >= 12 blocks install scripts by default unless allowScripts covers the
// package (RFC npm/rfcs#868). An update of @anthropic-ai/claude-code under
// such npm silently skips its postinstall, leaving the wrapper's placeholder
// stub behind — every `claude` invocation then fails with "Error: claude
// native binary not installed", which also breaks subsequent `claude update`
// runs. Detect that state, re-run the package postinstall to place the
// native binary, and retry the update once when it never ran because claude
// was already the broken stub.
func ensureClaudeNativeBinary(ctx context.Context, item *model.Item, res *Result, updateCmd []string, opts Options) *Result {
	healthy, stubIssue := claudeHealth(ctx)
	switch {
	case healthy:
		return res
	case !stubIssue:
		// Some other failure: do not attempt the stub repair, but never
		// report success when `claude` does not run.
		if res.Success {
			res.Success = false
			res.Error = "claude --version failed after update"
			item.Status = model.StatusError
		}
		return res
	}

	if ok, detail := repairClaudeNativeBinary(ctx); !ok {
		res.Success = false
		res.Error = "claude native binary missing and repair failed: " + detail
		item.Status = model.StatusError
		item.Log = detail
		return res
	}

	// The update command itself only fails with the stub error when claude
	// was already broken before this run; retry it now that the binary is
	// back so the actual upgrade still happens.
	if !res.Success {
		res = runCmd(ctx, item, opts, updateCmd[0], updateCmd[1:]...)
		if _, stillStub := claudeHealth(ctx); stillStub {
			// The retry re-broke it (npm skipped the postinstall again):
			// heal once more.
			if ok, detail := repairClaudeNativeBinary(ctx); !ok {
				res.Success = false
				res.Error = "claude native binary repair after retry failed: " + detail
				item.Status = model.StatusError
				item.Log = detail
				return res
			}
		}
	}

	if healthy, _ := claudeHealth(ctx); healthy {
		res.Success = true
		res.Error = ""
		item.Status = model.StatusDone
		if res.Output != "" {
			res.Output += "\n"
		}
		res.Output += "updash: repaired claude native binary (re-ran postinstall skipped by npm allowScripts)"
		return res
	}
	res.Success = false
	res.Error = "claude still fails after native-binary repair"
	item.Status = model.StatusError
	return res
}
