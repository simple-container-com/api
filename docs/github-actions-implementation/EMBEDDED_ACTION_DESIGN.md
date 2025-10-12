# Self-Contained Simple Container Actions Design

After analyzing the existing workflows (467+ lines), it's clear the actions need to be completely self-contained, embedding ALL functionality internally without requiring additional GitHub Actions.

## Current Complexity Analysis

The existing workflows contain these embedded operations:

### **Repository Operations**
- Multiple `actions/checkout@v5` calls with different options
- `fregante/setup-git-user@v2` for Git configuration
- LFS support, fetch-depth settings, specific ref handling
- Permission fixes for hosted runners (`sudo chown`)

### **Version Management** 
- `reecetech/version-increment@2023.10.2` for CalVer generation
- API-based version validation
- Custom version suffix handling

### **Complex Metadata Processing**
- Slack user ID mapping (20+ user mappings)
- Git metadata extraction (branch, author, commit message)
- Build URL generation and context preparation

### **Simple Container Operations**
- SC CLI installation with version management
- Config file creation and management
- DevOps repository checkout via SSH
- Secrets revelation and processing
- Pulumi installation
- Docker registry authentication
- Stack deployment execution

### **PR Preview System**
- Subdomain computation logic
- Custom bash script execution (`append-stack-profile.sh`)
- YAML configuration appending with encryption support

### **Professional Notifications**
- Complex Slack notifications with structured JSON payloads
- Multiple notification states (started/success/failure/cancelled)
- User mention support with ID mapping

### **Cleanup and Finalization**
- `rickstaa/action-create-tag@v1` for release tagging
- Duration calculation across job boundaries
- Cancellation handling with cleanup

## Redesigned Architecture

### **Docker-Based Actions**
Instead of composite actions, we need Docker-based actions that include ALL required tools and scripts.

```dockerfile
FROM ubuntu:22.04

# Install all required tools
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    yq \
    docker.io \
    ssh \
    && rm -rf /var/lib/apt/lists/*

# Install Simple Container CLI
RUN curl -s "https://dist.simple-container.com/sc.sh" | bash

# Install Pulumi
RUN curl -fsSL https://get.pulumi.com | sh
ENV PATH="/root/.pulumi/bin:${PATH}"

# Copy all embedded scripts
COPY scripts/ /scripts/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /scripts/*

ENTRYPOINT ["/entrypoint.sh"]
```

### **Embedded Scripts Structure**

```bash
scripts/
├── common/
│   ├── setup-git.sh              # Git user configuration
│   ├── generate-version.sh       # CalVer version generation
│   ├── extract-metadata.sh       # Git metadata and build context
│   ├── slack-user-mapping.sh     # User ID mapping
│   └── duration-calc.sh          # Duration calculation
├── sc-operations/
│   ├── install-sc.sh             # SC CLI installation
│   ├── setup-config.sh           # Config file management
│   ├── checkout-devops.sh        # DevOps repo operations
│   ├── reveal-secrets.sh         # Secrets management
│   └── deploy-stack.sh           # Stack deployment
├── pr-preview/
│   ├── compute-subdomain.sh      # Subdomain computation
│   ├── append-stack-profile.sh   # Stack profile appending
│   └── append-yaml-config.sh     # YAML configuration
├── notifications/
│   ├── send-slack.sh             # Slack notifications
│   ├── send-discord.sh           # Discord notifications
│   └── format-payload.sh         # Notification formatting
├── finalization/
│   ├── create-release-tag.sh     # Git tagging
│   ├── handle-cancellation.sh    # Cleanup operations
│   └── calculate-results.sh      # Result processing
└── docker-utils/
    ├── fix-permissions.sh        # Runner permission fixes
    └── docker-login.sh           # Registry authentication
```

### **Main Entrypoint Script**

```bash
#!/bin/bash
# entrypoint.sh - Main orchestrator for Simple Container actions

set -euo pipefail

ACTION_TYPE="${1:-deploy}"
source /scripts/common/setup-git.sh
source /scripts/common/extract-metadata.sh

case "$ACTION_TYPE" in
    "deploy-client-stack")
        /scripts/deploy-client-entrypoint.sh
        ;;
    "provision-parent-stack")
        /scripts/provision-parent-entrypoint.sh
        ;;
    "destroy-client-stack")
        /scripts/destroy-client-entrypoint.sh
        ;;
    "destroy-parent-stack")
        /scripts/destroy-parent-entrypoint.sh
        ;;
    *)
        echo "Unknown action type: $ACTION_TYPE"
        exit 1
        ;;
esac
```

## Complete Self-Contained Action Example

### **Deploy Client Stack Action**

```yaml
name: 'Deploy Simple Container Client Stack'
description: 'Complete deployment solution - no additional actions required'
branding:
  icon: 'upload-cloud'
  color: 'blue'

inputs:
  stack-name:
    description: 'Name of the stack to deploy'
    required: true
  environment:
    description: 'Target environment'
    required: true
    default: 'staging'
  sc-config:
    description: 'Simple Container configuration'
    required: true
  # ... all other inputs

runs:
  using: 'docker'
  image: 'Dockerfile'
  args:
    - 'deploy-client-stack'
  env:
    # Pass all inputs as environment variables
    STACK_NAME: ${{ inputs.stack-name }}
    ENVIRONMENT: ${{ inputs.environment }}
    SC_CONFIG: ${{ inputs.sc-config }}
    # GitHub context
    GITHUB_TOKEN: ${{ github.token }}
    GITHUB_REPOSITORY: ${{ github.repository }}
    GITHUB_SHA: ${{ github.sha }}
    GITHUB_REF_NAME: ${{ github.ref_name }}
    GITHUB_ACTOR: ${{ github.actor }}
    GITHUB_RUN_ID: ${{ github.run_id }}
    GITHUB_SERVER_URL: ${{ github.server_url }}
    # PR context for previews
    PR_NUMBER: ${{ github.event.pull_request.number }}
    PR_HEAD_REF: ${{ github.event.pull_request.head.ref }}
    PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}
```

### **Deploy Client Entrypoint**

```bash
#!/bin/bash
# scripts/deploy-client-entrypoint.sh

set -euo pipefail

echo "🚀 Starting Simple Container deployment (self-contained)"
echo "Stack: $STACK_NAME"
echo "Environment: $ENVIRONMENT"

# Phase 1: Setup and Preparation
echo "📋 Phase 1: Setup and Preparation"
/scripts/docker-utils/fix-permissions.sh
/scripts/common/setup-git.sh
/scripts/common/generate-version.sh
/scripts/common/extract-metadata.sh
/scripts/common/slack-user-mapping.sh

# Phase 2: Repository Operations
echo "📁 Phase 2: Repository Setup"
# Built-in git operations (no external checkout action needed)
git clone --depth 1 "$GITHUB_SERVER_URL/$GITHUB_REPOSITORY.git" /workspace
cd /workspace

if [[ -n "${PR_HEAD_REF:-}" ]]; then
    git fetch origin "$PR_HEAD_REF:$PR_HEAD_REF"
    git checkout "$PR_HEAD_REF"
fi

# Phase 3: Simple Container Setup
echo "🔧 Phase 3: Simple Container Setup"
/scripts/sc-operations/install-sc.sh
/scripts/sc-operations/setup-config.sh
/scripts/sc-operations/checkout-devops.sh
/scripts/sc-operations/reveal-secrets.sh

# Phase 4: PR Preview Configuration (if applicable)
if [[ "$PR_PREVIEW" == "true" ]]; then
    echo "🔍 Phase 4: PR Preview Configuration"
    /scripts/pr-preview/compute-subdomain.sh
    /scripts/pr-preview/append-stack-profile.sh
fi

# Phase 5: Custom Configuration
if [[ -n "${STACK_YAML_CONFIG:-}" ]]; then
    echo "📝 Phase 5: Custom Configuration"
    /scripts/pr-preview/append-yaml-config.sh
fi

# Phase 6: Send Start Notification
echo "📢 Phase 6: Start Notification"
/scripts/notifications/send-slack.sh "started"

# Phase 7: Deploy Stack
echo "🚀 Phase 7: Stack Deployment"
/scripts/docker-utils/docker-login.sh
/scripts/sc-operations/deploy-stack.sh

# Phase 8: Validation (if provided)
if [[ -n "${VALIDATION_COMMAND:-}" ]]; then
    echo "✅ Phase 8: Validation"
    eval "$VALIDATION_COMMAND"
fi

# Phase 9: Finalization
echo "🏁 Phase 9: Finalization"
/scripts/finalization/create-release-tag.sh
/scripts/common/duration-calc.sh
/scripts/notifications/send-slack.sh "success"

echo "✅ Deployment completed successfully"
```

## Benefits of Self-Contained Design

### **Complete Independence**
- No external GitHub Actions dependencies
- All tools bundled in Docker image
- Self-contained script execution

### **Exact Feature Parity** 
- All 467 lines of workflow logic embedded
- Every notification, every metadata extraction
- Complete Slack user mapping and formatting

### **Simple Usage**
```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "my-app"
          environment: "staging" 
          sc-config: ${{ secrets.SC_CONFIG }}
        # NO OTHER STEPS NEEDED!
```

### **Zero External Dependencies**
- No `actions/checkout@v5`
- No `fregante/setup-git-user@v2`
- No `reecetech/version-increment@2023.10.2`
- No `8398a7/action-slack@v3`
- No `rickstaa/action-create-tag@v1`

### **Professional Implementation**
- Docker-based for reliability
- Comprehensive error handling
- Full logging and debugging support
- Proper cleanup and resource management

## Implementation Strategy

1. **Create Docker Images** for each of the 4 actions
2. **Embed All Scripts** for complete functionality
3. **Comprehensive Testing** against existing workflow behavior
4. **Documentation** with exact migration instructions

This design provides truly drop-in replacements that customers can use without understanding any of the underlying complexity, while maintaining 100% feature compatibility with the existing workflows.
