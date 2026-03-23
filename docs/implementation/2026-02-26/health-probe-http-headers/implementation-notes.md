# HTTP Headers Support for Health Probes - Implementation Notes

**Date:** 2026-02-26
**Issue:** #171
**Implementation Status:** Complete

## Summary

This implementation adds support for custom HTTP headers in health probe configurations (readinessProbe, livenessProbe, and startupProbe) for Kubernetes deployments. The feature allows users to specify custom HTTP headers that will be sent with probe HTTP GET requests.

## Files Modified

### 1. Type Definitions (`pkg/clouds/k8s/types.go`)

**Changes:**
- Added `HTTPHeader` struct with `Name` and `Value` fields
- Updated `ProbeHttpGet` struct to include optional `HTTPHeaders []HTTPHeader` field
- Added comprehensive documentation with examples

**Lines Modified:** 110-121

**Code Added:**
```go
// HTTPHeader represents an HTTP header name-value pair for health probe requests.
type HTTPHeader struct {
    Name string `json:"name" yaml:"name"`
    Value string `json:"value" yaml:"value"`
}

type ProbeHttpGet struct {
    Path         string       `json:"path" yaml:"path"`
    Port         int          `json:"port" yaml:"port"`
    HTTPHeaders  []HTTPHeader `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}
```

### 2. Kubernetes Deployment (`pkg/clouds/pulumi/kubernetes/deployment.go`)

**Changes:**
- Updated `toProbeArgs()` function to convert `[]HTTPHeader` to `corev1.HTTPHeaderArray`
- Headers are only set when the slice is non-empty (backward compatibility)

**Lines Modified:** 279-328

**Implementation Details:**
- Creates HTTPHeaderArray with proper capacity pre-allocation
- Uses `sdk.String()` for pointer conversion
- Preserves header order
- Maintains existing TCP fallback logic

### 3. Unit Tests (`pkg/clouds/k8s/types_test.go`)

**Changes:**
- Added `TestHTTPHeader` to test header structure validation
- Added `TestProbeHttpGet_WithHeaders` to test headers in probe config
- Added `TestCloudRunProbe_WithHeaders` to test full probe structure

**Coverage:**
- Header creation and field access
- Single and multiple header configurations
- Empty and nil header handling
- Backward compatibility

### 4. Probe Conversion Tests (`pkg/clouds/pulumi/kubernetes/probe_test.go`)

**New File:** Comprehensive test suite for header conversion logic

**Test Cases:**
- HTTP probe with single header
- HTTP probe with multiple headers
- HTTP probe without headers (backward compatibility)
- HTTP probe with empty headers slice
- HTTP probe using main port fallback
- TCP probe (no path specified)
- Backward compatibility test
- Header order preservation test

**Total Test Functions:** 3
**Total Test Cases:** 9

### 5. JSON Schema (`docs/schemas/kubernetes/kubernetescloudextras.json`)

**Changes:**
- Updated `readinessProbe.httpGet` to include `httpHeaders` array
- Updated `livenessProbe.httpGet` to include `httpHeaders` array
- Header schema includes `name` (string) and `value` (string) fields
- Headers are optional (not in required array)

## Backward Compatibility

The implementation maintains 100% backward compatibility:

1. **Existing configurations work without changes:** The `HTTPHeaders` field has `omitempty` JSON and YAML tags
2. **No breaking changes:** All existing functionality remains unchanged
3. **Default behavior preserved:** When headers are not specified, probes behave exactly as before
4. **All existing tests pass:** No modifications were needed to existing tests

## Testing Strategy

### Unit Tests

**Type Tests (`types_test.go`):**
- Verify HTTPHeader struct creation and field access
- Test ProbeHttpGet with various header configurations
- Validate CloudRunProbe integration

**Conversion Tests (`probe_test.go`):**
- Test conversion from probe types to Kubernetes ProbeArgs
- Verify header conversion to Pulumi HTTPHeaderArray
- Test edge cases (nil, empty, single, multiple headers)
- Validate backward compatibility
- Test header order preservation

### Manual Testing

To manually test the implementation:

```yaml
# client.yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
      - name: Authorization
        value: "Bearer ${HEALTH_CHECK_TOKEN}"
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Known Issues and Limitations

### Current Limitations

1. **No header validation:** The implementation does not validate header names or values beyond basic structure. Future enhancements could add:
   - Validation for empty header names
   - Validation for newline characters in headers
   - Warnings for sensitive headers (Authorization, Cookie)

2. **No secret references:** Header values are plain strings. Future enhancements could support:
   - Kubernetes secret references: `value: ${secret:my-secret/token}`
   - Environment variable expansion: `value: ${ENV_VAR}`

3. **No header templates:** Predefined header sets are not available. Future enhancements could add:
   - Common header templates (OAuth2, API keys)
   - Namespace-level default headers

### Workarounds

1. **For secret values:** Use environment variable expansion in the shell before deployment
2. **For validation:** Add validation logic in a pre-deployment hook
3. **For templates:** Define header sets in a separate configuration file and reference them

## Performance Considerations

The implementation has minimal performance impact:

1. **Memory allocation:** Headers are only allocated when specified
2. **Conversion overhead:** Linear O(n) where n is the number of headers
3. **No impact when unused:** When headers are not specified, there's zero overhead

## Security Considerations

1. **Sensitive headers:** Headers with sensitive data (Authorization, Cookie) are stored in plain text in the configuration
   - **Recommendation:** Use Kubernetes secrets for sensitive values
   - **Future enhancement:** Implement secret reference support

2. **Header injection:** No validation prevents header injection attacks
   - **Risk level:** Low (users control their own configurations)
   - **Future enhancement:** Add header validation per RFC 7230

3. **Logging:** Headers may be logged in debug output
   - **Recommendation:** Implement header redaction in logging

## Migration Guide

### For Existing Users

No migration required! The feature is fully backward compatible.

### To Adopt the Feature

Simply add the `httpHeaders` field to your probe configurations:

**Before:**
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
```

**After:**
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
    httpHeaders:
      - name: X-Health-Check
        value: "true"
```

## Future Enhancements

As outlined in the design document, the following enhancements are out of scope for this implementation but could be added in future iterations:

1. **Secret References** - Support for referencing Kubernetes secrets
2. **Dynamic Headers** - Support for placeholder expansion
3. **Header Templates** - Predefined header sets for common scenarios
4. **Header Validation** - Stricter validation per RFC 7230
5. **Probe-Level Defaults** - Global default headers for all probes

## References

- Design Document: `docs/design/2026-02-26/health-probe-http-headers/design.md`
- Kubernetes Probes: https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
- HTTPGetAction API: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#httpgetaction-v1-core
- RFC 7230 (HTTP/1.1): https://datatracker.ietf.org/doc/html/rfc7230
