---
title: SC Components
description: A Component is a managed service you declare in server.yaml and consume with uses:. deployTo: picks the template; the plugin defaults everything else.
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2026-11-25'
---

# SC Components

An **SC Component** is a managed service that SC itself deploys and operates for
you — a backup agent, a monitoring collector, a notification gateway — declared
in the parent stack (`server.yaml`) like any other resource, and consumed by a
service stack (`client.yaml`) by naming it in `uses:`.

A Component sits between the two things SC already had:

| | Written by | Deployed by | Consumed by |
|---|---|---|---|
| **Resource** (`mongodb-atlas`, `s3-bucket`) | operator | its own plugin | `uses:` → env vars |
| **Service** (`client.yaml`) | service owner | a template | — |
| **Component** (`sc-backup`, `sc-notifier`) | operator | **a template the operator picks with `deployTo:`** | `uses:` → env vars |

The distinguishing property is that middle column. A Component is not a cloud
API wrapper — it is *a real service with its own image, its own HTTP endpoint
and its own database*, and the operator chooses **where it runs** by naming one
of their own `templates:` entries. The same `sc-backup` Component runs on
Lambda in one account and on ECS Fargate in another with a one-word change.

!!! note "Version floor"
    Components with defaulted required fields need **`sc` ≥ 2026.11.86**. Before
    that release, parse-time validation ran on the operator's raw config rather
    than the effective config, so a plugin default for a required key
    (`deployTo`, `image`) could never win — the operator had to write it. On an
    older `sc` the slim blocks below fail to parse.

    [`uses:` on a Component](#a-component-can-consume-another-component) needs a
    release **after `2026.11.86`**. On an older one it is rejected at parse time
    as an unknown top-level key — loud rather than silently unwired, but it does
    mean the `uses:` line and the version bump have to land together.

## Declaring one: ask for the bare minimum

A Component block should contain **only what SC cannot know**. Everything the
dispatched template needs — sizing, timeouts, the IAM role set, URL routing, the
worker cluster, schedules, the image, most env vars — is the plugin's own
*defaults*, merged under your config at deploy time.

```yaml
# server.yaml
resources:
  resources:
    staging:
      template: stack-per-app
      resources:
        my-backup:
          type: sc-backup
          config:
            # THE ONE REQUIRED INPUT: names an entry in this stack's
            # top-level `templates:` block. Everything the dispatched
            # template needs is inferred from it.
            deployTo: lambda

            # Where it answers. SC wires the DNS itself (see below).
            ingress:
              hostname: backup.example.com

            # Secrets you own. Always `${secret:...}`, never a literal.
            signingKey:       "${secret:my-backup-signing-key}"
            operatorPassword: "${secret:my-backup-operator-password}"
```

That is a working Component. What you did **not** write, and why:

| Field you didn't write | Where the value comes from |
|---|---|
| `image` | the plugin's own stamped release version — one release publishes the binary and the image under one CalVer tag, so the image can never drift from the plugin that dispatches it |
| `maxMemory`, `timeout` | the plugin's `component_defaults` for this component type |
| `cloudExtras.*` (IAM roles, `lambdaRoutingType`, `lambdaSchedules`) | same — including the scheduler tick, which is an EventBridge rule hitting an HTTP endpoint, not a goroutine (see [Why schedulers are EventBridge rules](#why-schedulers-are-eventbridge-rules)) |
| `env.*` | same, including values the plugin looks up in your account at deploy time (default VPC subnets, security groups) |
| `worker` (the Fargate cluster one-shot jobs run on) | same |
| `storage` | same — `sc-<type>-${name}-...--${env}` by convention |

### When you *do* need to write a field

Your value always wins. Three cases where that matters:

- **An existing resource.** A plugin's `storage.bucket` default is a *name
  template*; if you already have a bucket with history in it, write it out.
  Omitting it renames the bucket, which on most clouds means a new, empty one.
- **A non-default template.** `deployTo: ecs` instead of `deployTo: lambda`
  selects a different default set, because the defaults are declared per
  component type and read `${template.*}` tokens.
- **A genuine option.** `tenantEnvironments`, notification channels, severity
  thresholds — things no plugin can guess.

Write a field *because you mean it*, and leave a comment saying why. A field
that merely repeats the default is a field that will silently diverge from the
plugin on the next bump.

### What `ingress.hostname` does

You give a hostname; SC gives the Component a domain. Mechanically, core
**strips** `ingress.hostname` before dispatching the template, then wires DNS
*itself* after the dispatch using the endpoint URL the template published. You
never declare a `domain:` on the Component, and the template plugin never sees
the hostname.

Two consequences worth knowing before your first deploy:

- Your parent stack needs a **registrar**. Without one the dispatch fails with
  `stack has no registrar configured; cannot wire ingress.hostname on a
  Component` — deliberately loud rather than deploying an unreachable service.
- A brand-new hostname usually needs **two provisions**: the first creates the
  endpoint and the DNS record, the second lets anything that reads the
  hostname (the Component's own config, a consumer's env) see it resolved.

## Consuming one: `uses:` is the whole wiring

There is **one** mechanism, and it is the same one every SC resource uses. A
consumer — a deployed service, or [a sibling
Component](#a-component-can-consume-another-component) — names the Component in
its `uses:` block; SC fans out to that Component's registered compute processor;
the processor registers the consumer as a **tenant** and injects whatever it
needs to talk to the Component.

```yaml
# client.yaml
stacks:
  staging:
    type: cloud-compose
    config:
      uses:
        - mongodb
        - my-backup        # ← the whole wiring
        - my-observatory
```

On deploy, that one line gets the service:

- a **tenant row** on the Component (push-registered by the processor),
- a **per-consumer credential**, minted fresh — nothing hand-generated, nothing
  in your secret registry,
- **env vars** on the deployed service (`BACKUP_AGENT_URL` + `BACKUP_TOKEN` for
  `sc-backup`; `SC_NOTIFY_URL` + `SC_NOTIFY_TOKEN` for `sc-notifier`;
  `OBSERVATORY_AGENT_URL` for `sc-observatory`). The token lands as a *secret*
  env var, so it is a secret-manager reference at rest, not a plaintext value
  on the function.

!!! warning "`uses:` is ordered, and the order is load-bearing"
    Each entry's compute processor gets a **fresh collector**, and a processor
    can only read env vars contributed by entries listed **before** it. A
    Component that discovers its consumer's databases from env (like
    `sc-backup`, which finds `MONGO_URI` to know what to dump) must be listed
    **after** the resource that sets them:

    ```yaml
    uses:
      - mongodb          # sets MONGO_URI
      - my-backup        # reads it — must come after
      - my-observatory   # snapshots the effective config — last
    ```

    Listed first, `my-backup` sees nothing and silently registers a tenant with
    no databases. This is invisible in a deploy log: registration succeeds.

### A Component can consume another Component

The same one line works in a **Component's own** block, so wiring a backup agent
to a notifier is `uses:`, not a hostname reference plus a hand-minted token:

```yaml
# server.yaml
my-notifier:
  type: sc-notifier
  config:
    deployTo: lambda
    ingress: { hostname: notify.example.com }

my-backup:
  type: sc-backup
  config:
    deployTo: lambda
    ingress: { hostname: backup.example.com }
    uses:
      - my-notifier      # gets SC_NOTIFY_URL + SC_NOTIFY_TOKEN, minted per consumer
```

Three properties of this worth knowing:

- **Declaring it in config is what makes the ordering safe.** `my-backup` sorts
  first alphabetically, and a reference living only in a plugin default cannot
  influence provision order — so SC would provision the consumer before the
  provider and `${resource:…}` would fail open into a literal. Naming the
  dependency in `uses:` is visible to the ordering pass, which is why this is a
  config key and not something SC infers.
- **The consumer identity is `<parentStack>--<resourceName>`** —
  `infra--my-backup`, not `my-backup`. Every component lives in the one parent
  stack, so the stack name alone could not tell two publishers apart. This is
  the string you will see in the provider's tenant list and in alerts.
- **Injected names must start with `SC_`.** SC refuses to inject anything else
  into a workload, so a plugin bump can never silently shadow an application's
  own `DATABASE_URL` or `PATH`.

If you need a different config shape than the injected env vars,
`${resource:my-notifier.url}` and `${resource:my-notifier.token}` are available
— but note the asymmetry: `url` is one value for everyone, while `token` is
scoped to a consumer, so it resolves **only** for a resource that declares
`uses: [my-notifier]`.

### Credentials rotate on every consumer deploy

The per-consumer credential is **minted on each deploy**, not reused. That is
the intended design — nothing long-lived, nothing to rotate by hand — but it
has a consequence: if registration fails after the mint, the Component's stored
credential is already stale. `sc-backup` deliberately **hard-fails** the
consumer's deploy in that case (a backup that silently stops backing up is the
worst outcome); `sc-notifier` and `sc-observatory` **warn** and continue, and
recover on the next deploy.

## Why schedulers are EventBridge rules

A Component deployed to Lambda has **no long-running process**. There is no
place to put a `for { time.Sleep(…) }` loop: the container freezes between
invocations, and a timer that survives the freeze fires at an arbitrary later
time or not at all.

So every periodic job in a Component is an **EventBridge rule invoking an HTTP
endpoint on the Component itself**, authenticated with a scheduler token. The
plugin declares the rule in its `component_defaults`
(`cloudExtras.lambdaSchedules`) and puts the token into the rule's request
payload for you — which is why you never see a 20-line `lambdaSchedules` block
in a Component declaration.

Two things this constrains:

- **Schedule names must be short.** A name over **14 characters** is accepted at
  preview and rejected at apply. If a preview is green and the apply fails on a
  name, this is why.
- **Writes must be blind.** Read-modify-write across invocations is not safe on
  a frozen-between-calls runtime with a replicated store behind it. Component
  services are written against that constraint.

## Errors you will actually hit

| Message | Cause |
|---|---|
| `unknown or disallowed top-level key "x" for component type "sc-y"` | `x` is neither a well-known Component key (`deployTo`, `image`, `signingKey`, `env`, `ingress`, `uses`) nor on the plugin's allowlist. Usually a typo, or a key from a newer plugin version than the one loaded. Checked against **your** config, never the defaults |
| `template "lambda" (defaulted by the "sc-backup" plugin (not written in your config)) not declared in top-level templates: block` | you relied on the plugin's `deployTo` default and have no template by that name. Either declare the template or write `deployTo:` explicitly. The provenance clause is there so a name you never typed is traceable |
| `template "x" (type "y") is not accepted by any loaded plugin — the plugin's Schema must advertise supports_component: ["y"]` | the template you picked belongs to a plugin that hasn't opted into Component dispatch. Without the opt-in the dispatch would silently behave as pass-through, so it is refused |
| `operator-written value at "k" uses reserved prefix "$$sc:" — this prefix is reserved for plugin-authored defaults` | `$$sc:` marks a *plugin* default as interpolatable. Copy-pasting one into `server.yaml` is refused; write the resolved value instead |
| `stack has no registrar configured; cannot wire ingress.hostname on a Component` | the Component declares a hostname and the parent stack has no DNS registrar |
| `declares "uses: [x]" but no resource named "x" is declared in env "staging" (available: …)` | a typo in a `uses:` entry, or the sibling is declared in a different environment. The available names are listed for you |
| `declares "uses: [x]" — a resource cannot consume itself` | self-reference in `uses:`; there is no provider to fan out to |
| `tried to inject env variable "NOTIFY_URL" into a consumer, but SC only injects names matching ^SC_[A-Z0-9_]+$` | a provider plugin published an unprefixed name. Not your config's fault — report it to the plugin; an unprefixed injection could shadow your application's own variable |
| `component "my-backup" references ${resource:my-notifier.token}, which did not resolve` | the key you referenced is not one the used resource's plugin publishes. Refused rather than deployed, because `${resource:…}` resolution otherwise fails open — see the row below |
| a `${resource:…}` token appears **literally** in a deployed env var | an ordering problem: the value was referenced before the resource that publishes it provisioned. Placeholder resolution fails open by design, so this does not error — name the dependency in `uses:` so ordering sees it |

## Inspecting the effective config

The config SC provisions is the merge, not what you wrote. Two reliable ways to
see it:

- Each Component plugin's **README** carries an "Operator config" table listing
  every key with its default.
- The plugin's source has one `defaults := map[string]any{…}` literal (for
  example `plugin/cmd/sc-component-backup/main.go`), which is the actual bytes
  core merges.

And after a deploy, read it off the deployed thing: the Lambda's environment,
the ECS task definition, the EventBridge rule's target input. A field you
believe is defaulted but is absent there was never defaulted.

## Available Components

| Type | Repo | What it does |
|---|---|---|
| `sc-backup` | [sc-component-backup](https://github.com/simple-container-com/sc-component-backup) | scheduled database dumps to object storage, per-tenant, with a restore UI |
| `sc-observatory` | [sc-component-observatory](https://github.com/simple-container-com/sc-component-observatory) | fleet inventory + monitoring: version, commit, deploy time, effective config (secrets redacted) per service |
| `sc-notifier` | [sc-component-notifier](https://github.com/simple-container-com/sc-component-notifier) | outbound notifications (Telegram, email) with severity gating and per-publisher tokens |

## See also

- [Authoring an SC Component](../guides/component-authoring.md) — the plugin-side
  contract: `component_defaults`, merge semantics, the dispatch sequence.
- [Authoring an SC Plugin](../guides/plugin-authoring.md) — the surrounding
  plugin mechanics.
- [Template Placeholders](template-placeholders.md) — `${secret:…}`,
  `${resource:…}`, `${dependency:…}` in operator config.
- [Main Concepts](main-concepts.md) — parent vs service stacks.
