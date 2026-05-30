package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/simple-container-com/api/pkg/api"
)

// sanitizeForFilenameRe replaces any char outside the safelist with `_`, used
// when composing a tempfile name from a Pulumi resource name (which may
// contain registry hostnames, `:`, `/`, etc.).
var sanitizeForFilenameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// maxSafeNameLen caps the resource-name component of the staged-script
// basename so the full filename stays under common NAME_MAX limits (255
// bytes on Linux, 255 chars on macOS). A long ECR-derived image name +
// stack + service can easily push the unsanitised resource name past 200
// chars — at which point os.WriteFile returns ENAMETOOLONG and the caller
// falls back to inlining the full script, reintroducing the ARG_MAX
// failure this helper exists to avoid.
const maxSafeNameLen = 64
const stagedScriptPrefix = "sc-security-report-"

// stageSecurityReportScript writes the dynamically-built report script to a
// deterministic tempfile under $TMPDIR and returns the path.
//
// Why: the script is composed from the merged scan-results.json (Trivy +
// Grype) — which on an image carrying a fresh Ubuntu base can run into
// thousands of CVEs. The Pulumi `command:local:Command` resource invokes its
// `Create` field via `/bin/sh -c "<Create>"`, which means the script content
// counts against the kernel's ARG_MAX (typically 128 KB on Linux). On a
// chrome-base-derived image we observed 5,025 merged findings producing a
// >150 KB script body, and every deploy failed with:
//
//	error: fork/exec /bin/sh: argument list too long
//	error: update failed
//
// Staging the script to a tempfile and returning a short `sh <path>`
// invocation keeps the Create argv well under ARG_MAX regardless of how many
// vulnerabilities the report enumerates.
//
// ## Atomicity
//
// Writes through `os.CreateTemp + os.Rename` so concurrent deploys with
// different script content but colliding final paths (extremely unlikely
// given the 64-bit hash, but possible) can't observe a half-written file
// from another writer. `os.Rename` is atomic on the same filesystem; both
// the unique stage file and the final path live in `$TMPDIR`.
//
// ## Path stability
//
// Path is deterministic on (resourceName, script-content) — same inputs
// produce the same path, so Pulumi doesn't see spurious "drift" between
// runs that would otherwise re-trigger the resource on no-op refreshes.
//
// ## Fallback contract
//
// On any filesystem error, returns the empty string and the error; callers
// should fall back to inlining the script (preserves prior behaviour for
// short reports where ARG_MAX is not a concern) AND log the staging error
// so operators can investigate — silent fallback would re-introduce the
// exact failure mode this helper exists to fix.
func stageSecurityReportScript(resourceName, script string) (string, error) {
	sum := sha256.Sum256([]byte(resourceName + "\x00" + script))
	safeName := sanitizeForFilenameRe.ReplaceAllString(resourceName, "_")
	if len(safeName) > maxSafeNameLen {
		safeName = safeName[:maxSafeNameLen]
	}
	finalPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s%s-%s.sh", stagedScriptPrefix, safeName, hex.EncodeToString(sum[:8])))

	tmpFile, err := os.CreateTemp(os.TempDir(), stagedScriptPrefix+"stage-*.sh.tmp")
	if err != nil {
		return "", fmt.Errorf("create stage tempfile: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmpFile.Write([]byte(script)); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return "", fmt.Errorf("write stage tempfile: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		cleanup()
		return "", fmt.Errorf("chmod stage tempfile: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close stage tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return "", fmt.Errorf("rename stage tempfile to %s: %w", finalPath, err)
	}
	return finalPath, nil
}

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
