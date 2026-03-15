#!/bin/bash

# Process Claude response script
# Creates summary file from Claude's response

set -e

ISSUE_ID="$1"
BRANCH="$2"
WORKFLOW_RUN_ID="$3"
WORKFLOW_URL="$4"

if [ -z "$ISSUE_ID" ] || [ -z "$BRANCH" ] || [ -z "$WORKFLOW_RUN_ID" ] || [ -z "$WORKFLOW_URL" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <issue_id> <branch> <workflow_run_id> <workflow_url>"
  exit 1
fi

echo "Processing Claude response and making changes"

# Verify file system changes after Claude execution
echo "🔍 Checking for file system changes after Claude execution..."
echo "Git status before processing:"
git status --porcelain || echo "Git status failed"

# Check if any files were modified
MODIFIED_FILES=$(git status --porcelain | wc -l)
echo "Number of modified files detected by git: $MODIFIED_FILES"

# Stage all changes first to get accurate detection
echo "📁 Staging all changes for detection..."
git add -A 2>/dev/null || echo "Warning: git add -A failed"

# Check staged changes
STAGED_FILES=$(git diff --cached --name-only | wc -l)
echo "Number of staged changes: $STAGED_FILES"

# Unstage temporary files before proceeding
git reset HEAD .simple-forge/ 2>/dev/null || true
git reset HEAD .forge-tmp/ 2>/dev/null || true
git reset HEAD .forge-workspace/ 2>/dev/null || true
git reset HEAD tracking_response.json 2>/dev/null || true
git reset HEAD failure_response.json 2>/dev/null || true
git reset HEAD .claude_temp/ 2>/dev/null || true
git reset HEAD claude_error.txt 2>/dev/null || true
git reset HEAD claude_response.txt 2>/dev/null || true
git reset HEAD .claude-plugins.json 2>/dev/null || true

# Re-check what's left staged
REMAINING_STAGED=$(git diff --cached --name-only | wc -l)
echo "Number of non-temp staged changes: $REMAINING_STAGED"

if [ "$REMAINING_STAGED" -gt 0 ]; then
  echo "✅ Claude successfully made file system changes:"
  git diff --cached --name-status | head -20
  echo ""
  echo "📄 Files ready to commit:"
  git diff --cached --name-only | head -20
else
  echo "⚠️ No file system changes detected by git (after excluding temp files)"
  echo "This suggests Claude Code's file editing may not be working properly"
  echo "Checking for any new or modified files in working directory..."

  # Check for recently modified files
  echo "Recently modified files (last 5 minutes):"
  find . -type f -mmin -5 -not -path './.git/*' -not -path './.forge-tmp/*' -not -path './.forge-workspace/*' | head -10 || echo "No recently modified files found"

  # Check for untracked files
  echo "🔍 Untracked files (excluding temp directories):"
  git ls-files --others --exclude-standard | grep -v ".simple-forge/\|.forge-tmp/\|.forge-workspace/" | head -10 || echo "No untracked files"
fi

# Check if Claude response file exists in temp directory
CLAUDE_RESPONSE_FILE=".forge-tmp/claude_response.txt"
SUMMARY_FILE=".forge-tmp/CLAUDE_RESPONSE.md"

if [ ! -f "$CLAUDE_RESPONSE_FILE" ]; then
  echo "⚠️ Claude response file not found: $CLAUDE_RESPONSE_FILE"
  echo "This is expected when using Claude interactive mode - Claude modifies files directly"
  echo "No summary file needed as changes are applied directly to project files"
  exit 0
fi

# Create a summary file in temp directory
echo "# Code Generation Summary" > "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "**Workflow Run**: [$WORKFLOW_RUN_ID]($WORKFLOW_URL)" >> "$SUMMARY_FILE"
echo "**Issue**: #$ISSUE_ID" >> "$SUMMARY_FILE"
echo "**Branch**: $BRANCH" >> "$SUMMARY_FILE"
echo "**Timestamp**: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "## Claude Response" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
cat "$CLAUDE_RESPONSE_FILE" >> "$SUMMARY_FILE"

# In a real implementation, this would parse Claude's response
# and make actual code changes. For now, we'll just commit the response.

echo "Changes processed and ready for commit"
