# `pkg/plugin/` — SC plugin runtime

The SC-core-side plugin runtime plus the shared **plugin-side runtime library**
(`runtime/`) every SC plugin builds on: proto definitions, go-plugin lifecycle
wrappers, MonitorProxy, pluginSession, SCCore callback server, PeerInvoke
plumbing, and the handler scaffolding (`runtime.NewContext`, `runtime.Settle`,
`runtime.HarvestExports`, `runtime.ResourceInputForComputeProcess`,
`runtime.DecodeInputs`). Together with the dispatch wiring in
`pkg/clouds/pulumi/plugin_dispatch.go` + `plugin_preengine.go` + `plugin_sidecars.go`,
this is the base layer of the
[`sc-plugin-architecture`](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-10-24/sc-plugin-architecture)
design.

Read the design doc first — this README assumes you have the architecture context,
especially §3 "The two gRPC conversations" and Addendum C "state-storage and
secrets-provider as plugins". For the **trap catalogue** collected across the
five-plugin extraction wave, read
[forge/docs/design/2026-11-01/plugin-extraction-playbook/](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-11-01/plugin-extraction-playbook/)
before you start extracting a new one.

## Package shape

```
pkg/plugin/
  proto/v1/plugin.proto         # SC plugin bus service definition (SCCloudPlugin + SCCore)
  proto/v1/plugin.pb.go         # generated — do not edit; regen with `make proto`
  proto/v1/plugin_grpc.pb.go    # ditto
  proto/README.md               # proto scope, buf config, regen command
  goplugin.go                   # hashicorp/go-plugin HandshakeConfig + Plugin impl
  client.go                     # SC-core-side: plugin.NewClient
  server.go                     # plugin-side: plugin.Serve
  monitor_proxy.go              # loopback gRPC ResourceMonitor forwarding to the real engine
  callback_server.go            # SC-core-side SCCore server (ResolveSecret, PeerInvoke)
  peer.go                       # plugin-side PeerInvoke client helper
  compute.go                    # ComputeResult builder (env vars, tpl extensions)
  vmhosts.go                    # VmHost builder
  publisher.go                  # progress Publisher routed to LogStream
  runtime/                      # plugin-side handler scaffolding (new in 2026-11-01)
    context.go                  #   NewContext + Settle generic
    wait.go                     #   go:linkname (*Context).wait — the drain
    engine.go                   #   LocalEngineAddress for background ctx.Log calls
    harvest.go                  #   HarvestExports — Secret-preserving output harvest
    compute.go                  #   ResourceInputForComputeProcess parent-env swap
    decode.go                   #   DecodeInputs[T] via scPlugin.InputsToPlain
  roundtrip_test.go             # unit test: handshake + Schema roundtrip
  provision_smoke_test.go       # unit test: plugin dials MonitorAddress + RegisterResource
  integration_test.go           # (build tag `integration`) real Pulumi engine Stage A
  vps_integration_test.go       # (build tag `integration`) real VPS Stage B
  testdata/
    echo-plugin/                # minimal Schema-only fixture plugin
    provision-plugin/           # plugin that registers a resource via monitor
    vps-marker-plugin/          # plugin that SSHes to a VPS via command:remote:Command
    state-store-plugin/         # plugin that provides a state-storage type (Step 4b)
```

## What SC core registers on your plugin's behalf

| Plugin advertises in `SchemaResponse` | SC core wires it where |
|---|---|
| `resource_types` | Dispatch table in `pApi.PluginResourcesByType`. In-engine, plugin.Provision called from `configureResource`'s fall-through. Plugin dials MonitorProxy and calls `RegisterResource`. |
| `state_store_types` | Pre-engine sidecar in `pApi.InitStateStoreFuncByType`. Plugin.InitStateStore called from `pkg/clouds/pulumi/login.go` to resolve `backend_url` + env vars. |
| `secrets_provider_types` | Proto reserved; wiring pending (see Addendum C / Step 4b follow-up). |
| `provider_types` | Proto reserved; wiring pending (plugin-owned `pulumi:providers:<pkg>` resources). |
| `compute_processor_types` | Proto reserved; wiring pending. |
| `registrar_types` | Proto reserved; wiring pending. |
| `pulumi_plugins` | SC core's auto-pin reads this for the explicit-provider mechanism (04 §3). |

**Types with `:` are routed to plugins; bare names stay in-tree.** The config-tier
fall-through in `pkg/api/read.go` triggers on any `type:` string containing `:`
and wraps the config in `api.PluginFieldConfig`.

## Writing a plugin

The template at
[`simple-container-com/sc-plugin-echo`](https://github.com/simple-container-com/sc-plugin-echo)
is the fastest start — clone it, swap in your cloud's config structs, keep the
runtime wiring. The author guide at
[docs/docs/guides/plugin-authoring.md](../../docs/docs/guides/plugin-authoring.md)
walks the full path.

The minimum viable binary — using the runtime library every production plugin
builds on:

```go
package main

import (
    "context"
    "os"

    hclog "github.com/hashicorp/go-hclog"

    plugin "github.com/simple-container-com/api/pkg/plugin"
    scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
    "github.com/simple-container-com/api/pkg/plugin/runtime"
)

type myPlugin struct {
    scpluginv1.UnimplementedSCCloudPluginServer
    progress *plugin.Publisher
}

func (p *myPlugin) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
    return &scpluginv1.SchemaResponse{
        PluginVersion: "my-plugin-0.1.0",
        ResourceTypes: []string{"mycloud:storage:Bucket"},  // MUST contain `:` for routing
    }, nil
}

func (p *myPlugin) Provision(stdctx context.Context, req *scpluginv1.ProvisionRequest) (resp *scpluginv1.ProvisionResponse, err error) {
    // runtime.NewContext opens one sdk.Context against req.Header.MonitorAddress
    // and defers a drain-then-close via runtime.Settle. WithLocalEngineFallback
    // keeps background ctx.Log calls from panicking when the header has no
    // engine address.
    rctx, err := runtime.NewContext(stdctx, req.GetHeader(), runtime.WithLocalEngineFallback(p.progress))
    if err != nil {
        return nil, err
    }
    defer runtime.Settle(rctx, &resp, &err)

    // ... register cloud resources against rctx.Pulumi ...
    // ... rctx.Pulumi.Export(key, out) for every value the parent should see ...

    outputs, err := runtime.HarvestExports(stdctx, rctx.Pulumi)
    if err != nil {
        return nil, err
    }
    return &scpluginv1.ProvisionResponse{Urn: "...", Outputs: outputs}, nil
}

func main() {
    publisher := plugin.NewPublisher()
    plugin.Serve(plugin.ServerConfig{
        Impl:   &myPlugin{progress: publisher},
        Logger: hclog.New(&hclog.LoggerOptions{
            Name:   "my-plugin",
            Level:  hclog.Warn,
            Output: os.Stderr,
        }),
    })
}
```

**Three invariants `runtime.*` enforces for you** — each has caused a live
regression at least once (see the extraction playbook):

- **Drain before you return.** `runtime.Settle` runs `(*Context).wait` via
  `go:linkname` before closing the SDK context. Returning without draining
  drops every pending `RegisterResource` and SC core plans a destroy of each
  state entry on the next apply. (Playbook trap 7, 2026-10-27.)
- **Preserve the Secret flag on exports.** `runtime.HarvestExports` calls
  `internals.UnsafeAwaitOutput` and copies `.Secret` into
  `InputValue.Secret`. Dropping it lands the value plaintext in the parent
  stack root. (Playbook trap 11, 2026-10-31.)
- **Export cross-plugin bridge Outputs.** If your plugin wraps a PeerInvoke
  call in an `ApplyT`, you MUST `rctx.Pulumi.Export(unique-key, urnOut)` the
  resulting Output. Callers that discard the ResourceOutput let Pulumi
  dead-strip the orphan chain — the PeerInvoke never fires and the DNS
  record silently never registers. (Playbook trap 15, 2026-11-01.)

Build:
```sh
go build -o my-plugin ./cmd/my-plugin
```

Operator configuration (server.yaml):
```yaml
plugins:
  my-plugin:
    path: /opt/sc-plugins/my-plugin
    sha256: abc123...  # optional; verified at every spawn

resources:
  resources:
    staging:
      resources:
        my-bucket:
          type: mycloud:storage:Bucket
```

## Extracting a driver from `pkg/clouds/pulumi/<cloud>/` — checklist

The point of all of this is to let a driver live in its own repo. Five drivers
have come out this way so far (cloudflare, yandex, cloudru, mongodb, aws); the
next milestone is gcp. The playbook at
[forge/docs/design/2026-11-01/plugin-extraction-playbook/](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-11-01/plugin-extraction-playbook/)
is the canonical trap-by-trap guide — read it alongside this checklist.

Steps:

- [ ] **Decide the plugin's advertised type names.** Add `:` in each (required by
      the config-tier fall-through). Keep the type strings byte-identical to what
      the current in-tree driver passes to Pulumi RegisterResource — see
      [04-no-destroy-invariant.md §1.2](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md#12-the-urn-changes--type-parent-chain-or-name-differs).
      For existing drivers (migration path), use the explicit-provider mechanism
      from 04 §3 to keep provider URNs byte-identical.
- [ ] **Create a new repo** named `simple-container-com/sc-plugin-<cloud>`.
      Nested-module layout: `/api` (config structs, no Pulumi SDK) + `/plugin`
      (the binary — may vendor whatever typed Pulumi SDKs the driver needs).
      Pattern per [03 §2.2](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md).
- [ ] **Author the plugin.** Implement `Schema` + whatever RPCs match your advertised
      types. For resource plugins, Provision dials MonitorAddress and calls
      RegisterResource with the exact type token Pulumi expects
      (e.g. `cloudflare:index/record:Record` — NOT `sc-cloudflare:record:Record`).
- [ ] **Use `pkg/plugin/runtime` for the handler scaffolding** — do NOT re-copy
      the per-plugin `drain.go` / `pulumiContextFromHeader` / harvest triad from
      an older plugin. Every handler starts with
      `rctx, err := runtime.NewContext(stdctx, req.GetHeader(), runtime.WithLocalEngineFallback(p.progress))`
      + `defer runtime.Settle(rctx, &resp, &err)`. Returning outputs?
      `runtime.HarvestExports`. ComputeProcess? `runtime.ParentInfoFromRequest`
      + `runtime.ResourceInputForComputeProcess`. Decoding typed inputs?
      `runtime.DecodeInputs[T]`. These helpers encode the invariants in
      playbook traps 7, 11, 12, 14 — rolling your own re-earns the bugs.
- [ ] **Lift the current in-tree driver code** from `pkg/clouds/pulumi/<cloud>/`
      into the plugin's `/plugin/internal/`. Rewrite the entry points from the
      pApi.Register*() init() pattern to SCCloudPluginServer method implementations.
      Preserve resource names and parent chains byte-identically.
- [ ] **Preserve the config-tier schema.** Lift `pkg/clouds/<cloud>/` structs into
      the plugin repo's `/api` module. SC core imports those for `cmd/schema-gen`
      per [D3](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/06-decisions.md#d3).
- [ ] **Add a URN-contract test** in the plugin's own repo, mirroring
      `pkg/clouds/pulumi/cloudflare/urn_contract_test.go` (PR #479's seed) —
      asserts resource/provider names match a documented pattern. One failed
      byte-match blocks the plugin's release.
- [ ] **Add fixture-based migration test** — a scratch stack deployed by pre-split
      sc, then `pulumi preview --expect-no-changes` with the plugin-era sc.
      Required gate per [04 §4](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/04-no-destroy-invariant.md#4-the-acceptance-gate--a-conformance-test-not-a-hope).
- [ ] **Cut v1.0.0** — GitHub release with signed binary + sha256sums.txt.
- [ ] **Update SC core** — remove `_ "github.com/simple-container-com/api/pkg/clouds/pulumi/<cloud>"`
      blank import from `pkg/clouds/pulumi/init.go`; delete the subtree; update
      `go.mod`; measure new `make build` link RSS to verify shrinkage.

For composite drivers (kubernetes, k3s, mongodb that compose other clouds), add:
- [ ] **PeerInvoke RPC** (proto wiring pending — composite plugin asks SC core to
      invoke on another plugin, per [D8](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/06-decisions.md#d8)).

## Shipped surfaces (what the five plugins actually use)

| Surface | Plugin RPC | Shipped example |
|---|---|---|
| State-store | `InitStateStore` | aws `s3-bucket`, yandex `yc-object-storage` |
| Secrets-provider | `InitSecretsProvider` | aws `aws-kms`, yandex `yc-kms` |
| Provider | `Provider` | aws, yandex, cloudru providers; mongodb Atlas provider |
| Resource / Template | `Provision` | every resource/template token in `supported-resources.md` tagged "plugin" |
| Compute processor | `ComputeProcess` | mongodb `uses:` fan-out (MONGO_URI/USER/PASSWORD), aws `s3-bucket` consumer env |
| Registrar | `ProvisionRegistrar`, `NewDnsRecord`, `NewOverrideHeaderRule`, `ProvisionDomainForEndpoint` | cloudflare; aws delegates DNS to the operator-configured registrar via `PeerInvoke` |
| PreDestroy | `PreDestroy` | mongodb drops databases named in `-service-user` outputs before Pulumi destroys the cluster |
| VM driver | `VmHosts` | yandex `yandex-vm`, cloudru `cloudru-evolution-vm` |
| LogStream | `LogStream` (server-stream) | every plugin's `scPlugin.Publisher` routed to the engine's progress channel |
| Peer invoke | `SCCore.PeerInvoke` (plugin → core → peer plugin) | aws delegates `NewDnsRecord`, `NewOverrideHeaderRule`, `ProvisionDomainForEndpoint` to `cloudflare` without a Go-module import (D8) |

## Not yet wired

| What | Current state | Blocker |
|---|---|---|
| Fingerprint streamed health | Not yet in proto | Server-streaming RPC per Nomad pattern. Health inference today still happens inside SC core via `pkg/clouds/pulumi/healthcheck.go`. |

The two-phase plugin lifecycle (pre-engine + in-engine) + the SCCore callback
server (ResolveSecret / ResolveCredential / PeerInvoke) cover everything the
five extracted plugins needed.

## Testing

```sh
# Unit tests — fast, no external deps.
go test ./pkg/plugin/...

# Stage A — real Pulumi engine, local-fs backend. Requires `pulumi` CLI on PATH.
go test -tags integration -run TestIntegration_PluginRegistersAgainstRealEngine ./pkg/plugin/...

# Stage B — real VPS deploy via command:remote:Command. Requires SC_PLUGIN_VPS_* env vars.
SC_PLUGIN_VPS_HOST=... SC_PLUGIN_VPS_USER=... SC_PLUGIN_VPS_PASSWORD=... \
  go test -tags integration -run TestIntegration_PluginTouchesVPS ./pkg/plugin/...
```

The integration tests are build-tagged so plain `make check` + `go test ./...` stay
fast; CI's dedicated integration job (which already has `pulumi` installed per
commit `20a04303`) picks them up with `-tags integration`.
