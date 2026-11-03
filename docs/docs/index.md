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
AWS, GCP, Yandex Cloud, Cloud.ru, or any conformant Kubernetes cluster.

**SC is agent-first.** The same primitives drive the CLI and `sc`'s built-in
MCP server, so Claude Code, Cursor, and the hosted [Simple Forge](https://simple-forge.com)
platform work the same configurations you'd write by hand. If a doc page
describes a workflow, there is also a matching Forge skill (and an MCP tool)
that executes it.

## Start here

- [Installation](getting-started/installation.md) — get the `sc` CLI on your machine, including the `sc assistant mcp` server for agent integrations.
- [Quick Start](getting-started/quick-start.md) — deploy a first service end-to-end.
- [Main Concepts](concepts/main-concepts.md) — parent stacks, service stacks, and how they compose (the same vocabulary agents use).
- [AI Assistant](ai-assistant/index.md) — the full agent-first surface: `sc assistant`, the MCP server, and the Forge skill catalogue.
- [Reference](reference/supported-resources.md) — full list of resources, templates, and config keys.
- [Guides](guides/index.md) — task-focused walkthroughs (ECS Fargate, GKE Autopilot, Yandex Cloud, Cloud.ru, pure Kubernetes, secrets, migration).

## Forge — fully-agentic DevOps built on SC

[Simple Forge](https://simple-forge.com) is our hosted agent platform. It
emits SC YAML natively via the MCP server, so if you drive deployments
through Forge, every workflow run produces the same `server.yaml` /
`client.yaml` shapes documented here. Forge publishes a catalogue of
**skills** — `sc-parent-setup`, `sc-init-secrets`, `sc-service-deploy`,
`sc-debug`, `sc-destroy-rollback`, `sc-caveats`, and more — each
mirroring a workflow this documentation describes. If a doc and the
matching skill ever drift, that's a bug worth reporting.

## Help

Issues and questions: [support@simple-container.com](mailto:support@simple-container.com)
or [GitHub](https://github.com/simple-container-com).