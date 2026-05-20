#!/usr/bin/env bash

set -e;

VERSION="0.0.0"

debug() {
  if [ "$debug" = "debug" ]; then printf "DEBUG: %s$1 \n"; fi
}

# params char
# returns Integer
ord() {
  printf '%d' "'$1"
}


isNumber() {
  string=$1
  char=""
  while true; do
    substract="${string#?}"    # All but the first character of the string
    char="${string%"$substract"}"    # Remove $rest, and you're left with the first character
    string="$substract"
    # no more chars to compare then success
    if [ -z "$char" ]; then
      printf "true"
      return 1
    fi
    # break if some of the chars is not a number
    if [ "$(ord "$char")" -lt 48 ] || [ "$(ord "$char")" -gt 57 ]; then
      printf "false"
      return 0
    fi
  done
}

# params string {String}, Index {Number}
# returns char
getChar() {
  string=$1
  index=$2
  cursor=-1
  char=""
  while [ "$cursor" != "$index" ]; do
    substract="${string#?}"    # All but the first character of the string
    char="${string%"$substract"}"    # Remove $rest, and you're left with the first character
    string="$substract"
    cursor=$((cursor + 1))
  done
  printf "%s$char"
}

outcome() {
  result=$1
  printf "%s$result\n"
}


compareNumber() {
  if [ -z "$1" ] && [ -z "$2" ]; then
    printf "%s" "0"
    return
  fi

  [ $(($2 - $1)) -gt 0 ] && printf "%s" "-1"
  [ $(($2 - $1)) -lt 0 ] && printf "1"
  [ $(($2 - $1)) = 0 ] && printf "0"
}

compareString() {
  result=false
  index=0
  while true
  do
    a=$(getChar "$1" $index)
    b=$(getChar "$2" $index)

    if [ -z "$a" ] && [ -z "$b" ]
    then
      printf "0"
      return
    fi

    ord_a=$(ord "$a")
    ord_b=$(ord "$b")

    if [ "$(compareNumber "$ord_a" "$ord_b")" != "0" ]; then
      printf "%s" "$(compareNumber "$ord_a" "$ord_b")"
      return
    fi

    index=$((index + 1))
  done
}

includesString() {
  string="$1"
  substring="$2"
  if [ "${string#*"$substring"}" != "$string" ]
  then
    printf "1"
    return 1    # $substring is in $string
  fi
  printf "0"
  return 0    # $substring is not in $string
}

removeLeadingV() {
  printf "%s${1#v}"
}

# https://github.com/Ariel-Rodriguez/sh-semversion-2/pull/2
# Spec #2 https://semver.org/#spec-item-2
# MUST NOT contain leading zeroes
normalizeZero() {
  next=$(printf %s "${1#0}")
  if [ -z "$next" ]; then
    printf %s "$1"
  fi
  printf %s "$next"
}

semver_compare() {
  firstParam=$1 #1.2.4-alpha.beta+METADATA
  secondParam=$2 #1.2.4-alpha.beta.2+METADATA
  debug=${3:-1}
  verbose=${4:-1}

  [ "$verbose" = "verbose" ] && set -x

  version_a=$(printf %s "$firstParam" | cut -d'+' -f 1)
  version_a=$(removeLeadingV "$version_a")
  version_b=$(printf %s "$secondParam" | cut -d'+' -f 1)
  version_b=$(removeLeadingV "$version_b")

  a_major=$(printf %s "$version_a" | cut -d'.' -f 1)
  a_minor=$(printf %s "$version_a" | cut -d'.' -f 2)
  a_patch=$(printf %s "$version_a" | cut -d'.' -f 3 | cut -d'-' -f 1)
  a_pre=""
  if [ "$(includesString "$version_a" -)" = 1 ]; then
    a_pre=$(printf %s"${version_a#"$a_major.$a_minor.$a_patch-"}")
  fi

  b_major=$(printf %s "$version_b" | cut -d'.' -f 1)
  b_minor=$(printf %s "$version_b" | cut -d'.' -f 2)
  b_patch=$(printf %s "$version_b" | cut -d'.' -f 3 | cut -d'-' -f 1)
  b_pre=""
  if [ "$(includesString "$version_b" -)" = 1 ]; then
    b_pre=$(printf %s"${version_b#"$b_major.$b_minor.$b_patch-"}")
  fi

  a_major=$(normalizeZero "$a_major")
  a_minor=$(normalizeZero "$a_minor")
  a_patch=$(normalizeZero "$a_patch")
  b_major=$(normalizeZero "$b_major")
  b_minor=$(normalizeZero "$b_minor")
  b_patch=$(normalizeZero "$b_patch")

  unit_types="MAJOR MINOR PATCH"
  a_normalized="$a_major $a_minor $a_patch"
  b_normalized="$b_major $b_minor $b_patch"

  debug "Detected: $a_major $a_minor $a_patch identifiers: $a_pre"
  debug "Detected: $b_major $b_minor $b_patch identifiers: $b_pre"

  #####
  #
  # Find difference between Major Minor or Patch
  #

  cursor=1
  while [ "$cursor" -lt 4 ]
  do
    a=$(printf %s "$a_normalized" | cut -d' ' -f $cursor)
    b=$(printf %s "$b_normalized" | cut -d' ' -f $cursor)
    if [ "$a" != "$b" ]
    then
      debug "$(printf %s "$unit_types" | cut -d' ' -f $cursor) is different"
      outcome "$(compareNumber "$a" "$b")"
      return
    fi;
    debug "$(printf "%s" "$unit_types" | cut -d' ' -f $cursor) are equal"
    cursor=$((cursor + 1))
  done

  #####
  #
  # Find difference between pre release identifiers
  #

  if [ -z "$a_pre" ] && [ -z "$b_pre" ]; then
    debug "Because both are equals"
    outcome "0"
    return
  fi

  # Spec 11.3 a pre-release version has lower precedence than a normal version:
  # 1.0.0 < 1.0.0-alpha
  if [ -z "$a_pre" ]; then
    debug "Because A is the stable release. Pre-release version has lower precedence than a released version"
    outcome "1"
    return
  fi
   # 1.0.0-alpha < 1.0.0
  if [ -z "$b_pre" ]; then
    debug "Because B is the stable release. Pre-release version has lower precedence than a released version"
    outcome "-1"
    return
  fi


  isSingleIdentifier() {
    substract="${2#?}"
    if [ "${1%"$2"}" = "" ]; then
      printf "true"
      return 1;
    fi
    return 0
  }

  cursor=1
  while [ $cursor -lt 5 ]
  do
    a=$(printf %s "$a_pre" | cut -d'.' -f $cursor)
    b=$(printf %s "$b_pre" | cut -d'.' -f $cursor)

    debug "Comparing identifier $a with $b"

    # Exit when there is nothing else to compare.
    # Most likely because they are equals
    if [ -z "$a" ] && [ -z "$b" ]
    then
      debug "are equals"
      outcome "0"
      return
    fi;

    # Spec #11 https://semver.org/#spec-item-11
    # Precedence for two pre-release versions with the same major, minor, and patch version
    # MUST be determined by comparing each dot separated identifier from left to right until a difference is found

    # Spec 11.4.4: A larger set of pre-release fields has a higher precedence than a smaller set, if all of the preceding identifiers are equal.

    if [ -n "$a" ] && [ -z "$b" ]; then
      # When A is larger than B and preidentifiers are 1+n
      # 1.0.0-alpha.beta.1 1.0.0-alpha.beta
      # 1.0.0-alpha.beta.1.2 1.0.0-alpha.beta.1
      debug "Because A has larger set of pre-identifiers"
      outcome "1"
      return
    fi

    # When A is shorter than B and preidentifiers are 1+n
    # 1.0.0-alpha.beta 1.0.0-alpha.beta.d
    # 1.0.0-alpha.beta 1.0.0-alpha.beta.1.2
    if [ -z "$a" ] && [ -n "$b" ]; then
      debug "Because B has larger set of pre-identifiers"
      outcome "-1"
      return
    fi

    # Spec #11.4.1
    # Identifiers consisting of only digits are compared numerically.
    if [ "$(isNumber "$a")" = "true" ] || [ "$(isNumber "$b")" = "true" ]; then

      # if both identifiers are numbers, then compare and proceed
      # 1.0.0-beta.3 1.0.0-beta.2
      if [ "$(isNumber "$a")" = "true" ] && [ "$(isNumber "$b")" = "true" ]; then
        if [ "$(compareNumber "$a" "$b")" != "0" ]; then
          debug "Number is not equal $(compareNumber "$a" "$b")"
          outcome "$(compareNumber "$a" "$b")"
          return
        fi
      fi

      # Spec 11.4.3
      # 1.0.0-alpha.1 1.0.0-alpha.beta.d
      # 1.0.0-beta.3 1.0.0-1.2
      if [ "$(isNumber "$a")" = "false" ]; then
        debug "Because Numeric identifiers always have lower precedence than non-numeric identifiers."
        outcome "1"
        return
      fi
      # 1.0.0-alpha.d 1.0.0-alpha.beta.1
      # 1.0.0-1.1 1.0.0-beta.1.2
      if [ "$(isNumber "$b")" = "false" ]; then
        debug "Because Numeric identifiers always have lower precedence than non-numeric identifiers."
        outcome "-1"
        return
      fi
    else
      # Spec 11.4.2
      # Identifiers with letters or hyphens are compared lexically in ASCII sort order.
      # 1.0.0-alpha 1.0.0-beta.alpha
      if [ "$(compareString "$a" "$b")" != "0" ]; then
        debug "cardinal is not equal $(compareString a b)"
        outcome "$(compareString "$a" "$b")"
        return
      fi
    fi

    # Edge case when there is single identifier exaple: x.y.z-beta
    if [ "$cursor" = 1 ]; then

      # When both versions are single return equals
      # 1.0.0-alpha 1.0.0-alpha
      if [ -n "$(isSingleIdentifier "$b_pre" "$b")" ] && [ -n "$(isSingleIdentifier "$a_pre" "$a")" ]; then
        debug "Because both have single identifier"
        outcome "0"
        return
      fi

      # Return greater when has more identifiers
      # Spec 11.4.4: A larger set of pre-release fields has a higher precedence than a smaller set, if all of the preceding identifiers are equal.

      # When A is larger than B
      # 1.0.0-alpha.beta 1.0.0-alpha
      if [ -n "$(isSingleIdentifier "$b_pre" "$b")" ] && [ -z "$(isSingleIdentifier "$a_pre" "$a")" ]; then
        debug "Because of single identifier, A has more pre-identifiers"
        outcome "1"
        return
      fi

      # When A is shorter than B
      # 1.0.0-alpha 1.0.0-alpha.beta
      if [ -z "$(isSingleIdentifier "$b_pre" "$b")" ] && [ -n "$(isSingleIdentifier "$a_pre" "$a")" ]; then
        debug "Because of single identifier, B has more pre-identifiers"
        outcome "-1"
        return
      fi
    fi

    # Proceed to the next identifier because previous comparition was equal.
    cursor=$((cursor + 1))
  done
}


PLATFORM=${PLATFORM:-linux}
if [ "$(uname)" == "Darwin" ]; then
  PLATFORM=darwin
fi

ARCH=amd64
case $(uname -m) in
    x86_64) ARCH="amd64" ;;
    arm)    ARCH="arm64" ;;
    arm64)  ARCH="arm64" ;;
esac
BINDIR=~/.local/bin

mkdir -p $BINDIR

# Enhanced binary validation function
validate_sc_binary() {
  local binary_path="$1"
  
  # Check if file exists and is executable
  if [[ ! -f "$binary_path" ]]; then
    return 1
  fi
  
  if [[ ! -x "$binary_path" ]]; then
    return 1
  fi
  
  # Check if binary is complete (not corrupted)
  set +e
  "$binary_path" --version >/dev/null 2>&1
  local exit_code=$?
  set -e
  
  return $exit_code
}

# Progress indicator for downloads
show_progress() {
  local pid=$1
  local delay=0.1
  local spinstr='|/-\'
  local temp_file="/tmp/.sc_download_progress_$$"
  
  echo -n "Downloading Simple Container binary "
  while kill -0 $pid 2>/dev/null; do
    local temp=${spinstr#?}
    printf "[%c]" "$spinstr"
    local spinstr=$temp${spinstr%"$temp"}
    sleep $delay
    printf "\b\b\b"
  done
  printf "   \b\b\b"
}

# Phase 2c — verify sc.sh tarball signatures before extraction.
#
# Every published tarball at dist.simple-container.com ships a sibling
# `.cosign-bundle` (self-contained Sigstore bundle: cert + sig + Rekor
# entry). When `cosign` is on PATH, we download both, verify the bundle
# against the production OIDC identity, and only extract on success. When
# `cosign` is missing, we warn loudly and continue — installers on a
# fresh laptop without cosign should not be hard-blocked, per the
# graceful-fallback contract documented in docs/SECURITY.md.
#
# This closes Phase 2c. The identity regex MUST stay in sync with the
# "Verifying tarballs" block in docs/SECURITY.md.
verify_sc_tarball() {
  local tarball_path="$1"
  local bundle_url="$2"
  local temp_dir
  temp_dir=$(dirname "$tarball_path")
  local bundle_path="$tarball_path.cosign-bundle"

  if ! command -v cosign >/dev/null 2>&1; then
    echo "⚠️  cosign not found on PATH — skipping signature verification."
    echo "    For end-to-end supply-chain integrity install cosign:"
    echo "    https://docs.sigstore.dev/system_config/installation/"
    return 0
  fi

  echo -n "🔏 Fetching signature bundle... "
  if ! curl -fsSL "$bundle_url" -o "$bundle_path"; then
    echo "❌"
    echo "❌ Failed to fetch .cosign-bundle from $bundle_url"
    echo "    The published tarball at dist.simple-container.com is expected"
    echo "    to ship a sibling .cosign-bundle. Refusing to extract an"
    echo "    artifact whose signature can't be retrieved."
    return 1
  fi
  echo "✅"

  echo -n "🔍 Verifying tarball signature against build-workflow identity... "
  # Default: STRICT. Identity regex matches the production push.yaml on
  # refs/heads/main — the only workflow allowed to publish tarballs that
  # this code path accepts without further opt-in. Mirror in docs/SECURITY.md.
  #
  # Preview opt-in (SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH=<branch>) narrows
  # the trust extension to ONE named branch's branch-preview.yaml signature.
  # We deliberately do NOT support an "any branch" opt-in (e.g. =1), because
  # accepting `branch-preview.yaml@refs/heads/.+` would trust every push-
  # writer on any branch in the repo — a much broader radius than picking
  # up an unreviewed feature branch you actually want to test.
  #
  # Why this still requires explicit user action:
  #   - branch-preview.yaml runs on workflow_dispatch from feature branches
  #     that lack main's branch protection / required reviews / signed
  #     commits. The cosign certificate proves "this run dispatched from
  #     this ref" but cannot attest to the integrity of the workflow's
  #     contents at that ref (no SHA pinning at the identity layer).
  #   - For higher assurance, also set SIMPLE_CONTAINER_TRUST_PREVIEW_SHA
  #     to a 40-char commit SHA. We pass --certificate-github-workflow-sha
  #     to cosign so the Sigstore cert's workflow_sha claim must match
  #     EXACTLY — pinning to a specific commit, not a mutable branch head.
  #     This neutralizes "attacker pushes new commit to the branch then
  #     re-dispatches" and "CDN replays an old tarball signed from the
  #     same branch."
  #
  # IMPORTANT: do NOT pass --yes here. cosign 2.x only accepts --yes on
  # sign-blob (skip interactive confirmation); on verify-blob it errors
  # out with "unknown flag: --yes" — which is what broke every install
  # after Phase 2c shipped. Capture cosign's stderr (don't /dev/null it)
  # so future failures surface the real error instead of a generic
  # message.

  # Refuse the deprecated/never-shipped "=1" form loudly, with a hint at
  # the supported form. This forces the user to commit to a specific
  # branch instead of broadening trust to the entire repo's push-writers.
  if [ "${SIMPLE_CONTAINER_ALLOW_PREVIEW:-}" = "1" ]; then
    echo "❌"
    echo "❌ SIMPLE_CONTAINER_ALLOW_PREVIEW=1 is not supported (security: trusts every branch in the repo)."
    echo "    Use SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH=<branch-name> instead — it pins"
    echo "    cosign verification to one branch's branch-preview.yaml signature."
    echo "    Optionally also set SIMPLE_CONTAINER_TRUST_PREVIEW_SHA=<40-char-commit-sha>"
    echo "    to pin the exact commit of that branch (recommended for CI)."
    echo "    See https://github.com/simple-container-com/api/blob/main/docs/SECURITY.md#installing-preview--branch-preview-builds"
    return 1
  fi

  local identity_regex='^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$'
  local preview_branch="${SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH:-}"
  local preview_sha="${SIMPLE_CONTAINER_TRUST_PREVIEW_SHA:-}"
  local cosign_extra_args=()
  if [ -n "$preview_branch" ]; then
    # Validate against a conservative allowlist BEFORE interpolating into the
    # regex. Git's check-ref-format is more permissive than what we want here
    # (it allows `+`, `(`, `)`, `{`, `}`, `|`, `$` — all regex metachars).
    # Constraining to alphanumerics + `._/-` keeps the regex string literal-
    # equivalent so we don't need a separate escape pass, and matches the
    # naming conventions every Integrail/SC branch already uses (feat/fix/
    # chore/docs prefixes with kebab-case bodies).
    if ! printf '%s' "$preview_branch" | grep -qE '^[A-Za-z0-9._/-]+$'; then
      echo "❌"
      echo "❌ Invalid SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH value: $preview_branch"
      echo "    Allowed characters: letters, digits, dot, underscore, slash, hyphen."
      echo "    Refusing to interpolate into the cosign identity regex."
      return 1
    fi
    # Additional git-style rejections (parts of check-ref-format that our
    # allowlist already covers but worth being explicit about):
    case "$preview_branch" in
      ..*|*..*|*/..*|*..)
        echo "❌"; echo "❌ Branch name must not contain '..' segments."; return 1 ;;
      /*|*/)
        echo "❌"; echo "❌ Branch name must not start or end with '/'."; return 1 ;;
      *.lock|*.lock/*|*/*.lock)
        echo "❌"; echo "❌ Branch name segments must not end with '.lock'."; return 1 ;;
    esac

    # Optional SHA pin — must be 40 lowercase hex chars (canonical git SHA-1).
    if [ -n "$preview_sha" ]; then
      if ! printf '%s' "$preview_sha" | grep -qE '^[a-f0-9]{40}$'; then
        echo "❌"
        echo "❌ Invalid SIMPLE_CONTAINER_TRUST_PREVIEW_SHA value: $preview_sha"
        echo "    Must be 40 lowercase hex characters (a full git commit SHA-1)."
        return 1
      fi
      cosign_extra_args+=(--certificate-github-workflow-sha "$preview_sha")
    fi

    # Loud warning to stderr so a forgotten `export` in shell rc is visible
    # on every install, not just the first. T3 mitigation per review.
    echo "" >&2
    echo "⚠️  PREVIEW SIGNATURE TRUST EXTENDED" >&2
    echo "    SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH is set — accepting tarballs signed by" >&2
    echo "      branch-preview.yaml on refs/heads/$preview_branch" >&2
    if [ -n "$preview_sha" ]; then
      echo "    pinned to commit SHA $preview_sha" >&2
    else
      echo "    (no SHA pin — branch HEAD trusted; set SIMPLE_CONTAINER_TRUST_PREVIEW_SHA=<sha> to pin)" >&2
    fi
    echo "    Production-strict mode disabled. Unset the env var to restore strict mode." >&2
    echo "" >&2

    # Build the widened regex with the validated branch name. The branch
    # name has already been allowlist-restricted; the only metachar that
    # could appear is `.`, which we escape here to avoid e.g. `feat.main`
    # matching a regex intended for `feat/main` (gemini's "identity
    # shadowing" point). `/` and `-` are regex-safe.
    local escaped_branch
    escaped_branch=$(printf '%s' "$preview_branch" | sed 's/\./\\./g')
    identity_regex="^https://github\\.com/simple-container-com/api/\\.github/workflows/(push\\.yaml@refs/heads/main|branch-preview\\.yaml@refs/heads/${escaped_branch})\$"
  fi

  local cosign_err
  if ! cosign_err=$(COSIGN_EXPERIMENTAL=1 cosign verify-blob \
      --bundle "$bundle_path" \
      --certificate-identity-regexp "$identity_regex" \
      --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
      "${cosign_extra_args[@]}" \
      "$tarball_path" 2>&1); then
    echo "❌"
    echo "❌ Signature verification FAILED for $tarball_path"
    echo "    cosign output:"
    echo "$cosign_err" | sed 's/^/      /'
    # Detect the preview-signed-but-strict-mode case and give the user a
    # specific hint instead of the generic "compromised CDN" copy. The
    # cosign error includes the actual signer identity in `got subjects [...]`.
    if echo "$cosign_err" | grep -q 'branch-preview\.yaml@refs/heads/'; then
      # Try to surface the branch the tarball was actually signed from so
      # the user can copy-paste it as the env var value. The cosign error
      # text format is "got subjects [URL]".
      local actual_branch
      actual_branch=$(echo "$cosign_err" | grep -oE 'branch-preview\.yaml@refs/heads/[^]]+' | head -1 | sed 's|branch-preview\.yaml@refs/heads/||')
      echo "    The tarball was signed by branch-preview.yaml (a feature-branch"
      echo "    build), not by the production push.yaml@main workflow. If you"
      echo "    trust this preview, set:"
      echo ""
      if [ -n "$actual_branch" ]; then
        echo "      export SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH=$actual_branch"
      else
        echo "      export SIMPLE_CONTAINER_TRUST_PREVIEW_BRANCH=<branch-name>"
      fi
      echo ""
      echo "    Optionally also pin the exact commit:"
      echo "      export SIMPLE_CONTAINER_TRUST_PREVIEW_SHA=<40-char-sha>"
      echo ""
      echo "    See https://github.com/simple-container-com/api/blob/main/docs/SECURITY.md#installing-preview--branch-preview-builds"
    else
      echo "    The tarball does not bear a valid signature from the SC"
      echo "    production publish workflow. This could mean: tarball was"
      echo "    tampered in transit, CDN was compromised, or the signing"
      echo "    identity rotated — see https://github.com/simple-container-com/api"
    fi
    echo "    Refusing to extract."
    return 1
  fi
  echo "✅"
}

# Safe download with validation
safe_download_sc() {
  local url="$1"
  local temp_dir
  temp_dir=$(mktemp -d)
  local temp_binary="$temp_dir/sc"
  local temp_tarball="$temp_dir/$(basename "$url")"

  echo "🚀 Installing Simple Container..."
  echo "📦 Downloading from: $url"

  # Download tarball to a file (rather than streaming through tar) so we
  # can run cosign verify-blob against the bytes BEFORE extracting any
  # executable code. Streaming-then-verifying is a TOCTOU footgun.
  (
    cd "$temp_dir"
    curl -fL --progress-bar "$url" -o "$temp_tarball"
  ) &

  local download_pid=$!
  show_progress $download_pid

  # Wait for download to complete and check exit status
  wait $download_pid
  local download_status=$?

  if [[ $download_status -ne 0 ]]; then
    echo ""
    echo "❌ Failed to download sc from $url"
    rm -rf "$temp_dir"
    return 1
  fi

  echo " ✅"

  # Phase 2c — verify before extract. Refuses to extract on hard
  # signature failure (cosign present + bundle present + verify fails
  # or bundle missing). Graceful pass-through when cosign is not on
  # PATH (warns instead).
  if ! verify_sc_tarball "$temp_tarball" "$url.cosign-bundle"; then
    rm -rf "$temp_dir"
    return 1
  fi

  # Now extract — bytes are trusted (verified) or explicitly opted into
  # by the user (cosign absent + warning shown).
  if ! tar -xzpf "$temp_tarball" -C "$temp_dir" sc; then
    echo "❌ Failed to extract sc binary from tarball"
    rm -rf "$temp_dir"
    return 1
  fi
  
  # Validate the downloaded binary
  echo -n "🔍 Validating binary... "
  if ! validate_sc_binary "$temp_binary"; then
    echo "❌"
    echo "❌ Downloaded binary is corrupted or invalid"
    rm -rf "$temp_dir"
    return 1
  fi
  echo "✅"
  
  # Backup existing binary if it exists and is valid
  if [[ -f "$BINDIR/sc" ]] && validate_sc_binary "$BINDIR/sc"; then
    echo "📦 Backing up existing binary..."
    cp "$BINDIR/sc" "$BINDIR/sc.backup.$(date +%s)"
  fi
  
  # Atomically replace the binary
  echo -n "📦 Installing binary... "
  chmod +x "$temp_binary"
  mv "$temp_binary" "$BINDIR/sc"
  echo "✅"
  
  # Clean up
  rm -rf "$temp_dir"
  
  # Final validation
  echo -n "🧪 Testing installation... "
  if validate_sc_binary "$BINDIR/sc"; then
    local installed_version
    installed_version=$("$BINDIR/sc" --version 2>/dev/null || echo "unknown")
    echo "✅"
    echo "🎉 Simple Container $installed_version installed successfully!"
    return 0
  else
    echo "❌"
    echo "❌ Installation failed - binary validation failed"
    
    # Attempt to restore backup if available
    local latest_backup
    latest_backup=$(ls -t "$BINDIR"/sc.backup.* 2>/dev/null | head -n1)
    if [[ -n "$latest_backup" ]] && validate_sc_binary "$latest_backup"; then
      echo "🔄 Restoring previous working version..."
      cp "$latest_backup" "$BINDIR/sc"
      echo "✅ Previous version restored"
    fi
    return 1
  fi
}

CURRENT="0.0.0"
if [[ -f "$BINDIR/sc" ]]; then
  if validate_sc_binary "$BINDIR/sc"; then
    CURRENT="$($BINDIR/sc --version 2>/dev/null || echo "0.0.0")"
  else
    echo "⚠️  Existing sc binary is corrupted or invalid"
    CURRENT="null"
  fi
fi

FORCE_UPDATE="false"
if [[ -z "${SIMPLE_CONTAINER_VERSION:-}" ]]; then
URL="https://dist.simple-container.com/sc-${PLATFORM}-${ARCH}.tar.gz"
else
URL="https://dist.simple-container.com/sc-${PLATFORM}-${ARCH}-v${SIMPLE_CONTAINER_VERSION}.tar.gz"
VERSION="${SIMPLE_CONTAINER_VERSION:-}"
FORCE_UPDATE="true"
fi

VERSION_COMPARE="1"
if [[ "$CURRENT" != "null" ]]; then
  VERSION_COMPARE="$(semver_compare "$VERSION" "$CURRENT" || echo "1")"
fi

# Enhanced installation logic with better UX
if [[ ! -f "$BINDIR/sc" || $VERSION_COMPARE == "1" || ( "${FORCE_UPDATE}" == "true" && "$VERSION_COMPARE" != "0" ) ]]; then
  if ! safe_download_sc "$URL"; then
    echo "❌ Failed to install Simple Container"
    echo "💡 You can try:"
    echo "   - Check your internet connection"
    echo "   - Run the script again"
    echo "   - Set SIMPLE_CONTAINER_VERSION to a specific version"
    echo "   - Visit https://github.com/simple-container/simple-container for manual installation"
    exit 1
  fi
elif [[ -f "$BINDIR/sc" ]]; then
  # Binary exists and is up to date
  current_version=$("$BINDIR/sc" --version 2>/dev/null || echo "unknown")
  echo "✅ Simple Container $current_version is already installed and up to date"
  echo "💡 No download needed - using existing installation"
fi

# Install Pulumi if not present.
#
# Pinned version + SHA256 verify before exec (replaces the legacy
# curl-pipe-to-shell bootstrap pattern Pulumi's official installer
# uses). Closes Scorecard Pinned-Dependencies `downloadThenRun` warning
# on this line. Override version via `SC_PULUMI_VERSION` env var.
#
# Trust model: SHA256 sums come from Pulumi's checksums file at the
# same GitHub release URL as the tarball. This defends against
# tarball-in-flight tampering (CDN MITM) but NOT against a compromise
# of the release surface itself (where attacker swaps both files). For
# stronger trust, run cosign verify against checksums.txt.sig before
# parsing it; not done here to keep the bootstrap installer minimal.
SC_PULUMI_VERSION="${SC_PULUMI_VERSION:-3.239.0}"

install_pulumi_pinned() {
  local platform="$1" arch="$2"
  local tarball="pulumi-v${SC_PULUMI_VERSION}-${platform}-${arch}.tar.gz"
  local checksums="pulumi-${SC_PULUMI_VERSION}-checksums.txt"
  local base="https://github.com/pulumi/pulumi/releases/download/v${SC_PULUMI_VERSION}"
  local tmp
  tmp=$(mktemp -d)

  echo -n "📦 Downloading Pulumi v${SC_PULUMI_VERSION}... "
  if ! curl -fsSL "${base}/${tarball}" -o "${tmp}/${tarball}"; then
    echo "❌"
    rm -rf "$tmp"
    return 1
  fi
  echo "✅"

  echo -n "📦 Downloading checksums... "
  if ! curl -fsSL "${base}/${checksums}" -o "${tmp}/${checksums}"; then
    echo "❌"
    rm -rf "$tmp"
    return 1
  fi
  echo "✅"

  echo -n "🔍 Verifying SHA256... "
  # Extract the expected SHA for this specific tarball from the
  # checksums file (one line per artifact, format: `<sha>  <name>`).
  local expected
  expected=$(awk -v t="${tarball}" '$2 == t {print $1}' "${tmp}/${checksums}")
  if [[ -z "$expected" ]]; then
    echo "❌"
    echo "❌ Tarball ${tarball} not listed in checksums file. Refusing to install."
    rm -rf "$tmp"
    return 1
  fi
  if ! echo "${expected}  ${tmp}/${tarball}" | sha256sum -c >/dev/null 2>&1; then
    echo "❌"
    echo "❌ SHA256 mismatch on ${tarball}. Refusing to install."
    rm -rf "$tmp"
    return 1
  fi
  echo "✅"

  echo -n "📦 Installing Pulumi to ~/.pulumi... "
  mkdir -p "$HOME/.pulumi"
  if ! tar -xzf "${tmp}/${tarball}" -C "$HOME/.pulumi" --strip-components=1 2>/dev/null; then
    echo "❌"
    rm -rf "$tmp"
    return 1
  fi
  echo "✅"

  # Pulumi's tarball expands to a `pulumi/` dir at top level (containing
  # the `pulumi` binary + helpers). `--strip-components=1` flattens that
  # so binaries land directly under `~/.pulumi/`. Verify the binary
  # actually landed - a layout change in a future Pulumi release could
  # extract successfully into a different path without our knowing.
  if ! [[ -x "$HOME/.pulumi/pulumi" ]]; then
    echo "❌ Tarball extracted but $HOME/.pulumi/pulumi is missing or not executable"
    echo "    (likely an unexpected change to Pulumi's archive layout for v${SC_PULUMI_VERSION})"
    rm -rf "$tmp"
    return 1
  fi

  # Push it onto PATH for this script's remaining commands (notably the
  # final `exec sc` at end of file) so sc can shell out to pulumi
  # without the user restarting their terminal. Also hint at the
  # permanent fix.
  if [[ ":$PATH:" != *":$HOME/.pulumi:"* ]]; then
    export PATH="$HOME/.pulumi:$PATH"
    echo "💡 Pulumi added to PATH for this session. For persistence add to your shell rc:"
    echo "    export PATH=\"\$HOME/.pulumi:\$PATH\""
  fi

  rm -rf "$tmp"
}

if ! [ -x "$(command -v pulumi)" ]; then
  echo "🔧 Pulumi not found, installing..."
  if [[ "$PLATFORM" == "linux" ]]; then
    echo "📦 Installing Pulumi for Linux..."
    case "$(uname -m)" in
      x86_64|amd64) PULUMI_ARCH="x64" ;;
      aarch64|arm64) PULUMI_ARCH="arm64" ;;
      *)
        echo "⚠️  Unsupported architecture $(uname -m); skipping Pulumi install. Install manually: https://www.pulumi.com/docs/install/"
        PULUMI_ARCH=""
        ;;
    esac
    if [[ -n "$PULUMI_ARCH" ]]; then
      if install_pulumi_pinned linux "$PULUMI_ARCH"; then
        echo "✅ Pulumi v${SC_PULUMI_VERSION} installed successfully"
      else
        echo "⚠️  Pulumi installation failed, but Simple Container may still work for some operations"
      fi
    fi
  elif [[ "$PLATFORM" == "darwin" ]]; then
    echo "📦 Installing Pulumi via Homebrew..."
    if command -v brew >/dev/null 2>&1; then
      if brew install pulumi/tap/pulumi; then
        echo "✅ Pulumi installed successfully"
      else
        echo "⚠️  Pulumi installation failed, but Simple Container may still work for some operations"
      fi
    else
      echo "⚠️  Homebrew not found. Please install Pulumi manually: https://www.pulumi.com/docs/get-started/install/"
    fi
  fi
else
  pulumi_version=$(pulumi version 2>/dev/null | head -n1 || echo "unknown")
  echo "✅ Pulumi $pulumi_version is already installed"
fi

# Cleanup old backup files (keep only last 3)
cleanup_old_backups() {
  local backup_files
  backup_files=$(ls -t "$BINDIR"/sc.backup.* 2>/dev/null | tail -n +4)
  if [[ -n "$backup_files" ]]; then
    echo "$backup_files" | xargs rm -f
  fi
}

# Clean up old backups silently
cleanup_old_backups 2>/dev/null || true

export PATH="$PATH:$BINDIR"

# Add completions
# bash
path_export="export PATH=\"\$PATH:$BINDIR\""
if [[ -f "$HOME/.bashrc" ]]; then
  if [[ "$(cat $HOME/.bashrc | grep "$path_export")" == "" ]]; then
    echo "$path_export" >> "$HOME/.bashrc"
  fi
  completion_bash="source <(sc completion bash)"
  if [[ "$(cat $HOME/.bashrc | grep "$completion_bash")" == "" ]]; then
    echo "$completion_bash" >> "$HOME/.bashrc"
  fi
  unalias_cmd="unalias sc > /dev/null 2>/dev/null || true" # in case sc is defined as global alias
  if [[ "$(cat $HOME/.bashrc | grep "$unalias_cmd")" == "" ]]; then
    echo "$unalias_cmd" >> "$HOME/.bashrc"
  fi
fi

# zsh
if [[ "${PLATFORM}" == "darwin" && ! -f "$HOME/.zshrc" ]]; then
  touch "$HOME/.zshrc"
fi

if [[ -f "$HOME/.zshrc" ]]; then
  if [[ "$(cat "$HOME/.zshrc" | grep "$path_export")" == "" ]]; then
    # shellcheck disable=SC2129
    echo "$path_export" >> "$HOME/.zshrc"
    echo "unalias sc >/dev/null 2>/dev/null || true" >> "$HOME/.zshrc"
    echo "autoload -U compinit; compinit" >> "$HOME/.zshrc"
    if [[ "$PLATFORM" == "darwin" ]]; then
      $BINDIR/sc completion zsh > $(brew --prefix)/share/zsh/site-functions/_sc || echo ""
    fi
  fi
  completion_zsh="source <(sc completion zsh)"
  if [[ "$(cat $HOME/.zshrc | grep "$completion_zsh")" == "" ]]; then
    echo "$completion_zsh" >> "$HOME/.zshrc"
  fi
  if [[ "$(cat $HOME/.zshrc | grep "$unalias_cmd")" == "" ]]; then
    echo "$unalias_cmd" >> "$HOME/.zshrc"
  fi
fi

# Final validation before executing sc
if ! validate_sc_binary "$BINDIR/sc"; then
  echo "❌ Simple Container binary is not working properly"
  echo "💡 Try running the script again or check the installation"
  exit 1
fi

# Execute sc with all passed arguments
exec "$BINDIR/sc" "$@"
