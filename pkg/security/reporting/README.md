# Security Reporting Integration

This package provides integration with external security reporting systems for container image scan results.

## Features

### 1. SARIF Report Generation

Generate [SARIF (Static Analysis Results Interchange Format)](https://sarifweb.azurewebsites.net/) reports from vulnerability scan results. SARIF is a standard format for static analysis results that's supported by many platforms including GitHub Security.

**Usage:**
```go
import "github.com/simple-container-com/api/pkg/security/reporting"

// Generate SARIF from scan result
sarif, err := reporting.NewSARIFFromScanResult(scanResult, "myimage:latest")
if err != nil {
    return err
}

// Save to file
if err := sarif.SaveToFile("results.sarif"); err != nil {
    return err
}
```

### 2. DefectDojo Integration

Upload vulnerability scan results to [DefectDojo](https://www.defectdojo.org/), an open-source application vulnerability correlation and security orchestration tool.

**Configuration:**
```yaml
security:
  reporting:
    defectdojo:
      enabled: true
      url: "https://defectdojo.example.com"
      apiKey: "${secret:defectdojo-api-key}"
      engagementId: 123  # Use existing engagement
      # OR create new engagement:
      engagementName: "Container Scan"
      productName: "MyProduct"
      autoCreate: true
      tags: ["ci", "production"]
      environment: "production"
```

**Programmatic Usage:**
```go
client := reporting.NewDefectDojoClient(url, apiKey)
config := &reporting.DefectDojoUploaderConfig{
    EngagementID: 123,
}
result, err := client.UploadScanResult(ctx, scanResult, "myimage:latest", config)
```

### 3. GitHub Security Tab Integration

Upload vulnerability scan results to GitHub Security tab using SARIF format. Results appear in the repository's Security > Code scanning alerts section.

**Configuration:**
```yaml
security:
  reporting:
    github:
      enabled: true
      repository: "${github.repository}"  # e.g., "owner/repo"
      token: "${secret:github-token}"
      commitSha: "${git.sha}"
      ref: "${git.ref}"
      workspace: "${github.workspace}"  # For GitHub Actions
```

**Programmatic Usage:**
```go
config := &reporting.GitHubUploaderConfig{
    Repository: "owner/repo",
    Token:      "ghp_xxx",
    CommitSHA:  "abc123",
    Ref:        "refs/heads/main",
    Workspace:  "/github/workspace",
}
err := reporting.UploadToGitHub(ctx, scanResult, "myimage:latest", config)
```

### 4. Workflow Summary

Track and display a comprehensive summary of all security operations with timing information.

**Usage:**
```go
// Create summary
summary := reporting.NewWorkflowSummary("myimage:latest")

// Record operations
summary.RecordSBOM(sbomResult, nil, duration, "sbom.json")
summary.RecordScan(scan.ScanToolGrype, scanResult, nil, duration, "v1.2.3")
summary.RecordSigning(signResult, nil, duration)
summary.RecordUpload("github", nil, url, duration)

// Display summary
summary.Display()
```

**Output Example:**
```
╔══════════════════════════════════════════════════════════════════╗
║                    SECURITY WORKFLOW SUMMARY                      ║
╠══════════════════════════════════════════════════════════════════╣
║ Image: myimage:latest                                              ║
║ Duration: 2m34s                                                    ║
╠══════════════════════════════════════════════════════════════════╣
║ 📋 SBOM Generation                                                 ║
║   Status: ✅ SUCCESS                                              ║
║   Packages: 142                                                    ║
╠══════════════════════════════════════════════════════════════════╣
║ 🔍 Vulnerability Scanning                                            ║
║   Grype: 3 critical, 7 high, 12 medium vulnerabilities            ║
║   Trivy: 3 critical, 6 high, 11 medium vulnerabilities            ║
║   Merged: 3 critical, 7 high, 12 medium (deduplicated)           ║
╠══════════════════════════════════════════════════════════════════╣
║ 🔐 Image Signing                                                    ║
║   Status: ✅ SUCCESS                                              ║
║   Method: Keyless (OIDC)                                          ║
╠══════════════════════════════════════════════════════════════════╣
║ 📤 Report Uploads                                                   ║
║   DefectDojo: ✅ uploaded                                         ║
║   GitHub: ✅ uploaded                                             ║
╚══════════════════════════════════════════════════════════════════╝
```

## CLI Usage

### Scan with Reporting

```bash
# Scan and upload to GitHub Security
sc image scan \
  --image myimage:latest \
  --tool all \
  --upload-github \
  --github-repo owner/repo \
  --github-token $GITHUB_TOKEN \
  --github-ref refs/heads/main

# Scan and upload to DefectDojo
sc image scan \
  --image myimage:latest \
  --tool all \
  --upload-defectdojo \
  --defectdojo-url https://defectdojo.example.com \
  --defectdojo-api-key $DEFECTDOJO_API_KEY

# Generate SARIF file only
sc image scan \
  --image myimage:latest \
  --sarif-output results.sarif
```

## GitHub Actions Integration

Example GitHub Actions workflow:

```yaml
name: Container Security Scan

on:
  push:
    branches: [main]

permissions:
  contents: read
  security-events: write  # Required for uploading to GitHub Security

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build image
        run: docker build -t myimage:latest .

      - name: Scan and upload to GitHub Security
        run: |
          sc image scan \
            --image myimage:latest \
            --tool all \
            --upload-github \
            --github-repo ${GITHUB_REPOSITORY} \
            --github-token ${{ secrets.GITHUB_TOKEN }} \
            --github-ref ${GITHUB_REF} \
            --github-workspace ${GITHUB_WORKSPACE}
```

## DefectDojo Setup

### Prerequisites

1. DefectDojo instance running (v2.0+)
2. API key with appropriate permissions
3. Product and Engagement created (or enable auto-create)

### API Key Setup

```bash
# Get API key from DefectDojo
# Settings > API Keys > Create API Key
export DEFECTDOJO_API_KEY="your-api-key"
export DEFECTDOJO_URL="https://defectdojo.example.com"
```

### Auto-Create Mode

When `autoCreate: true`, the system will automatically create:
- Product if it doesn't exist
- Engagement if it doesn't exist

```yaml
security:
  reporting:
    defectdojo:
      enabled: true
      url: "${DEFECTDOJO_URL}"
      apiKey: "${DEFECTDOJO_API_KEY}"
      productName: "MyProduct"
      engagementName: "Container Scan"
      autoCreate: true
```

## Implementation Details

### SARIF Format

The SARIF generator creates compliant SARIF 2.1.0 files with:
- Tool information (Grype, Trivy, or Simple Container Security)
- Vulnerability results with proper severity mapping
- Package locations in purl format
- Fix information when available
- CVSS scores and reference URLs

### DefectDojo API

The DefectDojo client uses the REST API v2:
- Product management: `/api/v2/products/`
- Engagement management: `/api/v2/engagements/`
- Scan import: `/api/v2/import-scan/`

Supports:
- SARIF format uploads
- Auto-creation of products and engagements
- Tag-based organization
- Environment labeling

### GitHub API

The GitHub uploader supports two methods:

1. **Workspace Mode** (Recommended for GitHub Actions)
   - Writes SARIF to `$GITHUB_WORKSPACE/github-security-results/`
   - GitHub Actions automatically uploads to Security tab
   - No additional API calls needed

2. **Direct API Upload**
   - Uses GitHub REST API directly
   - Works outside of GitHub Actions
   - Requires `security_events` repository permission

## Error Handling

All reporting operations follow the fail-open philosophy:
- Upload failures are logged as warnings
- Don't block the main security workflow
- Errors are tracked in the workflow summary

```go
if e.Summary != nil {
    e.Summary.RecordUpload("defectdojo", err, url, duration)
}
```

## Performance Considerations

- SARIF generation: < 100ms for typical images
- DefectDojo upload: 1-3 seconds depending on network
- GitHub upload: < 500ms (workspace mode)
- All uploads run in parallel if configured

## Security Best Practices

1. **API Keys**: Use environment variables or secret management
   ```yaml
   apiKey: "${secret:defectdojo-api-key}"
   ```

2. **GitHub Tokens**: Use fine-grained permissions
   - Only `security_events: write` permission needed
   - Use repository-scoped tokens when possible

3. **HTTPS**: Always use HTTPS for API endpoints

4. **Access Control**: Limit API key permissions in external systems

## Troubleshooting

### DefectDojo Upload Fails

```
Error: getting engagement: engagement ID 123 not found
```

**Solution**: Enable auto-create or verify engagement exists:
```yaml
autoCreate: true
engagementName: "My Engagement"
productName: "My Product"
```

### GitHub Upload Not Showing

```
SARIF uploaded successfully but not visible in Security tab
```

**Solutions**:
- Verify `security-events: write` permission
- Check repository Settings > Security > Code scanning
- Allow 5-10 minutes for processing
- Check Actions tab for upload errors

### SARIF Validation Errors

```
Error: generating SARIF: invalid vulnerability data
```

**Solution**: Ensure scan results are complete and valid:
```bash
sc image scan --image myimage:latest --output scan.json
# Validate scan.json structure
```

## Contributing

When adding new reporting integrations:

1. Create client in separate file (e.g., `github.go`, `defectdojo.go`)
2. Implement `UploadXXX` function
3. Add configuration to `pkg/security/config.go`
4. Update executor's `UploadReports` method
5. Add CLI flags in `pkg/cmd/cmd_image/scan.go`
6. Update this README

## References

- [SARIF Specification](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
- [DefectDojo API Documentation](https://defectdojo.github.io/django-DefectDojo/rest/api/)
- [GitHub Code Scanning API](https://docs.github.com/en/rest/code-scanning)
- [SLSA Verification Summary](https://slsa.dev/spec/v1.0/verification_summary)
