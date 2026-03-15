#!/bin/bash

# Initialize workflow tracking script
# Reports workflow start to the service

set -e

JOB_ID="$1"
SERVICE_URL="$2"
WORKFLOW_RUN_ID="$3"
WORKFLOW_URL="$4"
API_KEY="$5"

if [ -z "$JOB_ID" ] || [ -z "$SERVICE_URL" ] || [ -z "$WORKFLOW_RUN_ID" ] || [ -z "$WORKFLOW_URL" ] || [ -z "$API_KEY" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <service_url> <workflow_run_id> <workflow_url> <api_key>"
  echo "Received parameters:"
  echo "  JOB_ID: '$JOB_ID'"
  echo "  SERVICE_URL: '$SERVICE_URL'"
  echo "  WORKFLOW_RUN_ID: '$WORKFLOW_RUN_ID'"
  echo "  WORKFLOW_URL: '$WORKFLOW_URL'"
  echo "  API_KEY: '$([ -n "$API_KEY" ] && echo "[SET]" || echo "[EMPTY]")'"
  exit 1
fi

echo "Starting workflow for job $JOB_ID"

# Initialize workflow tracking with HTTP status check
http_code=$(curl -s -w "%{http_code}" -X POST \
  "$SERVICE_URL/api/jobs/$JOB_ID/workflow-started" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"workflowRunId\": $WORKFLOW_RUN_ID,
    \"workflowUrl\": \"$WORKFLOW_URL\",
    \"startedAt\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
  }" \
  -o tracking_response.json)

echo "Workflow tracking HTTP Status: $http_code"

if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
  echo "✅ Workflow tracking initialized successfully"
else
  echo "⚠️ Failed to initialize workflow tracking (HTTP $http_code)"
  cat tracking_response.json
  
  if [[ "$http_code" == "401" ]] || [[ "$http_code" == "403" ]]; then
    echo "❌ Authentication failed for workflow tracking"
    echo "Please check the API key is configured correctly"
    exit 1
  else
    echo "⚠️ Workflow tracking failed but continuing with job execution"
  fi
fi
