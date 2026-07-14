//go:build !darwin

package elevate

import (
	"context"
)

// PromptMacPassword is only available on macOS.
func PromptMacPassword(reason string) (string, error) {
	return "", ErrDialogUnavailable
}

// PromptMacPasswordSession is only available on macOS.
func PromptMacPasswordSession(ctx context.Context, reason string) (*Session, error) {
	return nil, ErrDialogUnavailable
}
