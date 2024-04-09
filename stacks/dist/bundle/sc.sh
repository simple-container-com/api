#!/usr/bin/env bash

set -e;

PLATFORM=linux
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

if [[ ! -f "$BINDIR/sc" ]]; then
  (
    cd $BINDIR &&
    curl -fL "https://dist.simple-container.com/sc-${PLATFORM}-${ARCH}.tar.gz" | tar -xzp sc  &&
    chmod +x sc &&
    cd -
  )
fi

export PATH="$PATH:$BINDIR"

# Add completions
# bash
path_export="export PATH=\"\$PATH:$BINDIR\""
if [[ "$(cat ~/.bashrc | grep "$path_export")" == "" ]]; then
  echo "$path_export" >> ~/.bashrc
fi
completion_bash="source <(sc completion bash)"
if [[ "$(cat ~/.bashrc | grep "$completion_bash")" == "" ]]; then
  echo "$completion_bash" >> ~/.bashrc
fi
unalias_cmd="unalias sc > /dev/null 2>/dev/null || true" # in case sc is defined as global alias
if [[ "$(cat ~/.bashrc | grep "$unalias_cmd")" == "" ]]; then
  echo "$unalias_cmd" >> ~/.bashrc
fi

# zsh
if [[ -f "$HOME/.zshrc" && "$(cat ~/.zshrc | grep "$path_export")" == "" ]]; then
  # shellcheck disable=SC2129
  echo "$path_export" >> ~/.zshrc
  echo "unalias sc || echo ''" >> ~/.zshrc
  echo "autoload -U compinit; compinit" >> ~/.zshrc
  if [[ "$PLATFORM" == "darwin" ]]; then
    $BINDIR/sc completion zsh > $(brew --prefix)/share/zsh/site-functions/_sc || echo ""
  fi
fi
completion_zsh="source <(sc completion zsh)"
if [[ -f "$HOME/.zshrc" && "$(cat ~/.zshrc | grep "$completion_zsh")" == "" ]]; then
  echo "$completion_zsh" >> ~/.zshrc
fi
if [[ "$(cat ~/.zshrc | grep "$unalias_cmd")" == "" ]]; then
  echo "$unalias_cmd" >> ~/.zshrc
fi


$BINDIR/sc $@
