[thinking] # Architect Handoff — `sc-stack-outputs-empty-byo-vm-host-fields` (2nd dispatch attempt)

## 0. Independent verification of PM's diagnosis

I re-read the current branch source myself (not trusting PM's paraphrase) via `read_repository_file` against `simple-container-com/api` (ref: default branch, confirmed file SHAs below). **PM's corrected diagnosis is right; the brief's own root-cause hypothesis ("byo path diverges from VM-creation path") does not hold** — there is exactly one implemented VM driver (`byoDriver`); other providers return `"no vm driver for provider %q"` in `pkg/clouds/pulumi/vm/driver.go`.

What I confirmed directly:

1. **`pkg/clouds/pulumi/vm/vm.go`** (`Vm()`, sha `5e89d5d3f8f6e4d2c89233753df4791b31d5a216`) exports host-port and host-count **unconditionally for every driver including `byo`**:
   ```go
   ctx.Export(ToHostPortExport(input.Descriptor.Name, i), sdk.Int(h.Port))
   ...
   ctx.Export(ToHostCountExport(input.Descriptor.Name), sdk.Int(len(hosts)))
   ctx.Export(ToIngressEnabledExport(input.Descriptor.Name), sdk.Bool(cfg.IngressEnabled()))
   ```
   These are real, non-nil `sdk.Int`/`sdk.Bool` exports — not skipped for `byo`.

2. **`pkg/clouds/pulumi/vm/byo.go`** (`byoDriver.Hosts()`, sha `037de2ccc4265aa0dd8973cadb123f6ec087a1eb`) populates `HostFacts.Port` correctly via `h.EffectivePort()`. The export-side data is fine.

3. **`pkg/clouds/pulumi/preview.go`** (`(p *pulumi) toOutputsResult`, sha `7167bd6d287e3ad3ee9a8a8ef7cfc99bd806c4d9`) is what `sc stack outputs` renders through (`cmd_outputs.go` → `Provisioner.Outputs` → `pkg/provisioner/preview.go:Outputs` → `pulumi.OutputsStack` → `toOutputsResult`). Its type switch is:
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
       return res   // zero-value "" for anything else
   }),
   ```
   `sdk.Int`/`sdk.Bool` round-trip through the Pulumi automation-API checkpoint as JSON-decoded `float64`/`bool` native Go values — neither `string` nor `map[string]any`, and (correctly) not secret. They fall through every branch to the zero-value `""`. This exactly matches the reported symptom.

4. **Corroboration**: `pkg/clouds/pulumi/vm/compose_stack.go`'s `hostFromParent` reads the SAME exports (`ToHostCountExport`/`ToHostPortExport`) via `pApi.GetValueFromStack[any]`, with an explicit comment: *"A number, not an int — this is the shape a stack reference hands back after a round trip through the checkpoint's JSON... `GetValueFromStack[int]` would panic here."* That path already does the `any`→numeric coercion correctly on the caller side. It is a **different consumer** (inter-stack reference, not `sc stack outputs`) and is not broken — do not touch it.

**Verdict: root cause is confirmed in `toOutputsResult`, not in the `byo` driver.** The fix is a 2-branch addition to one function, not a byo-specific special case — per AC3's own escape clause this is the correct path since my independent read corroborates PM's diagnosis exactly.

## 1. Commit + branch identity

No code mutations in this turn — Architect does not implement. All file contents cited above were read live from the repository in this turn via `read_repository_file` (SHAs quoted per-file, as returned by the tool).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none — Architect turn produces no file mutations) | — | — |

## 3. Design for Developer

### 3.1 Fix site
**`pkg/clouds/pulumi/preview.go`, function `(p *pulumi) toOutputsResult`.** Add two branches to the existing type switch, inserted after the `map[string]any` branch and before the `value.Secret` catch-all (order is safe — the branches are mutually exclusive on `value.Value`'s dynamic type; no export in this codebase currently combines `sdk.Int`/`sdk.Bool` with `sdk.ToSecret`, so there is no interaction with the existing secret-marshal fallback):

```go
} else if f, ok := value.Value.(float64); ok {
    res = strconv.FormatFloat(f, 'f', -1, 64)
} else if b, ok := value.Value.(bool); ok {
    res = strconv.FormatBool(b)
} else if value.Secret {
```

Add `"strconv"` to the import block (currently: `"context"`, `"encoding/json"`, `github.com/pkg/errors`, `github.com/samber/lo`, pulumi `auto`/`apitype`, local `api`/`logger`/`color`).

Why `strconv.FormatFloat(f, 'f', -1, 64)` and not `fmt.Sprintf("%v", f)`: precision `-1` with format `'f'` yields the shortest decimal representation that round-trips exactly — for a whole-number float (every case here: ports and counts are always integers at the export side) it produces `"2222"`, not `"2222.000000"` or scientific notation. This is the one correctness detail worth getting right since a consumer parsing the string back to an int must see a clean integer.

### 3.2 Scope boundary (do not touch)
- `pkg/clouds/pulumi/vm/vm.go`, `byo.go` — already correct, exports are fine.
- `pkg/clouds/pulumi/vm/compose_stack.go`'s `hostFromParent` — already does its own `any`→numeric coercion correctly for its own (different) consumption path. Do not refactor it to share code with `toOutputsResult`; they serve different call sites (`GetValueFromStack[any]` vs. `auto.OutputMap` rendering) and forcing a shared abstraction is overkill for a 2-line fix (I am consciously not over-engineering this one).
- No other VM driver code (`cloudru-vm`, `yc-vm`, etc.) — not implemented, zero relation to this bug.

### 3.3 Unit test (new file: `pkg/clouds/pulumi/preview_test.go`, package `pulumi`)
No `preview_test.go` currently exists in this package (confirmed via code search — zero hits for `preview_test` under `pkg/clouds/pulumi`), so Developer creates it fresh, in-package (white-box), calling the unexported method directly:

```go
package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func TestToOutputsResult_RendersNumericAndBooleanOutputsAsRealValues(t *testing.T) {
	RegisterTestingT(t)

	p := &pulumi{}
	result := auto.OutputMap{
		"my-vm-vm-host-0-port":    {Value: float64(2222), Secret: false},
		"my-vm-vm-host-count":     {Value: float64(1), Secret: false},
		"my-vm-vm-ingress-enabled": {Value: true, Secret: false},
		// regression guard: existing string/secret rendering must be untouched (AC5)
		"my-vm-vm-host-0-address": {Value: "203.0.113.5", Secret: false},
	}

	out := p.toOutputsResult("my-stack", result)

	Expect(out.Outputs["my-vm-vm-host-0-port"]).To(Equal("2222"))
	Expect(out.Outputs["my-vm-vm-host-count"]).To(Equal("1"))
	Expect(out.Outputs["my-vm-vm-ingress-enabled"]).To(Equal("true"))
	Expect(out.Outputs["my-vm-vm-host-0-address"]).To(Equal("203.0.113.5"))
}
```

Developer should check the exact Gomega import path/convention used elsewhere in this package (other `_test.go` files under `pkg/clouds/pulumi/vm/` use `. "github.com/onsi/ginkgo"`-style `RegisterTestingT(t)` + Gomega `Expect` — follow whatever `pkg/clouds/pulumi/create_stack_test.go` already imports for consistency, since that file is the one existing test in this exact package directory).

This pins the actually-broken code path (`toOutputsResult`) rather than the brief's byo-driver hypothesis, per AC4's own escape clause — and it is lower cost than standing up a full Pulumi mock program + fake automation stack to exercise `OutputsStack` end-to-end, which would be scope creep for a type-switch fix.

### 3.4 Acceptance criteria mapping (for Developer to verify, not yet verified by me — no code mutations this turn)

| AC# | Criterion | How Developer verifies |
|---|---|---|
| AC1 | `-vm-host-{i}-port` renders non-empty | New unit test assertion `Equal("2222")` |
| AC2 | `-vm-host-count` renders non-empty | New unit test assertion `Equal("1")` |
| AC3 | Fix is general (float64 at minimum, bool included) in the shared path, not byo-specific | Both branches land in `toOutputsResult`, used by every resource type, not gated on driver |
| AC4 | Unit test pins the actually-broken path | `preview_test.go` added, calls `toOutputsResult` directly |
| AC5 | No regression to string/secret rendering | Same test asserts the string-typed output still renders correctly |
| AC6 | `make check` clean; test scoped to touched package only | `make check` + `go test ./pkg/clouds/pulumi/... -run TestToOutputsResult` (or just the new test name) — **never** `go test ./...` / `go build ./...` |

## 4. Tests run

None — Architect turn, no code touched, so no build/test commands were run against this fix. Developer must run the scoped commands in §3.4 AC6 and paste real output in their handoff.

## 5. Build-discipline constraint carried forward verbatim (CRITICAL — 2nd dispatch, 1st died to guest OOM)

- Read `CLAUDE.md` at the repo root FIRST (confirmed present at root, not under `api/`).
- **NEVER** run `go build ./...`, `go test ./...`, or `make build`/`make build-all` (link step alone needs ~13GB, will OOM a 12GB guest with zero swap — confirmed in `CLAUDE.md`'s measured table: `make build` = 12.9GB peak RSS).
- **ONLY** use `make check` (capped `go vet ./...`, measured 2.3GB peak) and a narrowly-scoped `go test ./pkg/clouds/pulumi/...` (just this package, not `./pkg/clouds/pulumi/vm/...` since the fix doesn't touch that subpackage) for verification.
- Keep the turn SHORT — minimize resumes; commit as soon as the fix + test are in, don't keep exploring.
- Kill any hanging/excessive-memory command immediately; never retry the same command blind.
- Recognize `exit status 143` / `signal: killed` with empty stderr as guest OOM, not a code problem — do not attempt to "fix" the code in response to that signature.
- `gofumpt -l -w ./pkg/clouds/pulumi/` + `golangci-lint run --fix ./pkg/clouds/pulumi/...` (scoped, not whole-module) before committing, per `CLAUDE.md`'s pre-push section.
- Commits need a DCO sign-off (`git commit -s`) — branch protection requires it plus human review; nothing self-merges.

## 6. Roadmap linkage

`slice_id`: `sc-stack-outputs-empty-byo-vm-host-fields` — carry this through so downstream characters can resolve `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md` in the `forge` repo.

**Empty-PR-OK:** no — this slice requires a real source fix + test; it is not a verification-only turn.

**Verdict:** signoff