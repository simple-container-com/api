---
title: Authoring an SC Component
description: The plugin-side contract for an SC Component — Schema opt-ins, component_defaults and the $$sc: sentinel, the dispatch sequence, PreProvision, and the runtime constraints.
platform: platform
product: simple-container
category: devguide
subcategory: authoring
date: '2026-11-25'
---

# Authoring an SC Component

An **SC Component** is a managed service SC deploys on the operator's behalf,
onto a compute target the operator picks with one line (`deployTo:`). This page
is the **plugin-author** side of that contract. For the operator's view — how a
Component is declared and consumed — read
[SC Components](../concepts/sc-components.md) first; this guide assumes it.

A Component repo has **three** modules, not the usual two:

| Module | What it is | Who links it |
|---|---|---|
| `api/` | the config type (`BackupConfig`), its `ResourceType` token, `init()` registration | `sc` and the plugin |
| `plugin/` | the gRPC plugin: `Schema`, `PreProvision`, `Provision`, `ComputeProcess` | `sc` over the local bus |
| `service/` | **the actual deployed application** — the HTTP service, its database access, its scheduler endpoints | nothing; it ships as an image |

That third module is what makes a Component different from a cloud plugin. You
are authoring a *service* and the deployment machinery that places it, in one
repo, released under one version — because the plugin defaults
[`image` from its own stamped version](#image-and-version-move-together), so the
two can never drift.

Prior art to read before starting: `sc-component-notifier` is the smallest
complete Component; `sc-component-backup` is the one that exercises every seam
on this page (PreProvision lookups, peer-invoke, Fargate workers, scheduler
ticks).

## 1. The dispatch, end to end

Everything on this page is a position in one sequence. `dispatchComponentTemplate`
(`pkg/clouds/pulumi/plugin_sidecars.go`) is the whole mechanism:

```
                                    ┌─ your plugin's Schema() supplied:
                                    │    component_resource_types
                                    │    component_defaults
                                    │    component_allowed_overrides
                                    │    component_secret_env_map
                                    ▼
  parse time   ValidateComponentConfigs
               ├─ merge defaults under the operator's raw
               ├─ require deployTo + image on the EFFECTIVE config
               ├─ allowlist top-level keys against the OPERATOR's raw
               └─ refuse `deployTo:` at a template whose plugin has not
                  advertised `supports_component`

  deploy time  PreProvision(your plugin)        ← operator's RAW, pre-defaults
               │  cloud lookups; publish tpl extensions on the collector
               ▼
               dispatchComponentTemplate
               ├─ resolve `deployTo:` → a `templates:` entry → its ProvisionFunc
               ├─ guard: overrides allowlist, `$$sc:` refusal (operator raw)
               ├─ mergeComponentDefaults      ← yours UNDER the operator's
               ├─ InterpolateDefaults         ← `$$sc:` strings substituted
               ├─ ResolvePlaceholders         ← `${resource:…}` incl. PreProvision's
               ├─ applyComponentSecretEnvMap  ← `${secret:…}` → env var names
               ├─ ── snapshot the merged raw ──
               ├─ strip `ingress.hostname`
               ├─ Provision(template plugin)   ← merged raw, minus ingress
               └─ wireComponentDomain          ← core does DNS itself
               │
               ▼
               Provision(your plugin)          ← the SAME merged raw (snapshot)
                 your own resources; the template's endpoint arrives as a
                 dependency output

  consumer     ComputeProcess(your plugin)     ← once per consumer `uses:` entry
  deploy         mint a credential, inject env, register a tenant
```

Three positions in that sequence are where authors get surprised, and each has
its own section below: [the two `Provision` calls](#4-provision-both-of-them),
[PreProvision's pre-defaults view](#3-preprovision-the-one-asymmetry), and
[the per-consumer collector](#5-computeprocess-being-consumed).

## 2. Schema — the opt-ins

Nothing about Components is inferred from a Go type. Dispatch is **set
membership on a string your Schema advertises**, never an interface assertion —
plugin-owned configs reach core as an opaque `*api.PluginFieldConfig`, so a type
assertion that compiles will fail in production.

```go
return &scpluginv1.SchemaResponse{
    // ...
    ComponentResourceTypes: []string{backupApi.ResourceType},   // 1
    ComponentDefaults: map[string][]byte{                        // 2
        backupApi.ResourceType: componentDefaultsJSON,
    },
    ComponentAllowedOverrides: map[string]*scpluginv1.StringList{ // 3
        backupApi.ResourceType: {Values: []string{
            "image", "env", "ingressRoutes", "cloudExtras",
            "worker", "databases", "storage", "metadata", "operatorPassword",
        }},
    },
    ComponentSecretEnvMap: map[string]*scpluginv1.StringMap{      // 4
        backupApi.ResourceType: {Entries: map[string]string{
            "signingKey":       "SERVICE_JWT_SECRET",
            "operatorPassword": "BACKUP_OPERATOR_PASSWORD",
        }},
    },
    SupportsPreProvision: true,                                   // 5
    PeerCapabilities:     []string{},                             // 6
}
```

**1. `component_resource_types`** — the types this plugin owns *as Components*.
Advertising a type here is what makes core treat it as a Component at all
(`IsComponentResourceType`).

**2. `component_defaults`** — JSON `map[string]any` per type, merged **under**
the operator's config. [Section 6](#6-component_defaults-and-the-sc-sentinel).

**3. `component_allowed_overrides` — note the dual use.** This list does two
jobs, and the second one catches everybody:

- it is the set of `.Raw` keys your plugin may **overlay onto the dispatched
  template**; and
- core's validator **reuses it as the allowed-top-level-keys list** for the
  Component's config.

So every config key your type owns beyond the five well-known ones
(`deployTo`, `image`, `signingKey`, `env`, `ingress`) must appear here or the
operator's config fails to parse with `unknown or disallowed top-level key`.
Keys you deliberately leave off are **operator-locked** — `vpc`, `subnets`,
`iam` are off by convention, so a plugin can never point an operator's function
at a network they didn't choose. The allowlist is checked against the
**operator's** raw only; your own defaults are plugin-authored by construction
and need no entry.

**4. `component_secret_env_map`** — for each config key holding a
`${secret:…}` reference, the env var name the deployed service should receive
it as. Core registers the mapping; the template plugin's overlay reads it and
emits `cfg.Secrets[<ENV_VAR>]`, so the value travels the normal secrets path
and lands as a secret-manager reference, not a plaintext env var. Declare your
secrets here rather than asking the operator to write `env:` entries.

**5. `supports_pre_provision`** — [section 3](#3-preprovision-the-one-asymmetry).

**6. `peer_capabilities`** — capability names other plugins may call over
`PeerInvoke`. Core's broker refuses any capability not on this list, so the
safe default is an empty list. Use it when your Component needs an operation
that belongs to another plugin's cloud: `sc-plugin-mongodb` exposes
`mint-scoped-user` / `delete-scoped-user`, which is how `sc-backup` gets a
scoped database user without linking the Atlas SDK.

!!! warning "`supports_component` is NOT yours to advertise"
    `supports_component` names **template** types, and it belongs to the plugin
    that owns the *template* (`sc-plugin-aws` advertises `aws-lambda`). It means
    "my `ProvisionFunc` knows how to read `ProvisionRequest.component_raw` and
    overlay the Component's fields onto my typed input struct". A Component
    plugin never sets it. If `deployTo:` names a template whose owner hasn't
    opted in, core refuses the whole stack rather than deploying a pass-through.

## 3. PreProvision — the one asymmetry

`PreProvision` exists for one reason: a deployed function's environment is set
at create time, so any value you need to *look up* in the operator's cloud must
land **before** the template is dispatched. Opt in with
`SupportsPreProvision: true`, do your lookups in the RPC (it runs in a Pulumi
context, so cloud SDK calls are legal), and publish results as template
extensions:

```go
collector.AddResourceTplExtension(req.GetResourceName(), map[string]string{
    "subnets-csv":        strings.Join(subnetIDs, ","),
    "security-groups-csv": sgID,
    "cluster-name":       clusterName,
})
```

Your `component_defaults` then reference them as
`"$$sc:${resource:${name}.subnets-csv}"`, and core's post-PreProvision
`ResolvePlaceholders` pass substitutes the real value into the merged config
before the template sees it.

!!! danger "PreProvision receives the operator's RAW config, pre-defaults"
    This is the **only** place in the Component contract where you do not see
    your own defaults, and it is structural: the defaults merge happens inside
    the dispatch, which by construction runs after this RPC returns.

    A branch keyed on a value the operator may have omitted therefore reads
    empty. `if raw["deployTo"] == "lambda"` is the classic: once `deployTo` is
    defaultable, that branch silently no-ops, your lookup never runs, and the
    `${resource:…}` tokens it was meant to feed survive into the deployed env
    **as literal strings** — placeholder resolution fails open, so nothing
    errors.

    Reproduce the merge yourself; it is side-effect-free:

    ```go
    eff := pApi.DeepMergeComponentRaw(pApi.ComponentDefaults[resourceType], raw)
    ```

## 4. `Provision` — both of them

A Component dispatch makes **two** `Provision` calls: the dispatched template's,
and your own. Since 2026-11-25 **both receive the same merged config** on
`ProvisionRequest.component_raw` — before that, your own call got the operator's
raw with `component_raw` unset, which made every defaulted field invisible to
your own provisioning code (`deployTo: lambda requires a worker block` on a
stack whose `worker` was entirely defaulted).

Read it with the fallback pattern, so one plugin binary works across `sc`
versions:

```go
raw := req.GetComponentRaw()          // the merged, effective config
if len(raw) == 0 {
    raw = scPlugin.InputsToPlain(req.GetInputs())  // older core
}
```

Two details about the snapshot your call receives:

- it is taken **before** core strips `ingress.hostname`, so you *do* see the
  hostname (you published it; the template plugin must not, because its
  registrar is nil across the plugin bus);
- it is a **deep copy**, so mutating it cannot disturb the template's view.

The template's endpoint URL arrives on your `DependencyOutputs` — that is how
you learn where your own service ended up answering.

## 5. `ComputeProcess` — being consumed

When a service names your Component in `uses:`, core fans out to your
`ComputeProcess` once per consumer. Do three things:

```go
// 1. mint a per-consumer credential — scoped to THIS consumer's stack
token, err := mintTenantToken(stackName, signingKey)
// 2. inject it
collector.AddEnvVariableIfNotExist(urlVar, serviceURL, resType, resName, stackName)
collector.AddSecretEnvVariableIfNotExist(tokenVar, token, resType, resName, stackName)
// 3. publish the same values as template extensions, so an operator with a
//    custom config shape can write `${resource:my-notifier.token}`
collector.AddResourceTplExtension(resName, map[string]string{
    "url": serviceURL, "token": token,
})
```

Then register the consumer as a tenant on your running service.

**Namespace your env vars.** `SC_`-prefixed names are the reserved namespace for
anything SC injects into a workload, which is what makes auto-injection safe:
an unprefixed `NOTIFY_URL` would shadow an application's own variable of that
name. New Components should use `SC_<THING>_URL` / `SC_<THING>_TOKEN`. (The
older `BACKUP_AGENT_URL` / `OBSERVATORY_AGENT_URL` names predate the
convention and are kept for compatibility — do not copy them.)

!!! warning "The per-consumer collector, and the leak it prevents"
    `AddResourceTplExtension` **overwrites** by resource name, and
    `AddEnvVariableIfNotExist` is **first-write-wins**. Two consumers of the
    same Component sharing one collector therefore means the second silently
    inherits the first's credential — and a single-consumer smoke cannot see
    it. Each `uses:` entry gets a **fresh** collector for exactly this reason.

    The regression test that matters: two consumers, assert two *distinct*
    credentials whose decoded tenant claims each match their **own** stack
    name. Comparing one consumer's value against an expectation passes either
    way.

**Registration failure: choose hard-fail or warn, and say which.** Both are
defensible and the choice is a product decision:

- **hard-fail** when silence is the worst outcome (`sc-backup`: a backup that
  stops backing up without telling anyone);
- **warn** when the state is recoverable on the next deploy (`sc-notifier`,
  `sc-observatory`).

Either way, note that the credential is minted *before* the registration call,
so a failed registration leaves your service holding a stale credential until
the next deploy re-mints. If your service can create a tenant lazily from a
first authenticated request, do that and let registration be an optimisation.

**Ordering.** Your processor can only read env vars contributed by `uses:`
entries listed **before** yours. Document the constraint in your README if your
Component discovers anything from the consumer's environment — the operator has
no other way to know.

## 6. `component_defaults` and the `$$sc:` sentinel

Defaults are a JSON `map[string]any` per component type. Build them in Go and
marshal once:

```go
var componentDefaultsJSON = func() []byte {
    defaults := map[string]any{
        "cloudExtras": map[string]any{
            "lambdaRoutingType": "function-url",
            "awsRoles":          []string{"s3:PutObject", "ecs:RunTask", "iam:PassRole"},
        },
        "storage": map[string]any{
            "type":   "s3",
            "bucket": "$$sc:sc-backup-${name}-archives--${env}",
            "region": "$$sc:${template.region}",
        },
        "worker": map[string]any{
            "cpu": 512, "memoryMB": 2048,
            "cluster": "$$sc:sc-backup-${name}-cluster",
        },
        "env": map[string]any{
            "BACKUP_ECS_SUBNETS": "$$sc:${resource:${name}.subnets-csv}",
        },
    }
    b, err := json.Marshal(defaults)
    if err != nil {
        panic("sc-component-backup: componentDefaultsJSON: " + err.Error())
    }
    return b
}()
```

### Merge semantics

`DeepMergeComponentRaw` — **operator wins, always**:

| Shape | Behaviour |
|---|---|
| nested maps | per-key union; recurse. Your keys the operator didn't write survive |
| **lists** | **operator REPLACES wholesale** — never appends |
| scalars | operator wins |

The list rule is deliberate: an append would silently reintroduce defaults the
operator may have meant to remove, and a half-plugin/half-operator list is a
value nobody can read off the config. It also means a list is all-or-nothing
for the operator — size your list defaults accordingly.

The operator-wins contract is the other half of the deal: a field the operator
*did* write must never change under a plugin version bump. Removing a historical
default is a migration, not a config change.

### The sentinel rule that costs an afternoon

`$$sc:` (**two** dollar signs) marks a default string as interpolatable. Core
checks it with `strings.HasPrefix`, which means:

!!! danger "The sentinel must be at position **0 of the entire string**"
    Not beside the token it guards. A default that is a whole JSON payload with
    a token in the middle needs the sentinel at the very front of the payload:

    ```go
    // WRONG — the prefix check fails, the string passes through verbatim
    "request": `{"token":"$$sc:${secret:sched-token}","path":"/tick"}`

    // RIGHT — sentinel first, whole payload interpolated
    "request": `$$sc:{"token":"${secret:sched-token}","path":"/tick"}`
    ```

    There is no error for getting this wrong. The literal `$$sc:…` reaches the
    template, and you find out when the deployed thing misbehaves.

An **operator**-written string with this prefix is refused at parse time
(`RefuseSentinelInOperatorRaw`, naming the key path) — the sentinel's real job
is to distinguish plugin-authored values from operator-authored ones, so
operator config cannot forge one.

### Token vocabulary

Inside a `$$sc:` string:

| Token | Resolves to | Resolved by |
|---|---|---|
| `${name}` | the component resource name (`my-backup`) | `InterpolateDefaults` |
| `${env}` | the dispatch environment (`staging`) | " |
| `${stack}` | the stack name | " |
| `${template.name}` | the `deployTo:` template's name | " |
| `${template.<key>}` | that template's config scalar (`${template.region}`) | " |
| `${parent:<res>.<field>}` | a parent-stack output | " (nil resolver ⇒ left in place) |
| `${parent-list:<res>.<field>}` | a parent-stack output **list** | " — see below |
| `${resource:<res>.<key>}` | a template extension (incl. your PreProvision's) | `ResolvePlaceholders`, later |
| `${secret:<name>}` | the secret registry | the normal secrets path |

Self-tokens substitute first, so they can nest inside a later token:
`${resource:${name}.subnets-csv}` becomes `${resource:my-backup.subnets-csv}`
and then resolves against the collector.

Two behaviours to know:

- **`${parent-list:…}` only works as the sole content** of the expression
  (whitespace aside). It replaces the string leaf with a real list, so there is
  no concatenation semantics — any other content leaves the string unchanged.
- **Unresolved tokens fail open.** After substitution the sentinel is retained
  only when an unresolved `${parent…}` token survives (a later pass may complete
  it); anything else loses the sentinel and the raw literal reaches the
  template. Nothing errors. This is why an unresolved `${resource:…}` shows up
  as a literal string in a deployed environment variable rather than as a
  failed deploy — and why your tests should assert resolved *values*, not
  absence of errors.

### `image` and `version` move together

Default `image` from the plugin's own stamped version, and release the binary
and the image under one tag:

```go
if version != "" && version != "dev" {
    defaults["image"] = "yourorg/sc-component-thing:" + version
}
```

Return no default for `dev`/empty so a local build doesn't claim a published
tag. This is what makes the operator's omission of `image` safe: the dispatching
plugin and the dispatched image are the same release by construction.

## 7. The service tier — what the runtime forbids

Your `service/` module runs wherever the operator pointed `deployTo:`, and the
cheapest target — Lambda — is also the most constrained. Write for it:

- **No long-running process.** The container freezes between invocations. A
  `for { time.Sleep(…) }` loop fires at an arbitrary later time, or never. Every
  periodic job must be an **HTTP endpoint** invoked by a schedule your
  `component_defaults` declare (`cloudExtras.lambdaSchedules`), authenticated
  with a scheduler token you put into the rule's request payload — which is the
  canonical use of the whole-payload sentinel above.
- **Schedule names ≤ 14 characters.** Longer names pass *preview* and fail at
  *apply*. A green preview proves nothing about this.
- **Blind writes only.** Read-modify-write across invocations is unsafe on a
  freeze-between-calls runtime over a replicated store: the read can come from
  a replica that hasn't seen the last write. Design the store API so updates
  are unconditional writes; where RMW is inherent (a "dismiss all" sweep),
  isolate it and say so in a comment.
- **A rekey is a delete plus an insert.** Changing a record's identity in place
  leaves the old row visible.

## 8. Testing

Unit-test the dispatch seams with fakes; none of this needs a cloud.

- **Defaults merge** — your defaults under a minimal operator config; assert the
  *effective* values, including one key the operator overrode and one list the
  operator replaced.
- **Interpolation** — assert the resolved string, not the sentinel's absence.
  `storage.bucket == "sc-backup-my-backup-archives--staging"` is a real
  assertion; `!strings.HasPrefix(v, "$$sc:")` passes for a value that was never
  interpolated at all.
- **The two `Provision` views** — assert your own call sees the merged map
  *including* `ingress`, that the template's view does **not** have
  `ingress.hostname`, and that mutating one doesn't change the other.
- **Per-consumer credentials** — two consumers, two distinct tokens, each
  claim matching its own stack.
- **`ComponentAllowedOverrides` completeness** — a config using every key your
  type documents must parse. This is the test that catches the dual-use trap
  when you add a field.

!!! tip "A test that passes on first write is not yet a regression test"
    Invert each guard, confirm you get the failure message you expected, then
    restore. The two that matter most here are the ones with no error path at
    all: a mis-positioned sentinel and an unresolved `${resource:…}` both
    produce *plausible-looking strings*, so only an inverted test proves your
    assertion would have caught them.

Then smoke it live. For a Component specifically: read the values **off the
deployed thing** — the function's environment, the task definition, the
schedule rule's target input — and confirm the consumer actually *reads* what
was injected. A resolved-but-unread env var logs identically to a working one.

## 9. The gotchas

### 9.1 A Component needs two provisions for a new hostname

Core strips `ingress.hostname` before dispatch and wires DNS itself afterwards
from the endpoint the template published. The first provision creates endpoint
and record; anything that *reads* the hostname (your own
`${resource:<name>.…-url}` extension, a consumer's env) sees it on the second.
Plan the bring-up as two runs rather than debugging the first.

### 9.2 Don't assert a plugin-owned type

Plugin configs arrive at core as `*api.PluginFieldConfig` with `.Raw`
populated. Any `switch cfg := x.(type) { case *myapi.Config: }` in a path that
runs inside `sc` compiles, passes a test that constructs the value locally, and
fails in production. Read `.Raw` by key.

### 9.3 The validator checks your allowlist against the operator's raw

Adding a config key means adding it to `component_allowed_overrides` in the
*same* release — otherwise operators who adopt the new key get
`unknown or disallowed top-level key` from a plugin that clearly supports it.

### 9.4 Credentials rotate on every consumer deploy

Each `ComputeProcess` mints fresh. Anything that caches a consumer credential
outside your tenant store will be stale after the consumer's next deploy. And
since the mint precedes registration, a failed registration leaves the rotated
credential unrecorded — see [section 5](#5-computeprocess-being-consumed).

### 9.5 Per-repo lint config, three modules

Each module builds and tests separately; `go build ./...` at the repo root
covers nothing. Run `gofumpt` and (where the repo has a `.golangci.yml`) `gci`
with **that repo's own** section list on every touched file including
`_test.go`, and run `go test ./... -count=1` rather than `go build` — a new
interface method breaks fakes in test files that `go build` never compiles.

### 9.6 A green deploy is not a running service

`sc deploy` exiting 0 means the provider reported success. Assert the thing
exists: a revision, a task, a 200 from the endpoint. This bites hardest on
targets whose providers downgrade a failed deploy to a warning.

## See also

- [SC Components](../concepts/sc-components.md) — the operator's view.
- [Authoring an SC Plugin](plugin-authoring.md) — the surrounding plugin
  mechanics: repo layout, config tier, release workflow.
- [Template Placeholders](../concepts/template-placeholders.md) and
  [Advanced Template Placeholders](../concepts/template-placeholders-advanced.md).
