package scan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ScanResult represents vulnerability scan results
type ScanResult struct {
	ImageDigest     string                 `json:"imageDigest"`
	Tool            ScanTool               `json:"tool"`
	Vulnerabilities []Vulnerability        `json:"vulnerabilities"`
	Summary         VulnerabilitySummary   `json:"summary"`
	ScannedAt       time.Time              `json:"scannedAt"`
	Digest          string                 `json:"digest"` // SHA256 of content
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// Vulnerability represents a single vulnerability
type Vulnerability struct {
	ID          string                 `json:"id"`          // CVE ID
	Severity    Severity               `json:"severity"`    // Critical, High, Medium, Low, Unknown
	Package     string                 `json:"package"`     // Package name
	Version     string                 `json:"version"`     // Installed version
	FixedIn     string                 `json:"fixedIn"`     // Fixed version (if available)
	Description string                 `json:"description"` // Vulnerability description
	URLs        []string               `json:"urls"`        // Reference URLs
	CVSS        float64                `json:"cvss"`        // CVSS score
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// VulnerabilitySummary aggregates vulnerability counts by severity
type VulnerabilitySummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Total    int `json:"total"`
}

// NewScanResult creates a new ScanResult
func NewScanResult(imageDigest string, tool ScanTool, vulns []Vulnerability) *ScanResult {
	result := &ScanResult{
		ImageDigest:     imageDigest,
		Tool:            tool,
		Vulnerabilities: vulns,
		Summary:         summarizeVulnerabilities(vulns),
		ScannedAt:       time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Calculate digest
	result.Digest = result.calculateDigest()

	return result
}

// calculateDigest calculates SHA256 digest of the scan result
func (r *ScanResult) calculateDigest() string {
	data, err := json.Marshal(r.Vulnerabilities)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash)
}

// ValidateDigest validates the digest matches the content
func (r *ScanResult) ValidateDigest() error {
	expected := r.calculateDigest()
	if r.Digest != expected {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expected, r.Digest)
	}
	return nil
}

// summarizeVulnerabilities aggregates vulnerability counts by severity
func summarizeVulnerabilities(vulns []Vulnerability) VulnerabilitySummary {
	summary := VulnerabilitySummary{}
	for _, v := range vulns {
		summary.Total++
		switch v.Severity {
		case SeverityCritical:
			summary.Critical++
		case SeverityHigh:
			summary.High++
		case SeverityMedium:
			summary.Medium++
		case SeverityLow:
			summary.Low++
		case SeverityUnknown:
			summary.Unknown++
		}
	}
	return summary
}

// String returns a human-readable summary
func (s VulnerabilitySummary) String() string {
	if s.Total == 0 {
		return "No vulnerabilities found"
	}
	return fmt.Sprintf("Found %d critical, %d high, %d medium, %d low vulnerabilities",
		s.Critical, s.High, s.Medium, s.Low)
}

// HasCritical returns true if there are critical vulnerabilities
func (s VulnerabilitySummary) HasCritical() bool {
	return s.Critical > 0
}

// HasHigh returns true if there are high vulnerabilities
func (s VulnerabilitySummary) HasHigh() bool {
	return s.High > 0
}

// HasMedium returns true if there are medium vulnerabilities
func (s VulnerabilitySummary) HasMedium() bool {
	return s.Medium > 0
}

// HasLow returns true if there are low vulnerabilities
func (s VulnerabilitySummary) HasLow() bool {
	return s.Low > 0
}

// MergeResults merges multiple scan results, deduplicating by vulnerability ID and package coordinates.
// When multiple scanners report the same package-level finding, the higher severity and richer metadata win.
func MergeResults(results ...*ScanResult) *ScanResult {
	if len(results) == 0 {
		return nil
	}

	// Use map to deduplicate by package-level finding identity.
	vulnMap := make(map[string]Vulnerability)

	var imageDigest string
	tools := []ScanTool{}

	for _, result := range results {
		if result == nil {
			continue
		}

		if imageDigest == "" {
			imageDigest = result.ImageDigest
		}

		tools = append(tools, result.Tool)

		for _, vuln := range result.Vulnerabilities {
			key := vulnerabilityKey(vuln)
			existing, found := vulnMap[key]
			if !found {
				vulnMap[key] = vuln
			} else {
				vulnMap[key] = mergeVulnerability(existing, vuln)
			}
		}
	}

	// Convert map back to slice
	vulns := make([]Vulnerability, 0, len(vulnMap))
	for _, vuln := range vulnMap {
		vulns = append(vulns, vuln)
	}
	sort.Slice(vulns, func(i, j int) bool {
		if severityPriority(vulns[i].Severity) != severityPriority(vulns[j].Severity) {
			return severityPriority(vulns[i].Severity) > severityPriority(vulns[j].Severity)
		}
		if vulns[i].Package != vulns[j].Package {
			return vulns[i].Package < vulns[j].Package
		}
		if vulns[i].Version != vulns[j].Version {
			return vulns[i].Version < vulns[j].Version
		}
		return vulns[i].ID < vulns[j].ID
	})

	// Create merged result
	merged := NewScanResult(imageDigest, ScanToolAll, vulns)
	merged.Metadata["mergedTools"] = tools

	return merged
}

func vulnerabilityKey(vuln Vulnerability) string {
	return fmt.Sprintf("%s|%s|%s", vuln.ID, vuln.Package, vuln.Version)
}

func mergeVulnerability(existing, candidate Vulnerability) Vulnerability {
	merged := existing
	if severityPriority(candidate.Severity) > severityPriority(existing.Severity) {
		merged.Severity = candidate.Severity
	}
	if merged.FixedIn == "" {
		merged.FixedIn = candidate.FixedIn
	}
	if merged.Description == "" {
		merged.Description = candidate.Description
	}
	if candidate.CVSS > merged.CVSS {
		merged.CVSS = candidate.CVSS
	}
	merged.URLs = appendUnique(existing.URLs, candidate.URLs...)
	return merged
}

func appendUnique(existing []string, candidates ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	merged := append([]string(nil), existing...)
	for _, value := range merged {
		seen[value] = struct{}{}
	}
	for _, value := range candidates {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		merged = append(merged, value)
		seen[value] = struct{}{}
	}
	return merged
}

// severityPriority returns priority for severity comparison (higher = more severe)
func severityPriority(s Severity) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	case SeverityUnknown:
		return 0
	default:
		return 0
	}
}
