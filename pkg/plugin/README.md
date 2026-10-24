# `pkg/plugin/` — SC plugin runtime

The SC-core-side plugin runtime: proto definitions, go-plugin lifecycle wrappers,
MonitorProxy, pluginSession. Together with the dispatch wiring in
`pkg/clouds/pulumi/plugin_dispatch.go` + `plugin_preengine.go`, this is Phase 0
Steps 1 + 2 + 4 + 4b of the
[`sc-plugin-architecture`](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-10-24/sc-plugin-architecture)
design.

Read the design doc first — this README assumes you have the architecture context,
especially §3 "The two gRPC conversations" and Addendum C "state-storage and
secrets-provider as plugins".

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

Minimum viable plugin binary — see `testdata/echo-plugin/main.go` for the smallest
real example:

```go
package main

import (
    "context"

    hclog "github.com/hashicorp/go-hclog"

    plugin "github.com/simple-container-com/api/pkg/plugin"
    scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type myPlugin struct {
    scpluginv1.UnimplementedSCCloudPluginServer
}

func (p *myPlugin) Schema(_ context.Context, _ *scpluginv1.SchemaRequest) (*scpluginv1.SchemaResponse, error) {
    return &scpluginv1.SchemaResponse{
        PluginVersion: "my-plugin-0.1.0",
        ResourceTypes: []string{"mycloud:storage:Bucket"},  // MUST contain `:` for routing
    }, nil
}

func (p *myPlugin) Provision(ctx context.Context, req *scpluginv1.ProvisionRequest) (*scpluginv1.ProvisionResponse, error) {
    // Dial MonitorAddress from req.Header, call pulumirpc.RegisterResource.
    // See testdata/provision-plugin/main.go for the pattern.
    //
    // The URN SC core will see is:
    //   urn:pulumi:<stack>::<project>::<req.ResourceType>::<req.ResourceName>
    // Choose names deterministically — see 04 §1.2 for the back-compat contract.
    return &scpluginv1.ProvisionResponse{Urn: "..."}, nil
}

func main() {
    plugin.Serve(plugin.ServerConfig{
        Impl:   &myPlugin{},
        Logger: hclog.New(&hclog.LoggerOptions{Name: "my-plugin"}),
    })
}
```

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

The point of all of this is to let a driver live in its own repo. Steps for a
greenfield driver (cloudflare, yandex, cloudru) using the already-wired paths:

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

## Not-yet-wired surfaces (planned)

| What | Current state | Blocker |
|---|---|---|
| Secrets-provider plugin | Proto in; sidecar wiring pending | `pkg/clouds/pulumi/provision.go:78-110` secretsProviderUrl plumbing needs the same sidecar pattern as InitStateStore |
| Provider plugin (plugin owns `pulumi:providers:<pkg>`) | Proto in | `initProvider` cast at `provision.go:336` — short-circuit for plugin providers is in Step 4; the plugin-provided provider URN needs plumbing through to downstream resources |
| Compute processor plugin | Proto in | Plugin's returned outputs don't yet flow through `ComputeContext.Collector`; downstream env/placeholders won't see them |
| Registrar plugin | Proto in | DNS registrar dispatch at `provision.go:241` needs the same fall-through as resource dispatch |
| PreDestroyHook plugin | Not yet in proto | Add a `PreDestroy` RPC and wire into `pApi.PreDestroyHookFuncs` |
| Fingerprint streamed health | Not yet in proto | Server-streaming RPC; adds reactive health checks per Nomad pattern |

Each is a scoped follow-up PR — none architecturally hard. The two-phase plugin
lifecycle (pre-engine + in-engine) already covers all of them.

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
