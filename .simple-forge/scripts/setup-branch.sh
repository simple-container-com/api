#!/bin/bash

# Setup and checkout branch script
# Creates or checks out the specified branch for the workflow

set -e

BRANCH_NAME="$1"

if [ -z "$BRANCH_NAME" ]; then
  echo "❌ Branch name is required"
  exit 1
fi

echo "Setting up branch: $BRANCH_NAME"

# For QA Engineer role, fetch issue details to get Developer's branch
# QA Engineer uses same-branch workflow (works in Developer's branch)
if [ -n "$ISSUE_ID" ] && [ -n "$SERVICE_URL" ] && [ -n "$SIMPLE_FORGE_API_KEY" ]; then
  echo "Fetching issue details to check role..."

  # Fetch issue details from service API
  ISSUE_RESPONSE=$(curl -s -X GET \
    "$SERVICE_URL/api/issues/$ISSUE_ID" \
    -H "Authorization: Bearer $SIMPLE_FORGE_API_KEY" \
    -H "Accept: application/json")

  # Extract role from response
  ROLE=$(echo "$ISSUE_RESPONSE" | jq -r '.role // empty')

  if [ "$ROLE" = "qa_engineer" ]; then
    echo "✅ Detected QA Engineer role - using same-branch workflow"

    # Extract Developer's branch from baseBranch field first
    DEVELOPER_BRANCH=$(echo "$ISSUE_RESPONSE" | jq -r '.baseBranch // empty')
    
    # If baseBranch is empty, try to extract from issue body (for manually created QA issues)
    if [ -z "$DEVELOPER_BRANCH" ]; then
      echo "⚠️  baseBranch field is empty, attempting to extract developer branch from issue body"
      ISSUE_BODY=$(echo "$ISSUE_RESPONSE" | jq -r '.body // empty')
      
      # Extract developer branch from issue body (format: "**Developer Branch:** `branch-name`")
      DEVELOPER_BRANCH=$(echo "$ISSUE_BODY" | grep -o '\*\*Developer Branch:\*\*[^`]*`[^`]*`' | sed 's/.*`\([^`]*\)`.*/\1/' | head -1)
      
      if [ -n "$DEVELOPER_BRANCH" ]; then
        echo "✅ Found developer branch in issue body: $DEVELOPER_BRANCH"
      else
        # Final fallback to branch field (for handoff-created issues)
        DEVELOPER_BRANCH=$(echo "$ISSUE_RESPONSE" | jq -r '.branch // empty')
        echo "⚠️  Could not extract from issue body, using branch field instead: $DEVELOPER_BRANCH"
      fi
    fi

    if [ -z "$DEVELOPER_BRANCH" ]; then
      echo "❌ Error: QA Engineer role detected but could not determine Developer's branch"
      echo "Tried: baseBranch field, issue body extraction, and branch field"
      echo "Issue response: $ISSUE_RESPONSE"
      exit 1
    fi

    echo "Developer's branch: $DEVELOPER_BRANCH"

    # Override BRANCH_NAME to use Developer's branch
    BRANCH_NAME="$DEVELOPER_BRANCH"
    echo "Using Developer's branch: $BRANCH_NAME"
  fi
fi

# Check if we're in a git repository, if not, clone it
if ! git rev-parse --git-dir > /dev/null 2>&1; then
  echo "No git repository found, cloning repository..."
  
  # Extract repository URL from GITHUB_REPOSITORY environment variable
  if [ -z "$GITHUB_REPOSITORY" ]; then
    echo "❌ GITHUB_REPOSITORY environment variable is required"
    exit 1
  fi
  
  if [ -z "$GITHUB_TOKEN" ]; then
    echo "❌ GITHUB_TOKEN environment variable is required"
    exit 1
  fi
  
  # Clone the repository
  REPO_URL="https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git"
  echo "Cloning repository: $GITHUB_REPOSITORY"
  
  # Debug: Show current directory contents and permissions
  echo "Debug: Current directory: $(pwd)"
  echo "Debug: Directory contents:"
  ls -la . || echo "Failed to list directory"
  echo "Debug: Directory permissions:"
  ls -ld . || echo "Failed to check directory permissions"
  
  # Always use temporary directory approach to avoid issues
  echo "Using temporary directory approach for reliability"
  TEMP_DIR=$(mktemp -d)
  echo "Debug: Temporary directory: $TEMP_DIR"
  
  # Clone to temporary directory with better error handling
  if ! git clone "$REPO_URL" "$TEMP_DIR"; then
    echo "❌ Failed to clone repository to temporary directory"
    echo "Debug: Git clone exit code: $?"
    rm -rf "$TEMP_DIR" 2>/dev/null || true
    exit 1
  fi
  
  echo "✅ Successfully cloned to temporary directory"
  
  # Move contents from temp directory to current directory
  echo "Moving repository contents to current directory..."
  
  # Copy .git directory first (most important)
  if [ -d "$TEMP_DIR/.git" ]; then
    cp -r "$TEMP_DIR/.git" . || {
      echo "❌ Failed to copy .git directory"
      rm -rf "$TEMP_DIR" 2>/dev/null || true
      exit 1
    }
    echo "✅ Copied .git directory"
  fi
  
  # Copy all other files and directories
  find "$TEMP_DIR" -mindepth 1 -maxdepth 1 ! -name '.git' -exec cp -r {} . \; 2>/dev/null || true

  # Fix ownership and permissions of copied files
  echo "Fixing ownership and permissions..."
  echo "Debug: Current user: $(id)"
  echo "Debug: .git directory ownership before fix:"
  ls -ld .git 2>/dev/null || echo ".git directory not found"

  # Fix permissions for .git directory (ownership should already be correct for non-root user)
  if [ -d ".git" ]; then
    # Check if we're running as non-root user
    if [ "$(id -u)" -ne 0 ]; then
      echo "Running as non-root user, ensuring permissions are correct..."
      # For non-root user, just ensure we have write permissions
      chmod -R u+w .git 2>/dev/null || echo "Warning: Could not fix permissions"
    else
      # Running as root, try to change ownership to current user
      chown -R $(id -u):$(id -g) .git 2>/dev/null || {
        echo "Warning: Could not change ownership of .git directory"
        echo "Debug: Trying to fix permissions instead..."
        chmod -R u+w .git 2>/dev/null || echo "Warning: Could not fix permissions"
      }
    fi
    echo "Debug: .git directory ownership after fix:"
    ls -ld .git 2>/dev/null || echo ".git directory not found"
  fi
  
  # Clean up temp directory
  rm -rf "$TEMP_DIR"
  echo "✅ Cleaned up temporary directory"
  
  # Fix git dubious ownership issue
  echo "Configuring git safe directory..."
  git config --global --add safe.directory /github/workspace || {
    echo "Warning: Could not configure safe directory"
  }
  
  # Verify git repository is properly set up with more debugging
  echo "Debug: Testing git repository setup..."
  if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "❌ Failed to set up git repository after cloning"
    echo "Debug: Git error output:"
    git rev-parse --git-dir 2>&1 || echo "Git command failed"
    echo "Debug: Checking if .git exists:"
    ls -la .git 2>/dev/null || echo ".git directory not found"
    echo "Debug: Git config test:"
    git config --list 2>&1 || echo "Git config failed"
    exit 1
  fi
  
  echo "✅ Repository cloned successfully"
fi

# Configure git
git config --local user.email "action@github.com"
git config --local user.name "GitHub Action"

# Configure git to use the token for authentication
git config --local url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"

# Check if branch exists remotely
if git ls-remote --heads origin "$BRANCH_NAME" | grep -q "$BRANCH_NAME"; then
  echo "Branch $BRANCH_NAME exists remotely, checking out and updating"
  git checkout "$BRANCH_NAME"
  git pull origin "$BRANCH_NAME"
  echo "✅ Branch updated to latest remote version"
else
  # For QA Engineer, the Developer's branch MUST exist
  if [ "$ROLE" = "qa_engineer" ]; then
    echo "❌ Error: QA Engineer role detected but Developer's branch '$BRANCH_NAME' does not exist remotely"
    echo "QA Engineer requires the Developer's branch to be available"
    exit 1
  fi

  echo "Branch $BRANCH_NAME does not exist, creating new branch from main"
  git checkout -b "$BRANCH_NAME"
fi

echo "✅ Branch setup completed"
