#!/bin/bash

# Claude Code execution monitoring script
# Monitors Claude execution progress and provides real-time feedback

echo "[$(date)] Claude monitoring started"

while true; do
  sleep 30
  echo "[$(date)] Claude still running... ($(ps aux | grep -c claude) processes)"

  # Check if output files are being written to
  if [ -f .forge-tmp/claude_response.txt ]; then
    response_size=$(wc -c < .forge-tmp/claude_response.txt 2>/dev/null || echo "0")
    echo "[$(date)] Response file size: $response_size bytes"

    # Show recent output if file has grown
    if [ "$response_size" -gt "${last_response_size:-0}" ]; then
      echo "[$(date)] New Claude output detected:"
      echo "--- Recent Claude Activity ---"

      # Parse recent JSON lines for meaningful content (no pipes to avoid buffer overflow)
      if command -v jq >/dev/null 2>&1; then
        # Use temporary file to avoid pipe buffer overflow
        tail -10 .forge-tmp/claude_response.txt 2>/dev/null > .forge-tmp/recent_lines.tmp
        while IFS= read -r line; do
          if [ -n "$line" ]; then
            MSG_TYPE=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
            case "$MSG_TYPE" in
              "assistant")
                # Extract tool usage
                TOOL_USES=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_use") | "\(.name): \(.input.file_path // .input.command // .input.description // "action")"' 2>/dev/null)
                if [ -n "$TOOL_USES" ]; then
                  echo "$TOOL_USES" > .forge-tmp/tool_uses.tmp
                  while read -r tool_info; do
                    echo "[CLAUDE] 🔧 $tool_info"
                  done < .forge-tmp/tool_uses.tmp
                fi

                # Extract text content
                TEXT_CONTENT=$(echo "$line" | jq -r '.message.content[]? | select(.type == "text") | .text' 2>/dev/null | head -c 100)
                if [ -n "$TEXT_CONTENT" ] && [ "$TEXT_CONTENT" != "null" ] && [ "$TEXT_CONTENT" != "" ]; then
                  if ! echo "$TEXT_CONTENT" | grep -q "session_id\|cwd\|tools"; then
                    echo "[CLAUDE] 💭 ${TEXT_CONTENT}..."
                  fi
                fi
                ;;
              "user")
                # Show tool results only if they're errors or very short
                IS_ERROR=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_result") | .is_error // false' 2>/dev/null)
                if [ "$IS_ERROR" = "true" ]; then
                  ERROR_MSG=$(echo "$line" | jq -r '.message.content[]? | select(.type == "tool_result") | .content' 2>/dev/null | head -c 80)
                  echo "[CLAUDE] ❌ Tool error: $ERROR_MSG"
                fi
                ;;
            esac
          fi
        done < .forge-tmp/recent_lines.tmp
        rm -f .forge-tmp/recent_lines.tmp .forge-tmp/tool_uses.tmp
      else
        # Fallback without jq - just show file count and size info
        echo "[CLAUDE] Response updated ($(wc -l < .forge-tmp/claude_response.txt) lines total)"
      fi

      echo "--- End Recent Activity ---"
      last_response_size=$response_size
    fi
  fi

  if [ -f .forge-tmp/claude_error.txt ]; then
    error_size=$(wc -c < .forge-tmp/claude_error.txt 2>/dev/null || echo "0")
    if [ "$error_size" -gt "${last_error_size:-0}" ]; then
      echo "[$(date)] New Claude error output detected ($error_size bytes):"
      echo "--- Recent Claude Errors ---"
      # Use temporary file to avoid pipe buffer overflow
      tail -10 .forge-tmp/claude_error.txt 2>/dev/null > .forge-tmp/recent_errors.tmp
      sed 's/^/[ERROR] /' .forge-tmp/recent_errors.tmp 2>/dev/null || echo "Cannot read error file"
      rm -f .forge-tmp/recent_errors.tmp
      echo "--- End Recent Errors ---"
      last_error_size=$error_size
    fi
  fi

  # Check system resources
  echo "[$(date)] Memory usage: $(free -h | grep Mem | awk '{print $3"/"$2}')"
  echo "[$(date)] CPU load: $(uptime | awk -F'load average:' '{print $2}')"
done
