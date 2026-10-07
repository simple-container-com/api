// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

const safeToEvict = "cluster-autoscaler.kubernetes.io/safe-to-evict"

func TestCaddyPodAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *k8s.CaddyConfig
		want    map[string]string
		wantErr string
	}{
		{name: "nil_config", cfg: nil},
		{name: "unset_field", cfg: &k8s.CaddyConfig{}},
		{
			name: "valid_key_passes_through",
			cfg:  &k8s.CaddyConfig{PodAnnotations: map[string]string{safeToEvict: "false"}},
			want: map[string]string{safeToEvict: "false"},
		},
		{
			name:    "caddyfile_entry_is_reserved",
			cfg:     &k8s.CaddyConfig{PodAnnotations: map[string]string{AnnotationCaddyfileEntry: "evil.example.com { }"}},
			wantErr: `reserved prefix "simple-container.com/"`,
		},
		{
			name:    "pulumi_keys_are_reserved",
			cfg:     &k8s.CaddyConfig{PodAnnotations: map[string]string{"pulumi.com/patchForce": "false"}},
			wantErr: `reserved prefix "pulumi.com/"`,
		},
		{
			name:    "invalid_key_rejected",
			cfg:     &k8s.CaddyConfig{PodAnnotations: map[string]string{"not a key": "x"}},
			wantErr: `invalid key "not a key"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := caddyPodAnnotations(tt.cfg)
			if tt.wantErr != "" {
				Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
				return
			}
			Expect(err).ToNot(HaveOccurred())
			if tt.want == nil {
				Expect(got).To(BeEmpty())
			} else {
				Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func deployTestCaddy(cfg *k8s.CaddyConfig) (*simpleContainerMocks, error) {
	mocks := NewSimpleContainerMocks()
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		input := api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Name: "cluster", Type: k8s.ResourceTypeCaddy},
			StackParams: &api.StackParams{StackName: "infra", Environment: "production"},
		}
		params := pApi.ProvisionParams{
			Log:            logger.New(),
			ComputeContext: pApi.NewComputeContextCollector(context.Background(), logger.New(), "infra", "production"),
		}
		_, err := DeployCaddyService(ctx, CaddyDeployment{CaddyConfig: cfg, ClusterName: "cluster"},
			input, params, sdk.String("").ToStringOutput())
		return err
	}, sdk.WithMocks("test", "test", mocks))
	return mocks, err
}

func annotationsAt(props resource.PropertyMap, path ...string) map[string]interface{} {
	cur := props.Mappable()
	for _, p := range path {
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func TestDeployCaddyService_PodAnnotationsReachOnlyThePodTemplate(t *testing.T) {
	RegisterTestingT(t)
	mocks, err := deployTestCaddy(&k8s.CaddyConfig{PodAnnotations: map[string]string{safeToEvict: "false"}})
	Expect(err).ToNot(HaveOccurred())

	deployments := mocks.InputsFor("kubernetes:apps/v1:Deployment")
	Expect(deployments).To(HaveLen(1))
	pod := annotationsAt(deployments[0], "spec", "template", "metadata", "annotations")
	Expect(pod).To(HaveKeyWithValue(safeToEvict, "false"))
	Expect(pod).To(HaveKeyWithValue("pulumi.com/patchForce", "true"))
	Expect(annotationsAt(deployments[0], "metadata", "annotations")).ToNot(HaveKey(safeToEvict))

	for _, typ := range []string{
		"kubernetes:core/v1:Service",
		"kubernetes:core/v1:Namespace",
		"kubernetes:policy/v1:PodDisruptionBudget",
		"kubernetes:core/v1:ConfigMap",
	} {
		for _, res := range mocks.InputsFor(typ) {
			Expect(annotationsAt(res, "metadata", "annotations")).ToNot(HaveKey(safeToEvict), typ)
		}
	}
}

func TestDeployCaddyService_RejectsReservedPodAnnotation(t *testing.T) {
	RegisterTestingT(t)
	mocks, err := deployTestCaddy(&k8s.CaddyConfig{PodAnnotations: map[string]string{AnnotationCaddyfileEntry: "x"}})
	Expect(err).To(MatchError(ContainSubstring("reserved prefix")))
	Expect(mocks.GetResourceCount("kubernetes:apps/v1:Deployment")).To(BeZero())
}

func TestNewSimpleContainer_SCAnnotationsWinOverPodAnnotations(t *testing.T) {
	RegisterTestingT(t)
	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()
	args.PodAnnotations = map[string]string{AnnotationEnv: "spoofed", "team": "edge"}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := NewSimpleContainer(ctx, args)
		return err
	}, sdk.WithMocks("project", "stack", mocks))
	Expect(err).ToNot(HaveOccurred())

	deployments := mocks.InputsFor("kubernetes:apps/v1:Deployment")
	Expect(deployments).To(HaveLen(1))
	pod := annotationsAt(deployments[0], "spec", "template", "metadata", "annotations")
	Expect(pod).To(HaveKeyWithValue(AnnotationEnv, args.ScEnv))
	Expect(pod).To(HaveKeyWithValue("team", "edge"))
}

func TestDeployCaddyService_TopologySpreadReachesPodSpec(t *testing.T) {
	RegisterTestingT(t)
	mocks, err := deployTestCaddy(&k8s.CaddyConfig{
		TopologySpreadConstraints: []k8s.TopologySpreadConstraint{
			{TopologyKey: "kubernetes.io/hostname", WhenUnsatisfiable: "ScheduleAnyway"},
		},
	})
	Expect(err).ToNot(HaveOccurred())

	deployments := mocks.InputsFor("kubernetes:apps/v1:Deployment")
	Expect(deployments).To(HaveLen(1))
	spec := annotationsAt(deployments[0], "spec", "template", "spec")
	tsc, ok := spec["topologySpreadConstraints"].([]interface{})
	Expect(ok).To(BeTrue(), "pod spec has no topologySpreadConstraints")
	Expect(tsc).To(HaveLen(1))
	c := tsc[0].(map[string]interface{})
	Expect(c).To(HaveKeyWithValue("topologyKey", "kubernetes.io/hostname"))
	Expect(c).To(HaveKeyWithValue("whenUnsatisfiable", "ScheduleAnyway"))
	Expect(c).To(HaveKey("labelSelector"))
}
