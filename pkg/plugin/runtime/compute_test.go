// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

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
