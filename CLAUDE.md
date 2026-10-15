# Simple Container (`api`) — notes for AI coding agents

This is the Simple Container core module: the `sc` CLI, the cloud provisioners under
`pkg/clouds/` (AWS, GCP, Yandex Cloud, Kubernetes, Cloudflare, Pulumi), and the
schema/codegen tooling under `cmd/`. MIT licensed; see `README.md` for what it does and
`docs/` for the user-facing docs.

Read this before your first build. The most common way an agent loses a session in this
repo is not a compile error — it is a silent out-of-memory kill, and it is easy to
misdiagnose as a hang or a toolchain bug.

## Compiling this module is memory-hungry. Cap it.

`pkg/clouds/pulumi/` links the Pulumi provider SDKs, which are among the heaviest Go
packages in existence to compile. Two things multiply:

- `go build ./...` / `go test ./...` compile **GOMAXPROCS packages concurrently**, and
- each `go tool compile` **parallelises its own backend** (`-c`, also derived from
  GOMAXPROCS), holding several functions' SSA state live at once — so per-process peak
  RSS rises too.

Single `compile` processes in this module have been measured **above 6 GB**. One fits in a
12 GB machine; two do not. On a laptop, a container, or a CI sandbox with ~12 GB and no
swap, the default invocation reliably dies.

**Use the capped entrypoints in the `Makefile`:**

```sh
make check        # go vet ./...  — START HERE; the only full-repo path that fits 12 GB
make test         # serialised, with GOMEMLIMIT
make build        # link the sc binary into dist/ — needs ~13 GB, see below
make build-all    # the full ./... compile+link — ~14 GB, ~76 min cold
```

Measured on this module with the caps applied — know these before you pick a command:

| command | peak RSS | wall | notes |
|---|---|---|---|
| `make check` | **2.3 GB** | ~3 min | compiles warm, vet analysis fresh |
| `make build` | **12.9 GB** | ~2 min | compiles warm — this is the *link* |
| `make build-all` | **14.2 GB** | ~76 min | from a cold cache |

**The caps bound the compile phase, not the link.** Linking `./cmd/sc` needs ~13 GB by
itself, and neither `GOMAXPROCS` nor `GOMEMLIMIT` reduces it — the linker is largely serial
already and its footprint is not Go heap. So on a machine under ~16 GB, **do not link at
all**: use `make check` as your gate and let CI produce the binaries. `make build` and
`make build-all` are for a large host.

They set `GOMAXPROCS=1`, `GOMEMLIMIT=6GiB` and `-p 1` by default, and every value is
overridable on a bigger machine (`make test GO_MAXPROCS=8 GO_TEST_PARALLEL=4` — note the
underscore: the make variables are `GO_*` so that a `GOMAXPROCS` inherited from your shell
cannot silently defeat the cap). Run `make help` for the list.

If you invoke `go` directly, carry the caps yourself:

```sh
GOMAXPROCS=1 GOMEMLIMIT=6GiB go build -p 1 ./...
```

**Do not run more than one build at a time.** Launching `go test ./...` while an earlier
`go build ./...` is still alive is a reliable way to exhaust the machine even with the
caps applied — the caps bound each `go` invocation, not the number of invocations. Wait
for one to finish.

**Avoid `-race` unless you know the machine has the RAM.** The race detector's shadow
memory is roughly a 5-10x multiplier and is the single largest amplifier here. `make test`
deliberately omits it; `make test-race` is the opt-in target for a large host.

### Recognising the failure

A memory kill produces **no compiler diagnostic**. What you see instead:

- `signal: killed`, or exit status `137` (128+SIGKILL — the kernel OOM killer), with
  empty or truncated stderr. If a supervisor notices first you may instead see `143`
  (128+SIGTERM), which looks like a timeout but often is not;
- a build that appears to hang and then returns nothing useful;
- in a sandbox, the whole turn or process group disappearing mid-build.

If that happens, **do not retry the same command** — it will fail the same way, and
repeated attempts are how agents burn an entire session here. Switch to `make check`. If
it was `make build` or `make build-all` that died, do not reach for a bigger cap: the link
is the floor (see the table above), so on this machine the answer is to stop linking and
let CI do it, not to retry with different flags.

## Prefer `go vet` over `go build` as your gate

`go build ./...` does **not** compile test packages, so it silently accepts changes that
break `_test.go` files — adding a method to an interface, for example, leaves every fake
in the test tree unimplementing it and the build still passes. `go vet ./...`
(`make check`) type-checks tests too, and it is dramatically cheaper — 2.3 GB against
12.9 GB for a link — because it never produces object code or a binary. Make it your
default full-repo check.

## Build & test tooling

`welder` is this repo's primary task runner and what CI uses — `welder.yaml` defines
`build`, `build-all`, `test`, `tools`, `fmt`, `linters`, `coverage` and the release tasks.
The `Makefile` does not replace it; it is a thin, memory-capped path for constrained
machines, and it shells out to plain `go` so it works without welder installed.

Note that **CI never builds `./...` for the real artifacts** — `.github/workflows/push.yaml`
builds per command (`go build -o … ./cmd/sc`, then `./cmd/<cmd>` per matrix leg) on
full-size runners. Copying the per-command shape is a good instinct; copying a bare
`go build ./...` from anywhere is not.

`bin/golangci-lint` and `bin/gofumpt` are provisioned by `welder run tools` (which reads
`tools.go`). Note `golangci-lint` is itself memory-hungry on this module — if `make lint`
is killed, lint the packages you touched rather than the whole repo:
`bin/golangci-lint run ./pkg/clouds/pulumi/...`.

## Before you push

`go build` does not run the linters, and they are a required gate. The config is
`.golangci.yml`; the two that most often fail a PR authored by an agent are import-group
order (`gci`) and formatting (`gofumpt`, stricter than `gofmt` — single const groups, tag
alignment, no redundant blank lines). Both are auto-fixable, and the repo already has a
task for it — `welder run fmt`, which is:

```sh
go mod tidy
bin/gofumpt -l -w ./
bin/golangci-lint run --fix --timeout 3m
```

`golangci-lint --fix` applies the `gci` import ordering from `.golangci.yml` for you, so
do not hand-roll the section flags — and note `gci` is **not** one of the binaries
`welder run tools` provisions (`bin/` has `gofumpt` and `golangci-lint`), so there is no
`bin/gci` to call.

On a constrained machine `golangci-lint` over the whole module may itself be OOM-killed.
Scope it to what you touched instead — it still fixes those files correctly:

```sh
bin/gofumpt -l -w ./pkg/clouds/pulumi/
bin/golangci-lint run --fix ./pkg/clouds/pulumi/...
```

Pass **every** file you touched, including `_test.go` — the CI linter walks the whole
module, so one skipped test file costs a full round-trip. Then `make check`.

**Review `git status` before committing.** When a change alters a function signature
across files, `git add <named-files>` silently omits anything you forgot to name: the
local build passes because the file is on disk, but the commit ships a caller with the
wrong arity. Prefer `git add -A <dir>` after reading the diff.

## Commits need a DCO sign-off

This repo enforces the Developer Certificate of Origin (`.github/workflows/dco.yml`).
Every commit must carry a `Signed-off-by:` trailer matching the commit author
(`git commit -s`), or the PR is blocked. Branch protection also requires human review —
nothing self-merges here.
