# Security Configuration Schema Reference

Complete reference for security configuration in Simple Container.

## SecurityDescriptor

Top-level security configuration.

```yaml
security:
  enabled: boolean            # Enable security operations (default: false)
  scan: ScanDescriptor        # Vulnerability scanning config
  signing: SigningDescriptor # Image signing config
  sbom: SBOMDescriptor        # SBOM generation config
  provenance: ProvenanceDescriptor # Provenance attestation config
  reporting: ReportingDescriptor   # External reporting / PR comment config
```

## ScanDescriptor

Vulnerability scanning configuration.

```yaml
scan:
  enabled: boolean          # Enable vulnerability scanning (default: false)
  tools:                    # Scanner tools to use
    - name: string          # Tool name: grype or trivy
      enabled: boolean      # Enable this scanner entry (default: true when omitted, explicit false is respected)
      required: boolean     # Fail if this specific scanner fails
      failOn: string        # Override global fail threshold for this scanner
      warnOn: string        # Override global warn threshold for this scanner
  failOn: string           # Optional quality gate on severity: critical, high, medium, low
  warnOn: string           # Warn on severity (doesn't block, default: high)
  required: boolean        # Fail deployment if scan fails (default: false)
  cache:
    enabled: boolean       # Enable scan result caching (default: true)
    ttl: duration         # Cache TTL (default: 6h)
    dir: string           # Optional cache directory
  output:
    local: string         # Local file path for merged JSON results
    registry: boolean     # Reserved for future registry export support; currently not used for vulnerability scans
```

**Example:**
```yaml
scan:
  enabled: true
  tools:
    - name: grype
  warnOn: high
```

## SigningDescriptor

Image signing configuration.

```yaml
signing:
  enabled: boolean          # Enable image signing (default: false)
  keyless: boolean          # Use keyless signing with OIDC (default: true)
  privateKey: string        # Path to private key (for key-based signing)
  publicKey: string         # Path to public key (for verification)
  required: boolean        # Fail deployment if signing fails (default: false)
  verify:
    enabled: boolean
    oidcIssuer: string       # OIDC issuer URL for verification
    identityRegexp: string   # Identity pattern for keyless verification
```

**Example (Keyless):**
```yaml
signing:
  enabled: true
  keyless: true
  required: true
  verify:
    enabled: true
    oidcIssuer: https://token.actions.githubusercontent.com
    identityRegexp: ^https://github.com/myorg/myrepo/.github/workflows/.*$
```

**Example (Key-based):**
```yaml
signing:
  enabled: true
  keyless: false
  privateKey: /secrets/cosign.key
  publicKey: /secrets/cosign.pub
```

For GitHub Actions keyless signing, the workflow job also needs:

```yaml
permissions:
  contents: read
  id-token: write
```

## SBOMDescriptor

SBOM generation configuration.

```yaml
sbom:
  enabled: boolean          # Enable SBOM generation (default: false)
  format: string           # SBOM format: cyclonedx-json, cyclonedx-xml, spdx-json, spdx-tag-value, syft-json
  generator: string        # Generator tool: syft (default)
  required: boolean        # Fail deployment if SBOM generation fails (default: false)
  attach:
    enabled: boolean       # Attach SBOM attestation to the image (default: true)
    sign: boolean          # Signed registry attestation (must be true when output.registry=true)
  cache:
    enabled: boolean       # Enable SBOM caching (default: true)
    ttl: duration         # Cache TTL (default: 24h)
    dir: string           # Optional cache directory
  output:
    local: string         # Local file path to save the generated SBOM
    registry: boolean     # Attach SBOM as attestation to registry (default: false)
```

**Example:**
```yaml
sbom:
  enabled: true
  format: cyclonedx-json
  generator: syft
  output:
    local: .sc/artifacts/sbom/
    registry: true
  required: true
```

## ProvenanceDescriptor

Provenance attestation configuration.

```yaml
provenance:
  enabled: boolean          # Enable provenance attestation (default: false)
  format: string           # Format: slsa-v1.0 (default)
  includeGit: boolean      # Include git metadata (default: true)
  includeDocker: boolean   # Include Dockerfile metadata (default: true)
  required: boolean        # Fail deployment if provenance generation fails (default: false)
  output:
    local: string         # Local file path to save provenance
    registry: boolean     # Preserve an attached registry attestation (default: false)
  builder:
    id: string            # Optional builder ID override
  metadata:
    includeEnv: boolean
    includeMaterials: boolean
```

**Example:**
```yaml
provenance:
  enabled: true
  format: slsa-v1.0
  includeGit: true
  includeDocker: true
  output:
    registry: true
```

## ReportingDescriptor

Security report publication configuration.

```yaml
reporting:
  defectdojo:
    enabled: boolean
    url: string              # Required when enabled
    apiKey: string           # Required when enabled
    engagementId: integer    # Required when using an existing engagement
    engagementName: string   # Required when autoCreate=true and engagementId is not set
    productId: integer       # Required when autoCreate=true and engagementId is not set, unless productName is used
    productName: string      # Required when autoCreate=true and engagementId is not set, unless productId is used
    testType: string
    environment: string       # Optional; must match an existing DefectDojo environment if set
    tags: [string]
    autoCreate: boolean
  prComment:
    enabled: boolean
    output: string           # Markdown file for CI to post as a sticky PR comment
```

Supported DefectDojo modes:

- Existing engagement: `url`, `apiKey`, and `engagementId`
- Auto-create product + engagement: `url`, `apiKey`, `autoCreate: true`, `engagementName`, and one of `productId` or `productName`

## Complete Example

```yaml
client:
  security:
    enabled: true
    scan:
      enabled: true
      tools:
        - name: grype
        - name: trivy
      warnOn: high
      cache:
        enabled: true
        ttl: 6h
        dir: .sc/cache/security
      output:
        local: .sc/scan-results/results.json
    signing:
      enabled: true
      keyless: true
      verify:
        enabled: true
        oidcIssuer: https://token.actions.githubusercontent.com
        identityRegexp: ^https://github.com/myorg/myrepo/.github/workflows/.*$
    sbom:
      enabled: true
      format: cyclonedx-json
      generator: syft
      attach:
        enabled: true
        sign: true
      cache:
        enabled: true
        ttl: 24h
        dir: .sc/cache/security
      output:
        local: .sc/sbom/sbom.json
        registry: true
    provenance:
      enabled: true
      format: slsa-v1.0
      includeGit: true
      includeDocker: true
      output:
        local: .sc/provenance/provenance.json
        registry: true
      required: false
    reporting:
      defectdojo:
        enabled: true
        url: https://defectdojo.example.com
        apiKey: ${secret:defectdojo-api-key}
        productName: my-service
        engagementName: staging
        testType: Container Scan
        environment: staging
        autoCreate: true
      prComment:
        enabled: true
        output: .sc/scan-results/comment.md
```

## Configuration Inheritance

Child stacks can inherit and override parent security configuration:

**Parent (base.yaml):**
```yaml
client:
  security:
    enabled: true
    scan:
      warnOn: high
```

**Child (production.yaml):**
```yaml
parent: base
client:
  security:
    scan:
      failOn: critical  # Adds a stricter quality gate in production
```

**Result:** Production keeps inherited warnings while adding a stricter quality gate.
