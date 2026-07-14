package cleaner

import (
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

// ItemTimeout returns the max duration for cleaning one item.
func ItemTimeout(item *model.Item) time.Duration {
	switch item.Category {
	case model.CatDockerClean:
		return 30 * time.Minute
	case model.CatSDKMAN, model.CatSDKClean:
		return 15 * time.Minute
	case model.CatVSCodeClean:
		return 10 * time.Minute
	default:
		return 10 * time.Minute
	}
}
