#!/usr/bin/env bash
# Install the pulumi CLI at the version of github.com/pulumi/pulumi/sdk/v3 in
# go.mod, SHA-256 verified against the per-release checksums file Pulumi
# publishes on GitHub Releases — deliberately NOT the get.pulumi.com installer,
# which pipes a remote download straight into a shell. (Phrasing matters here:
# the `action.rules.shell-curl-pipe-to-shell` Semgrep rule matches on the text,
# so spelling out the pattern we avoid fails the gate. Live-caught in PR #460.)
#
# WHY THIS EXISTS. `sc deploy` / `sc provision` shell out to the pulumi CLI —
# pkg/clouds/pulumi/login.go prepends ~/.pulumi/bin to PATH and then points
# PULUMI_HOME at ~/.pulumi/sc/<project>/<stack>. Nothing in this repo installed
# it: the Blacksmith / GitHub-hosted ubuntu-2204 image happened to ship one. The
# self-hosted Firecracker fleet (vars.FORGE_CI_RUNNER) does not, so once api's CI
# moved onto it every bare-metal `sc deploy` died with
#
#   failed to create stack: failed to run `pulumi version`:
#   exec: "pulumi": executable file not found in $PATH
#
# (run 39227894781, 2026-10-19 — the whole release built, then failed on the last
# step). The released images were never affected: github-actions.Dockerfile bakes
# pulumi in, and that image is where the published deploy-client-stack action and
# every forge-* service deploy run.
#
# WHY NOT BAKE IT INTO THE RUNNER IMAGE. The CLI has to track the SDK in go.mod;
# a copy in the guest rootfs would drift silently. This script is idempotent, and
# a fleet VM boots once and re-registers its runner in a loop inside the same
# long-lived guest, so $HOME survives and the download is paid once per VM rather
# than once per job.
#
# Keep the version derivation + checksum verification here in step with
# github-actions.Dockerfile (the "Pulumi: version from go.mod" layer) — they are
# the same contract installed two ways.
#
# Usage: run from the repo root.
#   PULUMI_DOCKER_PLUGIN=<ver>  optional — also install the docker resource
#                               plugin at that version (the engine tests in
#                               branch.yaml need it; the deploy jobs do not).
set -euo pipefail

GO_MOD="${GO_MOD:-go.mod}"
PULUMI_MODULE="github.com/pulumi/pulumi/sdk/v3"
INSTALL_DIR="${HOME}/.pulumi/bin"

[ -f "${GO_MOD}" ] || {
	echo "::error::${GO_MOD} not found — run this from the repo root" >&2
	exit 1
}

# A `replace` of the pulumi SDK would make the require-block version a lie. This
# is the one case `go list -m` used to handle for us; we parse go.mod directly
# because the jobs that need this have no usable Go toolchain (docker-finalize
# has no setup-go, and the fleet guest bakes an older Go than the `go` directive
# with GOTOOLCHAIN=local). Fail loudly rather than install the wrong version.
if grep -qE "(^|[[:space:]])${PULUMI_MODULE//./\\.}([[:space:]]+v[^[:space:]]+)?[[:space:]]+=>" "${GO_MOD}"; then
	echo "::error::${GO_MOD} has a replace directive for ${PULUMI_MODULE}; this script only reads the require block" >&2
	exit 1
fi

PULUMI_VERSION="$(awk -v m="${PULUMI_MODULE}" '$1 == m { print $2; exit }' "${GO_MOD}" | sed 's/^v//')"
[ -n "${PULUMI_VERSION}" ] || {
	echo "::error::no ${PULUMI_MODULE} version in ${GO_MOD}" >&2
	exit 1
}

case "$(uname -s)-$(uname -m)" in
	Linux-x86_64) PLATFORM="linux-x64" ;;
	Linux-aarch64 | Linux-arm64) PLATFORM="linux-arm64" ;;
	Darwin-x86_64) PLATFORM="darwin-x64" ;;
	Darwin-arm64) PLATFORM="darwin-arm64" ;;
	*)
		echo "::error::unsupported platform $(uname -s)-$(uname -m)" >&2
		exit 1
		;;
esac

# Put the install dir on PATH for later steps regardless of whether we download:
# a warm guest still needs the export. (sc prepends ~/.pulumi/bin itself, but
# anything else in the job — and this script's own smoke test — wants it too.)
export PATH="${INSTALL_DIR}:${PATH}"
if [ -n "${GITHUB_PATH:-}" ]; then
	echo "${INSTALL_DIR}" >> "${GITHUB_PATH}"
fi

if [ "$("${INSTALL_DIR}/pulumi" version 2>/dev/null || true)" = "v${PULUMI_VERSION}" ]; then
	echo "pulumi v${PULUMI_VERSION} already installed at ${INSTALL_DIR} — skipping download"
else
	TARBALL="pulumi-v${PULUMI_VERSION}-${PLATFORM}.tar.gz"
	CHECKSUMS="pulumi-${PULUMI_VERSION}-checksums.txt"
	BASE_URL="https://github.com/pulumi/pulumi/releases/download/v${PULUMI_VERSION}"
	WORK_DIR="$(mktemp -d)"
	trap 'rm -rf "${WORK_DIR}"' EXIT

	# --retry-all-errors because the fleet's guest egress is a fake-IP hijack
	# where transient i/o timeouts are the ambient condition — the same reason
	# the cosign steps retry (see push.yaml).
	echo "Downloading ${TARBALL}..."
	curl -fsSL --retry 3 --retry-all-errors -o "${WORK_DIR}/${TARBALL}" "${BASE_URL}/${TARBALL}"
	curl -fsSL --retry 3 --retry-all-errors -o "${WORK_DIR}/${CHECKSUMS}" "${BASE_URL}/${CHECKSUMS}"

	# Exact field match, not a substring grep: the checksums file also lists
	# sibling artifacts whose names contain this one as a prefix.
	EXPECTED_SHA="$(awk -v f="${TARBALL}" '$2 == f { print $1; exit }' "${WORK_DIR}/${CHECKSUMS}")"
	[ -n "${EXPECTED_SHA}" ] || {
		echo "::error::no checksum entry for ${TARBALL} in ${CHECKSUMS}" >&2
		exit 1
	}
	(cd "${WORK_DIR}" && echo "${EXPECTED_SHA}  ${TARBALL}" | sha256sum -c -)

	# The tarball extracts to a `pulumi/` directory holding the binaries.
	tar -xzf "${WORK_DIR}/${TARBALL}" -C "${WORK_DIR}"
	mkdir -p "${INSTALL_DIR}"
	mv -f "${WORK_DIR}/pulumi/"* "${INSTALL_DIR}/"
fi

# Smoke test, same intent as the image's build-time `pulumi version > /dev/null`:
# fail here, with the version printed, rather than inside `sc deploy`.
INSTALLED="$(pulumi version)"
[ "${INSTALLED}" = "v${PULUMI_VERSION}" ] || {
	echo "::error::installed pulumi reports ${INSTALLED}, wanted v${PULUMI_VERSION}" >&2
	exit 1
}
echo "pulumi ${INSTALLED} ready (${INSTALL_DIR})"

if [ -n "${PULUMI_DOCKER_PLUGIN:-}" ]; then
	pulumi plugin install resource docker "${PULUMI_DOCKER_PLUGIN}"
fi
