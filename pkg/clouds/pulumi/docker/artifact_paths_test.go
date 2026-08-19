// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// A stack's security block declares one output.local path, but every security
// operation runs once per image. These tests pin that two images never resolve
// to the same file: sharing one meant a concurrent sbom-att could attach the
// other image's SBOM, and the scan results and PR comment body overwrote each
// other.
func pathTestSecurity() *api.SecurityDescriptor {
	return &api.SecurityDescriptor{
		Enabled: true,
		SBOM: &api.SBOMDescriptor{
			Enabled: true,
			Output:  &api.OutputDescriptor{Local: ".sc/artifacts/sbom.json"},
		},
		Provenance: &api.ProvenanceDescriptor{
			Enabled: true,
			Output:  &api.OutputDescriptor{Local: ".sc/artifacts/provenance.json"},
		},
		Scan: &api.ScanDescriptor{
			Enabled: true,
			Output:  &api.OutputDescriptor{Local: ".sc/artifacts/scan-results.json"},
		},
		Reporting: &api.ReportingDescriptor{
			PRComment: &api.PRCommentDescriptor{Enabled: true, Output: ".sc/artifacts/security-report.md"},
		},
	}
}

const (
	imageA = "celery-worker-withdrawals"
	imageB = "celery-worker-tron-scanner"
)

func TestResolveSBOMOutputPath_DistinctPerImage(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	a := resolveSBOMOutputPath(s, imageA)
	b := resolveSBOMOutputPath(s, imageB)

	Expect(a).ToNot(Equal(b), "two images must not share one SBOM file")
	Expect(a).To(Equal(".sc/artifacts/sbom-celery-worker-withdrawals.json"))
	Expect(b).To(Equal(".sc/artifacts/sbom-celery-worker-tron-scanner.json"))
}

func TestResolveProvenanceOutputPath_DistinctPerImage(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	Expect(resolveProvenanceOutputPath(s, imageA)).ToNot(Equal(resolveProvenanceOutputPath(s, imageB)))
	Expect(resolveProvenanceOutputPath(s, imageA)).To(Equal(".sc/artifacts/provenance-celery-worker-withdrawals.json"))
}

func TestResolveScanOutputPath_DistinctPerImage(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	Expect(resolveScanOutputPath(s, imageA)).ToNot(Equal(resolveScanOutputPath(s, imageB)))
	Expect(resolveScanOutputPath(s, imageA)).To(Equal(".sc/artifacts/scan-results-celery-worker-withdrawals.json"))
}

func TestResolveCommentOutputPath_DistinctPerImage(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	Expect(resolveCommentOutputPath(s, imageA)).ToNot(Equal(resolveCommentOutputPath(s, imageB)))
	Expect(resolveCommentOutputPath(s, imageA)).To(Equal(".sc/artifacts/security-report-celery-worker-withdrawals.md"))
}

// Per-tool split composes on top of the per-image path, so grype and trivy
// results for the same image stay separate too.
func TestResolveToolScanOutputPath_ComposesImageThenTool(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	grypeA := resolveToolScanOutputPath(s, imageA, "grype", true)
	trivyA := resolveToolScanOutputPath(s, imageA, "trivy", true)
	grypeB := resolveToolScanOutputPath(s, imageB, "grype", true)

	Expect(grypeA).To(Equal(".sc/artifacts/scan-results-celery-worker-withdrawals-grype.json"))
	Expect(grypeA).ToNot(Equal(trivyA))
	Expect(grypeA).ToNot(Equal(grypeB))

	Expect(resolveToolScanOutputPath(s, imageA, "grype", false)).To(Equal(resolveScanOutputPath(s, imageA)))
}

// An image reference carrying a registry path, tag or digest must still produce
// one path segment.
func TestResolveSBOMOutputPath_SanitizesImageReference(t *testing.T) {
	RegisterTestingT(t)
	s := pathTestSecurity()

	got := resolveSBOMOutputPath(s, "europe-north1-docker.pkg.dev/proj/repo/app@sha256:abc")

	Expect(got).To(Equal(".sc/artifacts/sbom-europe-north1-docker.pkg.dev-proj-repo-app-sha256-abc.json"))
	Expect(got).To(HavePrefix(".sc/artifacts/"))
}

func TestResolvePaths_UnconfiguredFallbacks(t *testing.T) {
	RegisterTestingT(t)

	bare := &api.SecurityDescriptor{Enabled: true}

	Expect(resolveScanOutputPath(bare, imageA)).To(BeEmpty())
	Expect(resolveProvenanceOutputPath(bare, imageA)).To(BeEmpty())
	Expect(resolveToolScanOutputPath(bare, imageA, "grype", true)).To(BeEmpty())
	Expect(resolveCommentOutputPath(bare, imageA)).To(BeEmpty(), "no prComment block means no path")

	// SBOM falls back to a per-image temp path rather than empty.
	Expect(resolveSBOMOutputPath(bare, imageA)).To(Equal("/tmp/sbom-celery-worker-withdrawals.json"))
	Expect(resolveSBOMOutputPath(bare, imageA)).ToNot(Equal(resolveSBOMOutputPath(bare, imageB)))
}
