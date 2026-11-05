// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"strings"

	"github.com/pkg/errors"
)

// nullSecretCheckpointSentinel is the substring Pulumi's RPC layer
// emits when a stack's checkpoint carries a secret whose `value` key is
// missing — the shape a failed-mid-update leaves in state.
//
// See `sdk/v3/go/common/resource/plugin/rpc.go`: a secret marshals as
// `{SigKey: SecretSig, "value": element}`. When `element` disappears
// mid-marshal (SkipNulls / dropped-unknown path during a failure), the
// serialised secret object has no `value` and every subsequent op
// decoding state errors out with the sentinel below BEFORE planning
// anything. So the stack cannot preview, cannot deploy, cannot be torn
// down — it looks unrecoverable until the operator knows the escape.
//
// Live-caught 2026-10-24 on the Cloud.ru three-way smoke's `rucreate`
// stack after an apt-lock failure left every secret decrypting to
// JSON `null` (TLS keys included — hand-decrypt of the checkpoint
// confirmed it). Full write-up in forge roadmap
// `sc-failed-update-nulls-every-secret`.
//
// Keep this substring stable across pulumi-sdk upgrades: a rename
// upstream silently disables the guard.
const nullSecretCheckpointSentinel = "malformed RPC secret: missing value"

// nullSecretCheckpointRecoveryHint is the paragraph the operator-
// facing error grows when the sentinel fires. Multi-line on purpose —
// a terminal reader reaches the recovery step without scrolling, and a
// CI log parser grep-matches on the first line ("checkpoint secrets
// nulled by prior failed update").
//
// Reads as a standing rule rather than a one-off: the same shape bites
// every SC path that passes secrets through local.Command / remote.Command
// `Stdin` (vm, k3s, Container Apps, DNS), so the fix is named for the
// class and the recovery is identical for all of them.
const nullSecretCheckpointRecoveryHint = `checkpoint secrets nulled by prior failed update — this stack cannot preview, deploy, or destroy until state is repaired.

Recovery (safe, no external state touched):
  1. Restore the pulumi checkpoint from its sibling .bak:
       mv <stack>.json.bak <stack>.json
     (Pulumi writes .bak right before the failed update overwrote .json
     with the broken secret state.)
  2. Re-run the ORIGINAL command with -S (skip preview):
       sc provision -s <stack> -S
     The preview is what decodes state and trips the sentinel; a direct
     Up rewrites state with real secret values.
  3. Subsequent preview + deploy work normally again.

Why this happens: Pulumi serialises each secret as {sig, value}; a
failed-mid-marshal (SkipNulls / dropped unknown) emits a secret object
with no value key, and every subsequent op errors out before planning.
Any SC path that carries secrets on local.Command / remote.Command Stdin
is exposed to this failure class, not just the stack that tripped it.
Tracked upstream as a minimal-repro + bug-report follow-up; this guard
makes the failure survivable in the meantime.`

// annotateNullSecretCheckpointError wraps a Pulumi error with the
// recovery hint above when it carries the sentinel. Non-match → err is
// returned unchanged, so adding this to a call site has no behavioural
// cost in the normal case.
//
// Wraps with `errors.Wrap` so the original error is preserved in the
// chain for anything downstream that does `errors.Is` / `errors.As` on
// it — only the top-level `Error()` string gains the recovery hint.
func annotateNullSecretCheckpointError(err error) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), nullSecretCheckpointSentinel) {
		return err
	}
	return errors.Wrap(err, nullSecretCheckpointRecoveryHint)
}
