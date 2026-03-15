#!/bin/bash

# Job completion summary script
# Provides final summary of job execution status

set -e

JOB_ID="$1"
ISSUE_ID="$2"
BRANCH="$3"
JOB_STATUS="$4"
WORKFLOW_URL="$5"

if [ -z "$JOB_ID" ] || [ -z "$ISSUE_ID" ] || [ -z "$BRANCH" ] || [ -z "$JOB_STATUS" ] || [ -z "$WORKFLOW_URL" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <job_id> <issue_id> <branch> <job_status> <workflow_url>"
  exit 1
fi

echo "## Job Completion Summary"
echo "Job ID: $JOB_ID"
echo "Issue ID: $ISSUE_ID"
echo "Branch: $BRANCH"
echo "Status: $JOB_STATUS"
echo "Workflow URL: $WORKFLOW_URL"

if [ "$JOB_STATUS" = "success" ]; then
  echo "✅ Code generation completed successfully"
else
  echo "❌ Code generation failed or was cancelled"
  echo "Failure should have been reported to the service automatically"
fi
