#!/usr/bin/env bash
#
# Branch coverage for scripts/create-github-release.sh.
#
# Why this exists: that script is the last step of the production
# release pipeline and has no CI coverage of its own — it only ever
# runs for real, on a real tag, with a real GH_TOKEN. The idempotency
# fix (create on a fresh tag / upload --clobber on an existing one)
# added a branch that is invisible on the happy path, and two guards
# that must NOT regress (unsigned-release refusal, preview/RC
# short-circuit).
#
# How: a stub `gh` is prepended to PATH. It records its own argv and
# exits with whatever `STUB_GH_VIEW_EXIT` says for `release view`, so
# every branch can be driven without touching GitHub. Dist bundles are
# fabricated in a temp dir pointed at by SC_DIST_BUNDLE_DIR.
#
# No test framework — plain bash, consistent with the rest of
# scripts/. Run it after any edit to create-github-release.sh:
#
#   ./scripts/test-create-github-release.sh
#
# Exit codes:
#   0  all scenarios passed
#   1  at least one scenario failed (details on stderr)

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNDER_TEST="${SCRIPT_DIR}/create-github-release.sh"
VERSION_FIXTURE="2026.10.99"

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

failures=0
checks=0

# --- stub gh -----------------------------------------------------------
# Records every invocation (one line per call) to ${GH_LOG}. `release
# view` exits with ${STUB_GH_VIEW_EXIT} so the caller picks which
# branch the script under test takes; everything else exits 0.
stub_bin="${workdir}/bin"
mkdir -p "${stub_bin}"
cat >"${stub_bin}/gh" <<'STUB'
#!/usr/bin/env bash
echo "gh $*" >>"${GH_LOG}"
if [ "${1:-}" = "release" ] && [ "${2:-}" = "view" ]; then
  exit "${STUB_GH_VIEW_EXIT:-1}"
fi
exit 0
STUB
chmod +x "${stub_bin}/gh"

# --- helpers -----------------------------------------------------------

# make_bundle <dir> <signed|unsigned>
# Fabricates a dist bundle: tarball + .sha256 always; .sigstore.json
# and .cosign-bundle only when signed.
make_bundle() {
  local dir="$1" mode="$2"
  mkdir -p "${dir}"
  local base="${dir}/sc-linux-amd64-v${VERSION_FIXTURE}.tar.gz"
  : >"${base}"
  : >"${base}.sha256"
  if [ "${mode}" = "signed" ]; then
    : >"${base}.sigstore.json"
    : >"${base}.cosign-bundle"
    : >"${base}.sbom.cdx.json"
  fi
}

# run_under_test <bundle-dir> <view-exit> <version>
# Invokes the script with the stub gh on PATH. Sets globals:
#   RC      exit status
#   OUT     combined stdout+stderr
#   GH_LOG  path to the gh call log
run_under_test() {
  local bundle="$1" view_exit="$2" version="$3"
  GH_LOG="${workdir}/gh-calls.$$.$RANDOM.log"
  : >"${GH_LOG}"
  OUT="$(
    PATH="${stub_bin}:${PATH}" \
    GH_LOG="${GH_LOG}" \
    STUB_GH_VIEW_EXIT="${view_exit}" \
    VERSION="${version}" \
    GH_TOKEN="stub-token-not-a-credential" \
    GITHUB_REPOSITORY="simple-container-com/api" \
    SC_DIST_BUNDLE_DIR="${bundle}" \
    bash "${UNDER_TEST}" 2>&1
  )"
  RC=$?
}

# expect_rc <scenario> <want> <got>
expect_rc() {
  checks=$((checks + 1))
  if [ "$2" != "$3" ]; then
    echo "FAIL [$1] exit status: want $2, got $3" >&2
    echo "----- output -----" >&2
    echo "${OUT}" >&2
    echo "------------------" >&2
    failures=$((failures + 1))
  fi
}

# expect_gh_called <scenario> <substring>
expect_gh_called() {
  checks=$((checks + 1))
  if ! grep -qF -- "$2" "${GH_LOG}"; then
    echo "FAIL [$1] expected a gh call matching '$2'; calls were:" >&2
    sed 's/^/    /' "${GH_LOG}" >&2
    failures=$((failures + 1))
  fi
}

# expect_gh_not_called <scenario> <substring>
expect_gh_not_called() {
  checks=$((checks + 1))
  if grep -qF -- "$2" "${GH_LOG}"; then
    echo "FAIL [$1] did NOT expect a gh call matching '$2'; calls were:" >&2
    sed 's/^/    /' "${GH_LOG}" >&2
    failures=$((failures + 1))
  fi
}

# expect_no_gh_at_all <scenario>
expect_no_gh_at_all() {
  checks=$((checks + 1))
  if [ -s "${GH_LOG}" ]; then
    echo "FAIL [$1] expected zero gh calls; calls were:" >&2
    sed 's/^/    /' "${GH_LOG}" >&2
    failures=$((failures + 1))
  fi
}

# expect_output <scenario> <substring>
expect_output() {
  checks=$((checks + 1))
  case "${OUT}" in
    *"$2"*) ;;
    *)
      echo "FAIL [$1] expected output to contain '$2'; output was:" >&2
      echo "${OUT}" | sed 's/^/    /' >&2
      failures=$((failures + 1))
      ;;
  esac
}

# --- syntax gate -------------------------------------------------------
checks=$((checks + 1))
if ! bash -n "${UNDER_TEST}"; then
  echo "FAIL [syntax] bash -n rejected ${UNDER_TEST}" >&2
  failures=$((failures + 1))
fi

# --- scenario 1: fresh tag → gh release create (AC5a) ------------------
# `gh release view` 404s, dist bundle is signed → the create branch.
make_bundle "${workdir}/fresh" signed
run_under_test "${workdir}/fresh" 1 "${VERSION_FIXTURE}"
expect_rc "fresh-tag" 0 "${RC}"
expect_gh_called "fresh-tag" "gh release view ${VERSION_FIXTURE}"
expect_gh_called "fresh-tag" "gh release create ${VERSION_FIXTURE}"
expect_gh_called "fresh-tag" "--generate-notes"
expect_gh_called "fresh-tag" "--verify-tag"
expect_gh_not_called "fresh-tag" "gh release upload"

# --- scenario 2: existing tag → gh release upload --clobber (AC1/AC2) --
# `gh release view` succeeds → the upload branch, no duplicate create.
make_bundle "${workdir}/existing" signed
run_under_test "${workdir}/existing" 0 "${VERSION_FIXTURE}"
expect_rc "existing-tag" 0 "${RC}"
expect_gh_called "existing-tag" "gh release view ${VERSION_FIXTURE}"
expect_gh_called "existing-tag" "gh release upload ${VERSION_FIXTURE}"
expect_gh_called "existing-tag" "--clobber"
expect_gh_not_called "existing-tag" "gh release create"
expect_output "existing-tag" "already exists"
# `gh release upload` rejects these three flags — passing them would
# make the recovery path fail for a different reason.
expect_gh_not_called "existing-tag" "--generate-notes"
expect_gh_not_called "existing-tag" "--verify-tag"

# --- scenario 3: unsigned bundle, fresh tag → refuse (AC3) -------------
make_bundle "${workdir}/unsigned-fresh" unsigned
run_under_test "${workdir}/unsigned-fresh" 1 "${VERSION_FIXTURE}"
expect_rc "unsigned-fresh" 1 "${RC}"
expect_output "unsigned-fresh" "No SLSA provenance"
expect_no_gh_at_all "unsigned-fresh"

# --- scenario 4: unsigned bundle, EXISTING tag → still refuse (AC3) ----
# The regression this slice most risks: the unsigned refusal must gate
# the re-run path identically, i.e. fire before the create/upload
# branch is ever reached.
make_bundle "${workdir}/unsigned-existing" unsigned
run_under_test "${workdir}/unsigned-existing" 0 "${VERSION_FIXTURE}"
expect_rc "unsigned-existing" 1 "${RC}"
expect_output "unsigned-existing" "No SLSA provenance"
expect_no_gh_at_all "unsigned-existing"

# --- scenario 5: preview / RC tags short-circuit (AC4) -----------------
# Both spellings, and with a signed bundle present so the exit can only
# come from the preview guard.
for preview_version in \
  "${VERSION_FIXTURE}-pre.abc1234" \
  "${VERSION_FIXTURE}-pre.abc1234-preview.abc1234"; do
  make_bundle "${workdir}/preview" signed
  run_under_test "${workdir}/preview" 0 "${preview_version}"
  expect_rc "preview(${preview_version})" 0 "${RC}"
  expect_output "preview(${preview_version})" "Skipping Release creation"
  expect_no_gh_at_all "preview(${preview_version})"
done

# --- result ------------------------------------------------------------
if [ "${failures}" -ne 0 ]; then
  echo "${failures}/${checks} checks FAILED" >&2
  exit 1
fi
echo "all ${checks} checks passed"
