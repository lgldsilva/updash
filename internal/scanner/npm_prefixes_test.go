package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// withoutLegacyNpmPrefixes keeps NpmSource.Scan from probing the developer's
// real ~/.npm-global during unit tests.
func withoutLegacyNpmPrefixes(t *testing.T) {
	t.Helper()
	prev := npmLegacyPrefixCandidates
	npmLegacyPrefixCandidates = func() []string { return nil }
	t.Cleanup(func() { npmLegacyPrefixCandidates = prev })
}

// withLegacyNpmPrefix points the candidate seam at a synthetic legacy prefix
// layout and returns its path.
func withLegacyNpmPrefix(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "lib", "node_modules")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	prev := npmLegacyPrefixCandidates
	npmLegacyPrefixCandidates = func() []string { return []string{dir} }
	t.Cleanup(func() { npmLegacyPrefixCandidates = prev })
	return dir
}

const npmLsLegacyJSON = `{
	"dependencies": {
		"aaa": {"version": "1.0.0"},
		"bbb": {"version": "2.0.0"},
		"ccc": {"version": "3.0.0"}
	}
}`

const npmLsActiveJSON = `{
	"dependencies": {
		"aaa": {"version": "9.0.0"}
	}
}`

// A package whose name is also in the active prefix is shadowed information;
// a legacy-only outdated package is a normal update target pinned to its own
// prefix; a fresh legacy-only package stays invisible, matching the
// active-prefix behaviour.
func TestNpmScan_LegacyPrefixShadowAndOutdated(t *testing.T) {
	legacy := withLegacyNpmPrefix(t)
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"config", "get", "prefix"}, "/real/active-prefix", nil)
	setMock("npm", []string{"ls", "-g", "--json", "--depth=0"}, npmLsActiveJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "aaa"},
		`{"aaa":{"current":"9.0.0","wanted":"9.1.0","latest":"9.1.0"}}`, nil)
	setMock("npm", []string{"ls", "--prefix", legacy, "-g", "--json", "--depth=0"}, npmLsLegacyJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "--prefix", legacy, "aaa", "bbb", "ccc"}, `{
		"aaa": {"current":"1.0.0","wanted":"1.1.0","latest":"1.1.0"},
		"bbb": {"current":"2.0.0","wanted":"2.5.0","latest":"2.5.0"}
	}`, nil)

	items, err := (&NpmSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]*model.Item{}
	for _, it := range items {
		byKey[it.Prefix+"|"+it.Name] = it
	}
	active := byKey["|aaa"]
	if active == nil || active.Status != model.StatusOutdated || active.AvailableVer != "9.1.0" {
		t.Fatalf("active aaa: %+v", active)
	}
	shadow := byKey[legacy+"|aaa"]
	if shadow == nil || shadow.Status != model.StatusInfo || shadow.Log != npmShadowNote || shadow.AvailableVer != "" {
		t.Fatalf("shadowed aaa: %+v", shadow)
	}
	legacyOnly := byKey[legacy+"|bbb"]
	if legacyOnly == nil || legacyOnly.Status != model.StatusOutdated || legacyOnly.AvailableVer != "2.5.0" || legacyOnly.Prefix != legacy {
		t.Fatalf("legacy-only bbb: %+v", legacyOnly)
	}
	if _, fresh := byKey[legacy+"|ccc"]; fresh {
		t.Fatalf("fresh legacy-only ccc must stay invisible: %+v", byKey[legacy+"|ccc"])
	}
}

// A candidate whose canonical path equals the active prefix must not be
// scanned twice (that would duplicate every active item).
func TestNpmScan_LegacyPrefixEqualActiveSkipped(t *testing.T) {
	legacy := withLegacyNpmPrefix(t)
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"config", "get", "prefix"}, legacy, nil)
	setMock("npm", []string{"ls", "-g", "--json", "--depth=0"}, npmLsActiveJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "aaa"}, `{}`, nil)

	items, err := (&NpmSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Prefix != "" {
			t.Fatalf("unexpected legacy item %+v", it)
		}
	}
}

// A failed probe on an existing legacy prefix must surface as unverified — an
// unscanned installation must not read as "all good" — while active results
// survive.
func TestNpmScan_LegacyProbeFailureIsUnverified(t *testing.T) {
	legacy := withLegacyNpmPrefix(t)
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"config", "get", "prefix"}, "/real/active-prefix", nil)
	setMock("npm", []string{"ls", "-g", "--json", "--depth=0"}, npmLsActiveJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "aaa"},
		`{"aaa":{"current":"9.0.0","wanted":"9.1.0","latest":"9.1.0"}}`, nil)
	setMock("npm", []string{"ls", "--prefix", legacy, "-g", "--json", "--depth=0"}, "", context.DeadlineExceeded)

	items, err := (&NpmSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	hasUnverified := false
	hasActive := false
	for _, it := range items {
		if it.Status == model.StatusUnverified && it.Name == "npm prefix "+legacy {
			hasUnverified = true
		}
		if it.Prefix == "" && it.Name == "aaa" {
			hasActive = true
		}
	}
	if !hasUnverified || !hasActive {
		t.Fatalf("want unverified legacy prefix + intact active items, got %+v", items)
	}
}

// A candidate exists but the active prefix probe fails: shadow detection is
// impossible, so the run is honest about it.
func TestNpmScan_ActivePrefixProbeFailureIsUnverified(t *testing.T) {
	withLegacyNpmPrefix(t)
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"config", "get", "prefix"}, "", context.DeadlineExceeded)
	setMock("npm", []string{"ls", "-g", "--json", "--depth=0"}, npmLsActiveJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "aaa"}, `{}`, nil)

	items, err := (&NpmSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Status == model.StatusUnverified && it.Name == "npm legacy prefix" {
			return
		}
	}
	t.Fatalf("expected an unverified legacy-prefix item, got %+v", items)
}

// The active inventory failed, so a successful legacy scan cannot be
// shadow-joined. It must stay visible as unverified, not as an update target
// and not dropped.
func TestNpmScan_ActiveFailureKeepsLegacyUnverified(t *testing.T) {
	legacy := withLegacyNpmPrefix(t)
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"config", "get", "prefix"}, "/real/active-prefix", nil)
	setMock("npm", []string{"ls", "-g", "--json", "--depth=0"}, "", context.DeadlineExceeded)
	setMock("npm", []string{"ls", "--prefix", legacy, "-g", "--json", "--depth=0"}, npmLsLegacyJSON, nil)
	setMock("npm", []string{"outdated", "-g", "--json", "--prefix", legacy, "aaa", "bbb", "ccc"}, `{
		"bbb": {"current":"2.0.0","wanted":"2.5.0","latest":"2.5.0"}
	}`, nil)

	items, err := (&NpmSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	var sawActive, sawLegacy bool
	for _, it := range items {
		if it.Status == model.StatusOutdated {
			t.Fatalf("legacy package promoted to an update target: %+v", it)
		}
		if it.Status == model.StatusError && it.Name == "npm" {
			sawActive = true
		}
		if it.Status == model.StatusUnverified && it.Name == "npm prefix "+legacy {
			sawLegacy = true
			if !strings.Contains(it.Error, "shadow detection skipped") {
				t.Fatalf("legacy cause = %q", it.Error)
			}
		}
	}
	if !sawActive || !sawLegacy {
		t.Fatalf("want active error + unverified legacy, got %+v", items)
	}
}

func TestNpmGlobalRootExists(t *testing.T) {
	dir := t.TempDir()
	if npmGlobalRootExists(filepath.Join(dir, "missing")) {
		t.Fatal("missing prefix must not exist")
	}
	if npmGlobalRootExists("") {
		t.Fatal("empty prefix must not exist")
	}
	unixLayout := filepath.Join(dir, "lib", "node_modules")
	if err := os.MkdirAll(unixLayout, 0o750); err != nil {
		t.Fatal(err)
	}
	if !npmGlobalRootExists(dir) {
		t.Fatal("unix layout not detected")
	}
	winLayout := filepath.Join(t.TempDir(), "node_modules")
	if err := os.MkdirAll(winLayout, 0o750); err != nil {
		t.Fatal(err)
	}
	if !npmGlobalRootExists(filepath.Dir(winLayout)) {
		t.Fatal("windows layout not detected")
	}
}

func TestParseNpmLsGlobalVersions(t *testing.T) {
	out := []byte(`{"dependencies":{"@github/copilot":{"version":"1.0.73"},"pi":{"version":"0.81.1"}}}`)
	m := ParseNpmLsGlobalVersions(out)
	if m["@github/copilot"] != "1.0.73" || m["pi"] != "0.81.1" {
		t.Fatalf("versions = %v", m)
	}
	if ParseNpmLsGlobalVersions([]byte("not json")) != nil {
		t.Fatal("bad json must be nil")
	}
	if ParseNpmLsGlobalVersions([]byte(`{"dependencies":{}}`)) != nil {
		t.Fatal("empty deps must be nil")
	}
}

// Freshness confirmed by a channel that has no npm package (Grok's
// `update --check --json`) must still reach the agent item through the
// registry gate.
func TestResolveRegistryLatestFrom_GrokJSONLatestCmd(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("grok", []string{"update", "--check", "--json"},
		`{"currentVersion":"1.0.40","latestVersion":"1.0.46","updateAvailable":true,"channel":"stable"}`, nil)

	items := []*model.Item{{Name: "Grok", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.0.40"}}
	resolveRegistryLatestFrom(context.Background(), items, agentCatalog(), nil)

	if items[0].Status != model.StatusOutdated || items[0].AvailableVer != "1.0.46" {
		t.Fatalf("grok not flagged via latestCmd: %+v", items[0])
	}
}

func TestResolveRegistryLatestFrom_SkipsUnverified(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("grok", []string{"update", "--check", "--json"}, `{"latestVersion":"9.9.9"}`, nil)

	calls := 0
	prev := execCommand
	execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		return prev(ctx, name, args...)
	}
	defer func() { execCommand = prev }()

	it := &model.Item{Name: "Grok", Category: model.CatAgent, Status: model.StatusUnverified, CurrentVer: "installed", Error: "probe failed"}
	resolveRegistryLatestFrom(context.Background(), []*model.Item{it}, agentCatalog(), nil)
	if it.Status != model.StatusUnverified || it.AvailableVer != "" || it.Error != "probe failed" {
		t.Fatalf("unverified item rewritten: %+v", it)
	}
	if calls != 0 {
		t.Fatalf("unverified item was probed %d times", calls)
	}
}

func TestResolveRegistryLatestFrom_SkipsAffirmativeStates(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// No mocks registered: any probe would fall through to real execution, so
	// the assertions only hold if no probe runs at all.
	items := []*model.Item{
		{Name: "Grok", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "1.0.46"},
		{Name: "Grok", Category: model.CatAgent, Status: model.StatusOutdated, CurrentVer: "1.0.40", AvailableVer: "1.0.46"},
	}
	before := []model.Status{items[0].Status, items[1].Status}
	resolveRegistryLatestFrom(context.Background(), items, agentCatalog(), nil)
	for i := range items {
		if items[i].Status != before[i] {
			t.Fatalf("item[%d] moved %v → %v; affirmative states must not be re-probed", i, before[i], items[i].Status)
		}
	}
}

func TestApplyAgentOutdated_InfoBecomesOKWhenCurrent(t *testing.T) {
	it := &model.Item{Name: "Grok", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.0.46"}
	ApplyAgentOutdated(it, "1.0.46")
	if it.Status != model.StatusOK {
		t.Fatalf("verified-fresh info item must become OK: %+v", it)
	}
	older := &model.Item{Name: "Grok", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.0.40"}
	ApplyAgentOutdated(older, "1.0.46")
	if older.Status != model.StatusOutdated || older.AvailableVer != "1.0.46" {
		t.Fatalf("stale info item must become outdated: %+v", older)
	}
}

func TestMarkNpmManagedFresh(t *testing.T) {
	items := []*model.Item{
		{Name: "Copilot CLI", PackageID: "@github/copilot", Status: model.StatusInfo},
		{Name: "Codex", PackageID: "@openai/codex", Status: model.StatusOutdated},
		{Name: "Grok", Status: model.StatusInfo},
	}
	markNpmManagedFresh(items, map[string]string{"@github/copilot": "1.0.90"})
	if items[0].Status != model.StatusOK {
		t.Fatalf("managed fresh agent must be OK: %+v", items[0])
	}
	if items[1].Status != model.StatusOutdated {
		t.Fatalf("npm-flagged outdated must stay outdated: %+v", items[1])
	}
	if items[2].Status != model.StatusInfo {
		t.Fatalf("agent without npm package must stay info: %+v", items[2])
	}
}

// Agents managed by npm whose package npm considers current must not be
// re-probed against the registry: npm's silence is the evidence.
func TestResolveRegistryLatestFrom_SkipsNpmManaged(t *testing.T) {
	enableMocks()
	defer disableMocks()

	items := []*model.Item{{Name: "Copilot CLI", Category: model.CatAgent, Status: model.StatusInfo, PackageID: "@github/copilot", CurrentVer: "1.0.90"}}
	resolveRegistryLatestFrom(context.Background(), items, agentCatalog(), map[string]bool{"@github/copilot": true})
	if items[0].Status != model.StatusInfo {
		t.Fatalf("npm-managed agent must be skipped by the registry gate: %+v", items[0])
	}
}

func TestAgentUpdateCommand_NewAgents(t *testing.T) {
	if cmd := AgentUpdateCommand("MimoCode"); len(cmd) != 2 || cmd[0] != "mimo" || cmd[1] != "upgrade" {
		t.Fatalf("MimoCode cmd = %v, want [mimo upgrade]", cmd)
	}
	cmd := AgentUpdateCommand("Copilot CLI")
	if len(cmd) == 0 || cmd[0] != "npm" {
		t.Fatalf("Copilot CLI cmd = %v, want the npm-managed fallback", cmd)
	}
	if cmd := AgentUpdateCommand("Crush"); len(cmd) == 0 || cmd[0] != "npm" {
		t.Fatalf("Crush cmd = %v, want the npm-managed fallback", cmd)
	}
	if AgentKeepPolicy("Crush") != "" || AgentKeepPolicy("MimoCode") != "" {
		t.Fatal("auto agents must not expose a keep policy")
	}
}

func TestParseAgentLatest_JSONKey(t *testing.T) {
	def := agentDef{latestJSONKey: "latestVersion"}
	if got := parseAgentLatest(def, `{"latestVersion":"1.0.46","x":1}`); got != "1.0.46" {
		t.Fatalf("json latest = %q", got)
	}
	if got := parseAgentLatest(def, "not json"); got != "" {
		t.Fatalf("bad json latest = %q", got)
	}
	if got := parseAgentLatest(def, `{"latestVersion":42}`); got != "" {
		t.Fatalf("non-string latest = %q", got)
	}
	semverDef := agentDef{}
	if got := parseAgentLatest(semverDef, "grok 1.0.46 (abc) [stable]"); got != "1.0.46" {
		t.Fatalf("semver latest = %q", got)
	}
}
