# Provision Parent Stack Action

## Overview

The **Provision Parent Stack Action** replaces the `provision.yaml` workflow (150 lines) with a simple, reusable action that handles infrastructure provisioning using Simple Container's parent stack management.

## Action Purpose

**What it does**: Provisions shared infrastructure and parent stacks (server.yaml configurations) that provide foundational resources for client applications.

**What it replaces**: The entire `provision.yaml` workflow including:
- Infrastructure preparation and versioning
- Simple Container CLI installation and setup
- Secrets management and revelation
- Parent stack provisioning execution
- Comprehensive notification and tagging system

## Input Specification

### Required Inputs

```yaml
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
  
runner:
  description: "GitHub Actions runner type"
  required: false
  type: string
  default: "ubuntu-latest"
  
version-suffix:
  description: "Suffix for generated version (e.g., '-beta', '-rc1')"
  required: false
  type: string
  default: ""
  
dry-run:
  description: "Perform a dry run without actually provisioning resources"
  required: false
  type: boolean
  default: false
  
target-environment:
  description: "Specific environment to provision (if not all environments)"
  required: false
  type: string
  
pulumi-stack:
  description: "Specific Pulumi stack to provision (advanced usage)"
  required: false
  type: string
```

### Notification Configuration

```yaml
notify-on-start:
  description: "Send notification when provisioning starts"
  required: false
  type: boolean
  default: true
  
notify-on-completion:
  description: "Send notification when provisioning completes"
  required: false
  type: boolean
  default: true
  
slack-webhook-url:
  description: "Custom Slack webhook URL (overrides default from secrets)"
  required: false
  type: string
```

## Output Specification

```yaml
version:
  description: "Generated version for the provision operation (CalVer format)"
  
duration:
  description: "Provisioning duration in human-readable format (e.g., '12m45s')"
  
status:
  description: "Final provisioning status (success/failure/cancelled)"
  
build-url:
  description: "URL to the GitHub Actions build"
  
commit-sha:
  description: "Git commit SHA that triggered provisioning"
  
branch:
  description: "Git branch that triggered provisioning"
  
resources-provisioned:
  description: "Count of resources that were provisioned"
  
environments-updated:
  description: "List of environments that were updated"
```

## Workflow Implementation

### Phase 1: Preparation

**Responsibilities:**
- Generate CalVer version for infrastructure changes
- Extract Git metadata (branch, author, commit message)
- Set up build context and timestamps
- Validate Simple Container configuration

**Key Features:**
- **Version Management**: Automatic CalVer generation with API validation
- **Build Context**: Comprehensive metadata extraction for audit trails
- **Pre-validation**: Configuration validation before resource provisioning

**Implementation Details:**
```yaml
- name: Prepare Infrastructure Provisioning
  shell: bash
  run: |
    # Generate version using CalVer
    VERSION=$(date +%Y.%-m.%-d).${GITHUB_RUN_NUMBER}
    echo "version=$VERSION" >> $GITHUB_OUTPUT
    
    # Extract git metadata
    echo "branch=$GITHUB_REF_NAME" >> $GITHUB_OUTPUT
    echo "author=$GITHUB_ACTOR" >> $GITHUB_OUTPUT
    echo "commit-sha=$GITHUB_SHA" >> $GITHUB_OUTPUT
    
    # Set start timestamp for duration calculation
    echo "start-time=$(date +%s)" >> $GITHUB_OUTPUT
```

### Phase 2: Environment Setup

**Responsibilities:**
- Install Simple Container CLI with version management
- Set up Pulumi for infrastructure operations
- Reveal and configure secrets for cloud providers
- Prepare webhook configurations for notifications

**Key Features:**
- **Multi-Cloud Support**: Automatic detection and setup for AWS, GCP, Kubernetes
- **Secrets Management**: Secure revelation of cloud credentials and API keys
- **Tool Installation**: Version-specific installation of required tools

**Implementation Details:**
```yaml
- name: Setup Infrastructure Tools
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
    SIMPLE_CONTAINER_VERSION: ${{ inputs.sc-version }}
  run: |
    # Install Simple Container CLI
    bash <(curl -Ls "https://dist.simple-container.com/sc.sh") --version
    
    # Install Pulumi for infrastructure operations
    curl -fsSL https://get.pulumi.com | sh
    export PATH=$PATH:~/.pulumi/bin
    
    # Reveal secrets for cloud operations
    sc secrets reveal --force
    
    # Extract notification webhooks
    echo "discord-webhook=$(sc stack secret-get -s parent cicd-bot-discord-webhook-url)" >> $GITHUB_OUTPUT
    echo "slack-webhook=$(sc stack secret-get -s parent cicd-bot-slack-webhook-url)" >> $GITHUB_OUTPUT
```

### Phase 3: Infrastructure Provisioning

**Responsibilities:**
- Execute parent stack provisioning using Simple Container
- Monitor provisioning progress and handle errors
- Track resource creation and environment updates
- Handle dry-run operations for validation

**Key Features:**
- **Progress Monitoring**: Real-time progress tracking for long-running operations
- **Error Recovery**: Automatic retry logic for transient failures
- **Resource Tracking**: Comprehensive logging of provisioned resources
- **Dry-Run Support**: Validation mode without actual resource creation

**Implementation Details:**
```yaml
- name: Provision Parent Stack Infrastructure
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
    VERSION: ${{ steps.prepare.outputs.version }}
  run: |
    # Set up provisioning context
    export PROVISION_VERSION="$VERSION"
    
    if [[ "${{ inputs.dry-run }}" == "true" ]]; then
      echo "🔍 Performing dry-run provisioning..."
      sc provision --dry-run --verbose
    else
      echo "🚀 Starting infrastructure provisioning..."
      
      # Execute provisioning with progress tracking
      if [[ -n "${{ inputs.target-environment }}" ]]; then
        sc provision --environment "${{ inputs.target-environment }}" --verbose
      else
        sc provision --verbose
      fi
    fi
    
    # Extract provisioning results
    echo "resources-provisioned=$(sc status --count-resources)" >> $GITHUB_OUTPUT
    echo "environments-updated=$(sc status --list-environments)" >> $GITHUB_OUTPUT
```

### Phase 4: Finalization

**Responsibilities:**
- Calculate total provisioning duration
- Create Git release tag for successful provisions
- Send comprehensive notifications with infrastructure details
- Clean up temporary resources and handle failures

**Key Features:**
- **Release Management**: Automatic Git tagging for infrastructure versions
- **Comprehensive Notifications**: Detailed infrastructure change notifications
- **Audit Trail**: Complete record of infrastructure changes and timings

## Usage Examples

### Basic Infrastructure Provisioning

```yaml
name: Provision Infrastructure
on:
  push:
    branches: [main]
    paths: 
      - 'infrastructure/**'
      - '.sc/stacks/*/server.yaml'

jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Scheduled Infrastructure Updates

```yaml
name: Weekly Infrastructure Sync
on:
  schedule:
    # Run every Sunday at 2 AM UTC
    - cron: '0 2 * * 0'

jobs:
  sync-infrastructure:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          sc-version: "latest"
          notify-on-start: false
```

### Environment-Specific Provisioning

```yaml
name: Provision Development Environment
on:
  workflow_dispatch:
    inputs:
      target_env:
        description: 'Target environment to provision'
        required: true
        default: 'development'
        type: choice
        options:
        - development
        - staging
        - production

jobs:
  provision-env:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          target-environment: ${{ github.event.inputs.target_env }}
          version-suffix: "-${{ github.event.inputs.target_env }}"
```

### Dry-Run Validation

```yaml
name: Validate Infrastructure Changes
on:
  pull_request:
    branches: [main]
    paths:
      - 'infrastructure/**'
      - '.sc/**'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          dry-run: true
          notify-on-completion: false
      
      - name: Comment PR with validation results
        uses: actions/github-script@v7
        with:
          script: |
            const { data: comments } = await github.rest.issues.listComments({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number,
            });
            
            const botComment = comments.find(comment => 
              comment.user.type === 'Bot' && 
              comment.body.includes('Infrastructure Validation')
            );
            
            const body = `## 🔍 Infrastructure Validation Results
            
            **Status**: ✅ Validation Passed
            **Duration**: ${{ steps.provision.outputs.duration }}
            **Resources Validated**: ${{ steps.provision.outputs.resources-provisioned }}
            
            The infrastructure changes in this PR have been validated successfully.`;
            
            if (botComment) {
              await github.rest.issues.updateComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                comment_id: botComment.id,
                body: body
              });
            } else {
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.issue.number,
                body: body
              });
            }
```

## Advanced Features

### Multi-Cloud Infrastructure Support

**Automatic Cloud Detection:**
- Analyzes server.yaml configurations to determine required cloud providers
- Sets up appropriate credentials and tools for each provider
- Handles cross-cloud dependencies and resource references

**Supported Providers:**
- **AWS**: ECS Fargate, Lambda, RDS, S3, VPC, IAM
- **GCP**: GKE Autopilot, Cloud SQL, Storage, IAM
- **Kubernetes**: Generic Kubernetes clusters with Helm
- **Hybrid**: Cross-cloud configurations with proper networking

### Infrastructure Drift Detection

**Configuration Validation:**
- Compares desired state (server.yaml) with actual infrastructure
- Identifies configuration drift and suggests corrections
- Provides detailed reports on infrastructure changes needed

**Drift Correction:**
- Automatic correction of minor configuration drift
- Interactive approval for major infrastructure changes
- Rollback capabilities for failed corrections

### Resource Dependency Management

**Smart Provisioning Order:**
- Analyzes resource dependencies across environments
- Provisions resources in correct dependency order
- Handles circular dependencies with proper error reporting

**Cross-Environment Dependencies:**
- Manages shared resources across multiple environments
- Handles environment-specific variations of shared resources
- Provides dependency visualization and impact analysis

### Cost Optimization

**Resource Cost Analysis:**
- Pre-provisioning cost estimation for new resources
- Cost impact analysis for infrastructure changes
- Budget validation and approval workflows

**Optimization Recommendations:**
- Identifies over-provisioned resources
- Suggests cost-effective alternatives
- Provides usage-based scaling recommendations

## Security Features

### Least Privilege Access

**IAM Role Management:**
- Creates minimal required permissions for each resource
- Implements role-based access control across environments
- Regular audit and cleanup of unused permissions

**Credential Rotation:**
- Automatic rotation of service account credentials
- Secure storage and distribution of rotated credentials
- Zero-downtime credential updates

### Infrastructure Security

**Security Baseline Enforcement:**
- Applies security best practices to all provisioned resources
- Enforces encryption at rest and in transit
- Implements network security policies and access controls

**Compliance Monitoring:**
- Continuous compliance checking against security standards
- Automated remediation of security violations
- Compliance reporting and audit trail generation

## Monitoring and Observability

### Infrastructure Monitoring

**Resource Health Monitoring:**
- Continuous monitoring of provisioned infrastructure
- Automated alerting for resource failures or degradation
- Health dashboards with real-time status information

**Performance Metrics:**
- Infrastructure performance tracking and trending
- Resource utilization monitoring and optimization
- Capacity planning based on usage patterns

### Provisioning Analytics

**Operation Metrics:**
- Provisioning success rates and failure analysis
- Performance benchmarking for different resource types
- Historical trending of provisioning times and costs

**Change Impact Analysis:**
- Analysis of infrastructure changes and their impacts
- Risk assessment for major infrastructure modifications
- Rollback planning and disaster recovery procedures

## Error Handling and Recovery

### Automatic Recovery

**Transient Failure Handling:**
- Automatic retry logic for cloud API failures
- Exponential backoff for rate-limited operations
- Circuit breaker patterns for failing cloud services

**State Consistency:**
- Automatic state reconciliation after failures
- Rollback capabilities for partial provisioning failures
- State corruption detection and recovery

### Manual Intervention

**Expert Escalation:**
- Automatic escalation for complex failures requiring manual intervention
- Expert notification system with detailed failure context
- Manual override capabilities for emergency situations

## Migration Benefits

### Complexity Reduction

**Before (150 lines):**
```yaml
jobs:
  prepare: # 35 lines
    steps:
      - uses: actions/checkout@v4
      - uses: fregante/setup-git-user@v2
      - name: Get next version # Complex version logic
      # ... multiple setup steps

  build: # 65 lines  
    steps:
      - uses: actions/checkout@v3
      - name: prepare secrets # Complex secret handling
      - name: provision base stacks # Manual CLI execution
      # ... error handling and cleanup

  finalize: # 50 lines
    steps:
      - uses: actions/checkout@v4
      - uses: rickstaa/action-create-tag@v1
      - name: Extract git reference # Complex metadata extraction
      - name: provision base stacks success (Slack) # Manual notification
      # ... failure handling and cleanup
```

**After (Simple action):**
```yaml
steps:
  - uses: simple-container/actions/provision-parent-stack@v1
    with:
      sc-config: ${{ secrets.SC_CONFIG }}
```

### Operational Improvements

**Standardized Infrastructure:**
- Consistent provisioning patterns across all projects
- Centralized infrastructure management and updates
- Standardized security and compliance practices

**Reduced Maintenance:**
- Single action repository for all infrastructure logic
- Automatic propagation of security updates and bug fixes
- Simplified troubleshooting with centralized logging

This action transforms infrastructure provisioning from a complex, manual process into a simple, reliable, and standardized operation that provides the foundation for all Simple Container applications.
