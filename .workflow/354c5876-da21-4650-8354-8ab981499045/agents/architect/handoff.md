[thinking] # Handoff — Architect (Dan Johnson), run 354c5876

## 0. Scope of this turn

Architect turn. I re-derived PM's root-cause finding independently by reading the actual files (not trusting the prior handoff at face value), confirmed it's correct, and produced the exact fix design + test spec for Developer to implement. **I made no code changes in this turn** — that's Developer's job next. Everything below is grounded in file content I actually read this turn via `read_repository_file` / `search_repository_code` against `simple-container-com/api`.

## 1. Commit + branch identity

No code mutations in this turn — Architect role, design only. (Blob SHAs cited below are the *current* content hashes of files on `main` at read-time, not commits I made — cited so Developer can confirm they're starting from the same bytes.)

## 2. Root cause — confirmed, PM's finding is correct

I read all three files PM named and the exports they produce:

- **`pkg/clouds/pulumi/vm/vm.go`** (blob `5e89d5d3f8f6e4d2c89233753df4791b31d5a216`) — `Vm()` exports, for every driver including `byo`:
  ```go
  ctx.Export(ToHostPortExport(input.Descriptor.Name, i), sdk.Int(h.Port))
  ...
  ctx.Export(ToHostCountExport(input.Descriptor.Name), sdk.Int(len(hosts)))
  ```
  These are real, non-zero `sdk.Int` values — **the byo driver path is correct.** There is nothing to fix in `vm.go` or `byo.go`.

- **`pkg/clouds/pulumi/preview.go`** (blob `7167bd6d287e3ad3ee9a8a8ef7cfc99bd806c4d9`) — `(p *pulumi) toOutputsResult`, the function that serializes `auto.OutputMap` (the Pulumi Automation API's JSON-round-tripped output values) for `sc stack outputs` to print:

  ```go
  func (p *pulumi) toOutputsResult(stackName string, result auto.OutputMap) *api.OutputsResult {
      return &api.OutputsResult{
          StackName: stackName,
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
              return res   // falls through to "" for ANY other Go type
          }),
      }
  }
  ```

  The Automation API decodes a plain Pulumi export through JSON: a number becomes Go `float64`, a bool becomes Go `bool`. Neither matches `string` or `map[string]any`, and `value.Secret` is `false` for `sdk.Int`/`sdk.Bool` exports (never wrapped in `sdk.ToSecret`). Result: silent `""`.

  `search_repository_code` for `toOutputsResult` across the whole org returns exactly **one hit** — the definition itself — so there are no other call sites to worry about regressing, and no existing test pins current behavior (`preview_test.go` does not exist).

**Verdict: PM's finding stands. The fix belongs in `preview.go`, not in `byo.go`/`vm.go`.** Byo reads as the trigger only because it's currently the sole implemented `vm` driver (cloudru-vm etc. are future work) — the defect is generic CLI output serialization.

## 3. Design decision — ADR

**Decision:** extend `toOutputsResult`'s type switch with **one typed fallback branch** (`fmt.Sprintf("%v", value.Value)`) rather than enumerating `float64`, `bool`, `int`, etc. individually.

| Option | Pro | Con |
|---|---|---|
| Enumerate every JSON-decode type (`float64`, `bool`, …) | Explicit, easy to read each case | Leaves the exact same class of gap open for the next type nobody enumerated (this bug's own root cause) — rejected |
| **Single fallback `fmt.Sprintf("%v", value.Value)`** ✅ | Closes the whole class of gap once; `%v` renders `float64(22)` → `"22"`, `bool(true)` → `"true"` — matches what `strconv.FormatFloat`/`FormatBool` would produce for the shapes we actually emit | If a future export decoded to e.g. `[]any`, `%v` renders `"[1 2]"` rather than JSON `["1","2"]` — acceptable: PM's scope explicitly CUTs array/exotic shapes, and no current `vm` export is array-typed (confirmed: `vm.go` exports are `sdk.String`, `sdk.Int`, `sdk.Bool` only) |

Ordering is preserved exactly: `string` → `map[string]any` → `Secret` (unchanged — a secret int/bool still goes through the existing JSON-of-the-whole-`OutputValue` branch, matching today's behavior for secrets of any type) → **new** non-secret fallback for everything else, guarded against `nil` so an absent/unset export still renders `""` rather than the literal string `"<nil>"`.

### Exact code change for Developer

File: `pkg/clouds/pulumi/preview.go`

1. Add `"fmt"` to the import block (currently: `context`, `encoding/json`, `github.com/pkg/errors`, `github.com/samber/lo`, pulumi `auto`, `apitype`, `api`, `logger`, `color` — no `fmt` today).
2. Replace the body of `toOutputsResult`'s closure:

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
    } else if value.Value != nil {
        // Any other JSON-decoded shape of a plain (non-secret) Pulumi export —
        // numbers decode to float64, booleans to bool. host-port/host-count
        // (sdk.Int) and ingress-enabled (sdk.Bool) land here; previously fell
        // through to "" (sc-stack-outputs-empty-byo-vm-host-fields).
        res = fmt.Sprintf("%v", value.Value)
    }
    return res
}),
```

No other function in this file needs to change (`toUpdateResult`, `toPreviewResult`, `toDestroyResult`, `toRefreshResult` operate on different result shapes and are unaffected).

## 4. Test spec for Developer

New file `pkg/clouds/pulumi/preview_test.go` (none exists — confirmed via `search_repository_code`). Table-driven, constructing `auto.OutputMap{"key": auto.OutputValue{Value: ..., Secret: ...}}` directly (no live Pulumi stack needed):

| Case | `Value` | `Secret` | Expected `Outputs["key"]` |
|---|---|---|---|
| string | `"abc"` | false | `"abc"` |
| map | `map[string]any{"a":1.0}` | false | `` `{"a":1}` `` |
| secret (any underlying type) | `float64(22)` | true | non-empty JSON of the whole `OutputValue` (pins existing behavior) |
| **int-like (host-port/host-count regression case)** | `float64(2222)` | false | `"2222"` |
| **bool (ingress-enabled regression case)** | `true` | false | `"true"` |
| nil / unset | `nil` | false | `""` (must NOT become `"<nil>"`) |

This directly covers AC2's "targeted unit/integration test against the byo driver's output struct" — at the correct layer (the serializer, since the driver's struct itself is already correct per §2).

## 5. Build/test guidance (repo constraints, from `CLAUDE.md`)

- Gate with `go vet ./pkg/clouds/pulumi/...` (or `make check` for full-repo, 2.3 GB) — **not** `go build ./...`.
- Run tests scoped: `go test ./pkg/clouds/pulumi/...` — do not run the whole module.
- Lint touched files only: `bin/gofumpt -l -w ./pkg/clouds/pulumi/` + `bin/golangci-lint run --fix ./pkg/clouds/pulumi/...`.
- Every commit needs `git commit -s` (DCO) — branch protection blocks unsigned commits.
- No DB/schema/swagger impact — pure Go logic change behind an existing CLI command.

## 6. Live smoke guidance (for whoever runs it — DevOps per PM's §5)

`sc stack outputs <stack-with-byo-vm>` must show the real int for `<vmName>-vm-host-0-port` and `<vmName>-vm-host-count` keys (export names per `ToHostPortExport`/`ToHostCountExport` in `vm.go`), not `""`. Per the roadmap item's own DoD, only flip `status: shipped` with that live output pasted into `## History`; if only the unit test + code land, use `status: code-complete`.

## 7. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none) | — | Architect turn — design only, no files written |

## 8. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `sc stack outputs` reports real host-port/host-count for byo | PENDING | Root cause confirmed at `pkg/clouds/pulumi/preview.go` `toOutputsResult` (blob `7167bd6d287e3ad3ee9a8a8ef7cfc99bd806c4d9`, read this turn); fix designed in §3 above; not yet implemented — Developer's turn |
| AC2 | Smoke/unit test confirms population | PENDING | Test spec given in §4; `preview_test.go` confirmed absent via `search_repository_code` (0 hits for `preview_test path:pkg/clouds/pulumi`); not yet written |
| AC3 | Roadmap item Status/History updated with real outcome | PENDING | Current item (`docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md`, blob `5d75fbafcc5d1ded44631dc8fe7e62122f43fb00`, read this turn) still `status: in-progress`, dispatch-only History — closure is downstream's job (DevOps/QA) per PM §5, confirmed unchanged by this turn |

## 9. Tests run

None — design-only turn, no code mutated. Verification was code-reading: `read_repository_file` on `pkg/clouds/pulumi/preview.go`, `pkg/clouds/pulumi/vm/vm.go`, `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md`; `search_repository_code` for `toOutputsResult` (1 hit, confirms no other call sites) and for `preview_test` under `pkg/clouds/pulumi` (0 hits, confirms no existing test to regress). Developer must run `go vet ./pkg/clouds/pulumi/...` and `go test ./pkg/clouds/pulumi/...` per §5 after implementing §3/§4.

## 10. slice_id

`slice_id: sc-stack-outputs-empty-byo-vm-host-fields`

**Verdict:** signoff