#!/bin/bash

# Update context back to service script
# Updates the job context with Claude's response

set -e

JOB_ID="$1"
SERVICE_URL="$2"
BRANCH="$3"
API_KEY="$4"

if [ -z "$JOB_ID" ] || [ -z "$SERVICE_URL" ] || [ -z "$BRANCH" ] || [ -z "$API_KEY" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <service_url> <branch> <api_key>"
  exit 1
fi

echo "Updating context back to service"

# Create updated context with Claude's response (properly escaped)
echo "Preparing context update with Claude's response..."

# Create temp directory and get Claude's response and escape it properly for JSON
mkdir -p .forge-tmp

# Create the content file first to avoid argument length issues
if [ -f .forge-tmp/claude_response.txt ] && [ -s .forge-tmp/claude_response.txt ]; then
  # Extract a summary from Claude's response instead of the full content
  echo "Extracting Claude response summary..."

  # Parse Claude's streaming JSON response to extract meaningful text content
  {
    echo "I've processed the issue and made the following changes:"
    echo ""

    # Extract human-readable text from Claude's streaming JSON response
    if command -v jq >/dev/null 2>&1; then
      # Parse each JSON line and extract text content from assistant messages
      EXTRACTED_TEXT=""
      while IFS= read -r line; do
        if [ -n "$line" ] && echo "$line" | jq -e . >/dev/null 2>&1; then
          # Check if this is an assistant message with text content
          MSG_TYPE=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
          if [ "$MSG_TYPE" = "assistant" ]; then
            # Extract text content from the message
            TEXT_CONTENT=$(echo "$line" | jq -r '.message.content[]? | select(.type == "text") | .text' 2>/dev/null)
            if [ -n "$TEXT_CONTENT" ] && [ "$TEXT_CONTENT" != "null" ]; then
              # Filter out very short or system-like messages
              TEXT_LENGTH=$(echo -n "$TEXT_CONTENT" | wc -c)
              if [ "$TEXT_LENGTH" -gt 20 ] && ! echo "$TEXT_CONTENT" | grep -q "session_id\|uuid\|cwd"; then
                if [ -n "$EXTRACTED_TEXT" ]; then
                  EXTRACTED_TEXT="$EXTRACTED_TEXT

$TEXT_CONTENT"
                else
                  EXTRACTED_TEXT="$TEXT_CONTENT"
                fi
              fi
            fi
          fi
        fi
      done < .forge-tmp/claude_response.txt

      # Output the extracted text or fallback
      if [ -n "$EXTRACTED_TEXT" ]; then
        echo "$EXTRACTED_TEXT"
      else
        echo "Successfully completed the requested changes. Please review the workflow logs for detailed information about what was accomplished."
      fi
    else
      # Fallback without jq - use the original truncation approach
      echo "=== Claude Response Summary ==="
      head -c 2000 .forge-tmp/claude_response.txt
      if [ $(wc -c < .forge-tmp/claude_response.txt) -gt 2000 ]; then
        echo ""
        echo "... (content truncated - full details in workflow logs) ..."
      fi
      echo ""
      echo "=== End Summary ==="
    fi

    echo ""
    echo "Changes have been committed to branch $BRANCH."
  } > .forge-tmp/claude_summary.txt
else
  echo "I've processed the issue but no response content was generated. Changes have been committed to branch $BRANCH." > .forge-tmp/claude_summary.txt
fi

# Create properly formatted JSON using jq to ensure proper escaping
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Get version number safely
if [ -f .forge-tmp/context.json ]; then
  VERSION=$(jq '.version + 1 // 2' .forge-tmp/context.json 2>/dev/null || echo 2)
else
  VERSION=2
fi

# Ensure VERSION is a valid number
if ! [[ "$VERSION" =~ ^[0-9]+$ ]]; then
  VERSION=2
fi

# Extract Claude model information from the response
CLAUDE_MODEL=""
if [ -f .forge-tmp/claude_response.txt ] && [ -s .forge-tmp/claude_response.txt ] && command -v jq >/dev/null 2>&1; then
  # Look for model information in Claude's streaming JSON response
  # Claude Code typically includes model information in system messages or metadata
  while IFS= read -r line; do
    if [ -n "$line" ] && echo "$line" | jq -e . >/dev/null 2>&1; then
      # Check for model information in various message types
      MODEL_INFO=$(echo "$line" | jq -r '.model // .message.model // .metadata.model // empty' 2>/dev/null)
      if [ -n "$MODEL_INFO" ] && [ "$MODEL_INFO" != "null" ]; then
        CLAUDE_MODEL="$MODEL_INFO"
        break
      fi

      # Also check for model info in system messages
      MSG_TYPE=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
      if [ "$MSG_TYPE" = "system" ]; then
        # Look for model information in system message content
        SYSTEM_CONTENT=$(echo "$line" | jq -r '.message.content // .content // empty' 2>/dev/null)
        if echo "$SYSTEM_CONTENT" | grep -q "claude-3"; then
          # Extract model name from system content (e.g., "claude-3-5-sonnet-20241022")
          EXTRACTED_MODEL=$(echo "$SYSTEM_CONTENT" | grep -o "claude-3[^[:space:]]*" | head -1)
          if [ -n "$EXTRACTED_MODEL" ]; then
            CLAUDE_MODEL="$EXTRACTED_MODEL"
            break
          fi
        fi
      fi
    fi
  done < .forge-tmp/claude_response.txt
fi

# If no model found in response, try to detect from Claude Code version/config
if [ -z "$CLAUDE_MODEL" ]; then
  # Try to get model info from Claude Code directly
  if command -v claude >/dev/null 2>&1; then
    # Try to extract model from Claude Code help or version info
    CLAUDE_VERSION_INFO=$(claude --help 2>&1 | head -10 || echo "")
    if echo "$CLAUDE_VERSION_INFO" | grep -q "claude-3"; then
      CLAUDE_MODEL=$(echo "$CLAUDE_VERSION_INFO" | grep -o "claude-3[^[:space:]]*" | head -1)
    fi
  fi
fi

# Default to a reasonable fallback if still no model detected
if [ -z "$CLAUDE_MODEL" ]; then
  CLAUDE_MODEL="claude-3-5-sonnet-20241022"  # Default to current Claude 3.5 Sonnet
fi

echo "Detected Claude model: $CLAUDE_MODEL"

# Extract actual token usage from Claude's streaming JSON response
# Claude Code streaming JSON includes usage information with input_tokens and output_tokens
INPUT_TOKENS=0
OUTPUT_TOKENS=0

if [ -f .forge-tmp/claude_response.txt ] && [ -s .forge-tmp/claude_response.txt ] && command -v jq >/dev/null 2>&1; then
  echo "Extracting token usage from Claude's response..."

  # Parse Claude's streaming JSON response to find usage information
  # The usage object typically appears in message_stop events or final response
  while IFS= read -r line; do
    if [ -n "$line" ] && echo "$line" | jq -e . >/dev/null 2>&1; then
      # Extract usage information from the JSON line
      USAGE_INPUT=$(echo "$line" | jq -r '.usage.input_tokens // .message.usage.input_tokens // empty' 2>/dev/null)
      USAGE_OUTPUT=$(echo "$line" | jq -r '.usage.output_tokens // .message.usage.output_tokens // empty' 2>/dev/null)

      # Update totals if we found valid usage data (accumulate across multiple events)
      if [ -n "$USAGE_INPUT" ] && [ "$USAGE_INPUT" != "null" ] && [ "$USAGE_INPUT" -gt 0 ]; then
        INPUT_TOKENS=$((INPUT_TOKENS + USAGE_INPUT))
      fi
      if [ -n "$USAGE_OUTPUT" ] && [ "$USAGE_OUTPUT" != "null" ] && [ "$USAGE_OUTPUT" -gt 0 ]; then
        OUTPUT_TOKENS=$((OUTPUT_TOKENS + USAGE_OUTPUT))
      fi
    fi
  done < .forge-tmp/claude_response.txt
fi

# If no token usage found in response, fall back to estimation
if [ "$INPUT_TOKENS" -eq 0 ] && [ "$OUTPUT_TOKENS" -eq 0 ]; then
  echo "⚠️ No token usage found in Claude response, using estimation..."

  # Estimate output tokens from response content
  CONTENT_LENGTH=$(wc -c < .forge-tmp/claude_summary.txt)
  OUTPUT_TOKENS=$((CONTENT_LENGTH / 4))

  # Ensure minimum token count of 1 if content exists
  if [ "$CONTENT_LENGTH" -gt 0 ] && [ "$OUTPUT_TOKENS" -eq 0 ]; then
    OUTPUT_TOKENS=1
  fi

  # Estimate input tokens from the context that was sent
  if [ -f .forge-tmp/conversation.txt ]; then
    INPUT_CONTENT_LENGTH=$(wc -c < .forge-tmp/conversation.txt)
    INPUT_TOKENS=$((INPUT_CONTENT_LENGTH / 4))
  fi

  echo "Estimated tokens - Input: $INPUT_TOKENS, Output: $OUTPUT_TOKENS (based on content length)"
else
  echo "✅ Extracted actual token usage - Input: $INPUT_TOKENS, Output: $OUTPUT_TOKENS"
fi

# Calculate total tokens for backward compatibility
TOTAL_TOKENS=$((INPUT_TOKENS + OUTPUT_TOKENS))

# Use jq to create the JSON properly with file input, including model and token information
jq -n \
  --rawfile content .forge-tmp/claude_summary.txt \
  --arg timestamp "$TIMESTAMP" \
  --arg model "$CLAUDE_MODEL" \
  --argjson tokens "$TOTAL_TOKENS" \
  --argjson inputTokens "$INPUT_TOKENS" \
  --argjson outputTokens "$OUTPUT_TOKENS" \
  --argjson version "$VERSION" \
  '{
    "messages": [
      {
        "role": "assistant",
        "content": $content,
        "timestamp": $timestamp,
        "model": $model,
        "tokens": $tokens,
        "inputTokens": $inputTokens,
        "outputTokens": $outputTokens
      }
    ],
    "version": $version
  }' > .forge-tmp/updated_context.json

echo "Context JSON created, size: $(wc -c < .forge-tmp/updated_context.json) bytes"
echo "Validating JSON format..."
if jq empty .forge-tmp/updated_context.json 2>/dev/null; then
  echo "✅ JSON is valid"
else
  echo "❌ JSON is invalid, showing content:"
  cat .forge-tmp/updated_context.json
  echo "Falling back to minimal context update"
  echo '{"messages":[{"role":"assistant","content":"Workflow completed","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'","model":"'$CLAUDE_MODEL'","tokens":5,"inputTokens":0,"outputTokens":5}],"version":2}' > .forge-tmp/updated_context.json
fi

# Send updated context back to service with HTTP status check
http_code=$(curl -s -w "%{http_code}" -X POST \
  "$SERVICE_URL/api/jobs/$JOB_ID/context" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d @.forge-tmp/updated_context.json \
  -o .forge-tmp/context_update_response.json)

echo "Context update HTTP Status: $http_code"

if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
  echo "✅ Context updated successfully"
else
  echo "⚠️ Failed to update context (HTTP $http_code)"
  cat .forge-tmp/context_update_response.json

  if [[ "$http_code" == "401" ]] || [[ "$http_code" == "403" ]]; then
    echo "❌ Authentication failed for context update"
    echo "Context could not be saved back to service"
  else
    echo "⚠️ Context update failed due to service error"
  fi
fi

echo "Context update completed"
