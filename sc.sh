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

CURRENT="0.0.0"
if [[ -f "$BINDIR/sc"  ]]; then
  set +e
  $BINDIR/sc --version 1>/dev/null 2>/dev/null
  failure="$?"
  set -e
  if [[ $failure != "0" ]]; then
    CURRENT="null"
  else
    CURRENT="$($BINDIR/sc --version)"
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

if [[ ! -f "$BINDIR/sc" || $VERSION_COMPARE == "1" || ( "${FORCE_UPDATE}" == "true" && "$VERSION_COMPARE" != "0" ) ]]; then
  (
    cd $BINDIR &&
    curl -s -fL "$URL" | tar -xzp sc || ( echo "Failed to install sc from $URL" && exit 1) &&
    chmod +x sc &&
    cd - >/dev/null
  )
fi

if ! [ -x "$(command -v pulumi)" ]; then
  if [[ "$PLATFORM" == "linux" ]]; then
    curl -fsSL https://get.pulumi.com | sh
  elif [[ "$PLATFORM" == "darwin" ]]; then
    brew install pulumi/tap/pulumi
  fi
fi

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

$BINDIR/sc $@
