[thinking] # Handoff — PM (Max Warner), run fadc2560-8262-4860-bab6-19a04d636919

## 0. Context recap

Roadmap item: `sc-stack-outputs-leaks-secrets-in-clear` (repo: `simple-container-com/api`, item file lives in the **`forge`** repo at `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md` — not in `api`, confirmed by listing both repos). This is the **2nd dispatch** of this slice; the 1st (run `32521357`) crashed the developer character 3× on `exit status 143` (OOM/watchdog) before producing any commit or PR. No reusable code exists from that attempt.

I read `api`'s `CLAUDE.md` memory-cap guidance and the current implementation to ground the plan in real code rather than guesswork — see §3 "Findings for Architect" below.

## 1. Commit + branch identity

No code mutations in this turn. (PM role — planning only.)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|

(Empty — PM turn produces no source-file mutations. This is expected per the handoff contract.)

## 3. Restated requirements

`sc stack outputs` (cobra command `pkg/cmd/cmd_stack/cmd_outputs.go`, `NewOutputsCmd`) prints every resource output in the clear via `json.Marshal` of `api.OutputsResult.Outputs`. There is **no masking today** for outputs the underlying Pulumi SDK marks `Secret` (`auto.OutputValue.Secret`). Confirmed in code: `pkg/clouds/pulumi/preview.go`, function `toOutputsResult` — it unconditionally resolves `value.Value` to a plain string (or, when `value.Secret` is true but the value isn't a string/map, JSON-marshals the whole `OutputValue` struct, which **still embeds the secret's own value in the clear** — so today there is no masking path at all, not even accidental). Per the roadmap item's own prose, `--show-secrets` does **not exist yet in code** (I verified with a repo-wide search — zero hits outside docs/prompts referencing it as a planned/missing flag); the current behavior is "always show, always plaintext, no flag."

Task: add mask-by-default for secret-marked resource outputs, with an explicit opt-in flag to reveal plaintext — mirroring `sc secrets reveal`/`sc secrets hide` (`pkg/cmd/cmd_secrets/cmd_secrets.go`, `NewHideCmd`/`NewRevealCmd`).

## 4. Acceptance criteria (restated, numbered for downstream traceability)

| AC# | Criterion |
|---|---|
| AC1 | `sc stack outputs` masks any output where the underlying Pulumi value is secret-marked (`auto.OutputValue.Secret == true`), showing a fixed-width placeholder (e.g. `<secret:redacted>`) instead of the real value, **by default**, with no flag. |
| AC2 | A new opt-in flag (recommend `--show-secrets`, since it does not exist yet and the name already matches the roadmap item's own language and the `sc secrets reveal`/`hide` naming convention) reveals the real plaintext value when passed. |
| AC3 | Masking is implemented at (or funnels through) the same place/mechanism the CLI already uses for redaction elsewhere (see `pkg/api/logger/redact.go`'s `Redact()` and `pkg/clouds/pulumi/preview.go`'s existing `logger.Redact(result.StdOut)` pattern on Preview/Update results) — i.e. don't invent a parallel bespoke masking mechanism if the existing one can be reasonably extended; if it can't (string-replace redaction vs. key-based secret-flag masking are genuinely different problems), that's fine, but Architect should note why in the design. |
| AC4 | A unit test proves: (a) a `Secret:true` `auto.OutputValue` is masked in `OutputsStack`/`toOutputsResult` output by default, (b) a non-secret output is untouched, (c) the opt-in flag reveals the real value. Mirrors the existing pattern in `pkg/clouds/pulumi/kubernetes/credentials_test.go`'s `TestProvider_KubeconfigIsSecret` and `pkg/clouds/pulumi/preview_redact_test.go`'s `TestPreviewAndUpdateSummariesAreRedacted`. |
| AC5 | A live smoke re-run against a stack with a secret-marked output (the k3s cluster resource that caught this live, per `docs/docs/examples/k3s/byo-host/README.md`) confirms: default invocation → masked; `--show-secrets` → real value. |
| AC6 | `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md` in the **`forge`** repo gets its `## Status` + `## History` updated with outcome + evidence — `status: shipped` only if AC5 is live-verified; `status: code-complete` if code lands (AC1–AC4) but AC5 (live smoke) isn't done in this run. **Note the item file is in `forge`, not `api`** — cross-repo, same caveat the prior Driver handoff flagged for a sibling item. |
| AC7 | `docs/docs/examples/k3s/byo-host/README.md` (in `api`) — the passage explicitly stating *"There is no `--show-secrets` and no `-o`. Secret-marked outputs are printed in the clear, always"* is now **false** once this ships; update it to describe the new default-masked / `--show-secrets` behavior so the doc doesn't actively mislead the next operator. |

## 5. Scope decisions (MoSCoW)

| Item | Priority | Rationale |
|---|---|---|
| Mask secret-marked outputs by default in `toOutputsResult` / `OutputsStack` | **MUST** | This is the actual vulnerability — the leak happens at serialization, not at display. |
| `--show-secrets` opt-in flag on `sc stack outputs` | **MUST** | Without it the command loses legitimate functionality (operators sometimes genuinely need the kubeconfig). |
| Unit test(s) proving default-mask + opt-in-reveal | **MUST** | Non-negotiable per repo's own test culture (see `credentials_test.go`, `preview_redact_test.go` precedent) — this exact class of regression (secret printed unmarked) has bitten this repo before (`yc-provider-key-in-deploy-log.md`), so a test is cheap insurance against reintroduction. |
| Update the one doc (`byo-host/README.md`) that explicitly documents today's leaky behavior | **MUST** | Leaving it would actively mislead the next reader into believing the leak still exists (or worse, into assuming `--show-secrets` masks when it actually reveals). |
| Live smoke against a real k3s/byo stack | **NICE-TO-HAVE at code time, MUST for `status: shipped`** | Requires live cloud infra/credentials DevOps may or may not hold in this run. If unavailable, land as `code-complete` honestly — do not fabricate a smoke. |
| Masking format bikeshedding (exact placeholder string, color, etc.) | **CUT** | `<secret:redacted>` is fine, ship it, don't debate fixed-width padding or ANSI styling. |
| Extending masking to `sc stack secret-get` or other commands | **CUT — out of scope** | Brief is scoped to `sc stack outputs` only. `secret-get` already requires the caller to name the exact secret key, which is a different (intentional, single-value) disclosure model — do not touch it. |
| A generic "secrets-aware serialization" framework across the whole CLI | **CUT — out of scope** | Classic over-engineering; solve the one command that leaks, not a hypothetical future one. |

## 6. Findings for Architect (technical starting point — not a design, just where the wiring currently is)

- Entry point: `pkg/cmd/cmd_stack/cmd_outputs.go` → `NewOutputsCmd`. Needs a new `bool` flag registered (`cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, ...)`), analogous to how `cmd_destroy.go` registers `--with-secrets`.
- `sCmd.Params` is `api.StackParams` (`pkg/api/client.go`) — the cleanest path is almost certainly adding a field there (e.g. `ShowSecrets bool`) so it flows through `Root.Provisioner.Outputs(ctx, sCmd.Params)` → `provisioner.Outputs` → `pulumi.OutputsStack(ctx, cfg, stack, params)` → `toOutputsResult`, the same params-threading pattern already used for `SkipRefresh`/`SkipPreview`/`DetailedDiff`.
- The actual masking decision point is `pkg/clouds/pulumi/preview.go`, `toOutputsResult`: today it discards `value.Secret` entirely for the string case (the common case — a kubeconfig is a string). The fix needs `toOutputsResult` (or its caller) to take the reveal flag and, when false, replace any `value.Secret == true` entry's resolved string with a placeholder BEFORE marshaling — not after, since the bug here is literally "the secret's bytes reach the string that gets marshaled."
- Precedent for the masking format and "why" comment style: `pkg/clouds/pulumi/kubernetes/credentials.go` and `pkg/clouds/pulumi/k3s/cluster.go` carry exactly this kind of "this leaked before, here's why we wrap it" comment — Architect should follow that convention when touching `toOutputsResult`.
- Test precedent: `pkg/clouds/pulumi/preview_redact_test.go` and `pkg/clouds/pulumi/kubernetes/credentials_test.go` — same package, same style of "construct a fake secret, assert it never appears unmasked in the output struct."

## 7. Resource-exhaustion guidance (carried forward from the brief's own warning)

The prior attempt died 3× on `exit status 143` running `go vet`/`go build` under memory pressure (documented root cause in `api`'s `CLAUDE.md`: Pulumi SDK compile is 2.3 GB for `make check` vs 12.9 GB for a link). Architect/Developer: scope all builds to the touched packages —

```sh
GOMAXPROCS=1 GOMEMLIMIT=6GiB go vet ./pkg/clouds/pulumi/... ./pkg/cmd/cmd_stack/... ./pkg/api/...
GOMAXPROCS=1 GOMEMLIMIT=6GiB go test ./pkg/clouds/pulumi/ ./pkg/cmd/cmd_stack/...
```

— never `go build ./...` / `go test ./...` on this module from a constrained sandbox. Do not link the `sc` binary locally; let CI do it. If a scoped command still dies with a memory-kill signature (`143`/`137`, empty stderr), report it honestly as non-completion per the repo's own documented incident — do not silently pass.

## 8. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1–AC7 | (see §4) | N/A — not yet implemented | This is a PM planning turn; implementation belongs to Architect → Developer → QA → DevOps. No code exists yet to verify against. |

## 9. Tests run

None — no code mutated this turn.

## 10. Handoff guidance for Architect

- Produce the technical design for: (a) threading a `ShowSecrets`/reveal flag from the new CLI flag through `api.StackParams` to `toOutputsResult`; (b) the exact masking function/placeholder and where it lives (recommend colocating near `pkg/api/logger/redact.go` or directly in `preview.go`, Architect's call); (c) the unit test plan per AC4.
- Flag the doc update (AC7, `docs/docs/examples/k3s/byo-host/README.md`) as a Developer-owned doc change alongside the code, not a separate task.
- Flag AC5 (live smoke) as DevOps/QA-owned, contingent on live k3s/byo stack access — if that access isn't available in this chain, land as `status: code-complete` on the roadmap item (AC6) rather than overclaiming `shipped`.
- Remember the item's canonical status file lives in **`forge`**, not `api` — don't edit a non-existent `api/docs/roadmap/items/...` path.

**Verdict:** signoff