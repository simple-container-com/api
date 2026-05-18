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
#   0  Release created (or skipped because tag is preview/RC)
#   1  Hard failure (no .sigstore.json, no assets, gh release error)

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

# Collect every signed sidecar + SBOM + checksum + tarball for the
# specific version we just published. The gh CLI takes the tag
# positionally, followed by any number of asset paths.
assets=()
for f in sc-*-v${VERSION}.tar.gz \
         sc-*-v${VERSION}.tar.gz.sha256 \
         sc-*-v${VERSION}.tar.gz.cosign-bundle \
         sc-*-v${VERSION}.tar.gz.sigstore.json \
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
gh release create "${VERSION}" "${assets[@]}" \
  --repo "${GITHUB_REPOSITORY}" \
  --title "v${VERSION}" \
  --generate-notes \
  --verify-tag
