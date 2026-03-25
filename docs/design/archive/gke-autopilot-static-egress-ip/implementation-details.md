# Implementation Details

## Configuration Structure

### Go Struct Definitions

```go
// pkg/clouds/gcloud/gke_autopilot.go

type GkeAutopilotResource struct {
    Credentials   `json:",inline" yaml:",inline"`
    GkeMinVersion string           `json:"gkeMinVersion" yaml:"gkeMinVersion"`
    Location      string           `json:"location" yaml:"location"`
    Zone          string           `json:"zone" yaml:"zone"`
    Timeouts      *Timeouts        `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
    Caddy         *k8s.CaddyConfig `json:"caddy,omitempty" yaml:"caddy,omitempty"`
    
    // NEW: External Egress IP Configuration (Simple!)
    ExternalEgressIp *ExternalEgressIpConfig `json:"externalEgressIp,omitempty" yaml:"externalEgressIp,omitempty"`
    
    // Resource adoption fields
    Adopt       bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"`
}

// ExternalEgressIpConfig provides simple configuration for static egress IP
type ExternalEgressIpConfig struct {
    Enabled  bool   `json:"enabled" yaml:"enabled"`
    Existing string `json:"existing,omitempty" yaml:"existing,omitempty"`
}
```

### Validation Logic

```go
// pkg/clouds/gcloud/gke_autopilot.go

func (c *ExternalEgressIpConfig) Validate() error {
    if !c.Enabled {
        return nil // No validation needed if disabled
    }
    
    // Validate existing IP format if specified
    if c.Existing != "" {
        if !strings.HasPrefix(c.Existing, "projects/") {
            return errors.New("'existing' must be a full GCP resource path like 'projects/{project}/regions/{region}/addresses/{name}'")
        }
        parts := strings.Split(c.Existing, "/")
        if len(parts) != 6 || parts[2] != "regions" || parts[4] != "addresses" {
            return errors.New("invalid 'existing' format, expected 'projects/{project}/regions/{region}/addresses/{name}'")
        }
    }
    
    return nil
}

// No SetDefaults method needed - Simple Container handles all defaults automatically!
```

## Pulumi Implementation

### Extended Output Structure

```go
// pkg/clouds/pulumi/gcp/gke_autopilot.go

type GkeAutopilotOut struct {
    Cluster      *container.Cluster
    Caddy        *kubernetes.SimpleContainer
    
    // NEW: Cloud NAT resources
    StaticIp     *compute.Address      `json:"staticIp,omitempty"`
    Router       *compute.Router       `json:"router,omitempty"`
    Nat          *compute.RouterNat    `json:"nat,omitempty"`
}
```

### Resource Creation Functions

```go
// pkg/clouds/pulumi/gcp/gke_autopilot.go

func createCloudNatResources(
    ctx *sdk.Context,
    gkeInput *gcloud.GkeAutopilotResource,
    clusterName string,
    region string,
    opts []sdk.ResourceOption,
    params pApi.ProvisionParams,
) (*GkeAutopilotOut, error) {
    
    out := &GkeAutopilotOut{}
    
    if gkeInput.CloudNat == nil || !gkeInput.CloudNat.Enabled {
        return out, nil
    }
    
    // Validate and set defaults
    if err := gkeInput.CloudNat.Validate(); err != nil {
        return nil, errors.Wrap(err, "invalid Cloud NAT configuration")
    }
    gkeInput.CloudNat.SetDefaults(clusterName, region)
    
    params.Log.Info(ctx.Context(), "🌐 Configuring Cloud NAT with static egress IP for cluster %q", clusterName)
    
    // Step 1: Create or reference static IP
    staticIp, err := createOrReferenceStaticIp(ctx, gkeInput.CloudNat, region, opts, params)
    if err != nil {
        return nil, errors.Wrap(err, "failed to create static IP")
    }
    out.StaticIp = staticIp
    
    // Step 2: Create Cloud Router
    router, err := createCloudRouter(ctx, gkeInput.CloudNat, region, opts, params)
    if err != nil {
        return nil, errors.Wrap(err, "failed to create Cloud Router")
    }
    out.Router = router
    
    // Step 3: Create Cloud NAT
    nat, err := createCloudNat(ctx, gkeInput.CloudNat, router, staticIp, region, opts, params)
    if err != nil {
        return nil, errors.Wrap(err, "failed to create Cloud NAT")
    }
    out.Nat = nat
    
    // Export static IP address for external parties to allowlist
    ctx.Export(fmt.Sprintf("%s-egress-ip-address", clusterName), staticIp.Address)
    ctx.Export(fmt.Sprintf("%s-egress-ip-name", clusterName), staticIp.Name)
    
    params.Log.Info(ctx.Context(), "✅ Cloud NAT configured successfully with static egress IP")
    
    return out, nil
}

func createOrReferenceStaticIp(
    ctx *sdk.Context,
    config *gcloud.ExternalEgressIpConfig,
    clusterName string,
    region string,
    opts []sdk.ResourceOption,
    params pApi.ProvisionParams,
) (*compute.Address, error) {
    
    if config.Existing != "" {
        // Use existing static IP
        params.Log.Info(ctx.Context(), "🔗 Using existing static IP: %s", config.Existing)
        
        // Parse the existing IP reference to extract name
        // Format: projects/{project}/regions/{region}/addresses/{name}
        parts := strings.Split(config.Existing, "/")
        if len(parts) != 6 {
            return nil, errors.Errorf("invalid existing static IP reference format: %s", config.Existing)
        }
        addressName := parts[5]
        
        return compute.GetAddress(ctx, addressName, sdk.ID(config.Existing), nil, opts...)
    } else {
        // Create new static IP automatically
        staticIpName := fmt.Sprintf("%s-egress-ip", clusterName)
        params.Log.Info(ctx.Context(), "📍 Creating static IP address: %s", staticIpName)
        
        return compute.NewAddress(ctx, staticIpName, &compute.AddressArgs{
            Name:        sdk.String(staticIpName),
            Region:      sdk.String(region),
            AddressType: sdk.String("EXTERNAL"),
            Description: sdk.String(fmt.Sprintf("Static egress IP for GKE cluster %s", clusterName)),
        }, opts...)
    }
}

func createCloudRouter(
    ctx *sdk.Context,
    config *gcloud.CloudNatConfig,
    region string,
    opts []sdk.ResourceOption,
    params pApi.ProvisionParams,
) (*compute.Router, error) {
    
    params.Log.Info(ctx.Context(), "🔀 Creating Cloud Router: %s", config.Router.Name)
    
    return compute.NewRouter(ctx, config.Router.Name, &compute.RouterArgs{
        Name:        sdk.String(config.Router.Name),
        Region:      sdk.String(region),
        Network:     sdk.String("default"), // Use default VPC network
        Description: sdk.String("Cloud Router for GKE cluster NAT"),
        Bgp: &compute.RouterBgpArgs{
            Asn: sdk.Int(*config.Router.Asn),
        },
    }, opts...)
}

func createCloudNat(
    ctx *sdk.Context,
    config *gcloud.CloudNatConfig,
    router *compute.Router,
    staticIp *compute.Address,
    region string,
    opts []sdk.ResourceOption,
    params pApi.ProvisionParams,
) (*compute.RouterNat, error) {
    
    params.Log.Info(ctx.Context(), "🌉 Creating Cloud NAT Gateway: %s", config.Nat.Name)
    
    // Configure NAT IP allocation
    natIps := sdk.StringArray{staticIp.SelfLink}
    
    // Configure logging if enabled
    var logConfig *compute.RouterNatLogConfigArgs
    if *config.Nat.EnableLogging {
        logConfig = &compute.RouterNatLogConfigArgs{
            Enable: sdk.Bool(true),
            Filter: sdk.String(*config.Nat.LogFilter),
        }
    }
    
    return compute.NewRouterNat(ctx, config.Nat.Name, &compute.RouterNatArgs{
        Name:   sdk.String(config.Nat.Name),
        Router: router.Name,
        Region: sdk.String(region),
        
        // NAT configuration - use our specific static IP (not random GCP-assigned IPs)
        NatIpAllocateOption:              sdk.String("MANUAL_ONLY"), // Use only the IPs we specify in NatIps
        NatIps:                          natIps,                     // Our static IP address
        SourceSubnetworkIpRangesToNat:   sdk.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
        
        // Port allocation
        MinPortsPerVm: sdk.Int(*config.Nat.MinPortsPerVm),
        MaxPortsPerVm: sdk.Int(*config.Nat.MaxPortsPerVm),
        
        // Logging configuration
        LogConfig: logConfig,
        
        // Enable endpoint independent mapping for better performance
        EnableEndpointIndependentMapping: sdk.Bool(true),
    }, opts...)
}
```

### Integration with Main Function

```go
// pkg/clouds/pulumi/gcp/gke_autopilot.go - Modified GkeAutopilot function

func GkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    // ... existing validation code ...
    
    // Extract region from location (location can be regional or zonal)
    region := extractRegionFromLocation(gkeInput.Location)
    
    // NEW: Create Cloud NAT resources first (before cluster)
    natOut, err := createCloudNatResources(ctx, gkeInput, clusterName, region, opts, params)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to create Cloud NAT resources for cluster %q", clusterName)
    }
    
    // Create GKE cluster (existing code)
    cluster, err := container.NewCluster(ctx, clusterName, &container.ClusterArgs{
        EnableAutopilot:  sdk.Bool(true),
        Location:         sdk.String(location),
        Name:             sdk.String(clusterName),
        MinMasterVersion: sdk.String(gkeInput.GkeMinVersion),
        ReleaseChannel: &container.ClusterReleaseChannelArgs{
            Channel: sdk.String("STABLE"),
        },
        IpAllocationPolicy: &container.ClusterIpAllocationPolicyArgs{},
        // NOTE: NAT is configured at VPC level, no cluster-specific changes needed
    }, append(opts, sdk.IgnoreChanges([]string{"verticalPodAutoscaling"}), sdk.Timeouts(&timeouts))...)
    
    if err != nil {
        return nil, errors.Wrapf(err, "failed to create cluster %q in %q", clusterName, input.StackParams.Environment)
    }
    
    // Merge NAT resources into output
    out := GkeAutopilotOut{
        Cluster:  cluster,
        StaticIp: natOut.StaticIp,
        Router:   natOut.Router,
        Nat:      natOut.Nat,
    }
    
    // ... rest of existing code (kubeconfig, Caddy, etc.) ...
    
    return &api.ResourceOutput{Ref: out}, nil
}

func extractRegionFromLocation(location string) string {
    // Handle both regional (us-central1) and zonal (us-central1-a) locations
    parts := strings.Split(location, "-")
    if len(parts) >= 2 {
        return strings.Join(parts[:2], "-")
    }
    return location
}
```

## Pulumi Outputs

The implementation exports the static IP information for external use:

```go
// Exported outputs that users can access
ctx.Export(fmt.Sprintf("%s-egress-ip-address", clusterName), staticIp.Address) // The actual IP address (e.g., "203.0.113.42")
ctx.Export(fmt.Sprintf("%s-egress-ip-name", clusterName), staticIp.Name)       // The resource name (e.g., "my-cluster-egress-ip")
```

### Accessing the Static IP Address

After deployment, users can get the static IP address to share with external parties:

```bash
# Get the static IP address
pulumi stack output my-cluster-egress-ip-address
# Output: 203.0.113.42

# Get the resource name
pulumi stack output my-cluster-egress-ip-name  
# Output: my-cluster-egress-ip
```

This IP address can then be shared with:
- Third-party API providers for allowlisting
- Database administrators for firewall rules
- Security teams for audit trails
- External services that need to identify your cluster's traffic

## Required Imports

```go
// pkg/clouds/pulumi/gcp/gke_autopilot.go - Additional imports needed

import (
    // ... existing imports ...
    "strings"
    "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
)
```

## Error Handling

### Validation Errors

```go
// Comprehensive error messages for common configuration mistakes

var (
    ErrCloudNatBothCreateAndExisting = errors.New("cannot specify both 'create' and 'existing' for staticEgressIp")
    ErrCloudNatMissingStaticIp      = errors.New("must specify either 'create: true' or 'existing' for staticEgressIp")
    ErrCloudNatMissingName          = errors.New("'name' is required when 'create: true' for staticEgressIp")
    ErrCloudNatInvalidLogFilter     = errors.New("invalid logFilter, must be one of: ALL, ERRORS_ONLY, TRANSLATIONS_ONLY")
    ErrCloudNatInvalidPortRange     = errors.New("invalid port range configuration")
)
```

### Resource Creation Errors

```go
// Handle specific GCP API errors with helpful messages

func handleGcpError(err error, resource string) error {
    if err == nil {
        return nil
    }
    
    errMsg := err.Error()
    switch {
    case strings.Contains(errMsg, "already exists"):
        return errors.Wrapf(err, "%s already exists - consider using 'existing' reference instead of 'create'", resource)
    case strings.Contains(errMsg, "not found"):
        return errors.Wrapf(err, "%s not found - check the resource name and project", resource)
    case strings.Contains(errMsg, "quota exceeded"):
        return errors.Wrapf(err, "GCP quota exceeded for %s - check your project quotas", resource)
    case strings.Contains(errMsg, "permission denied"):
        return errors.Wrapf(err, "insufficient permissions to create %s - check IAM roles", resource)
    default:
        return errors.Wrapf(err, "failed to create %s", resource)
    }
}
```

## Testing Strategy

### Unit Tests

```go
// pkg/clouds/gcloud/gke_autopilot_test.go

func TestCloudNatConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        config  *CloudNatConfig
        wantErr bool
    }{
        {
            name: "valid create configuration",
            config: &CloudNatConfig{
                Enabled: true,
                StaticEgressIp: StaticEgressIpConfig{
                    Create: true,
                    Name:   "test-ip",
                },
            },
            wantErr: false,
        },
        {
            name: "valid existing configuration",
            config: &CloudNatConfig{
                Enabled: true,
                StaticEgressIp: StaticEgressIpConfig{
                    Existing: "projects/test/regions/us-central1/addresses/existing-ip",
                },
            },
            wantErr: false,
        },
        {
            name: "invalid both create and existing",
            config: &CloudNatConfig{
                Enabled: true,
                StaticEgressIp: StaticEgressIpConfig{
                    Create:   true,
                    Name:     "test-ip",
                    Existing: "projects/test/regions/us-central1/addresses/existing-ip",
                },
            },
            wantErr: true,
        },
        // ... more test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("CloudNatConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

```go
// pkg/clouds/pulumi/gcp/gke_autopilot_test.go

func TestGkeAutopilot_WithCloudNat(t *testing.T) {
    // Test with Pulumi mocks to verify resource creation order and dependencies
    // This would be similar to existing SimpleContainer tests
}
```

## Migration Path

### Backward Compatibility

The implementation maintains full backward compatibility:
- Existing clusters without `cloudNat` configuration continue to work unchanged
- Adding `cloudNat` configuration to existing clusters creates NAT resources without affecting the cluster
- No breaking changes to existing APIs or configurations

### Deployment Strategy

1. **New Clusters**: Simply add `cloudNat` configuration and deploy
2. **Existing Clusters**: Add configuration and run `sc deploy` - NAT resources are created and traffic automatically routes through them
3. **Rollback**: Set `enabled: false` and redeploy to remove NAT resources
