#!/bin/bash

# Real-time Claude message streaming functions for GitHub Actions
# This script provides functions to batch and send Claude's streaming messages to the API

# Configuration (can be overridden via environment variables)
# Reduced batch size and increased interval to prevent API concurrency issues
STREAMING_ENABLED="${STREAMING_ENABLED:-true}"
STREAMING_API_URL="${STREAMING_API_URL:-${API_URL:-https://forge.simple-container.com}}"
STREAMING_BATCH_SIZE="${STREAMING_BATCH_SIZE:-5}"  # Reduced from 20 to 5
STREAMING_BATCH_INTERVAL="${STREAMING_BATCH_INTERVAL:-10}"  # Increased from 5 to 10 seconds
STREAMING_MAX_RETRIES=5  # Increased from 3 to 5
STREAMING_RETRY_DELAY=2  # Increased from 1 to 2 seconds
STREAMING_RATE_LIMIT_DELAY=1  # Add delay between API calls

# Global state
STREAMING_MESSAGE_BUFFER=()
STREAMING_SEQUENCE_NUMBER=1
STREAMING_LAST_SEND_TIME=$(date +%s)
STREAMING_LAST_API_CALL=0  # Track last API call for rate limiting
STREAMING_PENDING_TOOLS=()  # Track pending tool calls to prevent concurrency

# Initialize streaming buffer file
init_streaming() {
  STREAMING_BUFFER_FILE="${1:-.forge-tmp/streaming_buffer.json}"
  mkdir -p "$(dirname "$STREAMING_BUFFER_FILE")"
  echo "[]" > "$STREAMING_BUFFER_FILE"

  if [ "$STREAMING_ENABLED" = "true" ]; then
    echo "📡 Streaming enabled: API=$STREAMING_API_URL, Batch size=$STREAMING_BATCH_SIZE, Interval=${STREAMING_BATCH_INTERVAL}s"
  else
    echo "📡 Streaming disabled"
  fi
}

# Add a message to the streaming buffer
add_streaming_message() {
  local msg_type="$1"
  local content="$2"
  local tool_name="$3"
  local tool_description="$4"
  local is_error="${5:-false}"
  local severity="${6:-info}"

  if [ "$STREAMING_ENABLED" != "true" ]; then
    return 0
  fi

  # Create message JSON
  local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.%3NZ")
  local message_json=$(cat <<EOF
{
  "timestamp": "$timestamp",
  "type": "$msg_type",
  "content": $(echo "$content" | jq -Rs . 2>/dev/null || echo "\"$content\""),
  "severity": "$severity"
EOF
)

  # Add optional fields
  if [ -n "$tool_name" ]; then
    message_json="$message_json, \"toolName\": \"$tool_name\""
  fi

  if [ -n "$tool_description" ]; then
    message_json="$message_json, \"toolDescription\": $(echo "$tool_description" | jq -Rs . 2>/dev/null || echo "\"$tool_description\"")"
  fi

  if [ "$is_error" = "true" ]; then
    message_json="$message_json, \"isError\": true"
  fi

  message_json="$message_json }"

  # Append to buffer file
  if [ -f "$STREAMING_BUFFER_FILE" ]; then
    local current_buffer=$(cat "$STREAMING_BUFFER_FILE")
    local updated_buffer=$(echo "$current_buffer" | jq --argjson msg "$message_json" '. + [$msg]' 2>/dev/null || echo "[$message_json]")
    echo "$updated_buffer" > "$STREAMING_BUFFER_FILE"

    # Check if we should flush the buffer
    local buffer_size=$(echo "$updated_buffer" | jq 'length' 2>/dev/null || echo "0")
    local current_time=$(date +%s)
    local time_since_last_send=$((current_time - STREAMING_LAST_SEND_TIME))

    if [ "$buffer_size" -ge "$STREAMING_BATCH_SIZE" ] || [ "$time_since_last_send" -ge "$STREAMING_BATCH_INTERVAL" ]; then
      flush_streaming_buffer
    fi
  fi
}

# Flush the streaming buffer and send to API
flush_streaming_buffer() {
  if [ "$STREAMING_ENABLED" != "true" ] || [ ! -f "$STREAMING_BUFFER_FILE" ]; then
    return 0
  fi

  local buffer_content=$(cat "$STREAMING_BUFFER_FILE")
  local message_count=$(echo "$buffer_content" | jq 'length' 2>/dev/null || echo "0")

  if [ "$message_count" -eq "0" ]; then
    return 0
  fi

  # Create batch payload
  local batch_id="batch_${GITHUB_RUN_ID}_${STREAMING_SEQUENCE_NUMBER}_$(date +%s)"
  local payload=$(cat <<EOF
{
  "messages": $buffer_content,
  "batchId": "$batch_id",
  "sequenceNumber": $STREAMING_SEQUENCE_NUMBER
}
EOF
)

  # Send to API with retries
  send_streaming_batch "$payload" "$batch_id"
  local send_status=$?

  if [ $send_status -eq 0 ]; then
    # Clear buffer on success
    echo "[]" > "$STREAMING_BUFFER_FILE"
    STREAMING_SEQUENCE_NUMBER=$((STREAMING_SEQUENCE_NUMBER + 1))
    STREAMING_LAST_SEND_TIME=$(date +%s)
  else
    echo "⚠️ Failed to send streaming batch after retries, buffering locally"
  fi
}

# Send a batch to the API with retry logic and rate limiting
send_streaming_batch() {
  local payload="$1"
  local batch_id="$2"
  local retry_count=0

  # Rate limiting: ensure minimum delay between API calls
  local current_time=$(date +%s)
  local time_since_last_call=$((current_time - STREAMING_LAST_API_CALL))
  if [ $time_since_last_call -lt $STREAMING_RATE_LIMIT_DELAY ]; then
    local sleep_time=$((STREAMING_RATE_LIMIT_DELAY - time_since_last_call))
    sleep $sleep_time
  fi
  STREAMING_LAST_API_CALL=$(date +%s)

  while [ $retry_count -lt $STREAMING_MAX_RETRIES ]; do
    local response=$(curl -X POST \
      "${STREAMING_API_URL}/api/jobs/${JOB_ID}/stream" \
      -H "Authorization: Bearer ${GITHUB_OIDC_TOKEN:-$API_KEY}" \
      -H "Content-Type: application/json" \
      -H "X-GitHub-Workflow-Run-ID: ${GITHUB_RUN_ID}" \
      -H "X-GitHub-Repository: ${GITHUB_REPOSITORY}" \
      -d "$payload" \
      --silent \
      --show-error \
      --max-time 10 \
      -w "\n%{http_code}" 2>&1)

    local http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "201" ] || [ "$http_code" = "200" ]; then
      echo "✅ Sent streaming batch $batch_id ($message_count messages)"
      return 0
    elif [ "$http_code" = "400" ]; then
      # 400 errors might be concurrency issues - retry with longer delay
      retry_count=$((retry_count + 1))
      if [ $retry_count -lt $STREAMING_MAX_RETRIES ]; then
        local delay=$((STREAMING_RETRY_DELAY * 3 * retry_count))  # Longer delay for 400 errors
        echo "⚠️ Streaming batch $batch_id failed with HTTP 400 (possible concurrency), retrying in ${delay}s (attempt $retry_count/$STREAMING_MAX_RETRIES)"
        sleep $delay
      else
        echo "❌ Streaming batch $batch_id failed with HTTP 400 after $STREAMING_MAX_RETRIES retries"
        echo "Response: $response_body"
        return 1
      fi
    elif [ "$http_code" = "401" ] || [ "$http_code" = "403" ] || [ "$http_code" = "404" ]; then
      # Non-retryable errors
      echo "❌ Streaming batch $batch_id failed with HTTP $http_code (non-retryable)"
      echo "Response: $response_body"
      return 1
    else
      # Retryable errors (5xx, timeout, network errors)
      retry_count=$((retry_count + 1))
      if [ $retry_count -lt $STREAMING_MAX_RETRIES ]; then
        local delay=$((STREAMING_RETRY_DELAY * (2 ** (retry_count - 1))))
        echo "⚠️ Streaming batch $batch_id failed with HTTP $http_code, retrying in ${delay}s (attempt $retry_count/$STREAMING_MAX_RETRIES)"
        sleep $delay
      else
        echo "❌ Streaming batch $batch_id failed after $STREAMING_MAX_RETRIES retries"
        return 1
      fi
    fi
  done

  return 1
}

# Finalize streaming (flush remaining messages and cleanup)
finalize_streaming() {
  if [ "$STREAMING_ENABLED" = "true" ]; then
    echo "📡 Finalizing streaming, flushing remaining messages..."
    flush_streaming_buffer

    # Send completion message
    add_streaming_message "system" "Claude execution completed" "" "" "false" "info"
    flush_streaming_buffer

    echo "📡 Streaming finalized"
  fi

  # Cleanup
  rm -f "$STREAMING_BUFFER_FILE" 2>/dev/null || true
  STREAMING_PENDING_TOOLS=()  # Clear pending tools
}

# Parse Claude streaming JSON and add to buffer with concurrency control
parse_claude_stream_for_api() {
  local line="$1"

  if [ "$STREAMING_ENABLED" != "true" ] || [ -z "$line" ]; then
    return 0
  fi

  # Skip processing if we have too many pending operations
  local pending_count=${#STREAMING_PENDING_TOOLS[@]}
  if [ $pending_count -gt 10 ]; then
    echo "⚠️ Too many pending streaming operations ($pending_count), skipping message"
    return 0
  fi

  # Use jq for proper JSON parsing
  if command -v jq >/dev/null 2>&1; then
    local msg_type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)

    case "$msg_type" in
      "assistant")
        # Check for tool usage within message content - process sequentially
        echo "$line" | jq -c '.message.content[]? | select(.type == "tool_use")' 2>/dev/null | while read -r tool_use; do
          local tool_name=$(echo "$tool_use" | jq -r '.name // "unknown"')
          local tool_id=$(echo "$tool_use" | jq -r '.id // ""' 2>/dev/null)
          local tool_input=$(echo "$tool_use" | jq -r '.input | tostring' 2>/dev/null || echo "{}")
          local tool_desc=$(echo "$tool_use" | jq -r '.input.description // .input.file_path // .input.command // "action"' 2>/dev/null)

          # Track pending tool to prevent concurrency issues
          if [ -n "$tool_id" ]; then
            STREAMING_PENDING_TOOLS+=("$tool_id")
          fi

          add_streaming_message "tool_use" "$tool_name: $tool_desc" "$tool_name" "$tool_input" "false" "info"
          
          # Add small delay between tool processing
          sleep 0.1
        done

        # Check for text content within message
        echo "$line" | jq -c '.message.content[]? | select(.type == "text")' 2>/dev/null | while read -r text_content; do
          local text=$(echo "$text_content" | jq -r '.text // empty' 2>/dev/null)
          if [ -n "$text" ] && [ "$text" != "null" ]; then
            # Filter out system messages and overly verbose content
            if ! echo "$text" | grep -q "session_id\|cwd\|tools\|Perfect!\|Great!\|Excellent!"; then
              # Truncate long messages
              if [ ${#text} -gt 500 ]; then
                text="${text:0:500}..."
              fi
              add_streaming_message "text" "$text" "" "" "false" "info"
            fi
          fi
        done
        ;;

      "tool_result")
        local tool_call_id=$(echo "$line" | jq -r '.tool_call_id // ""' 2>/dev/null)
        local is_error=$(echo "$line" | jq -r '.is_error // false' 2>/dev/null)
        local result_content=$(echo "$line" | jq -r '.tool_use_result // .content // "Result"' 2>/dev/null)

        # Remove from pending tools if we have the ID
        if [ -n "$tool_call_id" ]; then
          local new_pending=()
          for pending_id in "${STREAMING_PENDING_TOOLS[@]}"; do
            if [ "$pending_id" != "$tool_call_id" ]; then
              new_pending+=("$pending_id")
            fi
          done
          STREAMING_PENDING_TOOLS=("${new_pending[@]}")
        fi

        if [ "$is_error" = "true" ]; then
          # Truncate error messages
          if [ ${#result_content} -gt 500 ]; then
            result_content="${result_content:0:500}..."
          fi
          add_streaming_message "error" "$result_content" "" "" "true" "error"
        elif [ ${#result_content} -lt 100 ]; then
          # Only send short success messages
          add_streaming_message "tool_result" "$result_content" "" "" "false" "info"
        fi
        ;;

      "system")
        local subtype=$(echo "$line" | jq -r '.subtype // empty' 2>/dev/null)
        if [ "$subtype" = "init" ]; then
          add_streaming_message "system" "Claude session initialized" "" "" "false" "info"
        fi
        ;;
    esac
  fi
}
