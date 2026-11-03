# AI Assistant

`sc assistant` is the agent-first surface of Simple Container. It ships two
things: an **interactive CLI** that scaffolds configurations into an existing
repo, and an **MCP server** that exposes every SC primitive to AI coding
agents (Claude Code, Cursor, [Simple Forge](https://simple-forge.com)) as
first-class tools.

Agents and operators share the same vocabulary: templates, resources,
parent stacks, secrets. The CLI uses it when you type; the MCP server
exposes it when an agent asks. Both end up calling the same `sc`
primitives, so a parent stack scaffolded by an agent is byte-identical to
one scaffolded by `sc assistant devops`.

---

## The two modes

### Developer mode — in an app repo
```bash
sc assistant dev
```
Generates `client.yaml`, `docker-compose.yaml`, and a `Dockerfile` by
reading the repo (language, build system, existing CI, cloud signals) and
asking only the questions that are not already answered by the code.

### DevOps mode — in a platform / infrastructure repo
```bash
sc assistant devops
```
Generates `server.yaml`, provisioner config (state storage + secrets
provider), and the secrets wiring to let the parent stack provision
cloud resources. Matches the shape of the per-cloud parent guides
([ECS Fargate](../guides/parent-ecs-fargate.md),
[Yandex Cloud](../guides/parent-yandex-cloud.md),
[Cloud.ru](../guides/parent-cloudru.md),
[GKE Autopilot](../guides/parent-gcp-gke-autopilot.md),
[Kubernetes](../guides/parent-pure-kubernetes.md)).

---

## The MCP server

```bash
sc assistant mcp
```
Starts a local stdio-mode MCP server. Any client that speaks the MCP
protocol — Claude Code's `/mcp add`, Cursor's `settings.json`
`mcpServers`, Simple Forge's agent runtime — can connect and call SC
primitives directly. The agent sees the actual tool schema (templates
available, resource types owned by each plugin, placeholder expansion
rules); it does not have to guess YAML.

Key tools the MCP server exposes:

- **Stack primitives**: `list_stacks`, `read_stack`, `scaffold_stack`
- **Secrets lifecycle**: `secrets_add`, `secrets_hide`, `secrets_reveal`,
  `secrets_init`, `secrets_add_recipient`
- **Provision + deploy**: `provision`, `deploy`, `destroy`, `preview`
- **Plugin surface**: `list_plugins`, `plugin_schema`, `register_plugin`

See [MCP integration](mcp-integration.md) for the full schema and
client-side wiring examples.

---

## Agentic DevOps patterns

### Local-dev loop
An agent in your editor runs through the Getting Started flow for you:

1. **`sc assistant dev`** — scaffold `client.yaml` + `Dockerfile` for the
   current app.
2. **`sc secrets add` + `sc secrets hide`** — register and encrypt any
   plaintext credentials written in step 1.
3. **`sc deploy -s <stack> -e staging`** — ship it.
4. On failure, the agent reads the SC error output (`unknown template
   type …`, `unresolved placeholder …`) and either edits the YAML or
   asks you a focused question.

### Fully-agentic SDLC with Simple Forge

[Simple Forge](https://simple-forge.com) is the hosted platform that
runs the full SDLC against your repo through the same MCP surface. Each
operation it supports is published as a **skill** — a prompt an agent
follows, built around `sc` primitives:

| Skill | What it does |
|---|---|
| `sc-intake` | Reads a repo and classifies it (language, cloud, existing CI). |
| `sc-parent-setup` | Opens a PR with a parent stack for the chosen cloud. |
| `sc-init-secrets` | Initialises `.sc/cfg.{default,github}.yaml`, registers all plaintext secret files with `sc secrets add` + `sc secrets hide`, injects `SIMPLE_CONTAINER_CONFIG` into the GitHub repo. |
| `sc-add-service` | Opens a PR adding a `client.yaml` for a new service. |
| `sc-service-deploy` | Dispatches the service's deploy workflow and reports back. |
| `sc-static-deploy` | Same for static-site stacks. |
| `sc-debug` | Diagnoses a failed deploy from the GitHub Actions log + the SC error output. |
| `sc-destroy-rollback` | Rolls a parent back on a bad migration. |
| `sc-caveats` | Enumerates known SC gotchas (version pinning, env resets, build args, etc.) relevant to the current change. |

Each skill mirrors a workflow this documentation describes. If a doc
page and the matching skill disagree, that's a bug — report it so the
two stay co-canonical.

---

## Agent-safe secrets discipline

An agent that writes or edits a secret file MUST:

1. Run `sc secrets add <path>` **before** staging it.
2. Run `sc secrets hide` **before** `git add` / `git commit`.
3. **Never** run `git add -A` or `git add .` in a directory containing
   a plaintext secret file.

`sc` is fail-closed by default — it refuses to encrypt a file with no
registered recipients, and `secrets.yaml` files are auto-added to
`.gitignore` on `sc secrets add`. These invariants are what let an
agent run unsupervised: the system itself blocks the common
misuse, not the agent's judgment.

See the [installation guide](../getting-started/installation.md#agent-first-operations)
for the full discipline and the
[Secrets Management guide](../guides/secrets-management.md) for recipient
types, rotation, and recovering from an accidental commit.

---

## Where to go next

- **New to SC?** Start with the [installation guide](../getting-started/installation.md),
  which includes the agent-first entry points.
- **Setting up a parent stack via an agent?** The matching
  [parent-stack guide](../guides/index.md) now has an "Agent-driven
  setup" callout; every step in the guide is driveable via MCP.
- **Writing a plugin?** The [plugin authoring guide](../guides/plugin-authoring.md)
  covers how plugins expose their types to the MCP server via Schema RPC.
- **Looking for the tool schema?** See [MCP integration](mcp-integration.md).
