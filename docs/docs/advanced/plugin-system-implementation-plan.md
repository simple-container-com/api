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
    Type       string `yaml:"type"`        // "local", "git", "registry"
    Path       string `yaml:"path"`        // Local path or Git URL
    Ref        string `yaml:"ref"`         // Git branch/tag/commit
    Checksum   string `yaml:"checksum"`    // Security checksum
}
```

### 2. Communication Layer

#### Plugin Communication Options

**Option A: Go Plugin Architecture (Recommended)**
- Use Go's `plugin` package for dynamic loading
- Plugins compiled as `.so` files
- Direct Go API integration
- Best performance and type safety

**Option B: gRPC Communication**
- Language-agnostic plugin support
- Network-based communication
- More complex but supports non-Go plugins
- Better isolation and security

**Option C: Executable-based Plugins**
- Plugins as standalone executables
- JSON/YAML-based communication
- Highest compatibility but lower performance

**Recommendation**: Start with Go Plugin Architecture (Option A) for MVP, with gRPC support planned for v2.

#### Go Plugin Implementation
```go
type PluginHost struct {
    pluginDir     string
    loadedPlugins map[string]*LoadedPlugin
    logger        logger.Logger
}

type LoadedPlugin struct {
    plugin     *plugin.Plugin
    symbols    map[string]plugin.Symbol
    instance   Plugin
    config     PluginConfig
    healthTick *time.Ticker
}

func (ph *PluginHost) LoadPlugin(config PluginConfig) error {
    // 1. Download/locate plugin source
    // 2. Compile plugin if needed
    // 3. Load plugin using plugin.Open()
    // 4. Initialize plugin instance
    // 5. Register resources with Simple Container
    // 6. Start health monitoring
}
```

### 3. Configuration Format in server.yaml

```yaml
schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    organization: myorg
    stateStorage:
      type: s3-bucket
      config:
        bucketName: "${secret:state-bucket-name}"
        region: us-west-2
    secretsProvider:
      type: kms-key
      config:
        keyId: "${secret:kms-key-id}"

# New plugins section
plugins:
  - name: custom-provider
    source:
      type: git
      path: https://github.com/myorg/sc-custom-provider
      ref: v1.2.0
      checksum: sha256:abc123...
    enabled: true
    config:
      apiKey: "${secret:custom-provider-api-key}"
      endpoint: "https://api.custom-provider.com"
      
  - name: internal-tools
    source:
      type: local
      path: ./plugins/internal-tools
    enabled: true
    config:
      environment: production
      
resources:
  resources:
    production:
      custom-resource:
        type: custom-provider-resource  # Type from plugin
        name: my-custom-resource
        config:
          property1: value1
          property2: "${secret:custom-secret}"
```

### 4. Plugin Development Kit (PDK)

#### Plugin Template Structure
```
my-plugin/
├── plugin.go              # Main plugin implementation
├── resources/
│   ├── custom_resource.go  # Resource implementations
│   └── compute_processor.go # Compute processor implementations
├── schemas/
│   └── resource_schemas.go # JSON schema definitions
├── go.mod
├── go.sum
├── README.md
└── examples/
    └── server.yaml
```

#### Example Plugin Implementation
```go
package main

import (
    "context"
    "github.com/simple-container-com/api/pkg/api"
    pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
    sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type CustomPlugin struct {
    config PluginConfig
    logger logger.Logger
}

func (p *CustomPlugin) Name() string { return "custom-provider" }
func (p *CustomPlugin) Version() string { return "1.2.0" }
func (p *CustomPlugin) Description() string { return "Custom provider for XYZ service" }

func (p *CustomPlugin) RegisterResources() map[string]pApi.ProvisionFunc {
    return map[string]pApi.ProvisionFunc{
        "custom-provider-resource": p.ProvisionCustomResource,
        "custom-provider-database": p.ProvisionCustomDatabase,
    }
}

func (p *CustomPlugin) RegisterComputeProcessors() map[string]pApi.ComputeProcessorFunc {
    return map[string]pApi.ComputeProcessorFunc{
        "custom-provider-resource": p.CustomResourceComputeProcessor,
        "custom-provider-database": p.CustomDatabaseComputeProcessor,
    }
}

func (p *CustomPlugin) ProvisionCustomResource(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    // Custom resource provisioning logic using Pulumi
    // Access to all Simple Container facilities: logger, providers, registrar
    return &api.ResourceOutput{
        Ref: customResource, // Pulumi resource reference
    }, nil
}

func (p *CustomPlugin) CustomResourceComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    // Inject environment variables for applications
    collector.AddEnvVariableIfNotExist("CUSTOM_SERVICE_URL", "https://api.example.com", input.Descriptor.Type, input.Descriptor.Name, input.StackParams.StackName)
    collector.AddSecretEnvVariableIfNotExist("CUSTOM_SERVICE_TOKEN", "${secret:custom-token}", input.Descriptor.Type, input.Descriptor.Name, input.StackParams.StackName)
    
    return p.ProvisionCustomResource(ctx, stack, input, params)
}

// Plugin entry point
func NewPlugin() Plugin {
    return &CustomPlugin{}
}
```

### 5. Core Integration Points

#### 5.1 Resource Registration Integration
```go
// Extend provisioner.go to load plugins
func (p *pulumi) loadPlugins(ctx context.Context, cfg *api.ConfigFile) error {
    pluginHost := NewPluginHost(p.logger)
    
    for _, pluginConfig := range cfg.Provisioner.Plugins {
        if pluginConfig.Enabled {
            if err := pluginHost.LoadPlugin(pluginConfig); err != nil {
                return errors.Wrapf(err, "failed to load plugin %s", pluginConfig.Name)
            }
        }
    }
    
    // Register all plugin resources with existing registry
    for pluginName, plugin := range pluginHost.loadedPlugins {
        // Register resources
        pApi.RegisterResources(plugin.instance.RegisterResources())
        // Register compute processors
        pApi.RegisterComputeProcessor(plugin.instance.RegisterComputeProcessors())
        // Register providers
        for providerType, providerFunc := range plugin.instance.RegisterProviders() {
            pApi.RegisterProvider(providerType, providerFunc)
        }
    }
    
    return nil
}
```

#### 5.2 JSON Schema Generation Integration
```go
// Extend cmd/schema-gen/main.go to include plugin schemas
func (sg *SchemaGenerator) discoverPluginResources() ([]ResourceDefinition, error) {
    var pluginResources []ResourceDefinition
    
    // Load configured plugins
    pluginHost := NewPluginHost()
    for _, pluginConfig := range loadPluginConfigs() {
        plugin, err := pluginHost.LoadPlugin(pluginConfig)
        if err != nil {
            continue // Skip failed plugins in schema generation
        }
        
        // Extract schemas from plugin
        for resourceType, schema := range plugin.instance.GetResourceSchemas() {
            pluginResources = append(pluginResources, ResourceDefinition{
                Name:         schema.Name,
                Type:         resourceType,
                Provider:     plugin.config.Name,
                Description:  schema.Description,
                GoPackage:    fmt.Sprintf("plugin:%s", plugin.config.Name),
                GoStruct:     schema.StructName,
                ResourceType: resourceType,
                Schema:       schema.JSONSchema,
            })
        }
    }
    
    return pluginResources, nil
}
```

#### 5.3 Template Placeholder Support
Plugins automatically inherit Simple Container's template placeholder system:
- `${resource:plugin-resource.property}` - Access plugin resource properties
- `${dependency:plugin-service.plugin-resource.property}` - Cross-service plugin resource access
- `${secret:plugin-secret}` - Access plugin-specific secrets
- `${auth:plugin-provider}` - Access plugin authentication

#### 5.4 Compute Processor Integration
Plugin compute processors work identically to core processors:
- Automatic environment variable injection
- Support for named resource variants (e.g., `CUSTOM_DB_HOST`, `BILLING_DB_HOST`)
- Secret environment variable support
- Dependency tracking and ordering

## Implementation Phases

### Phase 1: Core Plugin Framework (4-6 weeks)
**Deliverables:**
- [ ] Plugin interface definition
- [ ] Go plugin loader implementation
- [ ] Basic plugin host with health monitoring
- [ ] server.yaml configuration parsing for plugins
- [ ] Integration with existing resource registration system
- [ ] Plugin security validation (checksums, signatures)

**Acceptance Criteria:**
- Load a simple plugin from local directory
- Register plugin resources with Simple Container
- Deploy using plugin-provided resources
- Plugin health monitoring and automatic restart

### Phase 2: Plugin Development Kit (2-3 weeks)
**Deliverables:**
- [ ] Plugin template generator (`sc plugin init`)
- [ ] Plugin build and packaging tools
- [ ] Plugin testing framework
- [ ] Documentation and examples
- [ ] Plugin validation utilities

**Acceptance Criteria:**
- Generate a working plugin template
- Build and test plugin locally
- Package plugin for distribution
- Validate plugin against Simple Container API

### Phase 3: Advanced Plugin Features (3-4 weeks)
**Deliverables:**
- [ ] Git-based plugin sources with version management
- [ ] Plugin dependency management
- [ ] Plugin configuration schema validation
- [ ] Enhanced error handling and debugging
- [ ] Plugin marketplace/registry integration

**Acceptance Criteria:**
- Load plugins from Git repositories
- Handle plugin version conflicts
- Validate plugin configurations
- Comprehensive error messages and debugging

### Phase 4: Enterprise Features (4-5 weeks)
**Deliverables:**
- [ ] Plugin sandboxing and security
- [ ] gRPC communication layer for cross-language plugins
- [ ] Plugin performance monitoring
- [ ] Plugin update and rollback mechanisms
- [ ] Enterprise plugin distribution

**Acceptance Criteria:**
- Secure plugin execution environment
- Support for non-Go plugins via gRPC
- Monitor plugin performance metrics
- Safe plugin updates with rollback capability

## Security Considerations

### Plugin Security Model
1. **Source Verification**
   - Mandatory checksums for all plugin sources
   - Optional code signing for enterprise plugins
   - Git commit verification for repository sources

2. **Runtime Sandboxing**
   - Resource usage limits (memory, CPU, disk)
   - Network access restrictions
   - File system access controls
   - Capability-based security model

3. **API Access Control**
   - Limited Simple Container API surface for plugins
   - Read-only access to sensitive configurations
   - Audit logging for all plugin operations

### Plugin Validation Pipeline
```yaml
# .sc/plugin-security.yaml
security:
  validation:
    - checksum_verification
    - code_scanning
    - dependency_audit
    - api_compatibility
  
  runtime:
    memory_limit: 256MB
    cpu_limit: 0.5
    network_policy: restricted
    file_access: plugin_directory_only
  
  audit:
    log_all_operations: true
    sensitive_data_access: deny
```

## Migration Strategy

### Existing Provider Migration
1. **AWS Provider Plugin**
   - Extract `/pkg/clouds/pulumi/aws/` into standalone plugin
   - Maintain API compatibility
   - Provide migration guide

2. **GCP Provider Plugin**
   - Extract `/pkg/clouds/pulumi/gcp/` into standalone plugin
   - Test against existing configurations
   - Document any breaking changes

3. **Kubernetes Provider Plugin**
   - Extract `/pkg/clouds/pulumi/kubernetes/` into standalone plugin
   - Ensure Helm operators continue working
   - Validate compute processor functionality

### Backward Compatibility
- Core providers remain embedded for stability
- Plugin system is opt-in initially
- Gradual migration path with deprecation notices
- Full compatibility with existing server.yaml files

## Testing Strategy

### Plugin Testing Framework
```go
type PluginTestSuite struct {
    plugin     Plugin
    testConfig PluginConfig
    pulumi     *testing.PulumiTest
}

func (pts *PluginTestSuite) TestResourceProvisioning() {
    // Test plugin resource provisioning
    resource, err := pts.plugin.ProvisionCustomResource(...)
    assert.NoError(pts.T(), err)
    assert.NotNil(pts.T(), resource.Ref)
}

func (pts *PluginTestSuite) TestComputeProcessor() {
    // Test environment variable injection
    collector := &MockComputeContextCollector{}
    _, err := pts.plugin.CustomResourceComputeProcessor(..., collector, ...)
    
    assert.NoError(pts.T(), err)
    assert.Contains(pts.T(), collector.EnvVariables(), "CUSTOM_SERVICE_URL")
}
```

### Integration Testing
- End-to-end testing with real Pulumi deployments
- Plugin compatibility testing across Simple Container versions
- Performance testing under plugin load
- Security testing with malicious plugins

## Documentation Plan

### Developer Documentation
1. **Plugin Development Guide**
   - Getting started with plugin development
   - API reference and examples
   - Best practices and patterns
   - Testing and debugging

2. **Plugin User Guide**
   - Installing and configuring plugins
   - Managing plugin versions
   - Troubleshooting common issues
   - Security considerations

3. **Migration Guides**
   - Moving from embedded providers to plugins
   - Upgrading existing configurations
   - Plugin compatibility matrix

### API Documentation
- Complete Plugin interface documentation
- Resource schema specification
- Compute processor API reference
- Configuration format documentation

## Success Metrics

### Technical Metrics
- Plugin load time < 2 seconds
- Zero performance regression in core functionality
- 100% backward compatibility with existing configurations
- Plugin memory usage < 100MB per plugin
- Plugin failure rate < 0.1%

### Developer Experience Metrics
- Plugin development time < 1 day for simple resources
- Plugin testing cycle < 10 minutes
- Documentation completeness > 95%
- Community plugin adoption > 10 plugins in first 6 months

### Business Metrics
- Reduced core maintenance burden by 40%
- Increased extensibility without core changes
- Faster integration of new cloud providers
- Enhanced enterprise customization capabilities

## Risk Mitigation

### Technical Risks
1. **Plugin Compatibility**
   - Risk: Plugin incompatibility across Simple Container versions
   - Mitigation: Semantic versioning, API stability guarantees, compatibility testing

2. **Performance Impact**
   - Risk: Plugins degrading core performance
   - Mitigation: Performance benchmarking, resource limits, lazy loading

3. **Security Vulnerabilities**
   - Risk: Malicious or vulnerable plugins
   - Mitigation: Security scanning, sandboxing, signed plugins

### Operational Risks
1. **Plugin Reliability**
   - Risk: Plugin failures affecting deployments
   - Mitigation: Health monitoring, automatic restarts, fallback mechanisms

2. **Dependency Management**
   - Risk: Plugin dependency conflicts
   - Mitigation: Isolated plugin environments, dependency resolution

## Conclusion

The Simple Container plugin system will provide a robust, secure, and developer-friendly way to extend Simple Container's capabilities without modifying the core codebase. By leveraging Go's plugin architecture and maintaining compatibility with existing Simple Container concepts, this system will enable:

- Rapid integration of new cloud providers and services
- Community-driven ecosystem development
- Enterprise customization capabilities
- Reduced maintenance burden on the core team
- Enhanced flexibility and extensibility

The phased implementation approach ensures that the plugin system is delivered incrementally with comprehensive testing and documentation, while maintaining backward compatibility and system stability.
