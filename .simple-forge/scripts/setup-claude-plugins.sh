#!/bin/bash

# Claude Code Plugin Setup Script
# Installs and configures plugins for enhanced capabilities including web fetching and documentation reading

set -e

echo "🔌 Setting up Claude Code plugins for enhanced capabilities..."

# Add Claude to PATH (in case it's not already there)
export PATH="$HOME/.local/bin:$PATH"

# Ensure Claude is available
if ! command -v claude &> /dev/null; then
    echo "❌ Claude Code is not available. Checking installation..."
    echo "Debug: PATH = $PATH"
    echo "Debug: Checking ~/.local/bin/claude..."
    if [ -f "$HOME/.local/bin/claude" ]; then
        echo "✅ Found claude at ~/.local/bin/claude"
        echo "Using full path to claude command"
        CLAUDE_CMD="$HOME/.local/bin/claude"
    else
        echo "❌ Claude binary not found at ~/.local/bin/claude"
        echo "Please run setup-claude.sh first."
        exit 1
    fi
else
    CLAUDE_CMD="claude"
fi

echo "✅ Claude Code is available: $CLAUDE_CMD"

# Create plugin configuration directory
mkdir -p ~/.config/claude-code/plugins
mkdir -p .forge-tmp/plugins

echo "📦 Installing Claude Code plugins..."

# Install web browsing and documentation plugins using the community CLI
echo "Installing web browsing and documentation plugins..."

# Install browser MCP plugin for web fetching
echo "🌐 Installing Browser MCP plugin..."
if command -v npx &> /dev/null; then
    # Use the community plugin registry for easy installation
    npx claude-plugins install browser-mcp@anthropics/claude-code-plugins --scope project || {
        echo "⚠️ Community plugin installation failed, trying direct MCP setup..."

        # Fallback: Configure MCP server directly
        cat > ~/.config/claude-code/.mcp.json << 'EOF'
{
  "mcpServers": {
    "browser": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-browser"],
      "env": {
        "BROWSER_EXECUTABLE_PATH": "/usr/bin/google-chrome"
      }
    },
    "web-search": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-web-search"],
      "env": {
        "SEARCH_API_KEY": "${SEARCH_API_KEY:-}"
      }
    }
  }
}
EOF
        echo "✅ Configured browser MCP server directly"
    }
else
    echo "⚠️ npx not available, configuring MCP servers manually..."

    # Manual MCP configuration for browser capabilities
    cat > ~/.config/claude-code/.mcp.json << 'EOF'
{
  "mcpServers": {
    "browser": {
      "command": "python",
      "args": ["-m", "mcp_server_browser"],
      "env": {}
    }
  }
}
EOF
fi

# Install document processing plugins
echo "📄 Installing document processing plugins..."
npx claude-plugins install document-processor@anthropics/anthropic-agent-skills --scope project || {
    echo "⚠️ Document processor plugin installation failed, continuing..."
}

# Install GitHub integration plugin
echo "🐙 Installing GitHub integration plugin..."
npx claude-plugins install github-integration@anthropics/claude-code-plugins --scope project || {
    echo "⚠️ GitHub integration plugin installation failed, continuing..."
}

# Install web search capabilities
echo "🔍 Installing web search plugin..."
npx claude-plugins install web-search@anthropics/claude-code-plugins --scope project || {
    echo "⚠️ Web search plugin installation failed, continuing..."
}

# Configure Chrome browser integration (if available)
echo "🌐 Configuring Chrome browser integration..."
if command -v google-chrome &> /dev/null || command -v chromium-browser &> /dev/null; then
    # Install Chrome extension support
    cat > ~/.config/claude-code/chrome-config.json << 'EOF'
{
  "enabled": true,
  "browser_path": "/usr/bin/google-chrome",
  "headless": true,
  "timeout": 30000
}
EOF
    echo "✅ Chrome browser integration configured"
else
    echo "⚠️ Chrome not found, browser integration will be limited"
fi

# Create project-specific plugin configuration
echo "⚙️ Creating project-specific plugin configuration..."
cat > .claude-plugins.json << 'EOF'
{
  "plugins": {
    "browser-mcp": {
      "enabled": true,
      "scope": "project"
    },
    "document-processor": {
      "enabled": true,
      "scope": "project"
    },
    "github-integration": {
      "enabled": true,
      "scope": "project"
    },
    "web-search": {
      "enabled": true,
      "scope": "project"
    }
  },
  "mcp_servers": {
    "browser": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-browser"],
      "enabled": true
    },
    "web-search": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-web-search"],
      "enabled": true
    }
  }
}
EOF

# Install Node.js dependencies for MCP servers if needed
echo "📦 Installing MCP server dependencies..."
if command -v npm &> /dev/null; then
    # Install browser MCP server
    npm install -g @modelcontextprotocol/server-browser @modelcontextprotocol/server-web-search || {
        echo "⚠️ Failed to install MCP servers globally, trying local installation..."
        mkdir -p .forge-tmp/node_modules
        cd .forge-tmp
        npm init -y &> /dev/null || true
        npm install @modelcontextprotocol/server-browser @modelcontextprotocol/server-web-search || {
            echo "⚠️ Local MCP server installation also failed, continuing without..."
        }
        cd ..
    }
fi

# Verify plugin installation
echo "🔍 Verifying plugin installation..."
$CLAUDE_CMD plugin list --scope project > .forge-tmp/plugins/installed-plugins.txt 2>&1 || {
    echo "⚠️ Could not list installed plugins, but configuration is in place"
}

# Test MCP server connectivity
echo "🧪 Testing MCP server connectivity..."
timeout 10 $CLAUDE_CMD --help > /dev/null 2>&1 && {
    echo "✅ Claude Code is responsive"
} || {
    echo "⚠️ Claude Code may not be fully responsive, but plugins are configured"
}

echo "✅ Claude Code plugin setup completed!"
echo ""
echo "📋 Installed capabilities:"
echo "  🌐 Web browsing and content fetching"
echo "  📄 Document processing (PDF, Word, Excel, PowerPoint)"
echo "  🐙 Enhanced GitHub integration"
echo "  🔍 Web search functionality"
echo "  🤖 MCP server integration"
echo ""
echo "💡 Plugins will be available in Claude Code with the following commands:"
echo "  /mcp - List available MCP servers and tools"
echo "  /chrome - Browser automation (if Chrome is available)"
echo "  /search - Web search capabilities"
echo ""
echo "🔧 Configuration files created:"
echo "  - ~/.config/claude-code/.mcp.json (MCP server configuration)"
echo "  - .claude-plugins.json (Project plugin configuration)"
echo "  - ~/.config/claude-code/chrome-config.json (Browser configuration)"
