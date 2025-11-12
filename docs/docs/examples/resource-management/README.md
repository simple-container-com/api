# Resource Management Examples

This directory contains examples demonstrating how to configure CPU and memory resources for Kubernetes deployments using Simple Container.

## Overview

Simple Container supports comprehensive resource management with separate configuration for:
- **Resource Limits**: Maximum resources a container can use
- **Resource Requests**: Resources guaranteed to be available for the container

## Key Benefits

- **Optimal Kubernetes Scheduling**: Proper resource requests ensure containers get scheduled on nodes with adequate resources
- **Resource Efficiency**: Different limits and requests allow better cluster resource utilization
- **Backward Compatibility**: Legacy configuration continues to work seamlessly
- **Smart Defaults**: When requests aren't specified, uses 50% of limits (Kubernetes best practice)

## Examples in this Directory

- `basic-resource-config.yaml` - Simple resource configuration with explicit limits and requests
- `legacy-compatibility.yaml` - Shows how legacy configuration still works
- `mixed-configuration.yaml` - Demonstrates mixing new and legacy approaches
- `production-optimized.yaml` - Production-ready configuration with optimal resource allocation

## Configuration Priority

Simple Container uses the following priority order when determining resource values:

1. **Explicit configuration** (highest priority)
   - `size.limits.cpu` / `size.requests.cpu`
   - `size.limits.memory` / `size.requests.memory`

2. **Legacy fields**
   - `size.cpu` / `size.memory` (used as limits)

3. **Docker Compose resources**
   - `deploy.resources.limits` / `deploy.resources.reservations`

4. **Smart defaults** (lowest priority)
   - CPU limit: 256m, Memory limit: 512MB
   - Requests: 50% of limits when not specified

## Best Practices

### Resource Requests
- Set requests to the minimum resources your application needs to function
- Use requests that reflect actual resource usage during normal operation
- Avoid setting requests too high as it wastes cluster resources

### Resource Limits
- Set limits to prevent containers from consuming excessive resources
- Leave some headroom above normal usage for traffic spikes
- Consider the impact on other containers sharing the same node

### Typical Ratios
- **CPU**: Requests often 25-50% of limits
- **Memory**: Requests often 50-75% of limits (memory usage is less variable than CPU)

## Quick Start

```yaml
# client.yaml
stacks:
  production:
    type: cloud-compose
    config:
      size:
        limits:
          cpu: "2000"    # 2 CPU cores maximum
          memory: "4096" # 4GB memory maximum
        requests:
          cpu: "500"     # 0.5 CPU cores guaranteed
          memory: "2048" # 2GB memory guaranteed
```

This configuration ensures your container gets 0.5 CPU cores and 2GB memory guaranteed, but can burst up to 2 CPU cores and 4GB when resources are available.
