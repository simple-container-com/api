# Self-Contained Simple Container Actions - Usage Examples

These examples show how customers can use the completely self-contained Simple Container actions that embed ALL workflow functionality internally.

## Key Benefits

✅ **No External Dependencies**: Zero additional GitHub Actions required  
✅ **Complete Feature Parity**: All 467+ lines of workflow logic embedded  
✅ **Drop-in Replacement**: Direct replacement for existing workflows  
✅ **Professional Quality**: Enterprise-grade error handling and notifications  

## Real Customer Migration

### Before: Complex Workflow (117 lines)

**Current Customer Usage** (integrail/everworker):

```yaml
name: Build and deploy everworker
on:
  push:
    branches: ['main']
  workflow_dispatch:
    inputs:
      environment:
        default: 'staging'
        options: [staging, demo, jarvis, dmstrategic, ...]

jobs:
  deploy-init:  # 20+ lines of access control logic
    runs-on: ubuntu-latest
    steps:
      - if: ${{ !contains('["approved-users"]', github.actor) && ... }}
        run: |
          echo "Access restricted"
          exit 1

  deploy:  # 97+ lines calling external workflow
    needs: [deploy-init]
    uses: integrail/devops/.github/workflows/build-and-deploy-service.yaml@main
    with:
      stack-name: 'everworker'
      environment: "${{ inputs.environment || 'staging' }}"
      runner: 'blacksmith-8vcpu-ubuntu-2204'
    secrets:
      sc-config: "${{ secrets.SC_CONFIG }}"
```

### After: Self-Contained Action (15 lines)

```yaml
name: Deploy everworker
on:
  push:
    branches: ['main']
  workflow_dispatch:
    inputs:
      environment:
        default: 'staging'
        options: [staging, demo, jarvis, dmstrategic, ...]

jobs:
  deploy:
    runs-on: blacksmith-8vcpu-ubuntu-2204
    environment: ${{ inputs.environment }}  # GitHub Environment protection
    steps:
      - name: Deploy Application Stack
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "everworker"
          environment: ${{ inputs.environment || 'staging' }}
          sc-config: ${{ secrets.SC_CONFIG }}
```

**Reduction**: 117 lines → 15 lines (87% reduction)

## Complete Usage Examples

### 1. Basic Production Deployment

```yaml
name: Production Deploy
on:
  push:
    tags: [v*]

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production
    steps:
      - name: Deploy to Production  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "my-app"
          environment: "production"
          sc-config: ${{ secrets.SC_CONFIG }}
          validation-command: |
            sleep 30
            curl -f https://api.mycompany.com/health
          cc-on-start: "false"  # No notifications on start
```

### 2. PR Preview with Validation

```yaml
name: PR Preview
on:
  pull_request:
    types: [labeled, synchronize]

jobs:
  deploy-preview:
    if: contains(github.event.pull_request.labels.*.name, 'pr-preview')
    runs-on: ubuntu-latest
    steps:
      - name: Deploy PR Preview  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "preview.mycompany.com"
          validation-command: |
            # Complex validation with curl/jq (embedded in action)
            ACTUAL_VERSION=$(curl -s https://pr${{ github.event.pull_request.number }}-preview.mycompany.com/api/version | jq -r '.version')
            if [ "$ACTUAL_VERSION" != "$DEPLOYED_VERSION" ]; then
              echo "Version mismatch!"
              exit 1
            fi
```

### 3. Multi-Environment Matrix Deploy

```yaml
name: Multi-Environment Deploy
on:
  workflow_dispatch:
    inputs:
      environments:
        description: 'Environments (JSON array)'
        default: '["staging", "demo"]'

jobs:
  deploy:
    runs-on: blacksmith-8vcpu-ubuntu-2204
    strategy:
      matrix:
        environment: ${{ fromJSON(github.event.inputs.environments) }}
    steps:
      - name: Deploy to ${{ matrix.environment }}  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "multi-env-app"
          environment: ${{ matrix.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
          sc-deploy-flags: "--verbose --skip-preview"
```

### 4. Infrastructure Provisioning

```yaml
name: Provision Infrastructure
on:
  push:
    branches: [main]
    paths: ['infrastructure/**']
  workflow_dispatch:

jobs:
  provision:
    runs-on: ubuntu-latest
    steps:
      - name: Provision Parent Stack  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/provision-parent-stack@v1
        with:
          sc-config: ${{ secrets.SC_CONFIG }}
          notify-on-completion: true
```

### 5. Complete PR Lifecycle

```yaml
name: PR Lifecycle
on:
  pull_request:
    types: [labeled, unlabeled, closed, synchronize]

jobs:
  deploy-preview:
    if: >
      (github.event.action == 'labeled' && github.event.label.name == 'pr-preview') ||
      (github.event.action == 'synchronize' && contains(github.event.pull_request.labels.*.name, 'pr-preview'))
    runs-on: ubuntu-latest
    steps:
      - name: Deploy PR Preview  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/deploy-client-stack@v1
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "dev.mycompany.com"

  cleanup-preview:
    if: >
      (github.event.action == 'unlabeled' && github.event.label.name == 'pr-preview') ||
      (github.event.action == 'closed' && contains(github.event.pull_request.labels.*.name, 'pr-preview'))
    runs-on: ubuntu-latest
    steps:
      - name: Cleanup PR Preview  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/destroy-client-stack@v1
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          auto-confirm: true
```

### 6. Scheduled Infrastructure Cleanup

```yaml
name: Weekly Infrastructure Cleanup
on:
  schedule:
    - cron: '0 2 * * 0'  # Sunday 2 AM

jobs:
  cleanup-old-stacks:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        stack: [temp-feature-1, temp-feature-2, old-test]
    steps:
      - name: Cleanup Old Stack  # ONLY STEP NEEDED!
        uses: simple-container-com/api/.github/actions/destroy-client-stack@v1
        continue-on-error: true
        with:
          stack-name: ${{ matrix.stack }}
          environment: "development"
          sc-config: ${{ secrets.SC_CONFIG }}
          auto-confirm: true
          skip-backup: true
```

## Embedded Functionality

Each self-contained action internally handles:

### **Repository Operations** 
- Git cloning with appropriate options
- LFS support
- Branch switching for PR previews
- Git user configuration

### **Version Management**
- CalVer generation with API validation
- Custom version suffix support
- App image version overrides

### **Complex Metadata Processing**
- Slack user ID mapping (20+ users)
- Git metadata extraction
- Build URL generation
- Duration calculations

### **Simple Container Operations**
- CLI installation with version management
- Configuration file creation
- DevOps repository checkout via SSH
- Secrets revelation and processing
- Docker registry authentication
- Stack deployment execution

### **PR Preview System**
- Subdomain computation
- Stack profile appending
- YAML configuration modification
- GitHub Step Summary updates

### **Professional Notifications**
- Slack notifications with structured payloads
- Discord notifications with embeds
- Multiple states (started/success/failure/cancelled)
- User mentions and team tagging

### **Cleanup and Finalization**
- Release tag creation
- Duration calculation across phases
- Comprehensive error handling
- Cancellation management

## Migration Strategy

### 1. Create New Workflow Files
```bash
# Keep old workflows for safety
cp .github/workflows/deploy.yml .github/workflows/deploy.old.yml

# Replace with self-contained action
# Edit deploy.yml to use simple-container-com/api/.github/actions/deploy-client-stack@v1
```

### 2. Test in Development
```bash
# Test with development environment first
gh workflow run deploy.yml -f environment=development
```

### 3. Gradual Production Rollout
```bash
# After successful development testing
gh workflow run deploy.yml -f environment=staging
gh workflow run deploy.yml -f environment=production
```

### 4. Remove Old Workflows
```bash
# Once confident in new actions
rm .github/workflows/deploy.old.yml
```

## Customer Benefits

### **Immediate Benefits**
- **87% fewer lines** to maintain (117 → 15 lines)
- **Zero external dependencies** - no actions/checkout, no external tools
- **Professional notifications** out of the box
- **Enterprise error handling** and recovery

### **Long-term Benefits**
- **Zero maintenance burden** - Simple Container team handles all updates
- **Automatic feature additions** - new features automatically available
- **Bug fixes propagated automatically** - no customer action needed
- **Security updates included** - always use latest secure practices

### **Developer Experience**
- **Single step deployment** - one action call does everything
- **Clear error messages** - embedded logging and debugging
- **Professional notifications** - Slack/Discord with proper formatting
- **GitHub integration** - Step summaries, outputs, proper status reporting

This self-contained approach transforms Simple Container from a complex, maintenance-heavy set of workflows into simple, reliable actions that any team can use immediately without understanding the underlying complexity.
