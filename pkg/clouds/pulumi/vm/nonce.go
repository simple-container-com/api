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
// does, and the two halves of the rule fail in opposite directions:
//
//   - A command that only READS the machine needs it. Its inputs are invariant by
//     nature — a host address and a script that does not change — so without a nonce
//     pulumi-command runs it once per stack lifetime and answers every later deploy
//     out of the checkpoint. That is not a stale cache, it is a check that stopped
//     checking while continuing to report green.
//   - A command that CHANGES the machine must never have it. A nonce there turns
//     every deploy into a full redeploy of everything — including, on a stack with a
//     published host port, a stop-start of the serving container for a deploy that
//     changed nothing.
//
// Two commands are reads: the active-slot read (VmComposeStack) and the compose
// preflight. The readiness gate (ready.go) is the third. The consequence the nonce
// has on the first of those is intended: a `sc deploy` that changes nothing still
// re-reads the active slot, and therefore still flips to the other one. A deploy that
// was asked for deploys.
//
// Live-caught twice, both green: the slot read (api#455) and the preflight and
// readiness gates (this change). Neither had a failing test, and both were written
// *with* triggers — "has Triggers" is not the property that matters, "has a trigger
// that moves when the command's answer could move" is.
var runNonce = time.Now().UTC().Format(time.RFC3339Nano)

// RunNonce is runNonce for the packages layered on this one (k3s, and whatever
// follows it) — exported as a function and not as a var so that there is exactly one
// value per process no matter who reads it. A second nonce would be worse than none:
// two commands against one machine could then disagree about which deploy they are
// part of.
func RunNonce() string {
	return runNonce
}

// TriggersForRead and TriggersForChange are the rule above expressed as something a
// call site has to choose between, rather than as a comment it can agree with and
// then get wrong. Both exist, including the one that adds nothing: the choice is the
// product, and `TriggersForChange(...)` says "this was decided" where a bare
// sdk.Array says "nobody thought about it".
//
// `base` is still required to hold everything that changes what the command does —
// the nonce decides whether it runs at all, not whether it is correct.
func TriggersForRead(base ...sdk.Input) sdk.Array {
	out := make(sdk.Array, 0, len(base)+1)
	out = append(out, base...)
	return append(out, sdk.String(runNonce))
}

// TriggersForChange is for a command that mutates the machine: no nonce, so it runs
// when its own inputs move and not otherwise.
func TriggersForChange(base ...sdk.Input) sdk.Array {
	out := make(sdk.Array, 0, len(base))
	return append(out, base...)
}
