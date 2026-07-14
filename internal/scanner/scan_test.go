package scanner

import (
	"context"
	"errors"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

type stubSource struct {
	cat   model.Category
	label string
	icon  string
	items []*model.Item
	err   error
}

func (s stubSource) Category() model.Category { return s.cat }
func (s stubSource) Label() string            { return s.label }
func (s stubSource) Icon() string             { return s.icon }
func (s stubSource) Scan(context.Context, model.PlatformInfo) ([]*model.Item, error) {
	return s.items, s.err
}

func TestIsCleanupCategory(t *testing.T) {
	if !IsCleanupCategory(model.CatCache) {
		t.Fatal("cache should be cleanup")
	}
	if IsCleanupCategory(model.CatBrew) {
		t.Fatal("brew should not be cleanup")
	}
}

func TestSourceTimeout(t *testing.T) {
	if SourceTimeout(model.CatBrew) < SourceTimeout(model.CatApt) {
		t.Fatal("brew should allow longer than apt")
	}
}

func TestIsCleanupCategory_All(t *testing.T) {
	cleanup := []model.Category{
		model.CatCache, model.CatSDKMAN, model.CatSDKClean, model.CatDockerClean, model.CatVSCodeClean,
	}
	for _, cat := range cleanup {
		if !IsCleanupCategory(cat) {
			t.Fatalf("%s should be cleanup", cat)
		}
	}
	if IsCleanupCategory(model.CatBrew) {
		t.Fatal("brew is not cleanup")
	}
}

func TestEnabledSources_Darwin(t *testing.T) {
	plat := model.PlatformInfo{OS: "darwin", HasBrew: true, HasMAS: true, HasNpm: true}
	srcs := EnabledSources(plat, true)
	if len(srcs) < 3 {
		t.Fatalf("expected several sources, got %d", len(srcs))
	}
}

func TestScanSource_BuildSummary(t *testing.T) {
	src := stubSource{
		cat:   model.CatCache,
		label: "Cache",
		icon:  "🧹",
		items: []*model.Item{
			{Status: model.StatusCleanCandidate, Reclaimable: "1G"},
			{Status: model.StatusOK},
			{Status: model.StatusError},
		},
		err: errors.New("scan warning"),
	}
	sum := ScanSource(context.Background(), src, model.PlatformInfo{})
	if sum.Total != 3 || sum.Outdated != 1 || sum.OK != 1 || sum.ErrorCount < 2 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	if sum.Reclaimable != "1G" {
		t.Fatalf("reclaimable = %q", sum.Reclaimable)
	}
}

func TestScanSource_ReclaimableMerge(t *testing.T) {
	src := stubSource{
		cat: model.CatCache,
		items: []*model.Item{
			{Status: model.StatusCleanCandidate, Reclaimable: "1G"},
			{Status: model.StatusCleanCandidate, Reclaimable: "2G"},
		},
	}
	sum := ScanSource(context.Background(), src, model.PlatformInfo{})
	if sum.Reclaimable != "1G + 2G" {
		t.Fatalf("reclaimable = %q", sum.Reclaimable)
	}
}
