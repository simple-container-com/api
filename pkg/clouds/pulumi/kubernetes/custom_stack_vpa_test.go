package kubernetes

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Regression: DeploySimpleContainer passes the already parentEnv-suffixed name as
// Service/Deployment (e.g. "myapp-tenant-a" for ScEnv=tenant-a, parentEnv=production).
// NewSimpleContainer must derive the VPA name AND its spec.targetRef from that name
// directly, not re-apply the suffix. Re-applying produced "myapp-tenant-a-tenant-a",
// so a custom-stack VPA targeted a non-existent deployment and never right-sized.
func TestNewSimpleContainer_CustomStackVPATargetsDeployment(t *testing.T) {
	mocks := NewSimpleContainerMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "myapp",
			Service:    "myapp-tenant-a",
			Deployment: "myapp-tenant-a",
			ScEnv:      "tenant-a",
			ParentEnv:  lo.ToPtr("production"),
			Domain:     "tenant-a.example.com",
			Prefix:     "/",
			Replicas:   2,
			IngressContainer: &k8s.CloudRunContainer{
				Name:     "myapp-tenant-a",
				MainPort: lo.ToPtr(8080),
				Ports:    []int{8080},
			},
			Log: logger.New(),
			Containers: []corev1.ContainerArgs{
				{Name: sdk.String("myapp-tenant-a"), Image: sdk.String("nginx:latest")},
			},
			Scale: &k8s.Scale{EnableHPA: true, MinReplicas: 2, MaxReplicas: 4, CPUTarget: lo.ToPtr(80)},
			VPA: &k8s.VPAConfig{
				Enabled:    true,
				UpdateMode: lo.ToPtr("Auto"),
				MinAllowed: &k8s.VPAResourceRequirements{CPU: lo.ToPtr("250m")},
			},
		}
		_, err := NewSimpleContainer(ctx, args)
		return err
	}, sdk.WithMocks("test", "test", mocks))
	require.NoError(t, err)

	mocks.mu.RLock()
	defer mocks.mu.RUnlock()

	// The VPA must target the actual (single-suffixed) Deployment, not a double-suffixed name.
	// Only VPA/HPA names derive from baseResourceName (the fix); the configmap/secret names
	// are intentionally left as-is, so scope the double-suffix guard to VPA/HPA.
	foundVPA := false
	for id, props := range mocks.createdResources {
		if strings.HasSuffix(id, "-vpa-cr") || strings.Contains(id, "-hpa") {
			assert.NotContains(t, id, "tenant-a-tenant-a", "VPA/HPA name double-suffixed: %s", id)
		}

		kind, ok := props["kind"]
		if !ok || !kind.IsString() || kind.StringValue() != "VerticalPodAutoscaler" {
			continue
		}
		foundVPA = true

		require.True(t, props["metadata"].IsObject(), "VPA metadata should be an object")
		vpaName := props["metadata"].ObjectValue()["name"]
		require.True(t, vpaName.IsString())
		assert.Equal(t, "myapp-tenant-a-vpa", vpaName.StringValue(), "VPA name double-suffixed")

		require.True(t, props["spec"].IsObject(), "VPA spec should be an object")
		targetRef := props["spec"].ObjectValue()["targetRef"]
		require.True(t, targetRef.IsObject(), "VPA spec.targetRef should be an object")
		refName := targetRef.ObjectValue()["name"]
		require.True(t, refName.IsString())
		assert.Equal(t, "myapp-tenant-a", refName.StringValue(),
			"VPA must target the real Deployment, not a double-suffixed name")
		assert.False(t, strings.Contains(refName.StringValue(), "tenant-a-tenant-a"))
	}
	require.True(t, foundVPA, "VPA should have been created and captured by the mock")
}
