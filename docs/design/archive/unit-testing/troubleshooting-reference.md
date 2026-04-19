# Pulumi Unit Testing Troubleshooting Reference

## Quick Reference Guide

This document provides quick solutions to common issues encountered when writing and maintaining Pulumi unit tests in Simple Container.

## Common Error Patterns

### 1. Data Race Errors

**Error Pattern:**
```
==================
WARNING: DATA RACE
Read at 0x00c0022be150 by goroutine 43:
  github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes.(*simpleContainerMocks).NewResource()
Write at 0x00c0022be150 by goroutine 39:
  github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes.(*simpleContainerMocks).NewResource()
FAIL	github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes	0.255s
```

**Root Cause:** Concurrent access to shared maps in mock structs without synchronization.

**Solution:**
```go
type simpleContainerMocks struct {
    // ✅ Add mutex for thread safety
    mu sync.RWMutex
    resourceCounts map[string]int
    createdResources map[string]resource.PropertyMap
}

func (m *simpleContainerMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    // ✅ Protect writes with mutex
    m.mu.Lock()
    m.resourceCounts[args.TypeToken]++
    count := m.resourceCounts[args.TypeToken]
    m.mu.Unlock()
    
    // ... rest of implementation
}

func (m *simpleContainerMocks) GetResourceCount(resourceType string) int {
    // ✅ Protect reads with read lock
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.resourceCounts[resourceType]
}
```

### 2. Nil Pointer Dereference

**Error Pattern:**
```
panic: runtime error: invalid memory address or nil pointer dereference
at simple_container_mocks_test.go:35
```

**Root Cause:** Accessing `args.Inputs.Mappable()` when `args.Inputs` is nil.

**Solution:**
```go
func (m *simpleContainerMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
    // ✅ Safe input handling
    outputs := make(map[string]interface{})
    if args.Inputs != nil {
        if inputMap := args.Inputs.Mappable(); inputMap != nil {
            outputs = inputMap
        }
    }
    
    // ... rest of implementation
}
```

### 3. Flaky Resource Count Tests

**Error Pattern:**
Tests that sometimes pass and sometimes fail, especially when checking resource counts.

**Root Cause:** Resource count validation inside `pulumi.RunErr` can have timing issues.

**Solution:**
```go
// ❌ WRONG - Inside Pulumi context
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    // ❌ This can be flaky
    assert.Equal(t, 1, mocks.GetResourceCount("kubernetes:core/v1:Service"))
    return err
}, pulumi.WithMocks("project", "stack", mocks))

// ✅ CORRECT - Outside Pulumi context
err := pulumi.RunErr(func(ctx *pulumi.Context) error {
    sc, err := NewSimpleContainer(ctx, args)
    // ✅ Only validate resource creation, not counts
    Expect(err).ToNot(HaveOccurred())
    Expect(sc).ToNot(BeNil())
    return err
}, pulumi.WithMocks("project", "stack", mocks))

Expect(err).ToNot(HaveOccurred())
// ✅ Validate counts AFTER Pulumi execution
Expect(mocks.GetResourceCount("kubernetes:core/v1:Service")).To(Equal(1))
```

### 4. Gomega Assertion Errors

**Error Pattern:**
```
Expected
    <*internal.OutputState>: &{...}
to equal
    <string>: "expected-value"
```

**Root Cause:** Trying to assert on Pulumi Output content directly.

**Solution:**
```go
// ❌ WRONG - Don't test Pulumi output content
Expect(sc.ServicePublicIP).To(Equal("203.0.113.12"))
Expect(sc.ServicePublicIP).To(ContainSubstring("203.0.113"))

// ✅ CORRECT - Only test output existence
Expect(sc.ServicePublicIP).ToNot(BeEmpty())
Expect(sc.ServicePublicIP).ToNot(BeNil())
```

### 5. Test Isolation Issues

**Error Pattern:**
Tests that fail when run together but pass when run individually.

**Root Cause:** Shared state between tests or improper mock cleanup.

**Solution:**
```go
func TestSomething(t *testing.T) {
    RegisterTestingT(t)
    
    // ✅ Create fresh mocks for each test
    mocks := NewSimpleContainerMocks()
    
    // ✅ Use unique names to avoid conflicts
    args := &SimpleContainerArgs{
        Namespace:  "test-namespace-" + t.Name(),
        Service:    "test-service-" + t.Name(),
        Deployment: "test-deployment-" + t.Name(),
        // ...
    }
    
    // ... test implementation
}
```

## Debugging Commands

### Run Tests with Race Detection
```bash
# Detect data races
go test -race ./pkg/clouds/pulumi/kubernetes/

# Run specific test with race detection
go test -race -run TestSpecificTest ./pkg/clouds/pulumi/kubernetes/
```

### Run Individual Tests
```bash
# Run single test
go test -run TestSimpleContainer_CreationSuccess ./pkg/clouds/pulumi/kubernetes/

# Run tests matching pattern
go test -run TestSimpleContainer_ ./pkg/clouds/pulumi/kubernetes/

# Verbose output
go test -v -run TestSimpleContainer_CreationSuccess ./pkg/clouds/pulumi/kubernetes/
```

### Performance Analysis
```bash
# Benchmark tests
go test -bench=. ./pkg/clouds/pulumi/kubernetes/

# CPU profiling
go test -cpuprofile=cpu.prof ./pkg/clouds/pulumi/kubernetes/

# Memory profiling
go test -memprofile=mem.prof ./pkg/clouds/pulumi/kubernetes/
```

## Mock Implementation Checklist

When creating or updating mocks, ensure:

- [ ] **Thread Safety**: All map access protected with mutexes
- [ ] **Nil Checks**: Safe handling of nil inputs
- [ ] **Resource Tracking**: Proper counting of created resources
- [ ] **Realistic Outputs**: Mock outputs match expected resource structure
- [ ] **Error Handling**: Proper error responses for invalid inputs

### Mock Template
```go
func (m *simpleContainerMocks) mockResourceType(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
    resourceID := fmt.Sprintf("%s-suffix", args.Name)

    // Add resource-specific outputs
    outputs["status"] = map[string]interface{}{
        "field": "value",
    }

    // Store for validation (thread-safe)
    props := resource.NewPropertyMapFromMap(outputs)
    m.mu.Lock()
    m.createdResources[resourceID] = props
    m.mu.Unlock()

    return resourceID, props, nil
}
```

## Test Structure Checklist

For each test function, ensure:

- [ ] **RegisterTestingT**: Called at the beginning of each test
- [ ] **Fresh Mocks**: New mock instance for each test
- [ ] **Unique Names**: Avoid conflicts between tests
- [ ] **Proper Timing**: Resource validation outside `pulumi.RunErr`
- [ ] **Error Handling**: Proper assertion of success/failure
- [ ] **Cleanup**: No shared state between tests

### Test Template
```go
func TestSimpleContainer_FeatureName(t *testing.T) {
    RegisterTestingT(t) // ✅ Required for Gomega
    
    mocks := NewSimpleContainerMocks() // ✅ Fresh mocks
    
    args := &SimpleContainerArgs{
        Namespace:  "test-namespace",
        Service:    "test-service", 
        Deployment: "test-deployment",
        // ... test-specific configuration
    }
    
    err := pulumi.RunErr(func(ctx *pulumi.Context) error {
        sc, err := NewSimpleContainer(ctx, args)
        
        // ✅ Only validate basic resource creation
        Expect(err).ToNot(HaveOccurred())
        Expect(sc).ToNot(BeNil())
        
        return nil
    }, pulumi.WithMocks("project", "stack", mocks))
    
    Expect(err).ToNot(HaveOccurred())
    
    // ✅ Validate resource counts outside Pulumi context
    Expect(mocks.GetResourceCount("kubernetes:core/v1:Namespace")).To(Equal(1))
    // ... other validations
}
```

## Performance Troubleshooting

### Slow Tests

**Symptoms:** Tests taking > 1 second each or > 30 seconds for full suite.

**Common Causes & Solutions:**

1. **Heavy Mock Operations**
   ```go
   // ❌ Expensive operations in mocks
   func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
       time.Sleep(100 * time.Millisecond) // Don't do this
       // ... heavy computation
   }
   
   // ✅ Lightweight mock operations
   func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
       // Simple, fast operations only
       return fmt.Sprintf("%s-id", args.Name), resource.NewPropertyMapFromMap(outputs), nil
   }
   ```

2. **Excessive Resource Creation**
   ```go
   // ❌ Creating unnecessary resources
   args := &SimpleContainerArgs{
       Containers: make([]Container, 100), // Too many
   }
   
   // ✅ Minimal test configuration
   args := &SimpleContainerArgs{
       Containers: []Container{
           {Name: "app", Image: "nginx:latest"},
       },
   }
   ```

3. **Missing Parallel Execution**
   ```go
   func TestMultipleScenarios(t *testing.T) {
       for _, tc := range testCases {
           tc := tc
           t.Run(tc.name, func(t *testing.T) {
               t.Parallel() // ✅ Enable parallel execution
               // ... test implementation
           })
       }
   }
   ```

### Memory Issues

**Symptoms:** High memory usage or out-of-memory errors during tests.

**Solutions:**

1. **Proper Mock Cleanup**
   ```go
   type simpleContainerMocks struct {
       mu sync.RWMutex
       resourceCounts   map[string]int
       createdResources map[string]resource.PropertyMap
   }
   
   // ✅ Add cleanup method if needed
   func (m *simpleContainerMocks) Reset() {
       m.mu.Lock()
       defer m.mu.Unlock()
       m.resourceCounts = make(map[string]int)
       m.createdResources = make(map[string]resource.PropertyMap)
   }
   ```

2. **Avoid Large Test Data**
   ```go
   // ❌ Large test configurations
   args.Containers = make([]Container, 1000)
   
   // ✅ Minimal necessary configuration
   args.Containers = []Container{
       {Name: "app", Image: "nginx:latest"},
   }
   ```

## Integration with CI/CD

### GitHub Actions Configuration

```yaml
- name: Run Pulumi Unit Tests
  run: |
    # Run with race detection
    go test -race ./pkg/clouds/pulumi/kubernetes/
    
    # Run with coverage
    go test -coverprofile=coverage.out ./pkg/clouds/pulumi/kubernetes/
    
    # Generate coverage report
    go tool cover -html=coverage.out -o coverage.html
```

### Test Quality Gates

```bash
#!/bin/bash
# test-quality-gate.sh

# Run tests with race detection
echo "Running tests with race detection..."
if ! go test -race ./pkg/clouds/pulumi/kubernetes/; then
    echo "❌ Tests failed or data races detected"
    exit 1
fi

# Check test coverage
echo "Checking test coverage..."
go test -coverprofile=coverage.out ./pkg/clouds/pulumi/kubernetes/
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')

if (( $(echo "$COVERAGE < 80" | bc -l) )); then
    echo "❌ Test coverage ($COVERAGE%) below threshold (80%)"
    exit 1
fi

echo "✅ All quality gates passed"
echo "✅ Test coverage: $COVERAGE%"
```

## Quick Fix Checklist

When tests are failing, check these common issues in order:

1. **[ ] Data Races**: Run with `-race` flag
2. **[ ] Nil Pointers**: Check mock input handling
3. **[ ] Timing Issues**: Move validations outside `pulumi.RunErr`
4. **[ ] Test Isolation**: Ensure fresh mocks and unique names
5. **[ ] Gomega Setup**: Verify `RegisterTestingT(t)` is called
6. **[ ] Output Assertions**: Only test existence, not content
7. **[ ] Thread Safety**: Verify mutex protection in mocks
8. **[ ] Resource Counting**: Check mock tracking implementation

Following this troubleshooting guide should resolve 95% of common Pulumi unit testing issues in Simple Container.
