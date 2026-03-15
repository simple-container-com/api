#!/bin/bash

# Setup Claude Code script
# Installs and verifies Claude Code installation

set -e

echo "Setting up Claude Code"

# Pin Claude Code version to avoid breaking changes from upstream updates.
# v2.1.41 introduced a breaking change in MCP server error handling that causes
# JSON parse errors ("Unrecognized token '/'") when MCP servers fail in CI.
# Last known working version: 2.1.39
CLAUDE_VERSION="${CLAUDE_VERSION:-2.1.39}"
echo "📌 Target Claude Code version: $CLAUDE_VERSION"

# Install Claude Code via npm (allows version pinning)
if command -v npm &> /dev/null; then
  echo "Installing Claude Code v$CLAUDE_VERSION via npm..."
  npm install -g "@anthropic-ai/claude-code@$CLAUDE_VERSION" || {
    echo "⚠️ npm install failed, falling back to official installer (latest version)..."
    curl -fsSL https://claude.ai/install.sh | bash
  }
else
  echo "⚠️ npm not available, using official installer (latest version)..."
  curl -fsSL https://claude.ai/install.sh | bash
fi

# Add Claude to PATH for current session
export PATH="$HOME/.local/bin:$PATH"

# Verify installation
if command -v claude &> /dev/null; then
  echo "✅ Claude Code installed successfully"
  claude --version
else
  echo "❌ Claude Code installation failed"
  echo "Checking if claude is in ~/.local/bin..."
  ls -la ~/.local/bin/ | grep claude || echo "Claude binary not found"
  exit 1
fi

# Configure Claude Code timeout settings via ~/.claude/settings.json
# When using an AI gateway proxy, requests can take longer than the default 2-minute
# BashTool timeout. Increase per-command and MCP timeouts to avoid pre-flight check failures.
echo "⏱️ Configuring Claude Code timeout settings..."
mkdir -p ~/.claude
if [ -f ~/.claude/settings.json ]; then
  echo "  Existing settings.json found, merging timeout env vars..."
  # Use jq to merge if available, otherwise overwrite
  if command -v jq &> /dev/null; then
    jq '.env = (.env // {}) + {
      "BASH_DEFAULT_TIMEOUT_MS": "300000",
      "BASH_MAX_TIMEOUT_MS": "1800000",
      "MCP_TIMEOUT": "30000",
      "MCP_TOOL_TIMEOUT": "300000"
    }' ~/.claude/settings.json > ~/.claude/settings.json.tmp && mv ~/.claude/settings.json.tmp ~/.claude/settings.json
  else
    # No jq — overwrite with timeout config
    cat > ~/.claude/settings.json << 'SETTINGS_EOF'
{
  "env": {
    "BASH_DEFAULT_TIMEOUT_MS": "300000",
    "BASH_MAX_TIMEOUT_MS": "1800000",
    "MCP_TIMEOUT": "30000",
    "MCP_TOOL_TIMEOUT": "300000"
  }
}
SETTINGS_EOF
  fi
else
  cat > ~/.claude/settings.json << 'SETTINGS_EOF'
{
  "env": {
    "BASH_DEFAULT_TIMEOUT_MS": "300000",
    "BASH_MAX_TIMEOUT_MS": "1800000",
    "MCP_TIMEOUT": "30000",
    "MCP_TOOL_TIMEOUT": "300000"
  }
}
SETTINGS_EOF
fi
echo "✅ Claude Code timeout settings configured:"
echo "  - BASH_DEFAULT_TIMEOUT_MS: 300000 (5 minutes)"
echo "  - BASH_MAX_TIMEOUT_MS: 1800000 (30 minutes)"
echo "  - MCP_TIMEOUT: 30000 (30 seconds)"
echo "  - MCP_TOOL_TIMEOUT: 300000 (5 minutes)"
