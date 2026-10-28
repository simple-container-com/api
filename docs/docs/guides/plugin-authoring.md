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

Four `plugin/cmd/sc-plugin-<cloud>/main.go` scaffolding pieces that need no
changes — keep them from the template:

- `pulumiContextFromHeader(ctx, header)` — opens one `sdk.Context` per RPC
  against the monitor address in the request header.
- `waitForRef(ref any) string` — blocks until a Pulumi
  `IDOutput`/`URNOutput`/`StringOutput` resolves. The countermeasure for
  Pulumi's async `RegisterResource`: a handler that returns before its
  outputs resolve lets `defer pctx.Close()` tear down the engine
  connection mid-registration, and the deploy hangs silently.
- `LogStream` — drains the plugin's progress Publisher onto the gRPC
  stream. Skip this and the plugin is silent for the whole of a
  multi-step RPC chain.
- `strVal(v *scpluginv1.InputValue) string` — reads a string input from
  the plugin bus's typed input map.

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

## 6. The three gotchas

Each cost a round trip at least once. Keep them in mind as you author.

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

### 6.2 Await before returning

Pulumi fires `RegisterResource` from a goroutine. A handler that returns
before its outputs resolve lets `defer pctx.Close()` tear the engine
connection down mid-registration. The resource never lands in state;
the deploy either hangs or reports green with the resource missing.

Every RPC handler must `waitForRef` (or equivalent) on the resources it
registered before returning. The template's `main.go` does this for
you — don't skip the `urn := waitForRef(out.Ref)` line.

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

### 6.4 `gci` section lists are per-repo

Each SC repo has its own `.golangci.yml` with a `linters-settings.gci.sections`
list. **Copy the one from the repo you're linting**, not the one you
remember. Running `gci` with the wrong section list is a lint-only diff —
`gofmt` doesn't catch it — and it fails CI from an operator-invisible place.

---

## 7. Prior art

Three plugin repos live today, each highlighting a different pattern:

- [`sc-plugin-cloudflare`](https://github.com/simple-container-com/sc-plugin-cloudflare) —
  registrar-only. Simplest `main.go` scaffolding; the cleanest copy source
  for `pulumiContextFromHeader`, `waitForRef`, `LogStream`.
- [`sc-plugin-yandex`](https://github.com/simple-container-com/sc-plugin-yandex) —
  full provider+resource+state-store+secrets-provider surface. Origin of
  the release-workflow OOM pins; also carries `decodeInputsAs[T]` for
  deep nested config.
- [`sc-plugin-cloudru`](https://github.com/simple-container-com/sc-plugin-cloudru) —
  dual-mode binary (plugin + lifted cobra CLI). The pattern for clouds
  with no Pulumi provider at all: resources become `pulumi-command`
  `local.Command`s shelling to the plugin binary by absolute path from
  the plugin cache.

The template at
[`sc-plugin-echo`](https://github.com/simple-container-com/sc-plugin-echo)
is lifted from these three — cloudflare's scaffolding, yandex's release
workflow, cloudru's `init()` + `RegisterPluginRequirement` pattern.

## 8. The acceptance test

**A new team member clones the template, implements a driver, cuts a
release, `sc` loads it — without reading SC core.** If you got this far,
you did.
