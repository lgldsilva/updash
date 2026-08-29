package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func TestRunCategoryUpdateSection_missingPreparedBatchFailsClosed(t *testing.T) {
	var sess *elevate.Session
	items := []*model.Item{{Name: "curl", Category: model.CatApt}}
	env := updateBatchEnv{
		plat:        model.PlatformInfo{OS: "linux"},
		summaries:   []*model.SourceSummary{{Category: model.CatApt, Icon: "📦", Label: "apt"}},
		prepared:    map[model.Category]*updater.PreparedUpdateBatch{},
		elevSession: &sess,
	}
	out := captureStdout(t, func() {
		ok, fail, skipped, res := runCategoryUpdateSection(context.Background(), env, model.CatApt, items)
		if ok != 0 || skipped != 0 || fail != 1 || len(res) != 1 {
			t.Fatalf("ok=%d fail=%d skipped=%d res=%+v", ok, fail, skipped, res)
		}
		if res[0].Success || !strings.Contains(res[0].Error, "missing prepared update batch") {
			t.Fatalf("want a plan-missing failure, got %+v", res[0])
		}
	})
	if !strings.Contains(out, "apt") {
		t.Fatalf("output=%q", out)
	}
}

func TestRunPreparedBrewUpdateBatch_subsetErrorsFailClosed(t *testing.T) {
	restoreHooks(t)
	planned := []*model.Item{{Name: "wget", Category: model.CatBrew}}
	batch, err := updater.PrepareUpdateBatch(context.Background(), model.CatBrew, planned)
	if err != nil {
		t.Fatal(err)
	}
	executePreparedBatch = func(context.Context, *updater.PreparedUpdateBatch, updater.Options) []*updater.Result {
		t.Fatal("subset errors must not execute a drifted plan")
		return nil
	}
	var sess *elevate.Session

	plainUnknown := []*model.Item{{Name: "ripgrep", Category: model.CatBrew}}
	res := runPreparedBrewUpdateBatch(context.Background(), batch, plainUnknown, updater.Options{}, Config{}, &sess)
	if len(res) != 1 || res[0].Success || !strings.Contains(res[0].Error, "not part of the prepared update batch") {
		t.Fatalf("plain subset: %+v", res)
	}

	s := elevate.NewSession()
	s.SetPasswordless()
	sess = s
	passwordUnknown := []*model.Item{{Name: "microsoft-office", Category: model.CatBrew}}
	res = runPreparedBrewUpdateBatch(context.Background(), batch, passwordUnknown, updater.Options{}, Config{}, &sess)
	if len(res) != 1 || res[0].Success || !strings.Contains(res[0].Error, "not part of the prepared update batch") {
		t.Fatalf("password subset: %+v", res)
	}
}

func TestRunPreparedBrewUpdateBatch_passwordUsesPreparedSubset(t *testing.T) {
	restoreHooks(t)
	plain := &model.Item{Name: "wget", Category: model.CatBrew}
	password := &model.Item{Name: "microsoft-office", Category: model.CatBrew}
	items := []*model.Item{plain, password}
	batch, err := updater.PrepareUpdateBatch(context.Background(), model.CatBrew, items)
	if err != nil {
		t.Fatal(err)
	}
	var executed []string
	executePreparedBatch = func(_ context.Context, got *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		out := make([]*updater.Result, 0, len(got.Items()))
		for _, item := range got.Items() {
			executed = append(executed, item.Name)
			out = append(out, &updater.Result{Item: item, Success: true})
		}
		return out
	}
	s := elevate.NewSession()
	s.SetPasswordless()
	sess := s
	res := runPreparedBrewUpdateBatch(context.Background(), batch, items, updater.Options{}, Config{}, &sess)
	if len(res) != 2 || !res[0].Success || !res[1].Success {
		t.Fatalf("results=%+v", res)
	}
	if strings.Join(executed, ",") != "wget,microsoft-office" {
		t.Fatalf("executed=%v, want the prepared subsets in order", executed)
	}
}

func TestRunBrewUpdateBatch_prepareErrorFailsClosed(t *testing.T) {
	restoreHooks(t)
	prepareUpdateBatch = func(context.Context, model.Category, []*model.Item) (*updater.PreparedUpdateBatch, error) {
		return nil, errors.New("planner refused")
	}
	executePreparedBatch = func(context.Context, *updater.PreparedUpdateBatch, updater.Options) []*updater.Result {
		t.Fatal("a failed plan must not execute")
		return nil
	}
	var sess *elevate.Session
	items := []*model.Item{{Name: "wget", Category: model.CatBrew}}
	res := runBrewUpdateBatch(context.Background(), items, updater.Options{}, Config{}, &sess)
	if len(res) != 1 || res[0].Success || res[0].Error != "planner refused" {
		t.Fatalf("results=%+v", res)
	}
}

func TestRunNativeElevatedItems_reusesPreparedBatch(t *testing.T) {
	restoreHooks(t)
	item := &model.Item{Name: "wget", Category: model.CatBrew}
	batch, err := updater.PrepareUpdateBatch(context.Background(), model.CatBrew, []*model.Item{item})
	if err != nil {
		t.Fatal(err)
	}
	var executed *updater.PreparedUpdateBatch
	executePreparedBatch = func(_ context.Context, got *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		executed = got
		return []*updater.Result{{Item: item, Success: true}}
	}
	updateCategory = func(context.Context, model.Category, []*model.Item, updater.Options) []*updater.Result {
		t.Fatal("native elevation must not replan an already prepared batch")
		return nil
	}
	primeMacSudo = func(context.Context) error { return nil }
	stdinIsTTYFn = func() bool { return true }

	var sess *elevate.Session
	res := runNativeElevatedItems(
		context.Background(),
		model.PlatformInfo{OS: "darwin"},
		[]*model.Item{item},
		updater.Options{},
		Config{},
		&sess,
		map[model.Category]*updater.PreparedUpdateBatch{model.CatBrew: batch},
	)
	if len(res) != 1 || !res[0].Success || executed == nil {
		t.Fatalf("res=%+v executed=%v", res, executed)
	}
	if got := executed.Items(); len(got) != 1 || got[0] != item {
		t.Fatalf("subset items=%v", executed.Items())
	}
}

func TestRunNativeElevatedItems_preparedSubsetErrorFailsClosed(t *testing.T) {
	restoreHooks(t)
	planned := []*model.Item{{Name: "wget", Category: model.CatBrew}}
	batch, err := updater.PrepareUpdateBatch(context.Background(), model.CatBrew, planned)
	if err != nil {
		t.Fatal(err)
	}
	executePreparedBatch = func(context.Context, *updater.PreparedUpdateBatch, updater.Options) []*updater.Result {
		t.Fatal("subset errors must not execute")
		return nil
	}
	primeMacSudo = func(context.Context) error { return nil }
	stdinIsTTYFn = func() bool { return true }

	unknown := &model.Item{Name: "ripgrep", Category: model.CatBrew}
	var sess *elevate.Session
	res := runNativeElevatedItems(
		context.Background(),
		model.PlatformInfo{OS: "darwin"},
		[]*model.Item{unknown},
		updater.Options{},
		Config{},
		&sess,
		map[model.Category]*updater.PreparedUpdateBatch{model.CatBrew: batch},
	)
	if len(res) != 1 || res[0].Success || !strings.Contains(res[0].Error, "not part of the prepared update batch") {
		t.Fatalf("results=%+v", res)
	}
}

func TestUpdatePlanErrorResults(t *testing.T) {
	items := []*model.Item{{Name: "a"}, {Name: "b"}}
	res := updatePlanErrorResults(items, fmt.Errorf("plan drift"))
	if len(res) != 2 {
		t.Fatalf("len=%d", len(res))
	}
	for i, r := range res {
		if r.Item != items[i] || r.Success || r.Error != "plan drift" {
			t.Fatalf("res[%d]=%+v", i, r)
		}
	}
}
