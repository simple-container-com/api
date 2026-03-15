#!/bin/bash

# Prepare Claude conversation script
# Creates conversation file from context and repository information

set -e

ISSUE_ID="$1"
REPOSITORY="$2"

if [ -z "$ISSUE_ID" ] || [ -z "$REPOSITORY" ]; then
  echo "❌ Missing required parameters"
  echo "Usage: $0 <issue_id> <repository>"
  exit 1
fi

echo "Preparing Claude conversation"

# Create temp directory and extract context from JSON
mkdir -p .forge-tmp
system_prompt=$(cat .forge-tmp/context.json | jq -r '.systemPrompt // "You are a helpful code assistant working on GitHub issues."')

# Create conversation file
echo "System: $system_prompt" > .forge-tmp/conversation.txt
echo "" >> .forge-tmp/conversation.txt

# Add messages from context
cat .forge-tmp/context.json | jq -r '.messages[] | "\(.role | ascii_upcase): \(.content)"' >> .forge-tmp/conversation.txt

# Add job-specific user prompt only for sequential jobs (to avoid duplication with normal interactions)
echo "" >> .forge-tmp/conversation.txt
user_prompt=$(cat .forge-tmp/context.json | jq -r '.userPrompt // empty')
is_sequential=$(cat .forge-tmp/context.json | jq -r '.isSequential // false')
if [ -n "$user_prompt" ] && [ "$user_prompt" != "null" ] && [ "$is_sequential" = "true" ]; then
  # This is a sequential job with scope constraints - use the UserPrompt
  echo "User: $user_prompt" >> .forge-tmp/conversation.txt
else
  # Normal job - use generic message (user interaction already in conversation history)
  echo "User: I need help with issue #$ISSUE_ID in repository $REPOSITORY." >> .forge-tmp/conversation.txt
  echo "Please analyze the codebase and provide a solution. Here's the current repository structure:" >> .forge-tmp/conversation.txt
  echo "" >> .forge-tmp/conversation.txt
fi

# Add repository structure (limited to avoid token limits)
find . -type f -name "*.go" -o -name "*.py" -o -name "*.js" -o -name "*.ts" -o -name "*.java" -o -name "*.md" -o -name "*.yml" -o -name "*.yaml" | head -50 | sort >> .forge-tmp/conversation.txt

echo "Conversation file prepared:"
head -20 .forge-tmp/conversation.txt
