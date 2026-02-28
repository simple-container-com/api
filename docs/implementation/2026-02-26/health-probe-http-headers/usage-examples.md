# HTTP Headers Support for Health Probes - Usage Examples

**Date:** 2026-02-26
**Issue:** #171

This document provides practical examples for using HTTP headers in health probe configurations.

## Table of Contents

1. [Basic Examples](#basic-examples)
2. [Authentication Headers](#authentication-headers)
3. [Multi-Tenant Applications](#multi-tenant-applications)
4. [API Version Headers](#api-version-headers)
5. [Custom Request Identifiers](#custom-request-identifiers)
6. [Global vs Container-Level Probes](#global-vs-container-level-probes)
7. [Environment Variable Integration](#environment-variable-integration)

## Basic Examples

### Simple Health Check Header

Add a custom header to identify health check requests:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### Multiple Headers

Add multiple custom headers:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
      - name: X-Environment
        value: "production"
      - name: X-Service
        value: "api-gateway"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Authentication Headers

### Bearer Token Authentication

Use a bearer token for authenticated health checks:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: Authorization
        value: "Bearer your-health-check-token-here"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### API Key Authentication

Use an API key for health checks:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-API-Key
        value: "your-api-key-here"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### Basic Authentication

Use basic authentication (not recommended for production):

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: Authorization
        value: "Basic dXNlcm5hbWU6cGFzc3dvcmQ="
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Multi-Tenant Applications

### Tenant Identification

Identify the tenant in health check requests:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Tenant-ID
        value: "tenant-123"
      - name: X-Tenant-Environment
        value: "production"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### Host-Based Routing

Specify the host for routing:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: Host
        value: "api.example.com"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## API Version Headers

### Version-Specific Health Check

Specify the API version for the health check:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-API-Version
        value: "v1"
      - name: Accept
        value: "application/json"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Custom Request Identifiers

### Request Tracking

Add a custom identifier for probe requests:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Request-ID
        value: "kube-probe-readiness"
      - name: X-Request-Source
        value: "kubernetes-liveness-probe"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Global vs Container-Level Probes

### Global Probe Configuration

Apply headers to all containers using the global probe configuration:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Global-Health-Check
        value: "true"
  initialDelaySeconds: 10
  periodSeconds: 5

containers:
  - name: web-service
    # Inherits global readinessProbe with headers

  - name: api-service
    # Inherits global readinessProbe with headers
```

### Container-Level Override

Override global probe configuration for a specific container:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Global-Health-Check
        value: "true"
  initialDelaySeconds: 10
  periodSeconds: 5

containers:
  - name: web-service
    # Inherits global readinessProbe

  - name: api-service
    readinessProbe:
      httpGet:
        path: /custom/health
        port: 8080
        httpHeaders:
          - name: X-Custom-Health-Check
            value: "true"
          - name: X-API-Service
            value: "specific"
      initialDelaySeconds: 15
      periodSeconds: 10
```

### Different Probes for Different Containers

Configure different probes for different containers:

```yaml
# client.yaml
containers:
  - name: web-service
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
        httpHeaders:
          - name: X-Service
            value: "web"
      initialDelaySeconds: 10
      periodSeconds: 5

  - name: api-service
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
        httpHeaders:
          - name: X-Service
            value: "api"
      initialDelaySeconds: 10
      periodSeconds: 5
```

## Environment Variable Integration

### Using Environment Variables

While the current implementation doesn't support inline environment variable expansion, you can use shell expansion:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Environment
        value: "${ENVIRONMENT}"
  initialDelaySeconds: 10
  periodSeconds: 5
```

Then deploy with:

```bash
export ENVIRONMENT=production
kubectl apply -f client.yaml
```

## Advanced Scenarios

### Conditional Headers Based on Environment

Different headers for different environments:

**Production (client-prod.yaml):**
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Environment
        value: "production"
      - name: X-Auth-Token
        value: "${PROD_HEALTH_TOKEN}"
  initialDelaySeconds: 10
  periodSeconds: 5
```

**Development (client-dev.yaml):**
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Environment
        value: "development"
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Liveness vs Readiness vs Startup Probes

Different headers for different probe types:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
    httpHeaders:
      - name: X-Probe-Type
        value: "readiness"
      - name: X-Check-Dependencies
        value: "true"
  initialDelaySeconds: 10
  periodSeconds: 5

livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
    httpHeaders:
      - name: X-Probe-Type
        value: "liveness"
  initialDelaySeconds: 30
  periodSeconds: 10
  failureThreshold: 3

startupProbe:
  httpGet:
    path: /health/startup
    port: 8080
    httpHeaders:
      - name: X-Probe-Type
        value: "startup"
  initialDelaySeconds: 0
  periodSeconds: 5
  failureThreshold: 30
```

## Troubleshooting Examples

### Debug Headers

Add headers to help debug probe issues:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Debug
        value: "true"
      - name: X-Request-ID
        value: "kube-probe-debug"
      - name: X-Verbose
        value: "1"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### Test Different Endpoints

Use headers to test different service endpoints:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Test-Endpoint
        value: "database"
      - name: X-Timeout
        value: "5000"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Best Practices

### 1. Use Descriptive Header Names

```yaml
# Good
httpHeaders:
  - name: X-Health-Check-Token
    value: "abc123"

# Avoid
httpHeaders:
  - name: X-Token
    value: "abc123"
```

### 2. Keep Headers Simple

```yaml
# Good - simple string values
httpHeaders:
  - name: X-Environment
    value: "production"

# Avoid - complex values that require parsing
httpHeaders:
  - name: X-Config
    value: "{\"env\":\"prod\",\"region\":\"us-east-1\"}"
```

### 3. Document Custom Headers

Add comments to explain the purpose of custom headers:

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      # Identify this as a health check request (vs user traffic)
      - name: X-Health-Check
        value: "true"
      # Provide authentication for health check endpoint
      - name: X-API-Key
        value: "${HEALTH_CHECK_API_KEY}"
  initialDelaySeconds: 10
  periodSeconds: 5
```

### 4. Use Environment-Specific Configurations

Separate configurations for different environments:

```bash
# Production
kubectl apply -f client-prod.yaml

# Staging
kubectl apply -f client-staging.yaml

# Development
kubectl apply -f client-dev.yaml
```

## Testing Your Configuration

After adding headers to your probe configuration, verify the deployment:

```bash
# Apply the configuration
kubectl apply -f client.yaml

# Check the pod status
kubectl get pods

# Describe the pod to see probe configuration
kubectl describe pod <pod-name>

# Check logs for probe execution
kubectl logs <pod-name>
```

## Common Issues and Solutions

### Issue: Headers Not Being Sent

**Symptom:** Probe failing without headers

**Solution:**
1. Verify the YAML syntax is correct
2. Ensure the probe is using HTTP (not TCP)
3. Check that the `path` field is specified

### Issue: Invalid Header Names

**Symptom:** Pod fails to start

**Solution:**
Use valid HTTP header names (alphanumeric with hyphens):
```yaml
# Valid
- name: X-Custom-Header

# Invalid
- name: "X Custom Header"  # spaces not allowed
- name: "X:Custom:Header"  # colons not allowed in name
```

### Issue: Headers Too Long

**Symptom:** Probe failures or slow responses

**Solution:**
Keep header values concise. Consider using references instead of long values:
```yaml
# Instead of long tokens
- name: Authorization
  value: "Bearer very-long-token-here"

# Use shorter identifiers or references
- name: X-Auth-Reference
  value: "token-123"
```

## Additional Resources

- [Kubernetes Probes Documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
- [HTTP Header Field Names (RFC 7230)](https://datatracker.ietf.org/doc/html/rfc7230#section-3.2)
- [Design Document](../design/2026-02-26/health-probe-http-headers/design.md)
- [Implementation Notes](implementation-notes.md)
