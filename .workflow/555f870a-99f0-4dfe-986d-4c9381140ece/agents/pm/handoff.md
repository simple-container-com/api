[thinking] # PM Handoff — slice `sc-fs-state-dir-not-gitignored`

## 0. Context digest (read before scoping)

Codebase reconnaissance complete. Key facts that shape the plan:

- **Init hook location**: `pkg/clouds/pulumi/fs_state_store.go` → `initFileSystemStateStore(ctx, stateStoreCfg api.StateStorageConfig, log logger.Logger) error`. Signature matches `pApi.InitStateStoreFunc`. It currently only does `url.Parse` + `os.MkdirAll(dir, 0o700)`. Registered in `pkg/clouds/pulumi/init.go` via `pApi.RegisterInitStateStore(fs.StateStorageTypeFileSystem, initFileSystemStateStore)`.
- **Dedup-safe ignore API already exists**: `pkg/api/git/ignore.go` → `(r *repo) AddFileToIgnore(filePath string) error`. It reads current `.gitignore` content, checks `strings.Contains(content, "\n"+filePath)` before appending — this IS the dedup behavior AC3 says not to reinvent. It creates `.gitignore` if absent (`readIgnore`).
- **Two existing call sites to mirror** (per brief):
  - `pkg/provisioner/common.go:248` (`InitProfile`) — calls `p.gitRepo.AddFileToIgnore(profileCfgFile)` where `profileCfgFile` is already a path **relative to repo root** (`api.ConfigFilePath("", p.profile)`).
  - `pkg/api/secrets/management.go:82` (`AddFile`) — same pattern, `c.gitRepo.AddFileToIgnore(filePath)`.
  - A third precedent exists outside the brief's two refs: `pkg/assistant/analysis/cache.go` (`ensureCacheInGitignore`) constructs its OWN `git.Repo` via `git.New(git.WithDetectRootDir())` + `repo.InitOrOpen(projectPath)` purely to call `AddFileToIgnore` — this is the pattern to copy when the init hook (unlike `common.go`/`management.go`) has **no existing `git.Repo` handle available** in its call chain (`fs_state_store.go` is a standalone function, not a method on a type carrying a `gitRepo` field — confirmed by reading `pulumi/provisioner.go`'s `pulumi` struct, which holds no git.Repo).
- **Root-dir detection already exists**: `git.WithDetectRootDir()` → `(r *repo) detectRootDir()` in `pkg/api/git/repo.go`, walks up via `git.PlainOpenWithOptions("", {DetectDotGit: true})`, sets `r.workDir = wt.Filesystem.Root()`. This is the mechanism to decide "inside repo root" vs "outside" (AC2) — compare the state dir's absolute path against the detected root via `filepath.Rel` (a `Rel` starting with `..` or failing ⇒ outside ⇒ skip).
- **Path resolution nuance**: `fs.FileSystemStateStorage.StorageUrl()` (in `pkg/clouds/fs/fs_state.go`) already makes relative paths absolute and prepends `file://`. `initFileSystemStateStore` re-parses that URL back into `dir := u.Path`. Any "is this inside the repo root" check must operate on this already-absolutized `dir`, not on the raw config value (which may be relative, `${git:root}/...`, or an explicit `file://` URL).
- **Stale doc to clean up (AC4)**: `docs/docs/examples/vm-compose/byo-host/server.yaml`, added in api#455 commit `a845fd71`, has a `# Add .sc/state to .gitignore yourself...` comment block directly above the `provisioner.config.state-storage` block — must be removed/rewritten once the real fix lands, per CLAUDE.md's doc-currency expectations.
- **Pulumi backend siblings (AC5, non-blocking)**: the DIY backend writes `.pulumi/history/<stack>/` and `.pulumi/backups/<stack>/` under the same `path:` root (confirmed by `diy.Login` backend usage in `login.go`, standard Pulumi DIY-backend layout) — these are children of the directory passed to `AddFileToIgnore`, so ignoring the directory entry itself (not individual files inside it) covers them. Worth a one-line note in the fix, not a separate code path.
- **12GB microVM constraint acknowledged** — Architect/Developer must use `make check` only; never `./...` build/test.

## 1. Restated requirement

`sc provision` with `state.type: fs` creates the state directory (existing hook) but never gitignores it. The checkpoint under that path contains the Pulumi stack's full resolved env (secrets rendered in clear) and — for `byo` VM host stacks — the SSH deploy private key. A plain `git add -A` / `git status` + commit silently stages and can leak this. Fix: when the configured path resolves inside the repo root, add it to `.gitignore` from the same init hook that already creates the directory — using the existing `git.Repo.AddFileToIgnore`, not a new mechanism.

## 2. Acceptance criteria (restated, numbered for Architect/Developer/QA to track)

| AC# | Criterion | Notes for implementer |
|---|---|---|
| AC1 | `type: fs` provision whose `path` resolves inside repo root → that path appears in `.gitignore`, written by SC (no doc instruction needed) | Call `AddFileToIgnore` from `initFileSystemStateStore` (or a helper it calls) after the `MkdirAll` succeeds. Use a `git.Repo` obtained via `git.New(git.WithDetectRootDir())` + `InitOrOpen`, mirroring `cache.go`'s `ensureCacheInGitignore` — since this hook has no existing repo handle threaded in. Write the ignore entry **relative to repo root** (same convention as `common.go`/`management.go`), not an absolute path — an absolute-path `.gitignore` entry only matches at repo root by accident and breaks if `.gitignore` entry conventions assume relative. |
| AC2 | A path outside repo root is NOT added to `.gitignore` | Resolve repo root via `detectRootDir`'s mechanism; compute `filepath.Rel(root, dir)`; if it errors or starts with `..` (or root detection itself fails, e.g. no git repo at all — unlikely in `sc provision` context but handle gracefully), skip the ignore step entirely. Do not treat this as a hard error — log at debug and continue, consistent with the existing hook's forgiving style (cf. the non-file-scheme branch that `log.Warn`s and returns nil). |
| AC3 | Re-provision does not duplicate the entry | Rely on `AddFileToIgnore`'s existing `strings.Contains(content, "\n"+filePath)` dedup (`pkg/api/git/ignore.go`) — do not add a second dedup layer. Test should provision twice and assert the `.gitignore` file has exactly one matching line. |
| AC4 | Remove the hand-written workaround comment in `docs/docs/examples/vm-compose/byo-host/server.yaml` (api#455, `a845fd71`) | Delete the "Add `.sc/state` to .gitignore yourself. Nothing does it for you..." paragraph (and the sentence fragment that references it) once AC1 ships — it is stale advice after this fix. Keep the rest of the comment block (explains `path`, mode 0700) intact. |
| AC5 | (non-blocking, "worth a look") Other DIY-backend-on-disk paths / Pulumi `.pulumi/history`,`.pulumi/backups` siblings | Confirmed: both are children of the same configured `path:` directory for the DIY backend — ignoring the directory path itself (not a file) covers them, since `.gitignore` directory entries are recursive by default. No extra entries needed. Document this conclusion in a code comment near the new logic so it isn't re-litigated. If Architect finds another DIY-backend-on-disk config elsewhere in the codebase with its own init hook, flag it in the PR description for a follow-up roadmap item — do not silently expand scope into fixing it now. |

## 3. Scope decisions

| Item | MUST / NICE-TO-HAVE / CUT | Rationale |
|---|---|---|
| Add `.gitignore` entry for in-repo `fs` state path from `initFileSystemStateStore` | **MUST** | Core of the slice — leak-prevention on the happy path |
| Reuse `git.Repo.AddFileToIgnore` exactly as called at `common.go`/`management.go` | **MUST** | Brief explicitly forbids inventing a new ignore mechanism |
| Out-of-repo path detection + skip | **MUST** | AC2 — a path outside repo cannot be committed; adding it to `.gitignore` is noise/wrong |
| Re-provision idempotency (rely on existing dedup) | **MUST** | AC3 — regression risk is a growing `.gitignore` on every `sc provision` |
| Remove stale doc workaround in `byo-host/server.yaml` | **MUST** | AC4, explicit in brief; leaving it is "stale advice" post-fix |
| Unit test covering in-repo / out-of-repo / idempotent re-provision | **MUST** | Brief's "Tests" section is explicit |
| Note on `.pulumi/history`/`.pulumi/backups` coverage | **NICE-TO-HAVE** (documented conclusion only, no code) | AC5 says "worth a quick look, not blocking" — conclusion already reached above; Architect should just leave a one-line comment, not design new mechanism |
| Auditing/fixing other DIY-backend-on-disk gaps elsewhere in repo | **CUT** (record_task if found) | Out of this slice's blast radius; brief scopes to `fs_state_store.go` only |
| Changing `FileSystemStateStorage.StorageUrl()` behavior | **CUT** | Already correct (absolutizes + schemes); not touched by this fix |
| Any change to `pApi.InitStateStoreFunc` signature or registration mechanism | **CUT** | No new capability needed — `log logger.Logger` is already passed, repo root can be detected independently inside the hook |

## 4. Out of scope (explicit)

- Any full-module build/test invocation (`go build ./...`, `go test ./...`, `make build`, `make build-all`) — forbidden by the 12GB microVM constraint. Gate is `make check` + `go test ./pkg/clouds/pulumi/... ./pkg/api/git/...` (narrow to touched packages).
- Modifying `AddFileToIgnore`'s dedup logic itself.
- Any other roadmap item's scope (this run is this slice only).

## 5. Handoff guidance for Architect

1. **Design the git-repo acquisition inside `initFileSystemStateStore`.** It has no existing `git.Repo` handle in scope. Two viable approaches — pick one and justify in the design note:
   - (a) Mirror `pkg/assistant/analysis/cache.go`'s `ensureCacheInGitignore`: construct a fresh `git.New(git.WithDetectRootDir())` + `InitOrOpen(root)` inline. Self-contained, no signature changes, proven pattern already in this codebase.
   - (b) Thread a `git.Repo` into the hook via a new field on the `InitStateStoreFunc` call site in `login.go` — rejected: brief explicitly scopes the fix to `fs_state_store.go`/`init.go`, and changing the shared `InitStateStoreFunc` signature touches every other registered hook (AWS, GCP, Yandex, Pulumi Cloud) for no benefit.
   - → **Recommend (a).**
2. **Root/outside-repo check**: after `detectRootDir`-equivalent gives you the repo root, `filepath.Rel(root, dir)` (where `dir` is the hook's existing already-absolutized `dir` variable) — reject/skip on error or a leading `..` segment. Decide also: what if `detectRootDir` itself fails (no enclosing git repo at all)? Recommend: log at debug/warn and skip ignoring (fail open on the ignore step, not on provisioning — never block `sc provision` over a `.gitignore` nicety).
3. **Path written to `.gitignore` must be relative to repo root**, matching the convention in `common.go`/`management.go` (they pass repo-root-relative paths, e.g. `.sc/cfg.yaml`). Compute it via the same `filepath.Rel(root, dir)` result from step 2.
4. **Write the unit test** in `pkg/clouds/pulumi/fs_state_store_test.go` (existing file, has the right shape — see `TestInitFileSystemStateStore` subtests for the pattern: `t.TempDir()`, table of `t.Run` subtests). New subtests needed:
   - in-repo path → `.gitignore` gains the entry (set up a real git repo in `t.TempDir()` first, e.g. via `git.New().Init(dir)` or shelling minimal git init — check what the existing repo tests use, likely `newRepoIn`-style helper from `pkg/api/git/repo_lifecycle_test.go`, or just `exec.Command("git","init")` if that test file doesn't export helpers across packages).
   - out-of-repo path (e.g. a path outside `t.TempDir()`'s git repo, or no repo at all) → `.gitignore` unchanged / absent.
   - call `initFileSystemStateStore` twice → `.gitignore` has exactly one line for the entry (dedup).
5. **Doc edit**: `docs/docs/examples/vm-compose/byo-host/server.yaml` — remove the "Add `.sc/state` to .gitignore yourself..." sentence from the top comment block, keep the rest.
6. **Gate**: `make check` (repo root) + `go test ./pkg/clouds/pulumi/... ./pkg/api/git/...` only. Do not run `make build`/`make build-all`/bare `./...`.
7. **DCO**: commits need `Signed-off-by:` trailer (`git commit -s`) per CLAUDE.md — Developer must not forget this or the PR is blocked by CI.
8. **Lint scope**: `bin/gofumpt -l -w ./pkg/clouds/pulumi/ ./pkg/api/git/` + `bin/golangci-lint run --fix ./pkg/clouds/pulumi/... ./pkg/api/git/...` (not whole-module, per CLAUDE.md memory constraint).

## 6. Files touched (this turn)

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | PM turn — planning only, no file mutations |

## 7. Tests run

N/A — PM turn, no code changed. Test obligations for Developer are specified in §5.4 above.

**Empty-PR-OK:** yes — this is the PM planning turn; no source files are expected to change, only this handoff document is produced for Architect/Developer to act on.

**Verdict:** signoff