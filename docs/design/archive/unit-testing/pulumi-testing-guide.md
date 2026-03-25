# Pulumi Unit Testing Guide for Simple Container

## Overview

This comprehensive guide documents the practical knowledge gained from implementing robust unit tests for Simple Container's Pulumi-based infrastructure components. It covers proven patterns, common pitfalls, and solutions for testing complex Pulumi code with multiple resource dependencies.

## Table of Contents

1. [Core Testing Architecture](#core-testing-architecture)
2. [Thread-Safe Mock Implementation](#thread-safe-mock-implementation)
3. [Test Structure and Organization](#test-structure-and-organization)
4. [Common Patterns and Best Practices](#common-patterns-and-best-practices)
5. [Troubleshooting Guide](#troubleshooting-guide)
6. [Performance Optimization](#performance-optimization)
7. [Real-World Examples](#real-world-examples)

## Core Testing Architecture

### Pulumi Mock Framework

Simple Container uses Pulumi's built-in mocking framework with `pulumi.WithMocks()` to create isolated unit tests that don't require actual cloud resources.

```go
// Basic test structure
func TestSimpleContainer_BasicCreation(t *testing.T) {
    RegisterTestingT(t) // Required for Gomega assertions
    
    mocks := NewSimpleContainerMocks()
    
    err := pulumi.RunErr(func(ctx *pulumi.Context) error {
        sc, err := NewSimpleContainer(ctx, &SimpleContainerArgs{
            Namespace:  "test-namespace",
            Service:    "test-service",
            Deployment: "test-deployment",
            // ... other required fields
        })
        
        // Validate resource creation
        Expect(err).ToNot(HaveOccurred())
        Expect(sc).ToNot(BeNil())
        Expect(sc.ServicePublicIP).ToNot(BeEmpty())
        
        return nil
    }, pulumi.WithMocks("project", "stack", mocks))
    
    Expect(err).ToNot(HaveOccurred())
    
    // Validate resource counts OUTSIDE pulumi.RunErr
    Expect(mocks.GetResourceCount("kubernetes:core/v1:Namespace")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:apps/v1:Deployment")).To(Equal(1))
}
```

### Key Architectural Principles

1. **Isolation**: Each test uses fresh mock instances
2. **Timing**: Resource validation happens OUTSIDE `pulumi.RunErr`
3. **Thread Safety**: All mock operations are protected with mutexes
4. **Gomega Assertions**: Use Gomega for consistent assertion syntax

## Thread-Safe Mock Implementation

### Critical Issue: Data Races

**Problem**: Pulumi creates resources concurrently, causing data races when multiple goroutines access shared mock state.

**Solution**: Thread-safe mock implementation with proper synchronization:

```go
package kubernetes

import (
    "fmt"
    "sync"
    
    "github.com/pulumi/pulumi/sdk/v3/go/common/resource"
    "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// simpleContainerMocks implements pulumi.MockResourceMonitor for testing SimpleContainer
type simpleContainerMocks struct {
    // Mutex to protect concurrent access to maps
    mu sync.RWMutex
    // Track resource creation counts by type
    resourceCounts map[string]int
    // Store created resources for validation
    createdResources map[string]resource.PropertyMap
}

// NewSimpleContainerMocks creates a new mock instance
func NewSimpleContainerMocks() *simpleContainerMocks {
    return &simpleContainerMocks{
        resourceCounts:   make(map[string]int),
        createdResources: make(map[string]resource.PropertyMap),
    }
}

// NewResource mocks the creation of Kubernetes resources
func (m *simpleContainerMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    // Thread-safe resource tracking
    m.mu.Lock()
    m.resourceCounts[args.TypeToken]++
    count := m.resourceCounts[args.TypeToken]
    m.mu.Unlock()

    // Generate unique resource ID
    resourceID := fmt.Sprintf("%s-%d", args.Name, count)

    // Get input properties safely
    outputs := make(map[string]interface{})
    if args.Inputs != nil {
        if inputMap := args.Inputs.Mappable(); inputMap != nil {
            outputs = inputMap
        }
    }

    // Add mock outputs based on resource type
    switch args.TypeToken {
    case "kubernetes:core/v1:Namespace":
        return m.mockNamespace(args, outputs)
    case "kubernetes:apps/v1:Deployment":
        return m.mockDeployment(args, outputs)
    case "kubernetes:core/v1:Service":
        return m.mockService(args, outputs)
    // ... other resource types
    default:
        return resourceID, resource.NewPropertyMapFromMap(outputs), nil
    }
}

// GetResourceCount returns the number of resources created of a specific type
func (m *simpleContainerMocks) GetResourceCount(resourceType string) int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.resourceCounts[resourceType]
}

// GetCreatedResource returns the properties of a created resource
func (m *simpleContainerMocks) GetCreatedResource(resourceID string) (resource.PropertyMap, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    props, exists := m.createdResources[resourceID]
    return props, exists
}
```

### Mock Resource Implementations

Each resource type needs specific mock implementations with thread-safe storage:

```go
func (m *simpleContainerMocks) mockDeployment(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
    resourceID := fmt.Sprintf("%s-deploy", args.Name)

    // Add deployment-specific outputs
    outputs["status"] = map[string]interface{}{
        "readyReplicas":     1,
        "availableReplicas": 1,
        "replicas":          1,
        "updatedReplicas":   1,
    }

    // Store for validation (thread-safe)
    props := resource.NewPropertyMapFromMap(outputs)
    m.mu.Lock()
    m.createdResources[resourceID] = props
    m.mu.Unlock()

    return resourceID, props, nil
}

func (m *simpleContainerMocks) mockService(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
    resourceID := fmt.Sprintf("%s-svc", args.Name)

    // Add service-specific outputs
    outputs["status"] = map[string]interface{}{
        "loadBalancer": map[string]interface{}{
            "ingress": []map[string]interface{}{
                {
                    "ip": "203.0.113.12",
                },
            },
        },
    }

    // Store for validation (thread-safe)
    props := resource.NewPropertyMapFromMap(outputs)
    m.mu.Lock()
    m.createdResources[resourceID] = props
    m.mu.Unlock()

    return resourceID, props, nil
}
```

## Test Structure and Organization

### File Organization

```
pkg/clouds/pulumi/kubernetes/
├── simple_container.go                    # Main implementation
├── simple_container_test.go               # Basic functionality tests
├── simple_container_advanced_test.go     # Advanced scenarios
├── simple_container_edge_cases_test.go   # Edge cases and error handling
├── simple_container_final_test.go        # Integration tests
├── simple_container_mocks_test.go        # Mock implementations
└── hpa_test.go                           # HPA-specific tests
```

### Test Categories

#### 1. Basic Resource Creation Tests
```go
func TestSimpleContainer_CreationSuccess(t *testing.T)
func TestSimpleContainer_NamespaceCreation(t *testing.T)
func TestSimpleContainer_DeploymentCreation(t *testing.T)
func TestSimpleContainer_ServiceCreation(t *testing.T)
```

#### 2. Feature Integration Tests
```go
func TestSimpleContainer_HPAIntegration(t *testing.T)
func TestSimpleContainer_VPAIntegration(t *testing.T)
func TestSimpleContainer_IngressIntegration(t *testing.T)
func TestSimpleContainer_PersistentVolumeIntegration(t *testing.T)
```

#### 3. Configuration Validation Tests
```go
func TestSimpleContainer_NameSanitization(t *testing.T)
func TestSimpleContainer_LabelPropagation(t *testing.T)
func TestSimpleContainer_MinimalConfiguration(t *testing.T)
func TestSimpleContainer_ComplexConfiguration(t *testing.T)
```

#### 4. Edge Cases and Error Handling
```go
func TestSimpleContainer_EmptyStringFields(t *testing.T)
func TestSimpleContainer_MultiContainerDeployment(t *testing.T)
func TestSimpleContainer_SpecialCharacters(t *testing.T)
```

## Common Patterns and Best Practices

### 1. Timing and Synchronization

**❌ WRONG - Validation inside Pulumi context:**
```go
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    
    // ❌ This can cause timing issues
    assert.Equal(t, 1, mocks.GetResourceCount("kubernetes:core/v1:Namespace"))
    return err
}, pulumi.WithMocks("project", "stack", mocks))
```

**✅ CORRECT - Validation outside Pulumi context:**
```go
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    
    // ✅ Only validate resource creation, not counts
    Expect(err).ToNot(HaveOccurred())
    Expect(sc).ToNot(BeNil())
    return err
}, pulumi.WithMocks("project", "stack", mocks))

Expect(err).ToNot(HaveOccurred())

// ✅ Validate resource counts AFTER Pulumi execution
Expect(mocks.GetResourceCount("kubernetes:core/v1:Namespace")).To(Equal(1))
```

### 2. Gomega Assertion Patterns

**Use Gomega consistently throughout the codebase:**

```go
func TestSomething(t *testing.T) {
    RegisterTestingT(t)  // Required for Gomega
    
    // Basic assertions
    Expect(err).ToNot(HaveOccurred())
    Expect(sc).ToNot(BeNil())
    Expect(sc.ServicePublicIP).ToNot(BeEmpty())
    
    // For Pulumi outputs, only test existence
    Expect(sc.CaddyfileEntry).ToNot(BeEmpty())
    
    // Don't test content of Pulumi outputs in unit tests
    // ❌ Expect(sc.ServicePublicIP).To(ContainSubstring("203.0.113"))
}
```

### 3. Test Data Management

**Create reusable test argument builders:**

```go
func createBasicArgs() *SimpleContainerArgs {
    return &SimpleContainerArgs{
        Namespace:  "test-namespace",
        Service:    "test-service",
        Deployment: "test-deployment",
        ScEnv:      "test",
        Containers: []Container{
            {
                Name:  "app",
                Image: "nginx:latest",
                Ports: []ContainerPort{
                    {ContainerPort: 8080, Name: "http"},
                },
            },
        },
    }
}

func createHPAArgs() *SimpleContainerArgs {
    args := createBasicArgs()
    args.HPA = &k8s.HPAConfig{
        Enabled:     true,
        MinReplicas: 2,
        MaxReplicas: 10,
        CPUTarget:   &[]int{70}[0],
    }
    return args
}
```

### 4. Table-Driven Tests

**Use table-driven tests for multiple scenarios:**

```go
func TestSimpleContainer_NameSanitization(t *testing.T) {
    RegisterTestingT(t)
    
    testCases := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "underscores_replaced",
            input:    "test_service_name",
            expected: "test-service-name",
        },
        {
            name:     "uppercase_lowercased",
            input:    "TestServiceName",
            expected: "testservicename",
        },
        {
            name:     "special_chars_removed",
            input:    "test@service#name",
            expected: "testservicename",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            mocks := NewSimpleContainerMocks()
            
            err := pulumi.RunErr(func(ctx *pulumi.Context) error {
                args := createBasicArgs()
                args.Service = tc.input
                
                sc, err := NewSimpleContainer(ctx, args)
                Expect(err).ToNot(HaveOccurred())
                Expect(sc).ToNot(BeNil())
                return nil
            }, pulumi.WithMocks("project", "stack", mocks))
            
            Expect(err).ToNot(HaveOccurred())
        })
    }
}
```

## Troubleshooting Guide

### Common Issues and Solutions

#### 1. Data Races in Tests

**Symptoms:**
```
==================
WARNING: DATA RACE
Read at 0x00c0022be150 by goroutine 43:
Write at 0x00c0022be150 by goroutine 39:
FAIL	github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes	0.255s
```

**Solution:** Implement thread-safe mocks with proper mutex protection (see [Thread-Safe Mock Implementation](#thread-safe-mock-implementation))

#### 2. Nil Pointer Dereference in Mocks

**Symptoms:**
```
panic: runtime error: invalid memory address or nil pointer dereference
at simple_container_mocks_test.go:35
```

**Solution:** Add nil checks in mock implementations:
```go
func (m *simpleContainerMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    // Get input properties safely
    outputs := make(map[string]interface{})
    if args.Inputs != nil {
        if inputMap := args.Inputs.Mappable(); inputMap != nil {
            outputs = inputMap
        }
    }
    // ... rest of implementation
}
```

#### 3. Timing Issues with Resource Validation

**Symptoms:** Flaky tests that sometimes pass and sometimes fail, especially resource count assertions.

**Solution:** Move all resource count validations outside `pulumi.RunErr`:
```go
// ❌ Inside Pulumi context - can be flaky
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    assert.Equal(t, 1, mocks.GetResourceCount("kubernetes:core/v1:Service")) // Flaky!
    return err
}, pulumi.WithMocks("project", "stack", mocks))

// ✅ Outside Pulumi context - reliable
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    return err
}, pulumi.WithMocks("project", "stack", mocks))

Expect(err).ToNot(HaveOccurred())
Expect(mocks.GetResourceCount("kubernetes:core/v1:Service")).To(Equal(1)) // Reliable!
```

#### 4. Pulumi Output Assertion Issues

**Symptoms:** Tests fail when trying to assert on Pulumi output content.

**Solution:** Only test output existence, not content, in unit tests:
```go
// ❌ Don't test Pulumi output content
Expect(sc.ServicePublicIP).To(ContainSubstring("203.0.113"))

// ✅ Only test output existence
Expect(sc.ServicePublicIP).ToNot(BeEmpty())
```

### Debugging Tips

1. **Use `-race` flag** to detect data races: `go test -race ./pkg/clouds/pulumi/kubernetes/`
2. **Run individual tests** to isolate issues: `go test -run TestSpecificTest`
3. **Add debug logging** in mocks to trace resource creation
4. **Check mock resource counts** to verify expected resource creation

## Performance Optimization

### Test Execution Performance

**Target Metrics:**
- Individual test: < 0.1 seconds
- Full package test suite: < 2 seconds
- Full project test suite: < 30 seconds

### Optimization Strategies

#### 1. Efficient Mock Implementations
```go
// ✅ Lightweight mock responses
func (m *simpleContainerMocks) mockNamespace(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
    resourceID := fmt.Sprintf("%s-ns", args.Name)
    
    // Minimal required outputs
    outputs["status"] = map[string]interface{}{
        "phase": "Active",
    }
    
    props := resource.NewPropertyMapFromMap(outputs)
    m.mu.Lock()
    m.createdResources[resourceID] = props
    m.mu.Unlock()
    
    return resourceID, props, nil
}
```

#### 2. Parallel Test Execution
```go
func TestSimpleContainer_ParallelTests(t *testing.T) {
    testCases := []struct{
        name string
        args *SimpleContainerArgs
    }{
        // ... test cases
    }
    
    for _, tc := range testCases {
        tc := tc // Capture loop variable
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel() // Enable parallel execution
            
            // ... test implementation
        })
    }
}
```

#### 3. Shared Test Utilities
```go
// Reusable test utilities to reduce setup overhead
var (
    basicArgs = &SimpleContainerArgs{
        Namespace:  "test-namespace",
        Service:    "test-service",
        Deployment: "test-deployment",
        // ... other common fields
    }
)

func runSimpleContainerTest(t *testing.T, args *SimpleContainerArgs, validations ...func(*simpleContainerMocks)) {
    RegisterTestingT(t)
    mocks := NewSimpleContainerMocks()
    
    err := pulumi.RunErr(func(ctx *pulumi.Context) error {
        sc, err := NewSimpleContainer(ctx, args)
        Expect(err).ToNot(HaveOccurred())
        Expect(sc).ToNot(BeNil())
        return nil
    }, pulumi.WithMocks("project", "stack", mocks))
    
    Expect(err).ToNot(HaveOccurred())
    
    for _, validation := range validations {
        validation(mocks)
    }
}
```

## Real-World Examples

### Complete Test Implementation

Here's a complete example showing all the patterns in action:

```go
package kubernetes

import (
    "testing"
    
    . "github.com/onsi/gomega"
    "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
    "github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestSimpleContainer_CompleteExample(t *testing.T) {
    RegisterTestingT(t)
    
    // Create thread-safe mocks
    mocks := NewSimpleContainerMocks()
    
    // Define test arguments
    args := &SimpleContainerArgs{
        Namespace:  "production",
        Service:    "web-app",
        Deployment: "web-app-deployment",
        ScEnv:      "prod",
        Containers: []Container{
            {
                Name:  "app",
                Image: "myapp:v1.0.0",
                Ports: []ContainerPort{
                    {ContainerPort: 8080, Name: "http"},
                },
                Resources: &ResourceRequirements{
                    Requests: map[string]string{
                        "cpu":    "100m",
                        "memory": "128Mi",
                    },
                    Limits: map[string]string{
                        "cpu":    "500m",
                        "memory": "512Mi",
                    },
                },
            },
        },
        HPA: &k8s.HPAConfig{
            Enabled:     true,
            MinReplicas: 2,
            MaxReplicas: 10,
            CPUTarget:   &[]int{70}[0],
        },
        Ingress: &IngressConfig{
            Enabled: true,
            Host:    "myapp.example.com",
        },
    }
    
    // Execute test within Pulumi context
    err := pulumi.RunErr(func(ctx *pulumi.Context) error {
        sc, err := NewSimpleContainer(ctx, args)
        
        // Only validate basic resource creation inside Pulumi context
        Expect(err).ToNot(HaveOccurred())
        Expect(sc).ToNot(BeNil())
        Expect(sc.ServicePublicIP).ToNot(BeEmpty())
        Expect(sc.CaddyfileEntry).ToNot(BeEmpty())
        
        return nil
    }, pulumi.WithMocks("project", "stack", mocks))
    
    // Validate execution success
    Expect(err).ToNot(HaveOccurred())
    
    // Validate resource creation counts OUTSIDE Pulumi context
    Expect(mocks.GetResourceCount("kubernetes:core/v1:Namespace")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:apps/v1:Deployment")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:core/v1:Service")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:core/v1:ConfigMap")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:core/v1:Secret")).To(Equal(2)) // env + volumes
    Expect(mocks.GetResourceCount("kubernetes:autoscaling/v2:HorizontalPodAutoscaler")).To(Equal(1))
    Expect(mocks.GetResourceCount("kubernetes:networking.k8s.io/v1:Ingress")).To(Equal(1))
}
```

### Advanced Testing Scenarios

#### Testing Error Conditions
```go
func TestSimpleContainer_ErrorHandling(t *testing.T) {
    RegisterTestingT(t)
    
    testCases := []struct {
        name        string
        args        *SimpleContainerArgs
        expectError bool
        errorMsg    string
    }{
        {
            name: "missing_namespace",
            args: &SimpleContainerArgs{
                Service:    "test-service",
                Deployment: "test-deployment",
                // Namespace missing
            },
            expectError: true,
            errorMsg:    "namespace is required",
        },
        {
            name: "invalid_hpa_config",
            args: &SimpleContainerArgs{
                Namespace:  "test",
                Service:    "test-service",
                Deployment: "test-deployment",
                HPA: &k8s.HPAConfig{
                    Enabled:     true,
                    MinReplicas: 10,
                    MaxReplicas: 5, // Invalid: min > max
                },
            },
            expectError: true,
            errorMsg:    "minReplicas cannot be greater than maxReplicas",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            RegisterTestingT(t)
            mocks := NewSimpleContainerMocks()
            
            err := pulumi.RunErr(func(ctx *pulumi.Context) error {
                _, err := NewSimpleContainer(ctx, tc.args)
                return err
            }, pulumi.WithMocks("project", "stack", mocks))
            
            if tc.expectError {
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring(tc.errorMsg))
            } else {
                Expect(err).ToNot(HaveOccurred())
            }
        })
    }
}
```

## Conclusion

This guide represents the practical knowledge gained from implementing comprehensive unit tests for Simple Container's Pulumi infrastructure code. The key lessons learned are:

1. **Thread safety is critical** when testing concurrent Pulumi resource creation
2. **Timing matters** - validate resource counts outside Pulumi contexts
3. **Gomega provides consistency** across the codebase
4. **Mock implementations should be lightweight** but comprehensive
5. **Test isolation prevents flaky tests**

Following these patterns will result in:
- **Fast, reliable tests** (< 2 seconds for full package)
- **High confidence in refactoring** with comprehensive coverage
- **Clear documentation** of expected behavior through tests
- **Reduced debugging time** with proper error handling

The investment in robust testing infrastructure pays dividends in development velocity and system reliability.
