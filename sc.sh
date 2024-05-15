#!/usr/bin/env bash

set -e;

VERSION="0.0.0"

function vercomp () {
   if [[ $1 == $2 ]]
   then
       return 0
   fi
   local IFS=.
   local i ver1=($1) ver2=($2)
   # fill empty fields in ver1 with zeros
   for ((i=${#ver1[@]}; i<${#ver2[@]}; i++))
   do
       ver1[i]=0
   done
   for ((i=0; i<${#ver1[@]}; i++))
   do
       if [[ -z ${ver2[i]} ]]
       then
           # fill empty fields in ver2 with zeros
           ver2[i]=0
       fi
       if ((10#${ver1[i]} > 10#${ver2[i]}))
       then
           return 1
       fi
       if ((10#${ver1[i]} < 10#${ver2[i]}))
       then
           return 2
       fi
   done
   return 0
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
  CURRENT="$($BINDIR/sc --version)"
fi

VERSION_COMPARE="$(vercomp "$CURRENT" "$VERSION" || echo "2")"
if [[ ! -f "$BINDIR/sc" || $VERSION_COMPARE == "2" ]]; then
  (
    cd $BINDIR &&
    curl -s -fL "https://dist.simple-container.com/sc-${PLATFORM}-${ARCH}.tar.gz" | tar -xzp sc  &&
    chmod +x sc &&
    cd -
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
if [[ -f "$HOME/.bashrc" ]]; then
  path_export="export PATH=\"\$PATH:$BINDIR\""
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
  if [[ "$(cat $HOME/.zshrc | grep "$path_export")" == "" ]]; then
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
