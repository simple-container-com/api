# Releases

This doc covers how Simple Container (`sc`) is released, where to read
release notes, how vulnerabilities are surfaced in notes, and how the
release ↔ disclosure flow ties together.

It satisfies the OpenSSF Best Practices `release_notes`,
`release_notes_vulns`, and `report_archive` criteria.

## Where to read release notes

| What | Where |
|---|---|
| **Per-release human-readable notes** | <https://github.com/simple-container-com/api/releases> |
| **Public vulnerability advisories** | <https://github.com/simple-container-com/api/security/advisories> |
| **Public issue / PR archive (reports + responses)** | <https://github.com/simple-container-com/api/issues> + <https://github.com/simple-container-com/api/pulls?q=is%3Apr+is%3Aclosed> |
| **Internal phase-by-phase hardening tracker** | [`HARDENING.md`](../HARDENING.md) (repo-side) |

## How release notes are produced

Every push to `main` triggers `.github/workflows/push.yaml`. After
build + sign + publish to dist, the `docker-finalize` job invokes
[`scripts/create-github-release.sh`](../scripts/create-github-release.sh)
which calls:

```bash
gh release create "${VERSION}" "${assets[@]}" \
  --repo "${GITHUB_REPOSITORY}" \
  --title "v${VERSION}" \
  --generate-notes \
  --verify-tag
```

The `--generate-notes` flag uses GitHub's auto-generation: it walks
back to the previous tag and produces a categorised summary of merged
PRs since then, **plus** a "New Contributors" callout. This is **not**
raw `git log` output — it groups by PR + author + label, with PR
titles and authors hyperlinked.

Release notes are attached to each Release object alongside the signed
sidecars (`.cosign-bundle`, `.sigstore.json`, `.sha256`, `.sbom.cdx.json`).
Consumers can verify the integrity of the artifacts AND read the notes
in the same Release UI.

## How vulnerabilities appear in release notes

Security-relevant changes follow these conventions so they surface
clearly in the auto-generated notes:

1. **Commit subject prefix**: security fixes use `fix(security):` /
   `fix(deps):` (for CVE-closing dep bumps) / `hotfix(...)`. Examples
   from history:

   - `fix(deps): bump docs python 3.9 → 3.12-slim + pin requests/urllib3 to patched versions`
     (closes GHSA-gc5v-m9x4-r6x2, GHSA-mf9v-mfxr-j63j, GHSA-qccp-gfcp-xxvc)
   - `fix(deps): migrate aws-sdk-go v1 → v2 (Vulnerabilities 8→10)`
     (addresses GO-2022-0635, GO-2022-0646)
   - `hotfix(sc.sh): drop invalid --yes flag on cosign verify-blob`
     (production outage — installs broken since #264)

2. **CVE / GHSA references**: the body of every security-fix commit
   includes the upstream advisory IDs (`GHSA-...`, `CVE-...`,
   `GO-YYYY-NNNN`) it closes. These show up in the release-notes
   summary via the linked PR.

3. **GitHub Security Advisories**: when a security report reaches
   maintainers via the channels in [`SECURITY.md`](SECURITY.md), the
   fix lands via a regular PR, and the corresponding GHSA is published
   on <https://github.com/simple-container-com/api/security/advisories>.
   The advisory cross-links the affected version + the patch PR + the
   release tag, so a consumer reading the release notes can pivot to
   the advisory page directly.

4. **N/A is honest**: if a given release fixed no publicly-known
   vulnerabilities, no vulnerability section is forced into the notes.
   The `--generate-notes` output simply omits a security callout.

## Reading a release for security implications

For a given release `vYYYY.M.X`:

1. Open <https://github.com/simple-container-com/api/releases/tag/YYYY.M.X>
   (note: tag has no `v` prefix in our scheme; the Release title shows
   `v` for human readability).
2. The auto-generated body lists every merged PR. Filter mentally for
   PRs whose title starts with `fix(security)`, `fix(deps)`, or
   `hotfix(...)`.
3. Click through to any PR for the full commit body — that's where the
   CVE / GHSA ID lives.
4. Cross-check against the advisory archive at
   <https://github.com/simple-container-com/api/security/advisories>
   for any advisory marked **Published** with a `Patched in` field
   matching this release.
5. The dist-side tarballs at `https://dist.simple-container.com/sc-*-vYYYY.M.X.tar.gz`
   ship the same release. Verify their integrity with `cosign verify-blob`
   per [`SECURITY.md`](SECURITY.md) before upgrading.

## Release cadence

- **Production releases**: cut automatically on every merge to `main`.
  Calver scheme `YYYY.M.X` driven by
  `reecetech/version-increment@2023.10.2`. There is no manual release
  process — every accepted change in `main` ships.
- **Preview releases**: produced by `.github/workflows/branch-preview.yaml`
  on a `workflow_dispatch` trigger from a feature branch. Tagged
  `vYYYY.M.X-pre.SHORTSHA-preview.SHORTSHA`. These do NOT create GitHub
  Release objects (filtered by `scripts/create-github-release.sh`),
  but their signed sidecars are still published to `dist.*`.
- **Hotfixes**: same flow as a regular fix. Merge to `main` →
  automatic release. No separate hotfix branch model. See PR #268 for
  the canonical hotfix example (cosign `--yes` flag bug).

## Vulnerability report archive

Public issue and PR threads — including security-fix PR discussions
once their corresponding advisory is published — are archived at:

- <https://github.com/simple-container-com/api/issues> — open + closed
  issues
- <https://github.com/simple-container-com/api/pulls?q=is%3Apr+is%3Aclosed>
  — closed PRs
- <https://github.com/simple-container-com/api/security/advisories> —
  Published GitHub Security Advisories

GitHub's full-text search across these surfaces makes the archive
searchable without additional infrastructure.

## Cross-references

- [`SECURITY.md`](SECURITY.md) — threat model, reporting channels,
  disclosure cadence
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — contribution requirements
- [`MAINTAINERS.md`](MAINTAINERS.md) — who handles security responses
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — system actors + trust
  boundaries the release pipeline operates within
- [`DEPENDENCIES.md`](DEPENDENCIES.md) — dep selection + tracking that
  drives most of the `fix(deps)` security PRs
- [`../HARDENING.md`](../HARDENING.md) — phase-by-phase hardening
  tracker, including the deferred-CVE log
