// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
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

// remoteCommandType is pulumi-command's SSH command resource — the
// thing every vm / k3s provision is built out of.
const remoteCommandType = "command:remote:Command"

// commandDeleteInput is the pulumi-command input holding the script
// run when the resource goes away. Its presence is the ONLY thing
// that makes replacing a remote command destructive.
const commandDeleteInput = "delete"

// isStatelessRemoteCommand reports whether this replace is of a
// remote command that will destroy nothing when it is replaced.
//
// Why this is not a `safeToReplaceTypes` entry. That allowlist is
// per-TYPE, and `command:remote:Command` is not uniformly safe: a
// command carrying a `delete` script runs that script against a live
// machine when it is replaced (pulumi-command replaces as
// create-then-delete), which is a real teardown and exactly what the
// guard is for. SC's k3s uninstall
// (pkg/clouds/pulumi/k3s/cluster.go's uninstallOnDestroy) and the vm
// ingress teardown (pkg/clouds/pulumi/vm/caddy.go) are both that
// shape. So the question is per-RESOURCE, not per-type, and the
// answer is in the inputs.
//
// Why the exemption is needed at all. SC nonces its commands: every
// install / readiness / read command gets the run timestamp in its
// `triggers` via pulumiVm.TriggersForGuardedChange or
// TriggersForRead, deliberately, so a host whose k3s was stopped out
// of band is repaired rather than reported as "no changes" (the
// scripts are idempotent and carry their own args-marker guard —
// k3s's installer answers an unchanged config with "already running
// as k3s with the requested configuration"). A replace on every run
// is therefore the NORMAL shape of a vm or k3s provision, not a rare
// event to wave through. Live-caught 2026-11-08 on a 3-node k3s
// stand: the second `sc provision` refused outright, naming nine
// resources — six k3s commands and three per-host readiness gates —
// every one of them a nonce-only trigger diff with `keys=[]`, so the
// guard's own explanation ("a changed REPLACE-forcing input") did not
// even apply. Nothing in the fleet could re-provision a vm stack.
//
// The failure direction when inputs cannot be read. Engine step
// events populate Inputs, but if that ever stops being true (or a
// delete script arrives as a secret placeholder rather than a
// string) a remote command becomes exempt rather than guarded. That
// is the chosen direction: a remote command holds no cloud object at
// the other end of its id — the id is a random suffix on the
// resource name — so the guard was never protecting state here, only
// the delete script. A non-string value is treated as present
// precisely because it is not provably empty.
func isStatelessRemoteCommand(meta apitype.StepEventMetadata) bool {
	if meta.Type != remoteCommandType {
		return false
	}
	// Both sides matter: the old state is what gets deleted, and the new
	// state is what a subsequent destroy would run.
	return !hasCommandDelete(meta.Old) && !hasCommandDelete(meta.New)
}

// hasCommandDelete reports whether a step's resource state declares a
// non-empty `delete` script.
func hasCommandDelete(state *apitype.StepEventStateMetadata) bool {
	if state == nil {
		return false
	}
	value, ok := state.Inputs[commandDeleteInput]
	if !ok {
		return false
	}
	script, ok := value.(string)
	if !ok {
		return true
	}
	return strings.TrimSpace(script) != ""
}

// providerRefKey is the replace-forcing "key" the engine names when what
// changed is not a property of the resource at all but which provider
// instance serves it.
const providerRefKey = "provider"

// pulumiUnknownValue is the engine's placeholder for a value it cannot
// compute during a preview, mirroring
// `pulumi/sdk/go/common/resource/plugin.UnknownStringValue`. Hardcoded
// rather than imported: that package pulls the whole plugin host into a
// file that otherwise needs only apitype, and the constant is part of
// Pulumi's wire format — it has not changed since the sentinel was
// introduced and cannot, since every provider SDK in every language
// hardcodes it too. The test pins it against the SDK's own copy.
const pulumiUnknownValue = "04da6b54-80e4-46f7-96ec-b56ff0331ba9"

// isUnknownProviderIDOnly reports whether this replace exists only
// because the engine does not yet know its provider's ID — a preview
// artifact, not a decision.
//
// The shape, measured on the 3-VPS fleet stand (2026-11-09) rather than
// inferred. A provider reference is `<urn>::<id>`, and for all seven
// Kubernetes resources the guard refused:
//
//	keys    = [provider]
//	oldProv = urn:…:providers:kubernetes::smoke-caddy--staging-caddy-kubeprovider::d85914e8-7282-41fc-82a9-ae3d49f014a6
//	newProv = urn:…:providers:kubernetes::smoke-caddy--staging-caddy-kubeprovider::04da6b54-80e4-46f7-96ec-b56ff0331ba9
//
// Identical URN; the new ID is the unknown sentinel. Nothing is being
// re-pointed at a different provider — the engine simply cannot name the
// ID of a provider that is itself in the plan, so it conservatively
// reports every dependent as a replace. The apply then computes the real
// diff and does not replace them: in the same stand's successful run the
// provider's own step was `refresh` then **`update`**, while its preview
// had planned create-replacement / replace / delete-replaced.
//
// Why a type allowlist is the wrong fix here, and why this had to be its
// own predicate. The seven resources were a Namespace, a Deployment, a
// Service, a ServiceAccount, a PodDisruptionBudget, a ClusterRole and a
// ClusterRoleBinding. Several of those replace genuinely destructively —
// a Service loses its ClusterIP and, where the cluster has one, its
// load-balancer address; a Namespace cascades to everything inside it.
// Exempting them by TYPE to get a provision through would blind the
// guard to exactly the faults it was written for. The honest exemption
// is not about what the resources are, it is about the fact that no
// input of theirs changed.
//
// Why this is not rare. SC nonces every read command on purpose (see
// pulumiVm.TriggersForRead), so a k3s stack's kubeconfig read is
// replaced on every provision; any provider configured from its stdout
// therefore has an unknown credential at preview, and every resource
// under that provider shows up here. Without this predicate the second
// and every subsequent `sc provision` of a k3s + Caddy stack is refused,
// naming resources the apply would only update.
//
// Both halves of the condition are load-bearing. A differing URN is a
// genuine re-pointing — a resource moved to another cluster's provider —
// and stays guarded. A known-but-different ID is a provider that really
// was replaced, and stays guarded. Only "same provider, ID not yet
// computed" is waved through, and only when `provider` is the sole
// replace-forcing key: a step that also changed a real input is still
// that input's replace.
func isUnknownProviderIDOnly(meta apitype.StepEventMetadata) bool {
	if len(meta.Keys) != 1 || meta.Keys[0] != providerRefKey {
		return false
	}
	if meta.Old == nil || meta.New == nil {
		return false
	}
	oldURN, oldID, ok := splitProviderRef(meta.Old.Provider)
	if !ok {
		return false
	}
	newURN, newID, ok := splitProviderRef(meta.New.Provider)
	if !ok {
		return false
	}
	// oldID != "" keeps this from exempting a step whose old provider was
	// itself never created; that is a different situation and the Old.ID
	// check in isDestructiveReplace already covers the resource, but the
	// provider reference deserves the same scepticism.
	return oldURN == newURN && oldID != "" && newID == pulumiUnknownValue
}

// splitProviderRef splits a Pulumi provider reference `<urn>::<id>` into
// its two halves. The URN itself contains `::` as a field separator, so
// the split is on the LAST occurrence — splitting on the first would
// return a fragment of the URN as the id and make the comparison above
// accidentally true for unrelated providers.
func splitProviderRef(ref string) (urn, id string, ok bool) {
	idx := strings.LastIndex(ref, "::")
	if idx < 0 {
		return "", "", false
	}
	return ref[:idx], ref[idx+2:], true
}

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
			"would drop the existing resource and create a new one at the same URN but pointing elsewhere.\n",
		len(replaces))
	// The message used to stop at "if the replace is intentional, set the
	// variable". An operator reading that has no way to tell a genuine catch
	// from a resource type whose replace never loses anything, so the honest
	// answer in both cases was to set the variable — which is how an escape
	// hatch stops being read. Give them the question to ask instead, and the
	// file where a false positive gets fixed for everyone.
	fmt.Fprintf(&b,
		"For each resource below, ask what the replace actually loses. If the answer is nothing — the whole "+
			"content comes from your config on every apply, or the provider keeps the old revision — then the "+
			"TYPE belongs on the allowlist in pkg/api/safe_to_replace.go (or its plugin's "+
			"`safe_to_replace_types`), and fixing it there fixes it for every stack. If the answer is data you "+
			"cannot regenerate, this is the catch the guard exists for: do not proceed. %s=1 is for a replace "+
			"you have reviewed and genuinely intend, one deploy at a time.\n",
		envAllowDestructiveReplace)
	for _, r := range replaces {
		// An empty key list used to render as `keys=[]`, which reads as
		// "no inputs changed" directly under a sentence blaming a changed
		// input — and that combination cost an hour on 2026-11-08. The
		// engine leaves Keys (and DetailedDiff) empty for a provider that
		// forces replacement without naming a property, pulumi-command's
		// `triggers` among them, so say that instead of printing a hole.
		// The message PREFIX above is the stable part for CI grepping; this
		// line is diagnostics.
		if len(r.Keys) == 0 {
			fmt.Fprintf(&b, "  - %s (%s): the provider named no replace-forcing key — check the resource's `triggers`\n", r.URN, r.Type)
			continue
		}
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
