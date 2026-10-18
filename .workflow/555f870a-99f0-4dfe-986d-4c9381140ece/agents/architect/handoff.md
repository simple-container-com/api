# Architect Handoff — slice `sc-fs-state-dir-not-gitignored`

## 1. Commit + branch identity

- Commit SHA: `4408f9363c1ccc85344ea9795522d93981a0666e`
- Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/555f870a-99f0-4dfe-986d-4c9381140ece`
- Verified via `list_repository_commits` (path-scoped) — this is the HEAD commit on the branch, authored by `simple-container-forge[bot]` (the write tool's commit identity), subject `docs: design doc for sc-fs-state-dir-not-gitignored`, and I re-read the committed blob at that SHA to confirm its content matches what I intended to write (full byte-for-byte match, 12057 bytes).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/design/2026-10-18/sc-fs-state-dir-not-gitignored/design.md` | +~290/-0 (new file) | New design doc: full technical design for gitignoring the `fs` state dir — read-verified code locations, the two existing `AddFileToIgnore` call sites, the `WithDetectRootDir` mechanism, a decision record rejecting `cache.go`'s `InitOrOpen` pattern (it has a `git init` side effect that's wrong for this call site), the exact containment check (including the `rel == "."` edge case), proposed code, a 5-case test matrix, the AC4 doc-cleanup diff, and the AC5 conclusion (directory-trailing-slash entry already covers Pulumi's `.pulumi/history`/`.pulumi/backups` siblings) |

No production code (`pkg/clouds/pulumi/fs_state_store.go`, `pkg/clouds/pulumi/fs_state_store_test.go`, `docs/docs/examples/vm-compose/byo-host/server.yaml`) was modified in this turn — that is Developer's job per this workflow's PM→Architect→Developer chain. This turn produced the design the Developer implements against.

## 3. Acceptance criteria verification

Architect's role here is to produce a verified, actionable design — not to implement. ACs are evaluated against "does the design satisfy this, and is it unambiguous enough for Developer to implement without re-deriving the hard parts."

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | In-repo `fs` path ends up in `.gitignore`, written by SC | DESIGNED | §6 of design.md: `ensureStateDirIgnored` call appended to `initFileSystemStateStore` after `MkdirAll` succeeds, using `git.New(git.WithDetectRootDir())` + `repo.AddFileToIgnore(rel+"/")` — both primitives read verbatim from `pkg/api/git/opts.go`, `pkg/api/git/repo.go`, `pkg/api/git/ignore.go` this turn |
| AC2 | Out-of-repo path NOT added | DESIGNED | §4: `filepath.Rel(root, dir)` check with explicit `rel == "."` (root-equality) handling beyond what the brief named — found by tracing `detectRootDir()`'s `Workdir()` return value against the hook's already-absolutized `dir` |
| AC3 | Re-provision doesn't duplicate | DESIGNED, delegated correctly | §2/§6: relies entirely on `AddFileToIgnore`'s existing `strings.Contains(currentContent, "\n"+filePath)` dedup, confirmed by reading `pkg/api/git/ignore.go` body this turn — no new dedup layer added |
| AC4 | Remove stale workaround in `byo-host/server.yaml` | DESIGNED (not yet applied) | §8 quotes the exact paragraph read from the live file this turn and specifies precisely what to delete vs. keep — Developer applies it |
| AC5 | Non-blocking — Pulumi `.pulumi/history`/`.pulumi/backups` siblings | ANALYZED, conclusion documented | §9: directory-trailing-slash gitignore entry is recursive, covers the siblings with no extra code — same conclusion PM reached, now tied to the specific code decision in §5/§6 |
| Tests | Unit test matrix for the init hook | DESIGNED | §7: 5-case table (in-repo/outside/root-equality/no-repo/idempotent) with the cwd-chdir caveat called out explicitly (`detectRootDir()` opens relative to process cwd, confirmed by reading `pkg/api/git/repo.go`'s `detectRootDir` body) |

No AC is marked PASS — this is a design turn, not an implementation turn. The design is complete and grounded in code actually read this turn (file paths and line references above are all backed by `read_repository_file`/`search_repository_code` calls made in this turn, not inherited from the PM's prior-turn claims, though they corroborate each other).

## 4. Tests run

None — no source/test files were modified this turn, only a markdown design doc. Developer's turn is where `make check` + `go test ./pkg/clouds/pulumi/... ./pkg/api/git/...` apply (per CLAUDE.md's memory constraint, re-confirmed by fetching `CLAUDE.md` this turn: `make check` = 2.3GB peak RSS, safe; `make build`/`make build-all` need 12.9–14.2GB and must not be run on this box).

## 5. One structural note for Developer (lead with this, per my working style)

The PM's handoff recommended mirroring `cache.go`'s `ensureCacheInGitignore`, which calls `repo.InitOrOpen(projectPath)` after `WithDetectRootDir()`. I traced that call chain this turn (`InitOrOpen` → `Init` → `initRepo`, all in `pkg/api/git/repo.go`) and it has a **side effect the PM didn't flag**: if no `.git` exists at the given path, `Init` runs `git.Init(...)` — i.e., it `git init`s a new repository. That's fine for `cache.go` (which always targets a project root that's presumed to already be a repo, or is fine becoming one), but wrong for this init hook, which runs on *every* `sc provision` against a `path:` that may legitimately be outside any repo or have no enclosing repo at all (AC2's exact scenario). Mirroring `cache.go` literally would silently create stray `.git` directories next to out-of-tree state paths. **Design uses `git.New(git.WithDetectRootDir())` alone, with no `InitOrOpen` call** — detection failure is treated as a skip (log + return), matching the hook's existing forgiving style for the non-`file://`-scheme branch. Flag this if QA or Developer want to double check my trace of `initRepo`.

**Verdict:** signoff