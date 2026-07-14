package updater

import (
	"context"
	"time"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// BatchTimeout returns the max duration for a category batch.
func BatchTimeout(cat model.Category) time.Duration {
	switch cat {
	case model.CatBrew:
		return 25 * time.Minute
	case model.CatMAS, model.CatApt, model.CatPacman:
		return 30 * time.Minute
	case model.CatWinget, model.CatChoco:
		return 30 * time.Minute
	case model.CatAgent:
		return 20 * time.Minute
	default:
		return 15 * time.Minute
	}
}

func withBatchTimeout(ctx context.Context, cat model.Category) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, BatchTimeout(cat))
}

// BrewItemTimeout caps a single brew upgrade so PKG installers fail fast with a diagnosis.
func BrewItemTimeout(name string) time.Duration {
	if scanner.BrewUpgradeNote(name) != "" {
		return 6 * time.Minute
	}
	return 12 * time.Minute
}
