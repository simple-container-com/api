package docker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/api"
)

// buildSecurityReportScript generates a shell script that reads scan results and
// outputs a unified security summary to the console, $GITHUB_STEP_SUMMARY, and
// an optional markdown file for PR comments.
func buildSecurityReportScript(imageRef, imageName string, security *api.SecurityDescriptor, commentOutputPath string) string {
	scanResultsPath := ""
	if security.Scan != nil && security.Scan.Output != nil && security.Scan.Output.Local != "" {
		scanResultsPath = security.Scan.Output.Local
	}

	// Build the report script. Uses jq for JSON parsing if available, falls back to grep.
	// The script is self-contained — no SC CLI dependency.
	//
	// The heading includes imageName so stacks with multiple images (e.g. web+worker)
	// produce visibly-distinct report sections in $GITHUB_STEP_SUMMARY — otherwise
	// identical-looking headers make them appear as duplicates.
	var sb strings.Builder
	sb.WriteString("set +e\n") // Don't exit on error — report is best-effort
	sb.WriteString("REPORT=''\n")
	sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}## Security Pipeline Summary — %s\\n\\n\"\n", shellEscape(imageName)))
	sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}**Image:** \\`%s\\`\\n\\n\"\n", shellEscape(imageRef)))
	sb.WriteString("REPORT=\"${REPORT}| Step | Status | Details |\\n\"\n")
	sb.WriteString("REPORT=\"${REPORT}| --- | --- | --- |\\n\"\n")

	writeScanStatus(&sb, security, scanResultsPath)
	writeSignStatus(&sb, security)
	writeSBOMStatus(&sb, security)
	writeProvenanceStatus(&sb, security)
	writeDefectDojoStatus(&sb, security)

	sb.WriteString("REPORT=\"${REPORT}\\n\"\n")

	writeVulnerabilityTable(&sb, security, scanResultsPath)
	writeReportOutputs(&sb, commentOutputPath)

	return sb.String()
}

func writeScanStatus(sb *strings.Builder, security *api.SecurityDescriptor, scanResultsPath string) {
	if security.Scan != nil && security.Scan.Enabled {
		scanStatus := "✅ Completed"
		if security.Scan.SoftFail {
			scanStatus = "⚠️ Warning (soft-fail)"
		}
		if scanResultsPath != "" {
			sb.WriteString(fmt.Sprintf(`SCAN_DETAIL="see logs"
if [ -f %[1]s ]; then
  if command -v jq >/dev/null 2>&1; then
    CRITICAL=$(jq -r '.summary.critical // 0' %[1]s 2>/dev/null)
    HIGH=$(jq -r '.summary.high // 0' %[1]s 2>/dev/null)
    TOTAL=$(jq -r '.summary.total // 0' %[1]s 2>/dev/null)
    SCAN_DETAIL="${CRITICAL} critical, ${HIGH} high, ${TOTAL} total"
  else
    TOTAL=$(grep -o '"total":[0-9]*' %[1]s 2>/dev/null | head -1 | cut -d: -f2)
    SCAN_DETAIL="${TOTAL:-?} total vulnerabilities"
  fi
fi
`, shellQuote(scanResultsPath)))
		} else {
			sb.WriteString("SCAN_DETAIL=\"see logs\"\n")
		}
		sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}| Scan | %s | ${SCAN_DETAIL} |\\n\"\n", scanStatus))
	} else {
		sb.WriteString("REPORT=\"${REPORT}| Scan | ⏭️ Disabled | |\\n\"\n")
	}
}

func writeSignStatus(sb *strings.Builder, security *api.SecurityDescriptor) {
	if security.Signing != nil && security.Signing.Enabled {
		mode := "key-based"
		if security.Signing.Keyless {
			mode = "keyless (OIDC)"
		}
		sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}| Sign | ✅ Signed | %s |\\n\"\n", mode))
		if security.Signing.Verify != nil && security.Signing.Verify.Enabled {
			sb.WriteString("REPORT=\"${REPORT}| Verify | ✅ Verified | Signature valid |\\n\"\n")
		}
	} else {
		sb.WriteString("REPORT=\"${REPORT}| Sign | ⏭️ Disabled | |\\n\"\n")
	}
}

func writeSBOMStatus(sb *strings.Builder, security *api.SecurityDescriptor) {
	verifyEnabled := security.Signing != nil && security.Signing.Verify != nil && security.Signing.Verify.Enabled
	if security.SBOM != nil && security.SBOM.Enabled {
		sbomDetail := security.SBOM.Format
		if shouldAttachSBOM(security.SBOM) && signingEnabled(security) {
			if verifyEnabled {
				sbomDetail += ", attestation verified"
			} else {
				sbomDetail += ", attestation attached"
			}
		}
		sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}| SBOM | ✅ Generated | %s |\\n\"\n", sbomDetail))
	} else {
		sb.WriteString("REPORT=\"${REPORT}| SBOM | ⏭️ Disabled | |\\n\"\n")
	}
}

func writeProvenanceStatus(sb *strings.Builder, security *api.SecurityDescriptor) {
	verifyEnabled := security.Signing != nil && security.Signing.Verify != nil && security.Signing.Verify.Enabled
	if security.Provenance != nil && security.Provenance.Enabled {
		provDetail := security.Provenance.Format
		if shouldAttachProvenance(security.Provenance) && signingEnabled(security) {
			if verifyEnabled {
				provDetail += ", attestation verified"
			} else {
				provDetail += ", attestation attached"
			}
		}
		sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}| Provenance | ✅ Attached | %s |\\n\"\n", provDetail))
	} else {
		sb.WriteString("REPORT=\"${REPORT}| Provenance | ⏭️ Disabled | |\\n\"\n")
	}
}

func writeDefectDojoStatus(sb *strings.Builder, security *api.SecurityDescriptor) {
	if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
		ddCfg := security.Reporting.DefectDojo
		detail := fmt.Sprintf("[%s](%s) · %s / %s",
			ddCfg.URL, ddCfg.URL,
			ddCfg.ProductName, ddCfg.EngagementName)
		sb.WriteString(fmt.Sprintf("REPORT=\"${REPORT}| DefectDojo | ✅ Uploaded | %s |\\n\"\n",
			shellEscape(detail)))
	}
}

func writeVulnerabilityTable(sb *strings.Builder, security *api.SecurityDescriptor, scanResultsPath string) {
	if scanResultsPath == "" || security.Scan == nil || !security.Scan.Enabled {
		return
	}
	sb.WriteString(fmt.Sprintf(`if [ -f %[1]s ] && command -v jq >/dev/null 2>&1; then
  VULN_COUNT=$(jq -r '.vulnerabilities | length' %[1]s 2>/dev/null || echo 0)
  if [ "$VULN_COUNT" -gt 0 ] 2>/dev/null; then
    VULN_TABLE=$(jq -r '
      .vulnerabilities
      | sort_by(-({"critical":4,"high":3,"medium":2,"low":1}[.severity] // 0))
      | .[]
      | "| \(.severity | ascii_upcase) | \(.id) | \(.package) | \(.version) | \(.fixedIn // "-") |"
    ' %[1]s 2>/dev/null)
    if [ -n "$VULN_TABLE" ]; then
      if [ "$VULN_COUNT" -gt 10 ]; then
        REPORT="${REPORT}<details>\n<summary>Vulnerabilities (${VULN_COUNT} findings)</summary>\n\n"
      else
        REPORT="${REPORT}### Vulnerabilities (${VULN_COUNT} findings)\n\n"
      fi
      REPORT="${REPORT}| Severity | CVE | Package | Installed | Fixed |\n"
      REPORT="${REPORT}| --- | --- | --- | --- | --- |\n"
      REPORT="${REPORT}${VULN_TABLE}\n"
      if [ "$VULN_COUNT" -gt 10 ]; then
        REPORT="${REPORT}\n</details>\n"
      fi
      REPORT="${REPORT}\n"
    fi
  fi
fi
`, shellQuote(scanResultsPath)))
}

func writeReportOutputs(sb *strings.Builder, commentOutputPath string) {
	// Print to console
	sb.WriteString("printf '%b' \"$REPORT\"\n")

	// Write to GITHUB_STEP_SUMMARY if available (GitHub Actions)
	sb.WriteString("if [ -n \"$GITHUB_STEP_SUMMARY\" ]; then\n")
	sb.WriteString("  printf '%b' \"$REPORT\" >> \"$GITHUB_STEP_SUMMARY\"\n")
	sb.WriteString("fi\n")

	// Write to comment output file if configured
	if commentOutputPath != "" {
		sb.WriteString(fmt.Sprintf("mkdir -p %s\n", shellQuote(filepath.Dir(commentOutputPath))))
		sb.WriteString(fmt.Sprintf("printf '%%b' \"$REPORT\" > %s\n", shellQuote(commentOutputPath)))
	}
}
