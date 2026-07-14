package elevate

import "errors"

// ErrDialogCancelled is returned when the user dismisses the password dialog.
var ErrDialogCancelled = errors.New("password dialog cancelled")

// ErrDialogUnavailable means native password dialogs are not supported on this OS.
var ErrDialogUnavailable = errors.New("native password dialog not available on this platform")
