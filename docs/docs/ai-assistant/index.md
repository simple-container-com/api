# AI Assistant

`sc assistant` is an interactive CLI that helps scaffold SC configurations
for an existing project. It analyses the repository (language, build system,
existing Dockerfile / docker-compose, cloud signals) and produces
`server.yaml`, `client.yaml`, and supporting files matching the detected
shape.

Two modes:

- **Developer mode** — generates `client.yaml`, `docker-compose.yaml`, and
  a `Dockerfile` for an application. Run from inside an app repo.
- **DevOps mode** — generates a `server.yaml` plus provisioner config and
  secret-backend wiring. Run from inside a platform/infrastructure repo.

```bash
# In an app repo
sc assistant dev

# In a platform repo
sc assistant devops
```

## MCP server

For programmatic / agentic use, `sc assistant` also exposes an MCP server.
Tools like Claude Code, Cursor, and [Forge](https://simple-forge.com)
consume it natively — they call SC primitives (templates, resources,
parent stacks) as first-class operations rather than guessing YAML.

```bash
sc assistant mcp
```

See the SC repo on GitHub for the MCP tool schema, deeper usage examples,
and the developer/devops mode internals. Those pages will surface in this
nav after a content audit pass.
