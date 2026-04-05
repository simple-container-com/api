# AI-Friendly Skills Documentation Structure - Design

## Overview

This document describes the architecture for creating an AI-friendly skills-based documentation structure for Simple Container (SC). The goal is to enable AI agents to understand and execute key workflows with minimal human intervention.

## Problem Statement

Current SC documentation under `docs/docs` is not structured for AI agent consumption. AI agents need:
1. Clear installation instructions
2. Step-by-step DevOps configuration with proper secrets
3. Service configuration with deployment type determination
4. Secrets management guidance

## Design Goals

1. **Machine-Readable**: Documentation should support semantic search and AI comprehension
2. **Step-by-Step**: Each skill provides clear, sequential instructions
3. **Complete Examples**: All configuration examples include complete YAML with credentials sections
4. **Self-Contained**: Each skill documents its prerequisites and outputs

## Proposed Structure

```
docs/docs/skills/
├── index.md                      # Skills overview and navigation
├── installation.md               # SC CLI installation skill
├── devops-setup.md               # DevOps infrastructure setup skill
├── service-setup.md              # Service configuration skill
├── deployment-types.md           # Deployment type determination skill
├── secrets-management.md         # Secrets configuration skill
└── cloud-providers/
    ├── aws.md                    # AWS-specific setup guide
    ├── gcp.md                    # GCP-specific setup guide
    └── kubernetes.md             # Kubernetes-specific setup
```

## Skills Design

### 1. Installation Skill (`installation.md`)

**Purpose**: Enable AI agent to install SC CLI from scratch

**Prerequisites**:
- Docker installed
- Cloud provider CLI (aws, gcloud, kubectl)
- Shell access (bash/zsh)

**Steps**:
1. Download SC CLI binary for platform
2. Verify checksum
3. Install to PATH
4. Run `sc version` to verify
5. Configure autocomplete

### 2. DevOps Setup Skill (`devops-setup.md`)

**Purpose**: Create complete server.yaml with proper cloud authentication

**Inputs**:
- Cloud provider selection
- Project name
- Region preference

**Outputs**:
- `server.yaml` file
- Authentication configuration

**Required Credentials by Provider**:
- AWS: Access key ID, Secret access key, Account ID, Region
- GCP: Project ID, Service account key, Region/Zone
- Kubernetes: kubeconfig, registry credentials

### 3. Service Setup Skill (`service-setup.md`)

**Purpose**: Create client.yaml based on deployment type

**Inputs**:
- Project type (microservice, lambda, static site)
- Technology stack
- Parent stack reference

**Outputs**:
- `client.yaml` file
- `docker-compose.yaml` (if needed)
- `Dockerfile` (if needed)

### 4. Deployment Types Skill (`deployment-types.md`)

**Purpose**: Guide AI agent to select correct deployment type

**Decision Tree**:
```
Is this a static website?
  YES → Use "static" type
  NO → Does it need multiple containers?
    YES → Use "cloud-compose" type
    NO → Use "single-image" type
```

### 5. Secrets Management Skill (`secrets-management.md`)

**Purpose**: Document secrets.yaml structure and placeholder resolution

**Placeholder Types**:
- `${auth:provider}` - Authentication references
- `${secret:name}` - Secrets from secrets.yaml
- `${resource:resourceName}` - Resources from server.yaml

### 6. Cloud Provider Skills

**AWS Guide** (`cloud-providers/aws.md`):
- Creating IAM user with required permissions
- Getting access keys
- Determining account ID and region
- ECR repository creation

**GCP Guide** (`cloud-providers/gcp.md`):
- Creating GCP project
- Getting service account credentials
- Enabling required APIs
- Artifact Registry setup

**Kubernetes Guide** (`cloud-providers/kubernetes.md`):
- kubeconfig generation
- Docker registry configuration
- Namespace setup

## Integration Points

### Existing Documentation Dependencies
1. `docs/docs/ai-assistant/templates-config-requirements.md` - Must be referenced/integrated
2. `docs/docs/ai-assistant/commands.md` - CLI commands reference
3. `docs/docs/reference/service-available-deployment-schemas.md` - Deployment types

### MkDocs Integration
Add new section to `mkdocs.yml`:
```yaml
nav:
  - Skills:
    - skills/index.md
    - Installation: skills/installation.md
    - DevOps Setup: skills/devops-setup.md
    - Service Setup: skills/service-setup.md
    - Deployment Types: skills/deployment-types.md
    - Secrets Management: skills/secrets-management.md
    - Cloud Providers:
      - skills/cloud-providers/aws.md
      - skills/cloud-providers/gcp.md
      - skills/cloud-providers/kubernetes.md
```

## Acceptance Criteria

1. **Installation**: AI agent can install SC CLI using documentation alone
2. **DevOps Config**: AI agent can create server.yaml with proper cloud authentication
3. **Service Config**: AI agent can create client.yaml for any deployment type
4. **Secrets**: AI agent knows how to obtain and configure all required secrets
5. **Consistency**: All examples include complete config with credentials section
6. **AI-Friendly**: Documentation supports semantic search for AI consumption

## File Manifest

| File | Purpose |
|------|---------|
| `docs/docs/skills/index.md` | Skills index and navigation |
| `docs/docs/skills/installation.md` | Installation skill |
| `docs/docs/skills/devops-setup.md` | DevOps setup skill |
| `docs/docs/skills/service-setup.md` | Service setup skill |
| `docs/docs/skills/deployment-types.md` | Deployment types skill |
| `docs/docs/skills/secrets-management.md` | Secrets management skill |
| `docs/docs/skills/cloud-providers/aws.md` | AWS guide |
| `docs/docs/skills/cloud-providers/gcp.md` | GCP guide |
| `docs/docs/skills/cloud-providers/kubernetes.md` | Kubernetes guide |

## Implementation Notes

- Each skill should have clear "Prerequisites", "Steps", and "Verification" sections
- All YAML examples should use placeholders like `${AWS_ACCESS_KEY_ID}` that AI can replace
- Each cloud provider guide should include exact CLI commands to obtain credentials
- Keep content focused on AI workflows - avoid deep technical dive into advanced configs