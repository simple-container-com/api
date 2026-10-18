[thinking] # Handoff — Dan Johnson (Architect), run 32521357-2668-4c22-ba01-607e7fca2c71

## 0. Roadmap linkage
`slice_id: sc-stack-outputs-leaks-secrets-in-clear` — roadmap item in `forge` repo (`docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`), `status: in-progress`. This is the architecture turn for its dispatched implementation run in the `api` repo, following PM (Max Warner)'s two planning handoffs. I re-verified PM's findings directly against the repo rather than trusting them blind — they check out.

## 1. Commit + branch identity
No code mutations in this turn — Architect role, design only. All evidence below is read directly from `simple-container-com/api` via `read_repository_file`/`search_repository_code` against commit `0a008cdeb9353785ee516148f7652e0edb976aaa` (the blob SHAs returned per-file are cited inline, not fabricated).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none) | — | Architect turn produces design only; no repo writes |

## 3. Verified findings (confirms PM's read, with exact evidence)

- `pkg/clouds/pulumi/preview.go` (blob sha `7167bd6d287e3ad3ee9a8a8ef7cfc99bd806c4d9`) — `toOutputsResult` is the single leak site, confirmed as the only hit for the symbol in the repo (`search_repository_code` for `toOutputsResult` → 1 result, this file). Current signature: `func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult`. Body:
  ```go
  Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
      var res string
      if s, ok := value.Value.(string); ok {
          res = s
      } else if m, ok := value.Value.(map[string]any); ok {
          j, _ := json.Marshal(m)
          res = string(j)
      } else if value.Secret {
          j, _ := json.Marshal(value)
          res = string(j)
      }
      return res
  }),
  ```
  Confirms PM's bug shape exactly: `value.Secret` is checked only in the `else if` tail, so any secret-marked output whose `Value` is a `string` or `map[string]any` bypasses the secret check entirely (string case is exactly the kubeconfig leak from the k3s smoke). The one branch that does look at `.Secret` still marshals the whole `OutputValue` struct, which embeds the plaintext `Value` — so that path leaks too, just via `json.Marshal(value)` instead of a direct string cast. **Both paths are unconditional plaintext; `value.Secret` currently gates nothing.**
- `pkg/clouds/pulumi/preview.go` — sole call site: `OutputsStack` → `p.toOutputsResult(stackSource.Name(), res)`. No other callers anywhere in the repo (confirmed by the same single-hit search).
- `pkg/provisioner/preview.go` (sha `dd96d3b94f0744dda00087f6328adf93dc8594e8`) — `(*provisioner).Outputs(ctx, params api.StackParams) (*api.OutputsResult, error)` is the layer above `OutputsStack`; it has two branches (child-stack path via `pv.OutputsStack(ctx, cfg, *stack, params)`, parent-stack path via `pv.OutputsStack(ctx, cfg, stack, params)`) — both already thread the full `params api.StackParams` value through unchanged, so a new field on `StackParams` reaches `OutputsStack` for free. No signature change needed at this layer.
- `pkg/cmd/cmd_stack/cmd_outputs.go` (sha `30605bfc6b78a49fc4406a08a4bc68a205be60d4`) — `NewOutputsCmd` registers **zero** flags of its own; it calls `sCmd.Root.Provisioner.Outputs(cmd.Context(), sCmd.Params)` then `json.Marshal`s and prints the result unconditionally. `sCmd.Params` is the shared `api.StackParams` populated by `root_cmd.RegisterStackFlags` (registered persistent on the parent `stack` cobra command in `cmd_stack.go`, sha `9817964237e7b78af6f7a872816fe233918781bc`). This confirms there is genuinely no `--show-secrets` flag anywhere in code today.
- `pkg/cmd/root_cmd/root.go` (sha `9865d593e1b24e77af241b1c10239c5db1d92b8c`) — `RegisterStackFlags` is the one place binding `StackParams` fields to cobra flags (`--profile`, `--stack`, `--env`, `--dir`, `--skip-refresh`, `--skip-preview`, `--diff`/`--detailed-diff`). This is a shared flag set across every stack subcommand (`outputs`, `secret-get`) — adding a reveal flag HERE would leak it onto `secret-get` too, which is explicitly out of scope (PM item 8, CUT). **The reveal flag must be registered locally inside `NewOutputsCmd`, not added to `RegisterStackFlags`.**
- `pkg/api/client.go` — confirmed `StackParams` struct (fetched via raw GitHub at the same commit, matches the file tree read):
  ```go
  type StackParams struct {
      StacksDir    string   `json:"stacksDir" yaml:"stacksDir"`
      StackDir     string   `json:"stackDir" yaml:"stackDir"`
      Profile      string   `json:"profile" yaml:"profile"`
      StackName    string   `json:"stack" yaml:"stack"`
      Environment  string   `json:"environment" yaml:"environment"`
      ParentEnv    string   `json:"parentEnv" yaml:"parentEnv"`
      SkipRefresh  bool     `json:"skipRefresh" yaml:"skipRefresh"`
      SkipPreview  bool     `json:"skipPreview" yaml:"skipPreview"`
      DetailedDiff bool     `json:"detailedDiff" yaml:"detailedDiff"`
      Version      string   `json:"version" yaml:"version"`
      Timeouts     Timeouts `json:",inline" yaml:",inline"`
      Parent       bool     `json:"parent" yaml:"parent"`
  }
  ```
  Also confirmed `api.OutputsResult` (`pkg/api/client.go`, via `search_repository_code`): `type OutputsResult struct { StackName string; Outputs map[string]any }` — the `Outputs` map's value type is already `any`, so a placeholder string slots in with no struct change.
- `pkg/cmd/cmd_secrets/cmd_reveal.go` (sha `ca92a53e2f0a14ccfbb5f4c97e0fe387e14ac634`) / `cmd_hide.go` (sha `8e81b402376261adb014982d17171d0c9bc9372c`) — confirmed pattern: a plain local `bool` captured by closure, bound with `cmd.Flags().BoolVarP(&forceReveal, "force", "F", forceReveal, "...")`, consumed directly in `RunE`. No struct field needed for `secrets reveal`/`hide` because they take no other params — `stack outputs` differs in already carrying `api.StackParams` end-to-end, so the struct-field approach (vs. a bare closure var) is correct for this command specifically, to survive the `Outputs(ctx, params)` → `OutputsStack(ctx, cfg, stack, params)` → `toOutputsResult(...)` hop.
- Docs: both `docs/docs/examples/k3s/byo-host/README.md` and `docs/docs/examples/vm-compose/byo-host/README.md` fetched and read in full. Confirmed: the k3s doc's "Checking the cluster by hand" section literally instructs `sc outputs -s mycompany -e staging --show-secrets -o json | jq ...` — a flag and a `-o` output-format option that **both** don't exist in `cmd_outputs.go` today (no `-o`/`--output` flag registered either — worth noting to Developer, it's a second pre-existing doc/code mismatch, not in scope to fix but shouldn't be papered over by the reveal-flag naming). The vm-compose doc only shows `sc stack outputs -s portal -e staging | jq ...` with no secrets involved, so it needs no change.

## 4. Architectural decisions (ADRs, numbered for traceability)

**ADR-1 — Masking happens in `toOutputsResult`, not at the `cmd` print layer.**
Why: `toOutputsResult` is the only place that has `auto.OutputValue.Secret` in scope; by the time `cmd_outputs.go` has the `*api.OutputsResult`, the type information (`any`) is gone — you cannot tell a masked string from a legitimate string output after the fact. Masking must happen where the Pulumi SDK type is still visible. Rejected alternative: mask in `cmd_outputs.go` based on a parallel "which keys are secret" list — adds a second channel that can drift from the actual Pulumi-reported secret-ness; strictly worse than branching once, in the one function that already inspects `.Secret`.

**ADR-2 — New field `RevealSecrets bool` on `api.StackParams`, not a new parameter threaded manually through 3 call sites.**
Why: `StackParams` already flows unchanged from `cmd_outputs.go` → `provisioner.Outputs` → `pulumi.OutputsStack` (confirmed above — no layer re-maps fields). Adding it to the struct means **zero signature changes** at `Outputs` or `OutputsStack`; only `toOutputsResult`'s signature changes (it currently takes `(stackName string, result auto.OutputMap)`, not the whole `params`). Rejected alternative: add a bare `revealSecrets bool` parameter to `OutputsStack`/`Outputs` — functionally equivalent but touches two more call sites for no benefit, and breaks the one other implementer of the `Provisioner.OutputsStack` interface method (`PulumiMock` in `pkg/clouds/pulumi/mocks/pulumi_mock.go`, and the `noopProvisioner` test fake in `pkg/api/testhelpers_test.go`) unnecessarily — those fakes already satisfy `OutputsStack(ctx, cfg, stack, params)` and need no edit if the new bool rides inside `params`.

**ADR-3 — `toOutputsResult` signature becomes `toOutputsResult(stackName string, result auto.OutputMap, revealSecrets bool) *api.OutputsResult`.**
Why: minimal, explicit, matches PM's proposal exactly, keeps the function pure/testable (no need to construct a full `StackParams` in a unit test — a bare bool is enough). `OutputsStack` passes `params.RevealSecrets` at its one call to `toOutputsResult`.

**ADR-4 — Flag name: `--reveal-secrets`, registered locally in `NewOutputsCmd` only. No `--show-secrets` alias.**
Why (confirming PM's lean-CUT recommendation, now a firm decision): `--show-secrets` never existed in code — only in two stale doc lines, one of which (the k3s doc) is already factually broken in a second way (`-o json` also doesn't exist). Shipping a back-compat alias for a flag that never shipped isn't back-compat, it's validating stale docs instead of fixing them. Fixing the one doc line (k3s README) is strictly cheaper and leaves zero dead flag surface. Decision: **CUT** item 7 from PM's list. Developer updates the k3s doc's example command to `sc outputs -s mycompany -e staging --reveal-secrets | jq -r '.["ru-cluster--staging-kubeconfig"]'` — also drop the fabricated `-o json` (plain `jq` on the default JSON stdout works identically; `cmd_outputs.go` already unconditionally prints JSON).

**ADR-5 — Placeholder string: `<secret:redacted>`, applied identically regardless of which of the two leak paths would have fired.**
Why: distinctive, greppable, matches the brief's own suggestion, and applying one constant to both paths (rather than one per path) keeps `toOutputsResult` simple — the fix is "check `value.Secret` FIRST, before any type switch on `.Value`", not "patch each existing branch individually". Concretely:
```go
func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap, revealSecrets bool) *api.OutputsResult {
	return &api.OutputsResult{
		StackName: stackName,
		Outputs: lo.MapValues(result, func(value auto.OutputValue, key string) any {
			if value.Secret && !revealSecrets {
				return "<secret:redacted>"
			}
			var res string
			if s, ok := value.Value.(string); ok {
				res = s
			} else if m, ok := value.Value.(map[string]any); ok {
				j, _ := json.Marshal(m)
				res = string(j)
			} else if value.Secret {
				// revealSecrets == true here (guarded above) — fall through to
				// marshal the raw value for non-string/non-map secret outputs.
				j, _ := json.Marshal(value.Value)
				res = string(j)
			}
			return res
		}),
	}
}
```
Note I also fixed a latent second bug while redesigning: the existing `else if value.Secret` branch does `json.Marshal(value)` (the whole `OutputValue`, which carries a `Secret bool` field alongside `Value`) rather than `json.Marshal(value.Value)` — cosmetically it still "worked" before because the output was always being shown regardless, but once `revealSecrets` can legitimately be true for this type shape, marshaling the wrapper struct would emit `{"Value":...,"Secret":true}` noise into the revealed output instead of the clean value. Flag this explicitly to Developer as a one-line fix bundled into the same block, not a separate PR.

## 5. Scope confirmation (ratifying PM's table, no changes)

| # | Item | Decision |
|---|---|---|
| 1 | `RevealSecrets bool` field on `StackParams` | **MUST**, per ADR-2 |
| 2 | `--reveal-secrets` flag, local to `NewOutputsCmd` | **MUST**, per ADR-4 |
| 3 | Fix both leak paths via secret-check-first in `toOutputsResult` | **MUST**, per ADR-5 |
| 4 | Thread `RevealSecrets` via `StackParams` (no extra params needed beyond `toOutputsResult`) | **MUST**, per ADR-2/3 |
| 5 | Unit tests: non-secret unaffected / secret masked by default / secret revealed with flag, covering both leak-path shapes (string value, non-string/map value) | **MUST** |
| 6 | Fix k3s byo-host doc's `--show-secrets -o json` → `--reveal-secrets` (drop fabricated `-o`) | **MUST** |
| 7 | `--show-secrets` back-compat alias | **CUT** — decided per ADR-4, not "lean toward," final |
| 8 | Extend masking to `sc stack secret-get` | **CUT** — confirmed, different code path (`stack.Secrets.Values`, not Pulumi outputs), different risk profile, out of brief's scope |
| 9 | Roadmap status flip in `forge` repo | **Deferred to closing character** (Developer/QA with a merged PR + green tests in hand) — not actionable from an `api`-repo code-design turn |

## 6. Test design for Developer

Target file: `pkg/clouds/pulumi/preview_test.go` (new — I could not locate an existing test file for `preview.go` via `search_repository_code` in this repo, 0 hits for the pattern; Developer should `ls pkg/clouds/pulumi/*_test.go` to confirm before assuming greenfield).

Minimum cases, each exercising `toOutputsResult` directly (no Pulumi automation-API stack needed — `auto.OutputMap` is a plain `map[string]auto.OutputValue`, constructible in-test):

1. `non-secret string output` → `revealSecrets=false` → plaintext; `revealSecrets=true` → same plaintext (unaffected either way).
2. `secret string output` (the kubeconfig-shape leak) → `revealSecrets=false` → `<secret:redacted>`; `revealSecrets=true` → real plaintext.
3. `secret non-string output` (e.g. `auto.OutputValue{Value: 12345, Secret: true}` — the join-token-as-non-string shape, or any type that isn't `string`/`map[string]any`) → `revealSecrets=false` → `<secret:redacted>`; `revealSecrets=true` → marshaled real value (verify it's `json.Marshal(value.Value)`, not the wrapper struct — catches the latent bug in ADR-5).
4. `secret map[string]any output` → same masked/revealed pair, confirming the map branch is also gated.

Per `CLAUDE.md`'s memory guidance, run scoped: `GOMAXPROCS=1 GOMEMLIMIT=6GiB go test -p 1 ./pkg/clouds/pulumi/... -run TestToOutputsResult` — do not run bare `go test ./...` in this module (measured OOM risk documented in the repo's own CLAUDE.md, confirmed by direct read of that file this turn, sha `626ed920ef80bed5631ddb3fc8707d3b224790f2`).

Lint: scope to touched files only — `bin/gofumpt -l -w ./pkg/clouds/pulumi/preview.go ./pkg/clouds/pulumi/preview_test.go ./pkg/cmd/cmd_stack/cmd_outputs.go ./pkg/api/client.go` then `bin/golangci-lint run --fix ./pkg/clouds/pulumi/... ./pkg/cmd/cmd_stack/...` — per CLAUDE.md, whole-module lint is itself OOM-risky.

Commit: needs DCO sign-off (`git commit -s`) — CLAUDE.md confirms `.github/workflows/dco.yml` enforces this and branch protection requires human review (no self-merge on this repo).

## 7. Acceptance criteria verification (brief's numbered ACs — Architect turn, design-level verification only; code-level PASS is Developer's to earn)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Find where `sc stack outputs` renders outputs and how it decides display | PASS (design-verified) | `toOutputsResult` in `pkg/clouds/pulumi/preview.go` (sha `7167bd6d287e3ad3ee9a8a8ef7cfc99bd806c4d9`), called from `OutputsStack` same file; invoked via `cmd_outputs.go` (sha `30605bfc6b78a49fc4406a08a4bc68a205be60d4`) → `Provisioner.Outputs`. No flag exists in code today — confirmed by reading `RegisterStackFlags` (root.go) and `NewOutputsCmd` directly; `--show-secrets` exists only as stale doc prose |
| AC2 | Add masking by default + explicit opt-in flag | Designed, not yet implemented | ADR-1 through ADR-5 above give Developer the exact signature changes and code block; no code written this turn (Architect role) |
| AC3 | Mirror `sc secrets reveal`/`hide` pattern | PASS (design-verified) | `cmd_reveal.go` (sha `ca92a53e2f0a14ccfbb5f4c97e0fe387e14ac634`)/`cmd_hide.go` (sha `8e81b402376261adb014982d17171d0c9bc9372c`) read in full; same flag-gates-plaintext spirit applied via `StackParams.RevealSecrets` + local `--reveal-secrets` flag (ADR-2/4) |
| AC4 | Tests: non-secret unaffected / secret masked by default / reveal flag unmasks | Designed, not yet implemented | Test matrix in §6 above (4 cases covering both leak-path value shapes); file target and exact `go test` invocation given |
| AC5 (DoD-1) | `sc stack outputs` doesn't print secret output unless opted in | Pending implementation | Will be PASS once Developer lands ADR-5's code block |
| AC6 (DoD-2) | Test/smoke demonstrates default-masked / opt-in-revealed | Pending implementation | Test matrix in §6 satisfies this once written and green |
| AC7 (DoD-3) | Record outcome in `forge` roadmap item | Deferred | Not actionable without a merged PR + test evidence in hand — belongs to the closing (Developer/QA) character, same as PM flagged |

## 8. Handoff guidance for Developer

1. Implement exactly the `toOutputsResult` block in ADR-5 — note the bundled fix (`json.Marshal(value.Value)` not `json.Marshal(value)`) is a real, separate latent bug caught during this design pass; call it out in your own handoff so QA knows to check it specifically, not just the masking happy path.
2. Add `RevealSecrets bool \`json:"revealSecrets" yaml:"revealSecrets"\`` to `StackParams` in `pkg/api/client.go`, alongside `SkipRefresh`/`SkipPreview` (same block, same style).
3. In `NewOutputsCmd` (`pkg/cmd/cmd_stack/cmd_outputs.go`), add `cmd.Flags().BoolVar(&sCmd.Params.RevealSecrets, "reveal-secrets", false, "Reveal secret-marked stack outputs in plaintext (default: masked)")` — bind directly to the struct field already on `sCmd.Params`, no local closure var needed (unlike `cmd_reveal.go`/`cmd_hide.go`, which have no persistent params struct to bind into).
4. Update `OutputsStack` in `preview.go`'s one call site: `p.toOutputsResult(stackSource.Name(), res, params.RevealSecrets)`.
5. Check `pkg/clouds/pulumi/mocks/pulumi_mock.go` and `pkg/api/testhelpers_test.go`'s `noopProvisioner.OutputsStack` — confirm both still compile unchanged (they implement the interface's existing 4-arg shape `(ctx, cfg, stack, params)`, which doesn't change — only `toOutputsResult`'s internal signature changes, and it's unexported/package-private to `pulumi`, so no mock regen needed there). Verify this with `go vet ./pkg/clouds/pulumi/...` after your edit, not by assumption.
6. Fix the k3s byo-host doc line per ADR-4 (drop `--show-secrets`, drop fabricated `-o json`).
7. Do not touch `cmd_secret_get.go` (CUT, item 8) — different code path, different risk profile, out of brief's scope.
8. Your own handoff must carry a REAL commit SHA from `git log -1` against `pkg/clouds/pulumi/preview.go` after you push, per this workflow's anti-fabrication rule — I observed the pre-change blob SHA above specifically so you have a before/after diff anchor.

**Empty-PR-OK:** no — this run should produce a real downstream code change; it is not a verification-only turn.

**Verdict:** signoff