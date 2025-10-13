# Destroy Client Stack Action

## Overview

The **Destroy Client Stack Action** replaces the `destroy-service.yaml` workflow (361 lines) with a simple, reusable action that handles safe destruction of Simple Container application stacks.

## Action Purpose

**What it does**: Safely destroys application stacks (client.yaml configurations) from specified environments using Simple Container CLI with proper cleanup and confirmation.

**What it replaces**: The entire `destroy-service.yaml` workflow including:
- Complex preparation and metadata extraction
- Environment setup and configuration handling
- PR preview cleanup
- Stack destruction with confirmation handling
- Comprehensive notification and cleanup system

## Input Specification

### Required Inputs

```yaml
stack-name:
  description: "Name of the stack to destroy (e.g., 'my-app', 'api-service')"
  required: true
  type: string
  
environment:
  description: "Environment to destroy (staging, prod, development, test)"
  required: true
  type: string
  
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret content)"
  required: true
  type: string
```

### Optional Inputs

```yaml
sc-version:
  description: "Simple Container CLI version to use"
  required: false
  type: string
  default: "2025.8.5"
  
sc-destroy-flags:
  description: "Additional flags for sc destroy command"
  required: false
  type: string
  default: ""
  
runner:
  description: "GitHub Actions runner type"
  required: false
  type: string
  default: "ubuntu-latest"
  
auto-confirm:
  description: "Automatically confirm destruction (dangerous - use with caution)"
  required: false
  type: boolean
  default: false
  
wait-timeout:
  description: "Maximum time to wait for destruction to complete (in minutes)"
  required: false
  type: number
  default: 30
```

### PR Preview Inputs

```yaml
pr-preview:
  description: "Enable PR preview mode for pull request cleanup"
  required: false
  type: boolean
  default: false
  
preview-domain-base:
  description: "Base domain for PR preview subdomains"
  required: false
  type: string
  default: "preview.mycompany.com"
```

### Stack Configuration

```yaml
stack-yaml-config:
  description: "Additional YAML configuration to append before destruction (base64 encoded)"
  required: false
  type: string
  
stack-yaml-config-encrypted:
  description: "Whether stack-yaml-config is encrypted with SSH RSA public key"
  required: false
  type: boolean
  default: false
```

### Safety and Notification

```yaml
require-confirmation:
  description: "Require explicit confirmation before destroying production stacks"
  required: false
  type: boolean
  default: true
  
notify-on-start:
  description: "Send notification when destruction starts"
  required: false
  type: boolean
  default: true
  
skip-backup:
  description: "Skip automatic backup before destruction (not recommended)"
  required: false
  type: boolean
  default: false
```

## Output Specification

```yaml
stack-name:
  description: "Stack name that was destroyed"
  
environment:
  description: "Environment that was destroyed"
  
duration:
  description: "Destruction duration in human-readable format (e.g., '3m12s')"
  
status:
  description: "Final destruction status (success/failure/cancelled)"
  
build-url:
  description: "URL to the GitHub Actions build"
  
commit-sha:
  description: "Git commit SHA that triggered destruction"
  
branch:
  description: "Git branch that triggered destruction"
  
resources-destroyed:
  description: "Count of resources that were destroyed"
  
backup-location:
  description: "Location of configuration backup (if created)"
  
cleanup-summary:
  description: "Summary of cleanup operations performed"
```

## Workflow Implementation

### Phase 1: Pre-Destruction Safety Checks

**Responsibilities:**
- Validate destruction permissions and access control
- Extract Git metadata and build context
- Perform safety checks for production environments
- Create configuration backup before destruction

**Key Features:**
- **Production Protection**: Enhanced safety checks for production environments
- **Backup Creation**: Automatic backup of stack configurations before destruction
- **Access Validation**: Verify user permissions for destructive operations
- **Audit Trail**: Comprehensive logging of destruction requests

**Implementation Details:**
```yaml
- name: Pre-Destruction Safety Checks
  shell: bash
  run: |
    # Production environment safety check
    if [[ "${{ inputs.environment }}" == "prod" && "${{ inputs.require-confirmation }}" == "true" ]]; then
      echo "🔴 WARNING: Production environment destruction requested"
      echo "Stack: ${{ inputs.stack-name }}"
      echo "Environment: ${{ inputs.environment }}"
      echo "Requestor: $GITHUB_ACTOR"
      
      # Additional confirmation for production
      if [[ "${{ inputs.auto-confirm }}" != "true" ]]; then
        echo "Production destruction requires manual confirmation"
        exit 1
      fi
    fi
    
    # Create configuration backup
    if [[ "${{ inputs.skip-backup }}" != "true" ]]; then
      backup_dir="backups/$(date +%Y%m%d_%H%M%S)_${{ inputs.stack-name }}_${{ inputs.environment }}"
      mkdir -p "$backup_dir"
      
      # Backup client configuration
      if [[ -f ".sc/stacks/${{ inputs.stack-name }}/client.yaml" ]]; then
        cp ".sc/stacks/${{ inputs.stack-name }}/client.yaml" "$backup_dir/"
      fi
      
      echo "backup-location=$backup_dir" >> $GITHUB_OUTPUT
    fi
    
    # Set start timestamp
    echo "start-time=$(date +%s)" >> $GITHUB_OUTPUT
```

### Phase 2: Environment Setup and Configuration

**Responsibilities:**
- Install Simple Container CLI with specified version
- Set up environment and reveal secrets
- Checkout devops repository for shared configurations
- Handle PR preview configuration cleanup
- Append custom stack configurations if needed

**Key Features:**
- **CLI Installation**: Version-specific installation with caching
- **Secrets Management**: Secure handling of SC_CONFIG and related secrets
- **Configuration Preparation**: Support for PR previews and custom configurations
- **Environment Validation**: Verify stack exists before attempting destruction

**Implementation Details:**
```yaml
- name: Setup Destruction Environment
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
    SIMPLE_CONTAINER_VERSION: ${{ inputs.sc-version }}
  run: |
    # Install Simple Container CLI
    bash <(curl -Ls "https://dist.simple-container.com/sc.sh") --version
    
    # Verify CLI installation
    if ! command -v sc >/dev/null 2>&1; then
      echo "❌ Simple Container CLI installation failed"
      exit 1
    fi
    
    # Setup devops repository if needed
    if [[ -n "${{ inputs.stack-yaml-config }}" || "${{ inputs.pr-preview }}" == "true" ]]; then
      # Extract SSH key and checkout devops repo
      mkdir -p ~/.ssh
      echo "${{ steps.extract-ssh.outputs.private-key }}" > ~/.ssh/id_rsa
      chmod 600 ~/.ssh/id_rsa
      
      git clone git@github.com:myorg/devops.git .devops
    fi
    
    # Reveal secrets for stack operations
    if ! sc secrets reveal --force; then
      echo "⚠️ Failed to reveal secrets for ${{ inputs.stack-name }}"
      echo "Stack may not have secrets configured - continuing"
    fi
    
    # Verify stack exists before destruction
    if ! sc status -s "${{ inputs.stack-name }}" -e "${{ inputs.environment }}" >/dev/null 2>&1; then
      echo "⚠️ Stack ${{ inputs.stack-name }} not found in ${{ inputs.environment }}"
      echo "status=not-found" >> $GITHUB_OUTPUT
      exit 0
    fi
```

### Phase 3: Stack Destruction

**Responsibilities:**
- Handle PR preview configuration if enabled
- Execute stack destruction with proper confirmation
- Monitor destruction progress with timeout handling
- Handle cancellation and cleanup on interruption

**Key Features:**
- **Confirmation Handling**: Automatic 'yes' response for confirmed destructions
- **Progress Monitoring**: Real-time monitoring with timeout protection
- **Error Recovery**: Graceful handling of destruction failures
- **Cancellation Support**: Proper cleanup when operations are cancelled

**Implementation Details:**
```yaml
- name: Destroy Stack
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
  timeout-minutes: ${{ inputs.wait-timeout }}
  run: |
    # Handle PR preview configuration
    if [[ "${{ inputs.pr-preview }}" == "true" ]]; then
      PR_NUMBER="${{ github.event.pull_request.number }}"
      SUBDOMAIN="pr${PR_NUMBER}-${{ inputs.preview-domain-base }}"
      
      # Append PR preview profile to client.yaml
      bash .devops/.github/workflows/scripts/append-stack-profile.sh \
        ".sc/stacks/${{ inputs.stack-name }}/client.yaml" \
        "$SUBDOMAIN" \
        "$PR_NUMBER"
    fi
    
    # Append custom configuration if provided
    if [[ -n "${{ inputs.stack-yaml-config }}" ]]; then
      bash .devops/.github/workflows/scripts/append-stack-yaml-config.sh \
        ".sc/stacks/${{ inputs.stack-name }}/client.yaml" \
        "${{ inputs.stack-yaml-config }}" \
        "${{ inputs.stack-yaml-config-encrypted }}"
    fi
    
    # Execute stack destruction
    echo "🗑️ Destroying stack ${{ inputs.stack-name }} in ${{ inputs.environment }}"
    
    # Prepare destruction command
    destroy_cmd="sc destroy -s ${{ inputs.stack-name }} -e ${{ inputs.environment }} ${{ inputs.sc-destroy-flags }}"
    
    # Execute with automatic confirmation
    if echo y | $destroy_cmd; then
      echo "✅ Stack destruction completed successfully"
      echo "status=success" >> $GITHUB_OUTPUT
      
      # Count destroyed resources (if available)
      resource_count=$(sc status -s "${{ inputs.stack-name }}" -e "${{ inputs.environment }}" --count-resources 2>/dev/null || echo "unknown")
      echo "resources-destroyed=$resource_count" >> $GITHUB_OUTPUT
    else
      echo "❌ Stack destruction failed"
      echo "status=failure" >> $GITHUB_OUTPUT
      exit 1
    fi

- name: Handle Cancellation
  if: cancelled()
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
  run: |
    echo "⚠️ Destruction cancelled by user"
    
    # Attempt to cancel ongoing Simple Container operations
    if command -v sc >/dev/null 2>&1; then
      sc cancel -s "${{ inputs.stack-name }}" -e "${{ inputs.environment }}" || true
    fi
    
    echo "status=cancelled" >> $GITHUB_OUTPUT
```

### Phase 4: Cleanup and Finalization

**Responsibilities:**
- Calculate total destruction duration
- Clean up temporary files and configurations
- Send comprehensive notifications about destruction results
- Generate cleanup summary and audit information

**Key Features:**
- **Duration Tracking**: Precise timing of destruction operations
- **Cleanup Summary**: Detailed report of what was destroyed and cleaned up
- **Notification System**: Professional notifications with destruction details
- **Audit Trail**: Complete record of destruction operation

## Usage Examples

### Basic Stack Destruction

```yaml
name: Destroy Development Stack
on:
  workflow_dispatch:
    inputs:
      stack_name:
        description: 'Stack name to destroy'
        required: true
      environment:
        description: 'Environment to destroy from'
        required: true
        default: 'development'

jobs:
  destroy:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        with:
          stack-name: ${{ github.event.inputs.stack_name }}
          environment: ${{ github.event.inputs.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

### PR Preview Cleanup

```yaml
name: Clean up PR Preview
on:
  pull_request:
    types: [closed]

jobs:
  cleanup-preview:
    runs-on: ubuntu-latest
    if: github.event.pull_request.head.repo.full_name == github.repository
    steps:
      - uses: actions/checkout@v4
      - uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "preview.mycompany.com"
          auto-confirm: true
          notify-on-start: false
```

### Production Destruction with Enhanced Safety

```yaml
name: Destroy Production Stack
on:
  workflow_dispatch:
    inputs:
      stack_name:
        description: 'Stack name to destroy'
        required: true
      confirmation:
        description: 'Type "DESTROY" to confirm'
        required: true

jobs:
  validate-confirmation:
    runs-on: ubuntu-latest
    steps:
      - name: Validate destruction confirmation
        if: ${{ github.event.inputs.confirmation != 'DESTROY' }}
        run: |
          echo "❌ Invalid confirmation. You must type 'DESTROY' exactly."
          exit 1

  destroy-production:
    needs: validate-confirmation
    runs-on: ubuntu-latest
    environment: production-destroy # Requires manual approval
    steps:
      - uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        with:
          stack-name: ${{ github.event.inputs.stack_name }}
          environment: "prod"
          sc-config: ${{ secrets.SC_CONFIG }}
          require-confirmation: true
          auto-confirm: true
          wait-timeout: 60
          sc-destroy-flags: "--verbose --force"
```

### Batch Stack Cleanup

```yaml
name: Cleanup Old Development Stacks
on:
  schedule:
    # Run every Sunday at 3 AM UTC
    - cron: '0 3 * * 0'

jobs:
  cleanup-old-stacks:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        stack: [old-feature-1, old-feature-2, legacy-test-stack]
    steps:
      - uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
        continue-on-error: true
        with:
          stack-name: ${{ matrix.stack }}
          environment: "development"
          sc-config: ${{ secrets.SC_CONFIG }}
          auto-confirm: true
          notify-on-start: false
          skip-backup: true
```

## Advanced Features

### Smart Destruction Validation

**Pre-Destruction Checks:**
- Verifies stack exists before attempting destruction
- Validates user permissions for the target environment
- Checks for dependent stacks that might be affected
- Identifies persistent resources that may need special handling

**Resource Impact Analysis:**
- Analyzes what resources will be destroyed
- Identifies shared resources used by multiple stacks
- Warns about potential data loss from database destruction
- Provides cost impact of resource destruction

### Backup and Recovery

**Automatic Backup Creation:**
- Creates timestamped backups of all stack configurations
- Backs up environment-specific configuration overrides
- Stores backup metadata for easy restoration
- Supports backup retention policies

**Recovery Procedures:**
- Quick restoration from configuration backups
- Recovery validation to ensure stack integrity
- Rollback procedures for failed destructions
- Emergency recovery from partial destruction failures

### Progressive Destruction

**Staged Destruction:**
- Destroys resources in dependency-aware order
- Handles resource interdependencies gracefully
- Provides progress updates during long-running destructions
- Allows interruption and resumption of destruction process

**Resource-Specific Handling:**
- Special handling for databases with data preservation options
- Graceful termination of running containers
- DNS record cleanup with proper TTL handling
- Load balancer draining before destruction

### Safety and Compliance

**Production Safeguards:**
- Multi-level confirmation for production environments
- Mandatory waiting periods for critical infrastructure
- Audit logging for all destruction operations
- Integration with change management systems

**Compliance Features:**
- Data retention compliance before destruction
- Regulatory approval workflows
- Destruction audit trails for compliance reporting
- Data sanitization verification

## Security Features

### Access Control

**Environment-Based Permissions:**
- Fine-grained permissions per environment
- Role-based access control for destruction operations
- Integration with GitHub environment protection rules
- Audit trail of all destruction attempts

### Data Protection

**Sensitive Data Handling:**
- Automatic identification of sensitive data resources
- Special confirmation requirements for data-containing resources
- Data export options before destruction
- Secure deletion verification for sensitive resources

## Monitoring and Alerting

### Real-Time Monitoring

**Destruction Progress:**
- Real-time progress updates during destruction
- Resource-by-resource destruction status
- Early warning for stuck or failed destructions
- Integration with monitoring dashboards

### Post-Destruction Validation

**Cleanup Verification:**
- Verifies all resources were properly destroyed
- Checks for orphaned resources requiring manual cleanup
- Validates DNS record cleanup
- Confirms cost reduction from resource destruction

## Error Handling and Recovery

### Failure Recovery

**Partial Destruction Handling:**
- Identifies partially destroyed stacks
- Provides options to complete or rollback partial destruction
- Manual intervention procedures for complex failures
- State reconciliation after failures

**Resource Leak Prevention:**
- Automatic detection of orphaned resources
- Cleanup procedures for leaked resources
- Cost monitoring for unexpected resource charges
- Automated alerts for cleanup failures

## Migration Benefits

### Complexity Reduction

**Before (361 lines):**
```yaml
jobs:
  prepare: # 75 lines
    steps:
      - name: Prepare metadata
      # Complex metadata extraction and user mapping
      
  destroy: # 140 lines
    steps:
      - name: Checkout repository
      - name: Write sc-config
      - name: Read deploy SSH private key
      - name: Checkout devops repo (stacks)
      - name: Install Simple Container CLI
      - name: Prepare environment and secrets
      - name: Compute PR preview subdomain
      - name: Append preview environment profile
      - name: Append custom stack YAML configuration
      - name: Destroy environment
      - name: Cancel if cancelled
      # Manual execution with complex error handling
      
  finalize: # 146 lines
    steps:
      - name: Calculate destroy duration
      - name: destroy-stack success (Slack)
      - name: destroy-stack canceled (Slack)  
      - name: destroy-stack failed (Slack)
      # Complex notification handling
```

**After (Simple action):**
```yaml
steps:
  - uses: simple-container-com/api/.github/actions/destroy@v2025.10.4
    with:
      stack-name: "my-app"
      environment: "staging"
      sc-config: ${{ secrets.SC_CONFIG }}
```

### Safety Improvements

**Enhanced Protection:**
- Built-in safety checks for production environments
- Automatic backup creation before destruction
- Improved confirmation mechanisms with audit trails
- Better error handling and recovery procedures

**Operational Benefits:**
- Standardized destruction procedures across all projects
- Centralized security and compliance controls
- Simplified troubleshooting with unified logging
- Consistent cleanup and notification patterns

This action transforms stack destruction from a complex, error-prone manual process into a safe, reliable, and auditable operation with comprehensive safety measures and recovery capabilities.
