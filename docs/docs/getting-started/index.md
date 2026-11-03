# Getting Started with Simple Container

Welcome to Simple Container! This section will help you get up and running quickly with your first deployment.

## Prerequisites

Before you begin, make sure you have:

- **Docker** installed and running on your machine
- **Cloud account** (AWS, GCP, Yandex Cloud, Cloud.ru, or a Kubernetes cluster) with appropriate permissions
- **Basic familiarity** with docker-compose for local development
- **A public key pair** (`ssh-rsa` or `ssh-ed25519`) for secrets encryption — covered in the [Installation](installation.md) guide

## Overview

Simple Container follows a simple workflow:

1. **Create a parent stack** - Define your cloud infrastructure (databases, storage, etc.)
2. **Create service stacks** - Define your applications that use the infrastructure
3. **Deploy locally** - Test with `docker-compose up`
4. **Deploy to cloud** - Deploy with `sc deploy`

!!! tip "Secrets are a first-class step, not an afterthought"
    Every `secrets.yaml` file must be registered with `sc secrets add` and
    encrypted with `sc secrets hide` BEFORE being committed. Skipping this
    is the single most common way SC configurations leak credentials. The
    [Installation guide](installation.md#step-4-create-secretsyaml) walks
    through the correct sequence.

!!! info "Driving SC with an AI agent?"
    `sc` ships an MCP server (`sc assistant mcp`) that Claude Code, Cursor,
    and the hosted [Simple Forge](https://simple-forge.com) platform
    consume natively. Agents call SC primitives (templates, resources,
    parent stacks, secrets) as first-class tools instead of guessing YAML.
    See the [AI Assistant guide](../ai-assistant/index.md) and the
    [Agent-first operations](installation.md#agent-first-operations)
    section of the installation guide.

## Your First Deployment Journey

## Getting Started Steps

1. **[Installation](installation.md)** - Install Simple Container CLI and set up your environment
2. **[Quick Start](quick-start.md)** - Deploy your first application in minutes
3. **[Core Concepts](../concepts/main-concepts.md)** - Understand the fundamental concepts

## Next Steps

Once you've completed the getting started guide, explore:

- **[Deployment Guides](../guides/index.md)** - Step-by-step tutorials for different platforms
- **[Examples](../examples/README.md)** - Real-world configuration examples
- **[Reference Documentation](../reference/supported-resources.md)** - Complete API reference

## Need Help?

If you have any issues or questions, please reach out at [support@simple-container.com](mailto:support@simple-container.com).