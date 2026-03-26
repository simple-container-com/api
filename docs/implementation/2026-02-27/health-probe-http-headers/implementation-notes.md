# HTTP Headers Support for Health Probes - Implementation Notes

**Date:** 2026-02-27
**Issue:** #172 (Parent: #168)
**Status:** Implemented
**Author:** Software Developer

## Overview

This document describes the implementation of HTTP headers support for health probe configurations (readinessProbe, livenessProbe, and startupProbe) in the simple-container-com/api platform.

## Implementation Summary

The implementation adds support for custom HTTP headers in health probes, allowing users to configure headers for authentication, routing, and other use cases. The implementation is fully backward compatible with existing configurations.

## Changes Made

### 1. Type Definitions (`pkg/clouds/k8s/types.go`)

**Lines 110-132:**

Added the `HTTPHeader` type with comprehensive documentation:

```go
// HTTPHeader represents an HTTP header name-value pair for health probe requests.
// This allows customizing HTTP headers sent in readiness, liveness, and startup probes.
//
// Example:
//
//	HTTPHeader{
//		Name:  "Authorization",
//		Value: "Bearer token123",
//	}
//
// Kubernetes Reference: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
type HTTPHeader struct {
	// Name is the header field name (case-insensitive per HTTP spec)
	Name string `json:"name" yaml:"name"`
	// Value is the header field value
	Value string `json:"value" yaml:"value"`
}

type ProbeHttpGet struct {
	Path        string       `json:"path" yaml:"path"`
	Port        int          `json:"port" yaml:"port"`
	HTTPHeaders []HTTPHeader `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}
```

**Key Design Decisions:**
- Used `omitempty` JSON/YAML tags for backward compatibility
- Structured type provides better type safety than map[string]string
- Consistent with Kubernetes HTTPHeader structure
- Comprehensive documentation with examples

### 2. Probe Conversion Logic (`pkg/clouds/pulumi/kubernetes/deployment.go`)

**Lines 308-318:**

Updated the `toProbeArgs()` function to convert HTTP headers to Kubernetes format:

```go
// Add HTTP headers if specified
if len(probe.HttpGet.HTTPHeaders) > 0 {
	httpHeaders := make(corev1.HTTPHeaderArray, 0, len(probe.HttpGet.HTTPHeaders))
	for _, header := range probe.HttpGet.HTTPHeaders {
		httpHeaders = append(httpHeaders, corev1.HTTPHeaderArgs{
			Name:  sdk.String(header.Name),
			Value: sdk.String(header.Value),
		})
	}
	httpGetArgs.HttpHeaders = httpHeaders
}
```

**Implementation Details:**
- Converts `[]HTTPHeader` to `corev1.HTTPHeaderArray`
- Uses Pulumi's `sdk.String()` for pointer conversion
- Only sets HTTPHeaders field when non-empty (preserves existing behavior)
- Maintains existing TCP fallback logic
- Header order is preserved

### 3. Unit Tests (`pkg/clouds/pulumi/kubernetes/deployment_test.go`)

Created comprehensive test suite with 328 lines covering:

1. **TestToProbeArgs_WithHeaders**: Tests various header configurations
   - Single header
   - Multiple headers
   - No headers (backward compatibility)
   - Empty headers slice
   - TCP probe without path
   - Authorization headers
   - Host headers

2. **TestToProbeArgs_PortResolution**: Tests port resolution logic
   - Explicit port
   - Container MainPort
   - First container port fallback

3. **TestToProbeArgs_BackwardCompatibility**: Verifies existing configurations work
   - Legacy configuration without headers

4. **TestToProbeArgs_HeaderPreservation**: Verifies header conversion
   - Multiple headers are properly converted

### 4. Existing Tests (`pkg/clouds/k8s/types_test.go`)

**Lines 12-152:**

Existing tests already covered the HTTPHeader type and ProbeHttpGet structure:
- `TestHTTPHeader`: Validates HTTPHeader type
- `TestProbeHttpGet_WithHeaders`: Validates ProbeHttpGet with headers
- `TestCloudRunProbe_WithHeaders`: Validates CloudRunProbe with headers

## Backward Compatibility

The implementation maintains 100% backward compatibility:

1. **Existing configurations without headers**: Continue to work without any changes
2. **`omitempty` tags**: Ensure null fields are not serialized
3. **No default behavior changes**: All existing tests pass without modification
4. **No breaking API changes**: The feature is purely additive

## Testing Results

All tests passed successfully:

```
run [test]ok  	github.com/simple-container-com/api/pkg/clouds/k8s	0.024s
run [test]ok  	github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes	0.146s
```

**Test Coverage:**
- Unit tests for type validation (existing)
- Unit tests for header conversion logic (new)
- Unit tests for backward compatibility (new)
- All existing tests continue to pass

## Usage Examples

### Basic Health Check with Headers

```yaml
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

### Health Check with Authentication

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
    httpHeaders:
      - name: Authorization
        value: "Bearer ${HEALTH_CHECK_TOKEN}"
  failureThreshold: 3
```

### Health Check with Multiple Headers

```yaml
startupProbe:
  httpGet:
    path: /health/startup
    port: 8080
    httpHeaders:
      - name: X-Custom-Header
        value: "custom-value"
      - name: X-Request-ID
        value: "probe-123"
      - name: User-Agent
        value: "kube-probe"
  initialDelaySeconds: 5
  periodSeconds: 10
```

### Host-Based Routing

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: Host
        value: "example.com"
      - name: X-Forwarded-Proto
        value: "https"
```

## Known Issues

None. The implementation is straightforward and follows established patterns in the codebase.

## Future Enhancements

Out of scope for this implementation but potential future improvements:

1. **Secret References**: Support for referencing Kubernetes secrets in header values
2. **Dynamic Headers**: Support for placeholder expansion (pod name, namespace, etc.)
3. **Header Validation**: Stricter validation of header names per RFC 7230
4. **Header Templates**: Predefined header sets for common scenarios

## References

- Design Document: `docs/design/2026-02-26/health-probe-http-headers/design.md`
- Kubernetes Probes Documentation: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
- Kubernetes HTTPGetAction API: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#httpgetaction-v1-core
- Issue #168: https://github.com/simple-container-com/api/issues/168

## Acceptance Criteria Status

- [x] HTTPHeader type is defined in `pkg/clouds/k8s/types.go`
- [x] ProbeHttpGet includes HTTPHeaders field
- [x] toProbeArgs() function converts headers to Kubernetes format
- [x] Backward compatibility is maintained (all existing tests pass)
- [x] Unit tests cover header conversion logic
- [x] Documentation includes header usage examples
- [x] Code is formatted and linted
- [x] All tests pass

## Verification

The implementation was verified by:

1. Running `welder run fmt` - All code formatting and linting passed
2. Running `welder run test` - All tests passed successfully
3. Manual review of the code to ensure it follows established patterns
4. Verification that existing tests continue to pass without modification
