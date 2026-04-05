# Product Requirements: SC AI-Friendly Documentation

**Issue:** #195 - Request: make SC documentation more consistent and more AI-friendly

## Problem Statement

Currently, SC documentation (under `docs/docs` folder) is not very AI-friendly. AI agents need clear, structured instructions to:
1. Install SC
2. Prepare DevOps config (including obtaining necessary secrets)
3. Prepare Service config (including determining deployment type)

The documentation needs to be organized in a "skills" format similar to Claude's skill structure.

## Goals

1. Create AI-consumable skill-based documentation structure
2. Provide step-by-step instructions for key workflows
3. Ensure consistent template configurations (leverage existing `templates-config-requirements.md`)
4. Document how to obtain all necessary secrets for each cloud provider

## Scope

### In Scope
- Installation guide for SC CLI
- DevOps configuration setup (parent stack / server.yaml)
- Service configuration setup (client.yaml)
- Deployment type determination
- Secrets management guide
- Cloud provider specific instructions (AWS, GCP, Kubernetes)

### Out of Scope
- Deep dive into advanced configurations
- CI/CD pipeline documentation
- Migration guides

## Key Documentation Areas

### 1. Installation Skill
- How to install SC CLI
- Prerequisites (Docker, cloud provider CLI)
- Verification steps
- Upgrade instructions

### 2. DevOps Configuration Skill (server.yaml template)
- Creating infrastructure stack
- Cloud provider authentication setup
- Required resources configuration
- How to obtain secrets:
  - AWS: Access keys, account ID, region selection
  - GCP: Project ID, service account credentials
  - Kubernetes: kubeconfig, Docker registry credentials

### 3. Service Configuration Skill (client.yaml template)
- Deployment type selection logic:
  - `cloud-compose`: Multi-container microservices
  - `single-image`: Single-container (Lambda, Cloud Run)
  - `static`: Static websites
- Template configuration requirements (from existing doc)
- Environment configuration

### 4. Secrets Management Skill
- secrets.yaml structure
- ${auth:provider} references
- ${secret:name} placeholders
- ${resource:resourceName} references

## Documentation Structure (Proposed Skills Format)

```
docs/docs/skills/
├── installation.md           # How to install SC CLI
├── devops-setup.md          # DevOps infrastructure setup
├── service-setup.md         # Service configuration setup
├── deployment-types.md      # Deployment type determination
├── secrets-management.md    # Secrets configuration
└── cloud-providers/
    ├── aws.md              # AWS-specific setup
    ├── gcp.md              # GCP-specific setup
    └── kubernetes.md       # Kubernetes-specific setup
```

## Acceptance Criteria

1. **Installation**: AI agent can install SC CLI from scratch
2. **DevOps Config**: AI agent can create complete server.yaml with proper authentication
3. **Service Config**: AI agent can create client.yaml based on deployment type
4. **Secrets**: AI agent knows how to obtain and configure all required secrets
5. **Consistency**: All examples include complete configuration with credentials section
6. **AI-Friendly**: Documentation is structured for semantic search and AI consumption

## Dependencies

- Existing `docs/docs/ai-assistant/templates-config-requirements.md` - Must be referenced/integrated
- Existing `docs/docs/ai-assistant/commands.md` - CLI commands reference
- Existing `docs/docs/reference/service-available-deployment-schemas.md` - Deployment types

## Priority

**Medium** - Documentation improvement for better AI agent usability