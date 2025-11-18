# Simple Container API - Development Instructions

## ⚠️ CRITICAL DEVELOPMENT WORKFLOW
**ALWAYS run `welder run fmt` after completing any code modifications to ensure proper formatting and linting compliance!**

## 📚 Documentation-First Approach
**When you need additional context or understanding:**
1. **Search documentation first**: Use `sc assistant search [query]` or browse `docs/docs/`
2. **Check examples**: Look in `docs/docs/examples/` for real-world patterns
3. **Review schemas**: Check `docs/schemas/` for configuration structure
4. **Consult architecture**: Understand the patterns below before implementing

## Project Overview
This is the Simple Container API project - an infrastructure-as-code platform for deploying applications across multiple cloud providers (AWS, GCP, Kubernetes, etc.). The project uses Go with Pulumi for infrastructure provisioning and includes comprehensive documentation, JSON schemas, and GitHub Actions integration.

## Essential Development Instructions

### 1. Build System (Welder)
- **Build tool**: This project uses `welder` for all build operations
- **Commands**:
  - `welder run fmt` - Format code and run linters (MANDATORY after code changes)
  - `welder run build` - Build the project
  - `welder run test` - Run tests
  - `welder run generate-schemas` - Generate JSON schemas from Go structures
  - `welder run generate-embeddings` - Generate AI assistant embeddings
- **Configuration**: `welder.yaml` in project root defines all build tasks

#### AI Assistant Commands
- **`sc assistant search [query]`** - Search documentation and examples
- **`sc assistant mcp --port 9999`** - Start MCP server for external AI tools
- **`sc assistant analyze`** - Analyze project structure (placeholder)
- **Embeddings**: Generated at build time for offline documentation search

### 2. Code Quality Requirements
- **Formatting**: ALWAYS run `welder run fmt` after any code modifications
- **Linting**: Code must pass all golangci-lint checks (configured in `.golangci.yml`)
- **Testing**: Run `go build ./...` to verify compilation
- **Imports**: Use `gofumpt` and `gci` for import organization (handled by welder fmt)

#### Linting Configuration (`.golangci.yml`)
- **Enabled linters**: govet, errcheck, ineffassign, gofumpt, gosimple, unused, staticcheck, gci
- **Import organization**: Standard → Default → GitHub → AWS → Pulumi → Simple Container
- **Commands**: `welder run fmt` (includes linting) and `welder run linters` (linting only)
- **Timeout**: 5 minutes with 4 concurrent workers

### 3. Adding New Cloud Resources
When adding support for new cloud resources:

#### Required JSON Schema Updates
- **Generated automatically**: Schemas are generated from Go structures using `welder run generate-schemas`
- **Location**: `docs/schemas/[provider]/[resource].json`
- **Providers**: `aws/`, `gcp/`, `kubernetes/`, `cloudflare/`, `mongodb/`, etc.
- **Source**: Go structures in `pkg/clouds/[provider]/` define the schema
- **Index update**: Add new schema to `docs/schemas/index.json` if needed

#### Required Documentation Updates
- **Supported Resources**: `docs/docs/reference/supported-resources.md`
- **Examples**: Create example in `docs/docs/examples/[category]/[resource]/`
- **Concepts**: Update relevant concept docs in `docs/docs/concepts/`
- **Getting Started**: Update if resource affects basic workflows

### 4. Documentation Structure
```
docs/
├── design/            # Design documents for major features and architecture decisions
│   ├── ai-assistant/
│   ├── deploy-feedback/
│   ├── resources-adoption/
│   ├── secrets-managers/
│   └── horizontal-pod-autoscaler/  # Example: HPA implementation design
└── docs/
    ├── getting-started/     # Initial setup and basic usage
    ├── concepts/           # Core concepts (stacks, resources, etc.)
    ├── guides/            # Step-by-step tutorials
    ├── examples/          # Code examples organized by category
    ├── reference/         # API reference and supported resources
    ├── advanced/          # Advanced topics
    └── ai-assistant/      # AI assistant documentation
```

**Design Document Requirements:**
- **All design documents must be placed under `docs/design/` folder**
- **Each major feature should have its own subdirectory** (e.g., `docs/design/horizontal-pod-autoscaler/`)
- **Include comprehensive documentation**: README.md, implementation phases, configuration examples, technical architecture
- **Design-first approach**: Create design documents before implementation for complex features

### 5. GitHub Actions Integration
- **Actions location**: `.github/actions/[action-name]/action.yml`
- **Workflow templates**: `pkg/clouds/github/templates.go`
- **Executor**: `pkg/githubactions/actions/`
- **Docker images**: Built via `github-actions.Dockerfile` and `github-actions-staging.Dockerfile`

### 6. Key Architecture Patterns

#### Configuration File Separation (handled via `pkg/api/`)
Simple Container uses a three-file configuration pattern:
- **`client.yaml`**: Application deployment configurations (client stacks)
- **`server.yaml`**: Infrastructure resource definitions (parent stacks)  
- **`secrets.yaml`**: Encrypted secrets and credentials
- **Profile support**: Multiple environments via `SC_PROFILE` (default, staging, prod)
- **API integration**: All configuration parsing handled through `pkg/api/` package

#### Core Architecture Components
- **API structure**: `pkg/api/` contains core types, interfaces, and configuration parsing
- **Cloud providers**: `pkg/clouds/[provider]/` for provider-specific implementations
- **Provisioner**: `pkg/provisioner/` for infrastructure operations and Pulumi integration
- **Assistant**: `pkg/assistant/` for AI assistant functionality and embeddings
- **MCP Server**: `pkg/assistant/mcp/` implements Model Context Protocol server for AI integration

#### Stack Architecture Pattern
- **Parent stacks**: Create and manage infrastructure resources (server.yaml)
- **Client stacks**: Deploy applications that consume parent resources (client.yaml)
- **Resource sharing**: Parent stack outputs become client stack environment variables
- **Separation of concerns**: Infrastructure management vs. application deployment

#### MCP (Model Context Protocol) Server
- **Purpose**: Provides JSON-RPC 2.0 interface for external AI tools (Windsurf, Cursor, etc.)
- **Command**: `sc assistant mcp --port 9999` to start the server
- **Capabilities**: Documentation search, project analysis, resource information
- **Integration**: Enables AI tools to access Simple Container context and documentation
- **Protocol**: Standards-compliant JSON-RPC 2.0 with CORS support

### 7. Testing and Validation
- **Unit tests**: Run `go test ./...`
- **Build verification**: `go build ./...`
- **Linting**: Included in `welder run fmt`
- **Schema validation**: Validate JSON schemas against examples

#### Testing Framework and Assertions
Simple Container uses **Gomega** for BDD-style assertions in unit tests:

**Required Setup**:
```go
import (
    "testing"
    . "github.com/onsi/gomega"  // Import Gomega matchers
)

func TestExample(t *testing.T) {
    RegisterTestingT(t)  // Required for Gomega integration
    // ... test code
}
```

**Table-Driven Test Pattern** (preferred approach):
```go
tests := []struct {
    name     string
    input    SomeType
    validate func(original, result SomeType)
}{
    {
        name: "descriptive test case name",
        input: SomeType{Field: "value"},
        validate: func(original, result SomeType) {
            Expect(result.Field).To(Equal(original.Field))
        },
    },
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        result := functionUnderTest(tt.input)
        tt.validate(tt.input, result)
    })
}
```

**Common Gomega Matchers**:
- **Equality**: `Expect(actual).To(Equal(expected))`
- **Nil checks**: `Expect(value).To(BeNil())` / `Expect(value).ToNot(BeNil())`
- **Identity**: `Expect(obj1).ToNot(BeIdenticalTo(obj2))` (different memory addresses)
- **Length**: `Expect(slice).To(HaveLen(3))`
- **Boolean**: `Expect(condition).To(BeTrue())` / `Expect(condition).To(BeFalse())`
- **Negation**: Use `ToNot()` instead of `To()` for negative assertions

**YAML Serialization Testing Notes**:
When testing `MustClone` or similar functions that use YAML marshaling, be aware of type conversions:
- `struct` → `map[string]interface{}`
- `[]string` → `[]interface{}`
- `map[string]string` → `map[string]interface{}`

Handle these conversions in test validations:
```go
// Instead of direct comparison
clone := cloned.(map[string]interface{})
clonedTags := clone["tags"].([]interface{})
for i, tag := range original.Tags {
    Expect(clonedTags[i]).To(Equal(tag))
}
```

### 8. Common Development Tasks

#### Adding a New Resource Type
1. Add implementation in `pkg/clouds/[provider]/` with proper Go structures
2. Register resource in `pkg/clouds/[provider]/init.go` for config reading
3. Register provisioning functions in `pkg/clouds/pulumi/[provider]/init.go` for Pulumi operations
4. Add compute processor if resource provides data to client applications (see Compute Processors)
5. Regenerate JSON schemas with `welder run generate-schemas`
6. Update `docs/schemas/index.json` if needed
7. Update `docs/docs/reference/supported-resources.md`
8. Create example in `docs/docs/examples/`
9. Run `welder run fmt`
10. Test with `go build ./...`

#### Updating Documentation
1. Edit relevant files in `docs/docs/`
2. Update examples if API changes
3. Verify links and references
4. Run documentation build locally if needed

#### Modifying GitHub Actions
1. Update action definitions in `.github/actions/`
2. Modify templates in `pkg/clouds/github/templates.go`
3. Update executor logic in `pkg/githubactions/actions/`
4. Test workflow generation with `sc cicd generate`
5. Run `welder run fmt`

### 9. Cloud Resource Registration System
Simple Container uses a registration-based system for cloud resources that requires updates in two locations:

#### Config Reading Registration (`pkg/clouds/[provider]/init.go`)
Register functions for reading and validating resource configurations:
```go
api.RegisterProviderConfig(api.ConfigRegisterMap{
    ResourceTypeNewResource: ReadNewResourceConfig,
})
```

#### Pulumi Provisioning Registration (`pkg/clouds/pulumi/[provider]/init.go`)
Register functions for actual infrastructure provisioning:
```go
api.RegisterResources(map[string]api.ProvisionFunc{
    gcloud.ResourceTypeNewResource: NewResourceProvisionFunc,
})
api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
    gcloud.ResourceTypeNewResource: NewResourceComputeProcessor,
})
```

#### Registration Types
- **RegisterProviderConfig**: Maps resource types to config reading functions
- **RegisterResources**: Maps resource types to Pulumi provisioning functions  
- **RegisterComputeProcessor**: Maps resource types to compute context processors (see Compute Processors below)
- **RegisterCloudComposeConverter**: Maps template types to docker-compose converters
- **RegisterCloudStaticSiteConverter**: Maps template types to static site converters

#### Compute Processors Concept
Compute processors handle the integration between parent stack resources and client stack applications:

**Purpose**: Extract outputs from parent stack resources and make them available to client applications as environment variables, secrets, and configuration.

**Two-Phase Architecture**:
1. **Provisioning Phase** (`ProvisionFunc`): Creates infrastructure resources in parent stacks
2. **Compute Phase** (`ComputeProcessorFunc`): Retrieves resource outputs and injects them into client applications

**Example Flow**:
```
Parent Stack: Creates GCS bucket with HMAC keys
    ↓ (exports: bucket name, access keys, location)
Compute Processor: Reads parent outputs via StackReference
    ↓ (transforms to environment variables)
Client Stack: Receives GCS_BUCKET_NAME, GCS_ACCESS_KEY, etc.
```

**Key Functions**:
- **StackReference**: Links client stack to parent stack outputs
- **AddEnvVariableIfNotExist**: Adds regular environment variables
- **AddSecretEnvVariableIfNotExist**: Adds sensitive environment variables
- **GetParentOutput**: Retrieves specific outputs from parent stack

### 10. Critical Implementation Notes
- **Documentation first**: Always consult docs/examples before implementing new features
- **Configuration separation**: Understand client.yaml vs server.yaml vs secrets.yaml patterns
- **API package centrality**: All configuration parsing goes through `pkg/api/` - never bypass it
- **Parent-client architecture**: Parent stacks create resources, client stacks consume them via compute processors
- **Registration required**: All new resources MUST be registered in both config and Pulumi init files
- **Compute processors**: Resources that need to provide data to client applications require compute processors
- **Panic recovery**: All GitHub Actions operations have comprehensive panic recovery
- **Context handling**: Use `context.WithoutCancel()` for cancellation operations
- **Resource naming**: Kubernetes resources must follow RFC 1123 naming (use sanitization)
- **Placeholder parsing**: Validate bounds for `${dependency:name.resource.property}` patterns
- **Notification system**: Integrate with existing Slack/Discord/Telegram alert system

### 11. VPA (Vertical Pod Autoscaler) Support
- **Application VPA**: Configure via `cloudExtras.vpa` in client.yaml for automatic resource optimization
- **Infrastructure VPA**: Configure via resource config (e.g., `caddy.vpa`) in server.yaml for infrastructure components
- **Update modes**: Off (recommendations only), Initial (pod creation), Recreation (pod restart), Auto (in-place)
- **Resource boundaries**: Always set `minAllowed` and `maxAllowed` to prevent resource starvation or runaway costs
- **Documentation**: VPA concepts in `docs/docs/concepts/vertical-pod-autoscaler.md`, examples in `docs/docs/examples/kubernetes-vpa/`

### 12. Memory Management
- **Create memories**: Use `create_memory` tool to preserve important context
- **Update SYSTEM_PROMPT.md**: Add new essential instructions when patterns emerge
- **Keep instructions current**: Remove outdated information, focus on actionable guidance
