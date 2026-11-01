---
title: Authoring an SC cloud plugin
description: Clone sc-plugin-echo, swap in your cloud's auth and resources, cut a release — the step-by-step for a new SC cloud plugin.
platform: platform
product: simple-container
category: devguide
subcategory: authoring
date: '2026-10-28'
---

# Authoring an SC cloud plugin

An SC cloud plugin is a separate executable `sc` downloads and talks to over
a local gRPC bus. Plugins own:

- a cloud's **config-tier types** (auth, registrar, template, resource, VM
  driver) — SC core parses `server.yaml` through them;
- the **provisioning logic** behind those types — the plugin registers
  Pulumi resources against the engine SC core already set up.

Three plugins ship today — `sc-plugin-cloudflare`, `sc-plugin-yandex`,
`sc-plugin-cloudru` — and the design calls for seven more
(`mws`, `selectel`, `vkcs`, `timeweb`, `mongodb`, `aws`, `gcloud`,
`k8s`). The template every new plugin starts from is
[`simple-container-com/sc-plugin-echo`](https://github.com/simple-container-com/sc-plugin-echo).

!!! info "What this page is not"
    This is the author guide. If you're an operator pinning a plugin, read
    [Upgrading to Plugins](upgrading-to-plugins.md).

---

## 1. Clone the template

```bash
gh repo create <org>/sc-plugin-<cloud> --template simple-container-com/sc-plugin-echo --public
cd sc-plugin-<cloud>
```

Rename the module paths and the plugin's own name:

```bash
# Module paths in both go.mod files
sed -i 's|sc-plugin-echo|sc-plugin-<cloud>|g' api/go.mod plugin/go.mod

# Package name throughout the sources — this is the one you'll do by hand too
grep -rlE 'echo|Echo' api/ plugin/ --include='*.go' \
  | xargs sed -i -e 's/\becho\b/<cloud>/g' -e 's/\bEcho\b/<Cloud>/g'

# Binary name in the release workflow + imports in main.go
sed -i 's|sc-plugin-echo|sc-plugin-<cloud>|g' .github/workflows/release.yml README.md CLAUDE.md
```

Verify the template still passes untouched before you change anything of
substance:

```bash
cd api && go test ./... && cd ../plugin && go test ./...
```

Both must print `ok`. If they don't, the rename is incomplete — grep for
`echo` and `Echo` and finish the sweep.

---

## 2. Author the config tier (`/api`)

The `/api` module is published as its own Go module. SC core imports it;
your plugin binary (`/plugin`) imports it. Everything here is **parse-time
only** — no cloud SDK is linked in, no network calls, no Pulumi.

### 2.1 Define the token constants

Replace `api/init.go`'s `const` block with your cloud's tokens:

```go
const ProviderType = "<cloud>"

const (
    AuthTypeAccessKey    = "<cloud>-access-key"
    RegistrarTypeDns     = "<cloud>-dns"
    TemplateTypeFunction = "<cloud>-function"
    ResourceTypeBucket   = "<cloud>-bucket"
    DriverVm             = "<cloud>-vm"
    StateStorageTypeBucket    = "<cloud>-state-bucket"
    SecretsProviderTypeKms    = "<cloud>-kms"
)
```

A token's string value is the **public contract** — renaming it turns an
existing operator's `server.yaml` into "unknown type". Treat them as frozen
once published.

### 2.2 Author the auth type

Replace `EchoAccountConfig` in `api/auth.go` with your cloud's credential
shape. The shape must implement `api.AuthConfig` plus
`ProviderType() string`, `CredentialsValue() string`,
`ProjectIdValue() string`, and `Validate() error`:

- `CredentialsValue` returns the opaque blob for `${auth:<name>}`
  interpolation. The template's fallback (`api.AuthToString(r)` when
  `r.Credentials.Credentials == ""`) is what you want: it preserves the
  operator's choice of inline fields vs an external secret.
- `ProjectIdValue` is what `${auth:<name>.projectId}` resolves to.
  Return the k8s-style `"n/a"` for a cloud without a project id notion.
- `Validate` must refuse at parse time what the first API call would
  otherwise return as a cloud-specific 401/403. Name the missing field.

### 2.3 Author the registrar + template configs

Same shape as the account config, but with cloud-specific fields
(zone id, bucket region, timeout, memory). Each needs a
`Read<Type>Config(*api.Config) (api.Config, error)` reader that calls
`api.ConvertConfig`.

### 2.4 Register everything in `init()`

Both halves — the config readers and the fool-protection declaration —
MUST list every token your plugin owns. Missing one in `OwnedTypes` is
a **silent** gap: an operator who only uses the un-listed token gets no
auto-inject from SC core's parse-time validator, and the deploy fails
later with "unknown resource type". The template's `init_test.go` guards
this via reflection.

```go
api.RegisterProviderConfig(api.ConfigRegisterMap{
    AuthTypeAccessKey:    ReadAuthAccessKeyConfig,
    RegistrarTypeDns:     ReadDnsRegistrarConfig,
    TemplateTypeFunction: ReadFunctionTemplateConfig,
})

api.RegisterPluginRequirement(api.PluginRequirement{
    PluginName: "<cloud>",
    OwnedTypes: []string{
        ProviderType, AuthTypeAccessKey, RegistrarTypeDns,
        TemplateTypeFunction, ResourceTypeBucket, DriverVm,
        StateStorageTypeBucket, SecretsProviderTypeKms,
    },
    DefaultSource: api.PluginSource{
        Type:       "github-release",
        Repository: "<org>/sc-plugin-<cloud>",
    },
})
```

---

## 3. Author the provisioning tier (`/plugin/internal`)

The handler scaffolding that used to be copy-pasted into every plugin's
`main.go` now lives in [`api/pkg/plugin/runtime`](https://github.com/simple-container-com/api/tree/main/pkg/plugin/runtime).
Import it, don't re-copy it — rolling your own re-earns the bugs it was built
to prevent. See [`pkg/plugin/runtime/wait.go`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/runtime/wait.go)
for the package doc.

**The five helpers every production plugin uses:**

- `runtime.NewContext(stdctx, req.GetHeader(), opts...)` — opens one
  `sdk.Context` per RPC. `WithLocalEngineFallback(publisher)` keeps
  background `ctx.Log` calls from panicking when the header carries no
  engine address.
- `runtime.Settle[T](rctx, &resp, &err)` — `defer`red at the top of
  every handler. Drains pending `RegisterResource` goroutines via
  `go:linkname` into `(*Context).wait`, closes the context, folds any
  wait-error into the handler's named return. **Without this**, a
  handler that returns before its registrations resolve lets Pulumi's
  teardown drop them silently, and SC core plans a destroy of every
  state entry on the next apply (playbook trap 7).
- `runtime.HarvestExports(stdctx, pctx)` — reads every `pctx.Export(k, v)`
  made during the handler, awaits each `sdk.Output`, and converts into
  `map[string]*scpluginv1.InputValue`. **Preserves the Secret flag** —
  the live-caught regression it exists to prevent (playbook trap 11)
  landed access keys and RDS passwords plaintext in the parent stack
  root.
- `runtime.ParentInfoFromRequest(req)` + `runtime.ResourceInputForComputeProcess(req, parent, cfg)`
  — ComputeProcess helpers. The latter swaps the child env for the
  parent's when `ParentInfo.ParentEnv` is non-empty, so parent-stack
  lookup keys resolve correctly on isolated stacks (playbook trap 12).
- `runtime.DecodeInputs[T](req.GetInputs(), &target)` — unmarshals the
  plugin's typed input map into your config struct via `scPlugin.InputsToPlain` +
  yaml round-trip. Handles nested maps and lists correctly, which the
  earlier scalar-only variants in aws/mongodb/cloudru lost (playbook
  trap 14).

Two things that stay per-plugin:

- `LogStream` — drains the plugin's progress Publisher onto the gRPC
  stream. Skip this and the plugin is silent for the whole of a
  multi-step RPC chain. The template wires it correctly; don't touch it.
- `strVal(v *scpluginv1.InputValue) string` and small input accessors —
  one-liners that read scalar fields off `InputValue`. Keep them local.

**Idiomatic handler skeleton:**

```go
func (p *myPlugin) Provision(stdctx context.Context, req *scpluginv1.ProvisionRequest) (resp *scpluginv1.ProvisionResponse, err error) {
    rctx, err := runtime.NewContext(stdctx, req.GetHeader(), runtime.WithLocalEngineFallback(p.progress))
    if err != nil {
        return nil, err
    }
    defer runtime.Settle(rctx, &resp, &err)

    var cfg MyCloudBucketConfig
    if err := runtime.DecodeInputs(req.GetInputs(), &cfg); err != nil {
        return nil, errors.Wrap(err, "decode bucket inputs")
    }

    // register cloud resources against rctx.Pulumi; rctx.Pulumi.Export(key, out)
    // every value a consumer might want.

    outputs, err := runtime.HarvestExports(stdctx, rctx.Pulumi)
    if err != nil {
        return nil, err
    }
    return &scpluginv1.ProvisionResponse{Urn: urn, Outputs: outputs}, nil
}
```

What you change:

### 3.1 `Schema()` — advertise everything you own

```go
return &scpluginv1.SchemaResponse{
    PluginVersion:         "<cloud>-0.0.1",
    ProviderTypes:         []string{echoApi.ProviderType},
    RegistrarTypes:        []string{echoApi.RegistrarTypeDns},
    ResourceTypes:         []string{echoApi.ResourceTypeBucket},
    ComputeProcessorTypes: []string{echoApi.ResourceTypeBucket},
    StateStoreTypes:       []string{echoApi.StateStorageTypeBucket},
    SecretsProviderTypes:  []string{echoApi.SecretsProviderTypeKms},
    PulumiPlugins: []*scpluginv1.PulumiPluginRequirement{
        {Package: "<cloud>", Version: "X.Y.Z"},  // github.com/pulumi/pulumi-<cloud>/sdk
    },
    VmDrivers: []*scpluginv1.VmDriverSpec{
        vm.Drivers[vm.Driver<Cloud>].ToProto(vm.Driver<Cloud>),
    },
}, nil
```

Keep `Schema()` in sync with `/api/init.go`'s `OwnedTypes` list — the
two must agree. The `init_test.go` reflection test catches gaps in the
`OwnedTypes` half; a `Schema()` that omits a token is caught at the
next conformance run.

### 3.2 Replace the RPC bodies

Each echo RPC in `plugin/internal/` registers a logical ComponentResource
with no cloud side-effects. For each one you own, replace the body with
real cloud-driven work:

- Construct a `pulumi.ProviderResource` with your cloud SDK's `NewProvider`.
- Register cloud resources (`cloudflare.NewRecord`, `yandex.NewDnsRecordSet`,
  …).
- For DNS registrars, honour the deploying stack's **`baseDnsZone`**
  from the request — NOT your configured `zoneName`, which is only a
  default. See §6.
- For VM drivers, construct a `tls.PrivateKey` deploy key, then compose
  the host via `plugin.BuildVmHosts(plugin.NewVmHost(ctx, name).
  AddressOf(fip, "externalIpv4Address").PrivateKeyOf(key,
  "privateKeyOpenssh").HostKeyOf(key, "publicKeyOpenssh").
  DependsOn(instance, fip))`. See
  [`api/pkg/plugin/vmhosts.go`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/vmhosts.go)
  for the full builder surface.

### 3.1 ComputeProcessor — env vars and template extensions for consumers

A `ComputeProcessor` runs for every dependency resource a consuming stack
declares (a compose service pointing at a MongoDB cluster, a Lambda
pointing at an S3 bucket). The plugin translates the resource's
provisioning outputs into:

- **env vars** the consumer runs with (`MONGO_URI`, `DATABASE_URL`),
  optionally secret-flagged;
- **template extensions** — maps the consumer's config reads to resolve
  `${resource:<name>.<key>}` and `${dependency:<name>.<resName>.<key>}`.

Use `plugin.NewComputeResult()` from
[`api/pkg/plugin/compute.go`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/compute.go):

```go
import scPlugin "github.com/simple-container-com/api/pkg/plugin"

func (p *myPlugin) ComputeProcess(ctx context.Context, req *scpluginv1.ComputeProcessRequest) (*scpluginv1.ComputeProcessResponse, error) {
    return scPlugin.NewComputeResult().
        Urn(fmt.Sprintf("<cloud>-compute:%s:%s", req.GetResourceType(), req.GetResourceName())).
        SecretEnvVar("MONGO_URI", mongoUri, req.GetResourceType(), req.GetResourceName(), req.GetHeader().GetStackName()).
        EnvVar("MONGO_DB", dbName, req.GetResourceType(), req.GetResourceName(), req.GetHeader().GetStackName()).
        ResourceTplExtension(req.GetResourceName(), map[string]string{
            "connectionString": connStr,
            "database":         dbName,
        }).
        Build()
}
```

**Open a StackReference against the parent** to pull previously-exported
outputs. The parent-stack reference string rides in
`req.ParentInfoJson` — JSON of
[`pApi.ParentInfo`](https://github.com/simple-container-com/api/blob/main/pkg/clouds/pulumi/api/cloud_provider.go)
with `FullReference`, `UsesResource`, `DependsOnResource`,
`ResourceEnv`, `StackName`:

```go
var parent pApi.ParentInfo
if raw := req.GetParentInfoJson(); len(raw) > 0 {
    if err := json.Unmarshal(raw, &parent); err != nil { return nil, err }
}
ref, _ := sdk.NewStackReference(pctx, parent.FullReference, nil)
projectId := ref.GetOutput(sdk.String(parent.StackName + "-project-id"))
```

!!! warning "What does NOT cross the bus"
    `pApi.ComputeContextCollector` has methods for `AddDependency(sdk.Resource)`,
    `AddOutput(sdk.Output)`, `AddPreProcessor`, `AddPostProcessor`. **None of
    these are serialisable** — Pulumi resource handles and Go closures stay
    in the plugin's own process. Apply them against the plugin's own
    `sdk.Context` BEFORE returning from Build; the sidecar's response-
    handling only reaches the env vars + tpl extensions. Document the
    semantic for any helper you build on top.

**Reference**: [`sc-plugin-echo/plugin/internal/compute.go`](https://github.com/simple-container-com/sc-plugin-echo/blob/main/plugin/internal/compute.go)
— minimal stub exercising every builder method.

### 3.2 PreDestroy — cleanup before Pulumi destroys

A `PreDestroy` hook runs cloud-specific cleanup before Pulumi's destroy
phase: mongodb drops databases named in service-user outputs, aws might
drain an SQS queue. Three fields SC core packs on the request (Phase 1e):

- `req.StackOutputs` — `map<string, bytes>`, each value is JSON of
  `auto.OutputValue` (`{"value": …, "secret": bool}`). Enumerate without
  decoding each one; filter by key suffix (mongodb matches `-service-user`)
  before unmarshalling.
- `req.ClientDescriptorJson` — JSON of `api.StackClientDescriptor` for
  `stack.Client.Stacks[env]`. Hooks read `cloudExtras` off this.
- `req.DestroyParamsJson` — JSON of `api.DestroyParams`. Carries env name
  + dry-run flags.

```go
func (p *myPlugin) PreDestroy(ctx context.Context, req *scpluginv1.PreDestroyRequest) (*scpluginv1.PreDestroyResponse, error) {
    // Decide whether to run from cloudExtras.
    var desc api.StackClientDescriptor
    if raw := req.GetClientDescriptorJson(); len(raw) > 0 {
        _ = json.Unmarshal(raw, &desc)
    }
    // Enumerate stack outputs by suffix.
    for key, raw := range req.GetStackOutputs() {
        if !strings.HasSuffix(key, "-service-user") { continue }
        var ov auto.OutputValue
        if err := json.Unmarshal(raw, &ov); err == nil {
            dropDatabase(ov.Value)
        }
    }
    return &scpluginv1.PreDestroyResponse{}, nil
}
```

!!! info "SC core is fire-and-forget on PreDestroy"
    The sidecar does not block the destroy on your response. Return
    `{}` even on partial-failure paths; log the failure through your
    progress Publisher so the operator sees it, but let the destroy
    proceed. If your cleanup is critical to data integrity, it belongs
    in the resource's own destroy script (pulumi-command `Delete`), not
    in PreDestroy.

**Reference**: [`sc-plugin-echo/plugin/internal/pre_destroy.go`](https://github.com/simple-container-com/sc-plugin-echo/blob/main/plugin/internal/pre_destroy.go).

### 3.3 Composite plugins — PeerInvoke (D8)

A composite plugin needs cloud resources another plugin owns —
`sc-plugin-mongodb`'s PrivateLink needs an AWS SecurityGroup + VpcEndpoint;
`sc-plugin-k3s` wants both AWS and GCP. Decision D8 of
[`06-decisions.md`](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/06-decisions.md)
rejects a direct plugin-to-plugin connection and the Go-module-import
alternative: **SC core brokers**, exposed via the SCCore callback RPC
`PeerInvoke`.

The plugin side dials via a go-plugin broker stream SC core served
specifically for this plugin:

```go
import scPlugin "github.com/simple-container-com/api/pkg/plugin"

peer, err := scPlugin.NewPeerClient(ctx, broker, req.GetHeader())
if err != nil { return nil, err }
defer peer.Close()

awsResp, err := peer.Provision(ctx, "sc-plugin-aws", &scpluginv1.ProvisionRequest{
    Header:       req.GetHeader(),
    ResourceType: "aws-security-group",
    ResourceName: "mongodb-privatelink-sg",
    // ... inputs ...
})
```

The broker arrives at your plugin's `GRPCServer` method (hashicorp/go-plugin
hands it there). Your `Impl` struct must capture it so handlers can dial
back later:

```go
type myPlugin struct {
    scpluginv1.UnimplementedSCCloudPluginServer
    broker *hplugin.GRPCBroker  // captured; nil until GRPCServer runs
    // ...
}
```

`BrokerAware` is live (`api` #514). `pkg/plugin/goplugin.go`'s
`scCloudPlugin.GRPCServer` type-asserts the Impl against the one-method
interface `type BrokerAware interface { SetBroker(*hplugin.GRPCBroker) }`
and calls `SetBroker` when it exists; `NewPeerClient` dials through it.
Used in production today by `sc-plugin-aws`, which delegates
`NewDnsRecord` / `ProvisionDomainForEndpoint` / `NewOverrideHeaderRule`
to the operator-configured registrar (typically `cloudflare`).

!!! danger "Dead-strip the ApplyT around PeerInvoke — the 2026-11-01 trap"
    If your bridge method returns a `*api.ResourceOutput{Ref: urnOut}`
    where `urnOut` is an `ApplyT` chain containing the PeerInvoke call,
    and the caller discards the ResourceOutput, Pulumi's SDK **dead-strips
    the orphan Output** and the PeerInvoke RPC never fires — the deploy
    reads green with the DNS record silently never registered. **Fix**:
    `ctx.Export(unique-key, urnOut)` inside the bridge method before
    returning, as a forcing function. Keys must be unique per
    (stack, resource/endpoint/rule) so stacks with multiple records
    don't collide. Live-caught on forge-notifier run 41339900839; fixed
    in `sc-plugin-aws v2026.11.01-e377550`. See
    [playbook trap 15](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-11-01/plugin-extraction-playbook/README.md).

**Rules of the road** (D8):
- A composite plugin NEVER imports a peer plugin's SDK as a Go module.
  The plugin bus is the only cross-plugin contract.
- A composite plugin NEVER maintains its own list of "which peer owns
  which type". Call `PeerInvoke` with the operator-configured peer name
  (from the plugin's own config), and let SC core refuse unknown peers.

**References**:
- [`api/pkg/plugin/peer.go`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/peer.go) — plugin-side helper.
- [`api/pkg/plugin/callback_server.go`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/callback_server.go) — core-side server (also carries the ResolveSecret/ResolveCredential stubs — Vault pattern, Addendum R work).
- Design: [`03-architecture.md`](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-10-24/sc-plugin-architecture/03-architecture.md) §9.

---

## 4. Test — the trap-free pattern

The template's `plugin/internal/urn_contract_test.go` uses
`pulumi.NewContext` + `RunInfo.Mocks` directly, **not**
`pulumi.RunErr(..., WithMocks)`. This matters:

!!! danger "The test-harness trap"
    `pulumi.RunErr` ends in the context's internal `wait()`, which drains
    the engine's RPC queue. Any registration **stranded on an unresolved
    input is silently completed by the harness itself on the test's behalf**.
    That hid the entire 2026-10-28 await-returns bug class behind a
    green suite (Addendum R §R3 of
    `04-no-destroy-invariant.md`). Open the context by hand so
    assertions land on your handler's RETURN rather than the harness's
    drain:

    ```go
    pctx, err := pulumi.NewContext(context.Background(), pulumi.RunInfo{
        Project: "test-project",
        Stack:   "test-stack",
        Mocks:   mocks,
    })
    defer pctx.Close()  // no-op under mocks
    ```

    If your handler returns before awaiting a registration, the mock
    captures no call and the URN assertion fails — exactly what you
    want.

Every URN your plugin produces for a Pulumi resource embeds a resource
**name**. Pulumi state keys off that name. If a future refactor changes
one, every operator's `sc deploy` under the previous plugin version will
replace+create (and in the DNS record case, actively delete live DNS).
The URN contract test freezes those names.

Add one test per Pulumi `type:name` your plugin registers. Pattern:

```go
m := &captureMocks{}
withTrapFreeContext(t, m, func(ctx *pulumi.Context) error {
    _, err := NewRegistrar(ctx, "stack.example.com", "configured.example.com")
    // ... exercise the handler ...
    return err
})
Expect(m.namesByType("<cloud>:dns:Record")).To(Equal([]string{"www-A"}))
```

---

## 5. Release

Push to `main` triggers the release workflow in
`.github/workflows/release.yml`:

- CalVer tag `vYYYY.MM.DD-<short-sha>`.
- Three binaries (linux/amd64, linux/arm64, darwin/arm64).
- Consolidated `sha256sums.txt`.
- GitHub Release with auto-generated notes.

!!! warning "Keep the OOM pins"
    The workflow's `GOMAXPROCS=1`, `GOMEMLIMIT=6GiB`, and
    `go build -p 1` are **not stylistic**. Default parallelism OOMs the
    standard 7 GB GHA runner on a plugin importing a big cloud SDK
    (pulumi-yandex is ~2 GB of generated types; pulumi-sbercloud is
    similar). Four of six existing SC plugin repos learned this on their
    first release run. Keep the pins even for a plugin whose SDK is
    small today — the cost is ~2 minutes of wall time and the saving is
    an avoidable round trip.

Manually-pushed `v*` tags produce named releases for pinning or rollback.

---

## 6. The gotchas

Each cost a round trip at least once. Keep them in mind as you author. The
canonical trap-by-trap catalogue (fifteen entries as of 2026-11-01) lives at
[forge/docs/design/2026-11-01/plugin-extraction-playbook/](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-11-01/plugin-extraction-playbook/)
— the subset below is the ones you will hit as an author rather than as the
operator picking up a bad release.

### 6.1 Zone-vs-baseDnsZone — the 2026-10-28 incident

A DNS registrar's `zoneName` config is a **default**. The deploying
stack's `baseDnsZone` is what MUST drive the actual zone lookup. SC core
sends it in `ProvisionRegistrarRequest.BaseDnsZone` (api v2026.10.37
and later). Writing a record into the wrong zone with
create-before-delete semantics *deletes* the live record from the right
one. See Addendum R of `04-no-destroy-invariant.md`.

```go
pref := &pApi.DnsPreference{BaseZone: req.GetBaseDnsZone()}
if pref.BaseZone == "" {
    pref = nil  // older core, or stack with no preference
}
```

### 6.2 Drain before you return

Pulumi fires `RegisterResource` from a goroutine. A handler that returns
before those registrations finish lets `pctx.Close()` tear the engine
connection down mid-flight. SC core sees no registrations for the handler's
run, concludes "those resources no longer exist in the program", and plans a
destroy of each state entry on the next apply. See the `wait.go` package doc
of [`pkg/plugin/runtime`](https://github.com/simple-container-com/api/blob/main/pkg/plugin/runtime/wait.go)
for the Pulumi-SDK linkname that `runtime.Settle` uses to drain.

**Don't skip the `defer runtime.Settle(rctx, &resp, &err)` line.** The
template wires it correctly out of the box; rolling your own with
`pctx.Close()` instead is the shape that cost every pre-cascade plugin
version one round trip.

### 6.3 `go.mod` replace directives don't resolve on CI

Local development with
`replace github.com/simple-container-com/api => /tmp/path` resolves on
your laptop and nowhere else. For a not-yet-merged API change, run
`go get github.com/simple-container-com/api@<branch>` to pin a
pseudo-version against the SHA, and `go mod tidy` to lock it. Commit
the resulting `go.sum`.

The `/plugin` module's `replace github.com/.../sc-plugin-<cloud>/api => ../api`
IS fine — that's a sibling module in the same repo and CI checks it
out together.

### 6.4 Operator-named resources need a stable URN separate from the export key

When you honour an operator's `name:` override (`atlasCfg.ClusterName`, a
bucket's explicit `bucketName`), use it ONLY for the resource's cloud-side
name and for export keys consumer stacks will read. The Pulumi URN that
anchors state **must stay stable** — derive it from the stack's identity, not
from a knob the operator can change. If you let the operator rename a cluster
and that rename flows into the Pulumi logical name, every orphan output under
the old name reads as "no longer in program" and gets destroyed on the next
apply — including the operator-owned Secret the parent exported.

**Pattern:**

```go
pulumiResourceName := toClusterName(stack.Name, input)   // stable, used in URNs
exportClusterName := pulumiResourceName
if cfg.ClusterName != "" {
    exportClusterName = cfg.ClusterName                  // operator-visible
}
cluster, _ := mongodbatlas.NewCluster(ctx,
    fmt.Sprintf("%s-cluster", pulumiResourceName),       // URN-anchored
    &mongodbatlas.ClusterArgs{Name: sdk.StringPtr(exportClusterName)})
ctx.Export(toClusterIdExport(exportClusterName), cluster.ClusterId)
```

Live-caught on forge-notifier run 41330569069 (2026-11-01) — the mongodb
plugin coupled URN naming to the operator's custom cluster name and renaming
the cluster deleted the CNAME and MONGO_PASSWORD secret. Fixed in
`sc-plugin-mongodb v2026.11.01-c9f5514`. See [playbook trap 13](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-11-01/plugin-extraction-playbook/README.md).

### 6.5 Collector helpers that distinguish incoming vs outgoing env vars

A plugin that runs both Provision (reads INCOMING env vars from the consumer
stack's `uses:` resources) **and** ComputeProcess (produces OUTGOING env
vars from this plugin's own resources) needs two separate pools in its
`ComputeContextCollector`. A single-pool implementation that returns `nil`
from `EnvVariables()` / `SecretEnvVariables()` silently drops the consumer's
MONGO_PASSWORD / bucket env / anything else their parent exported. The
deploy reads green; the deployed container boots with the env unset.

**Pattern:** seed the INCOMING pool from `req.GetComputeEnvVars()` in your
newProvisionCollector; expose `EnvVariables()` / `SecretEnvVariables()` as
reads of that pool. The OUTGOING pool is what `AddEnvVariableIfNotExist` /
`AddSecretEnvVariableIfNotExist` write into during ComputeProcess, and what
`ToResponse()` ships back in `ComputeProcessResponse.EnvVars`.

Live-caught on forge-notifier run 41336224941 (2026-11-01). Fixed in
`sc-plugin-aws v2026.11.01-91284ba`. See
[playbook trap 14](https://github.com/simple-container-com/forge/blob/main/docs/design/2026-11-01/plugin-extraction-playbook/README.md).

### 6.6 `gci` section lists are per-repo

Each SC repo has its own `.golangci.yml` with a `linters-settings.gci.sections`
list. **Copy the one from the repo you're linting**, not the one you
remember. Running `gci` with the wrong section list is a lint-only diff —
`gofmt` doesn't catch it — and it fails CI from an operator-invisible place.

---

## 7. Prior art

Five plugin repos live today, each highlighting a different pattern:

- [`sc-plugin-cloudflare`](https://github.com/simple-container-com/sc-plugin-cloudflare) —
  registrar-only. Simplest handler surface; cleanest reference for the
  `ProvisionRegistrar` / `NewDnsRecord` / `NewOverrideHeaderRule` /
  `ProvisionDomainForEndpoint` RPCs. Pages Direct Upload for static sites.
- [`sc-plugin-yandex`](https://github.com/simple-container-com/sc-plugin-yandex) —
  full provider + resource + state-store + secrets-provider surface.
  Origin of the release-workflow OOM pins and the `settle()` /
  `finishPulumiContext()` pattern now packaged in `pkg/plugin/runtime`.
  Multi-registrar slice + longest-zone picker; composite-output
  resolution for YC objects with multi-field IDs.
- [`sc-plugin-cloudru`](https://github.com/simple-container-com/sc-plugin-cloudru) —
  dual-mode binary (plugin + lifted cobra CLI). The pattern for clouds
  with no Pulumi provider at all: resources become `pulumi-command`
  `local.Command`s shelling to the plugin binary by absolute path from
  the plugin cache. VM driver + custom `cloudru-evolution-vm` plus
  the `VmHosts` RPC flow.
- [`sc-plugin-mongodb`](https://github.com/simple-container-com/sc-plugin-mongodb) —
  the first ComputeProcess-heavy plugin. `uses:` fan-out produces
  `MONGO_URI` / `MONGO_USER` / `MONGO_PASSWORD` env vars with secret
  flags preserved. `PreDestroy` with a database-drop hook. Also the
  origin of the URN-vs-export-key split for operator-named clusters
  (see §6.5).
- [`sc-plugin-aws`](https://github.com/simple-container-com/sc-plugin-aws) —
  the biggest surface by far: provider + state-store (`s3-bucket`) +
  secrets-provider (`aws-kms`) + templates (`aws-lambda`, `ecs-fargate`,
  `aws-static-website`) + resources (`s3-bucket`, `ecr-repository`,
  `aws-rds-postgres`, `aws-rds-mysql`, `aws-cloudtrail-security-alerts`)
  + the `cloud-helpers` Lambda. Also the first plugin to use the
  `BrokerAware` → `PeerInvoke` path to delegate DNS registration to
  cloudflare without a Go-module import (D8).

The template at
[`sc-plugin-echo`](https://github.com/simple-container-com/sc-plugin-echo)
is lifted from these five — cloudflare's RPC shape, yandex's release
workflow, cloudru's `init()` + `RegisterPluginRequirement` pattern,
mongodb's ComputeProcess + PreDestroy shape, aws's PeerInvoke bridge.

## 8. The acceptance test

**A new team member clones the template, implements a driver, cuts a
release, `sc` loads it — without reading SC core.** If you got this far,
you did.
