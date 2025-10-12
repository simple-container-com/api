#!/bin/bash
# Simple Container Stack Deployment Operations

set -euo pipefail

source /scripts/common/logging.sh

setup_deployment_environment() {
    log_info "Setting up deployment environment variables"
    
    # Set required environment variables for deployment
    export DEPLOY_STACK_NAME="${STACK_NAME}"
    export DEPLOY_ENVIRONMENT="${ENVIRONMENT}"
    export VERSION="${VERSION:-$(cat /tmp/deploy_version)}"
    
    # Set IMAGE_VERSION if app-image-version is provided
    if [[ -n "${APP_IMAGE_VERSION:-}" ]]; then
        export IMAGE_VERSION="${APP_IMAGE_VERSION}"
        log_info "Using IMAGE_VERSION: ${IMAGE_VERSION}"
    fi
    
    log_info "Deployment environment configured:"
    log_info "  Stack: ${DEPLOY_STACK_NAME}"
    log_info "  Environment: ${DEPLOY_ENVIRONMENT}"
    log_info "  Version: ${VERSION}"
}

reveal_stack_secrets() {
    log_info "Revealing stack secrets"
    
    # Reveal secrets with error handling
    if ! sc secrets reveal --force; then
        log_warning "Failed to reveal secrets for ${STACK_NAME} - may not have secrets configured"
        log_info "Continuing deployment without secrets..."
        return 0
    fi
    
    log_info "✅ Secrets revealed successfully"
}

authenticate_docker_registry() {
    log_info "Authenticating with Docker registries"
    
    # Get Docker registry credentials from Simple Container
    local docker_user
    local docker_pass
    
    if docker_user=$(sc stack secret-get -s integrail docker-registry-readonly-username 2>/dev/null) && \
       docker_pass=$(sc stack secret-get -s integrail docker-registry-readonly-password 2>/dev/null); then
        
        log_info "Authenticating with docker.everworker.ai registry"
        if echo "$docker_pass" | docker login docker.everworker.ai -u "$docker_user" --password-stdin; then
            log_info "✅ Docker registry authentication successful"
        else
            log_warning "⚠️ Docker registry authentication failed"
        fi
    else
        log_info "No Docker registry credentials found - skipping authentication"
    fi
    
    # Additional registry authentication can be added here
    # if [[ -n "${ADDITIONAL_REGISTRY:-}" ]]; then
    #     authenticate_additional_registry
    # fi
}

execute_deployment() {
    log_info "Executing Simple Container deployment"
    
    local deploy_flags="${SC_DEPLOY_FLAGS:-}"
    local deployment_command="sc deploy -s ${DEPLOY_STACK_NAME} -e ${DEPLOY_ENVIRONMENT} ${deploy_flags}"
    
    log_info "Running: ${deployment_command}"
    
    # Execute deployment with comprehensive logging
    if eval "$deployment_command"; then
        log_info "✅ Stack deployment completed successfully"
        echo "success" > /tmp/deploy_status
        return 0
    else
        local exit_code=$?
        log_error "❌ Stack deployment failed with exit code: ${exit_code}"
        echo "failure" > /tmp/deploy_status
        return $exit_code
    fi
}

handle_deployment_cancellation() {
    log_warning "⚠️ Deployment cancellation detected"
    
    # Attempt to cancel ongoing Simple Container operations
    if command -v sc >/dev/null 2>&1; then
        log_info "Cancelling Simple Container operations"
        if sc cancel -s "${DEPLOY_STACK_NAME}" -e "${DEPLOY_ENVIRONMENT}"; then
            log_info "✅ Simple Container operations cancelled"
        else
            log_warning "⚠️ Failed to cancel Simple Container operations"
        fi
    fi
    
    echo "cancelled" > /tmp/deploy_status
}

validate_deployment_prerequisites() {
    log_info "Validating deployment prerequisites"
    
    # Check if Simple Container CLI is available
    if ! command -v sc >/dev/null 2>&1; then
        log_error "Simple Container CLI not found"
        return 1
    fi
    
    # Check if stack configuration exists
    local stack_config_path="${WORKSPACE}/.sc/stacks/${STACK_NAME}/client.yaml"
    if [[ ! -f "$stack_config_path" ]]; then
        log_error "Stack configuration not found: ${stack_config_path}"
        return 1
    fi
    
    # Check if environment is valid
    if [[ -z "${ENVIRONMENT}" ]]; then
        log_error "Environment not specified"
        return 1
    fi
    
    log_info "✅ Deployment prerequisites validated"
    return 0
}

create_deployment_summary() {
    log_info "Creating deployment summary"
    
    local status=$(cat /tmp/deploy_status 2>/dev/null || echo "unknown")
    local version=$(cat /tmp/deploy_version 2>/dev/null || echo "unknown")
    local duration=$(cat /tmp/deploy_duration 2>/dev/null || echo "unknown")
    
    # Add to GitHub Step Summary if available
    if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
        cat >> "$GITHUB_STEP_SUMMARY" <<EOF
## 🚀 Deployment Summary

- **Stack**: ${STACK_NAME}
- **Environment**: ${ENVIRONMENT}
- **Version**: ${version}
- **Status**: ${status}
- **Duration**: ${duration}
- **Triggered by**: ${GITHUB_ACTOR}

### Deployment Details
- **Repository**: ${GITHUB_REPOSITORY}
- **Branch**: ${GITHUB_REF_NAME}
- **Commit**: ${GITHUB_SHA:0:7}
EOF

        # Add PR preview link if applicable
        if [[ "${PR_PREVIEW:-false}" == "true" && -f "/tmp/preview_url" ]]; then
            echo "" >> "$GITHUB_STEP_SUMMARY"
            echo "### 🔍 Preview Environment" >> "$GITHUB_STEP_SUMMARY"
            echo "$(cat /tmp/preview_url)" >> "$GITHUB_STEP_SUMMARY"
        fi
    fi
}

main() {
    log_info "Starting Simple Container stack deployment"
    
    # Set up trap for cancellation handling
    trap 'handle_deployment_cancellation' INT TERM
    
    # Validate prerequisites
    validate_deployment_prerequisites
    
    # Setup deployment environment
    setup_deployment_environment
    
    # Reveal secrets
    reveal_stack_secrets
    
    # Authenticate with Docker registries
    authenticate_docker_registry
    
    # Execute the deployment
    execute_deployment
    
    # Create deployment summary
    create_deployment_summary
    
    log_info "✅ Deployment process completed"
}

main "$@"
