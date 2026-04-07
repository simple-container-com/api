# Branch Preview Workflow — Implementation Notes

**Date**: 2026-04-07
**Feature**: `feature/branch-builds-for-preview-versions`
**Design doc**: `docs/design/2026-04-07/branch-preview-workflow/architecture.md`

---

## Deliverable

**File**: `.github/workflows/branch-preview.yaml`

---

## Implementation decisions

### Version generation
Using `reecetech/version-increment@2023.10.2` with `use_api: "false"` — identical to `push.yaml` for the CalVer increment logic, but without creating an actual git tag. A subsequent shell step appends `-preview.{short_sha7}` to the computed base version. The real git tag is only created in `publish-git-tag`.

### `${{ secrets.SC_CONFIG }}` in job summary
GitHub Actions processes all `${{ expr }}` in `run:` blocks before passing to the shell. To write the literal string `${{ secrets.SC_CONFIG }}` into the step summary markdown, the env var `SC_CONFIG_EXPR` is set via `"${{ '${{' }} secrets.SC_CONFIG }}"` — GHA evaluates `${{ '${{' }}` to the string `${{`, yielding `${{ secrets.SC_CONFIG }}` as the env var value. The heredoc then expands `${SC_CONFIG_EXPR}` safely.

### Backtick escaping in heredoc
Unquoted bash heredocs treat `` ` `` as command substitution. Markdown code fences (` ``` `) are written as `` \`\`\` `` in the heredoc to produce literal backticks in the output.

### `publish-sc-preview` bundle safety
Only versioned tarballs (`sc-{os}-{arch}-v{version}.tar.gz`) are placed in `.sc/stacks/dist/bundle/`. `sc.sh` and `version` are deliberately omitted. **Risk**: if `welder deploy -e prod` performs a full sync/delete of the CDN bucket, files absent from the bundle (including the live `sc.sh`) could be deleted. This must be validated against the `dist` stack Pulumi code before merging to a shared environment.

### `publish-git-tag` — separate release branch
A new `release/{version}` branch is created from the current HEAD. All four `action.yml` files are patched to reference `simplecontainer/github-actions:{version}` (replacing `:staging`). A commit is made on this branch, then tagged `v{version}`. Both the branch and the tag are pushed. The working branch is never modified.

### `docker-build` does not need `build-platforms`
The `github-actions.Dockerfile` only copies `dist/github-actions` (the server binary), not any SC platform tarballs. This allows `docker-build` to start as soon as `build-binaries` + `test` finish, in parallel with the `build-platforms` matrix.

### `publish-sc-preview` and `publish-git-tag` run in parallel
These two jobs have no dependency on each other:
- `publish-sc-preview` only needs SC platform tarballs + passing tests
- `publish-git-tag` only needs the Docker image to exist (gated by `docker-build`)
Both feed into `finalize`.

---

## Known issues / follow-ups

- [ ] Validate `welder deploy -e prod` bundle sync behavior (see `publish-sc-preview` risk above)
- [ ] Confirm `reecetech/version-increment` with `use_api: "false"` does not create git tags (needs runtime verification)
- [ ] `release/{version}` branches accumulate over time — consider a cleanup strategy (e.g., auto-delete after 30 days)

---

## Status

- [x] Implementation docs created
- [x] `prepare` job
- [x] `build-setup` job
- [x] `build-platforms` job (matrix)
- [x] `build-binaries` job
- [x] `test` job
- [x] `docker-build` job
- [x] `publish-sc-preview` job
- [x] `publish-git-tag` job
- [x] `finalize` job with build summary
