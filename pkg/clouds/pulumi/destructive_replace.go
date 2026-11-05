// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// The set of safe-to-replace types lives in `pkg/api` (see
// safe_to_replace.go) so both `pkg/stackread` and this package can
// consume it without an import cycle: stackread merges each plugin's
// `scpluginv1.SchemaResponse.safe_to_replace_types` advertisement
// there via `api.RegisterSafeToReplaceType`. The distinction stays
// STATEFUL vs STATELESS: stateful resources (MongoCluster, Atlas
// Project, DNS Record, databases, buckets) MUST NOT be replaced;
// stateless/immutable-revision resources (ECS task definition, …)
// are additive replaces. See
// forge/docs/roadmap/items/sc-safe-to-replace-via-plugin-schema.md.

// envAllowDestructiveReplace is the operator-level escape hatch for
// S4 of `cloudflare-plugin-wrong-zone`. Setting it to a truthy value
// (`1` / `true` / `yes`, case-insensitive) in the deploy environment
// lets a preview with input-driven destructive replaces proceed.
//
// The name is explicit on purpose: a `SC_` prefix keeps it in the
// same namespace as every other SC runtime switch (and makes it grep-
// able), and `DESTRUCTIVE_REPLACE` is the phrase the error message
// below names — an operator who sees the message and searches the
// repo lands here. Keep this name stable across releases; renaming
// silently disables every operator's current workaround.
const envAllowDestructiveReplace = "SC_ALLOW_DESTRUCTIVE_REPLACE"

// destructiveReplacesAllowed is the single parse site for
// envAllowDestructiveReplace so a truthy-value typo (e.g. `TRUE` vs
// `true`) does not silently fall through to "blocked". Mirrors Go's
// strconv.ParseBool allow-list.
func destructiveReplacesAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAllowDestructiveReplace))) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	}
	return false
}

// destructiveReplaceError is the sentinel preview returns when it
// detected one or more input-driven replaces that would destroy a
// live resource. It carries the full list so the caller (and the
// error message the operator sees) can name every URN and the keys
// that forced each replace, not just the first one.
type destructiveReplaceError struct {
	Replaces []destructiveReplace
}

// Error renders an operator-facing multi-line message. Format is
// stable so an operator who grep-matches on the message's prefix
// (e.g. "input-driven destructive replace") in a CI log parser does
// not break across releases.
func (e *destructiveReplaceError) Error() string {
	// Sort by URN so the message is deterministic — map iteration or
	// event-arrival order would otherwise make the same deploy log
	// different across runs for diffing.
	replaces := append([]destructiveReplace(nil), e.Replaces...)
	sort.Slice(replaces, func(i, j int) bool { return replaces[i].URN < replaces[j].URN })

	var b strings.Builder
	fmt.Fprintf(&b,
		"input-driven destructive replace: preview would destroy %d live resource(s) by re-pointing them at new objects. "+
			"This is the no-destroy invariant failing — a changed REPLACE-forcing input (zone id, region, immutable name, …) "+
			"would drop the existing resource and create a new one at the same URN but pointing elsewhere. "+
			"Review each resource below; if the replace is intentional, set %s=1 and re-run.\n",
		len(replaces), envAllowDestructiveReplace)
	for _, r := range replaces {
		fmt.Fprintf(&b, "  - %s (%s): keys=[%s]\n", r.URN, r.Type, strings.Join(r.Keys, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// destructiveReplaceErrorFrom returns a non-nil sentinel when the
// tracker caught any replaces AND the operator did not opt in. Keeps
// the preview call sites one-liner-free.
func destructiveReplaceErrorFrom(replaces []destructiveReplace) error {
	if len(replaces) == 0 {
		return nil
	}
	if destructiveReplacesAllowed() {
		return nil
	}
	return &destructiveReplaceError{Replaces: replaces}
}
