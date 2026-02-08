# Security CLI Commands Reference

Complete reference for Simple Container security CLI commands.

## sc image

Image security operations.

### sc image scan

Scan container images for vulnerabilities.

**Usage:**
```bash
sc image scan --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image to scan (e.g., myapp:v1.0)
- `--tool`: Scanner tool (grype, trivy, all) (default: grype)
- `--fail-on`: Block on severity (critical, high, medium, low)
- `--output`: Output file for JSON results

**Examples:**
```bash
# Scan with grype, block on critical
sc image scan --image myapp:v1.0 --fail-on critical

# Scan with trivy
sc image scan --image myapp:v1.0 --tool trivy

# Scan with both tools, save results
sc image scan --image myapp:v1.0 --tool all --output results.json
```

**Exit Codes:**
- 0: Success, no policy violations
- 1: Scan failed or policy violation
- 2: Tool not installed

### sc image sign

Sign container images with cosign.

**Usage:**
```bash
sc image sign --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image to sign
- `--keyless`: Use keyless signing with OIDC (default: true)
- `--key`: Path to private key (for key-based signing)

**Environment Variables:**
- `SIGSTORE_ID_TOKEN`: OIDC token for keyless signing
- `COSIGN_EXPERIMENTAL`: Enable experimental features

**Examples:**
```bash
# Keyless signing
export SIGSTORE_ID_TOKEN=$(gcloud auth print-identity-token)
sc image sign --image myapp:v1.0 --keyless

# Key-based signing
sc image sign --image myapp:v1.0 --key cosign.key
```

### sc image verify

Verify container image signatures.

**Usage:**
```bash
sc image verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image to verify
- `--key`: Path to public key (for key-based verification)

**Examples:**
```bash
# Verify keyless signature
sc image verify --image myapp:v1.0

# Verify with public key
sc image verify --image myapp:v1.0 --key cosign.pub
```

## sc sbom

Software Bill of Materials operations.

### sc sbom generate

Generate SBOM for container image.

**Usage:**
```bash
sc sbom generate --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--format`: SBOM format (cyclonedx-json, cyclonedx-xml, spdx-json, spdx-tag-value, syft-json) (default: cyclonedx-json)
- `--output`: Output file path

**Examples:**
```bash
# Generate CycloneDX JSON
sc sbom generate --image myapp:v1.0 --format cyclonedx-json --output sbom.json

# Generate SPDX JSON
sc sbom generate --image myapp:v1.0 --format spdx-json --output sbom.json
```

### sc sbom attach

Attach SBOM as signed attestation to registry.

**Usage:**
```bash
sc sbom attach --image IMAGE --sbom FILE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--sbom` (required): SBOM file to attach
- `--keyless`: Use keyless signing (default: true)
- `--key`: Path to private key

**Examples:**
```bash
# Attach with keyless signing
sc sbom attach --image myapp:v1.0 --sbom sbom.json --keyless

# Attach with key
sc sbom attach --image myapp:v1.0 --sbom sbom.json --key cosign.key
```

### sc sbom verify

Verify SBOM attestation.

**Usage:**
```bash
sc sbom verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--output`: Output file for verified SBOM

**Examples:**
```bash
# Verify and display
sc sbom verify --image myapp:v1.0

# Verify and save
sc sbom verify --image myapp:v1.0 --output verified-sbom.json
```

## sc provenance

Provenance attestation operations.

### sc provenance attach

Generate and attach provenance attestation.

**Usage:**
```bash
sc provenance attach --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--keyless`: Use keyless signing (default: true)
- `--key`: Path to private key

**Examples:**
```bash
# Attach provenance (auto-detects git metadata)
sc provenance attach --image myapp:v1.0 --keyless

# Attach with key
sc provenance attach --image myapp:v1.0 --key cosign.key
```

### sc provenance verify

Verify provenance attestation.

**Usage:**
```bash
sc provenance verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--output`: Output file for verified provenance

**Examples:**
```bash
# Verify provenance
sc provenance verify --image myapp:v1.0

# Verify and save
sc provenance verify --image myapp:v1.0 --output provenance.json
```

## sc release

Unified release workflow with integrated security.

### sc release create

Create release with build, security, and deployment.

**Usage:**
```bash
sc release create -s STACK -e ENVIRONMENT [flags]
```

**Flags:**
- `-s, --stack` (required): Stack name
- `-e, --environment` (required): Environment name
- `--yes`: Auto-approve deployment without prompts
- `--preview`: Preview changes without deploying (dry-run)

**Examples:**
```bash
# Create production release
sc release create -s mystack -e production

# Preview staging release
sc release create -s mystack -e staging --preview

# Auto-approve deployment
sc release create -s mystack -e production --yes
```

**Workflow:**
1. Load stack configuration
2. Build and push container images
3. Execute security operations (scan → sign → SBOM → provenance)
4. Deploy infrastructure

**Security Integration:**
- Security operations run automatically if configured in stack
- Scanning runs FIRST (fail-fast pattern)
- Signing, SBOM, and provenance run in parallel after scanning
- Deployment waits for ALL security operations to complete
- Graceful skipping when security disabled

## Exit Codes

All commands use standard exit codes:

- **0**: Success
- **1**: Command failed or policy violation
- **2**: Tool not installed or missing dependency
- **3**: Configuration error
- **130**: Interrupted by user (Ctrl+C)

## Environment Variables

### Signing
- `SIGSTORE_ID_TOKEN`: OIDC token for keyless signing
- `COSIGN_EXPERIMENTAL`: Enable experimental cosign features
- `COSIGN_PASSWORD`: Password for encrypted private keys

### CI/CD Detection
- `CI`: Set to `true` in CI environments
- `GITHUB_ACTIONS`: GitHub Actions environment
- `GITLAB_CI`: GitLab CI environment
- `CIRCLECI`: CircleCI environment

## Global Flags

Available for all commands:

- `-v, --verbose`: Verbose output
- `--silent`: Silent mode (errors only)
- `-h, --help`: Show help
