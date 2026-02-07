# Security Configuration Schema Reference

Complete reference for security configuration in Simple Container.

## SecurityDescriptor

Top-level security configuration.

```yaml
security:
  enabled: boolean          # Enable security operations (default: false)
  scan: ScanDescriptor     # Vulnerability scanning config
  signing: SigningDescriptor # Image signing config
  sbom: SBOMDescriptor     # SBOM generation config
  provenance: ProvenanceDescriptor # Provenance attestation config
```

## ScanDescriptor

Vulnerability scanning configuration.

```yaml
scan:
  enabled: boolean          # Enable vulnerability scanning (default: false)
  tools:                    # Scanner tools to use
    - name: string          # Tool name: grype, trivy, or all
  failOn: string           # Block deployment on severity: critical, high, medium, low
  warnOn: string           # Warn on severity (doesn't block)
  required: boolean        # Fail deployment if scan fails (default: false)
  cache:
    enabled: boolean       # Enable scan result caching (default: true)
    ttl: duration         # Cache TTL (default: 6h)
  output:
    local: string         # Local path to save results
    registry: boolean     # Attach results to registry (default: false)
```

**Example:**
```yaml
scan:
  enabled: true
  tools:
    - name: grype
  failOn: critical
  warnOn: high
  required: true
```

## SigningDescriptor

Image signing configuration.

```yaml
signing:
  enabled: boolean          # Enable image signing (default: false)
  keyless: boolean          # Use keyless signing with OIDC (default: true)
  privateKey: string        # Path to private key (for key-based signing)
  publicKey: string         # Path to public key (for verification)
  oidcIssuer: string       # OIDC issuer URL (default: https://oauth2.sigstore.dev/auth)
  identityRegexp: string   # Identity pattern for verification
  required: boolean        # Fail deployment if signing fails (default: false)
```

**Example (Keyless):**
```yaml
signing:
  enabled: true
  keyless: true
  required: true
```

**Example (Key-based):**
```yaml
signing:
  enabled: true
  keyless: false
  privateKey: /secrets/cosign.key
  publicKey: /secrets/cosign.pub
```

## SBOMDescriptor

SBOM generation configuration.

```yaml
sbom:
  enabled: boolean          # Enable SBOM generation (default: false)
  format: string           # SBOM format: cyclonedx-json, cyclonedx-xml, spdx-json, spdx-tag-value, syft-json
  generator: string        # Generator tool: syft (default)
  required: boolean        # Fail deployment if SBOM generation fails (default: false)
  cache:
    enabled: boolean       # Enable SBOM caching (default: true)
    ttl: duration         # Cache TTL (default: 24h)
  output:
    local: string         # Local path to save SBOM
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
    local: string         # Local path to save provenance
    registry: boolean     # Attach provenance as attestation to registry (default: false)
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
      failOn: critical
      warnOn: high
      required: true
      cache:
        enabled: true
        ttl: 6h
      output:
        local: .sc/scan-results/
    signing:
      enabled: true
      keyless: true
      required: true
    sbom:
      enabled: true
      format: cyclonedx-json
      generator: syft
      cache:
        enabled: true
        ttl: 24h
      output:
        local: .sc/sbom/
        registry: true
      required: true
    provenance:
      enabled: true
      format: slsa-v1.0
      includeGit: true
      includeDocker: true
      output:
        registry: true
      required: false
```

## Configuration Inheritance

Child stacks can inherit and override parent security configuration:

**Parent (base.yaml):**
```yaml
client:
  security:
    enabled: true
    scan:
      failOn: high
```

**Child (production.yaml):**
```yaml
parent: base
client:
  security:
    scan:
      failOn: critical  # Overrides parent
```

**Result:** Production has stricter scanning (critical) while inheriting other settings.
