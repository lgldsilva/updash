package cli

import (
	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/platform"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

// Test seams (overridden in unit tests). Production defaults call real deps.
var (
	detectPlatform               = platform.Detect
	runScannerAll                = scanner.RunAll
	runScannerFiltered           = scanner.RunAllFiltered
	runScannerFilteredForCleanup = scanner.RunAllFilteredForCleanup
	cleanOneFn                   = cleaner.CleanOne
	updateCategory               = updater.UpdateCategory
	prepareUpdateBatch           = updater.PrepareUpdateBatch
	executePreparedBatch         = updater.ExecutePreparedBatch
	primeMacSudo                 = elevate.PrimeMacOSUserSudo
	canElevateNP                 = elevate.CanElevateWithoutPassword
	nativeMacAvail               = elevate.NativeMacAuthAvailable
	promptMacSess                = elevate.PromptMacPasswordSession
	formatBytesFn                = cleaner.FormatBytes
	stdinIsTTYFn                 = stdinIsTTY
)
