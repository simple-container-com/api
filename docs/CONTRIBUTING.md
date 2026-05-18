# Contributing to Simple Container

Thanks for your interest in contributing. This doc covers how to get a
change in front of maintainers and what we look for in a clean PR.

## TL;DR

1. **Open an issue first** for non-trivial work — saves you a wasted PR
   if the change conflicts with in-flight work or doesn't fit the
   project direction.
2. **Branch from `main`**, name it `feat/...`, `fix/...`, `docs/...`,
   `chore/...`, or `hotfix/...`.
3. **Sign your commits.** SSH or GPG, your choice. `main` enforces signed
   commits; unsigned PRs are blocked at merge.
4. **Write tests** for behavioural changes. The bar is "is this change
   regressable?" — if yes, add a test.
5. **Don't suppress findings.** Lint, SAST, vuln-scan, and Scorecard
   warnings are signals, not noise. If a finding is a real false
   positive, document why in the PR description, not via
   `.trivyignore` / `# nosemgrep` / `// nolint:` unless explicitly
   sanctioned by a maintainer.
6. **One PR per concern.** Keep diffs reviewable.

## Project layout

- `cmd/sc/` — CLI entrypoint
- `pkg/api/` — public consumer-facing types
- `pkg/clouds/` — provider implementations (AWS, GCP, K8s, etc.)
- `pkg/security/` — HMAC cache + signing helpers
- `docs/docs/` — MkDocs site source, published to docs.simple-container.com
- `.github/workflows/` — CI pipelines (push.yaml is the prod path)
- `welder.yaml` — local build + tag-release tasks
- `scripts/` — helpers invoked from CI (kept small and testable)

## Local development

```bash
# Clone + build
git clone https://github.com/simple-container-com/api.git
cd api
go build ./...

# Unit tests
go test ./...

# Fuzz tests (HMAC cache parse path)
go test -run='^$' -fuzz=FuzzVerifyAndExtract -fuzztime=30s ./pkg/security/

# Lint
golangci-lint run    # if installed locally
```

CI runs golangci-lint, `go vet`, `staticcheck`, Semgrep, CodeQL, and
fuzz on every PR — set up local tooling to catch most of these before
push to save round-trips.

## Commit + PR conventions

### Commit messages

Conventional Commits. Subject line ≤ 72 chars:

```
fix(scorecard): pin pip via --require-hashes + Meteor version + CodeQL on all branches
feat(security): Go fuzz tests on HMAC cache parse path
chore: gofmt -s -w on 4 unfmt'd files
hotfix(sc.sh): drop invalid --yes flag on cosign verify-blob
docs(pinned-deps): inline Meteor Dockerfile into README, delete standalone
```

Allowed types: `feat`, `fix`, `hotfix`, `chore`, `docs`, `refactor`,
`test`, `perf`, `ci`, `deps`.

Body explains the *why* — what hidden constraint or threat-model
consideration drove this choice. Don't restate the diff.

### Sign your commits

```bash
git commit -S -s -m "your message"
```

`-S` GPG/SSH-signs the commit; `-s` adds a `Signed-off-by` trailer
(DCO). `main` enforces signed commits via branch protection; unsigned
commits are rejected at merge time.

To set up SSH signing:

```bash
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global gpg.format ssh
git config --global commit.gpgsign true
```

### Pull request

- Title: same Conventional Commits form as the commit subject.
- Body: summary + rationale + test plan + projected impact (e.g.,
  Scorecard score delta for security-leaning work, performance delta
  for perf work).
- Link any related issue with `Closes #N` / `Refs #N`.
- Pass all CI checks before requesting review.
- Code review is **required** — current branch protection enforces 1
  reviewer minimum; this will increase to 2 once admin-UI gates land.
- Use [`/codex-review`](https://github.com/openai/codex) or
  [`/gemini-review`](https://github.com/google-gemini/gemini-cli)
  for an LLM-driven pre-merge sanity pass on larger PRs. Reviewers
  hallucinate — verify any claim empirically before acting on it.

## Security-sensitive changes

If your change touches:

- `pkg/security/` (HMAC cache, signing helpers)
- `.github/workflows/push.yaml` (the prod publish path)
- `sc.sh` (the install bootstrap consumers run via `curl | bash`)
- `docs/SECURITY.md` (the threat model + identity-regex contract)
- Anything in the SLSA / cosign / sigstore chain

… open the PR with a **threat-model note**: which entry in
[`SECURITY.md`](SECURITY.md)'s STRIDE table + attack vectors V1–V5
does this change address or affect, and what's the reachability /
blast-radius story? Maintainers will pull in additional reviewers
(codex + gemini round + human security review) for changes here.

**Never disable verification** as a fix for a verification bug. If
sc.sh's cosign-verify path rejects a release, the bug is in the
signing pipeline or the verify call — not in the verification itself.
See PR #268 for the canonical example of how to handle this.

## Reporting security issues

Don't open a public issue for security bugs. See [SECURITY.md](SECURITY.md)
for the responsible-disclosure channels (GitHub Security Advisory
preferred, email fallback).

## Licensing

By contributing you agree your contributions are licensed under the
MIT License (see [LICENSE](../LICENSE)). The DCO trailer added by
`git commit -s` is your record of this.

## Maintainer cheatsheet

- Release tags are managed by `welder run tag-release` in
  `push.yaml` docker-finalize. The new step
  `Create GitHub Release on production tags` attaches signed sidecars.
- Don't push directly to `main` — branch protection rejects it.
- Don't force-push to shared branches.
- Hotfix flow: `hotfix/...` branch, regular review cycle, merge,
  next prod release picks it up automatically. Production releases
  are NOT manually cut — every push to `main` produces one.

## Questions

- Open a [discussion](https://github.com/simple-container-com/api/discussions)
- Ping a maintainer on the issue you filed

Thanks for helping make this safer.
