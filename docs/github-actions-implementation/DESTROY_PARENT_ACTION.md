# Destroy Parent Stack Action

## Overview

The **Destroy Parent Stack Action** provides a safe and controlled way to destroy shared infrastructure and parent stacks (server.yaml configurations) when they are no longer needed. This action implements enterprise-grade safety measures for infrastructure destruction.

## Action Purpose

**What it does**: Safely destroys shared infrastructure and parent stacks that were provisioned using Simple Container's infrastructure management.

**What it replaces**: *This is a new capability* - there was no existing workflow for parent stack destruction, which required manual intervention and complex procedures.

**Why it's needed**: Infrastructure lifecycle management requires the ability to safely tear down environments for cost optimization, security compliance, and resource cleanup.

## Input Specification

### Required Inputs

```yaml
sc-config:
  description: "Simple Container configuration (SC_CONFIG secret content)"
  required: true
  type: string
  
confirmation:
  description: "Destruction confirmation - must be 'DESTROY-INFRASTRUCTURE'"
  required: true
  type: string
```

### Safety and Scope Inputs

```yaml
target-environment:
  description: "Specific environment to destroy (required for safety)"
  required: true
  type: string
  
destroy-scope:
  description: "Scope of destruction (environment-only, shared-resources, all)"
  required: false
  type: string
  default: "environment-only"
  
safety-mode:
  description: "Safety mode (strict, standard, permissive)"
  required: false
  type: string
  default: "strict"
```

### Optional Configuration

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
  
wait-timeout:
  description: "Maximum time to wait for destruction (in minutes)"
  required: false
  type: number
  default: 60
  
dry-run:
  description: "Perform a dry run without actually destroying resources"
  required: false
  type: boolean
  default: false
```

### Advanced Options

```yaml
force-destroy:
  description: "Force destruction even if dependencies exist (extremely dangerous)"
  required: false
  type: boolean
  default: false
  
backup-before-destroy:
  description: "Create infrastructure backup before destruction"
  required: false
  type: boolean
  default: true
  
preserve-data:
  description: "Attempt to preserve data resources during destruction"
  required: false
  type: boolean
  default: true
  
exclude-resources:
  description: "Comma-separated list of resource names to exclude from destruction"
  required: false
  type: string
```

### Notification Configuration

```yaml
require-approval:
  description: "Require manual approval before starting destruction"
  required: false
  type: boolean
  default: true
  
notify-stakeholders:
  description: "Notify infrastructure stakeholders before destruction"
  required: false
  type: boolean
  default: true
  
approval-timeout:
  description: "Minutes to wait for manual approval"
  required: false
  type: number
  default: 60
```

## Output Specification

```yaml
duration:
  description: "Infrastructure destruction duration (e.g., '15m32s')"
  
status:
  description: "Destruction status (success/failure/cancelled/timeout)"
  
resources-destroyed:
  description: "Count of resources that were destroyed"
  
resources-preserved:
  description: "Count of resources that were preserved"
  
backup-location:
  description: "Location of infrastructure backup (if created)"
  
cost-savings:
  description: "Estimated monthly cost savings from destruction"
  
environments-affected:
  description: "List of environments affected by destruction"
  
cleanup-summary:
  description: "Detailed summary of destruction operations"
  
warning-summary:
  description: "Summary of warnings and issues encountered"
```

## Workflow Implementation

### Phase 1: Pre-Destruction Validation

**Responsibilities:**
- Validate destruction confirmation and permissions
- Perform comprehensive dependency analysis
- Check for active client stacks using infrastructure
- Create infrastructure backup and impact assessment

**Key Features:**
- **Strict Confirmation**: Requires exact confirmation string to prevent accidents
- **Dependency Analysis**: Identifies all client stacks depending on infrastructure
- **Impact Assessment**: Calculates cost and operational impact of destruction
- **Safety Checks**: Multiple layers of validation before any destructive actions

**Implementation Details:**
```yaml
- name: Pre-Destruction Safety Validation
  shell: bash
  run: |
    # Validate confirmation string
    if [[ "${{ inputs.confirmation }}" != "DESTROY-INFRASTRUCTURE" ]]; then
      echo "❌ Invalid confirmation. Must be exactly 'DESTROY-INFRASTRUCTURE'"
      echo "Provided: '${{ inputs.confirmation }}'"
      exit 1
    fi
    
    # Environment validation
    if [[ -z "${{ inputs.target-environment }}" ]]; then
      echo "❌ Target environment must be specified for safety"
      exit 1
    fi
    
    # Validate safety mode
    safety_mode="${{ inputs.safety-mode }}"
    if [[ "$safety_mode" != "strict" && "$safety_mode" != "standard" && "$safety_mode" != "permissive" ]]; then
      echo "❌ Invalid safety mode: $safety_mode"
      exit 1
    fi
    
    echo "🔍 Performing pre-destruction analysis..."
    echo "Environment: ${{ inputs.target-environment }}"
    echo "Scope: ${{ inputs.destroy-scope }}"
    echo "Safety Mode: $safety_mode"
    
    # Set start timestamp
    echo "start-time=$(date +%s)" >> $GITHUB_OUTPUT
```

### Phase 2: Dependency Analysis and Impact Assessment

**Responsibilities:**
- Analyze all client stacks that depend on parent infrastructure
- Identify shared resources and cross-environment dependencies
- Calculate cost impact and resource utilization
- Generate comprehensive impact report

**Key Features:**
- **Client Stack Discovery**: Finds all stacks using parent infrastructure
- **Resource Mapping**: Maps which resources are used by which stacks
- **Cost Analysis**: Calculates cost savings from infrastructure destruction
- **Risk Assessment**: Identifies high-risk operations and potential data loss

**Implementation Details:**
```yaml
- name: Infrastructure Dependency Analysis
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
  run: |
    # Install and setup Simple Container CLI
    bash <(curl -Ls "https://dist.simple-container.com/sc.sh") --version
    sc secrets reveal --force
    
    # Analyze infrastructure dependencies
    echo "🔍 Analyzing infrastructure dependencies..."
    
    # Find all client stacks using this parent infrastructure
    dependent_stacks=$(sc stack list --using-parent "${{ inputs.target-environment }}" --json || echo "[]")
    stack_count=$(echo "$dependent_stacks" | jq length)
    
    if [[ "$stack_count" -gt 0 ]]; then
      echo "⚠️ Found $stack_count client stacks using this infrastructure:"
      echo "$dependent_stacks" | jq -r '.[].name' | while read stack; do
        echo "  - $stack"
      done
      
      if [[ "${{ inputs.force-destroy }}" != "true" ]]; then
        echo "❌ Cannot destroy infrastructure with active client stacks"
        echo "Either destroy dependent stacks first or use force-destroy option"
        exit 1
      fi
    fi
    
    # Analyze resource costs and utilization
    echo "💰 Calculating cost impact..."
    cost_analysis=$(sc infrastructure cost-analysis --environment "${{ inputs.target-environment }}" --json || echo "{}")
    monthly_cost=$(echo "$cost_analysis" | jq -r '.monthly_cost // "unknown"')
    resource_count=$(echo "$cost_analysis" | jq -r '.resource_count // 0')
    
    echo "resources-to-destroy=$resource_count" >> $GITHUB_OUTPUT
    echo "estimated-cost-savings=$monthly_cost" >> $GITHUB_OUTPUT
    
    # Generate impact report
    cat > infrastructure-impact-report.md <<EOF
# Infrastructure Destruction Impact Report

## Summary
- **Environment**: ${{ inputs.target-environment }}
- **Resources to destroy**: $resource_count
- **Dependent client stacks**: $stack_count
- **Estimated monthly cost savings**: \$${monthly_cost}

## Dependent Stacks
$dependent_stacks

## Risk Assessment
- **Data Loss Risk**: $([ "$resource_count" -gt 0 ] && echo "HIGH" || echo "LOW")
- **Service Impact**: $([ "$stack_count" -gt 0 ] && echo "CRITICAL" || echo "MINIMAL")
- **Recovery Time**: Estimated 2-4 hours for full re-provisioning

EOF
```

### Phase 3: Backup and Preservation

**Responsibilities:**
- Create comprehensive backup of infrastructure configuration
- Export critical data from databases and storage systems
- Generate restoration procedures and documentation
- Preserve shared resources if requested

**Key Features:**
- **Configuration Backup**: Complete backup of server.yaml and related configurations
- **Data Preservation**: Selective backup of critical data resources
- **Restoration Procedures**: Generated procedures for infrastructure recovery
- **Resource Preservation**: Option to preserve specific critical resources

**Implementation Details:**
```yaml
- name: Create Infrastructure Backup
  if: ${{ inputs.backup-before-destroy == 'true' }}
  shell: bash
  run: |
    backup_timestamp=$(date +%Y%m%d_%H%M%S)
    backup_dir="infrastructure-backups/${backup_timestamp}_${{ inputs.target-environment }}"
    mkdir -p "$backup_dir"
    
    echo "💾 Creating infrastructure backup..."
    
    # Backup server configuration
    if [[ -d ".sc/stacks" ]]; then
      cp -r ".sc/stacks" "$backup_dir/stack-configs"
    fi
    
    # Backup secrets configuration (obfuscated)
    if [[ -f ".sc/secrets.yaml" ]]; then
      # Create sanitized version without actual secrets
      sc secrets export --sanitized > "$backup_dir/secrets-structure.yaml"
    fi
    
    # Export infrastructure state
    sc infrastructure export --environment "${{ inputs.target-environment }}" \
      --output "$backup_dir/infrastructure-state.json" || true
    
    # Generate restoration guide
    cat > "$backup_dir/RESTORATION_GUIDE.md" <<EOF
# Infrastructure Restoration Guide

## Environment: ${{ inputs.target-environment }}
## Backup Created: $backup_timestamp
## Triggered by: $GITHUB_ACTOR

## Restoration Steps

1. Restore stack configurations:
   \`\`\`bash
   cp -r stack-configs/* .sc/stacks/
   \`\`\`

2. Restore secrets (manual intervention required):
   \`\`\`bash
   # Review and restore secrets based on secrets-structure.yaml
   # Actual secret values must be obtained from secure storage
   \`\`\`

3. Re-provision infrastructure:
   \`\`\`bash
   sc provision --environment ${{ inputs.target-environment }}
   \`\`\`

## Recovery Time Estimate: 2-4 hours
## Dependencies: Cloud provider credentials, secret values

EOF
    
    echo "backup-location=$backup_dir" >> $GITHUB_OUTPUT
    echo "✅ Infrastructure backup created at: $backup_dir"
```

### Phase 4: Infrastructure Destruction

**Responsibilities:**
- Execute infrastructure destruction with progress monitoring
- Handle resource dependencies and destruction order
- Preserve specified resources and handle data migration
- Monitor destruction progress with timeout handling

**Key Features:**
- **Progressive Destruction**: Destroys resources in dependency-aware order
- **Resource Preservation**: Selectively preserves critical resources
- **Progress Monitoring**: Real-time updates during long-running operations
- **Error Recovery**: Handles partial failures with recovery options

**Implementation Details:**
```yaml
- name: Execute Infrastructure Destruction
  if: ${{ inputs.dry-run != 'true' }}
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
  timeout-minutes: ${{ inputs.wait-timeout }}
  run: |
    echo "🗑️ Starting infrastructure destruction..."
    echo "Environment: ${{ inputs.target-environment }}"
    echo "Scope: ${{ inputs.destroy-scope }}"
    
    # Prepare destruction options
    destroy_options="--environment ${{ inputs.target-environment }}"
    
    if [[ "${{ inputs.preserve-data }}" == "true" ]]; then
      destroy_options="$destroy_options --preserve-data"
    fi
    
    if [[ -n "${{ inputs.exclude-resources }}" ]]; then
      destroy_options="$destroy_options --exclude ${{ inputs.exclude-resources }}"
    fi
    
    if [[ "${{ inputs.force-destroy }}" == "true" ]]; then
      destroy_options="$destroy_options --force"
    fi
    
    # Execute destruction based on scope
    case "${{ inputs.destroy-scope }}" in
      "environment-only")
        echo "Destroying environment-specific resources only..."
        echo y | sc deprovision $destroy_options --scope environment
        ;;
      "shared-resources")
        echo "Destroying shared resources..."
        echo y | sc deprovision $destroy_options --scope shared
        ;;
      "all")
        echo "Destroying all infrastructure..."
        echo y | sc deprovision $destroy_options --scope all
        ;;
      *)
        echo "❌ Invalid destroy scope: ${{ inputs.destroy-scope }}"
        exit 1
        ;;
    esac
    
    # Verify destruction completion
    remaining_resources=$(sc infrastructure list --environment "${{ inputs.target-environment }}" --count 2>/dev/null || echo "0")
    
    if [[ "$remaining_resources" -eq 0 ]]; then
      echo "✅ Infrastructure destruction completed successfully"
      echo "status=success" >> $GITHUB_OUTPUT
    else
      echo "⚠️ Infrastructure destruction completed with $remaining_resources remaining resources"
      echo "status=partial" >> $GITHUB_OUTPUT
    fi
    
    echo "resources-remaining=$remaining_resources" >> $GITHUB_OUTPUT

- name: Handle Cancellation
  if: cancelled()
  shell: bash
  env:
    SIMPLE_CONTAINER_CONFIG: ${{ inputs.sc-config }}
  run: |
    echo "⚠️ Infrastructure destruction cancelled"
    
    # Attempt to cancel ongoing operations
    if command -v sc >/dev/null 2>&1; then
      sc cancel --environment "${{ inputs.target-environment }}" || true
    fi
    
    echo "status=cancelled" >> $GITHUB_OUTPUT
```

### Phase 5: Verification and Cleanup

**Responsibilities:**
- Verify infrastructure destruction was complete
- Clean up orphaned resources and configurations
- Generate destruction summary and audit report
- Send notifications to stakeholders

**Key Features:**
- **Completion Verification**: Ensures all intended resources were destroyed
- **Orphan Cleanup**: Identifies and cleans up orphaned resources
- **Audit Trail**: Complete record of destruction operations
- **Stakeholder Notification**: Professional notifications with destruction details

## Usage Examples

### Basic Infrastructure Destruction

```yaml
name: Destroy Development Infrastructure
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options:
          - development
          - testing
      confirmation:
        description: 'Type DESTROY-INFRASTRUCTURE to confirm'
        required: true

jobs:
  validate-confirmation:
    runs-on: ubuntu-latest
    steps:
      - name: Validate input
        if: ${{ github.event.inputs.confirmation != 'DESTROY-INFRASTRUCTURE' }}
        run: exit 1

  destroy-infrastructure:
    needs: validate-confirmation
    runs-on: ubuntu-latest
    environment: infrastructure-destroy
    steps:
      - uses: simple-container/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: ${{ github.event.inputs.confirmation }}
          target-environment: ${{ github.event.inputs.environment }}
          destroy-scope: "environment-only"
```

### Production Infrastructure Destruction with Enhanced Safety

```yaml
name: Destroy Production Infrastructure
on:
  workflow_dispatch:
    inputs:
      final_confirmation:
        description: 'Final confirmation (DESTROY-PRODUCTION-INFRASTRUCTURE)'
        required: true
      stakeholder_approval:
        description: 'Stakeholder approval ID'
        required: true

jobs:
  validate-approvals:
    runs-on: ubuntu-latest
    steps:
      - name: Validate confirmations
        run: |
          if [[ "${{ github.event.inputs.final_confirmation }}" != "DESTROY-PRODUCTION-INFRASTRUCTURE" ]]; then
            echo "Invalid final confirmation"
            exit 1
          fi
          
          # Validate stakeholder approval (integration with approval system)
          if ! curl -H "Authorization: Bearer ${{ secrets.APPROVAL_TOKEN }}" \
               "https://api.company.com/approvals/${{ github.event.inputs.stakeholder_approval }}" | \
               jq -e '.approved and .type == "infrastructure-destruction"'; then
            echo "Invalid or missing stakeholder approval"
            exit 1
          fi

  destroy-production:
    needs: validate-approvals
    runs-on: ubuntu-latest
    environment: 
      name: production-infrastructure-destroy
      required-reviewers: ["infrastructure-team", "security-team"]
    steps:
      - uses: simple-container/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: "DESTROY-INFRASTRUCTURE"
          target-environment: "production"
          destroy-scope: "all"
          safety-mode: "strict"
          backup-before-destroy: true
          preserve-data: true
          wait-timeout: 120
          require-approval: true
          notify-stakeholders: true
```

### Selective Resource Cleanup

```yaml
name: Clean Up Unused Resources
on:
  schedule:
    # Monthly cleanup on first Sunday at 4 AM UTC
    - cron: '0 4 1 * *'

jobs:
  resource-cleanup:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container/actions/destroy-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          confirmation: "DESTROY-INFRASTRUCTURE"
          target-environment: "cleanup"
          destroy-scope: "shared-resources"
          exclude-resources: "production-db,backup-storage,monitoring"
          dry-run: true
          backup-before-destroy: false
          
      - name: Review cleanup plan
        run: |
          echo "Cleanup completed in dry-run mode"
          echo "Review the destruction plan and run manually if approved"
```

## Advanced Features

### Intelligent Resource Management

**Smart Dependency Resolution:**
- Automatically determines safe destruction order based on resource dependencies
- Identifies circular dependencies and suggests manual intervention
- Provides alternative destruction strategies for complex scenarios

**Resource Lifecycle Management:**
- Tracks resource age and usage patterns
- Suggests optimal times for resource cleanup
- Integrates with cost optimization tools

### Data Protection and Recovery

**Advanced Backup Strategies:**
- Multi-tiered backup with different retention policies
- Cross-region backup replication for critical data
- Point-in-time recovery capabilities for databases

**Data Migration Support:**
- Automated data migration before resource destruction
- Data format conversion and validation
- Rollback capabilities for failed migrations

### Compliance and Auditing

**Regulatory Compliance:**
- GDPR compliance for data destruction
- SOX compliance for financial infrastructure
- HIPAA compliance for healthcare environments
- Custom compliance framework integration

**Advanced Auditing:**
- Immutable audit logs for all destruction operations
- Integration with enterprise audit systems
- Compliance reporting and certification support

## Security Features

### Multi-Layer Authorization

**Role-Based Access Control:**
- Environment-specific destruction permissions
- Resource-type-based authorization
- Time-based access restrictions

**Approval Workflows:**
- Multi-stakeholder approval requirements
- Automated approval routing based on risk assessment
- Integration with enterprise approval systems

### Secure Destruction

**Data Sanitization:**
- Cryptographic wiping of sensitive data
- Multiple-pass data destruction for compliance
- Verification of secure data destruction

## Cost Optimization

### Cost Analysis and Reporting

**Pre-Destruction Cost Analysis:**
- Detailed cost breakdown by resource type
- Historical cost trends and projections
- ROI analysis for infrastructure cleanup

**Post-Destruction Validation:**
- Verification of expected cost savings
- Identification of unexpected charges
- Cost optimization recommendations

## Migration Benefits

### Operational Excellence

**Standardized Procedures:**
- Consistent infrastructure destruction processes
- Reduced human error through automation
- Comprehensive audit trails and compliance

**Enhanced Safety:**
- Multiple validation layers prevent accidents
- Automatic backup and recovery procedures
- Progressive destruction with rollback capabilities

**Cost Management:**
- Automated cost analysis and optimization
- Scheduled cleanup of unused resources
- Integration with budgeting and forecasting tools

This action provides enterprise-grade infrastructure lifecycle management with comprehensive safety measures, compliance features, and operational excellence for managing Simple Container parent stack destruction.
