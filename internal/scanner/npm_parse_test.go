package scanner

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestParseNpmOutdatedJSON(t *testing.T) {
	raw := []byte(`{
		"foo": {"current":"1.0.0","wanted":"1.1.0","latest":"2.0.0"},
		"bar": {"current":"0.1.0","wanted":"0.2.0","latest":""}
	}`)
	items := ParseNpmOutdatedJSON(raw, model.CatNpm)
	if len(items) != 2 {
		t.Fatalf("items=%d want 2", len(items))
	}
	byName := map[string]*model.Item{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if byName["foo"].AvailableVer != "2.0.0" || byName["foo"].Status != model.StatusOutdated {
		t.Fatalf("foo=%+v", byName["foo"])
	}
	if byName["bar"].AvailableVer != "0.2.0" {
		t.Fatalf("bar avail=%q want wanted fallback", byName["bar"].AvailableVer)
	}
}

func TestParseNpmOutdatedJSON_empty(t *testing.T) {
	if got := ParseNpmOutdatedJSON([]byte(`{}`), model.CatNpm); len(got) != 0 {
		t.Fatalf("empty map: %v", got)
	}
	if got := ParseNpmOutdatedJSON([]byte(`not-json`), model.CatOpenCodePlugins); len(got) != 0 {
		t.Fatalf("invalid json: %v", got)
	}
	if got := ParseNpmOutdatedJSON(nil, model.CatNpm); len(got) != 0 {
		t.Fatalf("nil: %v", got)
	}
}

// Regression: a successful command with malformed JSON was previously
// converted to an OK placeholder, producing a false-green scan.
func TestNpmScan_MalformedJSONIsError(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("npm", []string{"outdated", "-g", "--json"}, "not valid json", nil)

	items, err := (&NpmSource{}).Scan(t.Context(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("malformed JSON must be an error, got %+v", items)
	}
}

func TestPnpmScan_MalformedJSONIsError(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("pnpm", []string{"outdated", "-g", "--json"}, "not valid json", nil)

	items, err := (&PnpmSource{}).Scan(t.Context(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("malformed JSON must be an error, got %+v", items)
	}
}

func TestInventorySourcesAreInfoAndPipxBadJSONIsError(t *testing.T) {
	if got := ParseBunPmLsGlobal("└── typescript@5.5.4"); len(got) != 1 || got[0].Status != model.StatusInfo {
		t.Fatalf("bun inventory must be info, got %+v", got)
	}
	enableMocks()
	defer disableMocks()
	setMock("pipx", []string{"list", "--json"}, "not json", nil)
	items, err := (&PipxSource{}).Scan(t.Context(), model.PlatformInfo{})
	if err != nil || len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("pipx bad JSON=%+v err=%v", items, err)
	}
}

func TestParseNpmOutdatedMap(t *testing.T) {
	raw := []byte(`{"@openai/codex":{"current":"0.1.0","wanted":"0.2.0","latest":"0.3.0"}}`)
	m := ParseNpmOutdatedMap(raw)
	if m["@openai/codex"] != "0.3.0" {
		t.Fatalf("map=%v", m)
	}
	if ParseNpmOutdatedMap([]byte(`{`)) != nil {
		t.Fatal("invalid should be nil")
	}
}
