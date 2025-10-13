# Deploy Client Stack Action

## Overview

The **Deploy Client Stack Action** replaces the complex `build-and-deploy-service.yaml` workflow (467 lines) with a simple, reusable action that handles all aspects of deploying Simple Container application stacks.

## Action Purpose

**What it does**: Deploys application stacks (client.yaml configurations) to specified environments using Simple Container CLI.

**What it replaces**: The entire `build-and-deploy-service.yaml` workflow including:
- Complex preparation and metadata extraction
- Multi-stage build and deployment process  
- PR preview handling
- Custom configuration appending
- Validation execution
- Comprehensive notification system

## Input Specification

### Required Inputs

```yaml
stack-name:
  description: "Name of the stack to deploy (e.g., 'my-app', 'api-service')"
  required: true
  type: string
  
environment:
  description: "Target environment (staging, prod, development, test)"
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
  
sc-deploy-flags:
  description: "Additional flags for sc deploy command"
  required: false
  type: string
  default: "--skip-preview"
  
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
  
app-image-version:
  description: "Application image version to set as IMAGE_VERSION env var"
  required: false
  type: string
  
validation-command:
  description: "Optional command to run after successful deployment"
  required: false
  type: string
```

### PR Preview Inputs

```yaml
pr-preview:
  description: "Enable PR preview mode for pull request deployments"
  required: false
  type: boolean
  default: false
  
preview-domain-base:
  description: "Base domain for PR preview subdomains"
  required: false
  type: string
  default: "preview.mycompany.com"
```

### Advanced Configuration

```yaml
stack-yaml-config:
  description: "Additional YAML configuration to append to client.yaml (base64 encoded)"
  required: false
  type: string
  
stack-yaml-config-encrypted:
  description: "Whether stack-yaml-config is encrypted with SSH RSA public key"
  required: false
  type: boolean
  default: false
  
cc-on-start:
  description: "Tag deployment watchers on start notification"
  required: false
  type: string
  default: "true"
```

## Output Specification

```yaml
version:
  description: "Generated version for the deployment (CalVer format)"
  
environment:
  description: "Environment that was deployed to"
  
stack-name:
  description: "Stack name that was deployed"
  
duration:
  description: "Deployment duration in human-readable format (e.g., '5m23s')"
  
status:
  description: "Final deployment status (success/failure/cancelled)"
  
build-url:
  description: "URL to the GitHub Actions build"
  
commit-sha:
  description: "Git commit SHA that was deployed"
  
branch:
  description: "Git branch that was deployed"
```

## Workflow Implementation

### Phase 1: Preparation

**Responsibilities:**
- Generate CalVer version with optional suffix
- Extract Git metadata (branch, author, commit message)
- Map GitHub usernames to Slack user IDs
- Validate access permissions for production deployments
- Set up build timestamps for duration calculation

**Key Features:**
- **Access Control**: Restricts production deployments to approved team members
- **Version Management**: Automatic CalVer generation with API validation  
- **Metadata Extraction**: Comprehensive build context for notifications
- **Permission Handling**: Fixes hosted runner permissions if needed

### Phase 2: Build and Deploy

**Responsibilities:**
- Install Simple Container CLI with specified version
- Set up environment and reveal secrets
- Checkout devops repository for shared configurations
- Handle PR preview configuration (if enabled)
- Append custom stack configurations
- Execute deployment with progress tracking
- Run post-deployment validation (if specified)

**Key Features:**
- **CLI Installation**: Version-specific installation with caching
- **Secrets Management**: Secure handling of SC_CONFIG and related secrets
- **Configuration Handling**: Support for encrypted and base64-encoded configs
- **Docker Registry**: Automatic authentication for private registries
- **Progress Tracking**: Real-time deployment progress and feedback

### Phase 3: Validation (Optional)

**Responsibilities:**
- Execute user-provided validation commands
- Set up environment variables for validation context
- Report validation results

**Environment Variables Available:**
- `DEPLOYED_VERSION`: The version that was deployed
- `STACK_NAME`: Name of the deployed stack  
- `ENVIRONMENT`: Target environment name

### Phase 4: Finalization

**Responsibilities:**
- Calculate total deployment duration
- Create Git release tag for successful deployments
- Send comprehensive notifications (Slack/Discord)
- Handle cleanup for failed or cancelled deployments

**Key Features:**
- **Release Tagging**: Automatic Git tag creation for successful deployments
- **Notification System**: Professional Slack notifications with build details
- **Error Handling**: Graceful cleanup and cancellation handling
- **Duration Tracking**: Precise build time calculation and reporting

## Usage Examples

### Basic Deployment

```yaml
name: Deploy Application
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: "my-app"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
```

### Production Deployment with Validation

```yaml
name: Deploy to Production
on:
  push:
    tags: [v*]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: "api-service"
          environment: "prod"
          sc-config: ${{ secrets.SC_CONFIG }}
          sc-version: "2025.8.5"
          validation-command: |
            # Wait for deployment to be ready
            sleep 30
            # Run health check
            curl -f https://api.mycompany.com/health
```

### PR Preview Deployment

```yaml
name: PR Preview
on:
  pull_request:
    branches: [main]

jobs:
  preview:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: "webapp"
          environment: "staging"
          sc-config: ${{ secrets.SC_CONFIG }}
          pr-preview: true
          preview-domain-base: "preview.mycompany.com"
```

### Advanced Configuration with Custom YAML

```yaml
name: Deploy with Custom Config
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        default: 'staging'

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
        with:
          stack-name: "service"
          environment: ${{ github.event.inputs.environment }}
          sc-config: ${{ secrets.SC_CONFIG }}
          stack-yaml-config: ${{ secrets.CUSTOM_STACK_CONFIG }}
          stack-yaml-config-encrypted: true
          app-image-version: ${{ github.sha }}
          sc-deploy-flags: "--verbose --force"
```

## Advanced Features

### PR Preview System

**Automatic Subdomain Generation:**
- Format: `pr{PR_NUMBER}-{preview-domain-base}`
- Example: `pr123-preview.mycompany.com`
- Automatic profile appending to client.yaml

**Features:**
- Dynamic environment variable injection
- Custom domain configuration per PR
- Automatic cleanup when PR is closed
- Build summary with preview links

### Custom Stack Configuration

**Configuration Appending:**
- Supports base64-encoded YAML configurations
- Optional RSA encryption for sensitive configs
- Automatic decryption using SSH private keys
- Merges seamlessly with existing client.yaml

**Use Cases:**
- Environment-specific scaling parameters
- Feature flags for specific deployments
- Custom resource configurations
- Sensitive environment variables

### Notification System

**Slack Integration:**
- Professional block-based message formatting
- User mention system with GitHub → Slack ID mapping
- Build status tracking (started/success/failure/cancelled)
- Duration reporting and direct links to build logs
- Customizable mention behavior for different notification types

**Discord Support:**
- Webhook-based notifications
- Consistent formatting with Slack messages
- Build status and duration reporting

### Error Handling and Recovery

**Automatic Cleanup:**
- Cancels ongoing Simple Container operations on failure
- Proper resource cleanup and state management
- Comprehensive error reporting in notifications

**Cancellation Handling:**
- Graceful handling of cancelled workflows
- Automatic `sc cancel` command execution
- Clean status reporting for cancelled deployments

## Security Features

### Access Control

**Production Restrictions:**
- Configurable team member allowlist for production deployments
- Automatic rejection of unauthorized production deployments
- Audit trail for all deployment attempts

### Secrets Management

**SC_CONFIG Handling:**
- Secure secret extraction and temporary file management
- SSH private key extraction for devops repository access
- Automatic cleanup of sensitive temporary files

**Configuration Encryption:**
- RSA encryption support for sensitive stack configurations
- Automatic decryption using stored SSH keys
- Secure handling of encrypted payloads

## Performance Optimizations

### Caching Strategies

**CLI Installation:**
- Runner-specific CLI caching
- Version-based cache keys
- Automatic cache invalidation for updates

**Repository Operations:**
- Efficient devops repository checkout
- Minimal fetch depth for faster clones
- Automatic LFS handling where needed

### Parallel Operations

**Multi-Step Parallelization:**
- Concurrent secret revelation and environment preparation
- Parallel metadata extraction and configuration processing
- Optimized build pipeline for reduced wait times

## Monitoring and Observability

### Build Metrics

**Duration Tracking:**
- Precise timestamp-based duration calculation
- Per-phase timing for performance analysis
- Historical build time trending

**Status Reporting:**
- Real-time build status updates
- Comprehensive failure reporting with context
- Build artifact and log retention

### Integration Metrics

**Deployment Success Rate:**
- Success/failure ratio tracking
- Environment-specific deployment analytics
- Performance benchmarking across different configurations

## Migration Benefits

### Complexity Reduction

**Before (467 lines):**
```yaml
# Complex job dependencies
jobs:
  prepare: # 94 lines
  build: # 228 lines  
  validation: # 18 lines
  finalize: # 127 lines
```

**After (Simple action):**
```yaml
steps:
  - uses: simple-container-com/api/.github/actions/deploy@v2025.10.4
    with:
      stack-name: "my-app"
      environment: "staging"
      sc-config: ${{ secrets.SC_CONFIG }}
```

### Maintainability Improvements

**Centralized Updates:**
- Single action repository for all deployment logic
- Immediate propagation of bug fixes and improvements
- Consistent behavior across all projects

**Standardized Patterns:**
- Uniform error handling and notification patterns
- Consistent CLI version management
- Standardized security practices

This action transforms complex deployment workflows into simple, reliable, and maintainable CI/CD components that any team can use effectively.
