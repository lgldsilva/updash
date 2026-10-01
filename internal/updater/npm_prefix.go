package updater

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// groupNpmByPrefix partitions npm items into per-prefix groups: the ambient
// prefix first, then non-default prefixes in lexicographic order. The planner
// emits one plan per group in this exact order, and both executors zip plans
// back onto groups using it. Pure.
func groupNpmByPrefix(items []*model.Item) [][]*model.Item {
	groups := make(map[string][]*model.Item)
	for _, it := range items {
		if it == nil {
			continue
		}
		groups[it.Prefix] = append(groups[it.Prefix], it)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if (keys[i] == "") != (keys[j] == "") {
			return keys[i] == "" // ambient prefix always first
		}
		return keys[i] < keys[j]
	})
	out := make([][]*model.Item, 0, len(keys))
	for _, k := range keys {
		out = append(out, groups[k])
	}
	return out
}

// npmUpdateArgs builds `npm update -g [--prefix P] <names...>`: only the
// non-protected, deduplicated names of one prefix group. Targeting names
// instead of a bare `npm update -g` keeps protected packages out, matches the
// per-package model used by brew, and — on npm 12 — is the only form that
// actually enumerates globals. Pure (no I/O).
func npmUpdateArgs(prefix string, items []*model.Item) []string {
	seen := make(map[string]bool, len(items))
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil || it.Name == "" || seen[it.Name] {
			continue
		}
		seen[it.Name] = true
		names = append(names, it.Name)
	}
	if len(names) == 0 {
		return nil
	}
	args := []string{commandUpdate, flagGlobal}
	if prefix != "" {
		args = append(args, flagPrefix, prefix)
	}
	return append(args, names...)
}

// npmPrefixGroupPlans builds one `npm update -g` per prefix group. Items
// without a Prefix target the ambient prefix; items with a Prefix are updated
// in that exact prefix so a legacy installation is refreshed where it lives
// instead of duplicated into the active one.
func npmPrefixGroupPlans(ctx context.Context, items []*model.Item) ([]CommandPlan, error) {
	var plans []CommandPlan
	for _, group := range groupNpmByPrefix(items) {
		args := npmUpdateArgs(group[0].Prefix, group)
		if len(args) == 0 {
			return nil, fmt.Errorf("npm update requires at least one package name")
		}
		if allow := npmAllowScriptsFlag(group); allow != "" {
			args = append(args, allow)
		}
		elevated, err := npmGroupElevation(ctx, group)
		if err != nil {
			return nil, err
		}
		plans = append(plans, CommandPlan{Name: npmCommand, Args: args, Scope: CommandScopeExact, Elevated: elevated})
	}
	return plans, nil
}

// npmGroupElevation decides sudo for one prefix group: explicit non-default
// prefixes are judged by their path (a user-owned legacy prefix needs no
// sudo); the ambient prefix keeps the `npm config get prefix` probe.
func npmGroupElevation(ctx context.Context, group []*model.Item) (bool, error) {
	if prefix := group[0].Prefix; prefix != "" {
		return npmPrefixNeedsSudo(prefix), nil
	}
	return npmGlobalElevation(ctx)
}

// npmPrefixNeedsSudo reports whether an explicit global npm prefix lives
// under the system /usr tree — the same rule npmGlobalElevation applies to
// the ambient prefix. Pure.
func npmPrefixNeedsSudo(prefix string) bool {
	return strings.HasPrefix(prefix, "/usr")
}
