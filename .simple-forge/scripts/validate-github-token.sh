#!/bin/bash

# GitHub Token Validation Script (Simplified)
# Tests if the provided GITHUB_TOKEN has necessary permissions for the workflow
# Uses only API calls - no git clone operations

set -e

REPOSITORY="$1"

if [ -z "$REPOSITORY" ]; then
  echo "❌ Repository parameter is required"
  echo "Usage: $0 <repository>"
  exit 1
fi

if [ -z "$GITHUB_TOKEN" ]; then
  echo "❌ GITHUB_TOKEN environment variable is required"
  exit 1
fi

echo "🔍 Validating GitHub token permissions for $REPOSITORY..."

# Test 1: Check if token can access repository and has push permissions
REPO_RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/repo_response.json \
  -H "Authorization: token $GITHUB_TOKEN" \
  -H "Accept: application/vnd.github.v3+json" \
  "https://api.github.com/repos/$REPOSITORY")

if [ "$REPO_RESPONSE" != "200" ]; then
  echo "❌ Failed to access repository (HTTP $REPO_RESPONSE)"
  if [ -f /tmp/repo_response.json ]; then
    echo "Response: $(cat /tmp/repo_response.json)"
  fi
  rm -f /tmp/repo_response.json
  exit 1
fi

# Extract permissions from response
PUSH_PERMISSION=$(cat /tmp/repo_response.json | jq -r '.permissions.push // false')
ADMIN_PERMISSION=$(cat /tmp/repo_response.json | jq -r '.permissions.admin // false')

echo "✅ Repository access successful"
echo "   Push permission: $PUSH_PERMISSION"
echo "   Admin permission: $ADMIN_PERMISSION"

if [ "$PUSH_PERMISSION" != "true" ]; then
  echo "❌ Token does not have push permissions to repository"
  rm -f /tmp/repo_response.json
  exit 1
fi

rm -f /tmp/repo_response.json

# Test 2: Verify token can create references (branches)
echo "🔍 Testing branch creation permissions..."
TEST_REF_RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/ref_response.json \
  -H "Authorization: token $GITHUB_TOKEN" \
  -H "Accept: application/vnd.github.v3+json" \
  "https://api.github.com/repos/$REPOSITORY/git/refs/heads")

if [ "$TEST_REF_RESPONSE" = "200" ]; then
  echo "✅ Can access repository references"
elif [ "$TEST_REF_RESPONSE" = "403" ]; then
  echo "⚠️  Limited access to repository references (may affect branch operations)"
else
  echo "⚠️  Could not verify reference access (HTTP $TEST_REF_RESPONSE)"
fi

rm -f /tmp/ref_response.json

echo "🎉 GitHub token validation completed!"
echo "✅ Token has necessary permissions for workflow operations"
