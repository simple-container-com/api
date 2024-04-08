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
    export PATH="$PATH:$BINDIR" &&
    cd -

    path_export="export PATH=\"\$PATH:$BINDIR\""
    if [[ "$(cat ~/.bashrc | grep "$path_export")" == "" ]]; then
      echo "$path_export" >> ~/.bashrc
      echo "unalias sc || echo ''" >> ~/.bashrc
      $BINDIR/sc completion zsh > "${fpath[1]}/_sc"
    fi
    if [[ -f "$HOME/.zshrc" && "$(cat ~/.zshrc | grep "$path_export")" == "" ]]; then
      # shellcheck disable=SC2129
      echo "$path_export" >> ~/.zshrc
      echo "unalias sc || echo ''" >> ~/.zshrc
      echo "autoload -U compinit; compinit" >> ~/.zshrc
      if [[ "$PLATFORM" == "darwin" ]]; then
        $BINDIR/sc completion zsh > $(brew --prefix)/share/zsh/site-functions/_sc
      fi
    fi
  )
fi

$BINDIR/sc $@
