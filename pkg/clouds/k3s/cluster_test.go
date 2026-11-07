// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestK3sClusterConfigValidate(t *testing.T) {
	valid := func() *K3sClusterConfig {
		return &K3sClusterConfig{VmResource: "vm"}
	}

	for _, tc := range []struct {
		name    string
		cfg     func() *K3sClusterConfig
		wantErr string
	}{
		{
			name: "minimal config is valid",
			cfg:  valid,
		},
		{
			name:    "vmResource is required",
			cfg:     func() *K3sClusterConfig { return &K3sClusterConfig{} },
			wantErr: "requires `vmResource`",
		},
		{
			// The field exists so that an operator can pin a release; a channel name
			// resolves at install time and would make two provisions of the same
			// config install two different control planes.
			name:    "a release channel is refused with the reason",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Version = "stable"; return c },
			wantErr: "install a different control plane on a later provision",
		},
		{
			name:    "a minor channel is refused too",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Version = "v1.33"; return c },
			wantErr: "install a different control plane on a later provision",
		},
		{
			name: "a pinned release is accepted",
			cfg:  func() *K3sClusterConfig { c := valid(); c.Version = "v1.32.13+k3s1"; return c },
		},
		{
			name:    "an endpoint with a scheme is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Endpoint = "https://k8s.example.ru"; return c },
			wantErr: "no scheme and no port",
		},
		{
			name:    "an endpoint with a port is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Endpoint = "k8s.example.ru:6443"; return c },
			wantErr: "no scheme and no port",
		},
		{
			name: "a bare host and a bare IP are both accepted",
			cfg: func() *K3sClusterConfig {
				c := valid()
				c.Endpoint = "k8s.example.ru"
				c.TlsSans = []string{"10.0.0.4", "cluster.internal"}
				return c
			},
		},
		{
			// A typo in a component name leaves the component RUNNING and k3s says
			// nothing, so this has to fail here rather than pass through.
			name:    "an unknown disable component is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Disable = []string{"treafik"}; return c },
			wantErr: "is not a k3s packaged component",
		},
		{
			name:    "disabling coredns is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Disable = []string{"coredns"}; return c },
			wantErr: "no workload can resolve a Service",
		},
		{
			name: "disabling the supported components is accepted",
			cfg: func() *K3sClusterConfig {
				c := valid()
				c.Disable = []string{"traefik", "servicelb"}
				return c
			},
		},
		{
			// k3s takes the LAST occurrence of a repeated flag, so a duplicate would
			// silently win over SC's own value.
			name:    "an extra arg that duplicates a managed flag is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.ExtraServerArgs = []string{"--tls-san", "x"}; return c },
			wantErr: "which SC sets itself",
		},
		{
			name:    "the = spelling of a managed flag is refused too",
			cfg:     func() *K3sClusterConfig { c := valid(); c.ExtraServerArgs = []string{"--disable=traefik"}; return c },
			wantErr: "which SC sets itself",
		},
		{
			name: "an unrelated extra arg is accepted",
			cfg: func() *K3sClusterConfig {
				c := valid()
				c.ExtraServerArgs = []string{"--kube-apiserver-arg=feature-gates=X=true"}
				return c
			},
		},
		{
			name: "one server is the default and is accepted explicitly",
			cfg:  func() *K3sClusterConfig { c := valid(); c.Servers = 1; return c },
		},
		{
			name: "three servers are accepted",
			cfg:  func() *K3sClusterConfig { c := valid(); c.Servers = 3; return c },
		},
		{
			// Refused rather than rounded: etcd elects a leader by majority, so two
			// members tolerate zero failures — either machine going away takes the
			// API server down with it, which is strictly worse than one server.
			name:    "two servers are refused with the quorum reason",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Servers = 2; return c },
			wantErr: "tolerate no failures at all",
		},
		{
			name:    "four servers are refused too",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Servers = 4; return c },
			wantErr: "needs an odd number (1, 3, 5)",
		},
		{
			name:    "a negative server count is refused",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Servers = -1; return c },
			wantErr: "`servers` must be positive",
		},
		{
			// The UPPER bound is deliberately not checked here: this config does not
			// know how many hosts the vm resource it names has. The provisioning
			// tier refuses that combination, where both numbers are in hand
			// (pkg/clouds/pulumi/k3s/cluster.go, rolesOf).
			name:    "a server count exceeding any plausible pool still validates here",
			cfg:     func() *K3sClusterConfig { c := valid(); c.Servers = 99; return c },
			wantErr: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			err := tc.cfg().Validate()
			if tc.wantErr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

// The default lives in exactly one place so the provisioning tier never re-derives
// it. A resource that disagreed with itself about which hosts are servers would join
// a machine as an agent on one apply and as an etcd member on the next, and the
// second of those is not reversible by changing the number back.
func TestEffectiveServers(t *testing.T) {
	RegisterTestingT(t)

	Expect((&K3sClusterConfig{}).EffectiveServers()).To(Equal(1))
	Expect((&K3sClusterConfig{Servers: 1}).EffectiveServers()).To(Equal(1))
	Expect((&K3sClusterConfig{Servers: 3}).EffectiveServers()).To(Equal(3))
}

// The dependency declaration is the entire mechanism by which a cluster sees its
// machines: provision.go provisions dependency-free resources first and hands their
// outputs to the ones that declare a dependency. A config that did not implement
// api.WithParentDependencies would be provisioned in the first pass, with an empty
// ResourceOutputs map, and fail with "vmResource is not a resource of this stack".
func TestK3sClusterConfigDeclaresItsVmDependency(t *testing.T) {
	RegisterTestingT(t)

	var cfg any = &K3sClusterConfig{VmResource: "my-vm"}

	withDeps, ok := cfg.(api.WithParentDependencies)
	Expect(ok).To(BeTrue(), "K3sClusterConfig must implement api.WithParentDependencies, "+
		"otherwise provision.go provisions it before the vm resource it needs")
	Expect(withDeps.DependsOnResources()).To(Equal([]api.ParentResourceDependency{{Name: "my-vm"}}))
}

// initProvider casts a resource config to api.AuthConfig before it can look a
// provider up, so a config that does not satisfy the interface fails at provision
// time with a cast error rather than at compile time.
func TestK3sClusterConfigIsAnAuthConfig(t *testing.T) {
	RegisterTestingT(t)

	var cfg any = &K3sClusterConfig{VmResource: "vm"}

	authCfg, ok := cfg.(api.AuthConfig)
	Expect(ok).To(BeTrue())
	Expect(authCfg.ProviderType()).To(Equal(ProviderType))
	// "{}" and not the serialised config: whatever is returned here is handed to
	// other code, and a k3s cluster has no cloud credentials of its own.
	Expect(authCfg.CredentialsValue()).To(Equal("{}"))
}

func TestKubeconfigURL(t *testing.T) {
	RegisterTestingT(t)

	Expect(KubeconfigURL("k8s.example.ru")).To(Equal("https://k8s.example.ru:6443"))
}
