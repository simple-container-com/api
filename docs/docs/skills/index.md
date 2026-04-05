---
title: Skills
description: AI-friendly skill-based documentation for Simple Container workflows
platform: platform
product: simple-container
category: skills
date: '2026-03-29'
---

# SC Skills Documentation

This section provides AI-friendly, step-by-step documentation for key Simple Container workflows. Each "skill" is designed to enable AI agents to complete tasks autonomously.

## Overview

SC skills provide structured, machine-readable documentation for:

1. **Installation** - Installing SC CLI from scratch
2. **DevOps Setup** - Creating infrastructure configuration
3. **Service Setup** - Creating service deployment configuration
4. **Deployment Types** - Selecting the correct deployment type
5. **Secrets Management** - Configuring secrets and credentials
6. **Cloud Providers** - Provider-specific setup guides

## Why Skills Format?

Traditional documentation is written for humans and can be ambiguous for AI agents. Skills format provides:

- **Clear Prerequisites**: What is needed before starting
- **Step-by-Step Instructions**: Numbered steps that can be followed sequentially
- **Complete Examples**: Full YAML configurations with all required fields
- **Verification Steps**: How to confirm the task completed successfully
- **Error Handling**: Common issues and how to resolve them

## Quick Navigation

| Skill | Purpose | Key Output |
|-------|---------|------------|
| [Installation](installation.md) | Install SC CLI | Working `sc` command |
| [DevOps Setup](devops-setup.md) | Create infrastructure | `server.yaml` |
| [Service Setup](service-setup.md) | Deploy services | `client.yaml` |
| [Deployment Types](deployment-types.md) | Select deployment type | `type` field in client.yaml |
| [Secrets Management](secrets-management.md) | Configure credentials | `secrets.yaml` |
| [Cloud Providers - AWS](cloud-providers/aws.md) | AWS setup | AWS authentication |
| [Cloud Providers - GCP](cloud-providers/gcp.md) | GCP setup | GCP authentication |
| [Cloud Providers - Kubernetes](cloud-providers/kubernetes.md) | K8s setup | kubeconfig |

## Prerequisites for All Skills

Before using any SC skill, ensure you have:

1. **Docker** - Required for building and running containers
2. **Git** - For repository management
3. **Cloud Provider CLI**:
   - AWS: `aws` CLI installed and configured
   - GCP: `gcloud` CLI installed and configured
   - Kubernetes: `kubectl` installed
4. **Access to Cloud Account** - With permissions to create resources

## Next Steps

Start with [Installation](installation.md) if you need to install the SC CLI, or choose the skill that matches your current task.