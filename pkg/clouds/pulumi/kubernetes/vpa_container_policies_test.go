package kubernetes

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// renderVPAContainerPolicies runs NewSimpleContainer with the given VPA config
// and returns the captured spec.resourcePolicy.containerPolicies array.
func renderVPAContainerPolicies(t *testing.T, vpa *k8s.VPAConfig) []resource.PropertyValue {
	t.Helper()
	mocks := NewSimpleContainerMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		args := &SimpleContainerArgs{
			Namespace:  "myapp",
			Service:    "myapp",
			Deployment: "myapp",
			ScEnv:      "production",
			Domain:     "myapp.example.com",
			Prefix:     "/",
			Replicas:   2,
			IngressContainer: &k8s.CloudRunContainer{
				Name: "myapp", MainPort: lo.ToPtr(8080), Ports: []int{8080},
			},
			Log: logger.New(),
			Containers: []corev1.ContainerArgs{
				{Name: sdk.String("myapp"), Image: sdk.String("nginx:latest")},
				{Name: sdk.String("cloudsql-proxy"), Image: sdk.String("cloud-sql-proxy:latest")},
			},
			VPA: vpa,
		}
		_, err := NewSimpleContainer(ctx, args)
		return err
	}, sdk.WithMocks("test", "test", mocks))
	require.NoError(t, err)

	mocks.mu.RLock()
	defer mocks.mu.RUnlock()
	for _, props := range mocks.createdResources {
		kind, ok := props["kind"]
		if !ok || !kind.IsString() || kind.StringValue() != "VerticalPodAutoscaler" {
			continue
		}
		require.True(t, props["spec"].IsObject(), "VPA spec should be an object")
		rp := props["spec"].ObjectValue()["resourcePolicy"]
		require.True(t, rp.IsObject(), "VPA spec.resourcePolicy should be an object")
		cps := rp.ObjectValue()["containerPolicies"]
		require.True(t, cps.IsArray(), "resourcePolicy.containerPolicies should be an array")
		return cps.ArrayValue()
	}
	t.Fatal("VPA should have been created and captured by the mock")
	return nil
}

// VPA.ContainerPolicies must render per-container entries alongside the "*"
// catch-all. The motivating case: an injected sidecar (cloudsql-proxy) set to
// mode "Off" so it keeps its small template request instead of being floored at
// the app container's minAllowed (the "*" policy).
func TestCreateVPA_PerContainerPolicies(t *testing.T) {
	arr := renderVPAContainerPolicies(t, &k8s.VPAConfig{
		Enabled:             true,
		UpdateMode:          lo.ToPtr("Auto"),
		MinAllowed:          &k8s.VPAResourceRequirements{CPU: lo.ToPtr("250m"), Memory: lo.ToPtr("64Mi")},
		ControlledResources: []string{"cpu", "memory"},
		ControlledValues:    lo.ToPtr("RequestsOnly"),
		ContainerPolicies: []k8s.VPAContainerPolicy{
			{ContainerName: "cloudsql-proxy", Mode: lo.ToPtr("Off")},
		},
	})
	require.Len(t, arr, 2, "expected exactly the '*' catch-all + the cloudsql-proxy override")

	sawStar, sawProxy := false, false
	for _, e := range arr {
		require.True(t, e.IsObject())
		o := e.ObjectValue()
		require.True(t, o["containerName"].IsString())
		switch o["containerName"].StringValue() {
		case "*":
			sawStar = true
			assert.Equal(t, "RequestsOnly", o["controlledValues"].StringValue())
			require.True(t, o["minAllowed"].IsObject())
			assert.Equal(t, "250m", o["minAllowed"].ObjectValue()["cpu"].StringValue())
		case "cloudsql-proxy":
			sawProxy = true
			require.True(t, o["mode"].IsString())
			assert.Equal(t, "Off", o["mode"].StringValue(), "sidecar must be excluded from VPA via mode Off")
			_, hasMin := o["minAllowed"]
			assert.False(t, hasMin, "sidecar policy must not inherit the app minAllowed floor")
		default:
			t.Fatalf("unexpected containerPolicy entry: %s", o["containerName"].StringValue())
		}
	}
	assert.True(t, sawStar, "expected a '*' catch-all containerPolicy")
	assert.True(t, sawProxy, "expected a per-container policy for cloudsql-proxy")
}

// Backward compat: with no ContainerPolicies, the render is the historical
// single "*" entry carrying the top-level floor/controlled* fields. Locks the
// guarantee that existing stacks re-render byte-identical (no Pulumi diff).
func TestCreateVPA_NoContainerPolicies_BackwardCompat(t *testing.T) {
	arr := renderVPAContainerPolicies(t, &k8s.VPAConfig{
		Enabled:             true,
		UpdateMode:          lo.ToPtr("Auto"),
		MinAllowed:          &k8s.VPAResourceRequirements{CPU: lo.ToPtr("250m"), Memory: lo.ToPtr("64Mi")},
		MaxAllowed:          &k8s.VPAResourceRequirements{CPU: lo.ToPtr("2"), Memory: lo.ToPtr("2560Mi")},
		ControlledResources: []string{"cpu", "memory"},
		ControlledValues:    lo.ToPtr("RequestsOnly"),
	})
	require.Len(t, arr, 1, "no ContainerPolicies must render exactly one '*' entry")
	o := arr[0].ObjectValue()
	assert.Equal(t, "*", o["containerName"].StringValue())
	assert.Equal(t, "RequestsOnly", o["controlledValues"].StringValue())
	require.True(t, o["controlledResources"].IsArray())
	assert.Len(t, o["controlledResources"].ArrayValue(), 2)
	assert.Equal(t, "250m", o["minAllowed"].ObjectValue()["cpu"].StringValue())
	assert.Equal(t, "2560Mi", o["maxAllowed"].ObjectValue()["memory"].StringValue())
}

// ContainerPolicies with no top-level fields renders ONLY the per-container
// entries — no "*" catch-all (hasTopLevel == false branch). Also exercises a
// per-container minAllowed override (not just mode Off).
func TestCreateVPA_ContainerPoliciesOnly_NoStar(t *testing.T) {
	arr := renderVPAContainerPolicies(t, &k8s.VPAConfig{
		Enabled:    true,
		UpdateMode: lo.ToPtr("Auto"),
		ContainerPolicies: []k8s.VPAContainerPolicy{
			{ContainerName: "cloudsql-proxy", Mode: lo.ToPtr("Off")},
			{ContainerName: "myapp", MinAllowed: &k8s.VPAResourceRequirements{CPU: lo.ToPtr("100m")}},
		},
	})
	require.Len(t, arr, 2, "containerPolicies-only must render exactly the listed entries, no '*'")
	for _, e := range arr {
		o := e.ObjectValue()
		assert.NotEqual(t, "*", o["containerName"].StringValue(), "no '*' catch-all when no top-level fields are set")
		if o["containerName"].StringValue() == "myapp" {
			require.True(t, o["minAllowed"].IsObject())
			assert.Equal(t, "100m", o["minAllowed"].ObjectValue()["cpu"].StringValue())
		}
	}
}

func TestValidateVPAConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		vpa     *k8s.VPAConfig
		wantErr string
	}{
		{"nil", nil, ""},
		{"disabled", &k8s.VPAConfig{Enabled: false, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: ""}}}, ""},
		{"valid", &k8s.VPAConfig{Enabled: true, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: "cloudsql-proxy", Mode: lo.ToPtr("Off")}}}, ""},
		{"empty name", &k8s.VPAConfig{Enabled: true, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: ""}}}, "must not be empty"},
		{"star reserved", &k8s.VPAConfig{Enabled: true, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: "*"}}}, "reserved"},
		{"duplicate", &k8s.VPAConfig{Enabled: true, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: "x"}, {ContainerName: "x"}}}, "duplicate"},
		{"bad mode", &k8s.VPAConfig{Enabled: true, ContainerPolicies: []k8s.VPAContainerPolicy{{ContainerName: "x", Mode: lo.ToPtr("off")}}}, "mode must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateVPAConfiguration(tc.vpa)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}
