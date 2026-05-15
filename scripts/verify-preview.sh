#!/usr/bin/env bash
# verify-preview.sh — end-to-end verification of a Phase 2 preview release.
#
# Usage:
#   scripts/verify-preview.sh [RUN_ID]
#
#   RUN_ID — GitHub Actions run id of a `branch-preview.yaml` dispatch.
#            If omitted, defaults to the latest successful run on the
#            current branch.
#
# Environment:
#   GH_TOKEN — must be set; needs `repo:read` + `actions:read` scope
#              on simple-container-com/api.
#
# What this script verifies (34 checks total):
#   1. The preview workflow succeeded and every matrix item is green.
#   2. Every published image carries a valid cosign keyless signature
#      bound to the PREVIEW trust root.
#   3. Every published image carries a CycloneDX SBOM attestation in
#      the registry .att slot, NOT overwritten by SLSA provenance
#      (round-1 regression guard for #257).
#   4. Every published image's SLSA L3 provenance verifies via the
#      GH-native attestation API.
#   5. Every published tarball has its .sha256, .cosign-bundle, and
#      .sigstore.json sidecars reachable on dist + its SBOM sidecar
#      is well-formed CycloneDX 1.5.
#   6. Cosign verify-blob succeeds against the .cosign-bundle.
#   7. gh attestation verify succeeds against the .sigstore.json.
#   8. Trust-root separation negative checks: a preview signature
#      MUST NOT verify against the PROD identity regex or the STAGING
#      identity regex. (Trust-root-leak regression guard.)
#
# Controls strengthened by this script + the matching workflow inputs:
#   - CICD-SEC-9 (Improper artifact integrity validation) — gains an
#     executable regression test for both positive and negative paths.
#   - NIST SSDF PS.2 (Mechanism to verify software integrity) — gives
#     PS.2 a runnable, repeatable mechanism rather than only docs.
#   - CIS SSCS §5 (Deployment verified at consume) — the documented
#     consumer commands become testable; SECURITY.md paste-and-go
#     correctness is asserted on every release.
#   - SLSA Build L3 verification — proves the spec's verification side
#     is achievable in practice, not just claimed.
#   - OWASP SCVS V6 (Pedigree + provenance) — verification dimension.
#
# Companion: .github/workflows/verify-attestations.yml runs the same
# checks (minus the negative tests by default; flip the
# `negative_tests` dispatch input to include them in CI).

set -uo pipefail

REPO="simple-container-com/api"
DIST_BASE="https://dist.simple-container.com"
OIDC="https://token.actions.githubusercontent.com"

PREVIEW_RE='^https://github\.com/simple-container-com/api/\.github/workflows/branch-preview\.yaml@refs/heads/.+$'
PROD_RE='^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$'
STAGING_RE='^https://github\.com/simple-container-com/api/\.github/workflows/build-staging\.yml@refs/heads/staging$'

PASS=0
FAIL=0
FAIL_DETAILS=()

# ------------------------------------------------------------------
# Output helpers
# ------------------------------------------------------------------
log_check() {  # log_check "<label>" <0|1>
  local label="$1" status="$2"
  if [ "$status" -eq 0 ]; then
    printf '  [PASS] %s\n' "$label"
    PASS=$((PASS + 1))
  else
    printf '  [FAIL] %s\n' "$label"
    FAIL=$((FAIL + 1))
    FAIL_DETAILS+=("$label")
  fi
}

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required tool: $1" >&2
    exit 2
  fi
}

# ------------------------------------------------------------------
# Pre-flight
# ------------------------------------------------------------------
require_tool gh
require_tool cosign
require_tool curl
require_tool jq
require_tool sha256sum
require_tool crane

if [ -z "${GH_TOKEN:-}" ]; then
  echo "GH_TOKEN must be set (use GH_SC for simple-container-com)" >&2
  exit 2
fi

GH_VERSION=$(gh --version | head -n 1 | awk '{print $3}')
GH_MAJOR=$(echo "$GH_VERSION" | cut -d. -f1)
GH_MINOR=$(echo "$GH_VERSION" | cut -d. -f2)
if [ "$GH_MAJOR" -lt 2 ] || { [ "$GH_MAJOR" -eq 2 ] && [ "$GH_MINOR" -lt 49 ]; }; then
  echo "gh >= 2.49 required for 'gh attestation verify'; have $GH_VERSION" >&2
  exit 2
fi

# ------------------------------------------------------------------
# Resolve RUN_ID + VERSION
# ------------------------------------------------------------------
RUN_ID="${1:-}"
if [ -z "$RUN_ID" ]; then
  BRANCH=$(git symbolic-ref --short HEAD)
  RUN_ID=$(gh run list --repo "$REPO" --workflow branch-preview.yaml \
             --branch "$BRANCH" --status success --limit 1 \
             --json databaseId --jq '.[0].databaseId')
  if [ -z "$RUN_ID" ]; then
    echo "No successful branch-preview.yaml run found on $BRANCH" >&2
    exit 2
  fi
  echo "Using latest run on $BRANCH: $RUN_ID"
fi

RUN_JSON=$(gh api "repos/$REPO/actions/runs/$RUN_ID")
RUN_STATUS=$(echo "$RUN_JSON" | jq -r '.status')
RUN_CONCLUSION=$(echo "$RUN_JSON" | jq -r '.conclusion')
HEAD_SHA=$(echo "$RUN_JSON" | jq -r '.head_sha' | cut -c1-7)

echo "============================================================"
echo "Run:       $RUN_ID"
echo "Status:    $RUN_STATUS / $RUN_CONCLUSION"
echo "Head SHA:  $HEAD_SHA"
echo "============================================================"
echo

# Section 1 — workflow smoke
echo "[1] Workflow smoke"
[ "$RUN_STATUS" = "completed" ] && [ "$RUN_CONCLUSION" = "success" ]
log_check "1.1 workflow_run completed successfully" $?

VERSION=$(gh api "repos/$REPO/git/refs/tags" --paginate \
            --jq "[.[] | .ref | ltrimstr(\"refs/tags/v\")] | map(select(contains(\"preview.$HEAD_SHA\"))) | last")
if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
  echo "could not resolve preview version for head_sha=$HEAD_SHA" >&2
  exit 2
fi
echo "Version:   $VERSION"
echo

# Section 2 — every matrix job passed
echo "[2] Per-job state"
JOBS_JSON=$(gh api "repos/$REPO/actions/runs/$RUN_ID/jobs" --paginate)
while IFS=$'\t' read -r name conclusion; do
  [ "$conclusion" = "success" ]
  log_check "2.x $name=$conclusion" $?
done < <(echo "$JOBS_JSON" | jq -r '.jobs[] | select(.name | test("Docker build|Build sc for")) | "\(.name)\t\(.conclusion)"')
echo

# Section 3 — image attestation (2 images × 3 primitives)
echo "[3] Image attestation"
for img in simplecontainer/github-actions simplecontainer/cloud-helpers; do
  case "$img" in
    *cloud-helpers*) tag="aws-$VERSION" ;;
    *)               tag="$VERSION" ;;
  esac
  ref="$img:$tag"

  cosign verify "$ref" \
      --certificate-identity-regexp "$PREVIEW_RE" \
      --certificate-oidc-issuer "$OIDC" >/dev/null 2>&1
  log_check "3a cosign verify $ref" $?

  cosign verify-attestation "$ref" --type cyclonedx \
      --certificate-identity-regexp "$PREVIEW_RE" \
      --certificate-oidc-issuer "$OIDC" >/dev/null 2>&1
  log_check "3b cosign verify-attestation cyclonedx $ref" $?

  gh attestation verify "oci://$ref" \
      --repo "$REPO" \
      --cert-identity-regex "$PREVIEW_RE" \
      --cert-oidc-issuer "$OIDC" >/dev/null 2>&1
  log_check "3c gh attestation verify $ref" $?
done
echo

# Section 4 — tarball attestation (3 platforms × 5 checks)
echo "[4] Tarball attestation"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for plat in linux-amd64 darwin-arm64 darwin-amd64; do
  T="sc-$plat-v$VERSION.tar.gz"
  ok=1
  for ext in "" .sha256 .cosign-bundle .sigstore.json .sbom.cdx.json; do
    if curl -fsSL "$DIST_BASE/$T$ext" -o "$tmp/$(basename "$T$ext")" 2>/dev/null; then
      log_check "4a $T$ext reachable on dist" 0
    else
      log_check "4a $T$ext reachable on dist" 1
      ok=0
    fi
  done
  [ "$ok" -eq 0 ] && continue

  ( cd "$tmp" && sha256sum -c "$T.sha256" >/dev/null 2>&1 )
  log_check "4b sha256sum -c $T.sha256" $?

  cosign verify-blob \
      --bundle "$tmp/$T.cosign-bundle" \
      --certificate-identity-regexp "$PREVIEW_RE" \
      --certificate-oidc-issuer "$OIDC" \
      "$tmp/$T" >/dev/null 2>&1
  log_check "4c cosign verify-blob $T" $?

  gh attestation verify "$tmp/$T" \
      --bundle "$tmp/$T.sigstore.json" \
      --repo "$REPO" \
      --cert-identity-regex "$PREVIEW_RE" \
      --cert-oidc-issuer "$OIDC" >/dev/null 2>&1
  log_check "4d gh attestation verify $T" $?

  jq -e '.bomFormat == "CycloneDX"' "$tmp/$T.sbom.cdx.json" >/dev/null 2>&1
  log_check "4e $T SBOM well-formed CycloneDX" $?
done
echo

# Section 5 — negative trust-root tests (regression guard for CICD-SEC-9)
echo "[5] Negative trust-root tests"
ref="simplecontainer/github-actions:$VERSION"

# Each check asserts the cosign verify call FAILS (trust-root separation).
if cosign verify "$ref" \
    --certificate-identity-regexp "$PROD_RE" \
    --certificate-oidc-issuer "$OIDC" >/dev/null 2>&1; then
  log_check "5a preview correctly REJECTED by prod regex" 1
else
  log_check "5a preview correctly REJECTED by prod regex" 0
fi

if cosign verify "$ref" \
    --certificate-identity-regexp "$STAGING_RE" \
    --certificate-oidc-issuer "$OIDC" >/dev/null 2>&1; then
  log_check "5b preview correctly REJECTED by staging regex" 1
else
  log_check "5b preview correctly REJECTED by staging regex" 0
fi
echo

# Summary
echo "============================================================"
echo "  PASS=$PASS   FAIL=$FAIL   total=$((PASS + FAIL))"
echo "============================================================"
if [ "$FAIL" -gt 0 ]; then
  echo "Failed checks:"
  for f in "${FAIL_DETAILS[@]}"; do
    echo "  - $f"
  done
  exit 1
fi
