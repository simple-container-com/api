#!/bin/bash

# Claude Code execution script with verbose options and timeout
# Attempts multiple verbosity levels and provides comprehensive error handling

set -e

PROMPT_FILE="${1:-prompt.txt}"
RESPONSE_FILE="${2:-claude_response.txt}"
ERROR_FILE="${3:-claude_error.txt}"
TIMEOUT_SECONDS="${4:-4500}"

# Source streaming functions for real-time message streaming to API
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/streaming-functions.sh" ]; then
  source "$SCRIPT_DIR/streaming-functions.sh"

  # Debug: Show streaming configuration
  echo "📡 Streaming Configuration:"
  echo "  - STREAMING_ENABLED: ${STREAMING_ENABLED:-true}"
  echo "  - JOB_ID: ${JOB_ID:-<not set>}"
  echo "  - API_URL: ${API_URL:-<not set>}"
  echo "  - STREAMING_API_URL: ${STREAMING_API_URL:-<not set>}"
  echo "  - API_KEY: ${API_KEY:+<set>}${API_KEY:-<not set>}"
  echo "  - GITHUB_RUN_ID: ${GITHUB_RUN_ID:-<not set>}"
  echo ""

  # Initialize streaming
  init_streaming ".forge-tmp/streaming_buffer.json"
else
  echo "⚠️ Streaming functions not found, streaming disabled"
  STREAMING_ENABLED=false
fi

# Export Claude Code timeout env vars as fallback (also configured in ~/.claude/settings.json)
# These prevent "BashTool pre-flight check is taking longer than expected" errors
# when AI gateway proxy adds latency to requests
export BASH_DEFAULT_TIMEOUT_MS="${BASH_DEFAULT_TIMEOUT_MS:-300000}"   # 5 minutes (default: 2min)
export BASH_MAX_TIMEOUT_MS="${BASH_MAX_TIMEOUT_MS:-1800000}"         # 30 minutes (default: 10min)
export MCP_TIMEOUT="${MCP_TIMEOUT:-30000}"                           # 30 seconds (default: 5s)
export MCP_TOOL_TIMEOUT="${MCP_TOOL_TIMEOUT:-300000}"                # 5 minutes

echo "🚀 Starting Claude Code execution..."
echo "🔧 UPDATED EXECUTE-CLAUDE.SH SCRIPT v2.1 (root permission fixes applied)"
echo "- Script location: $(realpath "$0")"
echo "- Script modified: $(stat -c %y "$0" 2>/dev/null || echo 'unknown')"
echo "- Prompt file: $PROMPT_FILE"
echo "- Response file: $RESPONSE_FILE"
echo "- Error file: $ERROR_FILE"
echo "- Timeout: $TIMEOUT_SECONDS seconds"

# Check if Claude is available
if ! command -v claude &> /dev/null; then
  echo "❌ Claude Code is not available"
  echo "Claude installation may have failed or Claude is not in PATH"
  echo "Available commands in ~/.local/bin:"
  ls -la ~/.local/bin/ || echo "~/.local/bin not found"
  echo "PATH contents: $PATH"
  exit 1
fi

# Check if prompt file exists
if [ ! -f "$PROMPT_FILE" ]; then
  echo "❌ Prompt file '$PROMPT_FILE' not found"
  exit 1
fi

echo "Prompt size: $(wc -c < "$PROMPT_FILE") characters"

# Debug filesystem permissions and working directory
echo "🔍 Debugging filesystem permissions for Claude file editing:"
echo "Current working directory: $(pwd)"
echo "Directory permissions: $(ls -ld . | awk '{print $1}')"
echo "Directory owner: $(ls -ld . | awk '{print $3 ":" $4}')"
echo "Current user: $(whoami)"
echo "User groups: $(groups)"
echo "Write test: $(touch .test_write && echo "✅ Can write" && rm .test_write || echo "❌ Cannot write")"
echo ""

# Execute Claude with proper argument passing
echo "Executing Claude Code..."
echo "========================================"

# Initialize output files
mkdir -p .forge-tmp
> "$RESPONSE_FILE"
> "$ERROR_FILE"

# Read prompt file into variable to avoid shell injection issues
echo "Reading prompt file..."
PROMPT=$(cat "$PROMPT_FILE")
if [ -z "$PROMPT" ]; then
  echo "❌ Prompt file is empty or could not be read"
  exit 1
fi

echo "Prompt loaded successfully (${#PROMPT} characters)"

# Execute Claude with proper -p argument (not stdin redirection)
echo "Calling Claude with prompt argument..."

# Try multiple approaches for tool access in CI environment
echo "Attempting Claude execution with full tool access..."

# Execute Claude with streaming JSON for real-time progress
echo "🔄 Starting Claude with streaming JSON output for real-time progress..."

# Create named pipe for streaming output processing
STREAM_PIPE=".forge-tmp/claude_stream"
mkfifo "$STREAM_PIPE" 2>/dev/null || true

# Start background process to parse streaming JSON and show progress
{
  echo "📡 Starting enhanced JSON stream parser with jq and API streaming..."
  while IFS= read -r line; do
    # Send to streaming API (non-blocking)
    parse_claude_stream_for_api "$line" &

    # Use jq for proper JSON parsing if available, fallback to grep
    if command -v jq >/dev/null 2>&1; then
      # Parse with jq for better accuracy - handle nested message structure
      MSG_TYPE=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
      case "$MSG_TYPE" in
        "assistant")
          TIMESTAMP=$(date '+%H:%M:%S')
          # Check for tool usage within message content
          TOOL_USES=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_use") | "\(.name):\(.input.description // .input.command // .input.file_path // "action")"' 2>/dev/null)
          if [ -n "$TOOL_USES" ]; then
            echo "$TOOL_USES" | while read -r tool_info; do
              TOOL_NAME=$(echo "$tool_info" | cut -d':' -f1)
              TOOL_DESC=$(echo "$tool_info" | cut -d':' -f2-)
              echo "[$TIMESTAMP] 🔧 $TOOL_NAME: $TOOL_DESC"
            done
          fi

          # Check for text content within message
          TEXT_CONTENT=$(echo "$line" | jq -r '.message.content[]? | select(.type == "text") | .text' 2>/dev/null | head -c 120)
          if [ -n "$TEXT_CONTENT" ] && [ "$TEXT_CONTENT" != "null" ] && [ "$TEXT_CONTENT" != "" ]; then
            # Filter out system messages and show meaningful content
            if ! echo "$TEXT_CONTENT" | grep -q "session_id\|cwd\|tools\|Perfect!\|Great!\|Excellent!"; then
              echo "[$TIMESTAMP] 💭 ${TEXT_CONTENT}..."
            fi
          fi
          ;;
        "tool_result")
          # Only show tool results if they contain errors or are very short (success indicators)
          RESULT_STATUS=$(echo "$line" | jq -r '.is_error // false' 2>/dev/null)
          if [ "$RESULT_STATUS" = "true" ]; then
            TIMESTAMP=$(date '+%H:%M:%S')
            ERROR_MSG=$(echo "$line" | jq -r '.tool_use_result // .content // "Error occurred"' 2>/dev/null | head -c 100)
            echo "[$TIMESTAMP] ❌ Error: $ERROR_MSG"
          fi
          ;;
        "user")
          # Filter out verbose tool results from user messages
          TOOL_RESULT=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_result") | .content' 2>/dev/null)
          if [ -n "$TOOL_RESULT" ] && [ "$TOOL_RESULT" != "null" ]; then
            # Only show if it's an error or very short output
            IS_ERROR=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_result") | .is_error // false' 2>/dev/null)
            if [ "$IS_ERROR" = "true" ]; then
              TIMESTAMP=$(date '+%H:%M:%S')
              ERROR_CONTENT=$(echo "$TOOL_RESULT" | head -c 100)
              echo "[$TIMESTAMP] ⚠️ Tool error: $ERROR_CONTENT"
            elif [ ${#TOOL_RESULT} -lt 50 ]; then
              TIMESTAMP=$(date '+%H:%M:%S')
              echo "[$TIMESTAMP] ✅ Tool completed: $TOOL_RESULT"
            fi
          fi
          ;;
        "system")
          TIMESTAMP=$(date '+%H:%M:%S')
          SUBTYPE=$(echo "$line" | jq -r '.subtype // empty' 2>/dev/null)
          if [ "$SUBTYPE" = "init" ]; then
            echo "[$TIMESTAMP] 🚀 Claude session initialized"
          fi
          ;;
      esac
    else
      # Fallback to grep-based parsing if jq not available
      if echo "$line" | grep -q '"type"'; then
        TIMESTAMP=$(date '+%H:%M:%S')
        MSG_TYPE=$(echo "$line" | grep -o '"type":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "unknown")
        case "$MSG_TYPE" in
          "tool_use")
            TOOL_NAME=$(echo "$line" | grep -o '"name":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "unknown")
            echo "[$TIMESTAMP] 🔧 Claude using tool: $TOOL_NAME"
            ;;
          "text")
            TEXT_CONTENT=$(echo "$line" | grep -o '"text":"[^"]*"' | cut -d'"' -f4 | head -c 100 2>/dev/null || echo "")
            if [ -n "$TEXT_CONTENT" ]; then
              echo "[$TIMESTAMP] 💭 Claude: ${TEXT_CONTENT}..."
            fi
            ;;
          "tool_result")
            echo "[$TIMESTAMP] ✅ Tool execution completed"
            ;;
        esac
      fi
    fi
    # Also write to response file
    echo "$line" >> "$RESPONSE_FILE"
  done < "$STREAM_PIPE"
} &
PARSER_PID=$!

# Execute Claude with streaming output and plugin support
# Using --dangerously-skip-permissions flag (now safe since we're running as non-root user)
# Added concurrency fixes: disable parallel tool calls, add retry logic
# Enhanced with Claude CLI built-in reliability options (Option 2) + retry logic (Option 3)
echo "🔧 CLAUDE RETRY: Using enhanced Claude CLI options with conversation flow error retry logic..."

# Retry function for conversation flow errors
retry_claude_streaming() {
  local max_retries=2
  local retry_count=0

  while [ $retry_count -le $max_retries ]; do
    echo "🔧 CLAUDE RETRY: Streaming attempt $((retry_count + 1))/$((max_retries + 1))..."

    if timeout "$TIMEOUT_SECONDS" claude \
      -p "$PROMPT" \
      --add-dir "$(pwd)" \
      --allowedTools 'Bash,Grep,Glob,LS,Read,Edit,MultiEdit,Write' \
      --output-format stream-json \
      --verbose \
      --dangerously-skip-permissions \
      --mcp-config ~/.config/claude-code/.mcp.json \
      --fallback-model sonnet \
      > "$STREAM_PIPE" 2> "$ERROR_FILE"; then
      return 0  # Success
    else
      # Check if it's a conversation flow error that we should retry
      if [ -f "$ERROR_FILE" ] && grep -q "tool use concurrency issues\|tool_use ids were found without tool_result blocks" "$ERROR_FILE"; then
        retry_count=$((retry_count + 1))
        if [ $retry_count -le $max_retries ]; then
          echo "🔧 CLAUDE RETRY: Detected conversation flow error, retrying in 2 seconds..."
          sleep 2
          continue
        else
          echo "🔧 CLAUDE RETRY: Conversation flow error persisted after $((max_retries + 1)) attempts"
          return 1
        fi
      else
        echo "🔧 CLAUDE RETRY: Non-retryable error or timeout occurred"
        return 1
      fi
    fi
  done
  return 1
}

if retry_claude_streaming; then

  # Wait for parser to finish
  wait $PARSER_PID 2>/dev/null || true
  rm -f "$STREAM_PIPE" 2>/dev/null || true

  # Finalize streaming (flush remaining messages)
  finalize_streaming 2>/dev/null || true

  echo "✅ Claude Code executed successfully"
  echo "Response size: $(wc -c < "$RESPONSE_FILE" 2>/dev/null || echo 0) bytes"

  # Check if response is actually meaningful and not an error
  if [ ! -s "$RESPONSE_FILE" ]; then
    echo "⚠️ Claude response is empty"
    echo "This might indicate an API issue or authentication problem"
    exit 1
  fi

  # Enhanced debugging: Check what Claude actually did
  echo "🔍 Analyzing Claude's response for tool usage..."

  # Look for tool usage indicators in the response
  TOOL_USAGE_FOUND=false
  if grep -qi "read.*file\|edit.*file\|write.*file\|multiedit\|created.*file\|modified.*file\|updated.*file" "$RESPONSE_FILE"; then
    echo "✅ Found tool usage indicators in Claude's response"
    TOOL_USAGE_FOUND=true
  else
    echo "⚠️ No clear tool usage indicators found in Claude's response"
  fi

  # Verify that files were actually modified
  echo "🔍 Verifying file modifications..."
  MODIFIED_FILES=$(find . -type f -mmin -2 -not -path './.git/*' -not -path './.forge-tmp/*' -not -name "$RESPONSE_FILE" -not -name "$ERROR_FILE" | wc -l)
  if [ "$MODIFIED_FILES" -eq 0 ]; then
    echo "⚠️ WARNING: No files were modified in the last 2 minutes"
    echo "Claude may have described changes without actually implementing them"

    # Show Claude's response summary for debugging
    echo "📄 Claude's response summary for analysis:"
    echo "=========================================="
    echo "Response contains $(wc -l < "$RESPONSE_FILE") lines and $(wc -c < "$RESPONSE_FILE") characters"
    echo "First 500 characters:"
    head -c 500 "$RESPONSE_FILE"
    echo ""
    echo "Last 300 characters:"
    tail -c 300 "$RESPONSE_FILE"
    echo ""
    echo "=========================================="

    # Check if Claude mentioned file modifications in the response
    if grep -qi "test.*pass\|implement.*test\|add.*test\|create.*test\|fix.*test" "$RESPONSE_FILE"; then
      echo "⚠️ Claude mentioned implementing tests but no files were actually changed"
      echo "This could mean Claude is describing changes instead of implementing them"
      echo "🔧 Tool access may be blocked in CI environment despite --dangerously-skip-permissions"
      echo "Continuing anyway as Claude is allowed to not change files"
    else
      echo "ℹ️ Claude response doesn't mention file modifications - this may be intentional"
    fi
  else
    echo "✅ Detected $MODIFIED_FILES file(s) modified in the last 2 minutes"
    echo "Recently modified files:"
    find . -type f -mmin -2 -not -path './.git/*' -not -path './.forge-tmp/*' -not -name "$RESPONSE_FILE" -not -name "$ERROR_FILE" | head -10
  fi

  echo "📄 Claude response summary (first 200 characters):"
  head -c 200 "$RESPONSE_FILE"
  echo ""

else
  claude_exit_code=$?

  # Finalize streaming (flush remaining messages)
  finalize_streaming 2>/dev/null || true

  # Clean up streaming resources
  kill $PARSER_PID 2>/dev/null || true
  wait $PARSER_PID 2>/dev/null || true
  rm -f "$STREAM_PIPE" 2>/dev/null || true

  echo "❌ Claude Code with streaming failed with exit code: $claude_exit_code"
  echo "🔄 Attempting fallback to regular buffered mode..."

  # Fallback: Try regular -p mode without streaming, with concurrency fixes
  > "$RESPONSE_FILE"  # Clear response file
  > "$ERROR_FILE"     # Clear error file

  echo "🔄 Fallback mode: Disabling streaming and parallel tool calls..."
  echo "🔍 Debug: Checking Claude authentication and configuration..."

  # Check if Claude is authenticated
  if ! claude auth status >/dev/null 2>&1; then
    echo "⚠️ Claude authentication check failed, attempting to use API key from environment"
  else
    echo "✅ Claude authentication appears to be working"
  fi

  # Show Claude version and config
  echo "🔧 Claude version: $(claude --version 2>/dev/null || echo 'unknown')"
  echo "🔧 MCP config exists: $([ -f ~/.config/claude-code/.mcp.json ] && echo 'yes' || echo 'no')"

  # Retry function for fallback mode
  retry_claude_fallback() {
    local max_retries=2
    local retry_count=0

    while [ $retry_count -le $max_retries ]; do
      echo "🔧 CLAUDE RETRY: Fallback attempt $((retry_count + 1))/$((max_retries + 1))..."

      if timeout "$TIMEOUT_SECONDS" claude \
        -p "$PROMPT" \
        --add-dir "$(pwd)" \
        --allowedTools 'Bash,Grep,Glob,LS,Read,Edit,MultiEdit,Write' \
        --output-format json \
        --dangerously-skip-permissions \
        --mcp-config ~/.config/claude-code/.mcp.json \
        --max-turns 35 \
        --fallback-model sonnet \
        --max-budget-usd 2.00 \
        2> "$ERROR_FILE" | tee "$RESPONSE_FILE" | while IFS= read -r line; do
          # Send to streaming API (non-blocking)
          parse_claude_stream_for_api "$line" &

          # Show basic progress in logs
          if command -v jq >/dev/null 2>&1; then
            MSG_TYPE=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
            if [ "$MSG_TYPE" = "assistant" ]; then
              TIMESTAMP=$(date '+%H:%M:%S')
              echo "[$TIMESTAMP] 🔄 Claude processing (fallback mode)..."
            fi
          fi
        done; then
        return 0  # Success
      else
        # Check response for conversation flow errors
        if [ -f "$RESPONSE_FILE" ] && grep -q "tool use concurrency issues\|tool_use ids were found without tool_result blocks" "$RESPONSE_FILE"; then
          retry_count=$((retry_count + 1))
          if [ $retry_count -le $max_retries ]; then
            echo "🔧 CLAUDE RETRY: Detected conversation flow error in response, retrying in 2 seconds..."
            > "$RESPONSE_FILE"  # Clear response file
            > "$ERROR_FILE"     # Clear error file
            sleep 2
            continue
          else
            echo "🔧 CLAUDE RETRY: Conversation flow error persisted in fallback mode after $((max_retries + 1)) attempts"
            return 1
          fi
        else
          echo "🔧 CLAUDE RETRY: Non-retryable error in fallback mode"
          return 1
        fi
      fi
    done
    return 1
  }

  if retry_claude_fallback; then

    # Finalize streaming (flush remaining messages)
    finalize_streaming 2>/dev/null || true

    echo "✅ Claude Code executed successfully (fallback mode)"
    echo "Response size: $(wc -c < "$RESPONSE_FILE" 2>/dev/null || echo 0) bytes"

    # Debug: Show what we captured
    echo "🔍 Debug: Response file contents (first 500 chars):"
    if [ -f "$RESPONSE_FILE" ]; then
      head -c 500 "$RESPONSE_FILE" || echo "[Could not read response file]"
    else
      echo "[Response file does not exist]"
    fi
    echo ""

    echo "🔍 Debug: Error file contents:"
    if [ -f "$ERROR_FILE" ] && [ -s "$ERROR_FILE" ]; then
      head -c 500 "$ERROR_FILE" || echo "[Could not read error file]"
    else
      echo "[Error file is empty or does not exist]"
    fi
    echo ""

    # Continue with normal success processing
    if [ ! -s "$RESPONSE_FILE" ]; then
      echo "⚠️ Claude response is empty"
      echo "This might indicate an API issue or authentication problem"
      echo "🔍 Checking for common issues:"

      # Check if Claude is properly installed
      if ! command -v claude >/dev/null 2>&1; then
        echo "❌ Claude command not found in PATH"
        exit 1
      fi

      # Check authentication
      echo "🔍 Testing Claude authentication..."
      if echo "Hello" | claude -p "Say hello back" --output-format text 2>/dev/null | grep -q "hello\|Hello"; then
        echo "✅ Claude authentication works for simple requests"
        echo "❌ Issue appears to be with complex requests or tool usage"
      else
        echo "❌ Claude authentication appears to be broken"
        echo "💡 Try running: claude login"
      fi

      exit 1
    fi

    # Process success normally (duplicate the success logic)
    echo "🔍 Analyzing Claude's response for tool usage..."

    # Look for tool usage indicators in the response
    TOOL_USAGE_FOUND=false
    if grep -qi "read.*file\|edit.*file\|write.*file\|multiedit\|created.*file\|modified.*file\|updated.*file" "$RESPONSE_FILE"; then
      echo "✅ Found tool usage indicators in Claude's response"
      TOOL_USAGE_FOUND=true
    else
      echo "⚠️ No clear tool usage indicators found in Claude's response"
    fi

    # Verify that files were actually modified
    echo "🔍 Verifying file modifications..."
    MODIFIED_FILES=$(find . -type f -mmin -2 -not -path './.git/*' -not -path './.forge-tmp/*' -not -name "$RESPONSE_FILE" -not -name "$ERROR_FILE" | wc -l)
    if [ "$MODIFIED_FILES" -eq 0 ]; then
      echo "⚠️ WARNING: No files were modified in the last 2 minutes"
      echo "Claude may have described changes without actually implementing them"

      # Show Claude's response summary for debugging
      echo "📄 Claude's response summary for analysis:"
      echo "=========================================="
      echo "Response contains $(wc -l < "$RESPONSE_FILE") lines and $(wc -c < "$RESPONSE_FILE") characters"
      echo "First 500 characters:"
      head -c 500 "$RESPONSE_FILE"
      echo ""
      echo "Last 300 characters:"
      tail -c 300 "$RESPONSE_FILE"
      echo ""
      echo "=========================================="

      # Check if Claude mentioned file modifications in the response
      if grep -qi "test.*pass\|implement.*test\|add.*test\|create.*test\|fix.*test" "$RESPONSE_FILE"; then
        echo "⚠️ Claude mentioned implementing tests but no files were actually changed"
        echo "This could mean Claude is describing changes instead of implementing them"
        echo "🔧 Tool access may be blocked in CI environment despite --dangerously-skip-permissions"
        echo "Continuing anyway as Claude is allowed to not change files"
      else
        echo "ℹ️ Claude response doesn't mention file modifications - this may be intentional"
      fi
    else
      echo "✅ Detected $MODIFIED_FILES file(s) modified in the last 2 minutes"
      echo "Recently modified files:"
      find . -type f -mmin -2 -not -path './.git/*' -not -path './.forge-tmp/*' -not -name "$RESPONSE_FILE" -not -name "$ERROR_FILE" | head -10
    fi

    echo "📄 Claude response summary (first 200 characters):"
    head -c 200 "$RESPONSE_FILE"
    echo ""

    echo "✅ Fallback mode completed successfully with response"
    exit 0
  fi

  echo "❌ Both streaming and fallback modes failed"
  echo "🔄 Attempting final retry with minimal options..."

  # Final fallback: Minimal Claude execution without streaming or parallel tools
  > "$RESPONSE_FILE"  # Clear response file
  > "$ERROR_FILE"     # Clear error file

  echo "🔄 Minimal mode: Using basic tools only with text output..."
  echo "🔍 Testing basic Claude functionality first..."

  # Test basic Claude functionality
  if echo "Please respond with just 'OK'" | claude -p "Please respond with just 'OK'" --output-format text 2>/dev/null | grep -q "OK"; then
    echo "✅ Basic Claude functionality confirmed"
  else
    echo "❌ Basic Claude functionality test failed"
    echo "🔍 This indicates a fundamental authentication or installation issue"
  fi

  echo "🔧 CLAUDE RETRY: Minimal mode with enhanced reliability options..."
  if timeout "$TIMEOUT_SECONDS" claude \
    -p "$PROMPT" \
    --add-dir "$(pwd)" \
    --allowedTools 'Read,Edit,Write' \
    --output-format text \
    --dangerously-skip-permissions \
    --max-turns 35 \
    --fallback-model sonnet \
    --max-budget-usd 1.00 \
    > "$RESPONSE_FILE" 2> "$ERROR_FILE"; then

    echo "✅ Claude Code executed successfully (minimal mode)"
    echo "Response size: $(wc -c < "$RESPONSE_FILE" 2>/dev/null || echo 0) bytes"

    # Debug: Show what we captured in minimal mode
    echo "🔍 Debug: Minimal mode response file contents (first 500 chars):"
    if [ -f "$RESPONSE_FILE" ]; then
      head -c 500 "$RESPONSE_FILE" || echo "[Could not read response file]"
    else
      echo "[Response file does not exist]"
    fi
    echo ""

    echo "🔍 Debug: Minimal mode error file contents:"
    if [ -f "$ERROR_FILE" ] && [ -s "$ERROR_FILE" ]; then
      head -c 500 "$ERROR_FILE" || echo "[Could not read error file]"
    else
      echo "[Error file is empty or does not exist]"
    fi
    echo ""

    if [ ! -s "$RESPONSE_FILE" ]; then
      echo "⚠️ Claude response is empty even in minimal mode"
      echo "This indicates a serious authentication or configuration problem"
      echo "🔍 Final diagnostic checks:"

      # Check environment variables
      echo "- CLAUDE_API_KEY set: ${CLAUDE_API_KEY:+yes}${CLAUDE_API_KEY:-no}"
      echo "- HOME directory: $HOME"
      echo "- Claude config dir exists: $([ -d ~/.config/claude-code ] && echo 'yes' || echo 'no')"

      # Try absolute minimal test
      echo "🔍 Trying absolute minimal Claude test..."
      if claude --version >/dev/null 2>&1; then
        echo "✅ Claude binary works"
        if claude -p "test" --output-format text 2>&1 | head -c 100; then
          echo "✅ Claude can process basic prompts"
        else
          echo "❌ Claude cannot process basic prompts"
        fi
      else
        echo "❌ Claude binary test failed"
      fi

      exit 1
    fi

    echo "📄 Claude response summary (first 200 characters):"
    head -c 200 "$RESPONSE_FILE"
    echo ""
    echo "✅ Minimal mode completed successfully with response"
    exit 0
  fi

  echo "❌ All execution modes failed (streaming, fallback, and minimal)"
  echo "🔍 This indicates a critical Claude configuration or authentication issue"
  echo "========================================="

  # Show all captured output
  echo "📄 Complete Claude output captured:"
  if [ -s "$RESPONSE_FILE" ]; then
    echo "[STDOUT CONTENT]:"
    cat "$RESPONSE_FILE"
    echo "[END STDOUT]"
  else
    echo "[STDOUT]: No output captured"
  fi

  if [ -s "$ERROR_FILE" ]; then
    echo "[STDERR CONTENT]:"
    cat "$ERROR_FILE"
    echo "[END STDERR]"
  else
    echo "[STDERR]: No error output captured"
  fi

  # Check for specific error conditions
  if [ $claude_exit_code -eq 124 ]; then
    echo "🕐 Claude execution timed out after $TIMEOUT_SECONDS seconds"
    echo "This could be due to:"
    echo "- API rate limits or slow responses"
    echo "- Large context requiring more processing time"
    echo "- Network connectivity issues"
    echo "- Claude service being unavailable"
    echo "- Authentication or API key issues"
  elif grep -q "Invalid API key\|Please run /login\|authentication" "$RESPONSE_FILE" "$ERROR_FILE" 2>/dev/null; then
    echo "🔑 Authentication issue detected"
    echo "Claude is not authenticated. This could be due to:"
    echo "- Missing or invalid CLAUDE_API_KEY environment variable"
    echo "- Need to run 'claude login' or '/login' command"
    echo "- API key expired or revoked"
    echo "- Network issues preventing authentication"
  elif grep -q "command not found\|No such file" "$ERROR_FILE" 2>/dev/null; then
    echo "📦 Claude installation issue detected"
    echo "Claude Code may not be properly installed or not in PATH"
  fi

  exit 1
fi
