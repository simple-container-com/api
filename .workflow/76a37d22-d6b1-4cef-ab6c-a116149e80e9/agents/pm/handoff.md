[thinking] # PM Handoff — `sc-stack-outputs-empty-byo-vm-host-fields` (2nd dispatch attempt)

## 0. Investigation summary (grounds the plan below)

I read the actual source before scoping, because the brief's own root-cause hypothesis ("the `byo` path takes a different branch than the VM-creation path") does not hold up against the current code — there is currently only **one** implemented VM driver (`byo`); `cloudru-vm`/`yc-vm`/etc. all return `"no vm driver for provider %q"` (`pkg/clouds/pulumi/vm/driver.go`). There is no second, VM-creation branch to diverge from yet.

What I found instead:

- `pkg/clouds/pulumi/vm/vm.go` (`Vm()`) exports host-port and host-count **unconditionally for every driver**, including `byo`:
  ```go
  ctx.Export(ToHostPortExport(input.Descriptor.Name, i), sdk.Int(h.Port))
  ...
  ctx.Export(ToHostCountExport(input.Descriptor.Name), sdk.Int(len(hosts)))
  ```
  These exports are real `sdk.Int` values — they are NOT skipped for `byo`. `byo.go`'s `Hosts()` populates `HostFacts.Port` correctly via `h.EffectivePort()`.
- The actual place I believe loses the data is `pkg/clouds/pulumi/preview.go`, `(p *pulumi) toOutputsResult`, which is what `sc stack outputs` (`pkg/cmd/cmd_stack/cmd_outputs.go` → `Provisioner.Outputs` → `OutputsStack`) renders through:
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
      return res   // ← falls through to "" for anything else
  })
  ```
  An `sdk.Int` export round-trips through Pulumi's automation API / JSON checkpoint as a Go `float64` (plain JSON number decoding), which is not a `string`, not a `map[string]any`, and (correctly) not `Secret`. So it falls through every branch and returns the **zero-value `""`**. This exactly matches the reported symptom: `-host-{i}-address`/`-user`/`-host-key` are strings (work fine), `-private-key`/`-private-key-password` are secrets (work via the secret branch), but `-host-{i}-port` (int) and `-vm-host-count` (int) are silently blanked. `-vm-ingress-enabled` (bool, `sdk.Bool`) is exported the same unconditional way and — by the same mechanism — is presumably also affected, though the brief doesn't mention it; Architect should confirm and decide whether to fold it into this fix (same root cause, same one-line type-switch addition, near-zero extra risk).
- A **separate, already-correct** code path exists for inter-stack consumption: `pkg/clouds/pulumi/vm/compose_stack.go`'s `hostFromParent` reads `host-count`/`host-port` via `pApi.GetValueFromStack[any]` and explicitly coerces the `any` (because it already knows Pulumi hands back a `float64` off the JSON checkpoint — see the comment there: *"A number, not an int... GetValueFromStack[int] would panic here"*). That path is NOT the one `sc stack outputs` uses, and is not broken — it already does the coercion. This is strong corroborating evidence that the type-coercion gap is specifically in `toOutputsResult`, not in the export side.

**I am handing this corrected diagnosis to Architect rather than silently narrowing the AC to match the brief's hypothesis** — Architect must independently verify against the branch's current source (don't trust my read blindly, confirm with `git show`/re-read before coding) and confirm the real fix site before Developer touches anything, per the ground rule that PM does not implement.

## 1. Commit + branch identity

No code mutations in this turn. This is a PM planning turn.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none — PM turn produces no file mutations) | — | — |

## 3. Requirements (restated from brief)

1. `sc stack outputs` run against a stack with a `byo` `type: vm` resource must report **real, non-empty** values for that resource's host-port output(s) (`<vmName>-vm-host-{i}-port`) and host-count output (`<vmName>-vm-host-count`).
2. Root cause must be fixed at its actual source, not patched around with a byo-specific special case if the bug is in the shared output-rendering path (it appears to be — see §0).
3. A unit test must pin the fix so it cannot silently regress.

## 4. Acceptance criteria

| AC# | Criterion |
|---|---|
| AC1 | `sc stack outputs` (via `Provisioner.Outputs` → `OutputsStack` → `toOutputsResult` in `pkg/clouds/pulumi/preview.go`, or wherever Architect's investigation lands) returns a non-empty, correctly-formatted value for a `byo` vm resource's `<name>-vm-host-{i}-port` output. |
| AC2 | Same command returns a non-empty, correctly-formatted value for `<name>-vm-host-count`. |
| AC3 | The fix is general (handles any non-string/non-map/non-secret Pulumi output value — at minimum numeric; bool strongly recommended given identical mechanism per §0) rather than a `byo`-only special case, UNLESS Architect's own source read contradicts my diagnosis in §0 — in which case Architect documents the actual root cause and fixes there instead. |
| AC4 | A unit test exists that pins the populated fields for the code path that was actually broken — e.g. a table/unit test on `toOutputsResult` (or equivalent) asserting a numeric (and ideally boolean) `auto.OutputValue` is rendered as its real value, not `""`. If the bug is confirmed instead to be byo-driver-specific, the test instead pins `byoDriver.Hosts()`/`Vm()` output population, matching the brief's own DoD wording. |
| AC5 | No regression to existing string and secret output rendering (existing tests for `-host-{i}-address`, `-host-{i}-private-key`, etc. still pass). |
| AC6 | `make check` (capped `go vet`) is clean; `go test` is scoped ONLY to the touched package(s) — never `go test ./...` or `go build ./...` (guest OOM risk, see §5). |

## 5. Scope decisions

| Item | Status | Notes |
|---|---|---|
| Fix the output-value coercion for numeric types (host-port, host-count) | **MUST** | Core of the slice; directly reported bug. |
| Extend the same fix to boolean outputs (e.g. `ingress-enabled`) while touching this function | **NICE-TO-HAVE** | Same root cause, same function, marginal extra line of code and test — include if Architect/Developer's diff is already touching this switch; do not let it balloon scope or delay the MUST. |
| Unit test pinning the fix | **MUST** | Explicit DoD in the brief. |
| Re-validate my root-cause hypothesis against live source before coding | **MUST** | I read this via read-only tools from this run; Architect/Developer must re-confirm with `git show`/current branch state, not trust my paraphrase blindly. |
| Refactor `toOutputsResult` broadly (e.g. generic reflection-based formatting) | **CUT** | Overkill for a 2-field bug; a targeted type-switch addition is the leanest correct fix. |
| Touching any other VM driver code (`cloudru-vm`, `yc-vm`, etc.) | **CUT** | Not implemented yet, out of scope, zero relation to this bug. |
| Fixing `hostFromParent`'s own int coercion in `compose_stack.go` | **CUT** | Already correct per §0 — do not touch what isn't broken. |

## 6. Operator / infra constraint carried forward (CRITICAL — do not drop)

This is the **2nd dispatch** of this slice. The 1st attempt died to a guest OOM crashloop with zero commits and zero PR, despite a clean Architect signoff — this is a known, documented infra issue (`docs/roadmap/items/api-repo-slices-oom-the-guest.md` in the `forge` repo). Architect must pass this through verbatim to Developer's brief:

- Read `CLAUDE.md` at the repo root FIRST (note: it lives at the repo root of `simple-container-com/api`, not under an `api/` subdirectory — confirmed by listing).
- **NEVER** run `go build ./...`, `go test ./...`, or `make build`/`make build-all` (link step alone needs ~13GB, will OOM a 12GB guest).
- **ONLY** use `make check` (capped `go vet ./...`, ~2.3GB) and narrowly-scoped `go test ./pkg/clouds/pulumi/...` (or the exact touched sub-package) for verification.
- Keep the turn SHORT — minimize resumes; commit as soon as the fix + test are in, don't keep exploring.
- Kill any hanging/excessive-memory command immediately; never retry the same command blind.
- Recognize `exit status 143` / `signal: killed` with empty stderr as guest OOM, not a code problem — do not attempt to "fix" the code in response to that signature.

## 7. Handoff guidance for Architect

1. Re-verify §0's diagnosis against the current branch source yourself (`pkg/clouds/pulumi/preview.go:toOutputsResult`, `pkg/clouds/pulumi/vm/vm.go`, `pkg/clouds/pulumi/vm/byo.go`) before committing to a design — don't take my read as gospel, confirm with your own `read_repository_file`/`git show`.
2. If confirmed: design the minimal type-switch addition to `toOutputsResult` (handle `float64` → format as an integer/number string; optionally `bool` → `"true"`/`"false"`), plus the new unit test asserting a non-string/non-secret numeric value round-trips as its real value.
3. If your independent read contradicts mine (e.g. you find the `byo` path genuinely skips populating `HostFacts.Port`/host count in some case I didn't see, such as a specific k3s or ready.go code path, or a different `OutputsStack` implementation than `pulumi/preview.go`), document that contradiction explicitly in your handoff and fix the real location — do not silently keep my hypothesis if the evidence doesn't support it.
4. Hand off to Developer with the exact file(s)/function(s) to touch, the test to add, and the build-discipline constraints from §6 carried verbatim.
5. `slice_id`: `sc-stack-outputs-empty-byo-vm-host-fields` — include this in your own handoff JSON so downstream characters can resolve the roadmap link.

**Empty-PR-OK:** no — this slice requires a real source fix + test; it is not a verification-only turn.

**Verdict:** signoff