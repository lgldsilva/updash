package scanner

import (
	"strings"
	"testing"
)

// The gcloud freshness probe must not use --only-filter-updates-available:
// modern gcloud removed that flag, and its permanent failure marked the item
// unverified, which used to block the whole --update gate (fail-closed).
// Mutable runs now skip inconclusive items visibly instead, but a
// permanently-failing probe still exits 2, so the flag stays forbidden.
func TestGcloudLatestCmdUsesSupportedFilter(t *testing.T) {
	for _, tool := range aiInfraCatalog() {
		if tool.name != "gcloud" {
			continue
		}
		joined := strings.Join(tool.latestCmd, " ")
		if strings.Contains(joined, "--only-filter-updates-available") {
			t.Fatalf("removed gcloud flag still in use: %v", tool.latestCmd)
		}
		found := false
		for _, arg := range tool.latestCmd {
			if strings.HasPrefix(arg, "--filter=") {
				found = true
			}
		}
		if !found {
			t.Fatalf("gcloud latestCmd must filter on update availability: %v", tool.latestCmd)
		}
		return
	}
	t.Fatal("gcloud entry missing from aiInfraCatalog")
}
