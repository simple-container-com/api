[thinking] # Handoff — PM (Max Warner), run 354c5876

## 0. Restated problem (from brief)

`sc stack outputs` prints **empty strings** for a `byo` vm resource's `host-port` and `host-count` fields. The brief's working hypothesis is "the byo driver's output-struct population is incomplete." I did the code-reading before handing this to Architect — the hypothesis about *where* the bug lives is wrong, and that materially changes scope, so Architect should start from my finding, not from the brief's guess.

## 1. Root-cause finding (code-read, not guesswork)

Traced the full path from provisioning to CLI print:

1. `pkg/clouds/pulumi/vm/vm.go` (`Vm()`) exports both fields correctly for **every** driver, byo included:
   - `ctx.Export(ToHostPortExport(name, i), sdk.Int(h.Port))`
   - `ctx.Export(ToHostCountExport(name), sdk.Int(len(hosts)))`
   These are genuine, non-empty `int` values (`byoDriver.Hosts()` in `pkg/clouds/pulumi/vm/byo.go` sets `Port: h.EffectivePort()` — never empty). **The byo driver populates HostFacts correctly.** This already falsifies the brief's stated hypothesis.

2. `sc stack outputs` → `cmd_stack/cmd_outputs.go` → `provisioner.Outputs` → `pulumi.OutputsStack` (`pkg/clouds/pulumi/preview.go`) → `stackSource.Outputs(ctx)` (Pulumi Automation API, returns `auto.OutputMap`) → **`(p *pulumi) toOutputsResult`**, which is the actual culprit:

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
    return res   // <-- falls through to "" for any other Go type
})
```

The Automation API round-trips Pulumi exports through JSON; a plain (non-secret) JSON number decodes into Go as `float64`, and a bool as `bool` — neither matches the `string` or `map[string]any` branches, and `value.Secret` is false for host-port/host-count (they are plain `sdk.Int`, not `sdk.ToSecret`). So the function silently returns `""` for **any non-string, non-secret, non-map export** — not just byo's. `host-port` (`sdk.Int`) and `host-count` (`sdk.Int`) hit exactly this gap. `ingress-enabled` (`sdk.Bool`) is exposed to the identical bug but wasn't what the smoke caught.

**Why it reads as "byo-specific" in the smoke:** byo is currently the *only* implemented vm driver (cloudru-vm etc. are future work per `vm-matrix.md`/`driver.go`), so every vm-resource stack output observed today is on the byo path — but the defect is in the generic CLI output-serialization function, not in driver-specific code. Scope correction matters: fixing it in `byo.go` would do nothing; the fix belongs in `preview.go`.

## 2. Scope (MoSCoW)

| Item | Priority | Notes |
|---|---|---|
| Fix `toOutputsResult` to render numeric (`float64`/`int`) and `bool` export values as their real value instead of `""` | MUST | `pkg/clouds/pulumi/preview.go` |
| Regression test proving `host-port`/`host-count` (ints) and `ingress-enabled` (bool) round-trip through `toOutputsResult` as non-empty | MUST | new `pkg/clouds/pulumi/preview_test.go` (none exists today) |
| Re-run/targeted smoke confirming `sc stack outputs` against a byo-vm stack shows real values | MUST (per DoD) | live evidence required to claim `status: shipped` |
| Extending the fix to arrays or other exotic export shapes | CUT | no evidence any consumer needs it; out of scope for this slice |
| Reworking `GetValueFromStack`/provisioning-time reads | CUT | unaffected — that path reads `auto.OutputValue.Value` as `any` directly, never through this stringifier; only the CLI-print path is broken |
| Touching `byo.go` / `vm.go` | CUT | confirmed correct already; touching them is wasted motion and risks an unrelated regression |

## 3. Acceptance criteria (restated + mapped to evidence Architect/Developer must produce)

| AC# | Criterion | How to verify |
|---|---|---|
| AC1 | `sc stack outputs` against a stack with a `byo` vm resource reports real, non-empty values for `host-port` and `host-count` | Live smoke output showing the two fields as the actual int values, not `""` |
| AC2 | A smoke re-run or targeted unit/integration test against the byo driver's output struct confirms both fields populated | Prefer the unit test in `preview_test.go` (fast, deterministic) **plus** one live smoke re-run per DoD — don't substitute one for the other, the brief asks for both an automated check and live confirmation |
| AC3 | `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md` `## Status` + `## History` updated with outcome/evidence; `status: shipped` only if live-verified, else `status: code-complete` | Whichever character closes this slice (QA/DevOps) does this edit — flag explicitly who, see below |

## 4. Implementation guidance for Architect

- Fix is narrowly scoped to `(p *pulumi) toOutputsResult` in `pkg/clouds/pulumi/preview.go`. Extend the type switch to cover at minimum `float64` (render via e.g. `strconv.FormatFloat` trimmed to an integer when it has no fractional part, or just `fmt.Sprintf("%v", ...)`) and `bool` (`strconv.FormatBool`). Consider a single fallback `fmt.Sprintf("%v", value.Value)` branch instead of enumerating every Go JSON-decode type, to avoid the same class of gap recurring for a type nobody thought of — Architect's call, but whichever approach is chosen must not regress the existing `string`/`map[string]any`/secret branches (there's no existing test pinning those branches either — a good moment to add coverage for all four cases in one table-driven test).
- This repo is memory-constrained (see `CLAUDE.md`): use `make check` (go vet, 2.3GB) as the full-repo gate, not `go build ./...`. Scope `go test` to `./pkg/clouds/pulumi/...` — don't run the whole module's tests.
- No DB/schema migration, no swagger regen — this is a pure Go logic fix behind an existing CLI command.
- DCO sign-off required on every commit (`git commit -s`), branch protection blocks unsigned commits.

## 5. Roadmap-closure ownership

Whoever lands the fix AND gets the live smoke evidence (likely DevOps, last in the SDLC chain) should flip `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md` to `status: shipped` with a `## History` entry citing the live `sc stack outputs` output. If the live smoke isn't reachable in this run, land the code + unit test and set `status: code-complete` instead — do not overclaim `shipped` without live evidence, per the brief's own instruction.

## 6. slice_id

`slice_id: sc-stack-outputs-empty-byo-vm-host-fields`

---

## Files touched

No code mutations in this turn — PM role, planning only.

| Path | Lines added / removed | What changed |
|---|---|---|
| (none) | — | — |

## Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `sc stack outputs` reports real host-port/host-count for byo | PENDING | Not yet implemented — this is Architect/Developer's work; root cause located at `pkg/clouds/pulumi/preview.go:toOutputsResult` (read via `read_repository_file`, confirmed against `pkg/clouds/pulumi/vm/vm.go` + `byo.go` exports) |
| AC2 | Smoke/unit test confirms population | PENDING | Guidance given above for the test to add; no test exists yet at `pkg/clouds/pulumi/preview_test.go` (confirmed absent via `search_repository_code`) |
| AC3 | Roadmap item Status/History updated with real outcome | PENDING | Current roadmap item (read via `read_repository_file` from `forge` repo, `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md`) still shows `status: in-progress` / dispatch-only History — closure is downstream's job per §5 above |

## Tests run

None — PM turn produces no code, runs no tests. Code-reading only (`read_repository_file` / `search_repository_code` against `simple-container-com/api` on `main`).

**Verdict:** signoff