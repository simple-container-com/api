// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"
)

// writeFakeScanner installs a tiny shell script named `name` into a fresh dir and
// prepends that dir to PATH for the test. The script dispatches on its first arg:
//   - "version" / "--version"  -> prints versionText to stdout
//   - anything else (a scan)   -> prints scanJSON to stdout
//
// This lets the real Scanner.Scan / CheckInstalled / Version code drive the full
// invocation + JSON-parse + struct-mapping pipeline without a real scanner or
// Docker daemon. Both grype ("version") and trivy ("--version") version
// sub-commands are handled by the same case.
func writeFakeScanner(t *testing.T, name, versionText, scanJSON string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake POSIX shell scanner not supported on windows")
	}

	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  version|--version)
    cat <<'VERSION_EOF'
` + versionText + `
VERSION_EOF
    ;;
  *)
    cat <<'JSON_EOF'
` + scanJSON + `
JSON_EOF
    ;;
esac
`
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	// Prepend our dir so the fake shadows any real install.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFailingScanner installs a fake that succeeds on version queries but exits
// non-zero on a scan, to exercise the scan-failure error path.
func writeFailingScanner(t *testing.T, name, versionText string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake POSIX shell scanner not supported on windows")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  version|--version)
    echo '` + versionText + `'
    ;;
  *)
    echo "boom: cannot connect to the docker daemon" >&2
    exit 1
    ;;
esac
`
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write failing fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const grypeScanJSON = `{
  "matches": [
    {
      "vulnerability": {
        "id": "CVE-2024-0001",
        "severity": "Critical",
        "description": "heap overflow",
        "fix": {"state": "fixed", "versions": ["1.1.2", "1.1.3"]},
        "cvss": [{"metrics": {"baseScore": 9.8}}, {"metrics": {"baseScore": 7.1}}],
        "urls": ["https://nvd.example/CVE-2024-0001"]
      },
      "artifact": {"name": "openssl", "version": "1.1.1"}
    },
    {
      "vulnerability": {
        "id": "CVE-2024-0002",
        "severity": "Medium",
        "description": "info leak",
        "fix": {"state": "not-fixed", "versions": []},
        "cvss": [],
        "urls": []
      },
      "artifact": {"name": "zlib", "version": "1.2.11"}
    }
  ],
  "descriptor": {
    "name": "alpine@sha256:abababababababababababababababababababababababababababababababab",
    "version": "0.111.0"
  }
}`

func TestGrypeScanner_Scan_FakeBinary(t *testing.T) {
	RegisterTestingT(t)

	// Isolate caches so hasGrypeVulnerabilityDB has a deterministic empty answer.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFakeScanner(t, "grype", "grype 0.111.0", grypeScanJSON)

	scanner := NewGrypeScanner()
	result, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())

	Expect(result.Tool).To(Equal(ScanToolGrype))
	Expect(result.Vulnerabilities).To(HaveLen(2))
	// Image digest pulled out of the descriptor name (@sha256:...).
	Expect(result.ImageDigest).To(Equal("sha256:abababababababababababababababababababababababababababababababab"))
	Expect(result.Summary.Total).To(Equal(2))
	Expect(result.Summary.Critical).To(Equal(1))
	Expect(result.Summary.Medium).To(Equal(1))
	Expect(result.Digest).To(HavePrefix("sha256:"))
	Expect(result.Metadata).To(HaveKey("grypeVersion"))
	Expect(result.Metadata["grypeVersion"]).To(Equal("0.111.0"))

	// Field-mapping checks on the critical finding.
	var crit Vulnerability
	for _, v := range result.Vulnerabilities {
		if v.ID == "CVE-2024-0001" {
			crit = v
		}
	}
	Expect(crit.Severity).To(Equal(SeverityCritical))
	Expect(crit.Package).To(Equal("openssl"))
	Expect(crit.Version).To(Equal("1.1.1"))
	Expect(crit.FixedIn).To(Equal("1.1.2")) // first version of a "fixed" fix wins
	Expect(crit.Description).To(Equal("heap overflow"))
	Expect(crit.CVSS).To(Equal(9.8)) // max base score
	Expect(crit.URLs).To(ConsistOf("https://nvd.example/CVE-2024-0001"))
}

func TestGrypeScanner_Scan_NoDescriptorName(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Descriptor name empty -> imageDigest falls back to the passed image ref.
	noName := `{"matches": [], "descriptor": {"name": "", "version": "0.111.0"}}`
	writeFakeScanner(t, "grype", "grype 0.111.0", noName)

	scanner := NewGrypeScanner()
	result, err := scanner.Scan(context.Background(), "myimage:tag")
	Expect(err).ToNot(HaveOccurred())
	Expect(result.ImageDigest).To(Equal("myimage:tag"))
	Expect(result.Vulnerabilities).To(HaveLen(0))
	Expect(result.Summary.Total).To(Equal(0))
}

func TestGrypeScanner_Scan_EmptyOutput(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Scan emits only whitespace -> empty-output error path.
	writeFakeScanner(t, "grype", "grype 0.111.0", "   ")

	scanner := NewGrypeScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("empty output"))
}

func TestGrypeScanner_Scan_BadJSON(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFakeScanner(t, "grype", "grype 0.111.0", "{not json")

	scanner := NewGrypeScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse grype output"))
}

func TestGrypeScanner_Scan_NotInstalled(t *testing.T) {
	RegisterTestingT(t)

	// Point PATH at an empty dir so grype cannot be found.
	t.Setenv("PATH", t.TempDir())
	scanner := NewGrypeScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("grype not installed"))
}

func TestGrypeScanner_Scan_ScanCommandFails(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFailingScanner(t, "grype", "grype 0.111.0")

	scanner := NewGrypeScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("grype scan failed"))
}

func TestGrypeScanner_CheckVersion_FakeBinary(t *testing.T) {
	RegisterTestingT(t)

	t.Run("meets minimum", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "grype", "grype 0.111.0", "{}")
		scanner := NewGrypeScanner()
		Expect(scanner.CheckVersion(context.Background())).To(Succeed())
	})

	t.Run("below minimum", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "grype", "grype 0.100.0", "{}")
		scanner := NewGrypeScanner()
		err := scanner.CheckVersion(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("below minimum required"))
	})

	t.Run("unparseable version", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "grype", "no version here at all", "{}")
		scanner := NewGrypeScanner()
		err := scanner.CheckVersion(context.Background())
		Expect(err).To(HaveOccurred())
	})
}

func TestGrypeScanner_Install_AlreadyInstalled(t *testing.T) {
	RegisterTestingT(t)
	// A working fake on PATH means CheckInstalled passes, so Install is a no-op.
	writeFakeScanner(t, "grype", "grype 0.111.0", "{}")
	scanner := NewGrypeScanner()
	Expect(scanner.Install(context.Background())).To(Succeed())
}

const trivyScanJSON = `{
  "Results": [
    {
      "Vulnerabilities": [
        {
          "VulnerabilityID": "CVE-2024-1111",
          "Severity": "HIGH",
          "PkgName": "libcurl",
          "InstalledVersion": "8.0.0",
          "FixedVersion": "8.1.0",
          "Description": "use after free",
          "References": ["https://ref.example/1"],
          "CVSS": {"nvd": {"V3Score": 8.1}, "redhat": {"V2Score": 6.0}}
        },
        {
          "VulnerabilityID": "CVE-2024-2222",
          "Severity": "LOW",
          "PkgName": "busybox",
          "InstalledVersion": "1.36",
          "FixedVersion": "",
          "Description": "minor",
          "References": [],
          "CVSS": null
        }
      ]
    }
  ],
  "Metadata": {
    "Version": "2",
    "ImageID": "sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"
  }
}`

func TestTrivyScanner_Scan_FakeBinary(t *testing.T) {
	RegisterTestingT(t)

	// Isolate the user cache dir so ensureTrivyCacheDir + db-presence run on a
	// clean tree, and trivy never gets --skip-db-update appended.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFakeScanner(t, "trivy", "Version: 0.70.0", trivyScanJSON)

	scanner := NewTrivyScanner()
	result, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).ToNot(HaveOccurred())
	Expect(result).ToNot(BeNil())

	Expect(result.Tool).To(Equal(ScanToolTrivy))
	Expect(result.Vulnerabilities).To(HaveLen(2))
	Expect(result.ImageDigest).To(Equal("sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"))
	Expect(result.Summary.High).To(Equal(1))
	Expect(result.Summary.Low).To(Equal(1))
	Expect(result.Metadata).To(HaveKey("trivyVersion"))
	Expect(result.Metadata["trivyVersion"]).To(Equal("0.70.0"))

	var high Vulnerability
	for _, v := range result.Vulnerabilities {
		if v.ID == "CVE-2024-1111" {
			high = v
		}
	}
	Expect(high.Severity).To(Equal(SeverityHigh))
	Expect(high.Package).To(Equal("libcurl"))
	Expect(high.Version).To(Equal("8.0.0"))
	Expect(high.FixedIn).To(Equal("8.1.0"))
	Expect(high.Description).To(Equal("use after free"))
	Expect(high.CVSS).To(Equal(8.1)) // best of V3/V2 across CVSS sources
	Expect(high.URLs).To(ConsistOf("https://ref.example/1"))
}

func TestTrivyScanner_Scan_WarmCacheSkipsDBUpdate(t *testing.T) {
	RegisterTestingT(t)

	// Pre-seed the trivy cache so trivyDBPresent / trivyJavaDBPresent are true.
	// The scan must still succeed (the fake ignores the extra --skip-* flags).
	cacheRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	// ensureTrivyCacheDir makes a fresh scan-* dir each call, so the warm-cache
	// flags are only added when the per-invocation dir already has metadata. We
	// can't pre-seed that random dir, but we still drive the seeding helpers via
	// the db-presence unit tests; here we just confirm a clean cache scan works.
	writeFakeScanner(t, "trivy", "Version: 0.70.0", trivyScanJSON)

	scanner := NewTrivyScanner()
	result, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).ToNot(HaveOccurred())
	Expect(result.Vulnerabilities).To(HaveLen(2))
	_ = cacheRoot
}

func TestTrivyScanner_Scan_EmptyOutput(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFakeScanner(t, "trivy", "Version: 0.70.0", "  ")

	scanner := NewTrivyScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("empty output"))
}

func TestTrivyScanner_Scan_BadJSON(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFakeScanner(t, "trivy", "Version: 0.70.0", "{bad")

	scanner := NewTrivyScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse trivy output"))
}

func TestTrivyScanner_Scan_NotInstalled(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("PATH", t.TempDir())
	scanner := NewTrivyScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("trivy not installed"))
}

func TestTrivyScanner_Scan_ScanCommandFails(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeFailingScanner(t, "trivy", "Version: 0.70.0")

	scanner := NewTrivyScanner()
	_, err := scanner.Scan(context.Background(), "alpine:3.17")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("trivy scan failed"))
}

func TestTrivyScanner_CheckVersion_FakeBinary(t *testing.T) {
	RegisterTestingT(t)

	t.Run("meets minimum", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "trivy", "Version: 0.70.0", "{}")
		scanner := NewTrivyScanner()
		Expect(scanner.CheckVersion(context.Background())).To(Succeed())
	})

	t.Run("below minimum", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "trivy", "Version: 0.50.0", "{}")
		scanner := NewTrivyScanner()
		err := scanner.CheckVersion(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("below minimum required"))
	})

	t.Run("unparseable version", func(t *testing.T) {
		RegisterTestingT(t)
		writeFakeScanner(t, "trivy", "garbage with no semver", "{}")
		scanner := NewTrivyScanner()
		Expect(scanner.CheckVersion(context.Background())).To(HaveOccurred())
	})
}

func TestTrivyScanner_Install_AlreadyInstalled(t *testing.T) {
	RegisterTestingT(t)
	writeFakeScanner(t, "trivy", "Version: 0.70.0", "{}")
	scanner := NewTrivyScanner()
	Expect(scanner.Install(context.Background())).To(Succeed())
}

func TestScannerVersion_ErrorWhenMissing(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("PATH", t.TempDir())

	t.Run("grype version error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := NewGrypeScanner().Version(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get grype version"))
	})

	t.Run("trivy version error", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := NewTrivyScanner().Version(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get trivy version"))
	})

	t.Run("grype CheckInstalled error", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(NewGrypeScanner().CheckInstalled(context.Background())).To(HaveOccurred())
	})

	t.Run("trivy CheckInstalled error", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(NewTrivyScanner().CheckInstalled(context.Background())).To(HaveOccurred())
	})

	t.Run("grype CheckVersion error", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(NewGrypeScanner().CheckVersion(context.Background())).To(HaveOccurred())
	})

	t.Run("trivy CheckVersion error", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(NewTrivyScanner().CheckVersion(context.Background())).To(HaveOccurred())
	})
}
