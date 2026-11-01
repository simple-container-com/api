# SC plugin bus — `pkg/plugin/proto/`

Protocol-buffer definitions for the Simple Container plugin bus. This is **Phase 0
Step 1** of the `sc-plugin-architecture` design — see
[`forge/docs/design/2026-10-24/sc-plugin-architecture/`](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-10-24/sc-plugin-architecture)
for the full design, [`03-architecture.md §3`](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md#3-the-two-grpc-conversations-by-name)
for why there are two gRPC conversations and this one is only one of them.

## Scope of v1 (the current `v1/plugin.proto`)

Five plugins ship on this surface today: `sc-plugin-aws`, `sc-plugin-cloudflare`,
`sc-plugin-cloudru`, `sc-plugin-mongodb`, `sc-plugin-yandex`. Everything the
in-tree drivers did is reachable through the RPCs below.

### `service SCCloudPlugin` (SC core → plugin)

| RPC | Purpose |
|---|---|
| `Schema` | Advertise the provider / auth / registrar / template / resource / compute-processor / state-store / secrets-provider / VM-driver types this plugin owns, plus the Pulumi provider plugins (versions, download URLs) the engine must have available. |
| `InitStateStore` | Configure the Pulumi state backend (S3, GCS, YC Object Storage, …) **before** the Automation API engine starts. |
| `InitSecretsProvider` | Pre-engine config for the secrets provider (KMS, Lockbox, …) that encrypts Pulumi state secrets. |
| `Provider` | Register the explicit `pulumi:providers:<pkg>` resource. Returns its URN for SC core to wire into every downstream `Provision` via `RequestHeader.provider_urns`. |
| `Provision` | Register one SC-level resource against the monitor. Returns the resulting URN + harvested outputs (see §Outputs below). |
| `ComputeProcess` | Resolve a `uses:` dependency into env variables + template-extension values for the consuming stack (e.g. `MONGO_URI` from a MongoDB cluster). |
| `ProvisionRegistrar` | DNS registrar bootstrap (e.g. zone lookup, token validation) before per-record calls. |
| `NewDnsRecord` | Register one DNS record. Returns the resulting URN. |
| `NewOverrideHeaderRule` | Register one HTTP header override rule (e.g. a Cloudflare Transform Rule). |
| `ProvisionDomainForEndpoint` | Register a hostname → endpoint binding (e.g. a Cloudflare Worker route or a YC API Gateway attachment). |
| `PreDestroy` | Cloud-specific cleanup before Pulumi's destroy phase (e.g. mongodb drops databases named in `-service-user` outputs). Fire-and-forget: SC core does not block destroy on the response. |
| `VmHosts` | Build the `[]VmHost` a VM-driver resource exposes to downstream SSH-based provisioners. |
| `LogStream` | Server-stream progress events from the plugin's `scPlugin.Publisher` onto the engine's log channel. |

### `service SCCore` (plugin → SC core, over a go-plugin broker stream)

| RPC | Purpose |
|---|---|
| `ResolveSecret` / `ResolveCredential` | Vault-pattern cleartext fetch — the plugin bus never carries secret plaintext. |
| `PeerInvoke` | Composite plugins ask SC core to invoke an RPC on a *peer* plugin by name (see [D8](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/06-decisions.md#d8-composite-plugins-over-peer-plugin-invocation-not-go-module-imports)). Used today by the aws plugin to delegate `NewDnsRecord` / `ProvisionDomainForEndpoint` / `NewOverrideHeaderRule` to the operator-configured registrar (typically `cloudflare`). |

### Outputs — the harvest contract

`ProvisionResponse.Outputs` carries the plugin's `pctx.Export(k, v)` calls as
`map<string, InputValue>` — SC core re-exports every entry onto the parent stack
root so `uses:` consumers resolve them. The `InputValue.Secret` flag is the
secret bit that SC core wraps with `sdk.ToSecret` on the re-export.

**Both halves of this contract must be lived by the plugin runtime**: the harvest
helper must await every `sdk.Output` and preserve the Secret flag, and bridge
methods that wrap PeerInvoke in an `ApplyT` must `ctx.Export` the resulting
`Output` so Pulumi does not dead-strip it. See
[`pkg/plugin/runtime`](../runtime/) for the shared implementation; the two live
regressions that produced it (`v2026.10.60`+ and the sc-plugin-aws
`v2026.11.01-e377550` cascade) are documented in
[forge/docs/design/2026-11-01/plugin-extraction-playbook/](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-11-01/plugin-extraction-playbook/)
traps 11, 13, 14, 15.

### Not yet in v1

- `Fingerprint` — server-streaming health channel (Nomad-pattern — see
  [03 §6.4](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md#64-health-progress-graceful-shutdown)). Health inference today still happens inside SC core.

## Versioning

The import path encodes the version: `pkg/plugin/proto/v1`. A v2 shape — if we ever
need one — ships under `pkg/plugin/proto/v2` and both coexist. HashiCorp's
`VersionedPlugins` pattern (one plugin binary can serve multiple protocol versions)
is the long-term fit; v1 is a single `ProtocolVersion` integer.

Field numbers in each message are **permanent** once they land on `main`. Deleted
fields are moved to `reserved <n>;` so nothing else reuses them.

## Regenerating Go

Generated Go stubs live alongside the proto at `pkg/plugin/proto/v1/plugin.pb.go`
and `plugin_grpc.pb.go` — they are checked in so a plain `go build ./...` works
without proto tooling installed. To regenerate:

```sh
# One-time tool install (bufbuild/buf bundles protoc internally):
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# From repo root:
make proto
```

The generation config lives in `buf.yaml` + `buf.gen.yaml` in this directory.

## Linting

`buf lint` runs against `buf.yaml` and is the entry point every reviewer should
expect to be green on this subtree:

```sh
cd pkg/plugin/proto && buf lint
```

Two STANDARD rules are deliberately disabled (`PACKAGE_DIRECTORY_MATCH` because
our Go-import-matching layout puts v1/ one level below where buf expects it, and
`SERVICE_SUFFIX` because `SCCloudPlugin` matches the project's naming convention
rather than appending `Service` — same stance as Pulumi's own `ResourceProvider`
and Terraform's `Provider`).
