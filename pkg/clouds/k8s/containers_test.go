// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/compose-spec/compose-go/types"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestConvertComposeToContainers_NilProject(t *testing.T) {
	RegisterTestingT(t)

	_, err := ConvertComposeToContainers(compose.Config{Project: nil}, &api.StackConfigCompose{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("compose config is nil"))
}

func TestConvertComposeToContainers_MissingService(t *testing.T) {
	RegisterTestingT(t)

	composeCfg := compose.Config{Project: &types.Project{
		Services: []types.ServiceConfig{{Name: "present"}},
	}}
	stackCfg := &api.StackConfigCompose{Runs: []string{"absent"}}

	_, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("service absent not found"))
}

func TestConvertComposeToContainers_FullMapping(t *testing.T) {
	RegisterTestingT(t)

	pull := "Always"
	composeCfg := compose.Config{Project: &types.Project{
		WorkingDir: "/work",
		Services: []types.ServiceConfig{
			{
				Name:        "api",
				Entrypoint:  types.ShellCommand{"/bin/entry"},
				Command:     types.ShellCommand{"serve", "--port", "8080"},
				Environment: types.MappingWithEquals{"ENV": lo.ToPtr("prod")},
				Ports:       []types.ServicePortConfig{{Target: 8080}},
				Build: &types.BuildConfig{
					Context: "./api",
					Args:    types.MappingWithEquals{"VERSION": lo.ToPtr("1.0")},
				},
			},
		},
	}}
	stackCfg := &api.StackConfigCompose{Runs: []string{"api"}, ImagePullPolicy: &pull}

	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(containers).To(HaveLen(1))

	c := containers[0]
	Expect(c.Name).To(Equal("api"))
	Expect(c.Command).To(Equal([]string{"/bin/entry"}))
	Expect(c.Args).To(Equal([]string{"serve", "--port", "8080"}))
	Expect(c.Env).To(HaveKeyWithValue("ENV", "prod"))
	Expect(c.Ports).To(Equal([]int{8080}))
	Expect(c.ComposeDir).To(Equal("/work"))
	Expect(c.Image.Context).To(Equal("./api"))
	// Context set, dockerfile empty => defaults to "Dockerfile".
	Expect(c.Image.Dockerfile).To(Equal("Dockerfile"))
	Expect(c.Image.Platform).To(Equal(api.ImagePlatformLinuxAmd64))
	Expect(c.Image.Build).ToNot(BeNil())
	Expect(c.Image.Build.Args).To(HaveKeyWithValue("VERSION", "1.0"))
	Expect(c.ImagePullPolicy).ToNot(BeNil())
	Expect(*c.ImagePullPolicy).To(Equal("Always"))
	// Single port => MainPort set to that port, no warnings.
	Expect(c.MainPort).ToNot(BeNil())
	Expect(*c.MainPort).To(Equal(8080))
	Expect(c.Warnings).To(BeEmpty())
}

func TestConvertComposeToContainers_ExplicitDockerfile(t *testing.T) {
	RegisterTestingT(t)

	composeCfg := compose.Config{Project: &types.Project{
		Services: []types.ServiceConfig{
			{
				Name: "svc",
				Build: &types.BuildConfig{
					Context:    "./svc",
					Dockerfile: "Dockerfile.prod",
				},
			},
		},
	}}
	stackCfg := &api.StackConfigCompose{Runs: []string{"svc"}}

	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(containers[0].Image.Dockerfile).To(Equal("Dockerfile.prod"))
}

func TestConvertComposeToContainers_NoBuild(t *testing.T) {
	RegisterTestingT(t)

	composeCfg := compose.Config{Project: &types.Project{
		Services: []types.ServiceConfig{{Name: "svc"}},
	}}
	stackCfg := &api.StackConfigCompose{Runs: []string{"svc"}}

	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(containers[0].Image.Context).To(Equal(""))
	Expect(containers[0].Image.Dockerfile).To(Equal(""))
	Expect(containers[0].Image.Build).ToNot(BeNil())
	Expect(containers[0].Image.Build.Args).To(BeEmpty())
	Expect(containers[0].MainPort).To(BeNil())
}

// TestConvertComposeToContainers_ResourcesError surfaces a parse error from the
// nested ToResources call.
func TestConvertComposeToContainers_ResourcesError(t *testing.T) {
	RegisterTestingT(t)

	composeCfg := compose.Config{Project: &types.Project{
		Services: []types.ServiceConfig{{Name: "svc"}},
	}}
	stackCfg := &api.StackConfigCompose{
		Runs: []string{"svc"},
		Size: &api.StackConfigComposeSize{Limits: &api.StackConfigComposeResources{Cpu: "bad"}},
	}

	_, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to convert stack compose resources"))
}

// TestConvertComposeToContainers_MultiPortWarning verifies that a service with
// more than one declared port and no resolvable main port records a warning and
// leaves MainPort unset.
func TestConvertComposeToContainers_MultiPortWarning(t *testing.T) {
	RegisterTestingT(t)

	composeCfg := compose.Config{Project: &types.Project{
		Services: []types.ServiceConfig{
			{Name: "multi", Ports: []types.ServicePortConfig{{Target: 8080}, {Target: 9090}}},
		},
	}}
	stackCfg := &api.StackConfigCompose{Runs: []string{"multi"}}

	containers, err := ConvertComposeToContainers(composeCfg, stackCfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(containers).To(HaveLen(1))
	// Quirk: the multi-port warning branch checks `MainPort == nil`, which is
	// always true at that point (it is never populated before this check), so the
	// `else if len(Ports) > 0` branch that would set MainPort is unreachable for
	// multi-port containers — MainPort stays nil and a warning is recorded.
	Expect(containers[0].MainPort).To(BeNil())
	Expect(containers[0].Warnings).ToNot(BeEmpty())
	Expect(containers[0].Warnings[0]).To(ContainSubstring("multiple ports and no main port"))
}

func TestFindIngressContainer(t *testing.T) {
	RegisterTestingT(t)

	t.Run("labeled ingress container is selected", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{
				{Name: "web", Labels: types.Labels{api.ComposeLabelIngressContainer: "true"}},
				{Name: "worker"},
			},
		}}
		containers := []CloudRunContainer{
			{Name: "web", Ports: []int{3000}},
			{Name: "worker"},
		}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		Expect(ic).ToNot(BeNil())
		Expect(ic.Name).To(Equal("web"))
		// MainPort derived from the single port.
		Expect(ic.MainPort).ToNot(BeNil())
		Expect(*ic.MainPort).To(Equal(3000))
	})

	t.Run("ingress port label overrides main port", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{
				{Name: "web", Labels: types.Labels{
					api.ComposeLabelIngressContainer: "true",
					api.ComposeLabelIngressPort:      "8443",
				}},
			},
		}}
		containers := []CloudRunContainer{{Name: "web", Ports: []int{3000}}}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		Expect(ic).ToNot(BeNil())
		Expect(ic.MainPort).ToNot(BeNil())
		Expect(*ic.MainPort).To(Equal(8443))
	})

	t.Run("invalid ingress port label records a warning", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{
				{Name: "web", Labels: types.Labels{
					api.ComposeLabelIngressContainer: "true",
					api.ComposeLabelIngressPort:      "not-a-port",
				}},
			},
		}}
		containers := []CloudRunContainer{{Name: "web", Ports: []int{3000}}}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		Expect(ic).ToNot(BeNil())
		Expect(ic.Warnings).ToNot(BeEmpty())
		Expect(ic.Warnings[0]).To(ContainSubstring("failed to convert to int"))
		// MainPort falls back to the single declared port.
		Expect(ic.MainPort).ToNot(BeNil())
		Expect(*ic.MainPort).To(Equal(3000))
	})

	t.Run("more than one ingress container errors", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			ComposeFiles: []string{"docker-compose.yaml"},
			Services: []types.ServiceConfig{
				{Name: "web", Labels: types.Labels{api.ComposeLabelIngressContainer: "true"}},
				{Name: "api", Labels: types.Labels{api.ComposeLabelIngressContainer: "true"}},
			},
		}}
		containers := []CloudRunContainer{{Name: "web"}, {Name: "api"}}

		_, err := FindIngressContainer(composeCfg, containers)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must have exactly 1 ingress container"))
	})

	t.Run("single container single port auto-selected without label", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{{Name: "solo"}},
		}}
		containers := []CloudRunContainer{{Name: "solo", Ports: []int{5000}}}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		Expect(ic).ToNot(BeNil())
		Expect(ic.Name).To(Equal("solo"))
		Expect(ic.MainPort).ToNot(BeNil())
		Expect(*ic.MainPort).To(Equal(5000))
	})

	t.Run("no label and multiple containers yields nil", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{{Name: "a"}, {Name: "b"}},
		}}
		containers := []CloudRunContainer{
			{Name: "a", Ports: []int{1000}},
			{Name: "b", Ports: []int{2000}},
		}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		Expect(ic).To(BeNil())
	})

	t.Run("single container with multiple ports not auto-selected", func(t *testing.T) {
		RegisterTestingT(t)
		composeCfg := compose.Config{Project: &types.Project{
			Services: []types.ServiceConfig{{Name: "solo"}},
		}}
		containers := []CloudRunContainer{{Name: "solo", Ports: []int{5000, 6000}}}

		ic, err := FindIngressContainer(composeCfg, containers)
		Expect(err).ToNot(HaveOccurred())
		// Auto-selection only happens for exactly one port.
		Expect(ic).To(BeNil())
	})
}
