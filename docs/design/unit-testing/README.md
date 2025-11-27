# Unit Testing Strategy for Simple Container Pulumi Components

## Overview

This document outlines a comprehensive strategy for implementing unit tests for Simple Container's Pulumi-based infrastructure components, with a focus on the `simple_container.go` module. The goal is to ensure reliability, maintainability, and correctness of our Kubernetes resource generation logic.

## Current State Analysis

### Existing Test Coverage
- ✅ **HPA Logic**: `hpa_test.go` - Comprehensive validation tests
- ✅ **K8s Types**: `types_test.go` - Scale configuration and memory parsing tests
- ❌ **SimpleContainer**: No unit tests for the main `NewSimpleContainer` function
- ❌ **Deployment Logic**: No unit tests for `DeploySimpleContainer`
- ❌ **Resource Generation**: No tests for individual K8s resource creation

### Testing Challenges Identified

1. **Complex Pulumi Context**: `NewSimpleContainer` requires a Pulumi context and creates multiple K8s resources
2. **Resource Dependencies**: Resources depend on each other (e.g., namespace → deployment → service)
3. **Output Values**: Many assertions need to work with Pulumi `Output` types
4. **Integration Complexity**: Function creates 8+ different K8s resources in one call

## Unit Testing Strategy

### 1. Pulumi Mock-Based Testing

#### Core Approach
Use Pulumi's built-in mocking framework with `pulumi.WithMocks()` to create isolated unit tests that don't require actual Kubernetes clusters.

#### Mock Implementation Pattern
```go
type simpleContainerMocks struct {
    resourceCounts map[string]int
}

func (m *simpleContainerMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    // Track resource creation
    m.resourceCounts[args.TypeToken]++
    
    // Return mock outputs based on resource type
    switch args.TypeToken {
    case "kubernetes:core/v1:Namespace":
        return m.mockNamespace(args)
    case "kubernetes:apps/v1:Deployment":
        return m.mockDeployment(args)
    case "kubernetes:core/v1:Service":
        return m.mockService(args)
    // ... other resource types
    }
}

func (m *simpleContainerMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
    return resource.NewPropertyMapFromMap(map[string]interface{}{}), nil
}
```

### 2. Test Structure and Organization

#### File Organization
```
pkg/clouds/pulumi/kubernetes/
├── simple_container.go
├── simple_container_test.go          # Main test file
├── simple_container_mocks_test.go    # Mock implementations
└── simple_container_testdata/        # Test data and fixtures
    ├── basic_config.yaml
    ├── hpa_config.yaml
    └── complex_config.yaml
```

#### Test Categories

##### 2.1 Resource Creation Tests
- **Namespace Creation**: Verify namespace with correct labels/annotations
- **Deployment Creation**: Validate deployment spec, containers, resources
- **Service Creation**: Check service configuration and port mapping
- **ConfigMap/Secret Creation**: Ensure proper data handling
- **PVC Creation**: Validate persistent volume claims
- **HPA Creation**: Test horizontal pod autoscaler integration
- **VPA Creation**: Test vertical pod autoscaler integration

##### 2.2 Configuration Validation Tests
- **Input Sanitization**: K8s name compliance (RFC 1123)
- **Label/Annotation Propagation**: Consistent labeling across resources
- **Resource Dependencies**: Proper namespace references
- **Conditional Logic**: Optional resources (ingress, PDB, etc.)

##### 2.3 Edge Case Tests
- **Minimal Configuration**: Test with bare minimum inputs
- **Maximum Configuration**: Test with all optional features enabled
- **Error Conditions**: Invalid inputs, missing required fields
- **Resource Conflicts**: Name collisions, invalid characters

##### 2.4 Integration Tests
- **End-to-End Scenarios**: Complete stack creation workflows
- **Multi-Environment**: Different configurations for dev/staging/prod
- **Scaling Scenarios**: Various replica and resource configurations

### 3. Implementation Plan

#### Phase 1: Foundation Setup
**Duration**: 2-3 days
**Files**: `simple_container_mocks_test.go`, basic test structure

1. **Mock Framework Setup**
   - Implement `simpleContainerMocks` struct
   - Create mock responses for all K8s resource types
   - Set up test utilities and helpers

2. **Basic Test Infrastructure**
   - Test runner configuration
   - Common test fixtures and data
   - Assertion helpers for Pulumi outputs

3. **Initial Test Cases**
   - Basic resource creation test
   - Namespace creation validation
   - Simple deployment test

#### Phase 2: Core Resource Testing
**Duration**: 3-4 days
**Files**: `simple_container_test.go` (main test cases)

1. **Resource Creation Tests**
   ```go
   func TestNewSimpleContainer_NamespaceCreation(t *testing.T)
   func TestNewSimpleContainer_DeploymentCreation(t *testing.T)
   func TestNewSimpleContainer_ServiceCreation(t *testing.T)
   func TestNewSimpleContainer_ConfigMapCreation(t *testing.T)
   func TestNewSimpleContainer_SecretCreation(t *testing.T)
   ```

2. **Configuration Validation Tests**
   ```go
   func TestNewSimpleContainer_LabelPropagation(t *testing.T)
   func TestNewSimpleContainer_AnnotationHandling(t *testing.T)
   func TestNewSimpleContainer_NameSanitization(t *testing.T)
   ```

3. **Conditional Resource Tests**
   ```go
   func TestNewSimpleContainer_WithIngress(t *testing.T)
   func TestNewSimpleContainer_WithPodDisruptionBudget(t *testing.T)
   func TestNewSimpleContainer_WithPersistentVolumes(t *testing.T)
   ```

#### Phase 3: Advanced Features Testing
**Duration**: 2-3 days
**Files**: Extended test cases, edge case handling

1. **HPA Integration Tests**
   ```go
   func TestNewSimpleContainer_HPACreation(t *testing.T)
   func TestNewSimpleContainer_HPAValidation(t *testing.T)
   func TestNewSimpleContainer_HPAWithoutResources(t *testing.T)
   ```

2. **VPA Integration Tests**
   ```go
   func TestNewSimpleContainer_VPACreation(t *testing.T)
   func TestNewSimpleContainer_VPAConfiguration(t *testing.T)
   ```

3. **Complex Scenarios**
   ```go
   func TestNewSimpleContainer_FullConfiguration(t *testing.T)
   func TestNewSimpleContainer_MultiContainer(t *testing.T)
   func TestNewSimpleContainer_ProductionScenario(t *testing.T)
   ```

#### Phase 4: Error Handling and Edge Cases
**Duration**: 2 days
**Files**: Error condition tests, validation tests

1. **Error Condition Tests**
   ```go
   func TestNewSimpleContainer_InvalidInputs(t *testing.T)
   func TestNewSimpleContainer_MissingRequiredFields(t *testing.T)
   func TestNewSimpleContainer_ResourceCreationFailure(t *testing.T)
   ```

2. **Edge Case Tests**
   ```go
   func TestNewSimpleContainer_EmptyConfiguration(t *testing.T)
   func TestNewSimpleContainer_MaximumConfiguration(t *testing.T)
   func TestNewSimpleContainer_SpecialCharacters(t *testing.T)
   ```

### 4. Technical Implementation Details

#### 4.1 Mock Resource Responses

Each K8s resource type needs specific mock responses:

```go
func (m *simpleContainerMocks) mockNamespace(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    outputs := args.Inputs.Mappable()
    outputs["status"] = map[string]interface{}{
        "phase": "Active",
    }
    return args.Name + "-ns-id", resource.NewPropertyMapFromMap(outputs), nil
}

func (m *simpleContainerMocks) mockDeployment(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    outputs := args.Inputs.Mappable()
    outputs["status"] = map[string]interface{}{
        "readyReplicas":     1,
        "availableReplicas": 1,
        "replicas":          1,
    }
    return args.Name + "-deploy-id", resource.NewPropertyMapFromMap(outputs), nil
}
```

#### 4.2 Test Utilities

```go
// Test utilities for common operations
func createTestArgs() *SimpleContainerArgs {
    return &SimpleContainerArgs{
        Namespace:  "test-namespace",
        Service:    "test-service",
        Deployment: "test-deployment",
        ScEnv:      "test",
        // ... other required fields
    }
}

func assertResourceCreated(t *testing.T, mocks *simpleContainerMocks, resourceType string, expectedCount int) {
    count := mocks.resourceCounts[resourceType]
    assert.Equal(t, expectedCount, count, "Expected %d %s resources, got %d", expectedCount, resourceType, count)
}

func extractOutputValue(t *testing.T, output pulumi.Output, callback func(interface{})) {
    var wg sync.WaitGroup
    wg.Add(1)
    
    output.ApplyT(func(val interface{}) interface{} {
        defer wg.Done()
        callback(val)
        return val
    })
    
    wg.Wait()
}
```

#### 4.3 Test Data Management

Create structured test data for different scenarios:

```go
type TestScenario struct {
    Name        string
    Args        *SimpleContainerArgs
    ExpectedResources map[string]int
    Validations []func(*testing.T, *SimpleContainer)
}

var testScenarios = []TestScenario{
    {
        Name: "basic_configuration",
        Args: &SimpleContainerArgs{
            Namespace: "basic-test",
            Service:   "basic-service",
            // ... minimal config
        },
        ExpectedResources: map[string]int{
            "kubernetes:core/v1:Namespace":    1,
            "kubernetes:apps/v1:Deployment":   1,
            "kubernetes:core/v1:Service":      1,
            "kubernetes:core/v1:ConfigMap":    1,
            "kubernetes:core/v1:Secret":       2, // env + volumes
        },
        Validations: []func(*testing.T, *SimpleContainer){
            validateBasicLabels,
            validateNamespaceCreation,
        },
    },
    // ... more scenarios
}
```

### 5. Testing Best Practices

#### 5.1 Test Isolation
- Each test should be independent and not rely on other tests
- Use fresh mock instances for each test
- Clean up any shared state between tests

#### 5.2 Assertion Strategies
- Test both resource creation and configuration
- Validate Pulumi outputs using `ApplyT` with proper synchronization
- Use table-driven tests for multiple scenarios
- Assert on both positive and negative cases

#### 5.3 Performance Considerations
- Mock tests should run quickly (< 1 second per test)
- Use parallel test execution where possible
- Avoid unnecessary resource creation in mocks

#### 5.4 Maintainability
- Keep mocks simple and focused
- Use helper functions to reduce code duplication
- Document complex test scenarios
- Regular review and refactoring of test code

### 6. Success Metrics

#### 6.1 Coverage Targets
- **Line Coverage**: 80%+ for `simple_container.go`
- **Branch Coverage**: 75%+ for conditional logic
- **Function Coverage**: 100% for public functions

#### 6.2 Quality Metrics
- **Test Execution Time**: < 30 seconds for full test suite
- **Test Reliability**: 0% flaky tests
- **Maintenance Overhead**: < 2 hours/month for test updates

#### 6.3 Validation Criteria
- All K8s resource types are tested
- All configuration paths are covered
- Error conditions are properly handled
- Integration with HPA/VPA is validated

### 7. Integration with CI/CD

#### 7.1 Test Execution
- Run unit tests on every PR
- Include test results in PR status checks
- Generate coverage reports for review

#### 7.2 Quality Gates
- Require 80% test coverage for new code
- Block merges if tests fail
- Automated test result notifications

### 8. Future Enhancements

#### 8.1 Property-Based Testing
- Use property-based testing for configuration validation
- Generate random valid configurations and test invariants
- Fuzz testing for input sanitization

#### 8.2 Integration Testing
- Extend to integration tests with real K8s clusters
- Test actual resource creation and behavior
- Performance testing under load

#### 8.3 Visual Testing
- Generate visual representations of created resources
- Compare expected vs actual resource graphs
- Documentation generation from tests

## Conclusion

This comprehensive unit testing strategy will significantly improve the reliability and maintainability of Simple Container's Kubernetes resource generation logic. The phased implementation approach allows for incremental progress while maintaining development velocity.

The focus on Pulumi's native mocking capabilities ensures that tests are fast, reliable, and closely aligned with the actual runtime behavior of our infrastructure code.

**Estimated Total Implementation Time**: 9-12 days
**Maintenance Overhead**: Low (< 2 hours/month)
**Expected Benefits**: 
- 80%+ reduction in infrastructure-related bugs
- Faster development cycles with confident refactoring
- Improved code documentation through test examples
- Better onboarding experience for new developers
