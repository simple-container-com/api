package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// helpersBuildDir must be DETERMINISTIC across calls for a given image name —
// this is the property that kills the pulumi-docker `build: update` phantom
// diff. A regression here (e.g. reverting to os.MkdirTemp's random suffix)
// reintroduces a per-preview diff on every helpers image, so guard it directly.
func TestHelpersBuildDir_DeterministicAcrossCalls(t *testing.T) {
	RegisterTestingT(t)

	const name = "cloudtrail-security-audit--prod-security-helpers"
	first := helpersBuildDir(name)
	second := helpersBuildDir(name)

	Expect(first).To(Equal(second), "build dir must be identical across calls for the same image name")
	Expect(first).To(HavePrefix(os.TempDir()), "build dir must live under the OS temp dir")
	Expect(filepath.Dir(first)).To(Equal(filepath.Clean(os.TempDir())),
		"build dir must be a single segment under TempDir, not nested/escaped")
}

// Distinct image names (e.g. the audit vs critical cloudtrail stacks, or the
// ECS/ALB cloud-helpers image) must map to distinct dirs so concurrent stacks
// in one provision process never clobber each other's Dockerfile.
func TestHelpersBuildDir_DistinctPerImageName(t *testing.T) {
	RegisterTestingT(t)

	audit := helpersBuildDir("cloudtrail-security-audit--prod-security-helpers")
	critical := helpersBuildDir("cloudtrail-security-critical--prod-security-helpers")
	ecs := helpersBuildDir("sc-cloud-helpers")

	Expect(audit).NotTo(Equal(critical))
	Expect(audit).NotTo(Equal(ecs))
	Expect(critical).NotTo(Equal(ecs))
}

// Path separators in the image name must be neutralised so the result stays a
// single path segment under TempDir (no directory traversal / escape).
func TestHelpersBuildDir_SanitisesSeparators(t *testing.T) {
	RegisterTestingT(t)

	dir := helpersBuildDir("../../etc/evil:tag")
	base := filepath.Base(dir)

	Expect(filepath.Dir(dir)).To(Equal(filepath.Clean(os.TempDir())),
		"a malicious image name must not escape TempDir")
	Expect(base).NotTo(ContainSubstring("/"))
	Expect(base).NotTo(ContainSubstring("\\"))
	Expect(base).NotTo(ContainSubstring(":"))
	Expect(strings.HasPrefix(base, "sc-helpers-")).To(BeTrue())
}
