# Container Security Guide

This guide covers the container security features in Simple Container, including vulnerability scanning, image signing, SBOM generation, and provenance attestation.

## Quick Start

### Prerequisites

Install the required security tools:

```bash
# Install cosign (for signing)
brew install cosign  # macOS
# or
wget https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64 -O /usr/local/bin/cosign
chmod +x /usr/local/bin/cosign

# Install syft (for SBOM)
brew install syft  # macOS
# or
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin

# Install grype (for vulnerability scanning)
brew install grype  # macOS
# or
curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin

# Install trivy (optional, additional scanner)
brew install trivy  # macOS
# or
wget https://github.com/aquasecurity/trivy/releases/latest/download/trivy_Linux-64bit.tar.gz
tar zxvf trivy_Linux-64bit.tar.gz && mv trivy /usr/local/bin/
```

### Basic Configuration

Add security configuration to your stack YAML:

```yaml
client:
  security:
    enabled: true
    scan:
      enabled: true
      tools:
        - name: grype
      failOn: critical
    signing:
      enabled: true
      keyless: true
    sbom:
      enabled: true
      format: cyclonedx-json
      output:
        local: .sc/artifacts/sbom/
        registry: true
    provenance:
      enabled: true
      format: slsa-v1.0
      output:
        registry: true
```

## Security Operations

### 1. Vulnerability Scanning

Scan container images for vulnerabilities before deployment:

```bash
# Scan with grype (default)
sc image scan --image myapp:v1.0 --fail-on critical

# Scan with trivy
sc image scan --image myapp:v1.0 --tool trivy --fail-on high

# Scan with both tools (deduplicated results)
sc image scan --image myapp:v1.0 --tool all --output results.json
```

**Policy Enforcement:**

- `--fail-on critical`: Block if Critical vulnerabilities found
- `--fail-on high`: Block if Critical OR High vulnerabilities found
- `--fail-on medium`: Block if Critical, High, OR Medium vulnerabilities found

### 2. Image Signing

Sign container images with Sigstore cosign:

```bash
# Keyless signing (requires OIDC)
export SIGSTORE_ID_TOKEN=$(gcloud auth print-identity-token)
sc image sign --image myapp:v1.0 --keyless

# Key-based signing
sc image sign --image myapp:v1.0 --key cosign.key

# Verify signature
sc image verify --image myapp:v1.0
```

### 3. SBOM Generation

Generate Software Bill of Materials:

```bash
# Generate CycloneDX JSON SBOM
sc sbom generate --image myapp:v1.0 --format cyclonedx-json --output sbom.json

# Generate SPDX JSON SBOM
sc sbom generate --image myapp:v1.0 --format spdx-json --output sbom.json

# Attach SBOM as signed attestation
sc sbom attach --image myapp:v1.0 --sbom sbom.json --keyless

# Verify SBOM attestation
sc sbom verify --image myapp:v1.0 --output verified-sbom.json
```

**Supported Formats:**
- `cyclonedx-json` (default)
- `cyclonedx-xml`
- `spdx-json`
- `spdx-tag-value`
- `syft-json`

### 4. Provenance Attestation

Generate SLSA provenance attestation:

```bash
# Attach provenance (auto-detects git metadata)
sc provenance attach --image myapp:v1.0 --keyless

# Verify provenance
sc provenance verify --image myapp:v1.0 --output provenance.json
```

### 5. Unified Release Workflow

Execute all security operations automatically during deployment:

```bash
# Create release with integrated security
sc release create -s mystack -e production

# Preview without deploying
sc release create -s mystack -e staging --preview

# Auto-approve deployment
sc release create -s mystack -e production --yes
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Deploy with Security

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      id-token: write  # Required for keyless signing
      contents: read

    steps:
      - uses: actions/checkout@v4

      - name: Install tools
        run: |
          curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin
          curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
          curl -sSfL https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64 -o /usr/local/bin/cosign
          chmod +x /usr/local/bin/cosign

      - name: Deploy with security
        run: sc release create -s mystack -e production --yes
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

### GitLab CI

```yaml
deploy:
  stage: deploy
  image: simple-container/sc:latest
  script:
    - apt-get update && apt-get install -y grype syft cosign
    - sc release create -s mystack -e production --yes
  only:
    - main
```

## Configuration Examples

### Minimal (Scan Only)

```yaml
client:
  security:
    enabled: true
    scan:
      enabled: true
      failOn: critical
```

### Full Security (All Features)

```yaml
client:
  security:
    enabled: true
    scan:
      enabled: true
      tools:
        - name: grype
        - name: trivy
      failOn: high
      warnOn: medium
      required: true
    signing:
      enabled: true
      keyless: true
      required: true
    sbom:
      enabled: true
      format: cyclonedx-json
      generator: syft
      output:
        local: .sc/artifacts/sbom/
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

### Production (Strict Policy)

```yaml
client:
  security:
    enabled: true
    scan:
      enabled: true
      tools:
        - name: grype
      failOn: critical
      warnOn: high
      required: true
    signing:
      enabled: true
      keyless: false
      privateKey: /secrets/cosign.key
      required: true
    sbom:
      enabled: true
      format: cyclonedx-json
      output:
        registry: true
      required: true
    provenance:
      enabled: true
      output:
        registry: true
      required: true
```

## Best Practices

### 1. Fail-Fast Scanning
Configure scanning to run FIRST in your workflow to catch vulnerabilities early:

```yaml
scan:
  enabled: true
  failOn: critical
  required: true
```

### 2. Keyless Signing in CI/CD
Use keyless signing with OIDC for CI/CD environments:

```yaml
signing:
  enabled: true
  keyless: true
```

### 3. SBOM Attachment
Always attach SBOMs to registry for supply chain transparency:

```yaml
sbom:
  enabled: true
  output:
    registry: true
```

### 4. Configuration Inheritance
Use parent stacks for base security config, override in children:

**Parent stack (base):**
```yaml
client:
  security:
    enabled: true
    scan:
      failOn: high
```

**Child stack (production - stricter):**
```yaml
parent: base
client:
  security:
    scan:
      failOn: critical
```

### 5. Cache Configuration
Enable caching for faster builds:

```yaml
sbom:
  cache:
    enabled: true
    ttl: 24h
scan:
  cache:
    enabled: true
    ttl: 6h
```

## Performance

### Overhead Benchmarks

- **Scanning**: ~2-5 seconds for small images, ~30-60 seconds for large images
- **Signing**: ~1-2 seconds (keyless), ~0.5 seconds (key-based)
- **SBOM Generation**: ~5-10 seconds for small images, ~30-90 seconds for large images
- **Provenance**: ~0.5-1 second
- **Total Overhead**: <10% of total deployment time when enabled
- **Zero Overhead**: When `enabled: false` or no security config

### Optimization Tips

1. **Enable caching** to reuse scan results and SBOMs
2. **Use single scanner** (grype OR trivy, not both) for faster scans
3. **Adjust fail-on threshold** based on environment (strict for prod, relaxed for dev)
4. **Disable optional features** in non-production environments

## Troubleshooting

See [Container Security Troubleshooting](../troubleshooting/container-security.md) for common issues and solutions.

## Compliance

See [NIST SP 800-218 Mapping](../compliance/nist-sp-800-218-mapping.md) for compliance documentation.
