Verification complete. Here is my handoff.

---

# QA Handoff — `sc-stack-outputs-empty-byo-vm-host-fields` (2nd dispatch attempt)

**Verdict:** failure

`failure_class`: `verification` — the test the brief's DoD requires has **never been executed**. I could not execute it either (guest OOM, root-caused below), and I will not convert an unexecuted test into a signoff. The code change itself verifies as correct; the *gate* does not close. Detail in §6.

## 0. Headline

The developer's fix is **substantively correct** — I reproduced the bug and the fix by execution, and the code is better than the Architect's design (their `float64`-only branch would have under-fixed it). The blocker is not the code. It is that **no build, vet, lint or test has ever run against this diff**, on any machine, and this guest physically cannot run one.

**Correction to the inherited diagnosis:** both the developer and `CLAUDE.md` describe this guest as 12 GB. It is **3.9 GB** with a 4 GB swapfile:

```
MemTotal:        3924196 kB   (3.7 GiB)
SwapTotal:       4194300 kB
SwapFree:         408952 kB   ← at turn start, already nearly drained
MemAvailable:      95668 kB
nproc:           2
```

So the developer's conclusion ("go test on this package is structurally unreachable") is right, but their stated reason (a ~13 GB link against 12 GB) is wrong by a factor of three. `CLAUDE.md`'s measured table (`make check` = 2.3 GB) assumes a machine with **~3× this guest's RAM** — `make check` is *not* a safe gate here either, which is the part the brief's build discipline gets wrong and which the next `api` dispatch needs to know.

## 1. Commit + branch identity

**No code mutations in this turn** — QA verification only. The diff under test:

```
$ git log -1 --format="%H %s" -- pkg/clouds/pulumi/preview.go
45195687449d83020d459053dc3674ae6fd3baf5 fix(pulumi): render non-string scalar stack outputs instead of ""
  author=David Black <david.black@simple-container.com>

$ git log -1 --format="%H %s" -- pkg/clouds/pulumi/preview_outputs_test.go
45195687449d83020d459053dc3674ae6fd3baf5 fix(pulumi): render non-string scalar stack outputs instead of ""
```

- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/76a37d22-d6b1-4cef-ab6c-a116149e80e9`
- **HEAD:** `688d2f95` (engine metadata commit on top of `45195687`)
- **DCO:** `Signed-off-by: David Black <david.black@simple-container.com>` — present, matches author ✅
- **Substance gate:** real source diff, **103 lines across 2 non-`.workflow/` files** ✅ — not a no-op PR
- Working tree clean.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| (none — QA verification turn, no file mutations) | — | — |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `-vm-host-{i}-port` renders non-empty | **PASS** | Independently re-derived harness (§4a): `float64(2222)`→`"2222"`, `json.Number("2222")`→`"2222"`, `int64(22)`→`"22"`. Pre-fix control (§4b) renders `""` — bug reproduced, then fixed |
| AC2 | `-vm-host-count` renders non-empty | **PASS** | Same: `float64(1)`→`"1"`, `int(3)`→`"3"`, `float64(1e6)`→`"1000000"` (no `1e+06` leak). Pre-fix: `""` |
| AC3 | Fix is general, not byo-specific | **PASS** | `preview.go:98` — single branch in `toOutputsResult`, the sole renderer for `OutputsStack` (`preview.go:82`), no driver gating. Verified `Vm()` exports these for *every* driver (`vm/vm.go:101,127,128`) |
| AC4 | Unit test pins the actually-broken path | **PASS (exists) / FAIL (unexecuted)** | `preview_outputs_test.go:47,62` call `p.toOutputsResult` directly, white-box in-package. 13 cases + secret/map test. **Never executed on any machine** — see §6 |
| AC5 | No regression to string/secret rendering | **PASS** | New branch sits strictly *after* `value.Secret` (`preview.go:95`→`98`), so it cannot steal secrets — confirmed by execution: numeric secret → `{"Value":42,"Secret":true}`, string → unquoted passthrough, `nil`→`""`, map→`{"a":"b"}` |
| AC6 | `make check` clean; test scoped to touched package | **FAIL — blocked, not flaky** | `go vet ./pkg/clouds/pulumi/` → silent kill at 10 s (swap 11 MB), then **exit 124** at 600 s with zero diagnostics. `go build ./pkg/clouds/pulumi/` → killed by me at 136 s with swap down to 135 MB. Toolchain proven healthy by control (§4c) |
| — | **Static-lint gate** (my standing rule) | **FAIL — tool absent** | `.golangci.yml` present + `make lint` at `Makefile:102` → gate **applies**. `bin/` unprovisioned, `golangci-lint` not on PATH. Partial substitutes in §5 |

**AC4 and AC6 are FAIL. I am not rounding them up.** The brief's definition of done is *"a unit test pins the populated fields"* — a test that has never been compiled does not pin anything yet.

## 4. Tests run

### 4a. Independent logic harness — 14/14 (I wrote my own, did not reuse the developer's)

Replicates `toOutputsResult`'s post-fix branch order byte-for-byte, covering every case in the committed test file plus the map/secret cases:

```
OK   host-port float64      got="2222"       want="2222"
OK   host-port json.Number  got="2222"       want="2222"
OK   host-count int         got="3"          want="3"
OK   big count decimal      got="1000000"    want="1000000"
OK   ingress true           got="true"       want="true"
OK   nil empty              got=""           want=""
OK   numeric secret         got="{\"Value\":42,\"Secret\":true}" want="{\"Value\":42,\"Secret\":true}"

14/14 passed, 0 failed
```

### 4b. Pre-fix control — the bug is real (the developer never proved this)

Same harness with only the new branch disabled (`else if false`):

```
FAIL host-port float64      got=""   want="2222"
FAIL host-count float64     got=""   want="1"
FAIL host-port json.Number  got=""   want="2222"
FAIL host-count int         got=""   want="3"
FAIL host-port int64        got=""   want="22"
FAIL big count decimal      got=""   want="1000000"
FAIL ingress true           got=""   want="true"
FAIL ingress false          got=""   want="false"
```

8/8 render `""` pre-fix → `""`-free post-fix. **Root cause and fix both confirmed by execution**, not by reading. The reported symptom is explained exactly.

### 4c. Toolchain control — proves the OOM is environmental, not a code defect

```
$ GOMAXPROCS=1 GOMEMLIMIT=1GiB go vet -p 1 ./pkg/api/logger/color/
exit=0
```
A light leaf package vets clean in seconds. The toolchain is healthy; only this package is unreachable.

### 4d. The gates that could not run

```
$ GOMAXPROCS=1 GOMEMLIMIT=2GiB go vet -p 1 ./pkg/clouds/pulumi/     # attempt 1
(exited 10s, empty log, swapfree=11MB at exit → silent OOM kill)

$ GOMAXPROCS=1 GOMEMLIMIT=2GiB timeout 600 go vet -p 1 ./pkg/clouds/pulumi/
=== go vet exit=124 ===      # my timeout; 0 bytes of diagnostics

$ GOMAXPROCS=1 GOMEMLIMIT=2GiB go build -p 1 ./pkg/clouds/pulumi/
t=10s  avail=254MB swapfree=1676MB
t=130s avail=209MB swapfree=483MB
probe2 avail=207MB swapfree=135MB  → killed by me to protect the guest
```

I monitored swap on a 8–10 s loop and killed at the 135 MB threshold rather than letting the kernel take the turn down; memory recovered to 2.3 GB available / 3.85 GB swap free. I did **not** retry either command — per the brief, repeated attempts are how these sessions get burned. `GOMEMLIMIT` cannot help: it caps the Go *heap*, and the cost here is the compiler's non-heap working set across the Pulumi provider SDKs.

## 5. What I verified in place of the unrunnable gates

Everything the new test asserts, checked against real sources rather than memory:

- **`auto.OutputValue`** = `struct { Value any; Secret bool }`, **no JSON tags** — read from `sdk/v3@v3.245.0/go/auto/stack.go:1413`. This is what makes the `{"Value":42,"Secret":true}` assertion (field names, that exact order) correct. ✅
- **`auto.OutputMap`** = `map[string]OutputValue` (`stack.go:1467`) — matches the composite literal. ✅
- **`api.OutputsResult.Outputs`** = `map[string]any` (`pkg/api/client.go:326`) — so `out.Outputs["out"] != tc.want` compares `any` against `string`, which compiles. ✅
- **`pulumi` struct** (`provisioner.go:26`) — all fields zero-valued-safe; `&pulumi{}` is valid and `toOutputsResult` touches no field. ✅
- **Sibling precedent**: `preview_redact_test.go` already uses `&pulumi{}` + `auto.*` literals in this exact package and compiles in CI. ✅
- **`gofmt -l` and `gofmt -e -l`** on both files → clean (formatting + no parse errors). ✅
- **`goheader`**: both files carry the required `SPDX-License-Identifier: MIT` / `Copyright (c) Simple Container`. ✅
- **`gci`**: `.golangci.yml:28-39` orders `standard | default | prefix(github.com) | prefix(github.com/aws) | prefix(github.com/pulumi) | prefix(github.com/simple-container-com)`. The test file is `encoding/json`, `testing` → blank → `github.com/pulumi/...` — correct, and identical to the sibling. ✅
- **`preview.go` imports unchanged** — `json.Marshal` reuses the existing `encoding/json`; the Architect's `strconv` would have needed a new import. Confirmed zero import lines in the diff. ✅

**I agree with the developer's divergence from the Architect.** `toInt` at `vm/compose_stack.go:682` really does accept `int/int32/int64/float32/float64/json.Number/string` for *these same* exports, with a comment stating the shape "is a float64 (or a json.Number, depending on the decoder)". A `float64`-only branch would have left `json.Number` and `int` rendering `""` — a fix that only sometimes fixes the bug. The `json.Marshal` approach is correct, narrower in diff, and needs no new import. Excluding `nil` is also right: `""` for an absent output beats the literal `"null"`.

**One residual nit (non-blocking, for the record):** a string-valued secret hits the `string` branch *before* `value.Secret` and renders in clear. That is **pre-existing** behaviour, not introduced here, and the committed test documents it explicitly. Worth its own slice given `sc-stack-outputs-leaks-secrets-in-clear` is already dispatched — I am not expanding scope into it.

## 6. Why this is `failure` and not `signoff`

The code is right. The gate is not closed. Three independent reasons I will not sign off:

1. **The brief's DoD is unmet.** It requires *"a unit test pins the populated fields"*. The test has never been compiled, let alone run — the realistic failure mode is a compile nit, which would make `go test ./...` **red on `main`** after auto-merge.
2. **`mergeStrategy=auto-merge` makes my signoff the last checkpoint.** Signing off here merges 103 lines of never-compiled Go with no further human look.
3. **My own standing lint rule fails closed.** `.golangci.yml` + `make lint` exist → the gate applies; `golangci-lint` is unavailable here. `gofumpt` is stricter than `gofmt`, so my clean `gofmt` does not discharge it.

**Mitigating context the operator should weigh:** CI on this repo *is* a real backstop for PRs, which I verified rather than assumed —

- `.github/workflows/branch.yaml:270` runs a hard-gating `go test ./...` on `pull_request`
- `branch.yaml:114` runs `bin/golangci-lint run --fix --timeout 3m -v`
- `coverage.yml:61-67` runs `go test ./...` and treats `[build failed]` as **fatal** (`exit 1`), tolerating only pre-existing runtime flakes

So **the PR's own CI will execute exactly the gate I could not**, on a full-size runner. Note `push.yaml` is `main`-only — the PR-time gate is `branch.yaml`/`coverage.yml`, not `push.yaml`.

**This makes the decision cheap for the operator:** glance at PR CI. Green `branch.yaml` → AC4/AC6 are discharged by CI and this is a clean merge. Red → the compile nit I flagged is real and bounces to the developer. Either way that is a 30-second human check, which is the right place for this call — not an autonomous merge of unexecuted code.

## 7. Gate A / Gate B — not applicable, with evidence

The isolated-stack gates do **not** apply, and I verified this structurally rather than citing a waiver I don't have. `deploy-isolated-stack.yml` in `simple-container-com/forge` accepts exactly these branch inputs:

```
aigateway_branch, baas_branch, baas_meeting_branch, concierge_branch,
concierge_meet_branch, conductor_branch, notifier_branch, runtime_branch,
sessions_branch, storage_branch
```

There is **no `api_branch` input**. `simple-container-com/api` is the `sc` CLI + provisioner library, not one of the 6 deployed `forge-*` services — there is no `<wf_id>--app.simple-forge.com` surface this diff could reach. Dispatching a stack would deploy 10 unrelated services from `main` and smoke endpoints this change cannot touch: cost and cross-test pollution for zero signal.

- `deploy_run_url`: n/a — `deploy_skip_reason`: no `api_branch` input exists; repo is not a Forge service
- `destroy_run_url`: n/a — `destroy_skip_reason`: nothing deployed
- `smoke_findings`: n/a — no live surface. The equivalent live check is `sc stack outputs` against a real `byo` vm stack, which needs cloud credentials + a provisioned host I do not hold (§8).

I did not spend a secret-fetch on `forge_api_key` / `ollama_api_key`: those authenticate the isolated Forge stack, which has no bearing on a `sc` CLI rendering function.

## 8. Operator actions

1. **Check PR CI before merging** (≈30 s, the decisive gate). Green `branch.yaml` `go test ./...` + `golangci-lint` → AC4/AC6 discharged, merge is clean. Red → bounce to developer. **Please don't force-merge past a red check here** — the whole gap is that the code is unexecuted.
2. **Optional true-live confirmation of AC1/AC2** — needs cloud creds + a provisioned `byo` host I do not hold:
   ```
   sc stack outputs <stack-with-byo-vm> | jq '."<vmName>-vm-host-0-port", ."<vmName>-vm-host-count"'
   ```
   Expect real values; pre-fix these were `""`.
3. **Fix the guest sizing claim — this will keep burning `api` slices.** The brief and `CLAUDE.md` both assume 12 GB; this guest has **3.9 GB + 4 GB swap**. `make check` is documented at 2.3 GB, which does *not* fit once swap is partly consumed. Updating the brief's discipline to say *"this package cannot be vetted or built in-guest at all; rely on PR CI"* would stop the next developer from spending their whole turn rediscovering it. This slice is now **2-for-2** on OOM.
4. **I endorse the developer's option (2) as its own slice** — put the 6 `e2e_*_test.go` files behind a build tag so this package's unit-test binary stops linking every provider SDK. It converts a recurring infra wall into a one-time repo fix and is independently useful. Option (1), a bigger swapfile, treats the symptom; given 3.9 GB of RAM, swap-thrashing a 13 GB link will be unusably slow even if it completes.

## 9. Secret hygiene

No credential values appear in this handoff. No secret aliases were fetched (none were applicable — §7). No long unexplained strings: the only high-entropy tokens are commit SHA `45195687449d83020d459053dc3674ae6fd3baf5`, HEAD `688d2f95`, run/workflow UUIDs, and the SDK version `v3.245.0`. I re-scanned the prior handoffs for leaked values and found none to redact.

## 10. Roadmap linkage

`slice_id`: `sc-stack-outputs-empty-byo-vm-host-fields` → `docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md` in the `forge` repo. Status should go to **`code-complete-pending-ci`**, not `shipped` — the fix is written and verified-by-equivalence, but its own test has not run. Do not mark shipped until PR CI is green.

**Empty-PR-OK:** no — this run carries a real 103-line source fix across 2 files; the engine's empty-diff escalation does not apply.

**Verdict:** failure