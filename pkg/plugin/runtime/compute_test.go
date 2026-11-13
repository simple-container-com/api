// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestParentInfoFromProvisionRequest_RoundTrips — a ParentInfo marshalled
// by SC core's makeResourceSidecar and carried as ProvisionRequest.
// ParentInfoJson decodes back to equal fields. The JSON shape uses Go
// struct field names (capitalised) because ParentInfo has no struct tags
// — same as the ComputeProcess helper in the same package.
func TestParentInfoFromProvisionRequest_RoundTrips(t *testing.T) {
	RegisterTestingT(t)

	want := &pApi.ParentInfo{
		StackName:     "forge-storage",
		FullReference: "organization/forge-storage/staging",
		ParentEnv:     "staging",
	}
	raw, err := json.Marshal(want)
	Expect(err).ToNot(HaveOccurred())

	req := &scpluginv1.ProvisionRequest{
		ParentInfoJson: raw,
	}
	got, err := ParentInfoFromProvisionRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	Expect(got.StackName).To(Equal(want.StackName))
	Expect(got.FullReference).To(Equal(want.FullReference))
	Expect(got.ParentEnv).To(Equal(want.ParentEnv))
}

// TestParentInfoFromProvisionRequest_EmptyMeansNil — the field is absent
// when either:
//   - The deploying stack has no parent (a parent-stack provision itself).
//   - An older SC core pre-dates the field (`sc-plugin-provisionrequest-
//     missing-parent-info` roadmap item).
//
// In both cases the plugin-side helper returns (nil, nil); the template
// body must nil-check `params.ParentStack` before reading any field. The
// two guardrails ship together — see the roadmap item for the framing.
func TestParentInfoFromProvisionRequest_EmptyMeansNil(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ProvisionRequest{}
	got, err := ParentInfoFromProvisionRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeNil())
}

// TestParentInfoFromProvisionRequest_RejectsMalformedJson — a corrupted
// field (ever, in any proto-handling bug) returns a wrapped error with
// the request-kind prefix, so the plugin surface always names "provision
// request" rather than "compute-process request" in the message.
func TestParentInfoFromProvisionRequest_RejectsMalformedJson(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ProvisionRequest{
		ParentInfoJson: []byte("not json"),
	}
	_, err := ParentInfoFromProvisionRequest(req)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("provision request"))
}

// TestDependencyOutputsFromProvisionRequest_RoundTrips — a
// map[string]*api.PluginResourceOutput marshalled by SC core's
// plugin_sidecars.go into ProvisionRequest.DependencyOutputsJson arrives
// on the plugin side as map[string]*api.ResourceOutput with the Ref
// pointing at a copied PluginResourceOutput. The sibling-consumer's
// type assertion on `Ref.(*api.PluginResourceOutput)` matches what
// in-tree callers do post-#569.
func TestDependencyOutputsFromProvisionRequest_RoundTrips(t *testing.T) {
	RegisterTestingT(t)

	want := map[string]*api.PluginResourceOutput{
		"my-registry": {
			PluginName: "gcp",
			Urn:        "urn:pulumi:parent::stack::gcp:artifactregistry:Repository::my-registry",
			Outputs: map[string]api.PluginOutputValue{
				"url": {
					Value:  "us-central1-docker.pkg.dev/project/my-registry",
					Secret: false,
				},
				"admin-sa-key": {
					Value:  "secret-bytes",
					Secret: true,
				},
			},
		},
	}
	b, err := json.Marshal(want)
	Expect(err).ToNot(HaveOccurred())

	req := &scpluginv1.ProvisionRequest{
		DependencyOutputsJson: b,
	}
	got, err := DependencyOutputsFromProvisionRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(HaveLen(1))
	Expect(got["my-registry"]).ToNot(BeNil())

	plOut, ok := got["my-registry"].Ref.(*api.PluginResourceOutput)
	Expect(ok).To(BeTrue(), "Ref should type-assert to *api.PluginResourceOutput for sibling consumers")
	Expect(plOut.PluginName).To(Equal("gcp"))
	Expect(plOut.Urn).To(Equal(want["my-registry"].Urn))
	Expect(plOut.Outputs).To(HaveLen(2))
	Expect(plOut.Outputs["url"].Value).To(Equal("us-central1-docker.pkg.dev/project/my-registry"))
	Expect(plOut.Outputs["url"].Secret).To(BeFalse())
	Expect(plOut.Outputs["admin-sa-key"].Secret).To(BeTrue())
}

// TestDependencyOutputsFromComputeProcessRequest_RoundTrips — the
// ComputeProcess-side symmetry of the Provision-side test above. Same
// JSON shape, same round-trip semantics; just a different request type.
// Lights up slice 1d scoped-user mint (sc-component-backup 06721b1,
// design doc §5.2) end-to-end at ComputeProcess time.
func TestDependencyOutputsFromComputeProcessRequest_RoundTrips(t *testing.T) {
	RegisterTestingT(t)

	want := map[string]*api.PluginResourceOutput{
		"the-mongo": {
			PluginName: "mongodb",
			Urn:        "urn:pulumi:parent::stack::mongodbatlas:Cluster::the-mongo",
			Outputs: map[string]api.PluginOutputValue{
				"mongo-uri": {Value: "mongodb+srv://u:p@host/", Secret: true},
				"database":  {Value: "forge_conductor", Secret: false},
			},
		},
	}
	b, err := json.Marshal(want)
	Expect(err).ToNot(HaveOccurred())

	req := &scpluginv1.ComputeProcessRequest{
		DependencyOutputsJson: b,
	}
	got, err := DependencyOutputsFromComputeProcessRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(HaveLen(1))
	Expect(got["the-mongo"]).ToNot(BeNil())

	plOut, ok := got["the-mongo"].Ref.(*api.PluginResourceOutput)
	Expect(ok).To(BeTrue())
	Expect(plOut.PluginName).To(Equal("mongodb"))
	Expect(plOut.Outputs).To(HaveLen(2))
	Expect(plOut.Outputs["mongo-uri"].Secret).To(BeTrue())
	Expect(plOut.Outputs["database"].Value).To(Equal("forge_conductor"))
}

// TestDependencyOutputsFromComputeProcessRequest_EmptyMeansNil — older
// SC core, or a ComputeProcess call whose consumer carries no plugin-owned
// sibling outputs, returns (nil, nil). The plugin's ComputeProcessor reads
// nil and skips the scoped-user mint path (backward-compat).
func TestDependencyOutputsFromComputeProcessRequest_EmptyMeansNil(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ComputeProcessRequest{}
	got, err := DependencyOutputsFromComputeProcessRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeNil())
}

// TestDependencyOutputsFromComputeProcessRequest_RejectsMalformedJson —
// if SC core packs garbage, surface it rather than silent-continue.
func TestDependencyOutputsFromComputeProcessRequest_RejectsMalformedJson(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ComputeProcessRequest{
		DependencyOutputsJson: []byte("{not valid json"),
	}
	_, err := DependencyOutputsFromComputeProcessRequest(req)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("decode DependencyOutputs from compute-process request"))
}

// TestDependencyOutputsFromProvisionRequest_EmptyMeansNil — a request
// that carries no dependencies (either the resource's config doesn't
// implement api.WithParentDependencies, or an older SC core pre-dates
// this field) returns (nil, nil). The plugin-side `params.ResourceOutputs`
// map stays unset and sibling-reading bodies error normally with their
// own resource-not-in-outputs sentence.
func TestDependencyOutputsFromProvisionRequest_EmptyMeansNil(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ProvisionRequest{}
	got, err := DependencyOutputsFromProvisionRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeNil())
}

// TestDependencyOutputsFromProvisionRequest_RejectsMalformedJson — a
// corrupted field (should never happen, but a proto-handling bug could
// land here) returns a wrapped error naming the provision request.
func TestDependencyOutputsFromProvisionRequest_RejectsMalformedJson(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ProvisionRequest{
		DependencyOutputsJson: []byte("not json"),
	}
	_, err := DependencyOutputsFromProvisionRequest(req)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("provision request"))
}

// TestComponentRawFromComputeProcessRequest_RoundTrips — mirrors the
// Provision-side ComponentRaw test on the ComputeProcess side. Same JSON
// shape (a flat map[string]any of the operator's component `.Raw`), same
// decoding semantics, different RPC carrier. Lights up a component
// plugin's compute path reading its own `databases:` / `atlasAdmin:` /
// ... config — the symmetric gap that kept sc-component-backup's
// `maybeProvisionScopedUser` a no-op on the live path.
func TestComponentRawFromComputeProcessRequest_RoundTrips(t *testing.T) {
	RegisterTestingT(t)

	want := map[string]any{
		"databases": []any{
			map[string]any{"resource": "the-mongo", "driver": "mongodb"},
		},
		"atlasAdmin": map[string]any{
			"publicKey":  "pub",
			"privateKey": "priv",
			"projectId":  "pid",
		},
	}
	b, err := json.Marshal(want)
	Expect(err).ToNot(HaveOccurred())

	req := &scpluginv1.ComputeProcessRequest{ComponentRaw: b}
	got, err := ComponentRawFromComputeProcessRequest(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	Expect(got["databases"]).To(HaveLen(1))
	admin, ok := got["atlasAdmin"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(admin["projectId"]).To(Equal("pid"))
}

// TestComponentRawFromComputeProcessRequest_EmptyMeansNil — a request
// with no ComponentRaw (every non-component ComputeProcess call, or an
// older SC core) returns (nil, nil). Plugins fall back to an empty
// Config.Raw and compute-process behaves as it did before the field.
func TestComponentRawFromComputeProcessRequest_EmptyMeansNil(t *testing.T) {
	RegisterTestingT(t)

	got, err := ComponentRawFromComputeProcessRequest(&scpluginv1.ComputeProcessRequest{})
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeNil())
}

// TestComponentRawFromComputeProcessRequest_RejectsMalformedJson — a
// corrupted field surfaces as a wrapped error naming the compute-process
// request.
func TestComponentRawFromComputeProcessRequest_RejectsMalformedJson(t *testing.T) {
	RegisterTestingT(t)

	req := &scpluginv1.ComputeProcessRequest{ComponentRaw: []byte("{not valid json")}
	_, err := ComponentRawFromComputeProcessRequest(req)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("decode ComponentRaw from compute-process request"))
}
