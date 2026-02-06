# Container Security Troubleshooting

Common issues and solutions for Simple Container security features.

## Vulnerability Scanning Issues

### Error: "grype not found"

**Problem:** Grype scanner not installed.

**Solution:**
```bash
# macOS
brew install grype

# Linux
curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin
```

### Error: "Failed to pull image for scanning"

**Problem:** Scanner cannot access image (authentication required).

**Solution:**
```bash
# Docker Hub
docker login

# AWS ECR
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin ACCOUNT.dkr.ecr.us-east-1.amazonaws.com

# GCP GCR
gcloud auth configure-docker
```

### Policy Violation: "Found X critical vulnerabilities"

**Problem:** Image has vulnerabilities exceeding fail-on threshold.

**Solution:**
1. Review scan results: `sc image scan --image IMAGE --output results.json`
2. Update base image to newer version
3. Apply security patches
4. Adjust fail-on threshold (not recommended for production):
   ```yaml
   scan:
     failOn: high  # Changed from critical
   ```

### Scan Takes Too Long

**Problem:** Scanning large images (>1GB) is slow.

**Solution:**
1. Enable caching:
   ```yaml
   scan:
     cache:
       enabled: true
       ttl: 6h
   ```
2. Use single scanner (grype OR trivy, not both)
3. Scan in parallel with builds (not sequential)

## Image Signing Issues

### Error: "SIGSTORE_ID_TOKEN not set"

**Problem:** Keyless signing requires OIDC token.

**Solution:**
```bash
# GitHub Actions (automatic)
# Ensure id-token: write permission

# Google Cloud
export SIGSTORE_ID_TOKEN=$(gcloud auth print-identity-token)

# AWS (with OIDC provider configured)
export SIGSTORE_ID_TOKEN=$(aws sts get-caller-identity --query 'Account' --output text)
```

### Error: "failed to sign: key not found"

**Problem:** Private key path incorrect for key-based signing.

**Solution:**
```bash
# Verify key exists
ls -la /path/to/cosign.key

# Generate new key pair
cosign generate-key-pair

# Update config
signing:
  keyless: false
  privateKey: /correct/path/to/cosign.key
```

### Error: "signature verification failed"

**Problem:** Signature invalid or wrong public key.

**Solution:**
1. Verify with same key used for signing:
   ```bash
   sc image verify --image IMAGE --key cosign.pub
   ```
2. Check signature exists:
   ```bash
   cosign tree IMAGE
   ```
3. Re-sign image if signature corrupted

### Warning: "Rekor entry not found"

**Problem:** Keyless signature not in transparency log.

**Solution:**
- This is expected for key-based signing
- For keyless signing, check SIGSTORE_ID_TOKEN was set correctly
- Verify Rekor service is accessible

## SBOM Generation Issues

### Error: "syft not found"

**Problem:** Syft tool not installed.

**Solution:**
```bash
# macOS
brew install syft

# Linux
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
```

### Error: "SBOM generation timeout"

**Problem:** Large image taking too long to analyze.

**Solution:**
1. Increase timeout (not currently configurable, default 5 minutes)
2. Use smaller base images
3. Enable caching:
   ```yaml
   sbom:
     cache:
       enabled: true
       ttl: 24h
   ```

### SBOM Shows 0 Packages

**Problem:** Syft couldn't detect packages.

**Solution:**
1. Verify image has package managers:
   ```bash
   docker run --rm IMAGE find /usr -name "package*.json" -o -name "go.mod" -o -name "pom.xml"
   ```
2. Check image format is supported (not scratch images)
3. Use format-specific flags (advanced)

### Error: "Failed to attach SBOM attestation"

**Problem:** Registry doesn't support OCI artifacts or authentication failed.

**Solution:**
1. Verify registry supports OCI artifacts (Docker Hub, ECR, GCR, ACR all support it)
2. Check authentication:
   ```bash
   docker login REGISTRY
   ```
3. Try local output first:
   ```yaml
   sbom:
     output:
       local: .sc/sbom/
       registry: false
   ```

## Provenance Issues

### Error: "git not found"

**Problem:** Git not installed or not in PATH.

**Solution:**
```bash
# macOS
brew install git

# Linux (Debian/Ubuntu)
apt-get install -y git

# Verify
git --version
```

### Error: "not a git repository"

**Problem:** Building outside git repository.

**Solution:**
1. Initialize git repo:
   ```bash
   git init
   git add .
   git commit -m "Initial commit"
   ```
2. Or disable git metadata:
   ```yaml
   provenance:
     includeGit: false
   ```

### Provenance Shows "unknown" Builder

**Problem:** CI environment not detected.

**Solution:**
- Provenance auto-detects: GitHub Actions, GitLab CI, CircleCI, Jenkins
- For other CI systems, builder.id defaults to "https://simple-container.com/local"
- This is not an error, just informational

## Release Workflow Issues

### Error: "stack not found"

**Problem:** Stack name incorrect or stack file doesn't exist.

**Solution:**
```bash
# List available stacks
sc stack list

# Verify stack file exists
ls stacks/STACKNAME.yaml

# Use correct stack name
sc release create -s correct-stack-name -e production
```

### Security Operations Skipped

**Problem:** Security config not recognized.

**Solution:**
1. Verify config in stack YAML:
   ```bash
   sc stack show -s STACKNAME -e ENVIRONMENT
   ```
2. Check security.enabled is true:
   ```yaml
   client:
     security:
       enabled: true
   ```
3. Verify individual features enabled:
   ```yaml
   scan:
     enabled: true
   signing:
     enabled: true
   ```

### Deployment Blocked by Scan

**Problem:** Vulnerabilities found, deployment blocked.

**Solution:**
1. View scan results to understand vulnerabilities
2. Fix vulnerabilities by updating packages
3. If urgent, temporarily adjust threshold:
   ```yaml
   scan:
     failOn: high  # Was: critical
     required: false  # Allow deployment to proceed with warnings
   ```

### Performance Issues

**Problem:** Deployment slow with security enabled.

**Solution:**
1. Enable all caching:
   ```yaml
   scan:
     cache:
       enabled: true
   sbom:
     cache:
       enabled: true
   ```
2. Use single scanner (not all):
   ```yaml
   scan:
     tools:
       - name: grype  # Remove trivy
   ```
3. Profile deployment to identify bottleneck:
   ```bash
   time sc release create -s STACK -e ENV --verbose
   ```

## Tool Installation Issues

### macOS: "command not found" after brew install

**Problem:** PATH not updated.

**Solution:**
```bash
# Add to ~/.zshrc or ~/.bashrc
export PATH="/usr/local/bin:$PATH"

# Reload shell
source ~/.zshrc
```

### Linux: Permission denied

**Problem:** Tools installed without execute permission.

**Solution:**
```bash
chmod +x /usr/local/bin/cosign
chmod +x /usr/local/bin/syft
chmod +x /usr/local/bin/grype
```

### Docker: "Cannot connect to Docker daemon"

**Problem:** Docker not running or permission issue.

**Solution:**
```bash
# Start Docker
sudo systemctl start docker

# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker
```

## Debugging Tips

### Enable Verbose Mode

```bash
sc release create -s STACK -e ENV --verbose
```

### Check Tool Versions

```bash
cosign version
syft version
grype version
trivy --version
```

### Test Individual Operations

```bash
# Test scan
sc image scan --image alpine:3.18 --tool grype

# Test sign
sc image sign --image alpine:3.18 --keyless

# Test SBOM
sc sbom generate --image alpine:3.18 --output /tmp/test-sbom.json

# Test provenance
sc provenance attach --image alpine:3.18 --keyless
```

### View Pulumi Logs

```bash
pulumi logs -s STACKNAME
```

### Check CI Environment Variables

```bash
# GitHub Actions
echo $GITHUB_ACTIONS
echo $GITHUB_ACTOR

# GitLab CI
echo $GITLAB_CI
echo $CI_PROJECT_PATH
```

## Getting Help

If issues persist:

1. Check documentation: https://docs.simple-container.com
2. Search issues: https://github.com/simple-container-com/api/issues
3. Report bug with:
   - Command that failed
   - Full error message
   - Tool versions (`cosign version`, `syft version`, etc.)
   - Stack configuration (sanitized)
   - CI environment (if applicable)
