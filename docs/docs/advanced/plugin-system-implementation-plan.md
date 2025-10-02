# Simple Container Plugin System - Implementation Plan

## Overview

This document outlines the comprehensive plan for implementing a plugin system in Simple Container that enables custom Pulumi providers and resources without modifying the core codebase. The plugin system will maintain full compatibility with existing Simple Container concepts including compute processors, template placeholders, and JSON schema generation.

## Current Architecture Analysis

### Existing Pulumi Integration
Simple Container currently integrates with Pulumi through:

- **Provisioner Interface**: `/pkg/clouds/pulumi/provisioner.go` - Main provisioner implementation
- **Resource Registration**: `/pkg/clouds/pulumi/api/mapping.go` - Registry for providers and resources
- **Provider Initialization**: Cloud-specific `/init.go` files that register resources via `api.RegisterResources()`
- **Compute Processors**: Functions that inject environment variables and handle resource-specific logic
- **JSON Schema Generation**: `/cmd/schema-gen/main.go` - Automatic schema generation from Go structs

### Key Registration Maps
```go
var (
    ProviderFuncByType         = map[string]ProvisionFunc{}
    ProvisionFuncByType        = map[string]ProvisionFunc{}
    ComputeProcessorFuncByType = map[string]ComputeProcessorFunc{}
    RegistrarFuncByType        = map[string]RegistrarFunc{}
)
```

## Plugin System Architecture

### 1. Plugin Interface Specification

#### Core Plugin API
```go
type Plugin interface {
    // Plugin metadata
    Name() string
    Version() string
    Description() string
    
    // Resource registration
    RegisterResources() map[string]ProvisionFunc
    RegisterComputeProcessors() map[string]ComputeProcessorFunc
    RegisterProviders() map[string]ProvisionFunc
    
    // Schema generation support
    GetResourceSchemas() map[string]ResourceSchema
    
    // Initialization and cleanup
    Initialize(ctx context.Context, config PluginConfig) error
    Cleanup(ctx context.Context) error
    
    // Health checking
    HealthCheck(ctx context.Context) error
}

type PluginConfig struct {
    Name          string                 `yaml:"name"`
    Source        PluginSource          `yaml:"source"`
    Configuration map[string]interface{} `yaml:"config"`
    Enabled       bool                   `yaml:"enabled"`
}

type PluginSource struct {
    Type       string `yaml:"type"`        // "local", "github-release", "git"
    Path       string `yaml:"path"`        // Local path, GitHub repository, or Git URL
    Ref        string `yaml:"ref"`         // Git branch/tag/commit or GitHub release version
    Checksum   string `yaml:"checksum"`    // Security checksum
}
```

### 2. Communication Layer

#### Plugin Communication Architecture - REVISED RECOMMENDATION

**Primary Recommendation: HashiCorp go-plugin with gRPC**

Based on HashiCorp's battle-tested experience (Terraform, Vault, Nomad, Waypoint) and industry best practices, we recommend using **HashiCorp's go-plugin library with gRPC** instead of Go's native plugin system.

**Why NOT Go Native Plugins:**
- **Fragility**: Requires exact Go version match, identical dependencies, same build flags - extremely brittle
- **No Windows Support**: Platform limitations make it unsuitable for cross-platform tools
- **In-Process Risk**: Plugin crashes bring down the entire host process
- **Distribution Nightmare**: Source code compatibility issues make versioning impossible

**Why HashiCorp go-plugin:**
- **Battle-Tested**: Used on millions of machines for 10+ years across HashiCorp's production tools
- **Process Isolation**: Plugins run as separate processes - crashes don't affect host
- **Security**: Plugins have limited access, support TLS communication, can run with different permissions
- **Cross-Platform**: Works on Linux, macOS, Windows via standard binaries
- **Cross-Language**: gRPC interface allows plugins in any language
- **Easy Distribution**: Just distribute binaries - no source compatibility required
- **Protocol Versioning**: Version checking without rebuild requirements

#### Architecture Overview

**Plugin Execution Model:**
```
┌─────────────────────────────────────┐
│  Simple Container Host Process      │
│  ┌───────────────────────────────┐  │
│  │   Plugin Client (go-plugin)   │  │
│  └───────────┬───────────────────┘  │
│              │ gRPC over              │
│              │ Unix Socket/TCP        │
└──────────────┼────────────────────────┘
               │
┌──────────────┼────────────────────────┐
│              │                        │
│  ┌───────────▼───────────────────┐   │
│  │   Plugin Server (go-plugin)   │   │
│  │                                │   │
│  │  ┌──────────────────────────┐ │   │
│  │  │  Your Plugin Logic       │ │   │
│  │  │  (Resource Provisioning) │ │   │
│  │  └──────────────────────────┘ │   │
│  └────────────────────────────────┘   │
│                                        │
│  Plugin Binary Process                │
│  (Separate OS Process)                │
└────────────────────────────────────────┘
```

#### Go Plugin Implementation with HashiCorp go-plugin

```go
// Plugin interface using go-plugin
type SimpleContainerPlugin interface {
    // Plugin metadata
    Name() string
    Version() string
    Description() string
    
    // Resource registration - returns resource type mappings
    RegisterResources() (map[string]ResourceHandler, error)
    
    // Compute processor registration
    RegisterComputeProcessors() (map[string]ComputeProcessorHandler, error)
    
    // Schema information for JSON schema generation
    GetResourceSchemas() (map[string]ResourceSchema, error)
    
    // Initialization
    Initialize(config PluginConfig) error
    
    // Health check
    HealthCheck() error
}

// ResourceHandler wraps the provisioning logic
type ResourceHandler struct {
    ProvisionFunc       func(ctx ProvisionContext, input ResourceInput) (*ResourceOutput, error)
    ComputeProcessorFunc func(ctx ComputeContext, input ResourceInput, collector EnvCollector) (*ResourceOutput, error)
}

// PluginHost manages plugin lifecycle
type PluginHost struct {
    pluginDir     string
    loadedPlugins map[string]*LoadedPlugin
    logger        logger.Logger
}

type LoadedPlugin struct {
    client    *plugin.Client  // go-plugin client
    rpcClient plugin.ClientProtocol
    instance  SimpleContainerPlugin
    config    PluginConfig
    binary    string
}

func (ph *PluginHost) LoadPlugin(config PluginConfig) error {
    // 1. Resolve plugin binary location (download if needed)
    binaryPath, err := ph.resolvePluginBinary(config)
    if err != nil {
        return errors.Wrapf(err, "failed to resolve plugin binary")
    }
    
    // 2. Configure go-plugin client
    client := plugin.NewClient(&plugin.ClientConfig{
        HandshakeConfig: handshakeConfig,
        Plugins: map[string]plugin.Plugin{
            "simple-container-plugin": &SimpleContainerPluginGRPC{},
        },
        Cmd:              exec.Command(binaryPath),
        AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
        Logger:           ph.logger,
    })
    
    // 3. Connect to plugin
    rpcClient, err := client.Client()
    if err != nil {
        return errors.Wrapf(err, "failed to connect to plugin")
    }
    
    // 4. Request plugin interface
    raw, err := rpcClient.Dispense("simple-container-plugin")
    if err != nil {
        return errors.Wrapf(err, "failed to dispense plugin")
    }
    
    pluginInstance := raw.(SimpleContainerPlugin)
    
    // 5. Initialize plugin
    if err := pluginInstance.Initialize(config); err != nil {
        client.Kill()
        return errors.Wrapf(err, "failed to initialize plugin")
    }
    
    // 6. Register resources with Simple Container
    resources, err := pluginInstance.RegisterResources()
    if err != nil {
        client.Kill()
        return errors.Wrapf(err, "failed to register resources")
    }
    
    for resourceType, handler := range resources {
        pApi.ProvisionFuncByType[resourceType] = handler.ProvisionFunc
        pApi.ComputeProcessorFuncByType[resourceType] = handler.ComputeProcessorFunc
    }
    
    // 7. Store loaded plugin
    ph.loadedPlugins[config.Name] = &LoadedPlugin{
        client:    client,
        rpcClient: rpcClient,
        instance:  pluginInstance,
        config:    config,
        binary:    binaryPath,
    }
    
    ph.logger.Infof("Successfully loaded plugin %s v%s", 
        pluginInstance.Name(), pluginInstance.Version())
    
    return nil
}

### 2.5 Critical Architecture: Plugin Context, State Management & Resource Outputs

This section addresses the critical architectural challenges of integrating plugins with Simple Container's provisioner, Pulumi state management, and configuration flow.

#### The Architectural Challenge

Plugins run as **separate OS processes** via gRPC, but they need to:

1. **Access Full Configuration**: Reconciled client.yaml, server.yaml, and secrets.yaml
2. **Use Pulumi State**: Connect to the same state storage (S3, local, etc.)
3. **Share Secrets Provider**: Use the same KMS/passphrase encryption for secrets
4. **Return Pulumi Resources**: Resources created by plugin must be consumable by parent process
5. **Support Template Placeholders**: Enable `${resource:plugin-resource.property}` access to plugin resource outputs

#### Solution Architecture: Plugins as Resource Provisioners within Parent Context

**Key Insight**: Plugins don't run separate Pulumi programs. Instead, plugins **provision resources within the parent's Pulumi SDK context** via RPC.

**Architecture Flow**:

```
┌─────────────────────────────────────────────────────────────────┐
│  Simple Container Parent Process                                 │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Pulumi SDK Context (*sdk.Context)                         │ │
│  │  - Connected to State Storage (S3/local)                   │ │
│  │  - Connected to Secrets Provider (KMS/passphrase)          │ │
│  │  - Single Pulumi program execution                         │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                    │
│                              │ (1) Call plugin via gRPC           │
│                              │     with ProvisionContext          │
│                              ▼                                    │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Plugin Client (go-plugin gRPC)                            │ │
│  │  - Serializes context information                          │ │
│  │  - Sends: ResourceInput, ProvisionParams, Credentials      │ │
│  │  - Receives: ResourceOutput (URNs, outputs, properties)    │ │
│  └───────────┬───────────────────┘  │
│              │ gRPC over              │
│              │ Unix Socket/TCP        │
└──────────────┼────────────────────────┘
               │
┌──────────────┼────────────────────────┐
│              │                        │
│  ┌───────────▼───────────────────┐   │
│  │   Plugin Server (separate process) │
│  │  - Receives serialized context     │
│  │  - Returns Pulumi resource specification │
│  └────────────────────────────────┘   │
│                                        │
│  Plugin Binary Process                │
│  (Separate OS Process)                │
└────────────────────────────────────────┘
```

#### Go Plugin Implementation with HashiCorp go-plugin

```go
// Plugin interface using go-plugin
type SimpleContainerPlugin interface {
    // Plugin metadata
    Name() string
    Version() string
    Description() string
    
    // Resource registration - returns resource type mappings
    RegisterResources() (map[string]ResourceHandler, error)
    
    // Compute processor registration
    RegisterComputeProcessors() (map[string]ComputeProcessorHandler, error)
    
    // Schema information for JSON schema generation
    GetResourceSchemas() (map[string]ResourceSchema, error)
    
    // Initialization
    Initialize(config PluginConfig) error
    
    // Health check
    HealthCheck() error
}

// ResourceHandler wraps the provisioning logic
type ResourceHandler struct {
    ProvisionFunc       func(ctx ProvisionContext, input ResourceInput) (*ResourceOutput, error)
    ComputeProcessorFunc func(ctx ComputeContext, input ResourceInput, collector EnvCollector) (*ResourceOutput, error)
}

// PluginHost manages plugin lifecycle
type PluginHost struct {
    pluginDir     string
    loadedPlugins map[string]*LoadedPlugin
    logger        logger.Logger
}

type LoadedPlugin struct {
    client    *plugin.Client  // go-plugin client
    rpcClient plugin.ClientProtocol
    instance  SimpleContainerPlugin
    config    PluginConfig
    binary    string
}

func (ph *PluginHost) LoadPlugin(config PluginConfig) error {
    // 1. Resolve plugin binary location (download if needed)
    binaryPath, err := ph.resolvePluginBinary(config)
    if err != nil {
        return errors.Wrapf(err, "failed to resolve plugin binary")
    }
    
    // 2. Configure go-plugin client
    client := plugin.NewClient(&plugin.ClientConfig{
        HandshakeConfig: handshakeConfig,
        Plugins: map[string]plugin.Plugin{
            "simple-container-plugin": &SimpleContainerPluginGRPC{},
        },
        Cmd:              exec.Command(binaryPath),
        AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
        Logger:           ph.logger,
    })
    
    // 3. Connect to plugin
    rpcClient, err := client.Client()
    if err != nil {
        return errors.Wrapf(err, "failed to connect to plugin")
    }
    
    // 4. Request plugin interface
    raw, err := rpcClient.Dispense("simple-container-plugin")
    if err != nil {
        return errors.Wrapf(err, "failed to dispense plugin")
    }
    
    pluginInstance := raw.(SimpleContainerPlugin)
    
    // 5. Initialize plugin
    if err := pluginInstance.Initialize(config); err != nil {
        client.Kill()
        return errors.Wrapf(err, "failed to initialize plugin")
    }
    
    // 6. Register resources with Simple Container
    resources, err := pluginInstance.RegisterResources()
    if err != nil {
        client.Kill()
        return errors.Wrapf(err, "failed to register resources")
    }
    
    for resourceType, handler := range resources {
        pApi.ProvisionFuncByType[resourceType] = handler.ProvisionFunc
        pApi.ComputeProcessorFuncByType[resourceType] = handler.ComputeProcessorFunc
    }
    
    // 7. Store loaded plugin
    ph.loadedPlugins[config.Name] = &LoadedPlugin{
        client:    client,
        rpcClient: rpcClient,
        instance:  pluginInstance,
        config:    config,
        binary:    binaryPath,
    }
    
    ph.logger.Infof("Successfully loaded plugin %s v%s", 
        pluginInstance.Name(), pluginInstance.Version())
    
    return nil
}

### 2.6 Critical Issue: Pulumi Provider SDK Dependencies

#### The Problem with Resource Specs

The architecture in section 2.5 has a **critical flaw** identified: if a plugin returns a `ResourceSpec` with:

```go
ResourceType: "digitalocean:droplet:Instance"  // Or any third-party provider
```

The parent SC process would need the DigitalOcean Pulumi SDK imported to create that resource:

```go
// This requires importing github.com/pulumi/pulumi-digitalocean/sdk/go/digitalocean
ctx.RegisterResource("digitalocean:droplet:Instance", ...)  // ❌ Won't work without SDK!
```

**This defeats the entire purpose of plugins** - every provider would need to be in the main SC binary.

#### The Solution: Plugins as Pulumi gRPC Resource Providers

Instead of returning resource specs, **plugins implement Pulumi's resource provider gRPC protocol** - the same protocol that all Pulumi providers use under the hood.

**How Pulumi Providers Work:**

```
User Program                    Pulumi Engine                   Provider (Plugin)
    │                                │                                │
    │  Create Resource              │                                │
    │  (aws:s3:Bucket)              │                                │
    ├──────────────────────────────>│                                │
    │                                │  gRPC: Create Resource         │
    │                                ├───────────────────────────────>│
    │                                │                                │
    │                                │  Provisions actual resource    │
    │                                │  in AWS via AWS SDK            │
    │                                │                                │
    │                                │<───────────────────────────────┤
    │                                │  Returns: URN, outputs, props  │
    │<───────────────────────────────┤                                │
    │  Resource created              │                                │
```

**All Pulumi providers** (AWS, GCP, Azure, DigitalOcean, Datadog, etc.) work this way:
1. Provider runs as separate process
2. Implements gRPC resource provider protocol
3. Pulumi engine communicates via gRPC
4. Provider does actual cloud API calls
5. All resources stored in single Pulumi state

**Our Plugin System Uses the Same Approach!**

#### Revised Architecture: Plugin as Pulumi Provider

```
Simple Container Parent Process
├── Pulumi SDK Context
│   ├── Connected to S3 state storage
│   ├── Connected to KMS secrets provider
│   └── Provisions resources:
│       ├── Core: Uses embedded provider implementations
│       │   ├── AWS resources → built-in AWS provider code
│       │   ├── GCP resources → built-in GCP provider code
│       │   └── K8s resources → built-in K8s provider code
│       └── Plugin: Uses plugin as dynamic provider
│           ├── Register plugin as Pulumi provider
│           ├── Plugin implements provider gRPC protocol
│           ├── Create resource via plugin provider
│           └── Plugin provisions using its own SDKs
│
└── Result: Single Pulumi state with ALL resources

Plugin Process (Separate)
├── Implements Pulumi Provider gRPC Interface:
│   ├── Check() - Validate resource configuration
│   ├── Create() - Provision resource
│   ├── Read() - Read resource state
│   ├── Update() - Update resource
│   ├── Delete() - Delete resource
│   └── Invoke() - Call functions
├── Uses any SDKs it needs:
│   ├── DigitalOcean SDK
│   ├── Datadog SDK
│   ├── Custom APIs
│   └── Anything else
└── Returns standard Pulumi responses
```

#### Implementation: Dynamic Provider Registration

**Step 1: Plugin Implements Pulumi Provider Protocol**

```go
// Plugin implements Pulumi's ResourceProvider interface
type CustomProviderServer struct {
    // Plugin's provider implementation
}

// Check validates resource inputs
func (p *CustomProviderServer) Check(ctx context.Context, req *pulumirpc.CheckRequest) (*pulumirpc.CheckResponse, error) {
    // Validate resource configuration
    return &pulumirpc.CheckResponse{
        Inputs: req.News,
    }, nil
}

// Create provisions a new resource
func (p *CustomProviderServer) Create(ctx context.Context, req *pulumirpc.CreateRequest) (*pulumirpc.CreateResponse, error) {
    // Plugin provisions resource using DigitalOcean SDK, Datadog API, etc.
    resourceInputs := parseInputs(req.Properties)
    
    // Use plugin's own SDK - NOT in parent process!
    droplet, err := p.digitalOceanClient.CreateDroplet(resourceInputs)
    if err != nil {
        return nil, err
    }
    
    // Return resource ID and outputs
    return &pulumirpc.CreateResponse{
        Id: droplet.ID,
        Properties: encodeOutputs(map[string]interface{}{
            "id":        droplet.ID,
            "ipAddress": droplet.IPAddress,
            "status":    droplet.Status,
        }),
    }, nil
}

// Update modifies existing resource
func (p *CustomProviderServer) Update(ctx context.Context, req *pulumirpc.UpdateRequest) (*pulumirpc.UpdateResponse, error) {
    // Update resource using plugin's SDK
    return &pulumirpc.UpdateResponse{
        Properties: req.News,
    }, nil
}

// Delete removes resource
func (p *CustomProviderServer) Delete(ctx context.Context, req *pulumirpc.DeleteRequest) (*emptypb.Empty, error) {
    // Delete resource using plugin's SDK
    return &emptypb.Empty{}, nil
}

// Read retrieves current resource state
func (p *CustomProviderServer) Read(ctx context.Context, req *pulumirpc.ReadRequest) (*pulumirpc.ReadResponse, error) {
    // Read resource state from cloud provider
    return &pulumirpc.ReadResponse{
        Id:         req.Id,
        Properties: req.Properties,
    }, nil
}
```

**Step 2: Parent Registers Plugin as Dynamic Provider**

```go
// In provisioner.go - plugin loading
func (p *pulumi) loadPluginAsProvider(pluginConfig PluginConfig) error {
    // 1. Launch plugin process
    pluginClient := plugin.NewClient(&plugin.ClientConfig{
        HandshakeConfig: handshakeConfig,
        Plugins: map[string]plugin.Plugin{
            "pulumi-provider": &PulumiProviderPlugin{},
        },
        Cmd:              exec.Command(pluginBinaryPath),
        AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
    })
    
    // 2. Get plugin's gRPC connection
    rpcClient, err := pluginClient.Client()
    if err != nil {
        return err
    }
    
    // 3. Get the provider service
    raw, err := rpcClient.Dispense("pulumi-provider")
    if err != nil {
        return err
    }
    
    providerPlugin := raw.(PulumiProviderService)
    
    // 4. Register with Pulumi engine as dynamic provider
    // Pulumi will now route requests for this provider type to our plugin
    providerInfo := ProviderInfo{
        Name:    pluginConfig.Name,
        Version: pluginConfig.Source.Version,
        GRPCEndpoint: providerPlugin.GRPCAddress(),
    }
    
    // 5. Store provider info for resource provisioning
    p.pluginProviders[pluginConfig.Name] = providerInfo
    
    return nil
}
```

**Step 3: Parent Uses Plugin Provider for Resources**

```go
// In provision.go - configureResource()
func (p *pulumi) configureResource(
    ctx *sdk.Context,
    stack api.Stack,
    env string,
    resName string,
    res api.ResourceDescriptor,
    collector pApi.ComputeContext,
    outs pApi.ResourcesOutputs,
) (*api.ResourceOutput, error) {
    // Check if this is a plugin resource type
    providerName, isPluginResource := p.getProviderForResourceType(res.Type)
    
    if isPluginResource {
        // Get plugin provider info
        providerInfo := p.pluginProviders[providerName]
        
        // Create provider resource pointing to plugin
        provider, err := pulumi.NewProvider(ctx, providerName, &pulumi.ProviderArgs{
            // This tells Pulumi to use our plugin's gRPC endpoint
            PluginDownloadURL: pulumi.String(providerInfo.GRPCEndpoint),
        })
        if err != nil {
            return nil, err
        }
        
        // Create resource using plugin provider
        // Format: "pluginname:module:ResourceType"
        resource, err := ctx.RegisterResource(
            res.Type,  // e.g., "customprovider:compute:Instance"
            resName,
            res.Config.Config,
            pulumi.Provider(provider),  // Use plugin as provider
        )
        
        // Resource is provisioned by plugin, but stored in parent's Pulumi state!
        return &api.ResourceOutput{
            Ref: resource,
        }, nil
    }
    
    // Existing logic for core resources
    // ...
}
```

#### Key Benefits of This Approach

**1. No SDK Dependencies in Parent** 
- Plugin contains DigitalOcean SDK, Datadog SDK, etc.
- Parent SC process only needs Pulumi engine
- Main binary stays small

**2. Single Pulumi State** 
- All resources in one state file
- Parent Pulumi engine coordinates everything
- No state synchronization issues

**3. Standard Pulumi Protocol** 
- Uses Pulumi's battle-tested provider protocol
- Same approach as AWS, GCP, Azure providers
- Mature and well-documented

**4. Full Provider Capabilities** 
- Plugins can use any SDKs/APIs they need
- Complete CRUD lifecycle management
- Custom validation and transformations

**5. Pulumi Features Work** 
- Dependencies between resources
- Template placeholders via outputs
- Secrets management
- State refresh and updates

#### Configuration Example

```yaml
# server.yaml
plugins:
  - name: digitalocean
    source:
      type: github-release
      repository: simple-container-plugins/sc-digitalocean-provider
      version: v1.0.0
      platform: auto
    enabled: true
    config:
      # Plugin-specific configuration
      apiToken: "${secret:do-api-token}"

resources:
  resources:
    production:
      # Resource type format: {plugin-name}:{module}:{type}
      web-server:
        type: digitalocean:compute:Droplet
        name: web-server-01
        config:
          size: "s-2vcpu-4gb"
          region: "nyc3"
          image: "ubuntu-22-04-x64"
          # Plugin has full DigitalOcean SDK available!
          
      monitoring:
        type: datadog:monitors:Monitor
        name: cpu-monitor
        config:
          type: "metric alert"
          query: "avg(last_5m):avg:system.cpu.user{*} > 90"
```

#### Provider Protocol Communication Flow

```
1. User defines resource in server.yaml:
   type: digitalocean:compute:Droplet

2. Parent SC loads DigitalOcean plugin:
   - Launches plugin process
   - Plugin starts gRPC server
   - Registers as Pulumi provider

3. Parent provisions resources:
   - Pulumi engine sees "digitalocean:*" resource
   - Routes to plugin via gRPC
   - Plugin creates DigitalOcean droplet via DO SDK
   - Plugin returns resource ID and outputs
   - Pulumi stores in state

4. Resource outputs available:
   - Template placeholders: ${resource:web-server.ipAddress}
   - Dependencies: Other resources can depend on it
   - Compute processors: Auto-inject env vars

5. All resources in single state file:
   {
     "resources": [
       {"type": "aws:s3:Bucket", ...},           # Core
       {"type": "digitalocean:compute:Droplet", ...}, # Plugin
       {"type": "datadog:monitors:Monitor", ...}      # Plugin
     ]
   }
```

#### Summary: The Correct Architecture

**Plugins are Pulumi Resource Providers**, not resource spec generators:

 No SDK Dependencies in Parent 
- Plugin contains DigitalOcean SDK, Datadog SDK, etc.
- Parent SC process only needs Pulumi engine
- Main binary stays small

 Single Pulumi State 
- All resources in one state file
- Parent Pulumi engine coordinates everything
- No state synchronization issues

 Standard Pulumi Protocol 
- Uses Pulumi's battle-tested provider protocol
- Same approach as AWS, GCP, Azure providers
- Mature and well-documented

 Full Provider Capabilities 
- Plugins can use any SDKs/APIs they need
- Complete CRUD lifecycle management
- Custom validation and transformations

 Pulumi Features Work 
- Dependencies between resources
- Template placeholders via outputs
- Secrets management
- State refresh and updates

The key insight is: **Plugins define resources, parent provisions them**. This keeps all Pulumi complexity in the parent process while allowing plugins to extend functionality safely.

### 2.7 Critical Clarification: Plugins Don't Receive Pulumi Context

#### The Question: What About *sdk.Context?

A natural question arises: **"How do we pass the Pulumi SDK context (`*sdk.Context`) to plugins?"**

**Answer: We don't! And we shouldn't!**

The `*sdk.Context` contains:
- Runtime state tied to the parent process
- Channels for communication with Pulumi engine
- Go-specific types that can't be serialized
- Active connections to state storage and secrets provider
- References to the parent's Pulumi engine instance

**Attempting to serialize `*sdk.Context` would not work and is architecturally incorrect.**

#### The Correct Mental Model: Two Different Abstractions

There are **two completely different ways** to interact with Pulumi, and it's critical to understand which one applies to which component:

**Abstraction 1: Pulumi Program API** (`*sdk.Context`, `ctx.RegisterResource()`) → Parent only
```go
// This is what Simple Container's parent process does
func provisionProgram(ctx *sdk.Context) error {
    // You HAVE *sdk.Context (Pulumi SDK context)
    // You call ctx.RegisterResource()
    // You write Pulumi program code
    
    bucket, err := s3.NewBucket(ctx, "my-bucket", &s3.BucketArgs{
        Bucket: pulumi.String("my-bucket-name"),
    })
    
    return nil
}
```

**Abstraction 2: Pulumi Provider API** (gRPC protocol, `Create()`, `Read()`, etc.) → Plugin only
```go
// This is what plugins do - they implement provider gRPC servers
type PluginProvider struct {
    // NO *sdk.Context here!
    config PluginConfig
    client *CustomSDKClient
}

// Plugins receive gRPC requests from Pulumi engine
func (p *PluginProvider) Create(
    ctx context.Context,              // Standard Go context (for timeout/cancellation)
    req *pulumirpc.CreateRequest,     // gRPC request with properties
) (*pulumirpc.CreateResponse, error) {
    
    // Parse properties from gRPC protobuf message
    props := req.Properties.AsMap()
    size := props["size"].(string)
    
    // Use plugin's own SDK to provision resource
    resource, err := p.client.CreateDroplet(ctx, size, props)
    if err != nil {
        return nil, err
    }
    
    // Return gRPC response with ID and outputs
    return &pulumirpc.CreateResponse{
        Id: resource.ID,
        Properties: &structpb.Struct{
            Fields: map[string]*structpb.Value{
                "ipAddress": structpb.NewStringValue(resource.IPAddress),
                "status":    structpb.NewStringValue(resource.Status),
            },
        },
    }, nil
}
```

#### Architecture: No Context Passing Required

```
┌──────────────────────────────────────────────────────────────────┐
│ Parent Process (Simple Container)                                │
│ Uses: Pulumi Program API                                         │
│                                                                    │
│ func provisionProgram(ctx *sdk.Context) error {                  │
│     // Parent HAS *sdk.Context                                   │
│     // Parent uses Pulumi Program API                              │
│                                                                    │
│     // For plugin resources, just call RegisterResource:         │
│     resource, err := ctx.RegisterResource(                       │
│         "digitalocean:compute:Droplet",  // Resource type        │
│         "my-droplet",                     // Name                 │
│         map[string]interface{}{          // Properties           │
│             "size":   "s-2vcpu-4gb",                             │
│             "region": "nyc3",                                    │
│             "image":  "ubuntu-22-04-x64",                        │
│         },                                                        │
│         pulumi.Provider(p.getPluginProvider("digitalocean")),  // Route to plugin     │
│     )                                                             │
│                                                                    │
│     // Pulumi engine handles everything else!                    │
│     return nil                                                    │
│ }                                                                 │
│                                                                    │
└────────────────────┬─────────────────────────────────────────────┘
                     │
                     │ Pulumi Engine (automatic)
                     │ 1. Sees "digitalocean:*" resource type
                     │ 2. Routes to plugin provider via gRPC
                     │ 3. Serializes properties to protobuf
                     │
                     │ gRPC Message:
                     │ CreateRequest {
                     │   urn: "urn:pulumi:prod::myapp::digitalocean:compute:Droplet::my-droplet"
                     │   properties: {
                     │     "size": "s-2vcpu-4gb",
                     │     "region": "nyc3",
                     │     "image": "ubuntu-22-04-x64"
                     │   }
                     │ }
                     │
┌────────────────────▼─────────────────────────────────────────────┐
│ Plugin Process (Separate OS Process)                             │
│ Implements: Pulumi Provider API                                  │
│                                                                    │
│ func (p *DigitalOceanPlugin) Create(                            │
│     ctx context.Context,              // Standard Go context!    │
│     req *pulumirpc.CreateRequest,     // gRPC request from Pulumi engine
│ ) (*pulumirpc.CreateResponse, error) {                           │
│                                                                    │
│     // NO *sdk.Context here!                                     │
│     // Just receives properties as JSON/protobuf                 │
│                                                                    │
│     // Parse properties from gRPC request                        │
│     props := req.Properties.AsMap()                              │
│     size := props["size"].(string)                               │
│     region := props["region"].(string)                           │
│     image := props["image"].(string)                             │
│                                                                    │
│     // Access plugin's stored config (from initialization)       │
│     apiToken := p.authData["digitalocean"].Config["apiToken"]   │
│                                                                    │
│     // Use DigitalOcean SDK (contained in plugin binary)         │
│     client := godo.NewFromToken(apiToken)                        │
│     droplet, err := client.Droplets.Create(ctx, &godo.DropletCreateRequest{ │
│         Name:   "my-droplet",                                    │
│         Size:   size,                                            │
│         Region: region,                                          │
│         Image:  godo.DropletCreateImage{Slug: image},           │
│     })                                                            │
│     if err != nil {                                              │
│         return nil, err                                          │
│     }                                                             │
│                                                                    │
│     // Return gRPC response                                      │
│     return &pulumirpc.CreateResponse{                            │
│         Id: strconv.Itoa(droplet.ID),                           │
│         Properties: &structpb.Struct{                            │
│             Fields: map[string]*structpb.Value{                  │
│                 "id":        structpb.NewNumberValue(float64(droplet.ID)), │
│                 "ipAddress": structpb.NewStringValue(droplet.Networks.V4[0].IPAddress), │
│                 "status":    structpb.NewStringValue(droplet.Status), │
│             },                                                    │
│         },                                                        │
│     }, nil                                                        │
│ }                                                                 │
│                                                                    │
└──────────────────────────────────────────────────────────────────┘
```

#### What Plugins Actually Receive

**1. During Plugin Initialization (one-time setup via HashiCorp go-plugin handshake):**

```go
// Simple Container-specific initialization payload
type PluginInitConfig struct {
    // Serialized configuration data (NOT runtime Pulumi state!)
    ConfigFile     *api.ConfigFile        // Parsed client.yaml + server.yaml
    SecretsData    map[string]string      // Resolved secrets from secrets.yaml
    AuthData       map[string]AuthConfig  // Authentication configs
    PluginConfig   map[string]interface{} // Plugin-specific config from server.yaml
    
    // State and secrets provider connection info (NOT runtime state!)
    StateStorage    StateStorageInfo      // How to connect to S3/local (not used by plugin)
    SecretsProvider SecretsProviderInfo   // How to use KMS/passphrase (not used by plugin)
}

// Plugin stores this configuration for later use
func (p *Plugin) Initialize(ctx context.Context, config *PluginInitConfig) error {
    p.config = config.ConfigFile
    p.secrets = config.SecretsData
    p.auth = config.AuthData
    
    // Initialize plugin's SDK clients using auth data
    p.doClient = godo.NewFromToken(config.AuthData["digitalocean"].APIToken)
    
    return nil
}
```

**2. During Resource Operations (per-resource via Pulumi provider gRPC protocol):**

```go
// Standard Pulumi provider protocol - from pulumi/pulumi/sdk/v3/proto/pulumi/provider.proto
message CreateRequest {
    string urn = 1;                        // Resource URN
    google.protobuf.Struct properties = 2; // Resource properties as JSON
    double timeout = 3;                    // Operation timeout
    bool preview = 4;                      // Is this a preview?
}

message CreateResponse {
    string id = 1;                         // Resource ID
    google.protobuf.Struct properties = 2; // Resource outputs as JSON
}

// Plugin receives properties, NOT *sdk.Context!
```

#### Complete Data Flow Example

**Step 1: Parent Process (Has *sdk.Context)**

```go
// In provisioner.go - parent's provisionProgram()
func (p *pulumi) provisionProgram(stack api.Stack, cfg *api.ConfigFile) func(ctx *sdk.Context) error {
    program := func(ctx *sdk.Context) error {
        // Parent HAS *sdk.Context
        // Parent uses Pulumi Program API
        
        // Configure plugin resources
        for resName, res := range stack.Server.Resources.Resources["production"].Resources {
            if isPluginResource(res.Type) {
                // Just call RegisterResource - Pulumi SDK call
                // Pulumi engine will handle routing to plugin
                resource, err := ctx.RegisterResource(
                    res.Type,              // "digitalocean:compute:Droplet"
                    resName,               // "web-server"
                    res.Config.Config,     // map[string]interface{} with properties
                    pulumi.Provider(p.getPluginProvider(res.Type)),
                )
                if err != nil {
                    return err
                }
                
                // resource is a Pulumi resource handle
                // Outputs available via resource.URN(), resource.ID(), etc.
                // No *sdk.Context passed to plugin at all!
            }
        }
        return nil
    }
    return program
}
```

**Step 2: Pulumi Engine (Automatic Routing - happens inside Pulumi SDK)**

```
Pulumi Engine Process:
1. Parent calls: ctx.RegisterResource("digitalocean:compute:Droplet", ...)
2. Engine sees resource type prefix "digitalocean:*"
3. Engine looks up registered provider for "digitalocean"
4. Engine finds plugin provider's gRPC endpoint
5. Engine serializes properties to protobuf (google.protobuf.Struct)
6. Engine makes gRPC call: Create(CreateRequest{urn: "...", properties: {...}})
7. Engine waits for gRPC response from plugin
```

**Step 3: Plugin Process (No *sdk.Context - Just gRPC)**

```go
// Plugin implements Pulumi provider gRPC server
func (p *DigitalOceanPlugin) Create(
    ctx context.Context,              // Standard Go context (for timeout/cancellation)
    req *pulumirpc.CreateRequest,     // gRPC request from Pulumi engine
) (*pulumirpc.CreateResponse, error) {
    
    // Parse properties from protobuf (automatic deserialization)
    props := req.Properties.AsMap()
    
    // Access stored configuration (from initialization, NOT passed per-request)
    apiToken := p.auth["digitalocean"].Config["apiToken"].(string)
    
    // Use DigitalOcean SDK (contained in plugin binary, NOT in parent)
    client := godo.NewFromToken(apiToken)
    droplet, err := client.Droplets.Create(ctx, &godo.DropletCreateRequest{
        Name:   props["name"].(string),
        Size:   props["size"].(string),
        Region: props["region"].(string),
        Image:  godo.DropletCreateImage{Slug: props["image"].(string)},
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to create droplet: %v", err)
    }
    
    // Build gRPC response
    outputs := &structpb.Struct{
        Fields: map[string]*structpb.Value{
            "id":        structpb.NewNumberValue(float64(droplet.ID)),
            "ipAddress": structpb.NewStringValue(droplet.Networks.V4[0].IPAddress),
            "status":    structpb.NewStringValue(droplet.Status),
            "region":    structpb.NewStringValue(droplet.Region.Slug),
        },
    }
    
    // Return gRPC response to Pulumi engine
    return &pulumirpc.CreateResponse{
        Id: strconv.Itoa(droplet.ID),
        Properties: outputs,
    }, nil
}
```

**Step 4: Back to Parent (Results Stored in State)**

```
Pulumi Engine:
1. Receives gRPC response from plugin: CreateResponse{id: "droplet-123456", properties: {...}}
2. Stores resource in parent's Pulumi state file (S3/local storage)
3. Returns resource handle to parent's ctx.RegisterResource() call
4. Parent can now access resource outputs
5. Outputs available for template placeholders: ${resource:web-server.ipAddress}
```

#### Key Architectural Points

**1. Two Separate Abstractions:**
- **Pulumi Program API** (`*sdk.Context`, `ctx.RegisterResource()`) → Parent only
- **Pulumi Provider API** (gRPC protocol, `Create()`, `Read()`, etc.) → Plugin only
- These abstractions never cross process boundaries

**2. No Context Serialization:**
- `*sdk.Context` stays in parent process forever
- Plugins receive gRPC requests with serialized properties (protobuf)
- Standard protobuf serialization (JSON-like structure)
- No Go-specific types crossing boundaries

**3. Plugin Has Standard context.Context:**
- Plugins receive `context.Context` (standard Go) for cancellation/timeout
- This is **NOT** `*sdk.Context` (Pulumi SDK-specific)
- Just standard context for any Go gRPC service
- Used for request cancellation, timeouts, tracing

**4. State Storage:**
- **Only parent** connects to S3/local state storage
- Plugin **never** touches state files directly
- Plugin **never** connects to secrets provider directly
- Pulumi engine (in parent) coordinates everything
- Single source of truth for all state

**5. Configuration Passing:**
- One-time configuration during plugin initialization (via HashiCorp go-plugin handshake)
- Not per-resource (too expensive for gRPC overhead)
- Plugin caches configuration in memory for all resource operations
- Credentials passed once, securely, during initialization

**6. Standard Pulumi Provider Protocol:**
- This is the **exact same protocol** that AWS, GCP, Azure, DigitalOcean, Datadog providers use
- Battle-tested in production for years
- Well-documented in Pulumi's protobuf definitions
- No custom protocol needed

#### Why This Architecture Works

**Benefits:**
- No serialization complexity - Properties are simple JSON-like data
- Process isolation - Plugin crashes don't affect parent
- Standard protocol - Uses proven Pulumi provider gRPC protocol
- Single state - All state in parent, no synchronization issues
- Clean separation - Parent writes programs, plugins implement providers
- SDK independence - Plugin SDKs never loaded in parent
- Type safety - gRPC protobuf provides type definitions

**Example of What Gets Serialized:**
```json
{
  "size": "s-2vcpu-4gb",
  "region": "nyc3",
  "image": "ubuntu-22-04-x64",
  "tags": ["web", "production"]
}
```

**What Does NOT Get Serialized:**
- `*sdk.Context` (runtime state, channels, connections)
- Pulumi engine references
- State storage connections
- Go-specific types or pointers

#### Summary: The Correct Model

**The question "How do we pass `*sdk.Context` to plugins?" is based on a misunderstanding.**

The correct model is:
- Parent **has** `*sdk.Context` and uses Pulumi **Program API**
- Plugin **implements** Pulumi **Provider API** (gRPC server)
- Pulumi engine **bridges** between them automatically
- Plugin **receives** gRPC requests with properties (JSON-like data)
- Plugin **returns** gRPC responses with outputs (JSON-like data)
- No serialization of `*sdk.Context` needed, wanted, or possible

This is **exactly how all Pulumi providers work**:
1. AWS provider doesn't have `*sdk.Context`
2. GCP provider doesn't have `*sdk.Context`
3. DigitalOcean provider doesn't have `*sdk.Context`
4. They all implement provider gRPC protocol
5. Pulumi engine coordinates communication
6. All state managed by Pulumi engine in main process

Our plugins work **identically** - we're just making provider development dynamic and pluggable!

### 2.8 Provisioner Integration: Dynamic Plugin Registration

#### Current Resource Registration Pattern (Core Providers)

Currently, Simple Container uses **compile-time registration** via `init()` methods:

**Example: AWS Provider Registration (pkg/clouds/pulumi/aws/init.go)**
```go
package aws

import (
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	// Called at compile time when package is imported
	api.RegisterInitStateStore(aws.ProviderType, InitStateStore)
	api.RegisterProvider(aws.ProviderType, Provider)
	
	// Register all AWS resources into global map
	api.RegisterResources(map[string]api.ProvisionFunc{
		aws.ResourceTypeS3Bucket:      S3Bucket,
		aws.SecretsProviderTypeAwsKms: KmsKeySecretsProvider,
		aws.TemplateTypeEcsFargate:    EcsFargate,
		aws.TemplateTypeAwsLambda:     Lambda,
		aws.ResourceTypeRdsPostgres:   RdsPostgres,
		aws.ResourceTypeRdsMysql:      RdsMysql,
		// ... more resources
	})
	
	// Register compute processors
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		aws.ResourceTypeS3Bucket:    S3BucketComputeProcessor,
		aws.ResourceTypeRdsPostgres: RdsPostgresComputeProcessor,
		// ... more processors
	})
}
```

**Global Registration Maps (pkg/clouds/pulumi/api/mapping.go)**
```go
var (
	InitStateStoreFuncByType   = map[string]InitStateStoreFunc{}
	ProviderFuncByType         = map[string]ProvisionFunc{}
	ProvisionFuncByType        = map[string]ProvisionFunc{}   // Resources lookup
	RegistrarFuncByType        = map[string]RegistrarFunc{}
	ComputeProcessorFuncByType = map[string]ComputeProcessorFunc{}
)

func RegisterResources(register map[string]ProvisionFunc) {
	ProvisionFuncByType = lo.Assign(ProvisionFuncByType, register)
}
```

**Current Resource Provisioning (pkg/clouds/pulumi/provision.go)**
```go
func (p *pulumi) configureResource(
	ctx *sdk.Context,
	stack api.Stack,
	env string,
	resName string,
	res api.ResourceDescriptor,
	collector pApi.ComputeContext,
	outs pApi.ResourcesOutputs,
) (*api.ResourceOutput, error) {
	// ... prepare params
	
	// Lookup resource function from global map
	if fnc, ok := pApi.ProvisionFuncByType[res.Type]; !ok {
		return nil, errors.Errorf("unknown resource type %q", res.Type)
	} else {
		// Call the function directly
		return fnc(ctx, stack, api.ResourceInput{...}, provisionParams)
	}
}
```

#### Why init() Pattern Won't Work for Plugins

**Problem:**
1. `init()` runs at **compile time** when packages are imported
2. Plugins are loaded **at runtime** dynamically
3. Plugins run in **separate processes** - can't register into parent's global maps
4. Plugin resources aren't known until configuration is loaded

**This is fundamentally different** from core resources that are compiled into the binary.

#### Plugin Registration Pattern: Runtime Provider Registration

**Key Difference:** Plugins don't register individual resources - they register as **Pulumi providers**.

**Architecture:**

```
Core Resources (Compile Time):
├── init() called during binary startup
├── Resources registered into ProvisionFuncByType map
└── Lookup via map[resourceType]func

Plugin Resources (Runtime):
├── Plugin loaded during provisioner initialization
├── Plugin registered as Pulumi provider (not in map)
├── Lookup via resource type prefix → route to plugin provider
└── Pulumi engine handles communication
```

#### Modified Provisioner: Hybrid Routing

The provisioner needs to support **both** patterns:

**Step 1: Plugin Loading During Provisioner Initialization**

```go
// In provisioner.go - new field
type pulumi struct {
	logger logger.Logger
	pubKey string
	
	// Existing fields...
	backend                   backend.Backend
	stackRef                  backend.StackReference
	
	// NEW: Plugin management
	pluginHost    *PluginHost              // Manages plugin lifecycle
	pluginProviders map[string]*PluginProviderInfo  // Plugin provider registry
	
	// Existing fields...
	fieldConfigReader api.ProvisionerFieldConfigReaderFunc
	provisionerCfg    *ProvisionerConfig
	configFile        *api.ConfigFile
}

type PluginProviderInfo struct {
	Name           string
	Plugin         *LoadedPlugin
	ProviderHandle pulumi.ProviderResource  // Pulumi provider resource
	ResourcePrefix string                    // e.g., "digitalocean:"
}
```

**Step 2: Load Plugins Before Provisioning**

```go
// In provisioner.go - called during initialization
func (p *pulumi) loadPlugins(ctx context.Context, cfg *api.ConfigFile) error {
	// Initialize plugin host
	p.pluginHost = NewPluginHost(p.logger, "./plugins")
	p.pluginProviders = make(map[string]*PluginProviderInfo)
	
	// Load each configured plugin
	for _, pluginConfig := range cfg.Provisioner.Plugins {
		if !pluginConfig.Enabled {
			continue
		}
		
		// Load plugin binary (download if needed)
		loadedPlugin, err := p.pluginHost.LoadPlugin(pluginConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to load plugin %s", pluginConfig.Name)
		}
		
		// Initialize plugin with configuration
		initConfig := &PluginInitConfig{
			ConfigFile:   cfg,
			SecretsData:  p.resolveSecrets(),
			AuthData:     p.getAuthConfigurations(),
			PluginConfig: pluginConfig.Configuration,
		}
		if err := loadedPlugin.Initialize(ctx, initConfig); err != nil {
			return errors.Wrapf(err, "failed to initialize plugin %s", pluginConfig.Name)
		}
		
		// Store plugin info for routing
		p.pluginProviders[pluginConfig.Name] = &PluginProviderInfo{
			Name:           pluginConfig.Name,
			Plugin:         loadedPlugin,
			ResourcePrefix: pluginConfig.Name + ":",  // e.g., "digitalocean:"
		}
		
		p.logger.Infof("Loaded plugin: %s v%s", pluginConfig.Name, loadedPlugin.Version())
	}
	
	return nil
}

// Call this during ProvisionStack
func (p *pulumi) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, stack api.Stack, params api.ProvisionParams) error {
	// Load plugins before provisioning
	if err := p.loadPlugins(ctx, cfg); err != nil {
		return errors.Wrap(err, "failed to load plugins")
	}
	
	// Continue with existing logic
	if err := p.createStackIfNotExists(ctx, cfg, stack); err != nil {
		return errors.Wrapf(err, "failed to create stack %q if not exists", stack.Name)
	}
	return p.provisionStack(ctx, cfg, stack, params)
}
```

**Step 3: Register Plugin as Pulumi Provider During Program Execution**

```go
// In provision.go - during provisionProgram()
func (p *pulumi) provisionProgram(stack api.Stack, cfg *api.ConfigFile) func(ctx *sdk.Context) error {
	program := func(ctx *sdk.Context) error {
		// Register plugin providers with Pulumi
		for pluginName, pluginInfo := range p.pluginProviders {
			// Create Pulumi provider resource that routes to plugin
			providerResource, err := p.registerPluginProvider(ctx, pluginInfo)
			if err != nil {
				return errors.Wrapf(err, "failed to register plugin provider %s", pluginName)
			}
			
			// Store provider handle for resource provisioning
			pluginInfo.ProviderHandle = providerResource
		}
		
		// Existing logic for registrar, resources, etc.
		if err := p.initRegistrar(ctx, stack, nil); err != nil {
			return errors.Wrapf(err, "failed to init registar")
		}
		
		// ... rest of provisioning
		for env, resources := range stack.Server.Resources.Resources {
			// ... existing logic
		}
		
		return nil
	}
	return program
}

func (p *pulumi) registerPluginProvider(ctx *sdk.Context, pluginInfo *PluginProviderInfo) (pulumi.ProviderResource, error) {
	// Create a dynamic provider that routes to plugin's gRPC endpoint
	provider, err := pulumi.NewProvider(
		ctx,
		pluginInfo.Name,
		&pulumi.ProviderArgs{
			// This tells Pulumi to use plugin's gRPC endpoint
			PluginDownloadURL: pulumi.String(pluginInfo.Plugin.GRPCAddress()),
		},
		pulumi.URN(fmt.Sprintf("urn:pulumi:%s::%s::pulumi:providers:%s",
			p.stackRef.Name(),
			p.configFile.ProjectName,
			pluginInfo.Name,
		)),
	)
	
	return provider, err
}
```

**Step 4: Modified configureResource() - Hybrid Routing**

```go
func (p *pulumi) configureResource(
	ctx *sdk.Context,
	stack api.Stack,
	env string,
	resName string,
	res api.ResourceDescriptor,
	collector pApi.ComputeContext,
	outs pApi.ResourcesOutputs,
) (*api.ResourceOutput, error) {
	p.logger.Info(ctx.Context(), "configure resource %q for stack %q in env %q", resName, stack.Name, env)
	
	if res.Name == "" {
		res.Name = resName
	}
	
	// NEW: Check if this is a plugin resource
	if pluginInfo := p.getPluginForResourceType(res.Type); pluginInfo != nil {
		return p.provisionPluginResource(ctx, stack, env, resName, res, collector, outs, pluginInfo)
	}
	
	// EXISTING: Core resource - use map lookup
	provisionParams, err := p.getProvisionParams(ctx, stack, res, env, "")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init provision params for %q", res.Type)
	}
	provisionParams.ComputeContext = collector
	provisionParams.ResourceOutputs = outs
	
	// Lookup from global map (registered via init())
	if fnc, ok := pApi.ProvisionFuncByType[res.Type]; !ok {
		return nil, errors.Errorf("unknown resource type %q", res.Type)
	} else {
		return fnc(ctx, stack, api.ResourceInput{
			Descriptor: &res,
			StackParams: &api.StackParams{
				StackName:   stack.Name,
				Environment: env,
			},
		}, provisionParams)
	}
}

// NEW: Determine if resource type belongs to a plugin
func (p *pulumi) getPluginForResourceType(resourceType string) *PluginProviderInfo {
	// Resource type format: "pluginname:module:Type"
	// e.g., "digitalocean:compute:Droplet"
	
	for _, pluginInfo := range p.pluginProviders {
		if strings.HasPrefix(resourceType, pluginInfo.ResourcePrefix) {
			return pluginInfo
		}
	}
	
	return nil  // Not a plugin resource
}

// NEW: Provision resource via plugin provider
func (p *pulumi) provisionPluginResource(
	ctx *sdk.Context,
	stack api.Stack,
	env string,
	resName string,
	res api.ResourceDescriptor,
	collector pApi.ComputeContext,
	outs pApi.ResourcesOutputs,
	pluginInfo *PluginProviderInfo,
) (*api.ResourceOutput, error) {
	p.logger.Info(ctx.Context(), "provisioning plugin resource %q via plugin %q", resName, pluginInfo.Name)
	
	// Create resource using plugin's Pulumi provider
	// Pulumi engine will route to plugin's gRPC server
	resource, err := ctx.RegisterResource(
		res.Type,  // "digitalocean:compute:Droplet"
		resName,   // "web-server"
		res.Config.Config,  // Properties
		pulumi.Provider(pluginInfo.ProviderHandle),  // Route to plugin
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision plugin resource %q", resName)
	}
	
	// Plugin handles provisioning via its gRPC Create() method
	// Outputs are automatically available through Pulumi
	
	return &api.ResourceOutput{
		Ref: resource,
		AdditionalOutputs: make(map[string]interface{}),
	}, nil
}
```

#### Resource Type Routing Logic

**Decision Tree:**

```
Resource Type: "s3-bucket"
├─ Check: Does it match plugin prefix pattern? (contains "pluginname:")
│  └─ NO → Lookup in ProvisionFuncByType map
│     └─ Found: aws.S3Bucket function
│        └─ Call function directly in parent process
│
Resource Type: "digitalocean:compute:Droplet"
├─ Check: Does it match plugin prefix pattern?
│  └─ YES → Prefix is "digitalocean:"
│     └─ Check: Is "digitalocean" plugin loaded?
│        └─ YES → Route to plugin provider
│           └─ ctx.RegisterResource() with pulumi.Provider(pluginProvider)
│              └─ Pulumi engine routes gRPC to plugin
│                 └─ Plugin creates resource in its process
```

#### Comparison: Core vs Plugin Registration

| Aspect | Core Resources | Plugin Resources |
|--------|----------------|------------------|
| **Registration Time** | Compile time (`init()`) | Runtime (provisioner initialization) |
| **Registration Method** | `api.RegisterResources()` into global map | Register as Pulumi provider |
| **Lookup Method** | Map lookup `ProvisionFuncByType[type]` | Resource type prefix matching |
| **Execution** | Function call in parent process | gRPC call to plugin process |
| **Resource Format** | `"s3-bucket"`, `"rds-postgres"` | `"pluginname:module:Type"` |
| **Binary Location** | Compiled into main binary | Separate plugin binary |
| **Dependencies** | SDK compiled into parent | SDK in plugin binary only |

#### Benefits of This Approach

**1. No Changes to Core Resources** 
- Existing `init()` pattern continues to work
- All core resources function identically
- Zero breaking changes

**2. Clear Separation** 
- Core resources: compile-time registration, map lookup
- Plugin resources: runtime registration, provider routing
- Different code paths, no conflicts

**3. Resource Type Convention** 
- Core resources: simple names (`"s3-bucket"`)
- Plugin resources: namespaced (`"digitalocean:compute:Droplet"`)
- Easy to distinguish at runtime

**4. Backward Compatibility** 
- Existing configurations work unchanged
- New plugin configurations use new format
- Gradual migration path

**5. Performance** 
- Core resources: direct function call (fast)
- Plugin resources: gRPC call (slightly slower, but isolated)
- Each optimized for its use case

#### Configuration Examples

**Core Resource (Existing Pattern):**
```yaml
resources:
  production:
    state-bucket:
      type: s3-bucket  # Simple type, no prefix
      name: my-state-bucket
      config:
        name: my-state-bucket
        allowOnlyHttps: true
```

**Plugin Resource (New Pattern):**
```yaml
plugins:
  - name: digitalocean
    source:
      type: github-release
      repository: sc-plugins/sc-digitalocean-provider
      version: v1.0.0
    enabled: true

resources:
  production:
    web-server:
      type: digitalocean:compute:Droplet  # Prefixed type
      name: web-server-01
      config:
        size: "s-2vcpu-4gb"
        region: "nyc3"
```

#### Summary: Hybrid Registration System

The provisioner will support **both patterns simultaneously**:

**Core Resources (Unchanged):**
- Compile-time registration via `init()` methods
- Resources stored in `ProvisionFuncByType` map
- Direct function call execution
- All existing code continues to work

**Plugin Resources (New):**
- Plugins loaded at runtime during initialization
- Plugins registered as Pulumi providers
- Resource type prefix routing (`"pluginname:"`)
- gRPC-based execution in separate process

**Routing Logic:**
```go
if resourceType contains ":" && plugin exists for prefix {
    → Route to plugin provider (gRPC)
} else {
    → Lookup in ProvisionFuncByType map (direct call)
}
