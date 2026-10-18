# Design: gitignore the `fs` state storage directory on provision

slice_id: `sc-fs-state-dir-not-gitignored`
status: proposed — ready for Developer

## 1. Problem (restated)

`state.type: fs` (`pkg/clouds/fs.FileSystemStateStorage`) stores the Pulumi DIY-backend
checkpoint — the stack's full resolved env (every `${secret:...}` rendered in clear) and,
for `byo` VM-host stacks, the SSH deploy private key — under a plain `path:` directory.
The init hook that creates that directory (`initFileSystemStateStore`,
`pkg/clouds/pulumi/fs_state_store.go`, added 2026-10-18, current content read this turn)
does `url.Parse` + `os.MkdirAll(dir, 0o700)` and nothing else. Nothing gitignores `dir`,
so when that path resolves inside the project's git checkout, `git status` / `git add -A`
offers the checkpoint for staging. The only mitigation today is a hand-written comment in
`docs/docs/examples/vm-compose/byo-host/server.yaml` telling the operator to gitignore it
themselves.

## 2. Existing primitives (read this turn, not assumed)

- `pkg/api/git/ignore.go` → `(r *repo) AddFileToIgnore(filePath string) error`. Reads
  `.gitignore` (creating it if absent via `readIgnore`), and only appends when
  `!strings.Contains(currentContent, "\n"+filePath)` — this IS AC3's dedup, verified by
  reading the function body; do not re-implement it.
- Two in-repo call sites already use it with a repo-root-relative path:
  `pkg/provisioner/common.go:248` (`p.gitRepo.AddFileToIgnore(profileCfgFile)`, where
  `profileCfgFile = api.ConfigFilePath("", p.profile)`, already relative) and
  `pkg/api/secrets/management.go:82` (`c.gitRepo.AddFileToIgnore(filePath)`).
- A third precedent, `pkg/assistant/analysis/cache.go`'s `ensureCacheInGitignore`, builds
  its own handle with no caller-supplied `git.Repo`:
  ```go
  repo, err := git.New(git.WithDetectRootDir())
  ...
  if err := repo.InitOrOpen(projectPath); err != nil { ... }
  if err := repo.AddFileToIgnore(cacheEntry); err != nil { ... }
  ```
- `git.WithDetectRootDir()` (`pkg/api/git/opts.go` → `(r *repo) detectRootDir()`,
  `pkg/api/git/repo.go`) opens the enclosing repo via
  `git.PlainOpenWithOptions("", &git.PlainOpenOptions{DetectDotGit: true, ...})` (walking
  up from the process cwd) and, on success, sets `r.gitRepo`, `r.gitFs`,
  `r.workDir = wt.Filesystem.Root()`, `r.wdFs = osfs.New(r.workDir)`. **This alone is
  enough state for `AddFileToIgnore`** — it only touches `r.wdFs` — no `Init`/`Open`/
  `InitOrOpen` call is required afterward.
- `fs.FileSystemStateStorage.StorageUrl()` (`pkg/clouds/fs/fs_state.go`, read this turn)
  already makes a relative `path:` absolute via `filepath.Abs` and prepends `file://`
  unless a scheme is already present. `initFileSystemStateStore` re-parses that URL back
  into `dir := u.Path` (or `u.Opaque` as fallback) — so `dir` is already an absolute,
  cleaned path by the time our new code runs. No extra `filepath.Abs` call is needed.

## 3. Decision: do NOT mirror `cache.go`'s `InitOrOpen` step

`cache.go` calls `repo.InitOrOpen(projectPath)` after `WithDetectRootDir()`. Tracing
`InitOrOpen` → `Init` → `initRepo` (`pkg/api/git/repo.go`) shows `initRepo` rebuilds a
**second** `*repo` from `r.opts` (which still carries `WithDetectRootDir`) and then
overwrites its `workDir` with the explicit argument — and if no git repository exists at
that path yet, `Init` calls `git.Init(...)`, i.e. **it runs `git init` as a side effect**.

That side effect is wrong for this call site: `initFileSystemStateStore` runs on every
`sc provision`, including against a `path:` that is legitimately outside any git repo
(AC2) or where there is no enclosing repo at all. Silently git-initing a directory because
a state-storage path happened to point near it is a surprise no operator asked for, and it
contradicts AC2's "outside repo root ⇒ do nothing" requirement at a more basic level (it
would create a *new* repo rooted at the state dir, then correctly skip adding an ignore
entry to *that* new repo — net effect: a stray unwanted `.git` in the deploy's wake).

→ **Use `git.New(git.WithDetectRootDir())` alone.** If detection fails (no enclosing git
repository), treat it exactly like the init hook's existing non-`file://`-scheme branch:
`log.Warn` and return `nil` — never fail `sc provision` over a `.gitignore` nicety.

## 4. Decision: path-containment check, including the root-equality edge case

```go
root := gitRepo.Workdir()                  // absolute, from detectRootDir()
rel, err := filepath.Rel(root, dir)        // dir is already absolute (see §2)
outside := err != nil ||
    rel == "." ||                           // dir IS the repo root — never ignore "."
    rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
```

`rel == "."` is the case the brief's AC2 wording doesn't name explicitly but the two
existing examples (`docs/docs/examples/vm-compose/byo-host/server.yaml`'s
`${git:root}/.sc/state`) make plausible as a misconfiguration (`path: ${git:root}`,
pointing the state dir AT the root) — ignoring `.` would gitignore the entire repository.
Treat it as "outside" (skip) for safety; Developer's test matrix must cover it explicitly
(§6).

## 5. Decision: the `.gitignore` entry itself

- Value written: `rel` with a trailing `/` appended (`rel + "/"`) — a directory-only
  gitignore pattern, matching AC5's already-reached conclusion that ignoring the directory
  covers Pulumi's `.pulumi/history/`/`.pulumi/backups/` children for free (they're written
  *under* this same `path:` directory by the DIY backend). The two existing call sites
  ignore individual *files*, not directories — this is the one place in the fix that
  doesn't mirror them 1:1, and it's why: those are files, this is a directory tree.
- `filepath.Rel` returns OS-native separators; this module and its CI are Linux-only
  (per CLAUDE.md), so no `/`-normalization pass is needed, but note it in a code comment
  so a future Windows port doesn't quietly ship a broken pattern.

## 6. Proposed code (fs_state_store.go)

```go
import (
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/api/git"
)

// ensureStateDirIgnored adds dir to .gitignore when it resolves inside the enclosing git
// repository. It never fails sc provision: detection or containment misses are logged and
// skipped, not returned as an error, because a missing .gitignore entry is a hygiene gap,
// not a reason to abort a deploy.
func ensureStateDirIgnored(ctx context.Context, dir string, log logger.Logger) {
	repo, err := git.New(git.WithDetectRootDir())
	if err != nil {
		log.Debug(ctx, "state storage directory %q is not inside a git repository (or none could be detected), skipping .gitignore update: %v", dir, err)
		return
	}
	root := repo.Workdir()
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		log.Debug(ctx, "state storage directory %q is outside the repository root %q, not adding to .gitignore", dir, root)
		return
	}
	entry := rel + string(filepath.Separator)
	if err := repo.AddFileToIgnore(entry); err != nil {
		log.Warn(ctx, "failed to add %q to .gitignore: %v", entry, err)
	}
}
```

Call site — append to `initFileSystemStateStore`, after the existing `MkdirAll` succeeds,
before the trailing `log.Debug`/`return nil`:

```go
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return errors.Wrapf(err, "failed to create state storage directory %q", dir)
	}
	ensureStateDirIgnored(ctx, dir, log)
	log.Debug(ctx, "state storage directory %q is ready", dir)
	return nil
```

No change to `pApi.InitStateStoreFunc`'s signature, no new registration — confirms PM's
scope cut.

## 7. Test matrix for `fs_state_store_test.go`

Extend the existing table-style `TestInitFileSystemStateStore` (pattern already in the
file: `t.TempDir()` + `t.Run` subtests). New subtests, each needs a real git repo set up
with the package's own `git.New().Init(...)` (mirrors `pkg/api/git/repo_lifecycle_test.go`'s
`newRepoIn` helper — that helper is private to the `git` package and not exported, so the
pulumi-package test must build its own minimal repo with `os/exec` `git init` or via the
public `git.New()`/`Init` API from the `git` package, which IS exported):

| Subtest | Setup | Expectation |
|---|---|---|
| in-repo path gets ignored | `git.New()` + `Init(root)`; `path = root + "/.sc/state"` | after calling `initFileSystemStateStore`, `root/.gitignore` contains `.sc/state/` |
| path outside repo is left alone | git repo at `root`; `path` is a sibling `t.TempDir()` NOT under `root` | no `.gitignore` entry added (file may not exist at all, or exists but lacks the entry) |
| path equal to repo root is left alone | git repo at `root`; `path = root` itself | no `.gitignore` entry added (guards the `rel == "."` catastrophe case from §4) |
| no enclosing git repo at all | plain `t.TempDir()`, no `git init` | `initFileSystemStateStore` still succeeds (returns nil error) and creates no `.gitignore` |
| idempotent re-provision | in-repo path; call `initFileSystemStateStore` twice | `.gitignore` has exactly ONE line matching the entry (count via `strings.Count`) |

Because `detectRootDir()` opens relative to the **process cwd**, these subtests must
`os.Chdir` into the fabricated repo root before calling `initFileSystemStateStore` and
restore the original cwd via `t.Cleanup` (existing `repo_lifecycle_test.go` comment on a
similar cwd-sensitive test: "must always restore the original cwd" — follow that
discipline here too, since Go test subtests can run in any order within a package even
without `t.Parallel()`).

## 8. Doc cleanup (AC4)

`docs/docs/examples/vm-compose/byo-host/server.yaml` (content read this turn) carries this
paragraph directly above `provisioner:`:

> `path` is a plain directory and SC creates it, mode 0700. Writing the Pulumi DIY URL in
> full (`file://${git:root}/.sc/state`) means the same thing.
>
> Add `.sc/state` to .gitignore yourself. Nothing does it for you: `sc init` ignores only
> the profile file and `sc secrets reveal` only the revealed secrets file. The checkpoint
> under this path carries the stack's inputs and outputs — the deploy key and the env file
> among them, encrypted with the passphrase above, which is exactly as strong as that
> passphrase and no stronger.

Delete the second paragraph (the "Add `.sc/state` to .gitignore yourself..." one) in full
once §6 ships — it becomes false once SC does this itself. Keep the first paragraph (it
documents `path`/mode 0700 behavior, unrelated to this fix) and the passphrase-strength
caveat if it's judged still useful — Developer's call, not load-bearing either way.

## 9. AC5 — non-blocking look, conclusion only (no new code)

`.pulumi/history/<stack>/` and `.pulumi/backups/<stack>/` (the Pulumi DIY backend's own
siblings) are written as children of the same `path:` directory. §5's directory-trailing-
slash entry (`rel + "/"`) already covers them — gitignore directory patterns are recursive
by default. No separate entries, no separate code path. (Not independently verified by
running a live `sc provision` this turn — this is a read of Pulumi's documented DIY-backend
layout via the existing PM research; flag for QA to confirm with a live smoke if one is run
for this slice.)

## 10. Non-goals (explicit, matches PM scope cuts)

- No change to `AddFileToIgnore`'s dedup logic.
- No change to `FileSystemStateStorage.StorageUrl()`.
- No change to `pApi.InitStateStoreFunc`'s signature or registration.
- No audit/fix of other DIY-backend-on-disk gaps elsewhere in the codebase — `record_task`
  if Developer trips over one.

## 11. Gate (unchanged from PM's brief)

`make check` (repo root) + `go test ./pkg/clouds/pulumi/... ./pkg/api/git/...`. Never bare
`./...`, never `make build`/`make build-all`. Lint scope:
`bin/gofumpt -l -w ./pkg/clouds/pulumi/` + `bin/golangci-lint run --fix ./pkg/clouds/pulumi/...`.
Commits need `git commit -s` (DCO).
