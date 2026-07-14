package model_test

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status model.Status
		want   string
	}{
		{model.StatusPending, "pending"},
		{model.StatusOK, "ok"},
		{model.StatusOutdated, "outdated"},
		{model.StatusError, "error"},
		{model.StatusUpdating, "updating"},
		{model.StatusDone, "done"},
		{model.StatusCleanCandidate, "clean-candidate"},
		{model.StatusCleaning, "cleaning"},
		{model.StatusCleaned, "cleaned"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status.String() = %q, want %q", got, tt.want)
			}
		})
	}

	// Unknown status
	if got := model.Status(99).String(); got != "unknown" {
		t.Errorf("Status(99).String() = %q, want %q", got, "unknown")
	}
}

func TestTabID_String(t *testing.T) {
	tests := []struct {
		tab  model.TabID
		want string
	}{
		{model.TabUpdates, "Updates"},
		{model.TabCleanup, "Cleanup"},
		{model.TabLogs, "Logs"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tab.String(); got != tt.want {
				t.Errorf("TabID.String() = %q, want %q", got, tt.want)
			}
		})
	}

	if got := model.TabID(99).String(); got != "?" {
		t.Errorf("TabID(99).String() = %q, want %q", got, "?")
	}
}

func TestCategoryConstants(t *testing.T) {
	// Verify all categories have unique string values
	seen := make(map[string]bool)
	categories := []model.Category{
		model.CatBrew, model.CatMAS, model.CatApt, model.CatPacman,
		model.CatFlatpak, model.CatSnap, model.CatWinget, model.CatChoco,
		model.CatScoop, model.CatNpm, model.CatPipx, model.CatGo,
		model.CatRustup, model.CatCargo, model.CatSDKMAN, model.CatDocker,
		model.CatWatchtower, model.CatCloud, model.CatAI, model.CatAgent,
		model.CatGHExt, model.CatNvm, model.CatOmz, model.CatCache,
		model.CatSDKClean, model.CatVSCodeClean, model.CatDockerClean,
	}

	for _, c := range categories {
		if seen[string(c)] {
			t.Errorf("duplicate category: %s", c)
		}
		seen[string(c)] = true
	}
}
