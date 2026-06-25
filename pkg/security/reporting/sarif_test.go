// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package reporting

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/scan"
)

func TestNewSARIFFromScanResult(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result errors", func(t *testing.T) {
		RegisterTestingT(t)
		data, err := NewSARIFFromScanResult(nil, "img")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("scan result is required"))
		Expect(data).To(BeNil())
	})

	t.Run("empty result produces valid SARIF skeleton", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{Tool: scan.ScanToolGrype, ImageDigest: "sha256:deadbeef"}
		data, err := NewSARIFFromScanResult(result, "registry/app:1.0")
		Expect(err).ToNot(HaveOccurred())

		var doc sarifReport
		Expect(json.Unmarshal(data, &doc)).To(Succeed())
		Expect(doc.Version).To(Equal("2.1.0"))
		Expect(doc.Schema).To(Equal("https://json.schemastore.org/sarif-2.1.0.json"))
		Expect(doc.Runs).To(HaveLen(1))
		Expect(doc.Runs[0].Tool.Driver.Name).To(Equal("grype"))
		Expect(doc.Runs[0].Tool.Driver.InformationURI).To(Equal("https://docs.simple-container.com"))
		Expect(doc.Runs[0].Results).To(HaveLen(0))
		Expect(doc.Runs[0].Invocations).To(HaveLen(1))
		Expect(doc.Runs[0].Invocations[0].ExecutionSuccessful).To(BeTrue())
		Expect(doc.Runs[0].Properties).To(HaveKeyWithValue("imageRef", "registry/app:1.0"))
		Expect(doc.Runs[0].Properties).To(HaveKeyWithValue("imageDigest", "sha256:deadbeef"))
	})

	t.Run("each vulnerability maps to a result and a rule", func(t *testing.T) {
		RegisterTestingT(t)
		result := &scan.ScanResult{
			Tool: scan.ScanToolTrivy,
			Vulnerabilities: []scan.Vulnerability{
				{
					ID: "CVE-2024-1", Severity: scan.SeverityCritical, Package: "openssl",
					Version: "1.1.1", FixedIn: "1.1.1k", Description: "buffer overflow",
					CVSS: 9.8, URLs: []string{"https://nvd/CVE-2024-1"},
				},
				{
					ID: "CVE-2024-2", Severity: scan.SeverityMedium, Package: "curl",
					Version: "7.0", Description: "info leak",
				},
			},
		}
		data, err := NewSARIFFromScanResult(result, "img")
		Expect(err).ToNot(HaveOccurred())

		var doc sarifReport
		Expect(json.Unmarshal(data, &doc)).To(Succeed())
		run := doc.Runs[0]
		Expect(run.Results).To(HaveLen(2))
		Expect(run.Tool.Driver.Rules).To(HaveLen(2))

		// Result level mapping: critical -> error, medium -> warning.
		levels := map[string]string{}
		for _, r := range run.Results {
			levels[r.RuleID] = r.Level
		}
		Expect(levels).To(HaveKeyWithValue("CVE-2024-1:openssl", "error"))
		Expect(levels).To(HaveKeyWithValue("CVE-2024-2:curl", "warning"))

		// Inspect the critical result's enrichment properties + location URI.
		var crit sarifResult
		for _, r := range run.Results {
			if r.RuleID == "CVE-2024-1:openssl" {
				crit = r
			}
		}
		Expect(crit.Message.Text).To(Equal("CVE-2024-1 affects openssl 1.1.1"))
		Expect(crit.Locations).To(HaveLen(1))
		Expect(crit.Locations[0].PhysicalLocation.ArtifactLocation.URI).To(Equal("pkg:openssl@1.1.1"))
		Expect(crit.PartialFingerprints).To(HaveKeyWithValue("primaryLocationLineHash", "CVE-2024-1|openssl|1.1.1"))
		Expect(crit.Properties).To(HaveKeyWithValue("package", "openssl"))
		Expect(crit.Properties).To(HaveKeyWithValue("installedVersion", "1.1.1"))
		Expect(crit.Properties).To(HaveKeyWithValue("fixedVersion", "1.1.1k"))
	})

	t.Run("same CVE on two packages produces two distinct rules", func(t *testing.T) {
		RegisterTestingT(t)
		// Rule key is CVE+package; libssl3 and openssl share a CVE but must not collide.
		result := &scan.ScanResult{
			Tool: scan.ScanToolGrype,
			Vulnerabilities: []scan.Vulnerability{
				{ID: "CVE-2024-9", Severity: scan.SeverityHigh, Package: "libssl3", Version: "3.0"},
				{ID: "CVE-2024-9", Severity: scan.SeverityHigh, Package: "openssl", Version: "3.0"},
			},
		}
		data, err := NewSARIFFromScanResult(result, "img")
		Expect(err).ToNot(HaveOccurred())

		var doc sarifReport
		Expect(json.Unmarshal(data, &doc)).To(Succeed())
		ruleIDs := make([]string, 0, len(doc.Runs[0].Tool.Driver.Rules))
		for _, rule := range doc.Runs[0].Tool.Driver.Rules {
			ruleIDs = append(ruleIDs, rule.ID)
		}
		Expect(ruleIDs).To(ConsistOf("CVE-2024-9:libssl3", "CVE-2024-9:openssl"))
	})
}

func TestSARIFLevel(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		severity scan.Severity
		want     string
	}{
		{"critical->error", scan.SeverityCritical, "error"},
		{"high->error", scan.SeverityHigh, "error"},
		{"medium->warning", scan.SeverityMedium, "warning"},
		{"low->note", scan.SeverityLow, "note"},
		{"unknown->note", scan.SeverityUnknown, "note"},
		{"unrecognized->note", scan.Severity("weird"), "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(sarifLevel(tc.severity)).To(Equal(tc.want))
		})
	}
}

func TestSARIFToolName(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil result yields generic name", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(sarifToolName(nil)).To(Equal("simple-container"))
	})

	t.Run("merged tool yields multi-scanner name", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(sarifToolName(&scan.ScanResult{Tool: scan.ScanToolAll})).To(Equal("simple-container-multi-scanner"))
	})

	t.Run("single tool yields tool name", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(sarifToolName(&scan.ScanResult{Tool: scan.ScanToolTrivy})).To(Equal("trivy"))
		Expect(sarifToolName(&scan.ScanResult{Tool: scan.ScanToolGrype})).To(Equal("grype"))
	})
}
