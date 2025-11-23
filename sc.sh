#!/usr/bin/env bash

set -e;

VERSION="0.0.0"

# Pulumi version that matches Simple Container's go.mod requirements
# NOTE: This version is automatically updated during the release process
# The welder.yaml build extracts the version from go.mod and updates this variable
# Manual updates are not needed - the release process handles synchronization
PULUMI_VERSION="3.184.0"

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
elif [[ "$(uname -s)" == CYGWIN* ]] || [[ "$(uname -s)" == MINGW* ]] || [[ "$(uname -s)" == MSYS* ]] || [[ -n "${WINDIR:-}" ]]; then
  PLATFORM=windows
fi

ARCH=amd64
case $(uname -m) in
    x86_64) ARCH="amd64" ;;
    arm)    ARCH="arm64" ;;
    arm64)  ARCH="arm64" ;;
esac
# Set binary directory and executable name based on platform
if [[ "$PLATFORM" == "windows" ]]; then
  BINDIR="$HOME/.local/bin"
  SC_BINARY="sc.exe"
else
  BINDIR=~/.local/bin
  SC_BINARY="sc"
fi

mkdir -p "$BINDIR"

# Windows Pulumi direct installation function
install_pulumi_windows_direct() {
  echo "📦 Installing Pulumi v${PULUMI_VERSION} via direct download for Windows..."
  local pulumi_dir="$HOME/.pulumi"
  local pulumi_bin="$pulumi_dir/bin"
  
  # Create Pulumi directory
  mkdir -p "$pulumi_bin"
  
  # Download and extract Pulumi using the specific version
  local temp_dir
  temp_dir=$(mktemp -d)
  local pulumi_url="https://get.pulumi.com/releases/sdk/pulumi-v${PULUMI_VERSION}-windows-x64.tar.gz"
  
  echo "📥 Downloading Pulumi v${PULUMI_VERSION} from $pulumi_url..."
  if curl -fL "$pulumi_url" | tar -xz -C "$temp_dir"; then
    # Move binaries to Pulumi directory
    if [[ -d "$temp_dir/pulumi" ]]; then
      cp -r "$temp_dir/pulumi"/* "$pulumi_bin/"
      chmod +x "$pulumi_bin"/*
      
      # Add to PATH if not already there
      if [[ ":$PATH:" != *":$pulumi_bin:"* ]]; then
        export PATH="$PATH:$pulumi_bin"
      fi
      
      echo "✅ Pulumi v${PULUMI_VERSION} installed successfully to $pulumi_bin"
    else
      echo "⚠️  Pulumi extraction failed - directory structure unexpected"
    fi
  else
    echo "⚠️  Pulumi v${PULUMI_VERSION} download failed"
  fi
  
  # Clean up
  rm -rf "$temp_dir"
}

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

# Safe download with validation
safe_download_sc() {
  local url="$1"
  local temp_dir
  temp_dir=$(mktemp -d)
  local temp_binary="$temp_dir/$SC_BINARY"
  
  echo "🚀 Installing Simple Container..."
  echo "📦 Downloading from: $url"
  
  # Download with progress indicator
  (
    cd "$temp_dir"
    if [[ "$PLATFORM" == "windows" ]]; then
      # Windows: extract and rename to .exe
      curl -fL --progress-bar "$url" | tar -xzp sc
      if [[ -f "sc" ]]; then
        mv "sc" "$SC_BINARY"
      fi
    else
      curl -fL --progress-bar "$url" | tar -xzp sc
    fi
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
  if [[ -f "$BINDIR/$SC_BINARY" ]] && validate_sc_binary "$BINDIR/$SC_BINARY"; then
    echo "📦 Backing up existing binary..."
    cp "$BINDIR/$SC_BINARY" "$BINDIR/$SC_BINARY.backup.$(date +%s)"
  fi
  
  # Atomically replace the binary
  echo -n "📦 Installing binary... "
  chmod +x "$temp_binary"
  mv "$temp_binary" "$BINDIR/$SC_BINARY"
  echo "✅"
  
  # Clean up
  rm -rf "$temp_dir"
  
  # Final validation
  echo -n "🧪 Testing installation... "
  if validate_sc_binary "$BINDIR/$SC_BINARY"; then
    local installed_version
    installed_version=$("$BINDIR/$SC_BINARY" --version 2>/dev/null || echo "unknown")
    echo "✅"
    echo "🎉 Simple Container $installed_version installed successfully!"
    return 0
  else
    echo "❌"
    echo "❌ Installation failed - binary validation failed"
    
    # Attempt to restore backup if available
    local latest_backup
    latest_backup=$(ls -t "$BINDIR"/$SC_BINARY.backup.* 2>/dev/null | head -n1)
    if [[ -n "$latest_backup" ]] && validate_sc_binary "$latest_backup"; then
      echo "🔄 Restoring previous working version..."
      cp "$latest_backup" "$BINDIR/$SC_BINARY"
      echo "✅ Previous version restored"
    fi
    return 1
  fi
}

CURRENT="0.0.0"
if [[ -f "$BINDIR/$SC_BINARY" ]]; then
  if validate_sc_binary "$BINDIR/$SC_BINARY"; then
    CURRENT="$("$BINDIR/$SC_BINARY" --version 2>/dev/null || echo "0.0.0")"
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
if [[ ! -f "$BINDIR/$SC_BINARY" || $VERSION_COMPARE == "1" || ( "${FORCE_UPDATE}" == "true" && "$VERSION_COMPARE" != "0" ) ]]; then
  if ! safe_download_sc "$URL"; then
    echo "❌ Failed to install Simple Container"
    echo "💡 You can try:"
    echo "   - Check your internet connection"
    echo "   - Run the script again"
    echo "   - Set SIMPLE_CONTAINER_VERSION to a specific version"
    echo "   - Visit https://github.com/simple-container/simple-container for manual installation"
    exit 1
  fi
elif [[ -f "$BINDIR/$SC_BINARY" ]]; then
  # Binary exists and is up to date
  current_version=$("$BINDIR/$SC_BINARY" --version 2>/dev/null || echo "unknown")
  echo "✅ Simple Container $current_version is already installed and up to date"
  echo "💡 No download needed - using existing installation"
fi

# Install Pulumi if not present
if ! [ -x "$(command -v pulumi)" ]; then
  echo "🔧 Pulumi not found, installing..."
  if [[ "$PLATFORM" == "linux" ]]; then
    echo "📦 Installing Pulumi v${PULUMI_VERSION} for Linux..."
    if curl -fsSL https://get.pulumi.com | sh -s -- --version ${PULUMI_VERSION}; then
      echo "✅ Pulumi v${PULUMI_VERSION} installed successfully"
    else
      echo "⚠️  Pulumi installation failed, but Simple Container may still work for some operations"
    fi
  elif [[ "$PLATFORM" == "darwin" ]]; then
    echo "📦 Installing Pulumi v${PULUMI_VERSION} for macOS..."
    # Use direct download for macOS to ensure specific version
    if curl -fsSL https://get.pulumi.com | sh -s -- --version ${PULUMI_VERSION}; then
      echo "✅ Pulumi v${PULUMI_VERSION} installed successfully"
    else
      echo "⚠️  Direct installation failed, trying Homebrew..."
      if command -v brew >/dev/null 2>&1; then
        if brew install pulumi/tap/pulumi; then
          echo "✅ Pulumi installed successfully via Homebrew"
          echo "⚠️  Note: Homebrew version may differ from required v${PULUMI_VERSION}"
        else
          echo "⚠️  Pulumi installation failed, but Simple Container may still work for some operations"
        fi
      else
        echo "⚠️  Homebrew not found. Please install Pulumi v${PULUMI_VERSION} manually: https://www.pulumi.com/docs/get-started/install/"
      fi
    fi
  elif [[ "$PLATFORM" == "windows" ]]; then
    echo "📦 Installing Pulumi v${PULUMI_VERSION} for Windows..."
    # Prefer direct download to ensure specific version
    echo "📦 Installing Pulumi via direct download..."
    install_pulumi_windows_direct
    
    # Fallback to package managers if direct download fails
    if ! [ -x "$(command -v pulumi)" ]; then
      if command -v choco >/dev/null 2>&1; then
        echo "📦 Direct download failed, trying Chocolatey..."
        if choco install pulumi; then
          echo "✅ Pulumi installed successfully via Chocolatey"
          echo "⚠️  Note: Chocolatey version may differ from required v${PULUMI_VERSION}"
        fi
      elif command -v winget >/dev/null 2>&1; then
        echo "📦 Direct download failed, trying winget..."
        if winget install pulumi; then
          echo "✅ Pulumi installed successfully via winget"
          echo "⚠️  Note: winget version may differ from required v${PULUMI_VERSION}"
        fi
      fi
    fi
  fi
else
  pulumi_version=$(pulumi version 2>/dev/null | head -n1 || echo "unknown")
  echo "✅ Pulumi $pulumi_version is already installed"
  
  # Check if installed version matches required version
  if [[ "$pulumi_version" != *"v${PULUMI_VERSION}"* ]]; then
    echo "⚠️  Warning: Installed Pulumi version ($pulumi_version) may not match Simple Container requirements (v${PULUMI_VERSION})"
    echo "💡 If you experience issues, consider updating Pulumi to v${PULUMI_VERSION}"
  fi
fi

# Cleanup old backup files (keep only last 3)
cleanup_old_backups() {
  local backup_files
  backup_files=$(ls -t "$BINDIR"/$SC_BINARY.backup.* 2>/dev/null | tail -n +4)
  if [[ -n "$backup_files" ]]; then
    echo "$backup_files" | xargs rm -f
  fi
}

# Clean up old backups silently
cleanup_old_backups 2>/dev/null || true

export PATH="$PATH:$BINDIR"

# Add completions and PATH export
# bash
path_export="export PATH=\"\$PATH:$BINDIR\""

# Windows-specific shell configuration
if [[ "$PLATFORM" == "windows" ]]; then
  # For Windows, try to configure common bash environments
  bash_configs=("$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile")
  
  for config_file in "${bash_configs[@]}"; do
    if [[ -f "$config_file" ]] || [[ "$config_file" == "$HOME/.bashrc" ]]; then
      # Create .bashrc if it doesn't exist (common in Git Bash)
      if [[ ! -f "$config_file" && "$config_file" == "$HOME/.bashrc" ]]; then
        touch "$config_file"
      fi
      
      if [[ -f "$config_file" ]]; then
        if [[ "$(cat "$config_file" | grep "$path_export")" == "" ]]; then
          echo "$path_export" >> "$config_file"
        fi
        completion_bash="source <($SC_BINARY completion bash)"
        if [[ "$(cat "$config_file" | grep "$completion_bash")" == "" ]]; then
          echo "$completion_bash" >> "$config_file"
        fi
        unalias_cmd="unalias sc > /dev/null 2>/dev/null || true"
        if [[ "$(cat "$config_file" | grep "$unalias_cmd")" == "" ]]; then
          echo "$unalias_cmd" >> "$config_file"
        fi
        break  # Only configure the first available config file
      fi
    fi
  done
else
  # Linux/macOS configuration
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
if ! validate_sc_binary "$BINDIR/$SC_BINARY"; then
  echo "❌ Simple Container binary is not working properly"
  echo "💡 Try running the script again or check the installation"
  exit 1
fi

# Execute sc with all passed arguments
exec "$BINDIR/$SC_BINARY" "$@"
