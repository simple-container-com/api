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
- `--fail-on`: Optional quality gate severity (critical, high, medium, low)
- `--output`: Output file for JSON results
- `--sarif-output`: Output file for SARIF results
- `--warn-on`: Warn on severity without failing (default: high)
- `--cache-dir`: Cache directory for scan results
- `--required`: Fail if a configured scanner cannot run (default: false)
- `--comment-output`: Write a markdown PR comment summary to a local file
- `--upload-defectdojo`: Upload merged results to DefectDojo
- `--defectdojo-*`: DefectDojo connection, routing, and metadata flags
  Existing engagement mode requires `--defectdojo-engagement-id`
  Auto-create mode requires `--defectdojo-auto-create`, `--defectdojo-engagement-name`, and either `--defectdojo-product-id` or `--defectdojo-product-name`
  `--defectdojo-environment` is optional and must match an environment that already exists in DefectDojo if you set it

**Examples:**
```bash
# Scan with grype, warn on high and above (default behavior)
sc image scan --image myapp:v1.0

# Scan with trivy
sc image scan --image myapp:v1.0 --tool trivy

# Scan with both tools, save JSON and SARIF
sc image scan --image myapp:v1.0 --tool all --output results.json --sarif-output results.sarif

# Enable a quality gate explicitly
sc image scan --image myapp:v1.0 --tool all --fail-on critical

# Upload to DefectDojo and generate a PR comment artifact
sc image scan \
  --image myapp:v1.0 \
  --tool all \
  --output results.json \
  --sarif-output results.sarif \
  --comment-output comment.md \
  --upload-defectdojo \
  --defectdojo-url https://defectdojo.example.com \
  --defectdojo-api-key $DEFECTDOJO_API_KEY

# Upload to an existing DefectDojo engagement using env-provided credentials
sc image scan \
  --image myapp:v1.0 \
  --tool all \
  --output results.json \
  --upload-defectdojo \
  --defectdojo-engagement-id 123
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
- `--keyless`: Use keyless signing with OIDC
- `--key`: Path to private key (for key-based signing)

**Environment Variables:**
- `SIGSTORE_ID_TOKEN`: OIDC token for keyless signing

**Examples:**
```bash
# Keyless signing in CI
sc image sign --image myapp@sha256:... --keyless

# Key-based signing
sc image sign --image myapp@sha256:... --key cosign.key
```

### sc image verify

Verify container image signatures.

**Usage:**
```bash
sc image verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image to verify
- `--public-key`: Path to public key (for key-based verification)
- `--oidc-issuer`: OIDC issuer for keyless verification
- `--identity-regexp`: Identity regexp for keyless verification

**Examples:**
```bash
# Verify keyless signature
sc image verify --image myapp@sha256:... \
  --oidc-issuer https://token.actions.githubusercontent.com \
  --identity-regexp '^https://github.com/myorg/myrepo/.github/workflows/.*$'

# Verify with public key
sc image verify --image myapp@sha256:... --public-key cosign.pub
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
sc sbom generate --image myapp@sha256:... --format cyclonedx-json --output sbom.json

# Generate SPDX JSON
sc sbom generate --image myapp@sha256:... --format spdx-json --output sbom.json
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
- `--keyless`: Use keyless signing with OIDC. If `--key` is omitted, keyless mode is used.
- `--key`: Path to private key
- `--password`: Password for encrypted private keys

**Examples:**
```bash
# Attach with keyless signing
sc sbom attach --image myapp@sha256:... --sbom sbom.json --keyless

# Attach with key
sc sbom attach --image myapp@sha256:... --sbom sbom.json --key cosign.key --password "$COSIGN_PASSWORD"
```

### sc sbom verify

Verify SBOM attestation.

**Usage:**
```bash
sc sbom verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--output` (required): Output file for verified SBOM
- `--keyless`: Use keyless verification
- `--key`: Path to public key for key-based verification
- `--cert-identity`: Certificate identity for keyless verification
- `--cert-issuer`: Certificate issuer for keyless verification

**Examples:**
```bash
# Verify keyless SBOM attestation
sc sbom verify --image myapp@sha256:... \
  --keyless \
  --cert-identity '^https://github.com/myorg/myrepo/.github/workflows/.*$' \
  --cert-issuer https://token.actions.githubusercontent.com \
  --output verified-sbom.json

# Verify with a public key
sc sbom verify --image myapp@sha256:... --key cosign.pub --output verified-sbom.json
```

## sc provenance

Provenance attestation operations.

### sc provenance generate

Generate provenance without attaching it to the registry.

**Usage:**
```bash
sc provenance generate --image IMAGE [flags]
```

**Examples:**
```bash
# Generate provenance locally
sc provenance generate --image myapp@sha256:... --output provenance.json
```

### sc provenance attach

Generate and attach provenance attestation.

**Usage:**
```bash
sc provenance attach --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--format`: Provenance format (`slsa-v1.0`)
- `--output`: Save the generated predicate locally before attaching
- `--builder-id`: Override builder ID embedded in the predicate
- `--source-root`: Repository root for git metadata detection
- `--context`: Build context path to include in the predicate
- `--dockerfile`: Dockerfile path to include as a build material
- `--include-git`: Include git metadata when available (default: true)
- `--include-dockerfile`: Include Dockerfile metadata when a path is supplied (default: true)
- `--include-env`: Include selected CI environment metadata
- `--include-materials`: Include resolved build materials (default: true)
- `--keyless`: Use keyless signing when no `--key` is supplied
- `--key`: Path to private key

**Examples:**
```bash
# Attach provenance (auto-detects git metadata)
sc provenance attach --image myapp@sha256:... --keyless

# Attach with key
sc provenance attach --image myapp@sha256:... --key cosign.key

# Save the generated predicate locally as well
sc provenance attach --image myapp@sha256:... --output provenance.json --key cosign.key
```

### sc provenance verify

Verify provenance attestation.

**Usage:**
```bash
sc provenance verify --image IMAGE [flags]
```

**Flags:**
- `--image` (required): Container image
- `--format`: Expected provenance format
- `--output`: Output file for verified provenance
- `--key`: Path to public key for key-based verification
- `--keyless`: Use keyless verification
- `--cert-identity`: Certificate identity regexp for keyless verification
- `--cert-issuer`: Certificate OIDC issuer for keyless verification
- `--expected-digest`: Expected image digest
- `--expected-builder-id`: Expected builder ID in the predicate
- `--expected-source-uri`: Expected source repository URI in provenance materials
- `--expected-commit`: Expected commit in provenance materials

**Examples:**
```bash
# Verify provenance on an immutable digest with policy checks
sc provenance verify \
  --image myapp@sha256:... \
  --keyless \
  --cert-identity '^https://github.com/myorg/myrepo/.github/workflows/.*$' \
  --cert-issuer https://token.actions.githubusercontent.com \
  --expected-builder-id https://github.com/myorg/myapp/actions/runs/123 \
  --expected-commit $GITHUB_SHA \
  --output provenance.json

# Verify with a public key
sc provenance verify --image myapp@sha256:... --key cosign.pub --expected-digest sha256:... --output provenance.json
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
- Scanning runs first so policy/reporting artifacts are produced before promotion
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
