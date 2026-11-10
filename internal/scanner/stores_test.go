package scanner

import "testing"

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
