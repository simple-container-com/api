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
