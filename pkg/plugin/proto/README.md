# SC plugin bus — `pkg/plugin/proto/`

Protocol-buffer definitions for the Simple Container plugin bus. This is **Phase 0
Step 1** of the `sc-plugin-architecture` design — see
[`forge/docs/design/2026-10-24/sc-plugin-architecture/`](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-10-24/sc-plugin-architecture)
for the full design, [`03-architecture.md §3`](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md#3-the-two-grpc-conversations-by-name)
for why there are two gRPC conversations and this one is only one of them.

## Scope of v1 (the current `v1/plugin.proto`)

A "walking skeleton" surface: the four RPCs you can't build an echo plugin without.

| RPC | Who calls | Purpose |
|---|---|---|
| `Schema` | SC core → plugin | Advertise provider / resource / registrar / state-store types this plugin owns, plus the Pulumi provider plugins (versions, download URLs) the engine must have available. |
| `InitStateStore` | SC core → plugin | Configure the Pulumi state backend (S3, GCS, YC Object Storage, …) **before** the Automation API engine starts. |
| `Provider` | SC core → plugin | Register the explicit `pulumi:providers:<pkg>` resource. Returns its URN for SC core to wire into every downstream `Provision` via `RequestHeader.provider_urns`. |
| `Provision` | SC core → plugin | Register one SC-level resource against the monitor. Returns the resulting URN + outputs. |
| `ResolveSecret` / `ResolveCredential` | plugin → SC core (callback) | Vault-pattern cleartext fetch — the plugin bus never carries secret plaintext. |

Not v1 — reserved for the next proto PR once the `pkg/plugin/` runtime (Step 2) is on
the floor:

- `ComputeProcess` — compute-side injection (env variables, dependency placeholders).
- `Registrar` — DNS registrar RPCs (`ProvisionRecords`, `NewRecord`, `NewOverrideHeaderRule`, `ProvisionDomainForEndpoint`, `NewWorkerScript`).
- `PreDestroy` — pre-destroy hooks per cloud.
- `Fingerprint` — server-streaming health channel (Nomad-pattern — see
  [03 §6.4](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md#64-health-progress-graceful-shutdown)).
- `PeerInvoke` — composite plugins ask SC core to invoke on a peer plugin
  (see [D8](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/06-decisions.md#d8-composite-plugins-over-peer-plugin-invocation-not-go-module-imports)).

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
