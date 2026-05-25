---
title: Simple Container
description: Open-source CLI for declaring and deploying cloud infrastructure as YAML.
platform: platform
product: simple-container
category: devguide
subcategory: index
date: '2024-06-12'
---

# Simple Container

Simple Container (SC) is an open-source CLI for declaring and deploying cloud
infrastructure as YAML. A `server.yaml` describes shared infrastructure (databases,
queues, secrets, registrars, deployment templates); a `client.yaml` describes a
service that consumes it. SC translates both into Pulumi calls and ships them to
AWS, GCP, or any conformant Kubernetes cluster.

## Start here

- [Installation](getting-started/installation.md) — get the `sc` CLI on your machine.
- [Quick Start](getting-started/quick-start.md) — deploy a first service end-to-end.
- [Main Concepts](concepts/main-concepts.md) — parent stacks, service stacks, and how they compose.
- [Reference](reference/supported-resources.md) — full list of resources, templates, and config keys.
- [Guides](guides/index.md) — task-focused walkthroughs (ECS Fargate, GKE Autopilot, pure Kubernetes, secrets, migration).

## Forge — built on SC

[Forge](https://simple-forge.com) is our AI workflow engine. It emits SC YAML
natively via the MCP server, so if you drive deployments through Forge, every
workflow run produces the same `server.yaml` / `client.yaml` shapes documented
here. The integration is first-class — Forge speaks SC primitives, not a
translation layer.

## Help

Issues and questions: [support@simple-container.com](mailto:support@simple-container.com)
or [GitHub](https://github.com/simple-container-com).
