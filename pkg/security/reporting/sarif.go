package reporting

import (
	"encoding/json"
	"fmt"

	"github.com/simple-container-com/api/pkg/security/scan"
)

type sarifReport struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool              `json:"tool"`
	Results     []sarifResult          `json:"results"`
	Invocations []sarifInvocation      `json:"invocations,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ShortDescription *sarifMessage     `json:"shortDescription,omitempty"`
	Help             *sarifMessage     `json:"help,omitempty"`
	Properties       map[string]string `json:"properties,omitempty"`
}

type sarifResult struct {
	RuleID              string                 `json:"ruleId"`
	Level               string                 `json:"level"`
	Message             sarifMessage           `json:"message"`
	Locations           []sarifLocation        `json:"locations,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
}

// NewSARIFFromScanResult converts a normalized scan result into SARIF 2.1.0.
func NewSARIFFromScanResult(result *scan.ScanResult, imageRef string) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("scan result is required")
	}

	rules := make(map[string]sarifRule, len(result.Vulnerabilities))
	results := make([]sarifResult, 0, len(result.Vulnerabilities))
	for _, vuln := range result.Vulnerabilities {
		rules[vuln.ID] = sarifRule{
			ID:   vuln.ID,
			Name: vuln.ID,
			ShortDescription: &sarifMessage{
				Text: fmt.Sprintf("%s in %s", vuln.ID, vuln.Package),
			},
			Help: &sarifMessage{
				Text: vuln.Description,
			},
			Properties: map[string]string{
				"severity": string(vuln.Severity),
			},
		}

		properties := map[string]interface{}{
			"package":          vuln.Package,
			"installedVersion": vuln.Version,
			"fixedVersion":     vuln.FixedIn,
			"cvss":             vuln.CVSS,
			"references":       vuln.URLs,
		}
		results = append(results, sarifResult{
			RuleID: vuln.ID,
			Level:  sarifLevel(vuln.Severity),
			Message: sarifMessage{
				Text: fmt.Sprintf("%s affects %s %s", vuln.ID, vuln.Package, vuln.Version),
			},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{
						URI: fmt.Sprintf("pkg:%s@%s", vuln.Package, vuln.Version),
					},
				},
			}},
			PartialFingerprints: map[string]string{
				"primaryLocationLineHash": fmt.Sprintf("%s|%s|%s", vuln.ID, vuln.Package, vuln.Version),
			},
			Properties: properties,
		})
	}

	driverRules := make([]sarifRule, 0, len(rules))
	for _, rule := range rules {
		driverRules = append(driverRules, rule)
	}

	document := sarifReport{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifDriver{
					Name:           sarifToolName(result),
					InformationURI: "https://docs.simple-container.com",
					Rules:          driverRules,
				},
			},
			Results: results,
			Invocations: []sarifInvocation{{
				ExecutionSuccessful: true,
			}},
			Properties: map[string]interface{}{
				"imageRef":    imageRef,
				"imageDigest": result.ImageDigest,
				"summary":     result.Summary,
			},
		}},
	}

	return json.MarshalIndent(document, "", "  ")
}

func sarifLevel(severity scan.Severity) string {
	switch severity {
	case scan.SeverityCritical, scan.SeverityHigh:
		return "error"
	case scan.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}

func sarifToolName(result *scan.ScanResult) string {
	if result == nil {
		return "simple-container"
	}
	if result.Tool == scan.ScanToolAll {
		return "simple-container-multi-scanner"
	}
	return string(result.Tool)
}
