// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type namingMocks struct {
	*simpleContainerMocks
	mu    sync.Mutex
	names []string
}

// NewResource records "<type>::<logical name> (<k8s name>)": the logical name is what
// the Pulumi URN is built from, the k8s name is what the cluster sees.
func (m *namingMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	k8sName := ""
	if md, ok := args.Inputs["metadata"]; ok && md.IsObject() {
		if n, ok := md.ObjectValue()["name"]; ok && n.IsString() {
			k8sName = n.StringValue()
		}
	}
	m.mu.Lock()
	m.names = append(m.names, fmt.Sprintf("%s::%s (%s)", args.TypeToken, args.Name, k8sName))
	m.mu.Unlock()
	return m.simpleContainerMocks.NewResource(args)
}

func autopilotWithCaddy(adopt bool) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Type: gcloud.ResourceTypeGkeAutopilot,
		Config: api.Config{Config: &gcloud.GkeAutopilotResource{
			Caddy: &k8s.CaddyConfig{Namespace: strPtr("caddy"), DeploymentName: strPtr("caddy")},
			Adopt: adopt,
		}},
	}
}

func strPtr(s string) *string { return &s }

func caddyDescriptor(name string, cfg *k8s.CaddyConfig) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Name:   name,
		Type:   k8s.ResourceTypeCaddy,
		Config: api.Config{Config: &k8s.CaddyResource{KubernetesConfig: &k8s.KubernetesConfig{}, CaddyConfig: cfg}},
	}
}

func namedCaddyDescriptor(name string, cfg *k8s.CaddyConfig) api.ResourceDescriptor {
	d := caddyDescriptor(name, cfg)
	d.Config.Config.(*k8s.CaddyResource).UseResourceName = true
	return d
}

func provisionCaddy(env, resName string, envResources map[string]api.ResourceDescriptor) ([]string, error) {
	mocks := &namingMocks{simpleContainerMocks: NewSimpleContainerMocks()}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		desc := envResources[resName]
		stack := api.Stack{Name: "root", Server: api.ServerDescriptor{Resources: api.PerStackResourcesDescriptor{
			Resources: map[string]api.PerEnvResourcesDescriptor{env: {Resources: envResources}},
		}}}
		input := api.ResourceInput{
			Descriptor:  &desc,
			StackParams: &api.StackParams{StackName: "root", Environment: env},
		}
		params := pApi.ProvisionParams{
			Log:            logger.New(),
			ComputeContext: pApi.NewComputeContextCollector(context.Background(), logger.New(), "root", env),
		}
		_, err := CaddyResource(ctx, stack, input, params)
		return err
	}, sdk.WithMocks("test", "test", mocks))
	sort.Strings(mocks.names)
	return mocks.names, err
}

// The names a kubernetes-caddy produced before useResourceName, captured by running
// provisionCaddy against c88b6f94. A change here renames a live Caddy: Pulumi replaces
// every object whose logical name moved.
var soleCaddyNames = []string{
	"kubernetes:apps/v1:Deployment::caddy-likeclaw-us (caddy-likeclaw-us)",
	"kubernetes:core/v1:ConfigMap::caddy-likeclaw-us-cfg-volumes (caddy-likeclaw-us-cfg-volumes)",
	"kubernetes:core/v1:Namespace::caddy-likeclaw-us-ns (caddy)",
	"kubernetes:core/v1:Secret::caddy-likeclaw-us-secret-volumes (caddy-likeclaw-us-secret-volumes)",
	"kubernetes:core/v1:Secret::caddy-likeclaw-us-secrets (caddy-likeclaw-us-secrets)",
	"kubernetes:core/v1:Service::caddy-likeclaw-us (caddy-likeclaw-us)",
	"kubernetes:core/v1:ServiceAccount::ingress-caddy-sa-likeclaw-us (ingress-caddy-sa-likeclaw-us)",
	"kubernetes:policy/v1:PodDisruptionBudget::caddy-likeclaw-us-pdb ()",
	"kubernetes:rbac.authorization.k8s.io/v1:ClusterRole::ingress-caddy-sa-likeclaw-us (ingress-caddy-sa-likeclaw-us)",
	"kubernetes:rbac.authorization.k8s.io/v1:ClusterRoleBinding::ingress-caddy-sa-likeclaw-us ()",
	"pkg:k8s/extensions:simpleServiceAccount::ingress-caddy-sa-likeclaw-us ()",
	"pulumi:providers:kubernetes::ingress--likeclaw-us-caddy-kubeprovider ()",
	"simple-container.com:k8s:SimpleContainer::caddy-likeclaw-us ()",
}

func TestCaddyResource_SoleCaddyKeepsTheEnvName(t *testing.T) {
	RegisterTestingT(t)
	names, err := provisionCaddy("likeclaw-us", "ingress", map[string]api.ResourceDescriptor{
		"ingress":  caddyDescriptor("ingress", &k8s.CaddyConfig{DeploymentName: strPtr("caddy")}),
		"registry": {Type: "gcp-artifact-registry"},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(names).To(Equal(soleCaddyNames))
}

// Adding a Caddy that uses its resource name next to an existing one leaves the
// existing one's names untouched.
func TestCaddyResource_NeighbourDoesNotRenameTheDefaultCaddy(t *testing.T) {
	RegisterTestingT(t)
	names, err := provisionCaddy("likeclaw-us", "ingress", map[string]api.ResourceDescriptor{
		"ingress":   caddyDescriptor("ingress", &k8s.CaddyConfig{DeploymentName: strPtr("caddy")}),
		"caddy-std": namedCaddyDescriptor("caddy-std", &k8s.CaddyConfig{}),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(names).To(Equal(soleCaddyNames))
}

// An adopted Autopilot cluster creates no Caddy objects, so it does not clash with the
// kubernetes-caddy next to it.
func TestCaddyResource_AdoptedAutopilotDoesNotCount(t *testing.T) {
	RegisterTestingT(t)
	names, err := provisionCaddy("likeclaw-us", "ingress", map[string]api.ResourceDescriptor{
		"ingress": caddyDescriptor("ingress", &k8s.CaddyConfig{}),
		"gke":     autopilotWithCaddy(true),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(names).To(Equal(soleCaddyNames))
}

// Before useResourceName this environment failed in Pulumi with a duplicate URN.
func TestCaddyResource_SecondCaddyWithoutUseResourceNameIsRefused(t *testing.T) {
	RegisterTestingT(t)
	_, err := provisionCaddy("likeclaw-us", "caddy-std", map[string]api.ResourceDescriptor{
		"caddy-std":          caddyDescriptor("caddy-std", &k8s.CaddyConfig{}),
		"k8s-engine-cluster": autopilotWithCaddy(false),
	})
	Expect(err).To(MatchError(ContainSubstring(`names its objects "caddy-likeclaw-us", as [k8s-engine-cluster] already does`)))
}

func TestCaddyResource_SecondCaddyInEnvGetsTheResourceName(t *testing.T) {
	RegisterTestingT(t)
	names, err := provisionCaddy("likeclaw-us", "caddy-std", map[string]api.ResourceDescriptor{
		"caddy-std":          namedCaddyDescriptor("caddy-std", &k8s.CaddyConfig{Namespace: strPtr("caddy")}),
		"k8s-engine-cluster": autopilotWithCaddy(false),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(names).To(Equal([]string{
		"kubernetes:apps/v1:Deployment::caddy-std-likeclaw-us (caddy-std-likeclaw-us)",
		"kubernetes:core/v1:ConfigMap::caddy-std-likeclaw-us-cfg-volumes (caddy-std-likeclaw-us-cfg-volumes)",
		"kubernetes:core/v1:Namespace::caddy-std-likeclaw-us-ns (caddy)",
		"kubernetes:core/v1:Secret::caddy-std-likeclaw-us-secret-volumes (caddy-std-likeclaw-us-secret-volumes)",
		"kubernetes:core/v1:Secret::caddy-std-likeclaw-us-secrets (caddy-std-likeclaw-us-secrets)",
		"kubernetes:core/v1:Service::caddy-std-likeclaw-us (caddy-std-likeclaw-us)",
		"kubernetes:core/v1:ServiceAccount::caddy-std-caddy-sa-likeclaw-us (caddy-std-caddy-sa-likeclaw-us)",
		"kubernetes:policy/v1:PodDisruptionBudget::caddy-std-likeclaw-us-pdb ()",
		"kubernetes:rbac.authorization.k8s.io/v1:ClusterRole::caddy-std-caddy-sa-likeclaw-us (caddy-std-caddy-sa-likeclaw-us)",
		"kubernetes:rbac.authorization.k8s.io/v1:ClusterRoleBinding::caddy-std-caddy-sa-likeclaw-us ()",
		"pkg:k8s/extensions:simpleServiceAccount::caddy-std-caddy-sa-likeclaw-us ()",
		"pulumi:providers:kubernetes::caddy-std--likeclaw-us-caddy-kubeprovider ()",
		"simple-container.com:k8s:SimpleContainer::caddy-std-likeclaw-us ()",
	}))
}

// Both Caddies of one environment run in one Pulumi program: no logical name may repeat.
func TestCaddyResource_TwoCaddiesInOneEnvHaveDisjointNames(t *testing.T) {
	RegisterTestingT(t)
	env := map[string]api.ResourceDescriptor{
		"caddy-a": caddyDescriptor("caddy-a", &k8s.CaddyConfig{}),
		"caddy-b": namedCaddyDescriptor("caddy-b", &k8s.CaddyConfig{}),
	}
	a, err := provisionCaddy("likeclaw-us", "caddy-a", env)
	Expect(err).ToNot(HaveOccurred())
	b, err := provisionCaddy("likeclaw-us", "caddy-b", env)
	Expect(err).ToNot(HaveOccurred())
	seen := map[string]bool{}
	for _, n := range a {
		seen[n] = true
	}
	for _, n := range b {
		Expect(seen).ToNot(HaveKey(n))
	}
	Expect(a).To(ContainElement("kubernetes:apps/v1:Deployment::caddy-likeclaw-us (caddy-likeclaw-us)"))
	Expect(b).To(ContainElement("kubernetes:apps/v1:Deployment::caddy-b-likeclaw-us (caddy-b-likeclaw-us)"))
}

// oneEnv wraps a single environment's resources in the per-env map the namer scans.
func oneEnv(env string, resources map[string]api.ResourceDescriptor) map[string]api.PerEnvResourcesDescriptor {
	return map[string]api.PerEnvResourcesDescriptor{env: {Resources: resources}}
}

func TestKubernetesCaddyDeploymentName(t *testing.T) {
	RegisterTestingT(t)

	name, err := KubernetesCaddyDeploymentName("caddy-std", "likeclaw-us", false, oneEnv("likeclaw-us", map[string]api.ResourceDescriptor{
		"caddy-std": caddyDescriptor("caddy-std", nil),
	}))
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(BeEmpty())

	name, err = KubernetesCaddyDeploymentName("caddy-std", "likeclaw-us", true, oneEnv("likeclaw-us", map[string]api.ResourceDescriptor{
		"caddy-std": namedCaddyDescriptor("caddy-std", nil),
	}))
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(Equal("caddy-std-likeclaw-us"))

	// Config maps the reader did not type yet are read the same way.
	name, err = KubernetesCaddyDeploymentName("caddy-std", "likeclaw-us", true, oneEnv("likeclaw-us", map[string]api.ResourceDescriptor{
		"gke": {Type: gcloud.ResourceTypeGkeAutopilot, Config: api.Config{Config: map[string]any{
			"caddy": map[string]any{"enable": true},
		}}},
		"other": {Type: k8s.ResourceTypeCaddy, Config: api.Config{Config: map[string]any{
			"kubeconfig": "x", "useResourceName": true,
		}}},
	}))
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(Equal("caddy-std-likeclaw-us"))

	_, err = KubernetesCaddyDeploymentName("ingress", "likeclaw-us", false, oneEnv("likeclaw-us", map[string]api.ResourceDescriptor{
		"other": {Type: k8s.ResourceTypeCaddy, Config: api.Config{Config: map[string]any{"kubeconfig": "x"}}},
	}))
	Expect(err).To(MatchError(ContainSubstring(`as [other] already does`)))

	// useResourceName on a resource called caddy still lands on caddy-<env>.
	_, err = KubernetesCaddyDeploymentName("caddy", "likeclaw-us", true, oneEnv("likeclaw-us", map[string]api.ResourceDescriptor{
		"gke": autopilotWithCaddy(false),
	}))
	Expect(err).To(MatchError(ContainSubstring(`names its objects "caddy-likeclaw-us"`)))
}

// A Caddy that sets `name:` explicitly is provisioned under that name, not under its map
// key, so the scan has to recognise its own entry by the same rule provision.go uses.
// Comparing the bare key made the sole Caddy of an environment clash with itself — a
// configuration that provisioned fine before useResourceName existed.
func TestKubernetesCaddyDeploymentName_ExplicitNameDoesNotClashWithItself(t *testing.T) {
	RegisterTestingT(t)

	env := map[string]api.ResourceDescriptor{"ingress": caddyDescriptor("myingress", nil)}

	name, err := KubernetesCaddyDeploymentName("myingress", "env1", false, oneEnv("env1", env))
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(BeEmpty())

	env = map[string]api.ResourceDescriptor{"ingress": namedCaddyDescriptor("myingress", nil)}
	name, err = KubernetesCaddyDeploymentName("myingress", "env1", true, oneEnv("env1", env))
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(Equal("myingress-env1"))

	// A real neighbour is still caught when this one carries an explicit name.
	_, err = KubernetesCaddyDeploymentName("myingress", "env1", false, oneEnv("env1", map[string]api.ResourceDescriptor{
		"ingress": caddyDescriptor("myingress", nil),
		"gke":     autopilotWithCaddy(false),
	}))
	Expect(err).To(MatchError(ContainSubstring(`as [gke] already does`)))
}

// Every environment is provisioned in one Pulumi program, and <resource>-<env> aliases
// across environments where caddy-<env> could not.
func TestKubernetesCaddyDeploymentName_ClashAcrossEnvironments(t *testing.T) {
	RegisterTestingT(t)

	all := map[string]api.PerEnvResourcesDescriptor{
		"us":     {Resources: map[string]api.ResourceDescriptor{"caddy-std": namedCaddyDescriptor("caddy-std", nil)}},
		"std-us": {Resources: map[string]api.ResourceDescriptor{"edge": caddyDescriptor("edge", nil)}},
	}

	_, err := KubernetesCaddyDeploymentName("caddy-std", "us", true, all)
	Expect(err).To(MatchError(ContainSubstring(`names its objects "caddy-std-us", as [edge in "std-us"] already does`)))

	// An unrelated Caddy in another environment is not a clash.
	_, err = KubernetesCaddyDeploymentName("caddy-std", "us", true, map[string]api.PerEnvResourcesDescriptor{
		"us": {Resources: map[string]api.ResourceDescriptor{"caddy-std": namedCaddyDescriptor("caddy-std", nil)}},
		"eu": {Resources: map[string]api.ResourceDescriptor{"edge": caddyDescriptor("edge", nil)}},
	})
	Expect(err).ToNot(HaveOccurred())
}

// caddyObjectsName mirrors GenerateCaddyDeploymentName's empty-env handling rather than
// trailing a hyphen that would reach the Pulumi logical name verbatim.
func TestKubernetesCaddyDeploymentName_EmptyEnv(t *testing.T) {
	RegisterTestingT(t)

	name, err := KubernetesCaddyDeploymentName("caddy-std", "", true, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(Equal("caddy-std"))

	name, err = KubernetesCaddyDeploymentName("caddy", "", true, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(name).To(BeEmpty())
}

// The parent exports the derived name in the caddy config; the child patches that
// deployment, not caddy-<parentEnv> (the Autopilot one).
func TestChildCaddyTarget(t *testing.T) {
	RegisterTestingT(t)

	dep, ns := ChildCaddyTarget(&k8s.CaddyConfig{DeploymentName: strPtr("caddy-std-likeclaw-us"), Namespace: strPtr("caddy")}, "work-dev", "likeclaw-us")
	Expect(dep).To(Equal("caddy-std-likeclaw-us"))
	Expect(ns).To(Equal("caddy"))

	dep, ns = ChildCaddyTarget(&k8s.CaddyConfig{}, "work-dev", "likeclaw-us")
	Expect(dep).To(Equal("caddy-likeclaw-us"))
	Expect(ns).To(Equal("caddy"))

	dep, _ = ChildCaddyTarget(nil, "production", "")
	Expect(dep).To(Equal("caddy-production"))
}

func TestExportedCaddyConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := &k8s.CaddyConfig{Namespace: strPtr("caddy"), DeploymentName: strPtr("caddy")}
	Expect(exportedCaddyConfig(CaddyDeployment{CaddyConfig: cfg})).To(BeIdenticalTo(cfg))

	got := exportedCaddyConfig(CaddyDeployment{CaddyConfig: cfg, ObjectsName: "caddy-std-likeclaw-us"})
	Expect(*got.DeploymentName).To(Equal("caddy-std-likeclaw-us"))
	Expect(*cfg.DeploymentName).To(Equal("caddy"))

	Expect(*exportedCaddyConfig(CaddyDeployment{ObjectsName: "caddy-std-likeclaw-us"}).DeploymentName).To(Equal("caddy-std-likeclaw-us"))
}
