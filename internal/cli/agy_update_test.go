package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

func TestCollectUpdateTargets_IncludesAgyInfoOnly(t *testing.T) {
	agy := &model.Item{Name: "Agy", Status: model.StatusInfo, Category: model.CatAgent, CurrentVer: "1.2.16"}
	agyBad := &model.Item{Name: "Agy", Status: model.StatusUnverified, Category: model.CatAgent}
	grok := &model.Item{Name: "Grok", Status: model.StatusInfo, Category: model.CatAgent, CurrentVer: "1.0.0"}
	cursor := &model.Item{Name: "Cursor", Status: model.StatusInfo, Category: model.CatAgent, CurrentVer: "1.0"}
	bun := &model.Item{Name: "typescript", Status: model.StatusInfo, Category: model.CatBun}
	git := &model.Item{Name: "git", Status: model.StatusOutdated, Category: model.CatBrew}
	summaries := []*model.SourceSummary{
		{Category: model.CatAgent, Label: "AI Agents", Items: []*model.Item{agy, agyBad, grok, cursor, nil}},
		{Category: model.CatBun, Label: "bun", Items: []*model.Item{bun}},
		nil,
		{Category: model.CatBrew, Label: "Homebrew", Items: []*model.Item{git}},
	}

	got := collectUpdateTargets(summaries, "")
	if len(got) != 2 || got[0] != git || got[1] != agy {
		t.Fatalf("targets = %v", namesOf(got))
	}
	only := collectUpdateTargets(summaries, "agy")
	if len(only) != 1 || only[0] != agy {
		t.Fatalf("--only agy = %v", namesOf(only))
	}
}

func namesOf(items []*model.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		if it != nil {
			out[i] = it.Name + ":" + it.Status.String()
		}
	}
	return out
}

func TestRunCheck_AgyInfoIsNotUnverified(t *testing.T) {
	restoreHooks(t)
	fakeScan([]*model.SourceSummary{agyInfoSummary()}, nil)

	out := captureStdout(t, func() {
		err := RunCheck(context.Background(), Config{})
		if err != nil || ExitCode(err) != 0 {
			t.Fatalf("check err=%v", err)
		}
	})
	if strings.Contains(out, "✘") || strings.Contains(out, "verification failed") || strings.Contains(out, "unverified source") {
		t.Fatalf("agy classified as a problem:\n%s", out)
	}
	if !strings.Contains(out, "freshness not verified; checked on update: agy update") {
		t.Fatalf("check note missing:\n%s", out)
	}
}

func TestRunCheck_AgyJSONIsNotUnverified(t *testing.T) {
	restoreHooks(t)
	fakeScan([]*model.SourceSummary{agyInfoSummary()}, nil)

	out := captureStdout(t, func() {
		err := RunCheck(context.Background(), Config{JSON: true})
		if err != nil || ExitCode(err) != 0 {
			t.Fatalf("json check err=%v", err)
		}
	})
	if strings.Contains(out, `"status": "unverified"`) || strings.Contains(out, `"status": "error"`) {
		t.Fatalf("json classified agy as a problem:\n%s", out)
	}
	for _, want := range []string{`"checked_on_update"`, `"status": "info"`, "checked on update: agy update"} {
		if !strings.Contains(out, want) {
			t.Fatalf("json missing %q:\n%s", want, out)
		}
	}
}

func TestRunUpdate_AgyZeroExitIsSuccess(t *testing.T) {
	cases := []struct {
		name string
		out  string
	}{
		{name: "already current", out: "✓ You are already on the latest version."},
		{name: "installed", out: "✓ Update successful!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restoreHooks(t)
			dir := writeFakeAgy(t, tc.out, 0)
			installAgyScan(t)
			var code int
			out := captureStdout(t, func() {
				ok, fail, err := RunUpdate(context.Background(), Config{})
				code = ExitCode(err)
				if ok != 1 || fail != 0 || err != nil {
					t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
				}
			})
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			if strings.Contains(out, "✘") || !strings.Contains(out, "Verified — nothing outdated remains") {
				t.Fatalf("success reported as a problem:\n%s", out)
			}
			assertAgyUpdateRan(t, dir)
		})
	}
}

func TestRunUpdate_AgyFailureIsClassified(t *testing.T) {
	restoreHooks(t)
	dir := writeFakeAgy(t, "download failed", 1)
	installAgyScan(t)

	var code int
	out := captureStdout(t, func() {
		_, fail, err := RunUpdate(context.Background(), Config{})
		code = ExitCode(err)
		if err == nil || fail < 1 {
			t.Fatalf("fail=%d err=%v", fail, err)
		}
	})
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "Agy") {
		t.Fatalf("failure did not name Agy:\n%s", out)
	}
	assertAgyUpdateRan(t, dir)
}

func TestRunUpdate_AgyDryRunPlansCommand(t *testing.T) {
	restoreHooks(t)
	dir := writeFakeAgy(t, "should not run", 0)
	installAgyScan(t)

	out := captureStdout(t, func() {
		_, _, err := RunUpdate(context.Background(), Config{DryRun: true})
		if err != nil || ExitCode(err) != 0 {
			t.Fatalf("dry-run err=%v", err)
		}
	})
	if !strings.Contains(out, "agy update") {
		t.Fatalf("dry-run missing command:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "stamp")); !os.IsNotExist(err) {
		t.Fatalf("dry-run executed agy update: %v", err)
	}
}

func TestPrintVerifyReport_AgyFailureWithoutOutdated(t *testing.T) {
	ran := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusError, CurrentVer: "1.2.16"}
	rescan := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	var stats verifyStats
	out := captureStdout(t, func() {
		stats = PrintVerifyReport(
			[]*model.SourceSummary{{Items: []*model.Item{rescan}}},
			[]*updater.Result{{Item: ran, Success: false, Error: "exit status 1"}},
			0, 1, 0,
		)
	})
	if stats.failed != 1 || stats.remaining != 0 {
		t.Fatalf("stats=%+v", stats)
	}
	if strings.Contains(out, "nothing outdated remains") || !strings.Contains(out, "exit status 1") {
		t.Fatalf("report=\n%s", out)
	}
}

func TestPrintVerifyReport_AgyOutdatedFailureIsNotDoubleCounted(t *testing.T) {
	item := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusOutdated, CurrentVer: "1.2.13"}
	var stats verifyStats
	captureStdout(t, func() {
		stats = PrintVerifyReport(
			[]*model.SourceSummary{{Items: []*model.Item{item}}},
			[]*updater.Result{
				nil,
				{Item: nil, Success: false},
				{Item: item, Success: false, Error: "exit status 1"},
				{Item: &model.Item{Name: "Agy", Category: model.CatAgent}, Success: false, Error: "⊘ already handled"},
			},
			0, 2, 0,
		)
	})
	if stats.failed != 1 || stats.remaining != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestCheckedOnUpdateFillsMissingNote(t *testing.T) {
	rep := BuildCheckReport([]*model.SourceSummary{{Items: []*model.Item{
		{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"},
		{Name: "Grok", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.0.0"},
		nil,
	}}}, nil)
	if len(rep.Problems) != 0 {
		t.Fatalf("problems=%+v", rep.Problems)
	}
	if len(rep.CheckedOnUpdate) != 1 || rep.CheckedOnUpdate[0].Status != "info" ||
		rep.CheckedOnUpdate[0].KeepPolicy != "checked on update: agy update" {
		t.Fatalf("checked=%+v", rep.CheckedOnUpdate)
	}
	if agentFreshnessNote(nil) != "freshness not verified" {
		t.Fatal("nil agent note")
	}
}

func TestPrintVerifyReport_AgySuccessIsNotInconclusive(t *testing.T) {
	ran := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusDone, CurrentVer: "1.2.16"}
	rescan := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	var stats verifyStats
	out := captureStdout(t, func() {
		stats = PrintVerifyReport(
			[]*model.SourceSummary{{Items: []*model.Item{rescan}}},
			[]*updater.Result{{Item: ran, Success: true, Output: "✓ You are already on the latest version."}},
			1, 0, 0,
		)
	})
	if stats.failed != 0 || stats.remaining != 0 {
		t.Fatalf("stats=%+v", stats)
	}
	if !strings.Contains(out, "nothing outdated remains") {
		t.Fatalf("report=\n%s", out)
	}
}

func agyInfoSummary() *model.SourceSummary {
	return &model.SourceSummary{
		Category: model.CatAgent,
		Icon:     "🤖",
		Label:    "AI Agents",
		Items: []*model.Item{{
			Name:       "Agy",
			Category:   model.CatAgent,
			Status:     model.StatusInfo,
			CurrentVer: "1.2.16",
			KeepPolicy: scanner.AgentUpdateCheckNote("Agy"),
		}},
	}
}

// installAgyScan makes every scan pass return a fresh informational Agy row,
// matching a rescan that still has no latest channel.
func installAgyScan(t *testing.T) {
	t.Helper()
	fakeScan(nil, nil)
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		return []*model.SourceSummary{agyInfoSummary()}
	}
}

func writeFakeAgy(t *testing.T, updateStdout string, updateExit int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("agy fixture is a shell script")
	}
	dir := t.TempDir()
	stamp := filepath.Join(dir, "stamp")
	body := "#!/bin/sh\nset -u\ncase \"$1\" in\n" +
		"--version)\nprintf '%s\\n' '1.2.16'\n;;\n" +
		"update)\n" +
		"printf '%s\\n' \"$1\" > " + shQuote(stamp) + "\n" +
		"printf 'CI=%s\\n' \"${CI-}\" >> " + shQuote(stamp) + "\n" +
		"printf 'NONINTERACTIVE=%s\\n' \"${NONINTERACTIVE-}\" >> " + shQuote(stamp) + "\n" +
		"printf 'DEBIAN_FRONTEND=%s\\n' \"${DEBIAN_FRONTEND-}\" >> " + shQuote(stamp) + "\n" +
		"printf 'NO_COLOR=%s\\n' \"${NO_COLOR-}\" >> " + shQuote(stamp) + "\n" +
		"printf '%s\\n' " + shQuote(updateStdout) + "\n" +
		"exit " + strconv.Itoa(updateExit) + "\n;;\n" +
		"*)\necho unexpected >&2\nexit 9\n;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "agy"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// A value already in the environment wins (GitHub sets CI=true). Drop the
	// keys so the stamp shows the values updash itself injects.
	dropInheritedUpdateEnv(t)
	t.Setenv("PATH", dir)
	return dir
}

func dropInheritedUpdateEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CI", "NONINTERACTIVE", "DEBIAN_FRONTEND", "NO_COLOR"} {
		prev, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if !ok {
				_ = os.Unsetenv(key)
				return
			}
			_ = os.Setenv(key, prev)
		})
	}
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func assertAgyUpdateRan(t *testing.T, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "stamp"))
	if err != nil {
		t.Fatalf("agy update did not run: %v", err)
	}
	text := string(raw)
	if !strings.HasPrefix(text, "update\n") {
		t.Fatalf("stamp = %q, want argv update", text)
	}
	for _, want := range []string{"CI=1\n", "NONINTERACTIVE=1\n", "DEBIAN_FRONTEND=noninteractive\n", "NO_COLOR=1\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stamp missing %q:\n%s", want, text)
		}
	}
}
