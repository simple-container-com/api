#!/bin/bash

# Upload handoff file script
# Uploads .forge-workspace/handoff.json file via the API endpoint

set -e

# Required parameters
JOB_ID="$1"
SERVICE_URL="$2"
API_KEY="$3"

# Constants
PRIMARY_HANDOFF_FILE=".forge-workspace/handoff.json"
MAX_FILE_SIZE=102400  # 100KB in bytes

# Role-specific docs folders where handoff.json may be committed
DOCS_HANDOFF_DIRS=("docs/product-manager" "docs/design" "docs/implementation" "docs/testing" "docs/review")

# Check if all required parameters are provided
if [ -z "$JOB_ID" ] || [ -z "$SERVICE_URL" ] || [ -z "$API_KEY" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <service_url> <api_key>"
  exit 1
fi

echo "📤 Uploading handoff file for job $JOB_ID"

# Find handoff file: primary location first, then search docs folders
HANDOFF_FILE=""
HANDOFF_SOURCE=""

if [ -f "$PRIMARY_HANDOFF_FILE" ]; then
  HANDOFF_FILE="$PRIMARY_HANDOFF_FILE"
  HANDOFF_SOURCE="workspace"
  echo "✓ Found handoff file at primary location: $HANDOFF_FILE"
else
  echo "ℹ️  Primary handoff file not found at $PRIMARY_HANDOFF_FILE"
  echo "🔍 Searching role-specific docs folders for committed handoff.json..."
  # Search for the most recently modified handoff.json in docs folders
  for dir in "${DOCS_HANDOFF_DIRS[@]}"; do
    if [ -d "$dir" ]; then
      # Find handoff.json files, sorted by modification time (newest first)
      FOUND=$(find "$dir" -name "handoff.json" -type f -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)
      if [ -n "$FOUND" ] && [ -f "$FOUND" ]; then
        HANDOFF_FILE="$FOUND"
        HANDOFF_SOURCE="committed"
        echo "✓ Found committed handoff file: $HANDOFF_FILE"
        break
      fi
    fi
  done
fi

if [ -z "$HANDOFF_FILE" ]; then
  echo "ℹ️  No handoff file found in any location"
  echo "No handoff to upload - exiting gracefully"
  exit 0
fi

# Validate file size
FILE_SIZE=$(stat -c%s "$HANDOFF_FILE" 2>/dev/null || stat -f%z "$HANDOFF_FILE" 2>/dev/null || echo "0")
if [ "$FILE_SIZE" -eq 0 ]; then
  echo "❌ Unable to determine file size"
  exit 1
fi

if [ "$FILE_SIZE" -gt "$MAX_FILE_SIZE" ]; then
  echo "❌ File size exceeds maximum allowed size of 100KB"
  echo "Current size: $FILE_SIZE bytes"
  exit 1
fi

echo "✓ File size validated: $FILE_SIZE bytes"

# Validate JSON syntax
if ! jq empty "$HANDOFF_FILE" 2>/dev/null; then
  echo "❌ Invalid JSON syntax in handoff file"
  exit 1
fi

echo "✓ JSON syntax validated"

# Read and escape JSON payload
# Use jq to properly escape the JSON for safe inclusion in the request body
PAYLOAD=$(cat "$HANDOFF_FILE" | jq -c .)

# Construct the request JSON
REQUEST_JSON=$(jq -n \
  --arg payload "$PAYLOAD" \
  --arg source "file" \
  '{payload: $payload, source: $source}')

# Upload via API endpoint
API_ENDPOINT="$SERVICE_URL/api/jobs/$JOB_ID/handoff"

echo "📡 Uploading to $API_ENDPOINT"

# Create temp directory for response
mkdir -p .forge-tmp

# Make the API request and capture HTTP status code
HTTP_CODE=$(curl -s -w "%{http_code}" -X POST \
  "$API_ENDPOINT" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_JSON" \
  -o .forge-tmp/handoff_response.json)

echo "HTTP Status Code: $HTTP_CODE"

# Parse response and check for success
if [ "$HTTP_CODE" -eq 200 ]; then
  # Check if response contains status: "valid"
  if command -v jq &> /dev/null; then
    STATUS=$(jq -r '.status' .forge-tmp/handoff_response.json 2>/dev/null || echo "")
    ERROR=$(jq -r '.error' .forge-tmp/handoff_response.json 2>/dev/null || echo "")
    if [ "$STATUS" = "valid" ] && [ -z "$ERROR" ]; then
      echo "✓ Handoff uploaded successfully"

      # Display handoff info if available
      HANDOFF_ID=$(jq -r '.handoffId' .forge-tmp/handoff_response.json 2>/dev/null || echo "")
      ROLE=$(jq -r '.role' .forge-tmp/handoff_response.json 2>/dev/null || echo "")
      if [ -n "$HANDOFF_ID" ]; then
        echo "Handoff ID: $HANDOFF_ID (role: $ROLE)"
      fi

      # Clean up workspace file on success (don't delete committed docs files)
      if [ "$HANDOFF_SOURCE" = "workspace" ]; then
        rm "$HANDOFF_FILE"
        echo "✓ Workspace handoff file cleaned up after successful upload"
      else
        echo "✓ Committed handoff file preserved at $HANDOFF_FILE"
      fi

      # Clean up response
      rm .forge-tmp/handoff_response.json

      exit 0
    else
      echo "❌ Upload failed: $ERROR"
      cat .forge-tmp/handoff_response.json
      rm .forge-tmp/handoff_response.json
      exit 1
    fi
  else
    # If jq is not available, assume success on 200
    echo "✓ Handoff uploaded successfully (jq not available for detailed response parsing)"
    if [ "$HANDOFF_SOURCE" = "workspace" ]; then
      rm "$HANDOFF_FILE"
    fi
    rm .forge-tmp/handoff_response.json
    exit 0
  fi
else
  echo "❌ Upload failed with HTTP code $HTTP_CODE"

  # Display error response
  if [ -f .forge-tmp/handoff_response.json ]; then
    if command -v jq &> /dev/null; then
      ERROR=$(jq -r '.error' .forge-tmp/handoff_response.json 2>/dev/null || echo "Unknown error")
      echo "Error: $ERROR"
    else
      cat .forge-tmp/handoff_response.json
    fi
    rm .forge-tmp/handoff_response.json
  fi

  # Handle specific error codes
  case "$HTTP_CODE" in
    400)
      echo "❌ Bad request - validation failed"
      exit 1
      ;;
    404)
      echo "❌ Not found - job does not exist"
      exit 1
      ;;
    500)
      echo "❌ Internal server error"
      exit 1
      ;;
    *)
      echo "❌ Unexpected error"
      exit 1
      ;;
  esac
fi
