# Kubernetes Custom Stacks - Design Summary

## Overview

This design enables `parentEnv` support in Kubernetes deployments, allowing multiple stack environments to coexist in the same namespace with proper resource isolation and independent routing.

## Problem Solved

**Current Issue**: When using `parentEnv` in `client.yaml`, deployments fail due to resource name conflicts in Kubernetes namespaces.

**Solution**: Deploy custom stacks to their parent's namespace with environment-specific resource naming and independent routing.

## Key Design Principles

### 1. **Namespace Sharing**
- Custom stacks deploy to their parent environment's namespace
- Reduces namespace proliferation and simplifies RBAC management
- Enables resource sharing where appropriate

### 2. **Resource Isolation**
- Each custom stack gets uniquely named resources with environment suffixes
- Independent ConfigMaps, Secrets, HPAs, and VPAs
- No conflicts with parent or sibling deployments

### 3. **Independent Routing**
- Each custom stack has its own domain and Caddy annotations
- Traffic routing is completely isolated between environments
- Supports preview environments and A/B testing scenarios

### 4. **Backward Compatibility**
- Existing deployments without `parentEnv` work unchanged
- No breaking changes to current Simple Container behavior
- Gradual adoption possible

## Configuration Structure

### Simple ParentEnv Usage
```yaml
# client.yaml
stacks:
  # Parent environment (standard behavior)
  staging:
    type: single-image
    config:
      image: "gcr.io/project/app:staging"
      domain: "staging.myapp.com"
  
  # Custom stack (new behavior)
  staging-preview:
    type: single-image
    parentEnv: staging  # Deploy to staging namespace
    config:
      image: "gcr.io/project/app:pr-123"
      domain: "pr-123.staging.myapp.com"
```

### Resource Naming Strategy
```
Namespace: staging
├── Deployment: myapp (parent)
├── Deployment: myapp-staging-preview (custom)
├── Service: myapp (parent)
├── Service: myapp-staging-preview (custom)
├── ConfigMap: myapp-config (parent)
├── ConfigMap: myapp-staging-preview-config (custom)
└── ... (all other resources follow same pattern)
```

## Implementation Highlights

### 1. **Environment Context Resolution**
```go
type EnvironmentContext struct {
    StackName       string  // "staging-preview"
    ParentEnv       string  // "staging"
    Namespace       string  // "staging"
    ResourceSuffix  string  // "staging-preview"
    IsCustomStack   bool    // true
}
```

### 2. **Resource Naming Logic**
```go
func (r *ResourceNamer) DeploymentName() string {
    if r.Environment.IsCustomStack {
        return fmt.Sprintf("%s-%s", r.ServiceName, r.Environment.ResourceSuffix)
    }
    return r.ServiceName
}
```

### 3. **Label and Selector Strategy**
```yaml
# Labels (for identification)
labels:
  app: myapp
  environment: staging-preview
  simplecontainer.com/parent-env: staging
  simplecontainer.com/custom-stack: "true"

# Selectors (for routing)
selector:
  app: myapp
  environment: staging-preview  # Unique per deployment
```

### 4. **Independent Routing**
```yaml
# Parent service
metadata:
  annotations:
    caddy.ingress.kubernetes.io/domain: "staging.myapp.com"

# Custom stack service
metadata:
  annotations:
    caddy.ingress.kubernetes.io/domain: "pr-123.staging.myapp.com"
```

## Use Cases Enabled

### 1. **Preview Environments**
- Deploy PR previews alongside staging
- Independent domains for each preview
- Isolated configuration and secrets

### 2. **A/B Testing**
- Run multiple variants in same namespace
- Different feature flags and configurations
- Independent scaling and monitoring

### 3. **Hotfix Testing**
- Test critical fixes alongside production
- Minimal resource overhead
- Quick deployment and rollback

### 4. **Feature Branch Development**
- Long-running feature development environments
- Share infrastructure with main environments
- Independent lifecycle management

## Benefits

### For Development Teams
- **Faster Feedback**: Quick preview environment deployment
- **Cost Efficiency**: Share namespace resources and quotas
- **Simplified Workflow**: Same RBAC and network policies
- **Isolated Testing**: No interference with parent environments

### For Operations Teams
- **Reduced Complexity**: Fewer namespaces to manage
- **Consistent Monitoring**: Related environments in same namespace
- **Simplified RBAC**: Single permission model per namespace
- **Resource Efficiency**: Better resource utilization

### For CI/CD Pipelines
- **Automated Previews**: Easy integration with PR workflows
- **Quick Cleanup**: Simple resource deletion
- **Parallel Deployments**: Multiple environments can deploy simultaneously
- **Consistent Patterns**: Same deployment logic for all environments

## Technical Validation

### Configuration Validation
- **Circular Reference Detection**: Prevents infinite parentEnv loops
- **Missing Parent Validation**: Ensures parent environments exist
- **Domain Conflict Detection**: Prevents domain collisions in same namespace
- **Resource Name Validation**: Ensures unique resource names

### Resource Isolation
- **Independent Scaling**: Each deployment has its own HPA/VPA
- **Separate Configuration**: Isolated ConfigMaps and Secrets
- **Unique Selectors**: No traffic cross-contamination
- **Proper Labeling**: Clear resource identification and grouping

## Migration Path

### Phase 1: Core Implementation
1. Environment context resolution
2. Resource naming strategy
3. Basic Kubernetes resource generation
4. Configuration validation

### Phase 2: Advanced Features
1. Enhanced monitoring and observability
2. Automatic cleanup mechanisms
3. CI/CD integration examples
4. Advanced routing configurations

### Phase 3: Optimization
1. Resource quota management per environment
2. Advanced conflict detection
3. Performance optimizations
4. Enhanced debugging tools

## Limitations and Considerations

### Current Limitations
- **Namespace Quotas**: All deployments share namespace resource limits
- **Network Policies**: May need adjustment for proper traffic isolation
- **Monitoring Complexity**: Need environment-specific metric labeling

### Best Practices
- **Clear Naming**: Use descriptive environment names
- **Resource Limits**: Set appropriate limits per deployment
- **Domain Strategy**: Use consistent domain naming patterns
- **Cleanup Policies**: Implement automatic cleanup for temporary environments

## Success Metrics

### Technical Metrics
- **Zero Resource Conflicts**: No deployment failures due to naming conflicts
- **Independent Scaling**: Each environment scales independently
- **Proper Isolation**: No cross-environment traffic or configuration leakage
- **Backward Compatibility**: Existing deployments remain unaffected

### User Experience Metrics
- **Deployment Speed**: Fast preview environment creation
- **Developer Productivity**: Easier testing and validation workflows
- **Operational Efficiency**: Reduced namespace management overhead
- **Cost Optimization**: Better resource utilization

## Future Enhancements

### Advanced Routing
- Path-based routing in addition to domain-based
- Traffic splitting between environments
- Canary deployment support

### Automation
- Automatic preview environment creation from Git branches
- Scheduled cleanup of stale environments
- Integration with external systems (Slack, JIRA, etc.)

### Monitoring
- Environment-specific dashboards
- Cross-environment comparison tools
- Resource usage analytics per environment

### Security
- Environment-specific security policies
- Isolated secret management
- Enhanced RBAC for custom stacks

## Conclusion

This design provides a **Simple Container way** to support complex deployment scenarios:
- **Minimal Configuration**: Just add `parentEnv: staging`
- **Maximum Flexibility**: Support for preview, testing, and development workflows
- **Zero Conflicts**: Proper resource isolation and naming
- **Operational Simplicity**: Fewer namespaces, consistent patterns

The implementation maintains Simple Container's core philosophy of simplicity while enabling advanced deployment patterns that modern development teams need.
