# Security Reporting Integration

This package provides integration with external security reporting systems for container image scan results.

## Features

### DefectDojo Integration

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

### Workflow Summary

Track and display a comprehensive summary of all security operations with timing information.

**Usage:**
```go
// Create summary
summary := reporting.NewWorkflowSummary("myimage:latest")

// Record operations
summary.RecordSBOM(sbomResult, nil, duration, "sbom.json")
summary.RecordScan(scan.ScanToolGrype, scanResult, nil, duration, "v1.2.3")
summary.RecordSigning(signResult, nil, duration)
summary.RecordUpload("defectdojo", nil, url, duration)

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
╚══════════════════════════════════════════════════════════════════╝
```

## CLI Usage

### Scan with Reporting

```bash
# Scan and upload to DefectDojo
sc image scan \
  --image myimage:latest \
  --tool all \
  --upload-defectdojo \
  --defectdojo-url https://defectdojo.example.com \
  --defectdojo-api-key $DEFECTDOJO_API_KEY
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

### DefectDojo API

The DefectDojo client uses the REST API v2:
- Product management: `/api/v2/products/`
- Engagement management: `/api/v2/engagements/`
- Scan import: `/api/v2/import-scan/`

Supports:
- Auto-creation of products and engagements
- Tag-based organization
- Environment labeling

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

- DefectDojo upload: 1-3 seconds depending on network
- All uploads run in parallel if configured

## Security Best Practices

1. **API Keys**: Use environment variables or secret management
   ```yaml
   apiKey: "${secret:defectdojo-api-key}"
   ```

2. **HTTPS**: Always use HTTPS for API endpoints

3. **Access Control**: Limit API key permissions in external systems

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

## Contributing

When adding new reporting integrations:

1. Create client in separate file (e.g., `defectdojo.go`)
2. Implement `UploadXXX` function
3. Add configuration to `pkg/security/config.go`
4. Update executor's `UploadReports` method
5. Add CLI flags in `pkg/cmd/cmd_image/scan.go`
6. Update this README

## References

- [DefectDojo API Documentation](https://defectdojo.github.io/django-DefectDojo/rest/api/)
- [SLSA Verification Summary](https://slsa.dev/spec/v1.0/verification_summary)
