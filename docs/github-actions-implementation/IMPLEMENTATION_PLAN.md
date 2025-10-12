# Simple Container GitHub Actions - Implementation Plan

## Executive Summary

This document provides a comprehensive technical plan for implementing 4 reusable GitHub Actions that abstract the complexity of the existing hardcoded Simple Container workflows.

**Goal**: Transform complex 467+ line workflows into simple, reusable actions that any team can use without understanding the underlying complexity.

## Current State Analysis

### Existing Workflow Complexity

**Current Hardcoded Workflows:**

| Workflow                      | Lines | Jobs                                     | Complexity |
|-------------------------------|-------|------------------------------------------|------------|
| build-and-deploy-service.yaml | 467   | 4 (prepare, build, validation, finalize) | Very High  |
| provision.yaml                | 150   | 3 (prepare, build, finalize)             | Medium     |
| destroy-service.yaml          | 361   | 3 (prepare, destroy, finalize)           | High       |
| *(destroy parent - missing)*  | -     | -                                        | -          |

**Key Issues:**
- ❌ **Code Duplication**: Similar patterns across multiple workflows
- ❌ **High Maintenance**: Updates require changes in multiple files
- ❌ **Complexity Barrier**: Teams need deep workflow knowledge to use Simple Container
- ❌ **Inconsistency**: Different teams implement different patterns
- ❌ **No Standardization**: Each workflow has unique quirks and implementations

### Common Components Identified

**Shared Functionality Across Workflows:**

1. **Simple Container CLI Installation**
   - Version management (`sc-version` input)
   - Dynamic installation from distribution endpoint
   - Runner-specific handling (hosted vs self-hosted)

2. **Secrets and Configuration Management**
   - SC_CONFIG secret handling
   - SSH key extraction for devops repository access
   - Additional secrets (webhooks, registry credentials)
   - Stack configuration copying

3. **Metadata and Version Management**
   - CalVer version generation
   - Git metadata extraction (branch, author, commit message)
   - Slack user ID mapping
   - Build timestamp tracking

4. **Notification System**
   - Slack integration with structured payloads
   - Discord webhook support
   - Started/success/failure/cancelled states
   - Duration calculation and reporting

5. **Error Handling and Cleanup**
   - Cancellation handling with `sc cancel` command
   - Graceful failure reporting
   - Build duration calculation even on failure

6. **PR Preview Handling**
   - Dynamic subdomain generation
   - Stack profile appending
   - Preview environment configuration

7. **Custom Configuration**
   - Stack YAML configuration appending
   - Environment variable injection
   - Encrypted configuration decryption

## Target Architecture

### Action Structure

**Proposed GitHub Actions:**

```
simple-container/actions/
├── deploy-client-stack/
│   ├── action.yml
│   ├── scripts/
│   │   ├── install-sc.sh
│   │   ├── prepare-environment.sh
│   │   ├── handle-deployment.sh
│   │   └── cleanup.sh
│   └── README.md
├── provision-parent-stack/
│   ├── action.yml
│   ├── scripts/
│   │   ├── install-sc.sh
│   │   ├── prepare-environment.sh
│   │   ├── handle-provision.sh
│   │   └── cleanup.sh
│   └── README.md
├── destroy-client-stack/
│   ├── action.yml
│   ├── scripts/
│   │   ├── install-sc.sh
│   │   ├── prepare-environment.sh
│   │   ├── handle-destroy.sh
│   │   └── cleanup.sh
│   └── README.md
└── destroy-parent-stack/
    ├── action.yml
    ├── scripts/
    │   ├── install-sc.sh
    │   ├── prepare-environment.sh
    │   ├── handle-deprovision.sh
    │   └── cleanup.sh
    └── README.md
```

### Shared Script Library

**Common Scripts Reused Across Actions:**

```bash
# scripts/install-sc.sh
# - Handles SC CLI installation with version management
# - Runner detection (hosted vs self-hosted)
# - Version validation and caching

# scripts/prepare-environment.sh  
# - SC_CONFIG secret handling
# - SSH key extraction and devops repo checkout
# - Environment variable setup
# - Secrets revelation and copying

# scripts/notifications.sh
# - Slack/Discord webhook handling
# - Status reporting (started/success/failure/cancelled)
# - Duration calculation and formatting
# - User ID mapping and mentions

# scripts/cleanup.sh
# - Cancellation handling
# - Resource cleanup
# - Error state management
```

## Action Specifications

### 1. Deploy Client Stack Action

**Purpose**: Deploy application stacks using Simple Container
**Replaces**: build-and-deploy-service.yaml (467 lines)

**Inputs:**
```yaml
stack-name:
  description: "Name of the stack to deploy"
  required: true
  
environment:
  description: "Target environment (staging, prod, etc.)"
  required: true
  default: "staging"
  
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret)"
  required: true
  
sc-version:
  description: "Simple Container CLI version"
  required: false
  default: "2025.8.5"
  
sc-deploy-flags:
  description: "Additional flags for sc deploy command"
  required: false
  default: "--skip-preview"
  
runner:
  description: "GitHub Actions runner type"
  required: false
  default: "ubuntu-latest"
  
pr-preview:
  description: "Enable PR preview mode"
  required: false
  type: boolean
  default: false
  
preview-domain-base:
  description: "Base domain for PR previews"
  required: false
  default: "preview.mycompany.com"
  
stack-yaml-config:
  description: "Additional YAML config to append (base64 encoded)"
  required: false
  
stack-yaml-config-encrypted:
  description: "Whether stack-yaml-config is encrypted"
  required: false
  type: boolean
  default: false
  
app-image-version:
  description: "Application image version for IMAGE_VERSION env var"
  required: false
  
validation-command:
  description: "Optional command to run for post-deployment validation"
  required: false
  
cc-on-start:
  description: "Tag deployment watchers on start"
  required: false
  default: "true"
```

**Outputs:**
```yaml
version:
  description: "Generated version for the deployment"
  
environment:
  description: "Environment that was deployed to"
  
stack-name:
  description: "Stack name that was deployed"
  
duration:
  description: "Deployment duration (e.g., '5m23s')"
  
status:
  description: "Deployment status (success/failure/cancelled)"
```

**Implementation Steps:**

1. **Prepare Phase**
   - Version generation using CalVer
   - Git metadata extraction
   - User ID mapping for notifications
   - Access control validation

2. **Build Phase**  
   - SC CLI installation
   - Environment preparation
   - Secrets revelation
   - Stack configuration preparation
   - PR preview handling (if enabled)
   - Custom YAML configuration appending
   - Deployment execution with progress tracking
   - Docker registry authentication
   - Validation execution (if provided)

3. **Finalize Phase**
   - Duration calculation  
   - Success/failure notifications
   - Version tagging
   - Cleanup operations

### 2. Provision Parent Stack Action

**Purpose**: Provision infrastructure using Simple Container
**Replaces**: provision.yaml (150 lines)

**Inputs:**
```yaml
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret)"
  required: true
  
sc-version:
  description: "Simple Container CLI version"  
  required: false
  default: "2025.8.5"
  
runner:
  description: "GitHub Actions runner type"
  required: false
  default: "ubuntu-latest"
```

**Outputs:**
```yaml
version:
  description: "Generated version for the provision"
  
duration:
  description: "Provision duration (e.g., '12m45s')"
  
status:
  description: "Provision status (success/failure/cancelled)"
```

**Implementation Steps:**

1. **Prepare Phase**
   - Version generation
   - Git metadata extraction

2. **Build Phase**
   - SC CLI installation  
   - Secrets revelation
   - Infrastructure provisioning using `sc provision`

3. **Finalize Phase**
   - Success/failure notifications
   - Version tagging

### 3. Destroy Client Stack Action

**Purpose**: Destroy application stacks using Simple Container  
**Replaces**: destroy-service.yaml (361 lines)

**Inputs:**
```yaml
stack-name:
  description: "Name of the stack to destroy"
  required: true
  
environment:
  description: "Environment to destroy"
  required: true
  default: "staging"
  
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret)"  
  required: true
  
sc-version:
  description: "Simple Container CLI version"
  required: false
  default: "2025.8.5"
  
sc-destroy-flags:
  description: "Additional flags for sc destroy command"
  required: false
  
runner:
  description: "GitHub Actions runner type"
  required: false  
  default: "ubuntu-latest"
  
pr-preview:
  description: "Enable PR preview mode"
  required: false
  type: boolean
  default: false
  
preview-domain-base:
  description: "Base domain for PR previews"
  required: false
  default: "preview.mycompany.com"
  
stack-yaml-config:
  description: "Additional YAML config to append (base64 encoded)"
  required: false
  
stack-yaml-config-encrypted:
  description: "Whether stack-yaml-config is encrypted"
  required: false
  type: boolean
  default: false
```

**Outputs:**
```yaml
stack-name:
  description: "Stack name that was destroyed"
  
environment:
  description: "Environment that was destroyed"
  
duration:
  description: "Destroy duration (e.g., '3m12s')"
  
status:
  description: "Destroy status (success/failure/cancelled)"
```

**Implementation Steps:**

1. **Prepare Phase**
   - Git metadata extraction
   - User ID mapping

2. **Destroy Phase**
   - SC CLI installation
   - Environment preparation
   - Secrets revelation
   - Stack configuration preparation (for PR previews)
   - Stack destruction using `echo y | sc destroy`

3. **Finalize Phase**
   - Duration calculation
   - Success/failure notifications

### 4. Destroy Parent Stack Action

**Purpose**: Destroy infrastructure using Simple Container
**Replaces**: *(new functionality - no existing workflow)*

**Inputs:**
```yaml
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret)"
  required: true
  
sc-version:
  description: "Simple Container CLI version"
  required: false
  default: "2025.8.5"
  
runner:
  description: "GitHub Actions runner type"
  required: false
  default: "ubuntu-latest"
  
confirm:
  description: "Confirmation flag for dangerous operation"
  required: true
  type: boolean
```

**Outputs:**
```yaml
duration:
  description: "Deprovision duration (e.g., '8m34s')"
  
status:
  description: "Deprovision status (success/failure/cancelled)"
```

**Implementation Steps:**

1. **Prepare Phase**
   - Confirmation validation
   - Git metadata extraction

2. **Destroy Phase**
   - SC CLI installation
   - Secrets revelation  
   - Infrastructure destruction using `echo y | sc deprovision`

3. **Finalize Phase**
   - Duration calculation
   - Success/failure notifications

## Technical Implementation Details

### Script Architecture

**Modular Script Design:**

```bash
#!/bin/bash
# action-name/scripts/main.sh

set -euo pipefail

# Source common utilities
source "$(dirname "$0")/../../shared/utils.sh"
source "$(dirname "$0")/../../shared/install-sc.sh"
source "$(dirname "$0")/../../shared/notifications.sh"

# Action-specific logic
main() {
    log_info "Starting $ACTION_NAME"
    
    # Phase 1: Preparation
    prepare_environment "$@"
    
    # Phase 2: Execution  
    case "$ACTION_NAME" in
        "deploy-client-stack")
            execute_deployment "$@"
            ;;
        "provision-parent-stack")
            execute_provision "$@"
            ;;
        "destroy-client-stack")
            execute_destruction "$@" 
            ;;
        "destroy-parent-stack")
            execute_deprovision "$@"
            ;;
    esac
    
    # Phase 3: Finalization
    finalize_action "$@"
}

main "$@"
```

### Common Utilities Library

**shared/utils.sh:**
```bash
#!/bin/bash
# Shared utilities for all Simple Container actions

# Logging functions
log_info() { echo "ℹ️  $*"; }
log_warn() { echo "⚠️  $*"; }  
log_error() { echo "❌ $*" >&2; }
log_success() { echo "✅ $*"; }

# Duration calculation
start_timer() {
    echo "$(date +%s)" > /tmp/action_start_time
}

calculate_duration() {
    local start_time=$(cat /tmp/action_start_time 2>/dev/null || echo "$(date +%s)")
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local minutes=$((duration / 60))
    local seconds=$((duration % 60))
    echo "${minutes}m${seconds}s"
}

# Git metadata extraction
extract_git_metadata() {
    export GIT_BRANCH="$GITHUB_REF_NAME"
    export GIT_AUTHOR="$GITHUB_ACTOR"
    export GIT_MESSAGE="$(git log -1 --pretty=%B | tr -d '\n' || echo "Unknown commit")"
    export GIT_URL="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
}

# Slack user ID mapping
map_slack_user() {
    local github_user="$1"
    declare -A slack_devs=(
        ["smecsia"]="U08BPTBPQCQ"
        ["garlicbreadcleric"]="U08BFSGHYG1" 
        ["demonat0r"]="U08BS0MNMEC"
        # ... other mappings
    )
    echo "${slack_devs[$github_user]:-$github_user}"
}

# Version generation using CalVer
generate_version() {
    local current_date=$(date +%Y.%-m.%-d)
    local run_number=${GITHUB_RUN_NUMBER:-1}
    echo "$current_date.$run_number"
}
```

**shared/install-sc.sh:**
```bash
#!/bin/bash
# Simple Container CLI installation

install_simple_container() {
    local version="${1:-2025.8.5}"
    local runner="${2:-ubuntu-latest}"
    
    log_info "Installing Simple Container CLI version $version"
    
    # Skip installation on hosted runners if already available
    if [[ "$runner" == "self-hosted" ]] && command -v sc >/dev/null 2>&1; then
        log_info "Simple Container CLI already available on hosted runner"
        return 0
    fi
    
    # Set version environment variable
    if [[ -n "$version" ]]; then
        export SIMPLE_CONTAINER_VERSION="$version"
    fi
    
    # Install from distribution endpoint
    if ! bash <(curl -Ls "https://dist.simple-container.com/sc.sh") --version; then
        log_error "Failed to install Simple Container CLI"
        return 1
    fi
    
    # Verify installation
    if ! command -v sc >/dev/null 2>&1; then
        log_error "Simple Container CLI not found in PATH after installation"
        return 1
    fi
    
    log_success "Simple Container CLI installed successfully"
    sc --version
}
```

**shared/notifications.sh:**
```bash
#!/bin/bash  
# Notification system for Slack and Discord

send_slack_notification() {
    local webhook_url="$1"
    local status="$2"
    local message="$3"
    local stack_name="${4:-}"
    local environment="${5:-}"
    local duration="${6:-}"
    
    if [[ -z "$webhook_url" ]]; then
        log_warn "No Slack webhook URL provided, skipping notification"
        return 0
    fi
    
    local emoji
    case "$status" in
        "started") emoji="🚧" ;;
        "success") emoji="✅" ;;
        "failure") emoji="❗" ;;
        "cancelled") emoji="❌" ;;
        *) emoji="ℹ️" ;;
    esac
    
    local slack_payload
    slack_payload=$(cat <<EOF
{
    "blocks": [
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "$emoji *<$GIT_URL|$status>* $message"
            }
        }
    ]
}
EOF
)
    
    if ! curl -X POST -H "Content-type: application/json" --data "$slack_payload" "$webhook_url"; then
        log_warn "Failed to send Slack notification"
    fi
}
```

### Error Handling and Cleanup

**Robust Error Handling:**
```bash
#!/bin/bash
# Error handling and cleanup

cleanup_on_error() {
    local exit_code=$?
    local stack_name="$1"
    local environment="$2"
    
    log_error "Action failed with exit code $exit_code"
    
    # Cancel ongoing Simple Container operations
    if [[ -n "$stack_name" && -n "$environment" ]]; then
        log_info "Attempting to cancel ongoing operations..."
        if command -v sc >/dev/null 2>&1; then
            sc cancel -s "$stack_name" -e "$environment" || log_warn "Failed to cancel operations"
        fi
    fi
    
    # Calculate duration even on failure
    local duration=$(calculate_duration)
    echo "duration=$duration" >> "$GITHUB_OUTPUT"
    echo "status=failure" >> "$GITHUB_OUTPUT"
    
    # Send failure notification
    if [[ -n "$SLACK_WEBHOOK_URL" ]]; then
        send_slack_notification "$SLACK_WEBHOOK_URL" "failure" \
            "Action failed after $duration" "$stack_name" "$environment" "$duration"
    fi
    
    exit $exit_code
}

# Set trap for error handling
trap 'cleanup_on_error "$STACK_NAME" "$ENVIRONMENT"' ERR
```

## Migration Strategy

### Phase 1: Action Development (Weeks 1-2)
- ✅ Create action repository structure
- ✅ Implement shared script library
- ✅ Develop individual action scripts
- ✅ Create comprehensive action.yml files
- ✅ Write documentation and examples

### Phase 2: Testing and Validation (Week 3)
- ✅ Unit testing of shared scripts
- ✅ Integration testing with real Simple Container projects
- ✅ Performance comparison with existing workflows
- ✅ Security review and validation

### Phase 3: Documentation and Migration (Week 4)
- ✅ Comprehensive migration guide
- ✅ Example workflows for common scenarios  
- ✅ Training materials for development teams
- ✅ Rollout plan and timeline

### Phase 4: Deployment and Adoption (Weeks 5-6)
- ✅ Release actions to GitHub Marketplace
- ✅ Migrate pilot projects
- ✅ Monitor performance and gather feedback
- ✅ Full rollout across all projects

## Success Metrics

### Complexity Reduction
- **Lines of Code**: 978 lines (3 workflows) → ~100 lines total (action usage)
- **Maintenance Burden**: 3 complex workflows → 1 centralized action repository
- **Time to Deploy**: Reduce setup time from hours to minutes

### Developer Experience
- **Learning Curve**: Eliminate need to understand workflow internals
- **Error Rate**: Reduce deployment failures through standardization
- **Documentation**: Single source of truth for Simple Container CI/CD

### Operational Benefits  
- **Consistency**: Uniform behavior across all projects
- **Updates**: Central updates benefit all users immediately
- **Support**: Centralized troubleshooting and optimization

## Risk Assessment

### Technical Risks
- **🔶 Medium**: Action complexity might introduce new failure modes
  - *Mitigation*: Comprehensive testing and gradual rollout
- **🔶 Medium**: GitHub Actions platform limitations
  - *Mitigation*: Fallback strategies and alternative implementations

### Adoption Risks
- **🔶 Medium**: Teams resistant to changing existing workflows
  - *Mitigation*: Clear migration guide and demonstrated benefits
- **🟢 Low**: Backward compatibility concerns  
  - *Mitigation*: Actions designed to be drop-in replacements

### Operational Risks
- **🔴 High**: Central point of failure for all Simple Container deployments
  - *Mitigation*: Robust testing, monitoring, and rapid response procedures
- **🔶 Medium**: Version management across multiple projects
  - *Mitigation*: Semantic versioning and clear upgrade paths

## Implementation Timeline

**Total Timeline: 6 weeks**

| Week | Phase | Activities |
|------|-------|------------|
| 1 | Development | Create repository, implement shared library |
| 2 | Development | Complete individual actions, initial testing |  
| 3 | Validation | Integration testing, security review |
| 4 | Documentation | Migration guide, examples, training materials |
| 5 | Deployment | Marketplace release, pilot migrations |
| 6 | Adoption | Full rollout, monitoring, optimization |

This implementation plan provides the foundation for transforming Simple Container CI/CD from complex, hardcoded workflows into simple, standardized, and maintainable GitHub Actions.
