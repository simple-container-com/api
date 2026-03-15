#!/bin/bash

# Report workflow failure script
# Reports workflow failures to the service with detailed error information

set -e

JOB_ID="$1"
SERVICE_URL="$2"
API_KEY="$3"
WORKFLOW_RUN_ID="$4"
WORKFLOW_URL="$5"
CONTEXT_OUTCOME="$6"
PREPARE_CLAUDE_OUTCOME="$7"
CLAUDE_RESPONSE_OUTCOME="$8"
PROCESS_RESPONSE_OUTCOME="$9"

if [ -z "$JOB_ID" ] || [ -z "$SERVICE_URL" ] || [ -z "$API_KEY" ] || [ -z "$WORKFLOW_RUN_ID" ] || [ -z "$WORKFLOW_URL" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <service_url> <api_key> <workflow_run_id> <workflow_url> [step_outcomes...]"
  echo "Received parameters:"
  echo "  JOB_ID: '$JOB_ID'"
  echo "  SERVICE_URL: '$SERVICE_URL'"
  echo "  API_KEY: '$([ -n "$API_KEY" ] && echo "[SET]" || echo "[EMPTY]")'"
  echo "  WORKFLOW_RUN_ID: '$WORKFLOW_RUN_ID'"
  echo "  WORKFLOW_URL: '$WORKFLOW_URL'"
  exit 1
fi

echo "Reporting workflow failure to service"

# Determine which step failed and create appropriate error message
FAILED_STEP="Unknown"
ERROR_MESSAGE="Workflow failed"

# Check specific failure scenarios
if [ "$CONTEXT_OUTCOME" = "failure" ]; then
  FAILED_STEP="Fetch context from service"
  ERROR_MESSAGE="Failed to fetch job context from service. This could be due to authentication issues or the job not being found."
elif [ "$PREPARE_CLAUDE_OUTCOME" = "failure" ]; then
  FAILED_STEP="Prepare Claude conversation"
  ERROR_MESSAGE="Failed to prepare Claude conversation context from the fetched job data."
elif [ "$CLAUDE_RESPONSE_OUTCOME" = "failure" ]; then
  FAILED_STEP="Run Claude with context"
  # Check if it was a timeout or other failure
  if grep -q "timed out after 20 minutes" claude_error.txt 2>/dev/null; then
    ERROR_MESSAGE="Claude Code execution timed out after 20 minutes. This could be due to API rate limits, slow responses, network issues, or Claude service being unavailable. Try again later or contact support if the issue persists."
  else
    ERROR_MESSAGE="Claude Code execution failed. This could be due to installation issues, API limits, authentication problems, or invalid prompts. Check the workflow logs for detailed error information."
  fi
elif [ "$PROCESS_RESPONSE_OUTCOME" = "failure" ]; then
  FAILED_STEP="Process Claude response and make changes"
  ERROR_MESSAGE="Failed to process Claude's response and generate code changes."
else
  FAILED_STEP="General workflow failure"
  ERROR_MESSAGE="The workflow failed at an unexpected step. Check the workflow logs for more details."
fi

echo "Failed step: $FAILED_STEP"
echo "Error message: $ERROR_MESSAGE"

# Report failure to service with HTTP status check
http_code=$(curl -s -w "%{http_code}" -X POST \
  "$SERVICE_URL/api/jobs/$JOB_ID/workflow-failed" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"workflowRunId\": $WORKFLOW_RUN_ID,
    \"workflowUrl\": \"$WORKFLOW_URL\",
    \"failedAt\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
    \"error\": \"$ERROR_MESSAGE\",
    \"step\": \"$FAILED_STEP\"
  }" \
  -o failure_response.json)

echo "Failure report HTTP Status: $http_code"

if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
  echo "✅ Workflow failure reported successfully"
else
  echo "⚠️ Failed to report workflow failure (HTTP $http_code)"
  cat failure_response.json
  echo "⚠️ Failure notification may not reach the user"
fi
