package kubernetes

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

// TestBuildPodTemplatePatch verifies the pod-template annotation patch targets
// spec.template.metadata, which triggers a rolling restart on change.
func TestBuildPodTemplatePatch(t *testing.T) {
	RegisterTestingT(t)

	annotations := map[string]string{
		"simple-container.com/caddy-update-hash": "abc123",
	}

	patchBytes, err := buildPodTemplatePatch(annotations)
	Expect(err).ToNot(HaveOccurred())

	var patch map[string]interface{}
	Expect(json.Unmarshal(patchBytes, &patch)).To(Succeed())

	// Must have spec.template.metadata.annotations path
	spec, ok := patch["spec"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "patch must have 'spec' key")
	template, ok := spec["template"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "spec must have 'template' key")
	metadata, ok := template["metadata"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "template must have 'metadata' key")
	ann, ok := metadata["annotations"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "metadata must have 'annotations' key")
	Expect(ann["simple-container.com/caddy-update-hash"]).To(Equal("abc123"))

	// Must NOT have top-level metadata key (that would be the deployment, not pod template)
	Expect(patch).ToNot(HaveKey("metadata"))
}

// TestBuildDeploymentMetadataPatch verifies the deployment-level annotation patch targets
// metadata only (not spec.template), so it does NOT trigger pod restarts.
func TestBuildDeploymentMetadataPatch(t *testing.T) {
	RegisterTestingT(t)

	annotations := map[string]string{
		"simple-container.com/caddy-updated-by": "my-stack",
		"simple-container.com/caddy-updated-at": "deadbeef",
	}

	patchBytes, err := buildDeploymentMetadataPatch(annotations)
	Expect(err).ToNot(HaveOccurred())

	var patch map[string]interface{}
	Expect(json.Unmarshal(patchBytes, &patch)).To(Succeed())

	// Must have top-level metadata.annotations
	metadata, ok := patch["metadata"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "patch must have 'metadata' key")
	ann, ok := metadata["annotations"].(map[string]interface{})
	Expect(ok).To(BeTrue(), "metadata must have 'annotations' key")
	Expect(ann["simple-container.com/caddy-updated-by"]).To(Equal("my-stack"))
	Expect(ann["simple-container.com/caddy-updated-at"]).To(Equal("deadbeef"))

	// Must NOT touch spec.template (no rolling restart)
	Expect(patch).ToNot(HaveKey("spec"))
}

// TestPatchTargetsSeparation verifies the two patch helpers produce disjoint JSON structures,
// confirming that informational annotations cannot accidentally trigger pod restarts.
func TestPatchTargetsSeparation(t *testing.T) {
	RegisterTestingT(t)

	podTemplateBytes, err := buildPodTemplatePatch(map[string]string{"k": "v"})
	Expect(err).ToNot(HaveOccurred())

	deploymentBytes, err := buildDeploymentMetadataPatch(map[string]string{"k": "v"})
	Expect(err).ToNot(HaveOccurred())

	var podPatch, deployPatch map[string]interface{}
	Expect(json.Unmarshal(podTemplateBytes, &podPatch)).To(Succeed())
	Expect(json.Unmarshal(deploymentBytes, &deployPatch)).To(Succeed())

	// Pod template patch must NOT have top-level metadata
	Expect(podPatch).ToNot(HaveKey("metadata"))
	// Deployment metadata patch must NOT have spec
	Expect(deployPatch).ToNot(HaveKey("spec"))
}

// TestBuildPreStopLifecycle verifies that preStop sleep injection works correctly.
func TestBuildPreStopLifecycle(t *testing.T) {
	t.Run("nil preStopSleepSeconds returns nil lifecycle", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(buildPreStopLifecycle(nil)).To(BeNil())
	})

	t.Run("zero preStopSleepSeconds returns nil lifecycle", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(buildPreStopLifecycle(lo.ToPtr(0))).To(BeNil())
	})

	t.Run("negative preStopSleepSeconds returns nil lifecycle", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(buildPreStopLifecycle(lo.ToPtr(-1))).To(BeNil())
	})

	t.Run("positive preStopSleepSeconds injects exec sleep", func(t *testing.T) {
		RegisterTestingT(t)

		lifecycle := buildPreStopLifecycle(lo.ToPtr(10))
		Expect(lifecycle).ToNot(BeNil())
		// PreStop is a PtrInput — verify the field is populated (non-nil interface)
		Expect(lifecycle.PreStop).ToNot(BeNil())
	})

	t.Run("preStopSleepSeconds 1 is accepted", func(t *testing.T) {
		RegisterTestingT(t)
		lifecycle := buildPreStopLifecycle(lo.ToPtr(1))
		Expect(lifecycle).ToNot(BeNil(), "smallest valid value should produce a lifecycle")
	})
}
