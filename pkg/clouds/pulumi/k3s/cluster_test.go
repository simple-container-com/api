// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k3s

import (
	"testing"

	. "github.com/onsi/gomega"

	cfgK3s "github.com/simple-container-com/api/pkg/clouds/k3s"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

func TestDisableSet(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name string
		cfg  cfgK3s.K3sClusterConfig
		vm   pulumiVm.VmOutput
		want []string
	}{
		{
			name: "unset disables traefik",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"},
			want: []string{"traefik"},
		},
		{
			name: "unset with compose ingress also disables servicelb",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"},
			vm:   pulumiVm.VmOutput{IngressEnabled: true},
			want: []string{"traefik", "servicelb"},
		},
		{
			name: "explicit empty list keeps everything",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm", Disable: []string{}},
			want: []string{},
		},
		{
			name: "explicit list wins",
			cfg:  cfgK3s.K3sClusterConfig{VmResource: "cluster-vm", Disable: []string{"metrics-server"}},
			vm:   pulumiVm.VmOutput{IngressEnabled: true},
			want: []string{"metrics-server"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(disableSet(&tc.cfg, &tc.vm)).To(Equal(tc.want))
		})
	}
}

// TestDisableSetSurvivesPlaceholderWalk is an end-to-end guard on the nil-vs-empty
// distinction disableSet is built on.
//
// Every parsed config is walked by the placeholder resolver before a provisioner sees
// it, and that walk used to turn every nil slice into an empty-but-present one
// (reflect.MakeSlice never returns nil). The unit test above therefore passed while
// the live behaviour was the opposite: `disable` arrived non-nil, the traefik default
// never applied, and the first `type: LoadBalancer` Service on the cluster sat
// `<pending>` behind a klipper-lb pod that could not bind the ports traefik held.
// Caught on the first live k3s smoke, 2026-10-18; fixed in
// provisioner/placeholders/placeholders.go.
func TestDisableSetSurvivesPlaceholderWalk(t *testing.T) {
	RegisterTestingT(t)

	cfg := cfgK3s.K3sClusterConfig{VmResource: "cluster-vm"}
	Expect(placeholders.New().Apply(&cfg)).To(BeNil())

	Expect(cfg.Disable).To(BeNil(), "an unset `disable` must stay nil through the placeholder walk")
	Expect(disableSet(&cfg, &pulumiVm.VmOutput{})).To(Equal([]string{"traefik"}))
}
