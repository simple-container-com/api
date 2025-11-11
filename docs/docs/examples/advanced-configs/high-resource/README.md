# High-Resource AI Development Environment Example

This example shows how to deploy a high-resource code execution environment with AI-powered development tools and dynamic Kubernetes pod management.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Parent**: `mycompany`
- **High Resources**: 32GB memory, 16 CPU for code execution environments
- **AI Integration**: Claude Sonnet 4 model with LLM proxy service
- **Dynamic Pods**: 10-minute timeout, 10Gi volume, ephemeral storage

## Key Features

- **High-Resource Runtime**: 32GB memory, 16 CPU for intensive code execution
- **Dynamic Pod Management**: 10-minute timeout, 10Gi volume, ephemeral storage
- **AI Integration**: Claude Sonnet 4 model with LLM proxy service
- **GitHub OAuth**: Complete OAuth flow with redirect URI
- **Docker Registry**: Private registry with authentication
- **Kubernetes Integration**: Direct kubeconfig and namespace management
- **Hardcoded Cluster IP**: External cluster reference

## Resource Configuration

### Service Resources (Allocated to the Service)
- **CPU**: 16384 (16 CPU cores allocated to the service)
- **Memory**: 32768MB (32GB RAM allocated to the service)
- **Ephemeral Storage**: 42949672960 bytes (40GB ephemeral storage)

### Runtime Environment Variables (Configuration for Dynamic Pods)
- **RUNTIME_MEMORY_LIMIT/REQUEST**: "32Gi" - Configuration for pods the service creates
- **RUNTIME_CPU_LIMIT/REQUEST**: "16" - Configuration for pods the service creates
- **RUNTIME_EPHEMERAL_STORAGE_LIMIT**: "10Gi" - Storage for dynamic pods
- **RUNTIME_VOLUME_SIZE**: 10Gi - Persistent storage per dynamic pod
- **RUNTIME_POD_START_TIMEOUT**: 10 minutes for pod startup

### AI Configuration
- **Model**: Claude Sonnet 4 (`claude-sonnet-4-20250514`)
- **LLM Proxy**: `https://llm-proxy.example.com/v1`
- **Debug Mode**: Disabled in cluster environment

## Environments

- **Staging**: `code.example.com`
- **Production**: Same configuration (inherited)

## Use Cases

- **AI-Powered Development**: Code generation and analysis with Claude Sonnet 4
- **High-Performance Computing**: Resource-intensive code execution
- **Dynamic Environments**: On-demand development environments
- **Container Development**: Docker-based development workflows
- **GitHub Integration**: OAuth-based repository access

## Usage

1. Ensure external Kubernetes cluster is available
2. Configure GitHub OAuth application
3. Set up private Docker registry credentials
4. Configure LLM proxy service
5. Deploy with high-resource allocation

## Parent Stack Requirements

This example requires a parent stack that provides:
- ECS deployment with high-resource support
- External Kubernetes cluster integration
- Secrets management for OAuth and registry credentials
- Domain management for OAuth redirects

## Performance Considerations

- **High Cost**: 32GB/16CPU allocation is expensive
- **Resource Optimization**: Consider auto-scaling based on usage
- **Pod Lifecycle**: 10-minute timeout for cost control
- **Storage Management**: Ephemeral storage cleanup after execution
