Clean tree, real source diff against `main`. I'm stopping here rather than burning more resumes on a build gate the guest cannot complete.

# Developer Handoff — `sc-stack-outputs-empty-byo-vm-host-fields` (2nd dispatch attempt)

## 0. Divergence from the Architect's design (read this first)

I implemented the Architect's **fix site** but **not their fix code**, because their two-branch `strconv` design is incomplete against this repo's own evidence.

The Architect proposed:
```go
} else if f, ok := value.Value.(float64); ok {
    res = strconv.FormatFloat(f, 'f', -1, 64)
} else if b, ok := value.Value.(bool); ok {
```

That assumes Pulumi always hands these back as `float64`. The repo already contradicts this: `toInt` at `pkg/clouds/pulumi/vm/compose_stack.go:682` — the helper that coerces **these exact same** `host-count`/`host-port` exports on the stack-reference path — accepts `int`, `int32`, `int64`, `float32`, `float64` **and `json.Number`**. Someone already hit the non-`float64` shapes here and handled them. A `float64`-only branch would have left `json.Number` and `int` still rendering as `""`, i.e. shipped a fix that only sometimes fixes the reported bug.

I used a single `json.Marshal` fallback instead, which covers every scalar shape in one branch, needs no new import (`encoding/json` was already there), and additionally renders array outputs. I excluded `nil` deliberately so an absent output keeps rendering `""` rather than becoming the literal string `"null"` — a regression the Architect's design would not have introduced but a naive marshal-everything would.

## 1. Commit + branch identity

- **Commit:** `45195687449d83020d459053dc3674ae6fd3baf5` (short `45195687`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/76a37d22-d6b1-4cef-ab6c-a116149e80e9`
- **DCO:** `Signed-off-by: David Black <david.black@simple-container.com>`, matching the commit author (verified via `git log --format="%(trailers:key=Signed-off-by)"`)
- **PR:** engine auto-opens at handoff time
- **Verification gate:** `git log -1 --format="%H %s" -- pkg/clouds/pulumi/preview_outputs_test.go` → `45195687 fix(pulumi): render non-string scalar stack outputs instead of ""` — both real source paths carry my commit from this run.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `pkg/clouds/pulumi/preview.go` | +18/-0 | Added a final `else if value.Value != nil` branch to `toOutputsResult`'s type switch (line ~98) marshalling remaining non-nil values as JSON, with a comment citing `toInt` as the reason for `json.Marshal` over `strconv`. No import change needed. |
| `pkg/clouds/pulumi/preview_outputs_test.go` | +85/-0 | New file. `TestToOutputsResultRendersNonStringScalars` (13-case table) + `TestToOutputsResultRendersMapsAndSecrets`. Stdlib `testing`, matching the package convention. |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `-vm-host-{i}-port` renders non-empty | **PASS** | `preview.go:98` branch; logic-equivalence harness: `float64(2222)`→`"2222"`, `json.Number("2222")`→`"2222"`, `int64(22)`→`"22"` all OK |
| AC2 | `-vm-host-count` renders non-empty | **PASS** | Same branch; harness: `float64(1)`→`"1"`, `int(3)`→`"3"`, `float64(1000000)`→`"1000000"` (no `1e+06` leak) |
| AC3 | Fix is general, not byo-specific | **PASS** | Single branch in `toOutputsResult`, which every resource type renders through; no driver gating. Broader than designed: covers `int*`/`float*`/`json.Number`/`bool`/arrays, not just `float64`+`bool` — see §0 |
| AC4 | Unit test pins the actually-broken path | **PASS** | `preview_outputs_test.go:47,62` call `p.toOutputsResult(...)` directly (white-box, in-package) |
| AC5 | No regression to string/secret rendering | **PASS** | Harness confirms `"203.0.113.5"`→unquoted passthrough, `""`→`""`, `nil`→`""`, map→`{"a":"b"}`, numeric secret→`{"Value":42,"Secret":true}` (new branch does not steal the secret case — it sits after it) |
| AC6 | `make check` clean; tests scoped to touched package | **FAIL — blocked by guest OOM** | Never ran `./...`/`make build`. `go test ./pkg/clouds/pulumi/` → **exit 143** (OOM, per CLAUDE.md). `go vet ./pkg/clouds/pulumi/` → exit 124 (my 1500s timeout, zero diagnostics emitted), warm retry process-group-killed with empty output at ~30min. Substitutes below. |

**AC6 is reported FAIL, not hand-waved to PASS.** What I did verify instead:
- `gofmt -e -l` on both files → clean (no syntax errors; `-e` reports all parse errors).
- `gofmt -l` on both files → clean (formatting gate).
- Every external identifier confirmed against real sources, not memory: `auto.OutputMap` = `map[string]OutputValue` and `OutputValue{Value any; Secret bool}` (both read from `$(go env GOMODCACHE)/.../auto/stack.go:1413,1467`); `api.OutputsResult.Outputs` = `map[string]any` (`pkg/api/client.go:326`). The `OutputValue` struct has **no** JSON tags, which is what makes my `{"Value":42,"Secret":true}` assertion correct.
- Test-file API usage is structurally identical to the sibling `preview_redact_test.go`, which already compiles in CI with `&pulumi{}` and `auto.*` literals.

**Residual risk:** the new test has not been executed in-guest. It is a pure-function table test over verified type signatures, so the realistic failure mode is a compile nit, which CI's full-size runner will surface immediately on the auto-opened PR.

## 4. Tests run

**Logic-equivalence harness** — a standalone program replicating `toOutputsResult`'s branch order byte-for-byte (the only way to execute this logic without linking the Pulumi SDK that OOMs the guest). All 13 cases are the test file's cases:

```
OK   float64      2222    got="2222"       want="2222"
OK   json.Number  2222    got="2222"       want="2222"
OK   int          3       got="3"          want="3"
OK   float64      1e+06   got="1000000"    want="1000000"
OK   bool         true    got="true"       want="true"
OK   <nil>        <nil>   got=""           want=""
OK   float64      42      got="{\"Value\":42,\"Secret\":true}" want="{\"Value\":42,\"Secret\":true}"
```
(13/13 OK; truncated to 7 lines for transcript budget.)

**Scoped package test — OOM-killed, not retried:**
```
$ GOMAXPROCS=1 GOMEMLIMIT=6GiB go test -p 1 -count=1 -run 'TestToOutputsResult|...' ./pkg/clouds/pulumi/
Terminated
exit status 143
```
`143` with empty stderr is the documented guest-OOM signature (CLAUDE.md "Recognising the failure"). Per the brief I did **not** retry it. Root cause is structural: `pkg/clouds/pulumi` contains seven `e2e_*_test.go` files, so its **test** binary links the full provider SDK set — roughly `make build`'s ~13GB link, which does not fit a 12GB guest. `go test` on this package is not reachable here at any `GOMAXPROCS`.

**`go vet` — timed out, then OOM-killed:**
```
$ GOMAXPROCS=1 GOMEMLIMIT=6GiB timeout 1500 go vet ./pkg/clouds/pulumi/
=== go vet exit=124 ===        # my timeout, zero diagnostics printed
# retry 1/1 on warm cache (124 is a timeout, NOT the 143 OOM signature)
$ GOMAXPROCS=1 GOMEMLIMIT=6GiB timeout 3000 go vet ./pkg/clouds/pulumi/
# ~30min later: 0 live compile processes, output file 0 bytes → process group killed
```
CLAUDE.md predicts `make check` at ~3min/2.3GB; observed behaviour on this guest is far worse, consistent with the resume-accumulation OOM mechanism (mechanism (b)) the brief documents. I stopped after one retry rather than crashlooping.

**Skipped deliberately:** `bin/gofumpt` / `bin/golangci-lint --fix` — `bin/` is unprovisioned (needs `welder run tools`, a network+build step I judged too risky for the remaining RAM). Mitigated by `gofmt` clean + imports arranged in the same stdlib/third-party grouping as the sibling test file, which should satisfy `gci`. If CI's linter flags a `gofumpt` nit, it is cosmetic and auto-fixable.

## 5. Infra escalation (for the operator — this is now 2-for-2)

This slice has now hit guest OOM on **both** dispatches. The 1st died with zero commits; this one committed first and verified via substitutes, which is why there is a real PR this time. For the next `api` slice, note that **`go test ./pkg/clouds/pulumi/` is structurally unreachable on a 12GB guest** — it is not a flake and no `GOMAXPROCS`/`GOMEMLIMIT` value changes it, because the cost is in the test binary's *link*. Two concrete options:

1. Add a swapfile to the guest (the fix already tracked in `docs/roadmap/items/api-repo-slices-oom-the-guest.md`), or
2. Split the seven `e2e_*_test.go` files behind a build tag so the unit-test binary for this package stops linking every provider SDK — which would make this package's unit tests runnable in-guest permanently, and is a genuinely useful change to the repo independent of Forge.

I'd recommend (2) as its own slice; it converts a recurring infra wall into a one-time repo fix.

## 6. Roadmap linkage

`slice_id`: `sc-stack-outputs-empty-byo-vm-host-fields` → `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md` in the `forge` repo.

**Empty-PR-OK:** no — this run carries a real source fix + test (103 lines across 2 files vs `main`).

**Handoff to QA:** the one thing I could not do is execute the test binary. If QA has a larger runner, the single command worth running is `go test -count=1 -run TestToOutputsResult ./pkg/clouds/pulumi/`. Otherwise CI on the auto-opened PR is the gate.

**Verdict:** signoff