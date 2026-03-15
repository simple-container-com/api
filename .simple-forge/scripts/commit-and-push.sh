#!/bin/bash

# Commit and push changes script
# Commits and pushes changes to the specified branch
# Made optional to prevent workflow failures

# Don't exit on errors - make commit optional
set +e

ISSUE_ID="$1"
JOB_ID="$2"
BRANCH="$3"
WORKFLOW_URL="$4"

if [ -z "$ISSUE_ID" ] || [ -z "$JOB_ID" ] || [ -z "$BRANCH" ] || [ -z "$WORKFLOW_URL" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <issue_id> <job_id> <branch> <workflow_url>"
  exit 1
fi

echo "🔄 Attempting to commit and push changes (optional step)..."

# Get the actual current branch (important for QA Engineer workflow where we work in Developer's branch)
ACTUAL_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "$BRANCH")
echo "📍 Current branch: $ACTUAL_BRANCH"
echo "📍 Target branch from workflow: $BRANCH"
if [ "$ACTUAL_BRANCH" != "$BRANCH" ]; then
  echo "ℹ️  Note: Working in different branch than workflow target (likely QA Engineer workflow)"
fi

# Configure git authentication for push operations
if [ -n "$GITHUB_TOKEN" ]; then
  echo "🔐 Configuring git authentication for push operations..."
  git config --local url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
  echo "✅ Git authentication configured"
else
  echo "⚠️ Warning: GITHUB_TOKEN not found, push operations may fail"
fi

# Detect role for commit message formatting
ROLE=""
if [ -n "$ISSUE_ID" ] && [ -n "$SERVICE_URL" ] && [ -n "$SIMPLE_FORGE_API_KEY" ]; then
  echo "Fetching issue details to check role..."

  # Fetch issue details from service API
  ISSUE_RESPONSE=$(curl -s -X GET \
    "$SERVICE_URL/api/issues/$ISSUE_ID" \
    -H "Authorization: Bearer $SIMPLE_FORGE_API_KEY" \
    -H "Accept: application/json")

  # Extract role from response
  ROLE=$(echo "$ISSUE_RESPONSE" | jq -r '.role // empty')

  if [ -n "$ROLE" ]; then
    echo "Detected role: $ROLE"
  fi
fi

# Add changes with error handling (excluding temporary directories)
echo "📁 Adding changes to git..."

# First, add all changes including untracked files
if ! git add -A; then
  echo "⚠️ Warning: Failed to add changes to git, but continuing workflow"
  exit 0
fi

# Add untracked files explicitly (in case git add -A missed them)
echo "🔍 Checking for untracked files..."
UNTRACKED_FILES=$(git ls-files --others --exclude-standard | grep -v ".simple-forge/\|.forge-tmp/\|tracking_response.json\|failure_response.json\|.claude_temp/\|claude_error.txt\|claude_response.txt\|.claude-plugins.json" || true)
if [ -n "$UNTRACKED_FILES" ]; then
  echo "Found untracked files, adding them:"
  echo "$UNTRACKED_FILES" | head -10
  echo "$UNTRACKED_FILES" | while read -r file; do
    git add -f "$file" 2>/dev/null || echo "  ⚠️ Failed to add: $file"
  done
fi

# Then remove temporary directories and files that shouldn't be committed
echo "🧹 Excluding temporary files and directories..."
git reset HEAD .simple-forge/ 2>/dev/null || true
git reset HEAD .forge-tmp/ 2>/dev/null || true
git reset HEAD tracking_response.json 2>/dev/null || true
git reset HEAD failure_response.json 2>/dev/null || true
git reset HEAD .claude_temp/ 2>/dev/null || true
git reset HEAD claude_error.txt 2>/dev/null || true
git reset HEAD claude_response.txt 2>/dev/null || true
git reset HEAD .claude-plugins.json 2>/dev/null || true
git reset HEAD .forge-workspace/ 2>/dev/null || true

# Show what will actually be committed
echo "📋 Files to be committed:"
git diff --cached --name-only | head -20

# Debug: Show git status if no files found
if git diff --cached --quiet; then
  echo "🔍 Debug: No staged changes found. Full git status:"
  git status --short || echo "Git status failed"
  echo "🔍 Debug: Untracked files (excluding temp dirs):"
  git ls-files --others --exclude-standard | grep -v ".simple-forge/\|.forge-tmp/\|.forge-workspace/" | head -10 || echo "No untracked files"
fi

# Check if there are changes to commit
if git diff --cached --quiet; then
  echo "No uncommitted changes found"

  # Check if there are unpushed commits using the ACTUAL current branch
  # First check if the remote branch exists
  if git ls-remote --exit-code --heads origin "$ACTUAL_BRANCH" >/dev/null 2>&1; then
    UNPUSHED_COMMITS=$(git log origin/"$ACTUAL_BRANCH".."$ACTUAL_BRANCH" --oneline 2>/dev/null | wc -l)
    if [ "$UNPUSHED_COMMITS" -gt 0 ]; then
      echo "Found $UNPUSHED_COMMITS unpushed commit(s) on $ACTUAL_BRANCH, pushing to remote..."
      if git push origin "$ACTUAL_BRANCH"; then
        echo "✅ Unpushed commits pushed successfully to $ACTUAL_BRANCH"
      else
        echo "⚠️ Warning: Failed to push unpushed commits, but continuing workflow"
      fi
    else
      echo "No unpushed commits found on $ACTUAL_BRANCH"
    fi
  else
    # Remote branch doesn't exist, check if we have local commits to push
    LOCAL_COMMITS=$(git log --oneline | wc -l)
    if [ "$LOCAL_COMMITS" -gt 0 ]; then
      echo "Remote branch $ACTUAL_BRANCH doesn't exist, pushing local branch..."
      if git push origin "$ACTUAL_BRANCH"; then
        echo "✅ Local branch pushed successfully as $ACTUAL_BRANCH"
      else
        echo "⚠️ Warning: Failed to push local branch, but continuing workflow"
      fi
    else
      echo "No commits to push"
    fi
  fi
else
  # Commit changes with error handling
  echo "💾 Creating commit..."

  # Determine commit message based on role
  if [ "$ROLE" = "qa_engineer" ]; then
    # QA Engineer commit message with 🧪 emoji
    COMMIT_MESSAGE="🧪 QA fixes for issue #$ISSUE_ID

Test validation and fixes by simple-forge QA Engineer
Job ID: $JOB_ID
Workflow: $WORKFLOW_URL"
  else
    # Default commit message with 🤖 emoji
    COMMIT_MESSAGE="🤖 Code generation for issue #$ISSUE_ID

Generated by simple-forge workflow
Job ID: $JOB_ID
Workflow: $WORKFLOW_URL"
  fi

  if git commit -m "$COMMIT_MESSAGE"; then
    echo "✅ Changes committed successfully"

    # Push changes - use the ACTUAL current branch, not the workflow target branch
    # This is critical for QA Engineer workflow where we work in Developer's branch
    echo "📤 Pushing changes to remote branch: $ACTUAL_BRANCH..."
    if git push origin "$ACTUAL_BRANCH"; then
      echo "✅ Changes pushed successfully to $ACTUAL_BRANCH"
    else
      echo "⚠️ Warning: Failed to push changes to remote, but continuing workflow"
      echo "   Changes are committed locally and can be pushed manually"
    fi
  else
    echo "⚠️ Warning: Failed to commit changes, but continuing workflow"
    echo "   This may be due to no actual changes or git configuration issues"
  fi
fi

echo "🔄 Commit and push step completed (optional step - workflow continues regardless)"
