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
	case "kubernetes:core/v1:ConfigMap":
		return m.mockConfigMap(args, outputs)
	case "kubernetes:core/v1:Secret":
		return m.mockSecret(args, outputs)
	case "kubernetes:core/v1:PersistentVolumeClaim":
		return m.mockPVC(args, outputs)
	case "kubernetes:networking.k8s.io/v1:Ingress":
		return m.mockIngress(args, outputs)
	case "kubernetes:policy/v1:PodDisruptionBudget":
		return m.mockPDB(args, outputs)
	case "kubernetes:autoscaling/v2:HorizontalPodAutoscaler":
		return m.mockHPA(args, outputs)
	case "kubernetes:apiextensions.k8s.io/v1:CustomResource":
		return m.mockCustomResource(args, outputs)
	default:
		return resourceID, resource.NewPropertyMapFromMap(outputs), nil
	}
}

// Call mocks function calls (not used in SimpleContainer but required by interface)
func (m *simpleContainerMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.NewPropertyMapFromMap(map[string]interface{}{}), nil
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

// Mock implementations for each resource type

func (m *simpleContainerMocks) mockNamespace(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-ns", args.Name)

	// Add namespace-specific outputs
	outputs["status"] = map[string]interface{}{
		"phase": "Active",
	}

	// Store for validation (thread-safe)
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

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

func (m *simpleContainerMocks) mockConfigMap(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-cm", args.Name)

	// ConfigMaps don't have complex status, just return inputs
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

func (m *simpleContainerMocks) mockSecret(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-secret", args.Name)

	// Secrets don't have complex status, just return inputs
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

func (m *simpleContainerMocks) mockPVC(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-pvc", args.Name)

	// Add PVC-specific outputs
	outputs["status"] = map[string]interface{}{
		"phase": "Bound",
	}

	// Store for validation (thread-safe)
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

func (m *simpleContainerMocks) mockIngress(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-ingress", args.Name)

	// Add ingress-specific outputs
	outputs["status"] = map[string]interface{}{
		"loadBalancer": map[string]interface{}{
			"ingress": []map[string]interface{}{
				{
					"ip": "203.0.113.13",
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

func (m *simpleContainerMocks) mockPDB(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-pdb", args.Name)

	// Add PDB-specific outputs
	outputs["status"] = map[string]interface{}{
		"currentHealthy": 1,
		"desiredHealthy": 1,
	}

	// Store for validation (thread-safe)
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

func (m *simpleContainerMocks) mockHPA(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-hpa", args.Name)

	// Add HPA-specific outputs
	outputs["status"] = map[string]interface{}{
		"currentReplicas": 2,
		"desiredReplicas": 2,
	}

	// Store for validation (thread-safe)
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}

func (m *simpleContainerMocks) mockCustomResource(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	resourceID := fmt.Sprintf("%s-cr", args.Name)

	// For VPA custom resources
	if kind, ok := outputs["kind"]; ok && kind == "VerticalPodAutoscaler" {
		outputs["status"] = map[string]interface{}{
			"recommendation": map[string]interface{}{
				"containerRecommendations": []map[string]interface{}{
					{
						"containerName": "app",
						"target": map[string]interface{}{
							"cpu":    "100m",
							"memory": "128Mi",
						},
					},
				},
			},
		}
	}

	// Store for validation (thread-safe)
	props := resource.NewPropertyMapFromMap(outputs)
	m.mu.Lock()
	m.createdResources[resourceID] = props
	m.mu.Unlock()

	return resourceID, props, nil
}
