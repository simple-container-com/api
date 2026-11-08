// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

// `lbConfig.https` had two consumers in the whole repo and not one assertion anywhere,
// which is part of how it came to be the switch that silently decides whether a site
// gets a certificate. These are the assertions.
func TestSimpleContainerLBConfig_OriginTlsEnabled(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name        string
		lbc         *SimpleContainerLBConfig
		edgeDefault *bool
		want        bool
	}{
		{
			// The majority case: no `lbConfig` at all, no edge-wide default. Plain
			// HTTP on the origin, because Cloudflare or a cloud LB terminates in
			// front. This is the behaviour that must not move.
			name: "no lbConfig and no edge default is plain http",
			want: false,
		},
		{
			name: "empty lbConfig and no edge default is plain http",
			lbc:  &SimpleContainerLBConfig{},
			want: false,
		},
		{
			// Every stack that sets the deprecated spelling today keeps working.
			name: "deprecated https true",
			lbc:  &SimpleContainerLBConfig{Https: true},
			want: true,
		},
		{
			name: "originTls true",
			lbc:  &SimpleContainerLBConfig{OriginTls: lo.ToPtr(true)},
			want: true,
		},
		{
			// The new capability: an edge that really is the front door says so once
			// instead of once per stack.
			name:        "edge default true is inherited when the stack says nothing",
			lbc:         &SimpleContainerLBConfig{},
			edgeDefault: lo.ToPtr(true),
			want:        true,
		},
		{
			name:        "nil lbConfig also inherits the edge default",
			edgeDefault: lo.ToPtr(true),
			want:        true,
		},
		{
			// And a stack can still opt out of an edge that terminates by default —
			// which is the whole reason `originTls` is a *bool and not a bool.
			name:        "explicit stack false beats an edge default of true",
			lbc:         &SimpleContainerLBConfig{OriginTls: lo.ToPtr(false)},
			edgeDefault: lo.ToPtr(true),
			want:        false,
		},
		{
			name:        "edge default false pins the stack to plain http",
			lbc:         &SimpleContainerLBConfig{},
			edgeDefault: lo.ToPtr(false),
			want:        false,
		},
		{
			// The stack is more specific than the edge, both ways round.
			name:        "explicit stack true beats an edge default of false",
			lbc:         &SimpleContainerLBConfig{OriginTls: lo.ToPtr(true)},
			edgeDefault: lo.ToPtr(false),
			want:        true,
		},
		{
			// `originTls` is consulted before `https`, so the readable spelling wins
			// where both are set and do not contradict.
			name: "originTls true alongside https true",
			lbc:  &SimpleContainerLBConfig{Https: true, OriginTls: lo.ToPtr(true)},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tc.lbc.OriginTlsEnabled(tc.edgeDefault)).To(Equal(tc.want))
		})
	}
}

func TestSimpleContainerLBConfig_ValidateTls(t *testing.T) {
	RegisterTestingT(t)

	Expect((*SimpleContainerLBConfig)(nil).ValidateTls()).To(BeNil())
	Expect((&SimpleContainerLBConfig{}).ValidateTls()).To(BeNil())
	Expect((&SimpleContainerLBConfig{Https: true}).ValidateTls()).To(BeNil())
	Expect((&SimpleContainerLBConfig{OriginTls: lo.ToPtr(false)}).ValidateTls()).To(BeNil())
	Expect((&SimpleContainerLBConfig{Https: true, OriginTls: lo.ToPtr(true)}).ValidateTls()).To(BeNil())

	// `https: true, originTls: false` is the one pair that cannot be honoured both
	// ways — one of them asks for a certificate and the other refuses one.
	err := (&SimpleContainerLBConfig{Https: true, OriginTls: lo.ToPtr(false)}).ValidateTls()
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("`lbConfig.https: true` contradicts `lbConfig.originTls: false`"))
	Expect(err.Error()).To(ContainSubstring("drop `https`"))
}

// `https` is a plain bool, so `https: false` is indistinguishable from unset and is NOT
// a contradiction with `originTls: true` — it is the overwhelmingly common state of
// every client.yaml that mentions lbConfig for some other reason (extraHelpers,
// requestBufferSize). Refusing that pair would refuse the normal case.
func TestSimpleContainerLBConfig_HttpsFalseIsNotAContradiction(t *testing.T) {
	RegisterTestingT(t)

	lbc := &SimpleContainerLBConfig{Https: false, OriginTls: lo.ToPtr(true), RequestBufferSize: "4MiB"}
	Expect(lbc.ValidateTls()).To(BeNil())
	Expect(lbc.OriginTlsEnabled(nil)).To(BeTrue())
}

// The client tier round-trips through yaml on the same path the server tier does, and
// `originTls` must not appear in a descriptor that never set it.
func TestSimpleContainerLBConfig_YamlRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	lbc, err := ConvertDescriptor(map[string]any{"https": true}, &SimpleContainerLBConfig{})
	Expect(err).To(BeNil())
	Expect(lbc.Https).To(BeTrue())
	Expect(lbc.OriginTls).To(BeNil())
	Expect(lbc.OriginTlsEnabled(nil)).To(BeTrue())

	lbc, err = ConvertDescriptor(map[string]any{"originTls": false}, &SimpleContainerLBConfig{})
	Expect(err).To(BeNil())
	Expect(lbc.Https).To(BeFalse())
	Expect(lbc.OriginTls).ToNot(BeNil())
	Expect(*lbc.OriginTls).To(BeFalse())

	out, err := ConvertDescriptor(&SimpleContainerLBConfig{Https: true}, &map[string]any{})
	Expect(err).To(BeNil())
	Expect(*out).ToNot(HaveKey("originTls"))
}
