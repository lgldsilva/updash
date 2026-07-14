//go:build !darwin

package elevate

import "context"

// NativeMacAuthAvailable is false off macOS.
func NativeMacAuthAvailable() bool { return false }

// PrimeMacOSUserSudo is unavailable off macOS.
func PrimeMacOSUserSudo(ctx context.Context) error {
	_ = ctx
	return ErrDialogUnavailable
}

// RunPrivilegedScript is unavailable off macOS.
func RunPrivilegedScript(ctx context.Context, script string) (string, error) {
	_ = ctx
	_ = script
	return "", ErrDialogUnavailable
}
