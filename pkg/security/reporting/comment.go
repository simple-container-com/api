package reporting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/simple-container-com/api/pkg/security/scan"
)

const scanCommentMarker = "<!-- image-scan-report -->"

// BuildScanResultsComment renders a concise markdown summary for PR comments.
func BuildScanResultsComment(imageRef string, result *scan.ScanResult, uploads []*UploadSummary) string {
	if result == nil {
		return scanCommentMarker + "\n## Image Scan Results\n\nNo scan results were produced.\n"
	}

	var b strings.Builder
	b.WriteString(scanCommentMarker + "\n")
	b.WriteString("## Image Scan Results\n\n")
	b.WriteString(fmt.Sprintf("Image: `%s`\n\n", imageRef))
	if result.ImageDigest != "" {
		b.WriteString(fmt.Sprintf("Digest: `%s`\n\n", result.ImageDigest))
	}

	tools := scanTools(result)
	if len(tools) > 0 {
		b.WriteString(fmt.Sprintf("Scanners: `%s`\n\n", strings.Join(tools, "`, `")))
	}

	b.WriteString("| Severity | Count |\n")
	b.WriteString("| --- | ---: |\n")
	b.WriteString(fmt.Sprintf("| Critical | %d |\n", result.Summary.Critical))
	b.WriteString(fmt.Sprintf("| High | %d |\n", result.Summary.High))
	b.WriteString(fmt.Sprintf("| Medium | %d |\n", result.Summary.Medium))
	b.WriteString(fmt.Sprintf("| Low | %d |\n", result.Summary.Low))
	b.WriteString(fmt.Sprintf("| Unknown | %d |\n", result.Summary.Unknown))
	b.WriteString(fmt.Sprintf("| Total | %d |\n\n", result.Summary.Total))

	topFindings := topFindings(result, 5)
	if len(topFindings) == 0 {
		b.WriteString("No critical or high vulnerabilities were found.\n")
	} else {
		b.WriteString("### Top Critical/High Findings\n\n")
		b.WriteString("| Severity | Vulnerability | Package | Installed | Fixed |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, vuln := range topFindings {
			fixedIn := vuln.FixedIn
			if fixedIn == "" {
				fixedIn = "-"
			}
			b.WriteString(fmt.Sprintf("| %s | `%s` | `%s` | `%s` | `%s` |\n",
				strings.ToUpper(string(vuln.Severity)),
				vuln.ID,
				vuln.Package,
				vuln.Version,
				fixedIn,
			))
		}
		b.WriteString("\n")
	}

	if len(uploads) > 0 {
		b.WriteString("### Report Uploads\n\n")
		for _, upload := range uploads {
			status := "uploaded"
			if !upload.Success {
				status = "failed"
			}
			line := fmt.Sprintf("- `%s`: %s", upload.Target, status)
			if upload.URL != "" {
				line += fmt.Sprintf(" (%s)", upload.URL)
			}
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

func scanTools(result *scan.ScanResult) []string {
	if result == nil {
		return nil
	}

	if result.Metadata != nil {
		if mergedTools, ok := result.Metadata["mergedTools"]; ok {
			switch values := mergedTools.(type) {
			case []scan.ScanTool:
				tools := make([]string, 0, len(values))
				for _, tool := range values {
					tools = append(tools, string(tool))
				}
				sort.Strings(tools)
				return tools
			case []interface{}:
				tools := make([]string, 0, len(values))
				for _, tool := range values {
					if name, ok := tool.(string); ok && name != "" {
						tools = append(tools, name)
					}
				}
				sort.Strings(tools)
				return tools
			}
		}
	}

	if result.Tool != "" {
		return []string{string(result.Tool)}
	}

	return nil
}

func topFindings(result *scan.ScanResult, limit int) []scan.Vulnerability {
	if result == nil || limit <= 0 {
		return nil
	}

	findings := make([]scan.Vulnerability, 0, len(result.Vulnerabilities))
	for _, vuln := range result.Vulnerabilities {
		if vuln.Severity == scan.SeverityCritical || vuln.Severity == scan.SeverityHigh {
			findings = append(findings, vuln)
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		left := findings[i]
		right := findings[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		return left.ID < right.ID
	})

	if len(findings) > limit {
		findings = findings[:limit]
	}

	return findings
}

func severityRank(severity scan.Severity) int {
	switch severity {
	case scan.SeverityCritical:
		return 4
	case scan.SeverityHigh:
		return 3
	case scan.SeverityMedium:
		return 2
	case scan.SeverityLow:
		return 1
	default:
		return 0
	}
}
