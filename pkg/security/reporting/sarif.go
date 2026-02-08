package reporting

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/simple-container-com/api/pkg/security/scan"
)

// SARIF represents the complete SARIF log file format
// Specification: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html
type SARIF struct {
	Version     string            `json:"version"`
	Schema      string            `json:"$schema"`
	Runs        []SARIFRun        `json:"runs"`
	InlineExternalProperties map[string]interface{} `json:"-"`;
}

// SARIFRun represents a single run in the SARIF file
type SARIFRun struct {
	Tool           SARIFTool           `json:"tool"`
	Invocations    []SARIFInvocation   `json:"invocations,omitempty"`
	Results        []SARIFResult       `json:"results"`
	Notifications  []SARIFNotification `json:"notifications,omitempty"`
	Properties     *SARIFRunProperties `json:"properties,omitempty"`
}

// SARIFTool represents the tool that generated the results
type SARIFTool struct {
	Driver SARIFToolDriver `json:"driver"`
}

// SARIFToolDriver represents the tool driver information
type SARIFToolDriver struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version,omitempty"`
	SemanticVersion string                 `json:"semanticVersion,omitempty"`
	InformationURI  string                 `json:"informationUri,omitempty"`
	Rules           []SARIFRule           `json:"rules,omitempty"`
}

// SARIFRule represents a reporting rule
type SARIFRule struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name,omitempty"`
	ShortDescription *SARIFMessage         `json:"shortDescription,omitempty"`
	FullDescription  *SARIFMessage         `json:"fullDescription,omitempty"`
	HelpURI         string                 `json:"helpUri,omitempty"`
	Properties      map[string]interface{} `json:"properties,omitempty"`
}

// SARIFInvocation represents an invocation of the tool
type SARIFInvocation struct {
	CommandLine   string    `json:"commandLine,omitempty"`
	StartTimeUTC  string    `json:"startTimeUtc,omitempty"`
	EndTimeUTC    string    `json:"endTimeUtc,omitempty"`
	Duration      float64   `json:"durationInSeconds,omitempty"`
	ExitCode      int       `json:"exitCode,omitempty"`
	ExitCodeName  string    `json:"exitCodeName,omitempty"`
	ExitSignal    string    `json:"exitSignal,omitempty"`
}

// SARIFResult represents a single result (vulnerability)
type SARIFResult struct {
	RuleID    string                 `json:"ruleId"`
	RuleIndex int                    `json:"ruleIndex,omitempty"`
	Level     string                 `json:"level"`
	Message   SARIFMessage           `json:"message"`
	Locations []SARIFLocation        `json:"locations"`
	CodeFlows []SARIFCodeFlow        `json:"codeFlows,omitempty"`
	Fixes     []SARIFFix             `json:"fixes,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// SARIFLocation represents a location in the artifact
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
	LogicalLocations []SARIFLogicalLocation `json:"logicalLocations,omitempty"`
}

// SARIFPhysicalLocation represents a physical location
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           *SARIFRegion          `json:"region,omitempty"`
}

// SARIFArtifactLocation represents an artifact location
type SARIFArtifactLocation struct {
	URI       string `json:"uri"`
	Index     int    `json:"index,omitempty"`
}

// SARIFRegion represents a region within an artifact
type SARIFRegion struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIFLogicalLocation represents a logical location
type SARIFLogicalLocation struct {
	Name       string `json:"name,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// SARIFCodeFlow represents a code flow
type SARIFCodeFlow struct {
	ThreadFlows []SARIFThreadFlow `json:"threadFlows"`
}

// SARIFThreadFlow represents a thread flow
type SARIFThreadFlow struct {
	Locations []SARIFThreadFlowLocation `json:"locations"`
}

// SARIFThreadFlowLocation represents a location in a thread flow
type SARIFThreadFlowLocation struct {
	Location SARIFLocation `json:"location"`
}

// SARIFFix represents a fix for a result
type SARIFFix struct {
	Description SARIFMessage `json:"description"`
	ArtifactChanges []SARIFArtifactChange `json:"artifactChanges"`
}

// SARIFArtifactChange represents an artifact change
type SARIFArtifactChange struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Replacements     []SARIFReplacement    `json:"replacements"`
}

// SARIFReplacement represents a replacement
type SARIFReplacement struct {
	DeletedRegion SARIFRegion `json:"deletedRegion"`
	InsertedContent *SARIFInsertedContent `json:"insertedContent,omitempty"`
}

// SARIFInsertedContent represents inserted content
type SARIFInsertedContent struct {
	Text string `json:"text,omitempty"`
}

// SARIFMessage represents a message
type SARIFMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}

// SARIFNotification represents a notification
type SARIFNotification struct {
	Level   string      `json:"level,omitempty"`
	Message SARIFMessage `json:"message"`
}

// SARIFRunProperties contains additional properties for the run
type SARIFRunProperties struct {
	ImageRef     string `json:"imageRef,omitempty"`
	ImageDigest  string `json:"imageDigest,omitempty"`
	ScanDuration string `json:"scanDuration,omitempty"`
	ScannedAt    string `json:"scannedAt,omitempty"`
}

// NewSARIFFromScanResult creates a SARIF report from scan results
func NewSARIFFromScanResult(result *scan.ScanResult, imageRef string) (*SARIF, error) {
	if result == nil {
		return nil, fmt.Errorf("scan result is nil")
	}

	// Create rules for each unique vulnerability type
	rules := createSARIFRules(result)

	// Create results from vulnerabilities
	results := createSARIFResults(result)

	// Create invocation info
	invocation := SARIFInvocation{
		StartTimeUTC: result.ScannedAt.Format(time.RFC3339Nano),
		EndTimeUTC:   result.ScannedAt.Add(time.Minute).Format(time.RFC3339Nano), // Estimate
		Duration:     60.0, // Placeholder - would be tracked in actual implementation
		ExitCode:     0,
		ExitCodeName: "SUCCESS",
	}

	// Create the run
	run := SARIFRun{
		Tool: SARIFTool{
			Driver: createToolDriver(result.Tool, rules),
		},
		Invocations: []SARIFInvocation{invocation},
		Results:     results,
		Properties: &SARIFRunProperties{
			ImageRef:    imageRef,
			ImageDigest: result.ImageDigest,
			ScannedAt:   result.ScannedAt.Format(time.RFC3339),
		},
	}

	// Create the SARIF file
	sarif := &SARIF{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs:    []SARIFRun{run},
	}

	return sarif, nil
}

// createToolDriver creates the tool driver information
func createToolDriver(tool scan.ScanTool, rules []SARIFRule) SARIFToolDriver {
	var name, infoURI string
	switch tool {
	case scan.ScanToolGrype:
		name = "Grype"
		infoURI = "https://github.com/anchore/grype"
	case scan.ScanToolTrivy:
		name = "Trivy"
		infoURI = "https://github.com/aquasecurity/trivy"
	case scan.ScanToolAll:
		name = "Simple Container Security"
		infoURI = "https://github.com/simple-container-com/api"
	default:
		name = string(tool)
		infoURI = ""
	}

	return SARIFToolDriver{
		Name:            name,
		SemanticVersion: "1.0.0",
		InformationURI:  infoURI,
		Rules:           rules,
	}
}

// createSARIFRules creates SARIF rules from vulnerabilities
func createSARIFRules(result *scan.ScanResult) []SARIFRule {
	// For vulnerability scanning, we create a generic rule
	// Individual vulnerabilities are instances of this rule
	return []SARIFRule{
		{
			ID: "vulnerability",
			Name: "Security Vulnerability",
			ShortDescription: &SARIFMessage{
				Text: "A security vulnerability was detected in the container image",
			},
			FullDescription: &SARIFMessage{
				Text: "A security vulnerability was detected in a package within the container image. This may allow attackers to compromise the system.",
			},
			Properties: map[string]interface{}{
				"category": "security",
			},
		},
	}
}

// createSARIFResults creates SARIF results from vulnerabilities
func createSARIFResults(result *scan.ScanResult) []SARIFResult {
	results := make([]SARIFResult, 0, len(result.Vulnerabilities))

	for _, vuln := range result.Vulnerabilities {
		sarifResult := SARIFResult{
			RuleID:    "vulnerability",
			RuleIndex: 0,
			Level:     severityToLevel(vuln.Severity),
			Message: SARIFMessage{
				Text: formatVulnerabilityMessage(vuln),
			},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: fmt.Sprintf("pkg:%s@%s", vuln.Package, vuln.Version),
						},
					},
				},
			},
			Properties: map[string]interface{}{
				"vulnerabilityId":   vuln.ID,
				"severity":          string(vuln.Severity),
				"package":           vuln.Package,
				"installedVersion":  vuln.Version,
				"fixedVersion":      vuln.FixedIn,
				"cvssScore":         vuln.CVSS,
				"references":        vuln.URLs,
			},
		}

		// Add fix information if available
		if vuln.FixedIn != "" {
			sarifResult.Fixes = []SARIFFix{
				{
					Description: SARIFMessage{
						Text: fmt.Sprintf("Update to version %s or later", vuln.FixedIn),
					},
					ArtifactChanges: []SARIFArtifactChange{
						{
							ArtifactLocation: SARIFArtifactLocation{
								URI: fmt.Sprintf("pkg:%s", vuln.Package),
							},
							Replacements: []SARIFReplacement{
								{
									DeletedRegion: SARIFRegion{},
									InsertedContent: &SARIFInsertedContent{
										Text: vuln.FixedIn,
									},
								},
							},
						},
					},
				},
			}
		}

		results = append(results, sarifResult)
	}

	return results
}

// severityToLevel converts scan severity to SARIF level
func severityToLevel(severity scan.Severity) string {
	switch severity {
	case scan.SeverityCritical:
		return "error"
	case scan.SeverityHigh:
		return "error"
	case scan.SeverityMedium:
		return "warning"
	case scan.SeverityLow:
		return "note"
	default:
		return "note"
	}
}

// formatVulnerabilityMessage formats a vulnerability as a human-readable message
func formatVulnerabilityMessage(vuln scan.Vulnerability) string {
	msg := fmt.Sprintf("%s: %s in %s@%s", vuln.ID, vuln.Severity, vuln.Package, vuln.Version)
	if vuln.Description != "" {
		msg += fmt.Sprintf(" - %s", vuln.Description)
	}
	if vuln.FixedIn != "" {
		msg += fmt.Sprintf(" (fixed in %s)", vuln.FixedIn)
	}
	return msg
}

// ToJSON converts the SARIF to JSON bytes
func (s *SARIF) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// SaveToFile saves the SARIF report to a file
func (s *SARIF) SaveToFile(path string) error {
	data, err := s.ToJSON()
	if err != nil {
		return fmt.Errorf("marshaling SARIF: %w", err)
	}

	if err := WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing SARIF file: %w", err)
	}

	return nil
}

// WriteFile is a wrapper for os.WriteFile for testing purposes
var WriteFile = os.WriteFile
