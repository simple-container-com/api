[thinking] # Handoff — Max Warner (PM), run 32521357-2668-4c22-ba01-607e7fca2c71

## 0. Roadmap linkage
`slice_id: sc-stack-outputs-leaks-secrets-in-clear` (roadmap item in `forge` repo, `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`, currently `status: in-progress`, this is its dispatched implementation run in the `api` repo).

## 1. Commit + branch identity
No code mutations in this turn — PM role, planning only.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none) | — | PM turn produces plan only; no repo writes |

## 3. Code-reading findings (grounds the plan below — verified against `simple-container-com/api` @ `0a008cdeb9353785ee516148f7652e0edb976aaa`)

- **Render path:** `pkg/cmd/cmd_stack/cmd_outputs.go` (`NewOutputsCmd`) calls `sCmd.Root.Provisioner.Outputs(ctx, sCmd.Params)` → `pkg/provisioner/preview.go` `(*provisioner).Outputs` → `pv.OutputsStack(ctx, cfg, stack, params)` → `pkg/clouds/pulumi/preview.go` `(*pulumi).OutputsStack`, which calls `p.toOutputsResult(stackSource.Name(), res)`.
- **The actual leak site:** `toOutputsResult` (`pkg/clouds/pulumi/preview.go`) does:
  ```go
  Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
      var res string
      if s, ok := value.Value.(string); ok { res = s }
      else if m, ok := value.Value.(map[string]any); ok { j,_ := json.Marshal(m); res = string(j) }
      else if value.Secret { j,_ := json.Marshal(value); res = string(j) }
      return res
  })
  ```
  Note the bug shape: a secret-marked output whose underlying `Value` is a **string** hits the *first* branch (`s, ok := value.Value.(string)`) and is returned as plaintext — `value.Secret` is never even consulted in that case. The `value.Secret` branch only triggers for non-string/non-map types, and even then it JSON-marshals the whole `OutputValue` (which embeds the plaintext `Value` field) — so that branch leaks too, just by a different serialization path. **Both paths are broken**, not just one.
- **No existing flag gates this.** I searched the repo for `ShowSecrets`, `show-secrets`, `reveal-secrets` in code: zero hits in `pkg/cmd/*` or `pkg/api/client.go`'s `StackParams`. The only occurrence of `--show-secrets` anywhere in the repo is prose in `docs/docs/examples/k3s/byo-host/README.md`, describing a flag that **does not exist in the implementation**. The brief's framing ("`--show-secrets` (if it exists) controls display, not masking") is actually generous — today there is no flag of any kind on `sc stack outputs`; it unconditionally prints every value, including secret-marked ones, in the clear. `cmd_outputs.go` registers zero flags beyond the stack-wide persistent ones (`--stack`, `--env`, `--dir`, etc. via `RegisterStackFlags`).
- **Existing "handle with care" pattern to mirror:** `pkg/cmd/cmd_secrets/cmd_reveal.go` (`sc secrets reveal`, `--force`/`-F`) and `cmd_hide.go` (`sc secrets hide`) — simple cobra bool flags gating a `Cryptor()` call. Same spirit, much smaller surface, applicable to `cmd_outputs.go`.
- **`api.StackParams`** (`pkg/api/client.go`) is the natural carrier for a new bool field (`RevealSecrets` or similar) — it already carries per-invocation bools like `SkipRefresh`, `SkipPreview`, `DetailedDiff`, flows unchanged through `Provisioner.Outputs` → `OutputsStack` → `toOutputsResult`.
- **Docs impact:** `docs/docs/examples/k3s/byo-host/README.md` and `docs/docs/examples/vm-compose/byo-host/README.md` both show `sc outputs -s ... --show-secrets -o json | jq ...` as the documented way to extract a kubeconfig/secret output. Whatever flag name ships, these two docs need updating (either to the new flag name, or ship `--show-secrets` as a real alias so the docs stay true — see scope decision below).

## 4. Scope decisions (MUST / NICE-TO-HAVE / CUT)

| # | Item | Priority |
|---|---|---|
| 1 | Add `RevealSecrets bool` (or equivalent) field to `api.StackParams` | MUST |
| 2 | New cobra flag on `sc stack outputs` only (not the shared `RegisterStackFlags`) — suggest `--reveal-secrets` per the brief's own suggested name, bound to that field, default `false` | MUST |
| 3 | Fix `toOutputsResult` to mask BOTH leak paths: a secret-marked output (any underlying type: string, map, or other) renders as a fixed placeholder (e.g. `<secret:redacted>`) by default, and the real value only when `revealSecrets` is `true` | MUST |
| 4 | Thread the flag through `Outputs` → `OutputsStack` → `toOutputsResult` call signature | MUST |
| 5 | Unit tests: (a) non-secret output unaffected by default or with flag, (b) secret output masked by default, (c) secret output revealed with flag on — for both the string-value leak path and the non-string/map leak path | MUST |
| 6 | Update the two docs pages (`k3s/byo-host/README.md`, `vm-compose/byo-host/README.md`) that currently instruct `--show-secrets` to use the real, shipped flag name | MUST (docs correctness — a reader following today's docs literally cannot reproduce the documented command since the flag never existed) |
| 7 | Ship `--show-secrets` as a back-compat alias of the new flag, since it's already load-bearing in two docs pages a user might already be following | NICE-TO-HAVE — Architect/Dev call: cheaper to just fix the two docs pages (item 6) than carry two flag names forever. Lean toward CUT unless Architect sees a reason to keep it |
| 8 | Extending the same masking to `sc stack secret-get` (`cmd_secret_get.go`, a different code path reading `stack.Secrets.Values`) | CUT — out of scope. The brief is specifically about `sc stack outputs`' resource-output rendering; `secret-get` reads a different map (`secrets.yaml`-sourced values, not Pulumi stack outputs) and already requires the caller to name the exact secret explicitly, which is a materially different risk profile (deliberate single-value fetch vs. "dump everything") |
| 9 | Roadmap item status flip (`docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`: `in-progress` → `code-complete`, `## History` entry with PR number + test evidence) | MUST, but **not mine to do this turn** — it belongs to whichever downstream character (DevOps/QA) lands the merged fix with real test evidence in hand. Flag this explicitly to whoever closes this slice: don't flip to `shipped` without a live/deployed verification per the repo's own definition of done |

## 5. Acceptance criteria (restated from the brief, numbered for traceability)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Locate where `sc stack outputs` renders outputs and how it currently decides display | Scoped | `toOutputsResult` in `pkg/clouds/pulumi/preview.go`, called from `OutputsStack` in the same file; invoked via `cmd_outputs.go` → `provisioner.Outputs`. Confirmed via `read_repository_file`/`search_repository_code` above — no flag exists today in code (only in stale docs prose) |
| AC2 | Add masking by default + explicit opt-in flag to reveal | Scoped — implementation task for Developer | Design: new `RevealSecrets bool` field on `api.StackParams` (`pkg/api/client.go`), new `--reveal-secrets` cobra flag registered only in `NewOutputsCmd` (`pkg/cmd/cmd_stack/cmd_outputs.go`), threaded to `toOutputsResult`'s new `revealSecrets bool` parameter |
| AC3 | Mirror `sc secrets reveal`/`hide` "sensitive value" pattern | Scoped | `cmd_reveal.go`/`cmd_hide.go` precedent read and cited above; same flag-gated-plaintext spirit, applied to `toOutputsResult`'s per-value masking instead of the whole secrets registry |
| AC4 | Tests covering: non-secret unaffected / secret masked by default / reveal flag unmasks | Scoped — Developer to write | Target: `pkg/clouds/pulumi/preview_test.go` (new file) exercising `toOutputsResult` directly with a fake `auto.OutputMap` containing one non-secret string output and one secret-marked output (cover both the string-value and non-string/map leak paths found in AC1), both with `revealSecrets=false` and `=true`. Per `CLAUDE.md`, scope test runs to `go test ./pkg/clouds/pulumi/... -run TestToOutputsResult` under the capped `GOMAXPROCS=1 GOMEMLIMIT=6GiB -p 1` invocation — do NOT run bare `go test ./...` on this module (OOM risk documented in repo's own CLAUDE.md) |
| AC5 (DoD-1) | `sc stack outputs` doesn't print a secret-marked output unless invoker opts in | Pending implementation | Will be PASS once AC2+AC3 land and the unit tests in AC4 go green |
| AC6 (DoD-2) | Test or documented manual smoke demonstrates default-masked / opt-in-revealed | Pending implementation | Unit tests (AC4) satisfy "a test... demonstrates" per the brief's own wording; a live smoke against a real stack with a secret output is NICE-TO-HAVE confirmation but not required to meet this AC's letter |
| AC7 (DoD-3) | Record outcome in `forge` repo's roadmap item, flip `status` appropriately | Deferred to closing character | Not actionable yet — no PR exists. Whoever lands the fix (with PR number + green test output in hand) does this edit in `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`'s `## Status`/`## History`, flipping `in-progress` → `code-complete` only — `shipped` requires a live/deployed verification per `forge`'s own CLAUDE.md, which this run (an `api`-repo code fix) cannot itself satisfy without a separate smoke |

## 6. Tests run
None — PM turn, no code mutated. Test obligations are specified above for the Developer/QA stages.

## 7. Handoff guidance for Architect

1. Confirm the exact flag name (`--reveal-secrets` recommended, see scope item 7 for the alias trade-off) and the exact placeholder string (brief suggests `<secret:redacted>` — keep it, it's distinctive and greppable in logs for anyone later auditing whether a leak occurred).
2. Design the signature change: `toOutputsResult(stackName string, result auto.OutputMap, revealSecrets bool) *api.OutputsResult` — single new parameter, minimal surface change. Verify there are no other callers of `toOutputsResult` beyond `OutputsStack` (my search found exactly one call site).
3. Confirm whether `StackParams.RevealSecrets` should be a top-level field or nested — I recommend top-level bool consistent with `SkipRefresh`/`SkipPreview` siblings already on that struct.
4. Hand to Developer with: the two leak-path bug (string-value branch bypasses the `Secret` check entirely; the `Secret` branch still marshals the real value) — both must be fixed, not just the obvious one.
5. QA should verify both leak paths are closed, not just the happy path the brief's prose describes (kubeconfig-as-string is exactly the string-value leak path — the bug that actually fired in the k3s smoke).

**Empty-PR-OK:** no — this run should produce a real downstream code change; it is not a verification-only turn.

**Verdict:** signoff