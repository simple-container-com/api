#!/bin/bash

# Run Claude with context script
# Executes Claude with comprehensive monitoring and logging

set -e

REPOSITORY="$1"
BRANCH="$2"
ISSUE_ID="$3"

if [ -z "$REPOSITORY" ] || [ -z "$BRANCH" ] || [ -z "$ISSUE_ID" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <repository> <branch> <issue_id>"
  exit 1
fi

echo "Running Claude Code with context"

# Detect environment and set script paths
if [ -d "/scripts" ] && [ -f "/scripts/monitor-claude.sh" ]; then
  # Docker environment - scripts are in /scripts/
  SCRIPTS_PATH="/scripts"
  echo "🐳 Detected Docker environment, using /scripts/"
elif [ -n "$SIMPLE_FORGE_WORK_DIR" ] && [ -d "$SIMPLE_FORGE_WORK_DIR/scripts" ]; then
  # Dockerless environment - scripts are extracted to $SIMPLE_FORGE_WORK_DIR/scripts/
  SCRIPTS_PATH="$SIMPLE_FORGE_WORK_DIR/scripts"
  echo "🔧 Detected dockerless environment, using $SIMPLE_FORGE_WORK_DIR/scripts/"
else
  # Fallback - try local .github/scripts
  SCRIPTS_PATH=".github/scripts"
  echo "⚠️  Using fallback path: .github/scripts/"
fi

# Ensure Claude is in PATH
export PATH="$HOME/.local/bin:$PATH"

# Debug: Show environment variables received from workflow
echo "🔍 Environment variables received from workflow:"
echo "  - JOB_ID: ${JOB_ID:-<not set>}"
echo "  - API_URL: ${API_URL:-<not set>}"
echo "  - STREAMING_API_URL: ${STREAMING_API_URL:-<not set>}"
echo "  - API_KEY: ${API_KEY:+<set>}${API_KEY:-<not set>}"
echo ""

# Ensure streaming environment variables are set (for real-time Claude message streaming)
if [ -z "$JOB_ID" ]; then
  echo "⚠️ Warning: JOB_ID environment variable not set. Streaming will be disabled."
  export STREAMING_ENABLED=false
else
  echo "📡 Streaming enabled for job: $JOB_ID"
  export JOB_ID="$JOB_ID"
  export STREAMING_ENABLED=true
fi

if [ -z "$API_URL" ] && [ -z "$STREAMING_API_URL" ]; then
  echo "⚠️ Warning: API_URL/STREAMING_API_URL not set. Using default."
  export STREAMING_API_URL="${STREAMING_API_URL:-https://forge.simple-container.com}"
else
  export STREAMING_API_URL="${STREAMING_API_URL:-$API_URL}"
  export API_URL="${API_URL}"
  echo "📡 Streaming API URL: $STREAMING_API_URL"
fi

# Export API_KEY if set
if [ -n "$API_KEY" ]; then
  export API_KEY="$API_KEY"
  echo "📡 API_KEY: <set>"
else
  echo "⚠️ Warning: API_KEY not set. Streaming authentication may fail."
fi

# Debug: Show exported environment variables
echo ""
echo "🔍 Exported environment variables:"
echo "  - STREAMING_ENABLED: ${STREAMING_ENABLED}"
echo "  - JOB_ID: ${JOB_ID:-<not set>}"
echo "  - API_URL: ${API_URL:-<not set>}"
echo "  - STREAMING_API_URL: ${STREAMING_API_URL:-<not set>}"
echo "  - API_KEY: ${API_KEY:+<set>}${API_KEY:-<not set>}"
echo ""

# Check if Claude is available
if ! command -v claude &> /dev/null; then
  echo "❌ Claude Code is not available"
  echo "Claude installation may have failed or Claude is not in PATH"
  echo "Available commands in ~/.local/bin:"
  ls -la ~/.local/bin/ || echo "~/.local/bin not found"
  echo "PATH contents: $PATH"
  echo "❌ Workflow will fail - monitoring logic will detect and handle the failure"
  exit 1
fi

# Create temporary directory for Claude files
mkdir -p .forge-tmp

# Use the prepared conversation context as the prompt
if [ -f .forge-tmp/conversation.txt ]; then
  echo "Using prepared conversation context as prompt"
  cp .forge-tmp/conversation.txt .forge-tmp/prompt.txt

  # Append automation instructions to the conversation
  cat >> .forge-tmp/prompt.txt << EOF

AUTOMATION CONTEXT:
Repository: $REPOSITORY
Branch: $BRANCH
Issue ID: $ISSUE_ID

CRITICAL REQUIREMENT: This is running in an automated GitHub Actions workflow. You MUST actually modify files on the filesystem - not just describe what should be changed. The workflow expects real file modifications that will be committed to the repository.

🚨 MANDATORY FILE MODIFICATION REQUIREMENTS:
- You MUST use your file editing tools to make actual changes to files
- Simply describing changes or providing code suggestions is NOT sufficient
- Every code change you mention MUST be implemented by actually editing the corresponding files
- The workflow will FAIL if you don't make real filesystem modifications
- Use these tools to make actual changes: Read, Edit, MultiEdit, Write
- Always verify your changes by reading files back after modification using Read

🌐 ENHANCED CAPABILITIES AVAILABLE:
- Web browsing and content fetching: Use /mcp browser tools to fetch documentation, examples, and resources from the web
- Document processing: Access and process PDF, Word, Excel, and PowerPoint documents
- Web search: Use search capabilities to find relevant information, libraries, and solutions
- GitHub integration: Enhanced GitHub API access for comprehensive repository operations
- External documentation: Fetch and read documentation from external sources to inform your solutions

💡 RECOMMENDED WORKFLOW WITH WEB CAPABILITIES:
1. If the issue involves external libraries, frameworks, or APIs, use web browsing to fetch current documentation
2. Search for best practices, examples, and solutions related to the problem domain
3. Read relevant documentation to ensure your implementation follows current standards
4. Use the enhanced GitHub integration for comprehensive repository analysis
5. Implement solutions based on the most current information available

🔧 TOOL USAGE VERIFICATION:
- BEFORE responding, you MUST verify that you have actually used file editing tools
- If you describe implementing code changes, you MUST have used Edit, MultiEdit, or Write tools
- The CI system will check for actual file modifications on the filesystem
- If no files are modified but you claim to have made changes, the workflow will FAIL
- Start your response by using Read tool to examine existing files
- End your response by using Read tool to verify your changes were applied

Please analyze the issue based on the conversation history above and make the necessary code changes directly to the files in this repository. You must:

1. Read existing files to understand the current state
2. ACTUALLY MODIFY FILES using your editing tools (not just describe changes)
3. Create new files if needed using Write tool
4. Verify changes by reading files back after modification
5. Ensure all changes are working and production-ready
6. Apply all changes immediately without asking for confirmation

VERIFICATION REQUIREMENT: After making changes, always read the modified files back to confirm your edits were actually applied to the filesystem. If a file doesn't reflect your changes, try the modification again.

Proceed with confidence and make all necessary ACTUAL FILE MODIFICATIONS based on the context provided above.
EOF
else
  echo "⚠️ No conversation context found, creating basic prompt"
  # Fallback to basic prompt if conversation context is missing
  cat > .forge-tmp/prompt.txt << EOF
You are working on a GitHub repository to solve issue #$ISSUE_ID in an automated workflow.

Repository: $REPOSITORY
Branch: $BRANCH
Issue ID: $ISSUE_ID

CRITICAL REQUIREMENT: This is running in an automated GitHub Actions workflow. You MUST actually modify files on the filesystem - not just describe what should be changed. The workflow expects real file modifications that will be committed to the repository.

🚨 MANDATORY FILE MODIFICATION REQUIREMENTS:
- You MUST use your file editing tools to make actual changes to files
- Simply describing changes or providing code suggestions is NOT sufficient
- Every code change you mention MUST be implemented by actually editing the corresponding files
- The workflow will FAIL if you don't make real filesystem modifications
- Use these tools to make actual changes: Read, Edit, MultiEdit, Write
- Always verify your changes by reading files back after modification using Read

🔧 TOOL USAGE VERIFICATION:
- BEFORE responding, you MUST verify that you have actually used file editing tools
- If you describe implementing code changes, you MUST have used Edit, MultiEdit, or Write tools
- The CI system will check for actual file modifications on the filesystem
- If no files are modified but you claim to have made changes, the workflow will FAIL
- Start your response by using Read tool to examine existing files
- End your response by using Read tool to verify your changes were applied

Please analyze the issue and make the necessary code changes directly to the files in this repository. You must:

1. Read existing files to understand the current state
2. ACTUALLY MODIFY FILES using your editing tools (not just describe changes)
3. Create new files if needed using Write tool
4. Verify changes by reading files back after modification
5. Ensure all changes are working and production-ready
6. Apply all changes immediately without asking for confirmation

VERIFICATION REQUIREMENT: After making changes, always read the modified files back to confirm your edits were actually applied to the filesystem. If a file doesn't reflect your changes, try the modification again.

The conversation history contains the full context of what needs to be done. Proceed with confidence and make all necessary ACTUAL FILE MODIFICATIONS.
EOF
fi

# Run Claude Code with timeout and comprehensive monitoring
echo "Executing Claude Code with 75-minute timeout..."
echo "Start time: $(date)"
echo "Prompt size: $(wc -c < .forge-tmp/prompt.txt) characters"

# Make scripts executable
chmod +x "$SCRIPTS_PATH/monitor-claude.sh"
chmod +x "$SCRIPTS_PATH/execute-claude.sh"
chmod +x "$SCRIPTS_PATH/system-diagnostics.sh"

# Check Claude options
echo "🔍 Checking Claude Code options..."
claude --help 2>&1 | head -20 || echo "Claude help not available"

# Start monitoring in background
"$SCRIPTS_PATH/monitor-claude.sh" &
MONITOR_PID=$!
echo "Started monitoring process: $MONITOR_PID"

# Show prompt preview for debugging
echo "📄 Prompt preview (first 500 characters):"
echo "========================================"
head -c 500 .forge-tmp/prompt.txt
echo ""
echo "========================================"
echo ""

# Verify working directory and permissions before Claude execution
echo "🔍 Pre-execution environment verification:"
echo "Working directory: $(pwd)"
echo "Directory permissions:"
ls -la . | head -5
echo "Git repository status:"
git status --porcelain | head -5 || echo "Git status check failed"
echo "Available disk space:"
df -h . | tail -1
echo ""

# Execute Claude with comprehensive error handling and full output logging
echo "📄 All Claude output will be captured in workflow logs..."
echo "🔄 Starting Claude execution - you should see interactive output below..."
if "$SCRIPTS_PATH/execute-claude.sh" .forge-tmp/prompt.txt .forge-tmp/claude_response.txt .forge-tmp/claude_error.txt 4500; then
  # Stop monitoring
  kill $MONITOR_PID 2>/dev/null || true
  echo "✅ Claude execution completed successfully"

  # Post-execution verification
  echo "🔍 Post-execution file system verification:"
  echo "Git status after Claude execution:"
  git status --porcelain || echo "Git status check failed"
  echo "Recently modified files (last 2 minutes):"
  find . -type f -mmin -2 -not -path './.git/*' -not -path './.forge-tmp/*' | head -10 || echo "No recently modified files found"
  echo ""

  # Show complete Claude output in workflow logs
  echo "========================================"
  echo "📄 COMPLETE CLAUDE OUTPUT:"
  echo "========================================"
  if [ -s .forge-tmp/claude_response.txt ]; then
    echo "[CLAUDE RESPONSE - $(wc -c < .forge-tmp/claude_response.txt) bytes]:"
    # Use head and tail to avoid buffer overflow with large files
    RESPONSE_SIZE=$(wc -c < .forge-tmp/claude_response.txt)
    if [ "$RESPONSE_SIZE" -gt 10000 ]; then
      echo "[SHOWING FIRST 5000 AND LAST 5000 CHARACTERS DUE TO SIZE]"
      head -c 5000 .forge-tmp/claude_response.txt
      echo ""
      echo "[... TRUNCATED $(($RESPONSE_SIZE - 10000)) CHARACTERS ...]"
      echo ""
      tail -c 5000 .forge-tmp/claude_response.txt
    else
      # Safe to cat smaller files
      cat .forge-tmp/claude_response.txt
    fi
    echo "[END CLAUDE RESPONSE]"
  else
    echo "[CLAUDE RESPONSE]: No response generated"
  fi

  if [ -s .forge-tmp/claude_error.txt ]; then
    echo "[CLAUDE ERRORS - $(wc -c < .forge-tmp/claude_error.txt) bytes]:"
    # Use head and tail to avoid buffer overflow with large error files
    ERROR_SIZE=$(wc -c < .forge-tmp/claude_error.txt)
    if [ "$ERROR_SIZE" -gt 5000 ]; then
      echo "[SHOWING FIRST 2500 AND LAST 2500 CHARACTERS DUE TO SIZE]"
      head -c 2500 .forge-tmp/claude_error.txt
      echo ""
      echo "[... TRUNCATED $(($ERROR_SIZE - 5000)) CHARACTERS ...]"
      echo ""
      tail -c 2500 .forge-tmp/claude_error.txt
    else
      # Safe to cat smaller error files
      cat .forge-tmp/claude_error.txt
    fi
    echo "[END CLAUDE ERRORS]"
  else
    echo "[CLAUDE ERRORS]: No errors logged"
  fi
  echo "========================================"
else
  claude_exit_code=$?
  # Stop monitoring
  kill $MONITOR_PID 2>/dev/null || true
  echo "❌ Claude execution failed with exit code: $claude_exit_code"

  # Show system diagnostics
  "$SCRIPTS_PATH/system-diagnostics.sh"

  # Show any partial response
  echo "Claude stdout (if any):"
  if [ -s .forge-tmp/claude_response.txt ]; then
    echo "Partial response (first 500 chars):"
    head -c 500 .forge-tmp/claude_response.txt 2>/dev/null || echo "Cannot read response file"
  else
    echo "No stdout from Claude"
  fi

  echo "❌ Workflow will fail - monitoring logic will detect and handle the failure"
  exit 1
fi
