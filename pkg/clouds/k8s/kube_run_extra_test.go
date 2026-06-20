// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/compose-spec/compose-go/types"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

// TestKubeRunInput_Getters covers the thin accessors that surface fields of the
// embedded StackConfigCompose.
func TestKubeRunInput_Getters(t *testing.T) {
	RegisterTestingT(t)

	deps := []api.StackConfigDependencyResource{
		{Name: "shared-db", Owner: "other-stack", Resource: "postgres"},
	}
	input := &KubeRunInput{
		Deployment: DeploymentConfig{
			StackConfig: &api.StackConfigCompose{
				BaseDnsZone:  "example.com",
				Dependencies: deps,
				Uses:         []string{"resA", "resB"},
			},
		},
	}

	Expect(input.OverriddenBaseZone()).To(Equal("example.com"))
	Expect(input.DependsOnResources()).To(Equal(deps))
	Expect(input.Uses()).To(ConsistOf("resA", "resB"))
}

func TestKubeRunInput_Getters_Empty(t *testing.T) {
	RegisterTestingT(t)

	input := &KubeRunInput{
		Deployment: DeploymentConfig{StackConfig: &api.StackConfigCompose{}},
	}

	Expect(input.OverriddenBaseZone()).To(Equal(""))
	Expect(input.DependsOnResources()).To(BeNil())
	Expect(input.Uses()).To(BeNil())
}

func TestToKubernetesRunConfig_Errors(t *testing.T) {
	RegisterTestingT(t)

	t.Run("template config of wrong type", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToKubernetesRunConfig("not-a-template", compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is not of type"))
	})

	t.Run("nil typed template config", func(t *testing.T) {
		RegisterTestingT(t)
		var tpl *CloudrunTemplate
		_, err := ToKubernetesRunConfig(tpl, compose.Config{Project: &types.Project{}}, &api.StackConfigCompose{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template config is nil"))
	})

	t.Run("missing service in compose config", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{Runs: []string{"ghost"}}
		_, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("service ghost not found"))
	})

	t.Run("malformed cloudExtras fails conversion", func(t *testing.T) {
		RegisterTestingT(t)
		// A scalar string cannot be unmarshalled into the CloudExtras struct, so
		// ConvertDescriptor errors and the wrap message surfaces.
		cloudExtras := any("not-a-mapping")
		stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &cloudExtras}
		_, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to convert cloudExtras field"))
	})

	t.Run("multiple ingress containers surfaces detect error", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			ComposeFiles: []string{"docker-compose.yaml"},
			Services: []types.ServiceConfig{
				{Name: "web", Labels: types.Labels{api.ComposeLabelIngressContainer: "true"}},
				{Name: "api", Labels: types.Labels{api.ComposeLabelIngressContainer: "true"}},
			},
		}}
		stackCfg := &api.StackConfigCompose{Runs: []string{"web", "api"}}
		_, err := ToKubernetesRunConfig(&CloudrunTemplate{}, composeCfg, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect ingress container"))
	})
}

func TestToKubernetesRunConfig_Minimal(t *testing.T) {
	RegisterTestingT(t)

	stackCfg := &api.StackConfigCompose{Runs: []string{}}
	res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)

	Expect(err).ToNot(HaveOccurred())
	input, ok := res.(*KubeRunInput)
	Expect(ok).To(BeTrue())
	Expect(input.Deployment.StackConfig).To(Equal(stackCfg))
	Expect(input.Deployment.Containers).To(BeNil())
	Expect(input.Deployment.IngressContainer).To(BeNil())
}

// TestToKubernetesRunConfig_AffinityMapping verifies the CloudExtras affinity
// rules are translated into the right GKE node-selector keys and that the full
// affinity config is preserved on the deployment.
func TestToKubernetesRunConfig_AffinityMapping(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nodePool and computeClass populate node selector", func(t *testing.T) {
		RegisterTestingT(t)
		cloudExtras := any(map[string]any{
			"affinity": map[string]any{
				"nodePool":     "pool-a",
				"computeClass": "Performance",
			},
		})
		stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &cloudExtras}

		res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)

		Expect(err).ToNot(HaveOccurred())
		input := res.(*KubeRunInput)
		Expect(input.Deployment.Affinity).ToNot(BeNil())
		Expect(input.Deployment.Affinity.NodePool).ToNot(BeNil())
		Expect(*input.Deployment.Affinity.NodePool).To(Equal("pool-a"))
		Expect(input.Deployment.NodeSelector).To(HaveKeyWithValue("cloud.google.com/gke-nodepool", "pool-a"))
		Expect(input.Deployment.NodeSelector).To(HaveKeyWithValue("node.kubernetes.io/instance-type", "Performance"))
	})

	t.Run("existing node selector is merged with affinity", func(t *testing.T) {
		RegisterTestingT(t)
		cloudExtras := any(map[string]any{
			"nodeSelector": map[string]any{"disktype": "ssd"},
			"affinity": map[string]any{
				"nodePool": "pool-b",
			},
		})
		stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &cloudExtras}

		res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)

		Expect(err).ToNot(HaveOccurred())
		input := res.(*KubeRunInput)
		Expect(input.Deployment.NodeSelector).To(HaveKeyWithValue("disktype", "ssd"))
		Expect(input.Deployment.NodeSelector).To(HaveKeyWithValue("cloud.google.com/gke-nodepool", "pool-b"))
	})

	t.Run("affinity without nodePool or computeClass leaves selector empty", func(t *testing.T) {
		RegisterTestingT(t)
		cloudExtras := any(map[string]any{
			"affinity": map[string]any{
				"exclusiveNodePool": true,
			},
		})
		stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &cloudExtras}

		res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)

		Expect(err).ToNot(HaveOccurred())
		input := res.(*KubeRunInput)
		Expect(input.Deployment.Affinity).ToNot(BeNil())
		Expect(input.Deployment.Affinity.ExclusiveNodePool).ToNot(BeNil())
		Expect(*input.Deployment.Affinity.ExclusiveNodePool).To(BeTrue())
		// Empty map allocated but no GKE keys set.
		Expect(input.Deployment.NodeSelector).To(BeEmpty())
	})
}

// TestToKubernetesRunConfig_CloudExtrasPassthrough verifies the assorted
// CloudExtras fields are threaded into the DeploymentConfig.
func TestToKubernetesRunConfig_CloudExtrasPassthrough(t *testing.T) {
	RegisterTestingT(t)

	cloudExtras := any(map[string]any{
		"priorityClassName": "high",
		"vpa":               map[string]any{"enabled": true},
		"rollingUpdate":     map[string]any{"maxSurge": 2},
		"disruptionBudget":  map[string]any{"minAvailable": 1},
		"ephemeralVolumes": []map[string]any{
			{"name": "scratch", "mountPath": "/scratch", "size": "50Gi"},
		},
		"readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/ready"}},
		"livenessProbe":  map[string]any{"httpGet": map[string]any{"path": "/live"}},
	})
	stackCfg := &api.StackConfigCompose{Runs: []string{}, CloudExtras: &cloudExtras}

	res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, compose.Config{Project: &types.Project{}}, stackCfg)

	Expect(err).ToNot(HaveOccurred())
	input := res.(*KubeRunInput)
	Expect(input.Deployment.PriorityClassName).ToNot(BeNil())
	Expect(*input.Deployment.PriorityClassName).To(Equal("high"))
	Expect(input.Deployment.VPA).ToNot(BeNil())
	Expect(input.Deployment.VPA.Enabled).To(BeTrue())
	Expect(input.Deployment.RollingUpdate).ToNot(BeNil())
	Expect(input.Deployment.RollingUpdate.MaxSurge).ToNot(BeNil())
	Expect(*input.Deployment.RollingUpdate.MaxSurge).To(Equal(2))
	Expect(input.Deployment.DisruptionBudget).ToNot(BeNil())
	Expect(input.Deployment.EphemeralVolumes).To(HaveLen(1))
	Expect(input.Deployment.EphemeralVolumes[0].Name).To(Equal("scratch"))
	Expect(input.Deployment.ReadinessProbe).ToNot(BeNil())
	Expect(input.Deployment.LivenessProbe).ToNot(BeNil())
}
