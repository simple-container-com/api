# Integration & Data Flow - Container Image Security

**Issue:** #105 - Container Image Security
**Document:** Integration Architecture and Data Flow
**Date:** 2026-02-05

---

## Table of Contents

1. [Integration Points](#integration-points)
2. [Data Flow Diagrams](#data-flow-diagrams)
3. [Execution Sequences](#execution-sequences)
4. [Pulumi Integration](#pulumi-integration)
5. [CI/CD Integration](#cicd-integration)
6. [Registry Integration](#registry-integration)
7. [Error Handling Flow](#error-handling-flow)

---

## Integration Points

### 1. Docker Build & Push Integration

**Primary Integration Point:** `pkg/clouds/pulumi/docker/build_and_push.go`

**Current Flow:**
```
BuildAndPushImage()
  → Build image with Docker
  → Push to registry
  → Return ImageOut with resource options
```

**Enhanced Flow:**
```
BuildAndPushImage()
  → Build image with Docker
  → Push to registry
  → Check if SecurityDescriptor configured
  → IF security enabled:
      → Execute security operations
      → Add security commands to Pulumi DAG
  → Return ImageOut with extended resource options
```

**Code Integration:**

```go
// File: pkg/clouds/pulumi/docker/build_and_push.go

func BuildAndPushImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image Image) (*ImageOut, error) {
    // ... existing build and push logic ...

    // NEW: Security operations integration
    if stack.Client != nil && stack.Client.Security != nil {
        securityOpts, err := executeSecurityOperations(ctx, res, stack, params, deployParams, image)
        if err != nil {
            // Log error but continue (fail-open by default)
            params.Log.Warn(ctx.Context(), "Security operations failed: %v", err)
        } else {
            // Add security command dependencies
            addOpts = append(addOpts, securityOpts...)
        }
    }

    addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{res}))
    return &ImageOut{
        Image:   res,
        AddOpts: addOpts,
    }, nil
}

// executeSecurityOperations runs security operations via Pulumi commands
func executeSecurityOperations(
    ctx *sdk.Context,
    dockerImage *docker.Image,
    stack api.Stack,
    params pApi.ProvisionParams,
    deployParams api.StackParams,
    image Image,
) ([]sdk.ResourceOption, error) {
    // Create execution context
    execContext, err := security.NewExecutionContext(stack, deployParams)
    if err != nil {
        return nil, err
    }

    // Create security executor
    executor, err := security.NewExecutor(
        stack.Client.Security,
        execContext,
        params.Log,
    )
    if err != nil {
        return nil, err
    }

    // Execute with Pulumi integration
    return executor.ExecuteWithPulumi(ctx, dockerImage, stack.Client.Security)
}
```

### 2. Stack Configuration Integration

**Files Modified:**
- `pkg/api/client.go` - Add `Security *SecurityDescriptor` field
- `pkg/api/security_config.go` - New file with security config types

**Configuration Loading:**

```go
// File: pkg/api/client.go (modified)

type StackConfigSingleImage struct {
    BaseDnsZone        string                 `json:"baseDnsZone" yaml:"baseDnsZone"`
    Domain             string                 `json:"domain" yaml:"domain"`
    Image              ImageDescriptor        `json:"image" yaml:"image"`
    // ... existing fields ...

    // NEW: Security configuration
    Security           *SecurityDescriptor    `json:"security,omitempty" yaml:"security,omitempty"`
}
```

**YAML Example:**

```yaml
# .sc/stacks/myapp/client.yaml
schemaVersion: "1.0"
baseDnsZone: example.com
domain: myapp.example.com
image:
  name: myapp
  context: ./
  dockerfile: Dockerfile
  platform: linux/amd64

# NEW: Security configuration
security:
  signing:
    enabled: true
    keyless: true
    verify:
      enabled: true
      oidcIssuer: "https://token.actions.githubusercontent.com"
      identityRegexp: "^https://github.com/myorg/.*$"

  sbom:
    enabled: true
    format: cyclonedx-json
    attach:
      enabled: true
      sign: true

  provenance:
    enabled: true

  scan:
    enabled: true
    tools:
      - name: grype
        required: true
        failOn: critical
```

### 3. CLI Command Integration

**New Command Structure:**

```
sc (root)
├── image
│   ├── sign        # sc image sign
│   ├── verify      # sc image verify
│   └── scan        # sc image scan
├── sbom
│   ├── generate    # sc sbom generate
│   ├── attach      # sc sbom attach
│   └── verify      # sc sbom verify
├── provenance
│   ├── attach      # sc provenance attach
│   └── verify      # sc provenance verify
└── release
    └── create      # sc release create (integrated workflow)
```

**Command Registration:**

```go
// File: pkg/cmd/root_cmd/root.go (modified)

func InitCommands(rootCmd *cobra.Command) {
    // ... existing commands ...

    // NEW: Security commands
    rootCmd.AddCommand(cmd_image.NewImageCommand())
    rootCmd.AddCommand(cmd_sbom.NewSBOMCommand())
    rootCmd.AddCommand(cmd_provenance.NewProvenanceCommand())
    rootCmd.AddCommand(cmd_release.NewReleaseCommand())
}
```

---

## Data Flow Diagrams

### 1. Full Security Workflow

```
┌─────────────────────────────────────────────────────────────────┐
│  User: sc deploy -s mystack -e production                       │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  Load Stack Configuration                                        │
│  - Parse .sc/stacks/mystack/client.yaml                         │
│  - Load SecurityDescriptor if present                           │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  Pulumi: BuildAndPushImage()                                    │
│  1. Build Docker image                                          │
│  2. Push to registry (ECR, GCR, etc.)                           │
│  3. Get image digest                                            │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  Check: Is SecurityDescriptor configured?                       │
└────────┬───────────────────────────────────────┬────────────────┘
         │ NO                                     │ YES
         ▼                                        ▼
    ┌─────────┐                     ┌────────────────────────────┐
    │  Skip   │                     │ security.ExecuteWithPulumi()│
    │Security │                     └────────────┬───────────────┘
    └─────────┘                                  │
                                                 ▼
                                    ┌────────────────────────────┐
                                    │ Create ExecutionContext    │
                                    │ - Detect CI environment    │
                                    │ - Extract OIDC token       │
                                    │ - Get git metadata         │
                                    └────────────┬───────────────┘
                                                 │
                                                 ▼
                                    ┌────────────────────────────┐
                                    │ Check Tool Availability    │
                                    │ - cosign version check     │
                                    │ - syft version check       │
                                    │ - grype version check      │
                                    └────────────┬───────────────┘
                                                 │
                                                 ▼
        ┌────────────────────────────────────────────────────────────┐
        │                SECURITY OPERATIONS                         │
        │                                                            │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 1. SCAN (Fail-Fast)                          │        │
        │  │    - Run Grype scan                          │        │
        │  │    - Run Trivy scan (optional)               │        │
        │  │    - Check policy: failOn=critical           │        │
        │  │    - IF critical found: STOP (fail-closed)   │        │
        │  └────────────────┬─────────────────────────────┘        │
        │                   ▼                                        │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 2. SIGN IMAGE                                │        │
        │  │    - Keyless: cosign sign (OIDC)             │        │
        │  │    - Key-based: cosign sign --key            │        │
        │  │    - Store signature in registry             │        │
        │  │    - Log Rekor entry                         │        │
        │  └────────────────┬─────────────────────────────┘        │
        │                   ▼                                        │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 3. GENERATE SBOM (Parallel with 4)           │        │
        │  │    - syft scan image                         │        │
        │  │    - Generate CycloneDX JSON                 │        │
        │  │    - Save locally (if configured)            │        │
        │  └────────────────┬─────────────────────────────┘        │
        │                   ▼                                        │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 4. ATTACH SBOM ATTESTATION                   │        │
        │  │    - cosign attest --predicate sbom.json     │        │
        │  │    - Sign attestation                        │        │
        │  │    - Push to registry                        │        │
        │  └────────────────┬─────────────────────────────┘        │
        │                   ▼                                        │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 5. GENERATE PROVENANCE                       │        │
        │  │    - Collect build materials                 │        │
        │  │    - Generate SLSA v1.0 provenance           │        │
        │  │    - Include builder ID, commit SHA          │        │
        │  └────────────────┬─────────────────────────────┘        │
        │                   ▼                                        │
        │  ┌──────────────────────────────────────────────┐        │
        │  │ 6. ATTACH PROVENANCE ATTESTATION             │        │
        │  │    - cosign attest --predicate provenance    │        │
        │  │    - Sign attestation                        │        │
        │  │    - Push to registry                        │        │
        │  └──────────────────────────────────────────────┘        │
        └────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│  Return Pulumi Resource Dependencies                             │
│  - All security commands as Pulumi dependencies                 │
│  - Deployment waits for security operations                     │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  Continue Deployment                                             │
│  - ECS Task Definition / Cloud Run Service                      │
│  - Use signed, scanned, attested image                          │
└─────────────────────────────────────────────────────────────────┘
```

### 2. Configuration Inheritance Flow

```
┌──────────────────────────────────────┐
│ Parent Stack (Security Baseline)     │
│ .sc/parent-stacks/security.yaml      │
│                                       │
│ security:                             │
│   signing:                            │
│     enabled: true                     │
│     keyless: true                     │
│   sbom:                               │
│     enabled: true                     │
│   scan:                               │
│     enabled: true                     │
│     tools:                            │
│       - name: grype                   │
│         failOn: critical              │
└──────────────┬───────────────────────┘
               │ INHERITS
               │
               ▼
┌──────────────────────────────────────┐
│ Child Stack (Application)             │
│ .sc/stacks/myapp/client.yaml         │
│                                       │
│ uses: security                        │
│                                       │
│ # Optional overrides                 │
│ security:                             │
│   scan:                               │
│     tools:                            │
│       - name: grype                   │
│         failOn: high   # Override     │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│ Merged Configuration                  │
│                                       │
│ security:                             │
│   signing:                            │
│     enabled: true      # From parent  │
│     keyless: true      # From parent  │
│   sbom:                               │
│     enabled: true      # From parent  │
│   scan:                               │
│     enabled: true      # From parent  │
│     tools:                            │
│       - name: grype                   │
│         failOn: high   # From child   │
└───────────────────────────────────────┘
```

---

## Execution Sequences

### Sequence 1: Keyless Signing in GitHub Actions

```
┌─────┐          ┌──────────┐        ┌────────┐        ┌────────┐        ┌─────────┐
│ CLI │          │ Executor │        │ Signer │        │ Cosign │        │ Sigstore│
└──┬──┘          └────┬─────┘        └───┬────┘        └───┬────┘        └────┬────┘
   │                  │                   │                 │                  │
   │ Deploy           │                   │                 │                  │
   ├─────────────────>│                   │                 │                  │
   │                  │                   │                 │                  │
   │                  │ Detect CI         │                 │                  │
   │                  ├──────────┐        │                 │                  │
   │                  │          │        │                 │                  │
   │                  │<─────────┘        │                 │                  │
   │                  │ CI=github-actions │                 │                  │
   │                  │                   │                 │                  │
   │                  │ Get OIDC Token    │                 │                  │
   │                  ├──────────┐        │                 │                  │
   │                  │          │        │                 │                  │
   │                  │<─────────┘        │                 │                  │
   │                  │ token=eyJhbG...   │                 │                  │
   │                  │                   │                 │                  │
   │                  │ Sign(image, opts) │                 │                  │
   │                  ├──────────────────>│                 │                  │
   │                  │                   │                 │                  │
   │                  │                   │ cosign sign     │                  │
   │                  │                   │ --yes           │                  │
   │                  │                   │ image:tag       │                  │
   │                  │                   ├────────────────>│                  │
   │                  │                   │                 │                  │
   │                  │                   │                 │ Fulcio: Issue    │
   │                  │                   │                 │ Cert with OIDC   │
   │                  │                   │                 ├─────────────────>│
   │                  │                   │                 │                  │
   │                  │                   │                 │ Certificate      │
   │                  │                   │                 │<─────────────────┤
   │                  │                   │                 │                  │
   │                  │                   │                 │ Rekor: Log Entry │
   │                  │                   │                 ├─────────────────>│
   │                  │                   │                 │                  │
   │                  │                   │                 │ Entry UUID       │
   │                  │                   │                 │<─────────────────┤
   │                  │                   │                 │                  │
   │                  │                   │ Success         │                  │
   │                  │                   │<────────────────┤                  │
   │                  │                   │                 │                  │
   │                  │ SignResult        │                 │                  │
   │                  │<──────────────────┤                 │                  │
   │                  │                   │                 │                  │
   │ Success          │                   │                 │                  │
   │<─────────────────┤                   │                 │                  │
   │                  │                   │                 │                  │
```

### Sequence 2: SBOM Generation and Attestation

```
┌─────┐       ┌──────────┐      ┌───────────┐      ┌──────┐      ┌──────────┐
│ CLI │       │ Executor │      │ Generator │      │ Syft │      │ Attacher │
└──┬──┘       └────┬─────┘      └─────┬─────┘      └──┬───┘      └────┬─────┘
   │               │                   │                │               │
   │ Deploy        │                   │                │               │
   ├──────────────>│                   │                │               │
   │               │                   │                │               │
   │               │ Generate SBOM     │                │               │
   │               ├──────────────────>│                │               │
   │               │                   │                │               │
   │               │                   │ syft scan      │               │
   │               │                   │ registry:image │               │
   │               │                   │ -o cyclonedx   │               │
   │               │                   ├───────────────>│               │
   │               │                   │                │               │
   │               │                   │ SBOM JSON      │               │
   │               │                   │<───────────────┤               │
   │               │                   │                │               │
   │               │                   │ Parse metadata │               │
   │               │                   ├───────┐        │               │
   │               │                   │       │        │               │
   │               │                   │<──────┘        │               │
   │               │                   │                │               │
   │               │ SBOM              │                │               │
   │               │<──────────────────┤                │               │
   │               │                   │                │               │
   │               │ Attach(image,sbom)│                │               │
   │               ├───────────────────┼────────────────┼──────────────>│
   │               │                   │                │               │
   │               │                   │                │               │ Write SBOM
   │               │                   │                │               │ to tmpfile
   │               │                   │                │               ├──────┐
   │               │                   │                │               │      │
   │               │                   │                │               │<─────┘
   │               │                   │                │               │
   │               │                   │                │ cosign attest │
   │               │                   │                │ --predicate   │
   │               │                   │                │ sbom.json     │
   │               │                   │                │<──────────────┤
   │               │                   │                │               │
   │               │                   │                │ Attestation   │
   │               │                   │                │ pushed        │
   │               │                   │                ├──────────────>│
   │               │                   │                │               │
   │               │ Success           │                │               │
   │               │<──────────────────┴────────────────┴───────────────┤
   │               │                   │                │               │
   │ Success       │                   │                │               │
   │<──────────────┤                   │                │               │
   │               │                   │                │               │
```

### Sequence 3: Vulnerability Scanning with Policy Enforcement

```
┌─────┐       ┌──────────┐      ┌─────────┐      ┌───────┐      ┌──────────┐
│ CLI │       │ Executor │      │ Scanner │      │ Grype │      │ Enforcer │
└──┬──┘       └────┬─────┘      └────┬────┘      └───┬───┘      └────┬─────┘
   │               │                  │                │               │
   │ Deploy        │                  │                │               │
   ├──────────────>│                  │                │               │
   │               │                  │                │               │
   │               │ Scan(image)      │                │               │
   │               ├─────────────────>│                │               │
   │               │                  │                │               │
   │               │                  │ grype scan     │               │
   │               │                  │ registry:image │               │
   │               │                  │ -o json        │               │
   │               │                  ├───────────────>│               │
   │               │                  │                │               │
   │               │                  │ Vulnerabilities│               │
   │               │                  │ JSON           │               │
   │               │                  │<───────────────┤               │
   │               │                  │                │               │
   │               │                  │ Parse results  │               │
   │               │                  ├───────┐        │               │
   │               │                  │       │        │               │
   │               │                  │<──────┘        │               │
   │               │                  │                │               │
   │               │ ScanResult       │                │               │
   │               │ Critical: 3      │                │               │
   │               │ High: 12         │                │               │
   │               │<─────────────────┤                │               │
   │               │                  │                │               │
   │               │ Enforce(results, config)          │               │
   │               ├───────────────────────────────────┼──────────────>│
   │               │                  │                │               │
   │               │                  │                │               │ Check policy:
   │               │                  │                │               │ failOn=critical
   │               │                  │                │               ├────────┐
   │               │                  │                │               │        │
   │               │                  │                │               │<───────┘
   │               │                  │                │               │
   │               │                  │                │               │ Critical > 0
   │               │                  │                │               │ FAIL!
   │               │                  │                │               │
   │               │ ERROR: Critical vulnerabilities found              │
   │               │<───────────────────────────────────────────────────┤
   │               │                  │                │               │
   │ ERROR         │                  │                │               │
   │ Deployment    │                  │                │               │
   │ blocked       │                  │                │               │
   │<──────────────┤                  │                │               │
   │               │                  │                │               │
```

---

## Pulumi Integration

### Resource Dependency Graph

```
┌─────────────────────┐
│  docker.Image       │
│  (Built & Pushed)   │
└──────────┬──────────┘
           │ DependsOn
           ▼
┌─────────────────────┐
│  local.Command      │
│  "scan-image"       │
│  (Grype scan)       │
└──────────┬──────────┘
           │ DependsOn (if scan passes)
           ▼
┌─────────────────────┐
│  local.Command      │
│  "sign-image"       │
│  (Cosign sign)      │
└──────────┬──────────┘
           │ DependsOn
           ├──────────────────┬──────────────────┐
           ▼                  ▼                  ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│ local.Command    │ │ local.Command    │ │ local.Command    │
│ "generate-sbom"  │ │ "attach-sbom"    │ │ "attach-prov"    │
│ (Syft)           │ │ (Cosign attest)  │ │ (Cosign attest)  │
└──────────┬───────┘ └──────────┬───────┘ └──────────┬───────┘
           │                    │                    │
           └────────────────────┴────────────────────┘
                                │ DependsOn
                                ▼
           ┌─────────────────────────────────┐
           │  aws.ecs.TaskDefinition         │
           │  OR                              │
           │  gcp.cloudrun.Service            │
           │  (Uses secured image)            │
           └──────────────────────────────────┘
```

### Pulumi Command Creation

```go
// Create Pulumi Command for each security operation

// 1. Scan Command
scanCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-scan", imageName),
    &local.CommandArgs{
        Create: sdk.Sprintf(
            "grype registry:%s -o json --fail-on critical",
            imageDigest,
        ),
    },
    sdk.DependsOn([]sdk.Resource{dockerImage}),
)

// 2. Sign Command
signCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-sign", imageName),
    &local.CommandArgs{
        Create: sdk.Sprintf(
            "cosign sign --yes %s",
            imageDigest,
        ),
        Environment: sdk.StringMap{
            "COSIGN_EXPERIMENTAL": sdk.String("1"),
            "SIGSTORE_ID_TOKEN":   sdk.String(oidcToken),
        },
    },
    sdk.DependsOn([]sdk.Resource{scanCmd}),
)

// 3. SBOM Generation Command
sbomCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-sbom", imageName),
    &local.CommandArgs{
        Create: sdk.Sprintf(
            "syft registry:%s -o cyclonedx-json --file /tmp/%s-sbom.json",
            imageDigest,
            imageName,
        ),
    },
    sdk.DependsOn([]sdk.Resource{signCmd}),
)

// 4. SBOM Attestation Command
attestSBOMCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-attest-sbom", imageName),
    &local.CommandArgs{
        Create: sdk.Sprintf(
            "cosign attest --yes --type cyclonedx --predicate /tmp/%s-sbom.json %s",
            imageName,
            imageDigest,
        ),
        Environment: sdk.StringMap{
            "COSIGN_EXPERIMENTAL": sdk.String("1"),
            "SIGSTORE_ID_TOKEN":   sdk.String(oidcToken),
        },
    },
    sdk.DependsOn([]sdk.Resource{sbomCmd}),
)

// Return all dependencies
return []sdk.ResourceOption{
    sdk.DependsOn([]sdk.Resource{
        scanCmd,
        signCmd,
        sbomCmd,
        attestSBOMCmd,
        // ... more commands
    }),
}, nil
```

---

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Deploy with Security

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write  # Required for OIDC keyless signing

    steps:
      - uses: actions/checkout@v4

      - name: Install Security Tools
        run: |
          # Install cosign
          curl -LO https://github.com/sigstore/cosign/releases/download/v3.0.2/cosign-linux-amd64
          sudo mv cosign-linux-amd64 /usr/local/bin/cosign
          sudo chmod +x /usr/local/bin/cosign

          # Install syft
          curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin

          # Install grype
          curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin

      - name: Deploy with Simple Container
        run: |
          sc deploy -s myapp -e production
        env:
          # OIDC token automatically available via id-token: write permission
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}

      - name: Upload SBOM
        uses: actions/upload-artifact@v4
        with:
          name: sbom
          path: .sc/artifacts/sbom/*.json
```

### GitLab CI Pipeline

```yaml
# .gitlab-ci.yml
deploy-production:
  stage: deploy
  image: simplecontainer/cli:latest
  id_tokens:
    SIGSTORE_ID_TOKEN:  # GitLab OIDC token for Sigstore
      aud: sigstore

  before_script:
    # Install security tools
    - apt-get update && apt-get install -y curl
    - curl -LO https://github.com/sigstore/cosign/releases/download/v3.0.2/cosign-linux-amd64
    - mv cosign-linux-amd64 /usr/local/bin/cosign && chmod +x /usr/local/bin/cosign
    - curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
    - curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin

  script:
    - sc deploy -s myapp -e production

  artifacts:
    paths:
      - .sc/artifacts/sbom/
    expire_in: 30 days
```

---

## Registry Integration

### OCI Artifact Storage

Security artifacts (signatures, SBOMs, provenance) are stored in the container registry using OCI artifact format:

```
docker.example.com/myapp:v1.0.0                    # Container image
  ├─ sha256:abc123...                               # Image digest
  │
  ├─ sha256:def456...                               # Signature (Cosign)
  │   └─ application/vnd.dev.cosign.simplesigning.v1+json
  │
  ├─ sha256:ghi789...                               # SBOM Attestation
  │   └─ application/vnd.in-toto+json
  │       └─ predicate: CycloneDX SBOM
  │
  └─ sha256:jkl012...                               # Provenance Attestation
      └─ application/vnd.in-toto+json
          └─ predicate: SLSA Provenance
```

### Registry API Interactions

```go
// Push signature (Cosign handles this)
cosign sign docker.example.com/myapp@sha256:abc123...
  → Push to: docker.example.com/myapp:sha256-abc123.sig

// Push SBOM attestation
cosign attest --type cyclonedx \
  --predicate sbom.json \
  docker.example.com/myapp@sha256:abc123...
  → Push to: docker.example.com/myapp:sha256-abc123.att

// Retrieve attestation
cosign verify-attestation \
  --type cyclonedx \
  docker.example.com/myapp@sha256:abc123...
  → Fetch from: docker.example.com/myapp:sha256-abc123.att
```

### Registry Compatibility

| Registry | OCI Artifacts | Keyless Signing | SBOM Attestation | Notes |
|----------|---------------|-----------------|------------------|-------|
| AWS ECR | ✅ | ✅ | ✅ | Full support |
| GCP GCR/Artifact Registry | ✅ | ✅ | ✅ | Full support |
| Docker Hub | ✅ | ✅ | ✅ | Full support |
| GitHub Container Registry | ✅ | ✅ | ✅ | Native GitHub integration |
| Harbor | ✅ | ✅ | ✅ | Full support v2.5+ |
| Azure ACR | ✅ | ✅ | ✅ | Full support |

---

## Error Handling Flow

### Fail-Fast Scanning

```
┌──────────────────────┐
│ Start Scan           │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│ Run Grype Scan       │
└──────────┬───────────┘
           │
           ▼
     ┌─────────────┐
     │ Parse Result│
     └──────┬──────┘
            │
            ▼
      ┌──────────────────────┐
      │ Check Policy:        │
      │ failOn = critical    │
      └──────┬──────┬────────┘
             │ YES  │ NO
   Critical  │      │  Continue
   Found     │      │
             ▼      ▼
    ┌────────────┐ ┌──────────────┐
    │  FAIL      │ │  Sign Image  │
    │  Stop      │ │  Continue    │
    │  Deployment│ │  Workflow    │
    └────────────┘ └──────────────┘
```

### Fail-Open Operations

```
┌──────────────────────┐
│ Start SBOM Gen       │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│ Run Syft             │
└──────────┬───────────┘
           │
           ▼
     ┌─────────────┐
     │ Check Error │
     └──────┬──────┘
            │
      ┌─────┴──────┐
      │ ERROR      │ SUCCESS
      ▼            ▼
┌───────────────┐ ┌──────────────┐
│ Log Warning   │ │ Attach SBOM  │
│ Continue      │ │ Continue     │
│ Deployment    │ │ Deployment   │
└───────────────┘ └──────────────┘
```

### Error Recovery

```go
// Retry with exponential backoff for transient failures
func executeWithRetry(ctx context.Context, operation func() error) error {
    maxRetries := 3
    baseDelay := time.Second

    for i := 0; i < maxRetries; i++ {
        err := operation()
        if err == nil {
            return nil
        }

        // Check if error is retryable
        if !isRetryable(err) {
            return err
        }

        // Exponential backoff
        delay := baseDelay * time.Duration(1<<uint(i))
        logger.Warn(ctx, "Operation failed (attempt %d/%d), retrying in %v: %v",
            i+1, maxRetries, delay, err)

        time.Sleep(delay)
    }

    return fmt.Errorf("operation failed after %d retries", maxRetries)
}

// Retryable errors: network errors, temporary registry issues
func isRetryable(err error) bool {
    // Network timeout
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }

    // DNS/connection errors
    if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
        return true
    }

    // Registry 5xx errors
    if strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), "503") {
        return true
    }

    return false
}
```

---

## Summary

This integration and data flow document provides:

1. **Integration Points** - Where security features hook into existing codebase
2. **Data Flow Diagrams** - Visual representation of execution flow
3. **Execution Sequences** - Step-by-step operation sequences
4. **Pulumi Integration** - Resource dependency management
5. **CI/CD Integration** - GitHub Actions and GitLab CI examples
6. **Registry Integration** - OCI artifact storage patterns
7. **Error Handling** - Fail-fast and fail-open strategies

**Key Integration Principles:**
- **Minimal Invasiveness** - Single hook point in `BuildAndPushImage()`
- **Declarative Configuration** - YAML-based security policies
- **Pulumi Native** - Uses `local.Command` resources for proper DAG
- **CI/CD Aware** - Auto-detects environment and configures OIDC
- **Fail-Safe** - Default fail-open behavior prevents breaking changes

**Next Steps:**
- Review [Implementation Plan](./implementation-plan.md) for file-by-file tasks
- Begin Phase 1 implementation with core infrastructure

---

**Status:** ✅ Integration & Data Flow Complete
**Related Documents:** [Architecture Overview](./README.md) | [Component Design](./component-design.md) | [API Contracts](./api-contracts.md)
