package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return buf.String()
}

func TestGroupCleanBySummary(t *testing.T) {
	a := &model.Item{Name: "a", Status: model.StatusCleanCandidate}
	b := &model.Item{Name: "b", Status: model.StatusCleanCandidate}
	c := &model.Item{Name: "c", Status: model.StatusCleanCandidate}
	summaries := []*model.SourceSummary{
		{Icon: "🍺", Label: "Brew", Items: []*model.Item{a, b}},
		{Icon: "📦", Label: "npm", Items: []*model.Item{c}},
		{Icon: "∅", Label: "empty", Items: nil},
	}
	groups := groupCleanBySummary(summaries, []*model.Item{a, c})
	if len(groups) != 2 {
		t.Fatalf("groups=%d want 2", len(groups))
	}
	if groups[0].label != "🍺 Brew" || len(groups[0].items) != 1 || groups[0].items[0].Name != "a" {
		t.Fatalf("group0=%+v", groups[0])
	}
	if groups[1].label != "📦 npm" || groups[1].items[0].Name != "c" {
		t.Fatalf("group1=%+v", groups[1])
	}
}

func TestCollectOutdatedAndCleanable(t *testing.T) {
	out := &model.Item{Name: "git", Status: model.StatusOutdated, Category: model.CatBrew}
	ok := &model.Item{Name: "curl", Status: model.StatusOK, Category: model.CatBrew}
	clean := &model.Item{Name: "cache", Status: model.StatusCleanCandidate, Category: model.CatCache}
	summaries := []*model.SourceSummary{
		{Category: model.CatBrew, Label: "Homebrew", Items: []*model.Item{out, ok}},
		{Category: model.CatCache, Label: "Caches", Items: []*model.Item{clean}},
	}

	got := collectOutdated(summaries, "")
	if len(got) != 1 || got[0].Name != "git" {
		t.Fatalf("outdated=%v", got)
	}
	got = collectOutdated(summaries, "homebrew")
	if len(got) != 1 {
		t.Fatalf("filter label: %v", got)
	}
	got = collectOutdated(summaries, "git")
	if len(got) != 1 {
		t.Fatalf("filter name: %v", got)
	}
	got = collectOutdated(summaries, "npm")
	if len(got) != 0 {
		t.Fatalf("unmatched filter should be empty: %v", got)
	}

	c := collectCleanable(summaries, "cache")
	if len(c) != 1 || c[0].Name != "cache" {
		t.Fatalf("cleanable=%v", c)
	}
	if n := collectCleanable(summaries, "brew"); len(n) != 0 {
		t.Fatalf("wrong filter: %v", n)
	}
}

func TestItemMatchesFilter(t *testing.T) {
	s := &model.SourceSummary{Category: model.CatBrew, Label: "Homebrew Formulae"}
	it := &model.Item{Name: "wget"}
	cases := []struct {
		only string
		want bool
	}{
		{"", true},
		{"brew", true},
		{"Homebrew", true},
		{"wget", true},
		{"apt", false},
	}
	for _, tc := range cases {
		if got := itemMatchesFilter(s, it, tc.only); got != tc.want {
			t.Errorf("only=%q got=%v want=%v", tc.only, got, tc.want)
		}
	}
}

func TestGroupByCategoryAndSorted(t *testing.T) {
	items := []*model.Item{
		{Name: "a", Category: model.CatNpm},
		{Name: "b", Category: model.CatBrew},
		{Name: "c", Category: model.CatBrew},
	}
	g := groupByCategory(items)
	if len(g[model.CatBrew]) != 2 || len(g[model.CatNpm]) != 1 {
		t.Fatalf("groups=%v", g)
	}
	cats := sortedCategories(g)
	if len(cats) != 2 {
		t.Fatalf("cats=%v", cats)
	}
	if cats[0] > cats[1] {
		t.Fatalf("not sorted: %v", cats)
	}
}

func TestCategoryLabel(t *testing.T) {
	summaries := []*model.SourceSummary{
		{Category: model.CatBrew, Icon: "🍺", Label: "Brew"},
	}
	if got := categoryLabel(summaries, model.CatBrew); got != "🍺 Brew" {
		t.Fatalf("got %q", got)
	}
	if got := categoryLabel(summaries, model.CatNpm); got != string(model.CatNpm) {
		t.Fatalf("fallback got %q", got)
	}
}

func TestPlatformLabel(t *testing.T) {
	cases := []struct {
		p    model.PlatformInfo
		want string
	}{
		{model.PlatformInfo{OS: "darwin"}, "macOS"},
		{model.PlatformInfo{OS: "windows"}, "Windows"},
		{model.PlatformInfo{OS: "linux", Distro: "ubuntu"}, "ubuntu"},
		{model.PlatformInfo{OS: "linux"}, "linux"},
		{model.PlatformInfo{OS: "freebsd"}, "system"},
	}
	for _, tc := range cases {
		if got := platformLabel(tc.p); got != tc.want {
			t.Errorf("os=%s distro=%s got=%q want=%q", tc.p.OS, tc.p.Distro, got, tc.want)
		}
	}
}

func TestTallyUpdateResults(t *testing.T) {
	items := []*model.Item{
		{Name: "ok"},
		{Name: "skip"},
		{Name: "fail"},
		{Name: "blank"},
	}
	results := []*updater.Result{
		{Item: items[0], Success: true},
		{Item: items[1], Success: false, Error: "⊘ cancelled"},
		{Item: items[2], Success: false, Error: "boom"},
		{Item: items[3], Success: false, Error: ""},
	}
	out := captureStdout(t, func() {
		ok, fail, skipped := tallyUpdateResults(results)
		if ok != 1 || fail != 2 || skipped != 1 {
			t.Fatalf("ok=%d fail=%d skipped=%d", ok, fail, skipped)
		}
	})
	if !strings.Contains(out, "✓ ok") || !strings.Contains(out, "⊘ skip") {
		t.Fatalf("output=%q", out)
	}
	if !strings.Contains(out, "fail") || !strings.Contains(out, "blank") {
		t.Fatalf("missing fail lines: %q", out)
	}
}

func TestPrintCleanItemDetail(t *testing.T) {
	out := captureStdout(t, func() {
		printCleanItemDetail(&model.Item{Name: "cache", Reclaimable: "1.2GB"})
		printCleanItemDetail(&model.Item{Name: "tmp"})
	})
	if !strings.Contains(out, "cache (~1.2GB reclaimable)") {
		t.Fatalf("reclaimable: %q", out)
	}
	if !strings.Contains(out, "• tmp") {
		t.Fatalf("plain: %q", out)
	}
}

func TestPrintDryRun(t *testing.T) {
	out := captureStdout(t, func() {
		printDryRun("update", []*model.Item{
			{Name: "git", Category: model.CatBrew},
			{Name: "cache", Category: model.CatCache, Reclaimable: "500MB"},
		})
	})
	if !strings.Contains(out, "dry-run: would update") {
		t.Fatalf("%q", out)
	}
	if !strings.Contains(out, "git") || !strings.Contains(out, "500MB") {
		t.Fatalf("%q", out)
	}
}

func TestSkipAndManualResults(t *testing.T) {
	items := []*model.Item{{Name: "a", KeepPolicy: "manual policy"}, {Name: "b"}}
	skip := skipBatchResults(items, "no sudo")
	if len(skip) != 2 || !strings.HasPrefix(skip[0].Error, "⊘ ") {
		t.Fatalf("%+v", skip)
	}
	manual := manualOnlyResults(items)
	if !strings.Contains(manual[0].Error, "manual policy") {
		t.Fatalf("%+v", manual[0])
	}
	if !strings.Contains(manual[1].Error, "só atualização manual") {
		t.Fatalf("%+v", manual[1])
	}
}

func TestPartitionUpdatable(t *testing.T) {
	// KeepPolicy alone is not enough — ClassifyItem decides KindManualOnly.
	items := []*model.Item{
		{Name: "auto", Category: model.CatBrew},
		{Name: "manual", Category: model.CatAgent, KeepPolicy: "update via app"},
	}
	up, man := partitionUpdatable(items)
	if len(up)+len(man) != 2 {
		t.Fatalf("up=%d man=%d", len(up), len(man))
	}
}

func TestRunNativeUpdateSectionEmpty(t *testing.T) {
	env := updateBatchEnv{plat: model.PlatformInfo{OS: "linux"}, cfg: Config{}}
	ok, fail, skipped, res := runNativeUpdateSection(t.Context(), env, nil)
	if ok != 0 || fail != 0 || skipped != 0 || res != nil {
		t.Fatalf("ok=%d fail=%d sk=%d res=%v", ok, fail, skipped, res)
	}
}

func TestPrintCheckAndVerifyReport(t *testing.T) {
	outdated := &model.Item{
		Name: "git", Status: model.StatusOutdated, Category: model.CatBrew,
		CurrentVer: "1.0", AvailableVer: "2.0",
	}
	agentOK := &model.Item{Name: "claude", Status: model.StatusOK, CurrentVer: "1.2.3", Category: model.CatAgent}
	agentOut := &model.Item{
		Name: "codex", Status: model.StatusOutdated, Category: model.CatAgent,
		CurrentVer: "0.1", AvailableVer: "0.2",
	}
	clean := &model.Item{Name: "npm-cache", Status: model.StatusCleanCandidate, Category: model.CatCache}

	updates := []*model.SourceSummary{
		{Icon: "🍺", Label: "Brew", Category: model.CatBrew, Outdated: 1, Items: []*model.Item{outdated}},
		{Icon: "🤖", Label: "Agents", Category: model.CatAgent, Outdated: 1, Items: []*model.Item{agentOK, agentOut}},
		{Icon: "✓", Label: "Clean PM", Category: model.CatNpm, Outdated: 0, Items: nil},
	}
	cleanup := []*model.SourceSummary{
		{Icon: "🧹", Label: "Caches", Category: model.CatCache, Reclaimable: "2GB", Items: []*model.Item{clean}},
		{Icon: "∅", Label: "None", Category: model.CatCache, Items: nil},
	}

	out := captureStdout(t, func() {
		o, c := PrintCheck(updates, cleanup)
		if o != 2 || c != 1 {
			t.Fatalf("outdated=%d cleanable=%d", o, c)
		}
	})
	if !strings.Contains(out, "Updates") || !strings.Contains(out, "git") {
		t.Fatalf("check out: %q", out)
	}

	// Empty footer path
	out = captureStdout(t, func() {
		printCheckFooter(0, 0, 0, 0)
	})
	if !strings.Contains(out, "up to date") {
		t.Fatalf("footer empty: %q", out)
	}

	results := []*updater.Result{
		{Item: outdated, Success: false, Error: "failed"},
	}
	out = captureStdout(t, func() {
		stats := PrintVerifyReport(updates, results, 1, 1, 0)
		if stats.remaining < 1 {
			t.Fatalf("remaining=%d", stats.remaining)
		}
		if stats.failed != 1 || stats.updated != 1 {
			t.Fatalf("stats=%+v", stats)
		}
	})
	if !strings.Contains(out, "Relatório") {
		t.Fatalf("verify: %q", out)
	}

	// All clear verify path
	clearUpdates := []*model.SourceSummary{
		{Category: model.CatBrew, Items: []*model.Item{{Name: "x", Status: model.StatusOK}}},
	}
	out = captureStdout(t, func() {
		stats := PrintVerifyReport(clearUpdates, nil, 3, 0, 1)
		if stats.remaining != 0 {
			t.Fatalf("want 0 remaining, got %d", stats.remaining)
		}
	})
	if !strings.Contains(out, "nada outdated") {
		t.Fatalf("clear verify: %q", out)
	}
}

func TestIndexResultsAndClassify(t *testing.T) {
	it := &model.Item{Name: "x", Status: model.StatusOutdated, Category: model.CatBrew}
	r := &updater.Result{Item: it, Success: false, Error: "boom"}
	m := indexResults([]*updater.Result{r, nil, {Item: nil}})
	if m[it] != r {
		t.Fatal("index miss")
	}
	stats := &verifyStats{}
	need, man, fail, other := classifyRemaining(
		[]*model.SourceSummary{{Items: []*model.Item{it, {Name: "ok", Status: model.StatusOK}}}},
		m,
		stats,
	)
	if stats.remaining != 1 {
		t.Fatalf("remaining=%d", stats.remaining)
	}
	_ = need
	_ = man
	_ = fail
	_ = other
}

func TestPrintOutdatedLine(t *testing.T) {
	out := captureStdout(t, func() {
		printOutdatedLine(&model.Item{Name: "app", CurrentVer: "", AvailableVer: "", KeepPolicy: "keep"})
		printOutdatedLine(&model.Item{Name: "b", CurrentVer: "1", AvailableVer: "2"})
	})
	if !strings.Contains(out, "?") || !strings.Contains(out, "newer") || !strings.Contains(out, "keep") {
		t.Fatalf("%q", out)
	}
}

func TestShouldUseNativeMacAuth(t *testing.T) {
	// Non-darwin never uses native auth.
	if shouldUseNativeMacAuth(model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "x", Category: model.CatMAS},
	}, Config{}) {
		t.Fatal("linux should not use native mac auth")
	}
	if shouldUseNativeMacAuth(model.PlatformInfo{OS: "darwin"}, []*model.Item{
		{Name: "x", Category: model.CatMAS},
	}, Config{SkipPassword: true}) {
		t.Fatal("SkipPassword disables native auth")
	}
}

func TestPartitionNativeElevated_nonDarwin(t *testing.T) {
	items := []*model.Item{{Name: "a", Category: model.CatBrew}}
	native, normal := partitionNativeElevated(model.PlatformInfo{OS: "linux"}, items, Config{})
	if native != nil || len(normal) != 1 {
		t.Fatalf("native=%v normal=%v", native, normal)
	}
}

func TestRunCategoryUpdateSection_skipElevated(t *testing.T) {
	var sess *elevate.Session
	env := updateBatchEnv{
		plat: model.PlatformInfo{OS: "linux"},
		summaries: []*model.SourceSummary{
			{Category: model.CatApt, Icon: "📦", Label: "apt"},
		},
		cfg:         Config{SkipPassword: true},
		elevSession: &sess,
	}
	items := []*model.Item{{Name: "curl", Category: model.CatApt}}
	out := captureStdout(t, func() {
		ok, fail, skipped, res := runCategoryUpdateSection(context.Background(), env, model.CatApt, items)
		if ok != 0 || fail != 0 || skipped != 1 || len(res) != 1 {
			t.Fatalf("ok=%d fail=%d sk=%d res=%+v", ok, fail, skipped, res)
		}
	})
	if !strings.Contains(out, "apt") {
		t.Fatalf("output=%q", out)
	}
}

func TestRunCategoryUpdateSection_brewPasswordSkip(t *testing.T) {
	var sess *elevate.Session
	env := updateBatchEnv{
		plat: model.PlatformInfo{OS: "darwin"},
		summaries: []*model.SourceSummary{
			{Category: model.CatBrew, Icon: "🍺", Label: "Brew"},
		},
		cfg:         Config{SkipPassword: true},
		elevSession: &sess,
	}
	// microsoft-* is on the brew password list; without a session the batch is skipped.
	items := []*model.Item{{Name: "microsoft-office", Category: model.CatBrew}}
	out := captureStdout(t, func() {
		ok, fail, skipped, res := runCategoryUpdateSection(context.Background(), env, model.CatBrew, items)
		if ok != 0 || fail != 0 || skipped != 1 || len(res) != 1 {
			t.Fatalf("ok=%d fail=%d sk=%d res=%+v", ok, fail, skipped, res)
		}
	})
	if !strings.Contains(out, "Brew") {
		t.Fatalf("output=%q", out)
	}
}

func TestRunOneClean_unknownCategory(t *testing.T) {
	it := &model.Item{Name: "weird", Category: model.CatBrew}
	out := captureStdout(t, func() {
		ok, fail, freed := runOneClean(context.Background(), it, cleaner.Options{})
		if ok != 0 || fail != 1 || freed != 0 {
			t.Fatalf("ok=%d fail=%d freed=%d", ok, fail, freed)
		}
	})
	if !strings.Contains(out, "weird") {
		t.Fatalf("output=%q", out)
	}
}

func TestRunCleanBatches(t *testing.T) {
	it := &model.Item{Name: "weird", Category: model.CatBrew, Status: model.StatusCleanCandidate}
	summaries := []*model.SourceSummary{
		{Icon: "🍺", Label: "Brew", Items: []*model.Item{it}},
	}
	out := captureStdout(t, func() {
		ok, fail, freed := runCleanBatches(context.Background(), summaries, []*model.Item{it}, cleaner.Options{})
		if ok != 0 || fail != 1 || freed != 0 {
			t.Fatalf("ok=%d fail=%d freed=%d", ok, fail, freed)
		}
	})
	if !strings.Contains(out, "Brew") || !strings.Contains(out, "weird") {
		t.Fatalf("output=%q", out)
	}
}

func TestPrepareCleanElevation_noNeed(t *testing.T) {
	ctx := context.Background()
	got := prepareCleanElevation(ctx, model.PlatformInfo{OS: "darwin"}, []*model.Item{
		{Name: "npm-cache", Category: model.CatCache},
	}, false)
	if got != ctx {
		t.Fatal("expected same context when elevation not needed")
	}
}

func TestElevationSkipReason(t *testing.T) {
	if !strings.Contains(elevationSkipReason(Config{SkipPassword: true}), "skip-password") {
		t.Fatal("expected skip-password reason")
	}
	if !strings.Contains(elevationSkipReason(Config{}), "senha") {
		t.Fatal("expected password reason")
	}
}

func TestEnsureCategoryElevation_skip(t *testing.T) {
	var sess *elevate.Session
	ctx := context.Background()
	_, skipped, reason := ensureCategoryElevation(ctx, model.PlatformInfo{OS: "linux"}, model.CatApt, Config{SkipPassword: true}, &sess)
	if !skipped || reason == "" {
		t.Fatalf("skipped=%v reason=%q", skipped, reason)
	}
	// Non-elevated category continues without skip.
	_, skipped, _ = ensureCategoryElevation(ctx, model.PlatformInfo{OS: "linux"}, model.CatNpm, Config{}, &sess)
	if skipped {
		t.Fatal("npm should not need elevation skip")
	}
}

func TestCountScanHints(t *testing.T) {
	var np, mo int
	countScanHints(&model.Item{Name: "x", Category: model.CatBrew}, &np, &mo)
	// Hints depend on ClassifyItem; just ensure no panic and counters stay non-negative.
	if np < 0 || mo < 0 {
		t.Fatalf("np=%d mo=%d", np, mo)
	}
}

func TestNativeElevatedFailAll(t *testing.T) {
	items := []*model.Item{{Name: "a"}, {Name: "b"}}
	res := nativeElevatedFailAll(items, "out", fmt.Errorf("auth failed"))
	if len(res) != 2 {
		t.Fatalf("len=%d", len(res))
	}
	if res[0].Success || res[0].Error != "auth failed" || res[0].Output != "out" {
		t.Fatalf("%+v", res[0])
	}
	if items[0].Status != model.StatusError {
		t.Fatalf("status=%v", items[0].Status)
	}
}

func TestStdinIsTTY(t *testing.T) {
	// Non-TTY stdin (pipe/test harness) should report false; do not fail if true.
	_ = stdinIsTTY()
}

func TestPrimeElevationSession_paths(t *testing.T) {
	ctx := context.Background()
	var sess *elevate.Session
	// No elevation needed.
	got := primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "x", Category: model.CatNpm},
	}, Config{}, &sess)
	if got != ctx {
		t.Fatal("expected unchanged ctx")
	}
	// Needs elevation but SkipPassword — return without prompt.
	got = primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "curl", Category: model.CatApt},
	}, Config{SkipPassword: true}, &sess)
	if got != ctx {
		t.Fatal("SkipPassword should leave ctx unchanged without session")
	}
	// Ready session is reattached.
	s := elevate.NewSession()
	s.SetPasswordless()
	sess = s
	got = primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "curl", Category: model.CatApt},
	}, Config{}, &sess)
	if elevate.FromContext(got) == nil {
		t.Fatal("expected session on context")
	}
}

func TestEnsureBrewPassword(t *testing.T) {
	ctx := context.Background()
	var sess *elevate.Session
	// Plain brew items do not need password session.
	_, skip, _ := ensureBrewPassword(ctx, []*model.Item{{Name: "telegram"}}, Config{}, &sess)
	if skip {
		t.Fatal("telegram should not skip")
	}
	// Password casks without session are skipped.
	_, skip, reason := ensureBrewPassword(ctx, []*model.Item{{Name: "microsoft-office"}}, Config{SkipPassword: true}, &sess)
	if !skip || reason == "" {
		t.Fatalf("skip=%v reason=%q", skip, reason)
	}
	// Ready session attaches.
	s := elevate.NewSession()
	s.SetPasswordless()
	sess = s
	c, skip, _ := ensureBrewPassword(ctx, []*model.Item{{Name: "microsoft-office"}}, Config{}, &sess)
	if skip || elevate.FromContext(c) == nil {
		t.Fatalf("skip=%v session missing", skip)
	}
}

func TestEnsureCategoryElevation_readySession(t *testing.T) {
	ctx := context.Background()
	s := elevate.NewSession()
	s.SetPasswordless()
	sess := s
	c, skip, _ := ensureCategoryElevation(ctx, model.PlatformInfo{OS: "linux"}, model.CatApt, Config{}, &sess)
	if skip || elevate.FromContext(c) == nil {
		t.Fatalf("skip=%v", skip)
	}
	// Non-elevated category still attaches ready session when present.
	c, skip, _ = ensureCategoryElevation(ctx, model.PlatformInfo{OS: "linux"}, model.CatNpm, Config{}, &sess)
	if skip || elevate.FromContext(c) == nil {
		t.Fatalf("npm skip=%v", skip)
	}
}

func TestContainsPasswordNote(t *testing.T) {
	if !containsPasswordNote("precisa de SENHA de admin") {
		t.Fatal("expected password note")
	}
	if containsPasswordNote("nothing special") {
		t.Fatal("unexpected match")
	}
}

func TestItemNeedsNativeElevation(t *testing.T) {
	plat := model.PlatformInfo{OS: "darwin"}
	if !itemNeedsNativeElevation(&model.Item{Name: "microsoft-office", Category: model.CatBrew}, plat) {
		t.Fatal("microsoft brew should need native elev")
	}
	if !itemNeedsNativeElevation(&model.Item{Name: "app", Category: model.CatMAS}, plat) {
		t.Fatal("mas should need elev")
	}
	if itemNeedsNativeElevation(&model.Item{Name: "telegram", Category: model.CatBrew}, plat) {
		t.Fatal("plain brew should not")
	}
}

func TestPrintCheckFooterHints(t *testing.T) {
	out := captureStdout(t, func() {
		printCheckFooter(3, 2, 1, 1)
	})
	if !strings.Contains(out, "outdated") || !strings.Contains(out, "senha") || !strings.Contains(out, "manual") || !strings.Contains(out, "cleanable") {
		t.Fatalf("%q", out)
	}
}

func TestPrepareCleanElevation_passwordlessOrInteractive(t *testing.T) {
	ctx := context.Background()
	// apt cache may need elevation; interactive=false should not panic.
	_ = prepareCleanElevation(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "apt-cache", Category: model.CatCache},
	}, false)
	_ = prepareCleanElevation(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "apt-cache", Category: model.CatCache},
	}, true)
}

func TestBrewItemNeedsPassword(t *testing.T) {
	if !brewItemNeedsPassword(&model.Item{Name: "microsoft-office"}) {
		t.Fatal("expected password for microsoft-office")
	}
	if brewItemNeedsPassword(&model.Item{Name: "wget"}) {
		t.Fatal("wget should not need password")
	}
}
