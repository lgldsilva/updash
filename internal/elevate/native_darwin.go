//go:build darwin

package elevate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// NativeMacAuthAvailable reports whether the system authorization sheet can be used.
func NativeMacAuthAvailable() bool { return true }

// PrimeMacOSUserSudo shows the native macOS authentication sheet for the console
// user and runs sudo -v so brew/mas can call sudo internally as that user (not root).
func PrimeMacOSUserSudo(ctx context.Context) error {
	script := `set u to short user name of (system info)
try
  do shell script "sudo -v" user name u password ""
on error errMsg number errNum
  error errMsg number errNum
end try`
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		combined := text + " " + err.Error()
		if strings.Contains(combined, "User canceled") ||
			strings.Contains(combined, "-128") ||
			strings.Contains(strings.ToLower(combined), "cancel") {
			return ErrDialogCancelled
		}
		if text != "" {
			return fmt.Errorf("native auth: %w — %s", err, text)
		}
		return fmt.Errorf("native auth: %w", err)
	}
	return nil
}

// RunPrivilegedScript is deprecated for brew/mas updates; use PrimeMacOSUserSudo.
func RunPrivilegedScript(ctx context.Context, script string) (string, error) {
	_ = script
	if err := PrimeMacOSUserSudo(ctx); err != nil {
		return "", err
	}
	return "", nil
}
