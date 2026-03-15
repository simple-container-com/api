#!/bin/bash

# Fetch context from service script
# Retrieves job context from the service API

set -e

JOB_ID="$1"
SERVICE_URL="$2"
API_KEY="$3"

if [ -z "$JOB_ID" ] || [ -z "$SERVICE_URL" ] || [ -z "$API_KEY" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <service_url> <api_key>"
  exit 1
fi

echo "Fetching context for job $JOB_ID"

# Create temp directory and fetch context with HTTP status code
mkdir -p .forge-tmp
http_code=$(curl -s -w "%{http_code}" -X GET \
  "$SERVICE_URL/api/jobs/$JOB_ID/context" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Accept: application/json" \
  -o .forge-tmp/context_response.json)

echo "HTTP Status Code: $http_code"

# Check for successful response (2xx status codes)
if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
  mv .forge-tmp/context_response.json .forge-tmp/context.json
  echo "Context fetched successfully"
  cat .forge-tmp/context.json | jq .
else
  echo "Failed to fetch context from service (HTTP $http_code)"
  cat .forge-tmp/context_response.json

  # Check for authentication errors
  if [[ "$http_code" == "401" ]] || [[ "$http_code" == "403" ]]; then
    echo "❌ Authentication failed - invalid or missing API key"
    echo "Please check the API key is configured correctly"
    exit 1
  elif [[ "$http_code" == "404" ]]; then
    echo "❌ Job not found - job may have been deleted or expired"
    exit 1
  else
    echo "⚠️ Service error - using fallback context"
    echo '{"systemPrompt": "You are a helpful code assistant.", "messages": [], "version": 1}' > .forge-tmp/context.json
  fi
fi
