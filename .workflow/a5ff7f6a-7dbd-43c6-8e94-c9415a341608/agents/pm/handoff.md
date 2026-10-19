[thinking] # PM Handoff — `sc-stack-outputs-leaks-secrets-in-clear`

**Slice:** `sc-stack-outputs-leaks-secrets-in-clear` (`docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`, repo `api`)
**Dispatch attempt:** 3rd. Both prior attempts (runs `32521357`, `fadc2560`) died in Developer to guest OOM with zero commits/PRs — this is an infra issue, not a scoping issue. The plan below is unchanged from what a clean dispatch needs; I'm flagging the OOM discipline forward explicitly in the handoff guidance so Architect/Developer don't re-discover it the hard way.

## 1. Commit + branch identity
No code mutations in this turn. (PM role — planning only.)

## 2. Files touched
| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none (plan only) |

## 3. Requirement restated

`sc stack outputs -s <stack>` currently prints **every** output value in the clear — including outputs the owning resource marks secret (Pulumi `auto.OutputValue.Secret == true`). Real credentials (a k3s cluster-admin kubeconfig, a join token) have already leaked into a live-smoke terminal this way. `--show-secrets` today only gates *whether* secret outputs are fetched/shown at all — once shown, there is no masking. Fix: mask by default, reveal only on explicit opt-in.

## 4. Root cause / where this lives (verified by reading the code, not guessing)

- `pkg/cmd/cmd_stack/cmd_outputs.go` — the `outputs` subcommand. Currently just calls `Provisioner.Outputs()` and JSON-marshals the result verbatim. No flag at all today — the brief's claim "`--show-secrets` controls whether shown" describes `sc stack secret-get`/doc text, not this command's actual flag surface (confirmed: `grep` for `show-secrets`/`ShowSecrets` across the repo returns **zero** hits outside docs/prose). So today `sc stack outputs` has **no secret-aware flag whatsoever** — it always prints in the clear.
- `pkg/clouds/pulumi/preview.go` → `toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult` is the single choke point both parent-stack and child-stack output reads funnel through (`OutputsStack` → `toOutputsResult`). It already inspects `value.Secret` (Pulumi's own secret marker) but only uses it to decide whether to JSON-wrap the *raw* value — never to mask it.
- `pkg/api/client.go` — `OutputsResult{StackName, Outputs map[string]any}` is the wire/JSON shape consumers see; `pkg/api/StackParams` is the existing params struct threaded CLI-flag → `Provisioner.Outputs` → `OutputsStack` → `toOutputsResult`, i.e. the natural place to carry a new opt-in bool without inventing a new plumbing path.
- `pkg/provisioner/preview.go` → `(*provisioner).Outputs` is the dispatch layer between the CLI and the per-cloud `OutputsStack`; no change needed here beyond passing `params` through (it already does).

## 5. Scope (MoSCoW)

| # | Item | Label |
|---|---|---|
| 1 | Add `ShowSecrets bool` field to `api.StackParams` | MUST |
| 2 | Register `--show-secrets` flag **only on `sc stack outputs`** (not the shared `RegisterStackFlags`, which also backs `cancel`/`destroy`/`secret-get` — don't widen blast radius) | MUST |
| 3 | In `toOutputsResult`, when `!params.ShowSecrets && value.Secret`: replace the value with a fixed-width placeholder `<secret:redacted>` instead of the plaintext/JSON-wrapped value | MUST |
| 4 | When `params.ShowSecrets == true` (or `value.Secret == false`): behave exactly as today (no behavior change for non-secret outputs, ever) | MUST |
| 5 | Unit test proving: (a) default invocation masks a secret-marked output to the placeholder, (b) `--show-secrets` reveals the plaintext, (c) a non-secret output is unaffected either way | MUST |
| 6 | Update `docs/docs/examples/k3s/byo-host/README.md` ("There is no `--show-secrets`... printed in the clear, always") — this doc is now factually wrong once shipped; flip the `umask 077` guidance to mention `--show-secrets` | NICE-TO-HAVE (do if time allows in the same PR; do not let it block the code+test landing) |
| 7 | Reusing the exact `--show-secrets` flag name (vs. inventing `--reveal-secrets`) | MUST — brief explicitly allows either, but `--show-secrets` already exists in doc prose/user expectation and avoids a second near-synonym flag in the CLI surface |
| 8 | Any new output-format option (`-o`), structural JSON change (e.g. tagging which keys were secret), or masking of `sc provision -P` preview summaries | CUT — out of scope, not required by DoD, and `preview` output already has its own `logger.Redact` path for private-key material |
| 9 | Touching `sc secrets reveal`/`sc secrets hide` (the repo-secrets-file mechanism) | CUT — unrelated mechanism, brief only asks to *mirror the pattern*, not merge it |

## 6. Acceptance criteria (restated as testable)

| AC# | Criterion |
|---|---|
| AC1 | `sc stack outputs -s <stack>` invoked with no flag masks every output where the underlying Pulumi value is secret-marked, printing a fixed placeholder (e.g. `<secret:redacted>`) instead of plaintext. |
| AC2 | `sc stack outputs -s <stack> --show-secrets` reveals the same outputs in plaintext (today's behavior), unchanged. |
| AC3 | Outputs that are **not** secret-marked print in plaintext in both invocations — masking must not leak into non-secret keys. |
| AC4 | A unit test exists (in `pkg/clouds/pulumi`, next to `toOutputsResult`/`preview_redact_test.go`) exercising AC1–AC3 directly against `toOutputsResult` with a fake `auto.OutputMap` carrying both secret and non-secret entries. |
| AC5 | No change to `OutputsResult`'s JSON shape (`stackName`, `outputs`) — this is purely a value-substitution at the point outputs are already rendered, so no consumer (CI scripts piping `sc stack outputs \| jq`) breaks on shape. |
| AC6 | `make check` is lint/vet-clean on the touched packages (`pkg/api`, `pkg/clouds/pulumi`, `pkg/cmd/cmd_stack`, `pkg/cmd/root_cmd`). |

## 7. Out-of-scope / explicitly cut
- Changing the shared `RegisterStackFlags` (used by `cancel`/`destroy`/the parent `stack` cmd) — flag lives on `outputs` only.
- Masking Pulumi preview/update summaries (separate code path, already has its own redaction for private keys).
- Any change to how `sc secrets hide/reveal` manages the repo-committed secrets store.
- New JSON/YAML output modes.

## 8. Handoff guidance for Architect

- Single-package change, small surface: `pkg/api/client.go` (one field), `pkg/cmd/cmd_stack/cmd_outputs.go` (one flag), `pkg/clouds/pulumi/preview.go` (the masking branch in `toOutputsResult`), plus a new/extended `_test.go` in `pkg/clouds/pulumi`.
- `toOutputsResult` is a pure function today (`stackName string, result auto.OutputMap`) — it will need `params api.StackParams` (or just the bool) threaded in as a new argument; check both call sites (`OutputsStack`'s one call) when changing its signature.
- **Mandatory build discipline (carried forward from the dispatch brief — this is the 3rd attempt and the first two died here):** Developer must NOT run `go build ./...` / `go test ./...` / `make build`/`make build-all` — link alone needs ~13GB in a 12GB guest. Use `make check` (capped `go vet ./...`, ~2.3GB) plus `go test ./pkg/clouds/pulumi/...` and `go test ./pkg/cmd/cmd_stack/...` narrowly. Keep the turn short (commit before resume 5-10). Read `api/CLAUDE.md` first — it's already merged and has the exact caps.
- Docs update (item 6) is NICE-TO-HAVE — land it in the same PR if trivial, but don't let it block code+test from merging if time runs short.

**Verdict:** signoff