# HTTP Headers Support in Health Probe Configuration

**Date:** 2026-02-26
**Issue:** #168
**Status:** Design
**Author:** System Architect

## Overview

This design document outlines the architecture for adding HTTP headers support to health probe configurations (readinessProbe, livenessProbe, and startupProbe) in the simple-container-com/api platform. Currently, health probes support basic HTTP GET requests with path and port configuration, but do not support custom HTTP headers.

## Problem Statement

The current health probe implementation in `pkg/clouds/k8s/types.go` defines `ProbeHttpGet` with only `path` and `port` fields:

```go
type ProbeHttpGet struct {
    Path string `json:"path" yaml:"path"`
    Port int    `json:"port" yaml:"port"`
}
```

When converted to Kubernetes probes in `pkg/clouds/pulumi/kubernetes/deployment.go`, the HTTPGetActionArgs are created without any HTTP headers:

```go
probeArgs.HttpGet = &corev1.HTTPGetActionArgs{
    Path: sdk.String(probe.HttpGet.Path),
    Port: sdk.Int(probePort),
}
```

This limitation prevents users from configuring health checks that require custom headers, such as:
- Authentication tokens
- Host headers
- Custom request identifiers
- API version headers
- Multi-tenant routing headers

## Design Goals

1. **Backward Compatibility**: Existing configurations without httpHeaders must continue to work without any changes
2. **Consistency**: The implementation should follow Kubernetes conventions for HTTP headers in probes
3. **Simplicity**: The API should be straightforward and easy to use
4. **Type Safety**: Leverage Go's type system for compile-time validation
5. **Documentation**: Provide clear examples and usage guidelines

## Proposed Solution

### 1. Data Model Changes

#### Add HTTPHeader Type

Create a new type to represent an HTTP header with proper validation:

```go
// HTTPHeader represents a single HTTP header for health probes
type HTTPHeader struct {
    Name  string `json:"name" yaml:"name"`
    Value string `json:"value" yaml:"value"`
}
```

**Location:** `pkg/clouds/k8s/types.go`

**Rationale:**
- Structured type provides better type safety than map[string]string
- Easier to validate and enforce constraints (e.g., no empty names)
- Consistent with Kubernetes HTTPHeader structure
- Allows for future extensions (e.g., sensitive value handling)

#### Update ProbeHttpGet Structure

Extend the existing `ProbeHttpGet` to include an optional `HTTPHeaders` field:

```go
type ProbeHttpGet struct {
    Path         string       `json:"path" yaml:"path"`
    Port         int          `json:"port" yaml:"port"`
    HTTPHeaders  []HTTPHeader `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}
```

**Location:** `pkg/clouds/k8s/types.go:110-113`

**Key Design Decisions:**
- Using slice instead of map: Preserves header order and allows duplicate header names (valid in HTTP)
- `omitempty` tag: Ensures backward compatibility with existing YAML configs
- Optional field: Headers are not required for basic health checks

### 2. Kubernetes Integration

#### Update Probe Conversion Function

Modify the `toProbeArgs` function in `pkg/clouds/pulumi/kubernetes/deployment.go` to convert HTTP headers to the Kubernetes format:

```go
func toProbeArgs(c *ContainerImage, probe *k8s.CloudRunProbe) *corev1.ProbeArgs {
    // ... existing port resolution logic ...

    probeArgs := &corev1.ProbeArgs{
        PeriodSeconds:       sdk.IntPtrFromPtr(...),
        InitialDelaySeconds: sdk.IntPtrFromPtr(probe.InitialDelaySeconds),
        FailureThreshold:    sdk.IntPtrFromPtr(probe.FailureThreshold),
        SuccessThreshold:    sdk.IntPtrFromPtr(probe.SuccessThreshold),
        TimeoutSeconds:      sdk.IntPtrFromPtr(probe.TimeoutSeconds),
    }

    // Use HttpGet probe if path is specified
    if probe.HttpGet.Path != "" {
        httpGetArgs := &corev1.HTTPGetActionArgs{
            Path: sdk.String(probe.HttpGet.Path),
            Port: sdk.Int(probePort),
        }

        // Add HTTP headers if specified
        if len(probe.HttpGet.HTTPHeaders) > 0 {
            httpHeaders := make(corev1.HTTPHeaderArray, 0, len(probe.HttpGet.HTTPHeaders))
            for _, header := range probe.HttpGet.HTTPHeaders {
                httpHeaders = append(httpHeaders, corev1.HTTPHeaderArgs{
                    Name:  sdk.String(header.Name),
                    Value: sdk.String(header.Value),
                })
            }
            httpGetArgs.HTTPHeaders = httpHeaders
        }

        probeArgs.HttpGet = httpGetArgs
    } else {
        probeArgs.TcpSocket = corev1.TCPSocketActionArgs{
            Port: sdk.String(toPortName(probePort)),
        }
    }

    return probeArgs
}
```

**Location:** `pkg/clouds/pulumi/kubernetes/deployment.go:279-314`

**Implementation Details:**
- Convert `[]HTTPHeader` to `corev1.HTTPHeaderArray`
- Use Pulumi's `sdk.String()` for pointer conversion
- Only set HTTPHeaders field if non-empty (preserves current behavior when headers not specified)
- Maintain existing TCP fallback logic

### 3. Configuration Examples

#### YAML Configuration

```yaml
# client.yaml
runs:
  - web-service

readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
      - name: Authorization
        value: "Bearer ${HEALTH_CHECK_TOKEN}"  # Can reference secrets
  initialDelaySeconds: 10
  periodSeconds: 5

livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
    httpHeaders:
      - name: X-Liveness-Probe
        value: "kube-probe"
  failureThreshold: 3
```

#### Container-Level Configuration

```yaml
# docker-compose.yaml
services:
  web-service:
    labels:
      - "com.simple-container.readinessProbe.path=/health"
      - "com.simple-container.readinessProbe.port=8080"
      - "com.simple-container.readinessProbe.httpHeaders[0].name=X-Health-Check"
      - "com.simple-container.readinessProbe.httpHeaders[0].value=true"
```

#### Global vs. Container-Level

```yaml
# client.yaml - Global probe applied to ingress container
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-GLOBAL-PROBE
        value: "true"

containers:
  - name: web-service
    readinessProbe:
      httpGet:
        path: /custom/health  # Override global path
        port: 8080
        # Inherits global headers, can add more or override
```

### 4. Validation Rules

#### HTTPHeader Validation

The implementation should enforce the following validation rules:

1. **Header Name Requirements:**
   - Must not be empty
   - Must be valid HTTP header name (RFC 7230)
   - Case-insensitive (store as provided, Kubernetes normalizes)
   - Recommended: alphanumeric with hyphens

2. **Header Value Requirements:**
   - Can be empty (some headers don't require values)
   - Must not contain newlines or carriage returns
   - Should support environment variable expansion

3. **Security Considerations:**
   - Log warnings for sensitive headers (Authorization, Cookie, etc.)
   - Document that secret values should use Kubernetes secrets
   - Support secret references via placeholder syntax

#### Validation Implementation

Add validation in the type conversion layer:

```go
func (h *HTTPHeader) Validate() error {
    if h.Name == "" {
        return errors.New("header name cannot be empty")
    }
    if strings.ContainsAny(h.Name, "\r\n") {
        return errors.New("header name cannot contain newlines")
    }
    if strings.ContainsAny(h.Value, "\r\n") {
        return errors.New("header value cannot contain newlines")
    }
    return nil
}
```

### 5. Dependencies and Integration Points

#### Affected Components

1. **Type Definitions** (`pkg/clouds/k8s/types.go`)
   - Add `HTTPHeader` type
   - Update `ProbeHttpGet` struct

2. **Kubernetes Deployment** (`pkg/clouds/pulumi/kubernetes/deployment.go`)
   - Update `toProbeArgs()` function
   - Import Pulumi corev1 types (already imported)

3. **Schema Documentation** (`docs/schemas/`)
   - Update JSON schemas for probe configurations
   - Document new fields in API reference

4. **User Documentation**
   - Add examples to health probe guides
   - Document best practices for header usage

#### No Changes Required

- ECS Fargate provider (uses different probe mechanism)
- Docker Compose conversion layer (health check is separate)
- Service discovery and routing components
- Ingress configuration

### 6. Migration Path

#### Backward Compatibility

The design maintains 100% backward compatibility:

1. Existing configurations without `httpHeaders` field continue to work
2. `omitempty` tags ensure null fields are not serialized
3. No changes to default behavior
4. No breaking API changes

#### Migration Strategy

No migration required - the feature is purely additive. Users can adopt incrementally:

```yaml
# Stage 1: No headers (current state)
readinessProbe:
  httpGet:
    path: /health
    port: 8080

# Stage 2: Add headers to existing probes
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
```

### 7. Testing Strategy

#### Unit Tests Required

1. **Type Validation Tests**
   - Test HTTPHeader validation logic
   - Test edge cases (empty names, special characters)
   - Test header value constraints

2. **Conversion Tests**
   - Test conversion from CloudRunProbe to Kubernetes ProbeArgs
   - Test with and without headers
   - Test multiple headers
   - Test header order preservation

3. **Integration Tests**
   - Test end-to-end probe configuration with headers
   - Test that probes are properly applied to pods
   - Test header injection in actual HTTP requests

#### Test Example Structure

```go
func TestToProbeArgsWithHeaders(t *testing.T) {
    container := &ContainerImage{...}
    probe := &k8s.CloudRunProbe{
        HttpGet: k8s.ProbeHttpGet{
            Path: "/health",
            Port: 8080,
            HTTPHeaders: []k8s.HTTPHeader{
                {Name: "X-Auth", Value: "token123"},
                {Name: "X-Custom", Value: "value"},
            },
        },
    }

    result := toProbeArgs(container, probe)

    assert.NotNil(t, result.HttpGet)
    assert.Len(t, result.HttpGet.HTTPHeaders, 2)
    assert.Equal(t, "X-Auth", *result.HttpGet.HTTPHeaders[0].Name)
    assert.Equal(t, "token123", *result.HttpGet.HTTPHeaders[0].Value)
}
```

### 8. Documentation Requirements

#### API Documentation

Update the following documentation:

1. **API Reference** (`docs/docs/reference/`)
   - Document `ProbeHttpGet` structure
   - Document `HTTPHeader` type
   - Provide JSON schema examples

2. **User Guides** (`docs/docs/guides/`)
   - Add health probe configuration guide
   - Include header usage examples
   - Document common use cases

3. **Examples** (`docs/docs/examples/`)
   - Add example with authenticated health checks
   - Add example with host-based routing
   - Add example with custom probe headers

#### Code Comments

Add comprehensive Go documentation:

```go
// HTTPHeader represents an HTTP header name-value pair for health probe requests.
// This allows customizing HTTP headers sent in readiness, liveness, and startup probes.
//
// Example:
//
//    HTTPHeader{
//        Name:  "Authorization",
//        Value: "Bearer token123",
//    }
//
// Kubernetes Reference: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
type HTTPHeader struct {
    // Name is the header field name (case-insensitive per HTTP spec)
    Name string `json:"name" yaml:"name"`
    // Value is the header field value
    Value string `json:"value" yaml:"value"`
}
```

### 9. Future Enhancements

Out of scope for this implementation but worth noting:

1. **Secret References**
   - Support for referencing Kubernetes secrets in header values
   - Example: `value: ${secret:my-secret/token}`

2. **Dynamic Headers**
   - Support for placeholder expansion (pod name, namespace, etc.)
   - Example: `value: ${POD_NAME}`

3. **Header Templates**
   - Predefined header sets for common scenarios
   - Example: "oauth2-health-check" template

4. **Header Validation**
   - Stricter validation of header names per RFC 7230
   - Warning on deprecated headers

5. **Probe-Level Defaults**
   - Global default headers for all probes
   - Namespace-level header policies

### 10. Open Questions

None identified - the design is straightforward and aligns with Kubernetes conventions.

## Acceptance Criteria

The implementation will be considered complete when:

1. [ ] `HTTPHeader` type is defined in `pkg/clouds/k8s/types.go`
2. [ ] `ProbeHttpGet` includes `HTTPHeaders []HTTPHeader` field
3. [ ] `toProbeArgs()` function converts headers to Kubernetes format
4. [ ] Backward compatibility is maintained (all existing tests pass)
5. [ ] Unit tests cover header conversion logic
6. [ ] Documentation includes header usage examples
7. [ ] JSON schemas are updated for validation
8. [ ] Configuration examples demonstrate header usage

## References

- [Kubernetes Probes Documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
- [Kubernetes HTTPGetAction API](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#httpgetaction-v1-core)
- [Pulumi Kubernetes SDK v4](https://www.pulumi.com/registry/packages/kubernetes/api-docs/)
- [RFC 7230 - HTTP/1.1 Message Syntax](https://datatracker.ietf.org/doc/html/rfc7230)
- [Issue #168](https://github.com/simple-container-com/api/issues/168)
