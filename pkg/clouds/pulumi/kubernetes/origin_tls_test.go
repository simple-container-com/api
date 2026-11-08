// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestResolveEdgeOriginTls(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name  string
		caddy *k8s.CaddyConfig
		in    *k8s.KubeRunInput
		want  *bool
	}{
		{
			// The state of every stack that exists today: nothing declared
			// anywhere, so no edge-wide default and each stack's lbConfig decides
			// alone — byte-identical to the behaviour before `originTls` existed.
			name: "nothing declared anywhere",
			want: nil,
		},
		{
			name:  "nil caddy config and nil input",
			caddy: nil,
			in:    nil,
			want:  nil,
		},
		{
			name:  "caddy resource states the default",
			caddy: &k8s.CaddyConfig{TlsConfig: k8s.TlsConfig{OriginTls: lo.ToPtr(true)}},
			want:  lo.ToPtr(true),
		},
		{
			// A template `originTls` is honoured when the edge resource is silent —
			// an operator may well declare it on whichever of the two they are
			// looking at.
			name: "template states the default when caddy is silent",
			in:   kubeRunInputWithOriginTls(lo.ToPtr(true)),
			want: lo.ToPtr(true),
		},
		{
			// The caddy RESOURCE wins: the edge is the thing that terminates, and
			// the template only says how stacks reach it.
			name:  "caddy resource beats the template",
			caddy: &k8s.CaddyConfig{TlsConfig: k8s.TlsConfig{OriginTls: lo.ToPtr(false)}},
			in:    kubeRunInputWithOriginTls(lo.ToPtr(true)),
			want:  lo.ToPtr(false),
		},
		{
			// Unset on the caddy resource must stay distinguishable from false, or
			// the template below it could never be reached.
			name:  "an unset caddy resource falls through rather than answering false",
			caddy: &k8s.CaddyConfig{ProvisionIngress: true},
			in:    kubeRunInputWithOriginTls(lo.ToPtr(true)),
			want:  lo.ToPtr(true),
		},
		{
			// And explicit false must survive all the way down to the lbConfig
			// resolution, where it pins the whole edge to plain HTTP.
			name:  "explicit false is carried, not flattened to nil",
			caddy: &k8s.CaddyConfig{TlsConfig: k8s.TlsConfig{OriginTls: lo.ToPtr(false)}},
			want:  lo.ToPtr(false),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := resolveEdgeOriginTls(tc.caddy, tc.in)
			if tc.want == nil {
				Expect(got).To(BeNil())
				return
			}
			Expect(got).ToNot(BeNil())
			Expect(*got).To(Equal(*tc.want))
		})
	}
}

// The edge default only ever matters through this call, so assert the whole chain the
// Caddyfile site address is actually rendered from: an edge that terminates serves
// `https://` for a stack that says nothing, and a stack can still opt out.
func TestEdgeDefaultReachesTheSiteScheme(t *testing.T) {
	RegisterTestingT(t)

	edge := resolveEdgeOriginTls(&k8s.CaddyConfig{TlsConfig: k8s.TlsConfig{OriginTls: lo.ToPtr(true)}}, nil)

	Expect((*api.SimpleContainerLBConfig)(nil).OriginTlsEnabled(edge)).To(BeTrue())
	Expect((&api.SimpleContainerLBConfig{}).OriginTlsEnabled(edge)).To(BeTrue())
	Expect((&api.SimpleContainerLBConfig{OriginTls: lo.ToPtr(false)}).OriginTlsEnabled(edge)).To(BeFalse())

	// And with no edge default, a stack that says nothing stays on plain HTTP —
	// the arrangement every existing stack is in.
	Expect((*api.SimpleContainerLBConfig)(nil).OriginTlsEnabled(nil)).To(BeFalse())
}

func kubeRunInputWithOriginTls(originTls *bool) *k8s.KubeRunInput {
	in := &k8s.KubeRunInput{}
	in.OriginTls = originTls
	return in
}
