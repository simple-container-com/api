#!/bin/bash
# Simple Container Deploy Client Stack - Complete Self-Contained Action
# Replaces 467+ lines of hardcoded workflow logic

set -euo pipefail

# Enable error trapping
trap 'handle_error $? $LINENO' ERR

handle_error() {
    local exit_code=$1
    local line_number=$2
    echo "❌ Error occurred on line $line_number with exit code $exit_code"
    /scripts/notifications/send-slack.sh "failure" "Deployment failed on line $line_number"
    /scripts/finalization/handle-cancellation.sh
    exit $exit_code
}

# Source common utilities
source /scripts/common/utils.sh
source /scripts/common/logging.sh

# Initialize logging
init_logging "deploy-client-stack"

log_phase "INITIALIZATION" "Starting Simple Container Deployment"
log_info "Stack: ${STACK_NAME}"
log_info "Environment: ${ENVIRONMENT}"
log_info "Repository: ${GITHUB_REPOSITORY}"
log_info "Actor: ${GITHUB_ACTOR}"

# Validate required inputs
if [[ -z "${STACK_NAME:-}" || -z "${ENVIRONMENT:-}" || -z "${SC_CONFIG:-}" ]]; then
    log_error "Missing required inputs: STACK_NAME, ENVIRONMENT, or SC_CONFIG"
    exit 1
fi

# Set global variables
export DEPLOY_START_TIME=$(date +%s)
export WORKSPACE="/workspace"
export SC_CONFIG_FILE="$WORKSPACE/.sc/cfg.default.yaml"
export DEVOPS_DIR="$WORKSPACE/.devops"

#######################
# PHASE 1: SETUP AND PREPARATION
#######################
log_phase "PHASE 1" "Setup and Preparation"

# Fix permissions for hosted runners
if [[ "${RUNNER:-}" == "integrail" ]]; then
    log_info "Fixing permissions for hosted runner"
    /scripts/docker-utils/fix-permissions.sh
fi

# Setup Git configuration
log_info "Setting up Git configuration"
/scripts/common/setup-git.sh

# Generate version using CalVer
log_info "Generating deployment version"
/scripts/common/generate-version.sh

# Extract metadata (branch, author, commit message, etc.)
log_info "Extracting build metadata"
/scripts/common/extract-metadata.sh

# Setup Slack user mapping
log_info "Setting up notification mappings"
/scripts/common/slack-user-mapping.sh

#######################
# PHASE 2: REPOSITORY OPERATIONS  
#######################
log_phase "PHASE 2" "Repository Operations"

# Clone repository (replaces actions/checkout@v5)
log_info "Cloning repository: ${GITHUB_REPOSITORY}"
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"

# Clone with appropriate options based on context
if [[ -n "${PR_HEAD_REF:-}" ]]; then
    log_info "PR context detected - cloning PR branch: ${PR_HEAD_REF}"
    git clone --depth 0 "https://github.com/${GITHUB_REPOSITORY}.git" .
    git fetch origin "${PR_HEAD_REF}:${PR_HEAD_REF}"
    git checkout "${PR_HEAD_REF}"
else
    log_info "Regular deployment - cloning default branch"
    git clone --depth 0 "https://github.com/${GITHUB_REPOSITORY}.git" .
fi

# Enable LFS if needed
if git lfs ls-files | grep -q .; then
    log_info "Git LFS detected - pulling LFS files"
    git lfs pull
fi

#######################
# PHASE 3: SIMPLE CONTAINER SETUP
#######################
log_phase "PHASE 3" "Simple Container Setup"

# Simple Container CLI is pre-installed in the action image
log_info "Using pre-built Simple Container CLI"
sc --version

# Setup Simple Container configuration
log_info "Setting up SC configuration"
/scripts/sc-operations/setup-config.sh

# Checkout DevOps repository for shared configurations
log_info "Setting up DevOps repository access"
/scripts/sc-operations/checkout-devops.sh

# Reveal secrets and setup environment
log_info "Revealing secrets and setting up environment"
/scripts/sc-operations/reveal-secrets.sh

#######################
# PHASE 4: PR PREVIEW CONFIGURATION
#######################
if [[ "${PR_PREVIEW:-false}" == "true" ]]; then
    log_phase "PHASE 4" "PR Preview Configuration"
    
    if [[ -z "${PR_NUMBER:-}" ]]; then
        log_error "PR preview enabled but PR_NUMBER not available"
        exit 1
    fi
    
    log_info "Computing PR preview subdomain for PR #${PR_NUMBER}"
    /scripts/pr-preview/compute-subdomain.sh
    
    log_info "Appending PR preview profile to client.yaml"
    /scripts/pr-preview/append-stack-profile.sh
    
    # Add PR preview link to GitHub Step Summary
    /scripts/pr-preview/add-summary-link.sh
fi

#######################
# PHASE 5: CUSTOM CONFIGURATION
#######################
if [[ -n "${STACK_YAML_CONFIG:-}" ]]; then
    log_phase "PHASE 5" "Custom Configuration"
    
    log_info "Applying custom YAML configuration"
    /scripts/pr-preview/append-yaml-config.sh
fi

#######################
# PHASE 6: START NOTIFICATION
#######################
log_phase "PHASE 6" "Deployment Start Notification"

log_info "Sending deployment start notification"
/scripts/notifications/send-slack.sh "started"

#######################
# PHASE 7: STACK DEPLOYMENT
#######################
log_phase "PHASE 7" "Stack Deployment"

# Docker registry authentication
log_info "Authenticating with Docker registry"
/scripts/docker-utils/docker-login.sh

# Deploy the stack
log_info "Executing stack deployment"
/scripts/sc-operations/deploy-stack.sh

#######################
# PHASE 8: VALIDATION (OPTIONAL)
#######################
if [[ -n "${VALIDATION_COMMAND:-}" ]]; then
    log_phase "PHASE 8" "Post-Deployment Validation"
    
    log_info "Running validation command"
    /scripts/validation/run-validation.sh
fi

#######################
# PHASE 9: FINALIZATION
#######################
log_phase "PHASE 9" "Finalization and Cleanup"

# Create release tag
log_info "Creating release tag"
/scripts/finalization/create-release-tag.sh

# Calculate deployment duration
log_info "Calculating deployment duration"
/scripts/common/duration-calc.sh

# Send success notification
log_info "Sending success notification"
/scripts/notifications/send-slack.sh "success"

# Set action outputs
log_info "Setting action outputs"
/scripts/finalization/set-outputs.sh

log_phase "COMPLETE" "Deployment completed successfully!"
log_info "✅ Stack ${STACK_NAME} deployed to ${ENVIRONMENT}"
log_info "🚀 Version: $(cat /tmp/deploy_version)"
log_info "⏱️ Duration: $(cat /tmp/deploy_duration)"

exit 0
