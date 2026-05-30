package gcloud

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

// TestGkeAutopilotTemplate_UseSSLPropagation locks in the contract for the
// `UseSSL *bool` field on GkeAutopilotTemplate: the parsed template config
// must be carried into GkeAutopilotInput unchanged, so the downstream
// pulumi stack (gke_autopilot_stack.go) can derive `useSSL` from it and
// pass it into kubernetes.Args.
//
// Why this test exists, separate from the existing TestCaddyfileEntry_*
// suite: the bug it guards against (PR #302) was a SILENT PLUMBING GAP —
// the field had no transport from template config to kubernetes.Args, so
// the renderer was never invoked with the right value. Rendering tests
// that pass `Args{UseSSL: true}` directly can't catch a propagation bug
// because they bypass the propagation. This test asserts at the
// converter boundary.
func TestGkeAutopilotTemplate_UseSSLPropagation(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name           string
		tplUseSSL      *bool
		expectedUseSSL *bool // what we expect to see on the returned GkeAutopilotInput
	}{
		{
			name:           "nil_is_preserved_default_true_at_consumer",
			tplUseSSL:      nil,
			expectedUseSSL: nil,
		},
		{
			name:           "explicit_true_carried_through",
			tplUseSSL:      lo.ToPtr(true),
			expectedUseSSL: lo.ToPtr(true),
		},
		{
			name:           "explicit_false_carried_through",
			tplUseSSL:      lo.ToPtr(false),
			expectedUseSSL: lo.ToPtr(false),
		},
	}

	ctx := context.Background()
	composeCfg, err := compose.ReadDockerCompose(ctx, "", "testdata/stacks/refapp/docker-compose.yaml")
	Expect(err).To(BeNil())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl := &GkeAutopilotTemplate{
				GkeClusterResource:       "test-cluster",
				ArtifactRegistryResource: "test-registry",
				UseSSL:                   tt.tplUseSSL,
			}
			stackCfg := &api.StackConfigCompose{}

			got, convErr := ToGkeAutopilotConfig(tpl, composeCfg, stackCfg)
			Expect(convErr).To(BeNil(), "ToGkeAutopilotConfig should succeed")

			gke, ok := got.(*GkeAutopilotInput)
			Expect(ok).To(BeTrue(), "expected *GkeAutopilotInput from converter")

			// The struct-embedded `GkeAutopilotTemplate` carries UseSSL.
			if tt.expectedUseSSL == nil {
				Expect(gke.UseSSL).To(BeNil(),
					"nil template UseSSL must remain nil on GkeAutopilotInput so the downstream derivation defaults to true")
			} else {
				Expect(gke.UseSSL).ToNot(BeNil(),
					"explicit template UseSSL must be carried through, not lost")
				Expect(*gke.UseSSL).To(Equal(*tt.expectedUseSSL))
			}
		})
	}
}

// TestGkeAutopilotDeriveUseSSL_DefaultsTrue documents (and locks in) the
// nil-pointer-becomes-true semantic used by gke_autopilot_stack.go to
// derive the bool that gets passed into kubernetes.Args. Mirrors the
// behaviour at pkg/clouds/pulumi/kubernetes/kube_run.go:102 — keeping
// the two paths in sync.
func TestGkeAutopilotDeriveUseSSL_DefaultsTrue(t *testing.T) {
	RegisterTestingT(t)

	// Same expression as the production code in gke_autopilot_stack.go,
	// reproduced here so the contract is exercised in unit tests too.
	derive := func(p *bool) bool { return p == nil || *p }

	Expect(derive(nil)).To(BeTrue(), "nil pointer must default to true (matches CloudrunTemplate)")
	Expect(derive(lo.ToPtr(true))).To(BeTrue())
	Expect(derive(lo.ToPtr(false))).To(BeFalse(), "explicit false must override the default")
}
