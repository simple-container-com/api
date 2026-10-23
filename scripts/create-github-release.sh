#!/usr/bin/env bash
#
# Create a GitHub Release on the just-pushed production tag and attach
# every signed sidecar produced by the publish pipeline.
#
# Invoked from .github/workflows/push.yaml docker-finalize job, after
# `welder run tag-release` (which pushes the git tag) and `welder
# deploy -e prod` (which uploads the tarballs + sidecars to dist).
#
# Why a Release object: OpenSSF Scorecard's Signed-Releases check
# queries GitHub's /releases API, NOT the tag list or the CDN. Without
# Release objects, the check scores -1 (no releases found), even
# though we've been signing tarballs since Phase 2 (PR #257).
#
# Scorecard recognises these signature suffixes (per upstream
# probes/releasesAreSigned/impl.go):
#   .asc / .minisig / .sig / .sign / .sigstore / .sigstore.json
# We attach `.sigstore.json` — the SLSA build provenance bundle from
# actions/attest-build-provenance@v4. After 5 of the last 5 releases
# ship a Release object with a .sigstore.json asset → 10/10.
#
# Required env:
#   VERSION             - calver from prepare.outputs.version (no `v`)
#   GH_TOKEN            - GitHub token with contents:write scope
#   GITHUB_REPOSITORY   - owner/repo, populated automatically in Actions
#
# Optional env:
#   SC_DIST_BUNDLE_DIR  - dist bundle directory; default
#                         `.sc/stacks/dist/bundle` matching push.yaml
#
# Exit codes:
#   0  Release created, or its assets refreshed on an already-existing
#      Release, or skipped because the tag is preview/RC
#   1  Hard failure (no .sigstore.json, no assets, gh release error)
#
# Idempotency: re-running a release job is the normal recovery path in
# this pipeline (image legs hit transient CDN/registry flakes), so the
# script must survive re-entry on a tag whose Release already exists.
# See the create/upload branch at the bottom of this file.
#
# Verification: scripts/test-create-github-release.sh exercises every
# branch below against a stub `gh` on PATH — run it after any edit here.

set -euo pipefail

: "${VERSION:?VERSION env var required}"
: "${GH_TOKEN:?GH_TOKEN env var required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY env var required}"

# Skip preview / release-candidate tags. push.yaml fires on push to
# main; preview tags come from branch-preview.yaml and never trigger
# this path, but guarding here makes the script safe to invoke from
# anywhere (e.g., manual one-off backfill for an older prod tag).
if [[ "${VERSION}" == *"-pre."* || "${VERSION}" == *"-preview."* ]]; then
  echo "Skipping Release creation: ${VERSION} is a preview/RC tag."
  exit 0
fi

cd "${SC_DIST_BUNDLE_DIR:-.sc/stacks/dist/bundle}"
shopt -s nullglob

# Refuse to publish a Release without a Scorecard-recognised signature
# for at least one tarball. The Phase 2 sign/SBOM/SLSA steps are still
# `continue-on-error` (per the 14-day bake-in plan), so it's technically
# possible for the dist bundle to contain tarballs + .sha256 but no
# .sigstore.json. Creating an unsigned GitHub Release in that state
# would yield a Scorecard `release artifact vX.Y.Z not signed` finding —
# worse than no release at all.
arr_sigstores=( sc-*-v${VERSION}.tar.gz.sigstore.json )
if [ ${#arr_sigstores[@]} -eq 0 ]; then
  echo "::error title=No SLSA provenance::dist bundle has zero .sigstore.json sidecars for v${VERSION}; refusing to create an unsigned GitHub Release."
  exit 1
fi

# Dual-publish each .sigstore.json as a `.intoto.jsonl` alias so
# OpenSSF Scorecard's `releasesHaveProvenance` probe finds provenance
# alongside the existing signature. Scorecard's
# probes/releasesHaveProvenance/impl.go matches on the
# `.intoto.jsonl` suffix and ignores `.sigstore.json`, even though the
# latter IS the SLSA build-provenance bundle from
# actions/attest-build-provenance@v4. Keep the .sigstore.json as
# canonical for proper consumers (cosign / sigstore CLI); the
# .intoto.jsonl copy exists solely to satisfy the probe. Naming-wise,
# the Sigstore bundle is a single JSON object — not line-delimited —
# so `.intoto.jsonl` is technically misleading but is what Scorecard
# expects today (tracked in scorecard#3699).
for sb in sc-*-v${VERSION}.tar.gz.sigstore.json; do
  cp "$sb" "${sb%.sigstore.json}.intoto.jsonl"
done

# Collect every signed sidecar + SBOM + checksum + tarball for the
# specific version we just published. The gh CLI takes the tag
# positionally, followed by any number of asset paths.
assets=()
for f in sc-*-v${VERSION}.tar.gz \
         sc-*-v${VERSION}.tar.gz.sha256 \
         sc-*-v${VERSION}.tar.gz.cosign-bundle \
         sc-*-v${VERSION}.tar.gz.sigstore.json \
         sc-*-v${VERSION}.tar.gz.intoto.jsonl \
         sc-*-v${VERSION}.tar.gz.sbom.cdx.json; do
  assets+=( "$f" )
done

if [ ${#assets[@]} -eq 0 ]; then
  echo "::error title=No release assets::dist bundle has no tarballs or sidecars to attach"
  exit 1
fi

# Tag name matches what welder.yaml's tag-release task pushed, which
# is `${project:version}` — NO `v` prefix. (branch-preview.yaml
# separately pushes `v`-prefixed tags for preview builds, but those
# are filtered out by the preview-tag guard above.) The Release
# `--title` keeps the human-readable `v` for UI niceness.
#
# --verify-tag ensures `welder run tag-release` already pushed the
# tag (it ran two steps earlier in push.yaml). --generate-notes pulls
# release notes from commits/PRs since the previous tag (default
# GitHub formatting — flat list of merged PRs).
#
# Idempotency: `gh release create` hard-fails with "a release with the
# same tag name already exists" when re-entered on a tag that already
# has a Release — and with `set -e` that reds the whole job even though
# every artefact published correctly. Re-running is the supported
# recovery path here (push.yaml sets `cancel-in-progress: false`), so
# branch on whether the Release exists and refresh its assets instead.
#
# `gh release upload` takes neither --title, --generate-notes nor
# --verify-tag: the existing Release object already carries the title
# and notes from the run that created it, and the tag is proven to
# exist by the Release referencing it. --clobber overwrites same-named
# assets in place rather than erroring or duplicating them.
#
# A transient `gh release view` failure (rate-limit, auth blip) is
# indistinguishable from a 404 here and falls through to the create
# branch. That is deliberate and safe: `create` is still the
# authoritative action and still fails loudly under `set -e` if the
# Release really did exist. Do not add retry/backoff around `view`.
if gh release view "${VERSION}" --repo "${GITHUB_REPOSITORY}" >/dev/null 2>&1; then
  echo "Release ${VERSION} already exists — refreshing its assets (re-run recovery path)."
  gh release upload "${VERSION}" "${assets[@]}" \
    --repo "${GITHUB_REPOSITORY}" \
    --clobber
else
  gh release create "${VERSION}" "${assets[@]}" \
    --repo "${GITHUB_REPOSITORY}" \
    --title "v${VERSION}" \
    --generate-notes \
    --verify-tag
fi
