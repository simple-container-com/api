#!/bin/bash

# Claude Code Plugin Test Script
# Tests the installed plugins to verify web fetching and documentation reading capabilities

set -e

echo "🧪 Testing Claude Code plugins functionality..."

# Ensure Claude is available
if ! command -v claude &> /dev/null; then
    echo "❌ Claude Code is not available. Please run setup-claude.sh first."
    exit 1
fi

# Create test directory
mkdir -p .forge-tmp/plugin-tests

echo "📋 Testing available plugins and MCP servers..."

# Test 1: List available MCP servers
echo "🔍 Test 1: Checking available MCP servers..."
claude --help | grep -i mcp || echo "MCP help not available"

# Test 2: Create a simple test prompt for web capabilities
echo "🌐 Test 2: Testing web browsing capabilities..."
cat > .forge-tmp/plugin-tests/web-test-prompt.txt << 'EOF'
Please test your web browsing capabilities by:
1. Use /mcp to list available MCP servers
2. If browser MCP is available, try to fetch the title of https://httpbin.org/html
3. Report what web browsing tools are available to you

This is a test to verify plugin installation. Please respond with what capabilities you have available.
EOF

# Test 3: Execute Claude with MCP enabled to test plugins
echo "🚀 Test 3: Executing Claude with plugin test..."
timeout 60 claude \
  -p "$(cat .forge-tmp/plugin-tests/web-test-prompt.txt)" \
  --mcp \
  > .forge-tmp/plugin-tests/plugin-test-response.txt 2> .forge-tmp/plugin-tests/plugin-test-error.txt || {
  echo "⚠️ Plugin test execution failed or timed out"
}

# Analyze test results
echo "📊 Analyzing test results..."

if [ -s .forge-tmp/plugin-tests/plugin-test-response.txt ]; then
    echo "✅ Claude responded to plugin test"
    echo "Response size: $(wc -c < .forge-tmp/plugin-tests/plugin-test-response.txt) bytes"

    # Check for MCP-related content in response
    if grep -qi "mcp\|browser\|web\|fetch" .forge-tmp/plugin-tests/plugin-test-response.txt; then
        echo "✅ Response contains MCP/web-related content"
    else
        echo "⚠️ Response doesn't mention MCP or web capabilities"
    fi

    echo "📄 First 300 characters of response:"
    head -c 300 .forge-tmp/plugin-tests/plugin-test-response.txt
    echo ""
else
    echo "❌ No response from Claude plugin test"
fi

if [ -s .forge-tmp/plugin-tests/plugin-test-error.txt ]; then
    echo "⚠️ Errors during plugin test:"
    cat .forge-tmp/plugin-tests/plugin-test-error.txt
fi

# Test 4: Check plugin configuration files
echo "🔧 Test 4: Verifying plugin configuration files..."

if [ -f ~/.config/claude-code/.mcp.json ]; then
    echo "✅ MCP configuration file exists"
    echo "MCP servers configured:"
    jq -r '.mcpServers | keys[]' ~/.config/claude-code/.mcp.json 2>/dev/null || echo "Could not parse MCP config"
else
    echo "⚠️ MCP configuration file not found"
fi

if [ -f .claude-plugins.json ]; then
    echo "✅ Project plugin configuration exists"
    echo "Project plugins configured:"
    jq -r '.plugins | keys[]' .claude-plugins.json 2>/dev/null || echo "Could not parse plugin config"
else
    echo "⚠️ Project plugin configuration not found"
fi

# Test 5: Check for Node.js MCP server dependencies
echo "📦 Test 5: Checking MCP server dependencies..."

if command -v npm &> /dev/null; then
    echo "✅ npm is available"

    # Check global installations
    if npm list -g @modelcontextprotocol/server-browser &> /dev/null; then
        echo "✅ Browser MCP server installed globally"
    else
        echo "⚠️ Browser MCP server not found globally"
    fi

    if npm list -g @modelcontextprotocol/server-web-search &> /dev/null; then
        echo "✅ Web search MCP server installed globally"
    else
        echo "⚠️ Web search MCP server not found globally"
    fi

    # Check local installations
    if [ -f .forge-tmp/node_modules/@modelcontextprotocol/server-browser/package.json ]; then
        echo "✅ Browser MCP server installed locally"
    fi

    if [ -f .forge-tmp/node_modules/@modelcontextprotocol/server-web-search/package.json ]; then
        echo "✅ Web search MCP server installed locally"
    fi
else
    echo "⚠️ npm not available - MCP servers may not be functional"
fi

# Summary
echo ""
echo "🎯 Plugin Test Summary:"
echo "======================"

# Count successful indicators
SUCCESS_COUNT=0
TOTAL_TESTS=5

[ -s .forge-tmp/plugin-tests/plugin-test-response.txt ] && ((SUCCESS_COUNT++))
[ -f ~/.config/claude-code/.mcp.json ] && ((SUCCESS_COUNT++))
[ -f .claude-plugins.json ] && ((SUCCESS_COUNT++))
command -v npm &> /dev/null && ((SUCCESS_COUNT++))
[ -f .forge-tmp/node_modules/@modelcontextprotocol/server-browser/package.json ] || npm list -g @modelcontextprotocol/server-browser &> /dev/null && ((SUCCESS_COUNT++))

echo "✅ $SUCCESS_COUNT/$TOTAL_TESTS tests passed"

if [ $SUCCESS_COUNT -ge 3 ]; then
    echo "🎉 Plugin installation appears successful!"
    echo "Claude Code should now have enhanced web browsing and documentation reading capabilities."
else
    echo "⚠️ Some plugin functionality may be limited."
    echo "Check the setup logs and ensure all dependencies are installed."
fi

echo ""
echo "📚 Available capabilities (if plugins are working):"
echo "  🌐 Web browsing: Fetch content from URLs"
echo "  🔍 Web search: Search for information online"
echo "  📄 Document processing: Handle various document formats"
echo "  🐙 Enhanced GitHub integration: Advanced repository operations"
echo ""
echo "💡 Usage in Claude Code:"
echo "  - Use '/mcp' command to list available MCP servers"
echo "  - Use browser tools to fetch web content"
echo "  - Use search tools to find information"
echo "  - Plugins are automatically available when using --mcp flag"
