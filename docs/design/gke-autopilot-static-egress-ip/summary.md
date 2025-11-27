# GKE Autopilot Static Egress IP - Design Summary

## Overview

This design provides a **simple** way to enable static egress IP addresses for GKE Autopilot clusters, staying true to Simple Container's philosophy of minimal configuration with maximum automation.

## Key Design Principles

### 1. **Simplicity First**
- Only 2 configuration fields: `enabled` and optional `existing`
- No complex router, NAT, or networking configuration exposed
- Simple Container handles all the complexity automatically

### 2. **Smart Defaults**
- Automatic static IP creation if none specified
- Production-ready Cloud Router configuration
- Optimized Cloud NAT settings for GKE Autopilot
- Consistent resource naming based on cluster name

### 3. **Zero Breaking Changes**
- Completely optional feature (disabled by default)
- No changes to existing cluster configurations
- No impact on client.yaml or application deployments

## Configuration Structure

### Simple Configuration
```yaml
# server.yaml
resources:
  my-cluster:
    type: gcp-gke-autopilot-cluster
    config:
      projectId: "my-project"
      location: "us-central1"
      gkeMinVersion: "1.28"
      
      # NEW: Simple external egress IP configuration
      externalEgressIp:
        enabled: true  # Required: enable/disable feature
        existing: "projects/my-project/regions/us-central1/addresses/shared-ip"  # Optional: use existing IP
```

### Go Struct
```go
type ExternalEgressIpConfig struct {
    Enabled  bool   `json:"enabled" yaml:"enabled"`                    // Required
    Existing string `json:"existing,omitempty" yaml:"existing,omitempty"` // Optional
}
```

## What Simple Container Creates Automatically

When `externalEgressIp.enabled: true`:

1. **Static IP Address** (if not using existing)
   - Name: `{cluster-name}-egress-ip`
   - Type: External, Regional
   - Region: Extracted from cluster location

2. **Cloud Router**
   - Name: `{cluster-name}-router`
   - ASN: 64512 (private ASN)
   - Network: default VPC

3. **Cloud NAT Gateway**
   - Name: `{cluster-name}-nat`
   - Port allocation: 64-65536 per VM
   - Logging: Errors only
   - Endpoint independent mapping: Enabled

## User Experience

### Minimal Configuration
```yaml
externalEgressIp:
  enabled: true
```
**Result**: Static IP created automatically, all egress traffic uses this IP

### Shared IP Configuration
```yaml
externalEgressIp:
  enabled: true
  existing: "projects/my-project/regions/us-central1/addresses/shared-ip"
```
**Result**: Uses existing static IP, creates separate router/NAT for this cluster

### Disabled (Default)
```yaml
# No externalEgressIp section
```
**Result**: Standard GKE Autopilot behavior with dynamic egress IPs

## Implementation Benefits

### For Users
- **One-line enablement**: Just set `enabled: true`
- **Predictable egress IP**: All cluster traffic uses known static IP
- **No networking expertise required**: Simple Container handles complexity
- **Cost effective**: Only pay for what you use

### For Simple Container
- **Consistent with philosophy**: Minimal config, maximum automation
- **Maintainable**: Less configuration surface area to support
- **Extensible**: Can add advanced options later if needed
- **Testable**: Simple validation and fewer edge cases

## Migration Strategy

### New Clusters
```yaml
# Add to server.yaml
externalEgressIp:
  enabled: true
# Deploy normally
```

### Existing Clusters
```yaml
# Add to existing server.yaml
externalEgressIp:
  enabled: true
# Run: sc deploy
# Zero downtime - traffic automatically routes through new NAT
```

## Cost Implications

### Additional Costs (Approximate)
- **Static IP**: ~$1.46/month per reserved IP
- **Cloud NAT**: ~$32/month per gateway + $0.045/GB processed
- **Total**: ~$35/month + data processing costs

### Cost Optimization
- Share static IPs across multiple clusters using `existing`
- Monitor usage through GCP billing console
- Simple Container uses optimized settings to minimize costs

## Future Considerations

### Possible Future Enhancements (Not in Scope)
- Multiple static IPs for high availability
- Custom port allocation ranges
- Advanced logging configuration
- Integration with monitoring/alerting

### Current Limitations
- Single static IP per cluster (unless using shared)
- Regional scope (not global)
- Standard Cloud NAT pricing applies

## Success Criteria

### Must Have
- ✅ Simple 2-field configuration
- ✅ Automatic resource creation with smart defaults
- ✅ Zero breaking changes to existing functionality
- ✅ Clear documentation with examples

### Should Have
- ✅ Support for existing static IP reuse
- ✅ Consistent resource naming
- ✅ Production-ready default settings
- ✅ Proper validation and error messages

### Could Have (Future)
- Advanced configuration options
- Multi-IP support
- Custom monitoring integration
- Cost optimization recommendations

## Conclusion

This design provides a **Simple Container way** to enable static egress IPs:
- **Minimal configuration**: Just `enabled: true`
- **Maximum automation**: Simple Container handles all complexity
- **Production ready**: Optimized defaults for real-world usage
- **Cost effective**: Only essential resources created

The design stays true to Simple Container's core philosophy while solving a real user need for predictable egress IP addresses.
