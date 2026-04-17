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
      warnOn: high
    signing:
      enabled: true
      keyless: true
    sbom:
      enabled: true
      format: cyclonedx-json
      output:
        local: .sc/artifacts/sbom.json
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
# Scan with grype (default warn-only behavior)
sc image scan --image myapp:v1.0

# Scan with trivy
sc image scan --image myapp:v1.0 --tool trivy

# Scan with both tools (deduplicated results) and emit SARIF
sc image scan --image myapp:v1.0 --tool all --output results.json --sarif-output results.sarif

# Add a quality gate explicitly when needed
sc image scan --image myapp:v1.0 --tool all --fail-on critical
```

**Policy Enforcement:**

- `--warn-on high`: Default CI behavior, report issues without blocking
- `--fail-on critical`: Block if Critical vulnerabilities found
- `--fail-on high`: Block if Critical OR High vulnerabilities found
- `--fail-on medium`: Block if Critical, High, OR Medium vulnerabilities found

### 2. Image Signing

Sign container images with Sigstore cosign:

```bash
# Keyless signing in CI (requires a supported OIDC token)
sc image sign --image myapp@sha256:... --keyless

# Key-based signing
sc image sign --image myapp@sha256:... --key cosign.key

# Verify a GitHub Actions keyless signature
sc image verify --image myapp@sha256:... \
  --oidc-issuer https://token.actions.githubusercontent.com \
  --identity-regexp '^https://github.com/myorg/myrepo/.github/workflows/.*$'

# Verify a key-based signature
sc image verify --image myapp@sha256:... --public-key cosign.pub
```

### 3. SBOM Generation

Generate Software Bill of Materials:

```bash
# Generate CycloneDX JSON SBOM
sc sbom generate --image myapp@sha256:... --format cyclonedx-json --output sbom.json

# Generate SPDX JSON SBOM
sc sbom generate --image myapp@sha256:... --format spdx-json --output sbom.json

# Attach SBOM as signed attestation
sc sbom attach --image myapp@sha256:... --sbom sbom.json --keyless

# Verify a GitHub Actions keyless SBOM attestation
sc sbom verify --image myapp@sha256:... \
  --keyless \
  --cert-identity '^https://github.com/myorg/myrepo/.github/workflows/.*$' \
  --cert-issuer https://token.actions.githubusercontent.com \
  --output verified-sbom.json

# Verify a key-based SBOM attestation
sc sbom verify --image myapp@sha256:... --key cosign.pub --output verified-sbom.json
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
sc provenance attach --image myapp@sha256:... --keyless

# Attach provenance and save the generated predicate locally
sc provenance attach --image myapp@sha256:... --output provenance.json --key cosign.key

# Verify provenance and assert expected digest / build identity
sc provenance verify \
  --image myapp@sha256:... \
  --keyless \
  --cert-identity '^https://github.com/myorg/myrepo/.github/workflows/.*$' \
  --cert-issuer https://token.actions.githubusercontent.com \
  --expected-commit $GITHUB_SHA \
  --output provenance.json
```

### 4a. Scan Reporting

Generate machine-readable and review-friendly artifacts from the merged scan result:

```bash
# Save merged JSON, emit a markdown PR comment artifact, and upload to DefectDojo
sc image scan \
  --image myapp:v1.0 \
  --tool all \
  --output results.json \
  --sarif-output results.sarif \
  --comment-output comment.md \
  --upload-defectdojo \
  --defectdojo-url https://defectdojo.example.com \
  --defectdojo-api-key $DEFECTDOJO_API_KEY
```

DefectDojo supports two configuration modes:

- Existing engagement: provide `url`, `apiKey`, and `engagementId`
- Auto-create product + engagement: provide `url`, `apiKey`, `autoCreate: true`, `engagementName`, and one of `productId` or `productName`
- `environment` is optional. If you set it, it must match an environment that already exists on the target DefectDojo instance.

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

Keyless signing in GitHub Actions requires `id-token: write`. Without it, `sc image sign --keyless`, `sc sbom attach --keyless`, and `sc provenance attach --keyless` cannot fetch an OIDC token.

Recommended image CI/CD order:

1. Build and push a temporary image tag.
2. Capture the immutable digest.
3. Scan the digest and emit JSON, SARIF, and PR comment artifacts.
4. Generate SBOM for the digest.
5. Sign the digest.
6. Attach SBOM and provenance attestations to the digest.
7. Verify signature and attestations.
8. Upload scan artifacts and optional DefectDojo results.
9. Promote the verified digest to release tags.

You do not need a repo-local wrapper script for this. The intended integration is to call `sc` directly from your own pipeline.

```yaml
name: Secure Image Release

on:
  push:
    branches: [main]

jobs:
  image:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write

    steps:
      - uses: actions/checkout@v4
      - name: Build sc
        run: go build -o ./bin/sc ./cmd/sc
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        id: build
        with:
          context: .
          file: Dockerfile
          push: true
          tags: myorg/myapp:ci-${{ github.run_id }}
      - name: Scan image digest
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
          DEFECTDOJO_URL: ${{ secrets.DEFECTDOJO_URL }}
          DEFECTDOJO_API_KEY: ${{ secrets.DEFECTDOJO_API_KEY }}
        run: |
          mkdir -p security-artifacts
          ./bin/sc image scan \
            --image "${IMAGE_REF}" \
            --tool all \
            --warn-on high \
            --output security-artifacts/scan.json \
            --sarif-output security-artifacts/scan.sarif \
            --comment-output security-artifacts/pr-comment.md \
            --upload-defectdojo \
            --defectdojo-url "${DEFECTDOJO_URL}" \
            --defectdojo-api-key "${DEFECTDOJO_API_KEY}" \
            --defectdojo-auto-create \
            --defectdojo-product-name myapp \
            --defectdojo-engagement-name production
      - name: Generate SBOM
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
        run: ./bin/sc sbom generate --image "${IMAGE_REF}" --format cyclonedx-json --output security-artifacts/sbom.json
      - name: Sign image
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
        run: ./bin/sc image sign --image "${IMAGE_REF}" --keyless
      - name: Verify image signature
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
        run: |
          ./bin/sc image verify \
            --image "${IMAGE_REF}" \
            --oidc-issuer https://token.actions.githubusercontent.com \
            --identity-regexp '^https://github.com/${{ github.repository }}/.github/workflows/.*$'
      - name: Attach and verify SBOM
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
        run: |
          ./bin/sc sbom attach --image "${IMAGE_REF}" --sbom security-artifacts/sbom.json --keyless
          ./bin/sc sbom verify \
            --image "${IMAGE_REF}" \
            --keyless \
            --cert-identity '^https://github.com/${{ github.repository }}/.github/workflows/.*$' \
            --cert-issuer https://token.actions.githubusercontent.com \
            --output security-artifacts/verified-sbom.json
      - name: Attach and verify provenance
        env:
          IMAGE_REF: myorg/myapp@${{ steps.build.outputs.digest }}
        run: |
          ./bin/sc provenance attach --image "${IMAGE_REF}" --keyless --output security-artifacts/provenance.json
          ./bin/sc provenance verify \
            --image "${IMAGE_REF}" \
            --keyless \
            --cert-identity '^https://github.com/${{ github.repository }}/.github/workflows/.*$' \
            --cert-issuer https://token.actions.githubusercontent.com \
            --expected-commit "${{ github.sha }}" \
            --output security-artifacts/verified-provenance.json
      - name: Promote verified digest
        run: docker buildx imagetools create --tag myorg/myapp:latest myorg/myapp@${{ steps.build.outputs.digest }}
```

### Local Binary Test

For local command testing, build the binary and use key-based signing unless you already have a valid `SIGSTORE_ID_TOKEN` from a supported OIDC issuer:

```bash
go build -o /tmp/sc ./cmd/sc
COSIGN_PASSWORD='' cosign generate-key-pair --output-key-prefix /tmp/sc-test-cosign

/tmp/sc image scan --image registry.example.com/myapp@sha256:... --tool all --output scan.json --sarif-output scan.sarif
/tmp/sc sbom generate --image registry.example.com/myapp@sha256:... --format cyclonedx-json --output sbom.json
COSIGN_PASSWORD='' /tmp/sc image sign --image registry.example.com/myapp@sha256:... --key /tmp/sc-test-cosign.key
/tmp/sc image verify --image registry.example.com/myapp@sha256:... --public-key /tmp/sc-test-cosign.pub
COSIGN_PASSWORD='' /tmp/sc sbom attach --image registry.example.com/myapp@sha256:... --sbom sbom.json --key /tmp/sc-test-cosign.key
/tmp/sc sbom verify --image registry.example.com/myapp@sha256:... --key /tmp/sc-test-cosign.pub --output verified-sbom.json
COSIGN_PASSWORD='' /tmp/sc provenance attach --image registry.example.com/myapp@sha256:... --key /tmp/sc-test-cosign.key --output provenance.json
/tmp/sc provenance verify --image registry.example.com/myapp@sha256:... --key /tmp/sc-test-cosign.pub --expected-digest sha256:... --output verified-provenance.json
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
      warnOn: high
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
      warnOn: medium
    signing:
      enabled: true
      keyless: true
    sbom:
      enabled: true
      format: cyclonedx-json
      generator: syft
      cache:
        enabled: true
        ttl: 24h
        dir: .sc/cache/security
      output:
        local: .sc/artifacts/sbom.json
        registry: true
      attach:
        enabled: true
        sign: true
    provenance:
      enabled: true
      format: slsa-v1.0
      includeGit: true
      includeDocker: true
      output:
        registry: true
      required: false
    reporting:
      defectdojo:
        enabled: true
        url: https://defectdojo.example.com
        apiKey: ${secret:defectdojo-api-key}
        productName: my-service
        engagementName: production
        autoCreate: true
        testType: Container Scan
      prComment:
        enabled: true
        output: .sc/artifacts/pr-comment.md
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
    signing:
      enabled: true
      keyless: false
      privateKey: /secrets/cosign.key
    sbom:
      enabled: true
      format: cyclonedx-json
      output:
        registry: true
      attach:
        enabled: true
        sign: true
    provenance:
      enabled: true
      output:
        registry: true
      required: false
```

## Best Practices

### 1. Warn First, Gate Deliberately
Use warnings as the default policy in CI, and add `failOn` only for environments that need a hard gate:

```yaml
scan:
  enabled: true
  warnOn: high
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
    dir: .sc/cache/security
```

### 6. DefectDojo client.yaml Examples

Existing engagement:

```yaml
client:
  security:
    enabled: true
    reporting:
      defectdojo:
        enabled: true
        url: https://defectdojo.example.com
        apiKey: ${secret:defectdojo-api-key}
        engagementId: 123
```

Auto-create product + engagement:

```yaml
client:
  security:
    enabled: true
    reporting:
      defectdojo:
        enabled: true
        url: https://defectdojo.example.com
        apiKey: ${secret:defectdojo-api-key}
        autoCreate: true
        productName: my-service
        engagementName: staging
        testType: Container Scan
        environment: staging
```

Leave `environment` unset if your DefectDojo instance does not already define that environment label.

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
3. **Adjust fail-on threshold** only where you want a quality gate (for example, production release)
4. **Disable optional features** in non-production environments

## Troubleshooting

See [Container Security Troubleshooting](../troubleshooting/container-security.md) for common issues and solutions.

## Compliance

See [NIST SP 800-218 Mapping](../compliance/nist-sp-800-218-mapping.md) for compliance documentation.
