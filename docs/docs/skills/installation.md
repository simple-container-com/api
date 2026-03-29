---
title: Installation
description: How to install Simple Container CLI
platform: platform
product: simple-container
category: skills
subcategory: installation
date: '2026-03-29'
---

# Installation Skill

This skill guides you through installing the Simple Container (SC) CLI. Follow these steps to get SC running on your system.

## Prerequisites

Before installing SC, ensure you have:

- **Operating System**: Linux, macOS, or Windows with WSL2
- **Docker**: Installed and running (`docker --version`)
- **Git**: Installed (`git --version`)
- **curl**: For downloading the CLI
- **Shell**: bash or zsh

## Steps

### Step 1: Determine Your Platform

SC supports multiple platforms. Determine your platform:

```bash
# For macOS (Intel)
PLATFORM="darwin_amd64"

# For macOS (Apple Silicon)
PLATFORM="darwin_arm64"

# For Linux
PLATFORM="linux_amd64"

# For Linux ARM64
PLATFORM="linux_arm64"
```

### Step 2: Download SC CLI

Download the latest version of SC:

```bash
# Get latest version number
VERSION=$(curl -s https://api.github.com/repos/simple-container/com/releases/latest | grep -o '"tag_name": "[^"]*"' | cut -d'"' -f4)

# Download for your platform
curl -L "https://github.com/simple-container/com/releases/download/${VERSION}/sc_${PLATFORM}" -o /tmp/sc

# Make executable
chmod +x /tmp/sc
```

### Step 3: Install SC to PATH

Install SC to your system PATH:

```bash
# For local install (current user)
sudo mv /tmp/sc /usr/local/bin/sc

# Verify installation
sc version
```

### Step 4: Configure Shell Autocomplete

Enable tab completion for your shell:

```bash
# For bash
sc completion bash > /etc/bash_completion.d/sc

# For zsh
sc completion zsh > ~/.zsh/completions/_sc

# Reload shell
exec $SHELL
```

### Step 5: Verify Installation

Run the verification commands:

```bash
# Check version
sc version

# View help
sc --help

# Check available commands
sc --help
```

## Troubleshooting

### "sc: command not found"

The CLI is not in your PATH. Check:

```bash
# Verify sc is in PATH
which sc

# Check /usr/local/bin is in your PATH
echo $PATH
```

### Docker Not Running

Start Docker:

```bash
# For macOS
open -a Docker

# For Linux
sudo systemctl start docker

# Verify Docker is running
docker version
```

### Permission Denied

You need sudo to install to /usr/local/bin:

```bash
sudo mv /tmp/sc /usr/local/bin/sc
```

## Upgrade Instructions

To upgrade to the latest version:

```bash
# Download latest version
VERSION=$(curl -s https://api.github.com/repos/simple-container/com/releases/latest | grep -o '"tag_name": "[^"]*"' | cut -d'"' -f4)
curl -L "https://github.com/simple-container/com/releases/download/${VERSION}/sc_${PLATFORM}" -o /tmp/sc

# Replace existing installation
sudo mv /tmp/sc /usr/local/bin/sc

# Verify upgrade
sc version
```

## Next Steps

After installation, proceed to:

1. [DevOps Setup](devops-setup.md) - Set up infrastructure configuration
2. [Cloud Provider Setup](cloud-providers/aws.md) - Configure your cloud provider