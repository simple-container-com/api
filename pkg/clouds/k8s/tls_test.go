// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package k8s

import (
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestClientTlsEnabled(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name string
		cfg  *TlsConfig
		want bool
	}{
		{
			// A template that embeds TlsConfig and sets nothing is the common case,
			// and it must keep the historical `useSSL` default.
			name: "both unset defaults to true",
			cfg:  &TlsConfig{},
			want: true,
		},
		{
			// Not merely a convenience: caddy.go reads this off a config that is
			// routinely absent, and a nil-receiver panic there is a provision crash.
			name: "nil receiver defaults to true",
			cfg:  nil,
			want: true,
		},
		{
			name: "deprecated useSSL is still honoured",
			cfg:  &TlsConfig{UseSSL: lo.ToPtr(false)},
			want: false,
		},
		{
			name: "clientTls false",
			cfg:  &TlsConfig{ClientTls: lo.ToPtr(false)},
			want: false,
		},
		{
			name: "clientTls true",
			cfg:  &TlsConfig{ClientTls: lo.ToPtr(true)},
			want: true,
		},
		{
			// Agreeing spellings are not a contradiction, so they resolve rather
			// than refuse — an operator mid-migration may well set both.
			name: "both set and agreeing",
			cfg:  &TlsConfig{ClientTls: lo.ToPtr(false), UseSSL: lo.ToPtr(false)},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(tc.cfg.ClientTlsEnabled()).To(Equal(tc.want))
		})
	}
}

// nil means "this edge states no edge-wide default", which is what preserves today's
// behaviour exactly — not false, which would pin every stack to plain HTTP.
func TestOriginTlsDefault(t *testing.T) {
	RegisterTestingT(t)

	Expect((*TlsConfig)(nil).OriginTlsDefault()).To(BeNil())
	Expect((&TlsConfig{}).OriginTlsDefault()).To(BeNil())
	Expect(lo.FromPtr((&TlsConfig{OriginTls: lo.ToPtr(true)}).OriginTlsDefault())).To(BeTrue())
	// Explicit false is distinguishable from unset, and pins the edge to plain HTTP.
	originTls := (&TlsConfig{OriginTls: lo.ToPtr(false)}).OriginTlsDefault()
	Expect(originTls).ToNot(BeNil())
	Expect(*originTls).To(BeFalse())
}

// The one combination that cannot be honoured both ways is refused rather than resolved
// silently: whichever way a precedence rule broke the tie, half the operators who wrote
// it would be wrong about what their edge does.
func TestValidateTlsRefusesAContradiction(t *testing.T) {
	RegisterTestingT(t)

	Expect((*TlsConfig)(nil).ValidateTls()).To(BeNil())
	Expect((&TlsConfig{}).ValidateTls()).To(BeNil())
	Expect((&TlsConfig{ClientTls: lo.ToPtr(true)}).ValidateTls()).To(BeNil())
	Expect((&TlsConfig{UseSSL: lo.ToPtr(false)}).ValidateTls()).To(BeNil())
	Expect((&TlsConfig{ClientTls: lo.ToPtr(true), UseSSL: lo.ToPtr(true)}).ValidateTls()).To(BeNil())

	err := (&TlsConfig{ClientTls: lo.ToPtr(true), UseSSL: lo.ToPtr(false)}).ValidateTls()
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("`clientTls: true` contradicts `useSSL: false`"))
	// The message must name the way out, not just the problem.
	Expect(err.Error()).To(ContainSubstring("drop `useSSL`"))
}

// THE backwards-compatibility test. `useSSL` moved from a direct field into an inlined
// embedded struct, and an inline embedding that silently stops parsing looks exactly
// like an edge that was never configured — forge-baas and forge-atrium both set
// `useSSL` today, so this is the assertion that says the rename is safe.
//
// It goes through api.ConvertDescriptor because that (pkg/api/mapping.go:59) is how a
// server.yaml descriptor actually becomes one of these structs: yaml.Marshal into bytes,
// yaml.Unmarshal into the target. A struct-literal test would prove nothing about it.
func TestUseSSLStillParsesAfterTheInlineEmbedding(t *testing.T) {
	RegisterTestingT(t)

	t.Run("caddy resource", func(t *testing.T) {
		// The shape an existing parent server.yaml has today.
		from := map[string]any{
			"provisionIngress": true,
			"useSSL":           false,
		}
		cfg, err := api.ConvertDescriptor(from, &CaddyConfig{})
		Expect(err).To(BeNil())
		Expect(cfg.ProvisionIngress).To(BeTrue())
		// Promoted through the embedded TlsConfig, so `cfg.UseSSL` still compiles.
		Expect(cfg.UseSSL).ToNot(BeNil(), "`useSSL` stopped parsing — existing parent stacks silently lose it")
		Expect(*cfg.UseSSL).To(BeFalse())
		Expect(cfg.ClientTlsEnabled()).To(BeFalse())
		// And the edge states no origin-TLS default, so nothing changes for it.
		Expect(cfg.OriginTlsDefault()).To(BeNil())
	})

	t.Run("cloudrun template", func(t *testing.T) {
		from := map[string]any{
			"caddyResource": "caddy",
			"useSSL":        true,
		}
		tpl, err := api.ConvertDescriptor(from, &CloudrunTemplate{})
		Expect(err).To(BeNil())
		Expect(lo.FromPtr(tpl.CaddyResource)).To(Equal("caddy"))
		Expect(lo.FromPtr(tpl.UseSSL)).To(BeTrue())
		Expect(tpl.ClientTlsEnabled()).To(BeTrue())
	})

	t.Run("the new spellings parse too", func(t *testing.T) {
		from := map[string]any{
			"clientTls": false,
			"originTls": true,
		}
		cfg, err := api.ConvertDescriptor(from, &CaddyConfig{})
		Expect(err).To(BeNil())
		Expect(cfg.ClientTlsEnabled()).To(BeFalse())
		Expect(lo.FromPtr(cfg.OriginTlsDefault())).To(BeTrue())
		// `useSSL` absent, so nothing to contradict.
		Expect(cfg.ValidateTls()).To(BeNil())
	})

	t.Run("a contradiction written in yaml is refused", func(t *testing.T) {
		from := map[string]any{"clientTls": false, "useSSL": true}
		cfg, err := api.ConvertDescriptor(from, &CaddyConfig{})
		Expect(err).To(BeNil())
		Expect(cfg.ValidateTls()).ToNot(BeNil())
	})
}

// ToKubernetesRunConfig is the one gate every Kubernetes flavour passes through — k3s
// and the gcp/aws templates all delegate here — so validating both tiers at this point
// is what makes a contradiction impossible to deploy, rather than impossible to deploy
// on whichever path someone remembered.
func TestToKubernetesRunConfigValidatesBothTiers(t *testing.T) {
	RegisterTestingT(t)

	emptyCompose := compose.Config{Project: &types.Project{}}

	t.Run("server-tier contradiction", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &CloudrunTemplate{TlsConfig: TlsConfig{ClientTls: lo.ToPtr(true), UseSSL: lo.ToPtr(false)}}
		_, err := ToKubernetesRunConfig(tpl, emptyCompose, &api.StackConfigCompose{Runs: []string{}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("contradicts `useSSL: false`"))
	})

	t.Run("client-tier contradiction", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigCompose{
			Runs:     []string{},
			LBConfig: &api.SimpleContainerLBConfig{Https: true, OriginTls: lo.ToPtr(false)},
		}
		_, err := ToKubernetesRunConfig(&CloudrunTemplate{}, emptyCompose, stackCfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("`lbConfig.https: true` contradicts"))
	})

	t.Run("both tiers consistent", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := &CloudrunTemplate{TlsConfig: TlsConfig{UseSSL: lo.ToPtr(true), OriginTls: lo.ToPtr(true)}}
		stackCfg := &api.StackConfigCompose{
			Runs:     []string{},
			LBConfig: &api.SimpleContainerLBConfig{OriginTls: lo.ToPtr(true)},
		}
		res, err := ToKubernetesRunConfig(tpl, emptyCompose, stackCfg)
		Expect(err).ToNot(HaveOccurred())
		in, ok := res.(*KubeRunInput)
		Expect(ok).To(BeTrue())
		Expect(in.ClientTlsEnabled()).To(BeTrue())
		Expect(lo.FromPtr(in.OriginTlsDefault())).To(BeTrue())
	})

	t.Run("a bare template and no lbConfig is still accepted", func(t *testing.T) {
		RegisterTestingT(t)
		// The state of nearly every stack in the fleet. Nothing to validate and
		// nothing to change: client TLS on, no edge-wide origin-TLS default.
		res, err := ToKubernetesRunConfig(&CloudrunTemplate{}, emptyCompose, &api.StackConfigCompose{Runs: []string{}})
		Expect(err).ToNot(HaveOccurred())
		in, ok := res.(*KubeRunInput)
		Expect(ok).To(BeTrue())
		Expect(in.ClientTlsEnabled()).To(BeTrue())
		Expect(in.OriginTlsDefault()).To(BeNil())
		Expect(in.Deployment.StackConfig.LBConfig.OriginTlsEnabled(in.OriginTlsDefault())).To(BeFalse())
	})
}

// Marshalling back out must not invent keys. A parent stack that never said `clientTls`
// must not acquire one on the next `sc provision` — the descriptor is round-tripped
// through yaml on more than one path, and `omitempty` on the pointers is what keeps that
// true.
func TestTlsConfigMarshalsOnlyWhatWasSet(t *testing.T) {
	RegisterTestingT(t)

	out, err := api.ConvertDescriptor(&CaddyConfig{TlsConfig: TlsConfig{UseSSL: lo.ToPtr(true)}}, &map[string]any{})
	Expect(err).To(BeNil())
	Expect(*out).To(HaveKeyWithValue("useSSL", true))
	Expect(*out).ToNot(HaveKey("clientTls"))
	Expect(*out).ToNot(HaveKey("originTls"))
}
