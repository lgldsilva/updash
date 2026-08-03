package scanner

import "testing"

func TestParsePnpmOutdatedGlobal(t *testing.T) {
	out := []byte(`{
  "/home/u/.local/share/pnpm/global": {},
  "typescript": {"current": "5.4.0", "wanted": "5.5.4", "latest": "5.5.4"},
  "up-to-date-pkg": {"current": "1.0.0", "wanted": "1.0.0", "latest": "1.0.0"}
}`)
	items := ParsePnpmOutdatedGlobal(out)
	if len(items) != 1 {
		t.Fatalf("want 1 item (dir row + up-to-date skipped), got %d", len(items))
	}
	if items[0].Name != "typescript" || items[0].AvailableVer != "5.5.4" {
		t.Errorf("bad item: %+v", items[0])
	}
	if got := ParsePnpmOutdatedGlobal([]byte("nope")); got != nil {
		t.Fatalf("want nil on bad json, got %v", got)
	}
}

func TestParseBunPmLsGlobal(t *testing.T) {
	out := `/home/u/.bun/install/global/node_modules (2)
├── @anthropic-ai/claude-code@1.2.3
└── typescript@5.5.4
`
	items := ParseBunPmLsGlobal(out)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Name != "@anthropic-ai/claude-code" || items[0].CurrentVer != "1.2.3" {
		t.Errorf("scoped pkg parsed wrong: %+v", items[0])
	}
	if items[1].Name != "typescript" || items[1].CurrentVer != "5.5.4" {
		t.Errorf("plain pkg parsed wrong: %+v", items[1])
	}
	if items := ParseBunPmLsGlobal(""); len(items) != 0 {
		t.Fatalf("want 0 items on empty output, got %d", len(items))
	}
}
