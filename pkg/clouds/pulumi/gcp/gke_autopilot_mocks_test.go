package gcp

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// gkeAutopilotMocks implements pulumi.MockResourceMonitor for testing GKE Autopilot
type gkeAutopilotMocks struct {
	// Track resource creation counts by type
	resourceCounts map[string]int
	// Store created resources for validation
	createdResources map[string]resource.PropertyMap
	// Control whether to simulate failures
	simulateFailures map[string]bool
}

// newGkeAutopilotMocks creates a new mock instance
func newGkeAutopilotMocks() *gkeAutopilotMocks {
	return &gkeAutopilotMocks{
		resourceCounts:   make(map[string]int),
		createdResources: make(map[string]resource.PropertyMap),
		simulateFailures: make(map[string]bool),
	}
}

// NewResource mocks the creation of GCP resources
func (m *gkeAutopilotMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	// Check for simulated failures
	if m.simulateFailures[args.TypeToken] {
		return "", nil, fmt.Errorf("simulated failure for %s", args.TypeToken)
	}

	// Check if this is a resource lookup (has ID) vs creation (no ID)
	isLookup := args.ID != ""

	// Only track resource creation for actual new resources, not lookups
	if !isLookup {
		m.resourceCounts[args.TypeToken]++
	}

	// Generate unique resource ID (will be used in switch cases)
	_ = fmt.Sprintf("%s-%d", args.Name, m.resourceCounts[args.TypeToken])

	// Get input properties
	outputs := args.Inputs.Mappable()

	// Add mock outputs based on resource type
	switch args.TypeToken {
	case "gcp:container/cluster:Cluster":
		return m.mockGkeCluster(args, outputs)
	case "gcp:container:Cluster": // Alternative format
		return m.mockGkeCluster(args, outputs)
	case "gcp:container/Cluster": // Another alternative format
		return m.mockGkeCluster(args, outputs)
	case "gcp:compute/address:Address":
		return m.mockStaticIP(args, outputs)
	case "gcp:compute/router:Router":
		return m.mockCloudRouter(args, outputs)
	case "gcp:compute/routerNat:RouterNat":
		return m.mockCloudNat(args, outputs)
	case "gcp:storage/bucket:Bucket":
		return m.mockGcsBucket(args, outputs)
	case "gcp:serviceaccount/account:Account":
		return m.mockServiceAccount(args, outputs)
	case "gcp:serviceaccount/key:Key":
		return m.mockServiceAccountKey(args, outputs)
	case "kubernetes:core/v1:Namespace":
		return m.mockKubernetesNamespace(args, outputs)
	case "kubernetes:apps/v1:Deployment":
		return m.mockKubernetesDeployment(args, outputs)
	case "kubernetes:core/v1:Service":
		return m.mockKubernetesService(args, outputs)
	default:
		// Generic mock for unknown resource types
		return m.mockGenericResource(args, outputs)
	}
}

// Call mocks function calls (handles LookupCluster for adoption tests)
func (m *gkeAutopilotMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch args.Token {
	case "gcp:container/getCluster:getCluster":
		// Mock the LookupCluster call for adoption tests
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"name":             "existing-cluster",
			"location":         "us-central1",
			"minMasterVersion": "1.28",
			"enableAutopilot":  false, // Simulate non-autopilot cluster
			"project":          "test-project",
		}), nil
	default:
		return resource.NewPropertyMapFromMap(map[string]interface{}{}), nil
	}
}

// Mock implementations for specific resource types

func (m *gkeAutopilotMocks) mockGkeCluster(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add GKE cluster-specific outputs
	outputs["endpoint"] = "https://203.0.113.10"
	outputs["masterAuth"] = map[string]interface{}{
		"clusterCaCertificate": "LS0tLS1CRUdJTi0tLS0t", // Base64 mock cert
	}
	outputs["project"] = "test-project"
	outputs["location"] = "us-central1" // Default mock location
	outputs["status"] = "RUNNING"

	// Store for validation
	resourceID := fmt.Sprintf("%s-cluster-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockStaticIP(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add static IP-specific outputs
	outputs["address"] = "203.0.113.42"
	outputs["selfLink"] = fmt.Sprintf("projects/test-project/regions/us-central1/addresses/%s", args.Name)
	outputs["addressType"] = "EXTERNAL"
	outputs["status"] = "RESERVED"

	resourceID := fmt.Sprintf("%s-ip-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockCloudRouter(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Cloud Router-specific outputs
	outputs["selfLink"] = fmt.Sprintf("projects/test-project/regions/us-central1/routers/%s", args.Name)
	outputs["creationTimestamp"] = "2023-01-01T00:00:00.000-00:00"

	resourceID := fmt.Sprintf("%s-router-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockCloudNat(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Cloud NAT-specific outputs
	outputs["selfLink"] = fmt.Sprintf("projects/test-project/regions/us-central1/routers/test-router/nats/%s", args.Name)
	outputs["natIpAllocateOption"] = "MANUAL_ONLY"

	resourceID := fmt.Sprintf("%s-nat-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockGcsBucket(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add GCS bucket-specific outputs
	outputs["url"] = fmt.Sprintf("gs://%s", args.Name)
	outputs["selfLink"] = fmt.Sprintf("https://www.googleapis.com/storage/v1/b/%s", args.Name)

	resourceID := fmt.Sprintf("%s-bucket-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockServiceAccount(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Service Account-specific outputs
	outputs["email"] = fmt.Sprintf("%s@test-project.iam.gserviceaccount.com", args.Name)
	outputs["uniqueId"] = "123456789012345678901"

	resourceID := fmt.Sprintf("%s-sa-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockServiceAccountKey(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Service Account Key-specific outputs (base64 encoded JSON)
	_ = `{"type":"service_account","project_id":"test-project","private_key_id":"key123"}` // mockKey not used
	outputs["privateKey"] = "eyJ0eXBlIjoic2VydmljZV9hY2NvdW50In0="                         // base64 of mock JSON

	resourceID := fmt.Sprintf("%s-key-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockKubernetesNamespace(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Kubernetes namespace-specific outputs
	outputs["status"] = map[string]interface{}{
		"phase": "Active",
	}

	resourceID := fmt.Sprintf("%s-ns-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockKubernetesDeployment(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Kubernetes deployment-specific outputs
	outputs["status"] = map[string]interface{}{
		"readyReplicas":     1,
		"availableReplicas": 1,
		"replicas":          1,
	}

	resourceID := fmt.Sprintf("%s-deploy-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockKubernetesService(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Add Kubernetes service-specific outputs
	outputs["status"] = map[string]interface{}{
		"loadBalancer": map[string]interface{}{
			"ingress": []interface{}{
				map[string]interface{}{
					"ip": "203.0.113.100",
				},
			},
		},
	}

	resourceID := fmt.Sprintf("%s-svc-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

func (m *gkeAutopilotMocks) mockGenericResource(args pulumi.MockResourceArgs, outputs map[string]interface{}) (string, resource.PropertyMap, error) {
	// Generic mock for unknown resource types
	resourceID := fmt.Sprintf("%s-generic-id", args.Name)
	m.createdResources[resourceID] = resource.NewPropertyMapFromMap(outputs)

	return resourceID, resource.NewPropertyMapFromMap(outputs), nil
}

// Helper methods for test validation

// GetResourceCount returns the number of resources created of a specific type
func (m *gkeAutopilotMocks) GetResourceCount(resourceType string) int {
	return m.resourceCounts[resourceType]
}

// GetCreatedResource returns the properties of a created resource by ID
func (m *gkeAutopilotMocks) GetCreatedResource(resourceID string) (resource.PropertyMap, bool) {
	props, exists := m.createdResources[resourceID]
	return props, exists
}

// SimulateFailure configures the mock to simulate failures for a specific resource type
func (m *gkeAutopilotMocks) SimulateFailure(resourceType string, shouldFail bool) {
	m.simulateFailures[resourceType] = shouldFail
}

// Reset clears all tracked data (useful between tests)
func (m *gkeAutopilotMocks) Reset() {
	m.resourceCounts = make(map[string]int)
	m.createdResources = make(map[string]resource.PropertyMap)
	m.simulateFailures = make(map[string]bool)
}
