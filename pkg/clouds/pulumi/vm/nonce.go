// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"time"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// runNonce makes a command re-run once per `sc deploy`, no matter what else is
// equal. One value per process, so that two stacks deployed by one invocation agree.
//
// Which commands get it is not a preference, it is a property of what the command
// does, and the cases fail in opposite directions:
//
//   - A command that only READS the machine needs it. Its inputs are invariant by
//     nature — a host address and a script that does not change — so without a nonce
//     pulumi-command runs it once per stack lifetime and answers every later deploy
//     out of the checkpoint. That is not a stale cache, it is a check that stopped
//     checking while continuing to report green.
//   - A command that CHANGES the machine and cannot tell on the machine whether
//     there is anything to change must NOT have it. A nonce there turns every deploy
//     into a redeploy of everything it touches — including, on a stack with a
//     published host port, a stop-start of the serving container for a deploy that
//     changed nothing.
//   - A command that changes the machine but PROVES REMOTELY that there is nothing
//     to do — create-if-absent, an install guarded by a marker file — should have
//     it. Its trigger set cannot move when the machine drifts underneath it (nothing
//     in the config changed; the machine did), so without a nonce the deploy that
//     would repair the drift is the one deploy that is skipped. That is the shape of
//     the worst fault this file has produced: see TriggersForGuardedChange.
//
// Live-caught three times, every one of them green: the slot read (api#455), the
// preflight and readiness gates, and the ingress (this change). None had a failing
// test, and all were written *with* triggers — "has Triggers" is not the property
// that matters, "has a trigger that moves when the command's answer could move" is.
var runNonce = time.Now().UTC().Format(time.RFC3339Nano)

// RunNonce is runNonce for the packages layered on this one (k3s, and whatever
// follows it) — exported as a function and not as a var so that there is exactly one
// value per process no matter who reads it. A second nonce would be worse than none:
// two commands against one machine could then disagree about which deploy they are
// part of.
func RunNonce() string {
	return runNonce
}

// TriggersForRead, TriggersForChange and TriggersForGuardedChange are the rule above
// expressed as something a call site has to choose between, rather than as a comment
// it can agree with and then get wrong. All three exist, including the one that adds
// nothing: the choice is the product, and `TriggersForChange(...)` says "this was
// decided" where a bare sdk.Array says "nobody thought about it".
//
// `base` is still required to hold everything that changes what the command does —
// the nonce decides whether it runs at all, not whether it is correct.
func TriggersForRead(base ...sdk.Input) sdk.Array {
	return withNonce(base)
}

// TriggersForChange is for a command that mutates the machine and cannot decide
// remotely whether it needs to: no nonce, so it runs when its own inputs move and not
// otherwise.
func TriggersForChange(base ...sdk.Input) sdk.Array {
	out := make(sdk.Array, 0, len(base))
	return append(out, base...)
}

// TriggersForGuardedChange is for a command that mutates the machine and begins by
// checking, ON the machine, whether it has to. Mechanically it is TriggersForRead;
// the separate name is the review question, because the three preconditions are not
// visible in the trigger set and getting any of them wrong is expensive:
//
//  1. Running it against a machine that already matches changes nothing observable —
//     either because it checks first (`docker network inspect || create`, the k3s
//     installer's `is-active && version && args-marker` guard) or because it is a
//     rewrite of a file with identical content. Not "harmless to run twice": a
//     container restart is harmless twice and still disqualifies.
//
//  2. The no-op is CHEAP: no image pull that cannot be skipped, no service restart,
//     no reissue of anything rate-limited.
//
//  3. The command carries NO Delete script. A nonce replaces the resource on every
//     deploy, and pulumi-command performs a replacement as create-then-delete — so a
//     Delete here would tear down, moments after creating it, whatever the Create
//     just built. Put the Delete on its own resource with no triggers (see
//     ensureIngress and VmComposeStack's teardown command, which do exactly that).
//
//     And note what MOVING an existing Delete off a command costs, because it is not
//     free and it is not obvious: the Delete that runs during a replacement is the
//     one in the OLD state, so the first deploy after the move replaces the command
//     (its triggers gained the nonce) and runs the old Delete one last time. That
//     provision ends with the thing torn down, green, and the NEXT one puts it back.
//     Measured on the smoke stand, 2026-10-18: provision 8 logged "ingress is up"
//     and then "removed the ingress", provision 9 restored it and logged no teardown.
//     Nothing in the code can prevent it — only a human knowing to provision twice —
//     so a release that moves a Delete has to say so in its notes.
//
// The fault this case exists for was live on the ingress until 2026-10-18 and is the
// reason the rule has three cases rather than two: `ingress-up` had both a trigger
// that moved with the config and a Delete that removed the ingress, so changing one
// character of `ingress.email` brought Caddy up and then deleted it — container gone,
// /etc/sc/caddy gone, 443 answering nothing, `sc provision` exit 0. Re-running the
// provision could not repair it either: with the config now unchanged, every ingress
// command was skipped as up-to-date, and the deploy stayed green against a machine
// with no TLS on it.
func TriggersForGuardedChange(base ...sdk.Input) sdk.Array {
	return withNonce(base)
}

func withNonce(base []sdk.Input) sdk.Array {
	out := make(sdk.Array, 0, len(base)+1)
	out = append(out, base...)
	return append(out, sdk.String(runNonce))
}
