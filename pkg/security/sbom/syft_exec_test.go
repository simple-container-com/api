// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package sbom

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"
)

// fakeSyft installs a shell script named "syft" into a fresh temp dir and
// prepends that dir to PATH so exec.LookPath resolves it ahead of any real
// syft binary. The script branches on its first argument:
//   - "version": prints versionLine to stdout
//   - anything else (the registry:IMAGE form): prints sbomStdout to stdout and
//     sbomStderr to stderr, then exits with exitCode.
//
// This lets us drive the SyftGenerator.Generate / Version / CheckInstalled /
// CheckVersion code paths without a real syft install or registry access.
func fakeSyft(t *testing.T, versionLine, sbomStdout, sbomStderr string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake syft shell script requires a POSIX shell")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"version\" ]; then\n" +
		"  printf '%s' " + shellQuote(versionLine) + "\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s' " + shellQuote(sbomStdout) + "\n" +
		"printf '%s' " + shellQuote(sbomStderr) + " 1>&2\n" +
		"exit " + itoa(exitCode) + "\n"

	path := filepath.Join(dir, "syft")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake syft: %v", err)
	}

	// Prepend our dir so it wins over /usr/local/bin/syft if present.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// shellQuote single-quotes a string for safe embedding in /bin/sh.
func shellQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func TestSyftGeneratorGenerate_FakeSyft(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Unsupported format errors before exec", func(t *testing.T) {
		RegisterTestingT(t)
		g := NewSyftGenerator()
		_, err := g.Generate(context.Background(), "app:v1", Format("bogus"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not supported by syft"))
	})

	t.Run("CycloneDX JSON success extracts package count and digest", func(t *testing.T) {
		RegisterTestingT(t)
		stdout := `{"bomFormat":"CycloneDX","components":[{"name":"a"},{"name":"b"}]}`
		stderr := "loaded image sha256:abc123def4567890123456789012345678901234567890123456789012345678"
		fakeSyft(t, "syft 1.41.0", stdout, stderr, 0)

		g := NewSyftGenerator()
		sbom, err := g.Generate(context.Background(), "app:v1", FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).ToNot(BeNil())
		Expect(sbom.Format).To(Equal(FormatCycloneDXJSON))
		Expect(string(sbom.Content)).To(Equal(stdout))
		Expect(sbom.Metadata).ToNot(BeNil())
		Expect(sbom.Metadata.ToolName).To(Equal("syft"))
		Expect(sbom.Metadata.ToolVersion).To(Equal("1.41.0"))
		Expect(sbom.Metadata.PackageCount).To(Equal(2))
		Expect(sbom.ImageDigest).To(Equal("sha256:abc123def4567890123456789012345678901234567890123456789012345678"))
		Expect(sbom.ValidateDigest()).To(BeTrue())
	})

	t.Run("SPDX JSON success extracts package count", func(t *testing.T) {
		RegisterTestingT(t)
		stdout := `{"packages":[{"name":"a"},{"name":"b"},{"name":"c"}]}`
		fakeSyft(t, "syft 1.41.0", stdout, "", 0)

		g := NewSyftGenerator()
		sbom, err := g.Generate(context.Background(), "app@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", FormatSPDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Metadata.PackageCount).To(Equal(3))
		// No digest in stderr -> falls back to the @sha256 portion of the image ref.
		Expect(sbom.ImageDigest).To(Equal("sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"))
	})

	t.Run("Non-JSON format skips package count", func(t *testing.T) {
		RegisterTestingT(t)
		stdout := "SPDXVersion: SPDX-2.3\n"
		fakeSyft(t, "syft 1.41.0", stdout, "", 0)

		g := NewSyftGenerator()
		sbom, err := g.Generate(context.Background(), "app:v1", FormatSPDXTagValue)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Metadata.PackageCount).To(Equal(0))
		Expect(string(sbom.Content)).To(Equal(stdout))
	})

	t.Run("syft command failure surfaces stderr", func(t *testing.T) {
		RegisterTestingT(t)
		fakeSyft(t, "syft 1.41.0", "", "manifest unknown", 1)

		g := NewSyftGenerator()
		_, err := g.Generate(context.Background(), "app:v1", FormatCycloneDXJSON)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("syft command failed"))
		Expect(err.Error()).To(ContainSubstring("manifest unknown"))
	})

	t.Run("empty syft output errors", func(t *testing.T) {
		RegisterTestingT(t)
		fakeSyft(t, "syft 1.41.0", "", "", 0)

		g := NewSyftGenerator()
		_, err := g.Generate(context.Background(), "app:v1", FormatCycloneDXJSON)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty output"))
	})

	t.Run("success with unparseable version falls back to unknown", func(t *testing.T) {
		RegisterTestingT(t)
		stdout := `{"components":[]}`
		fakeSyft(t, "this output has no version", stdout, "", 0)

		g := NewSyftGenerator()
		sbom, err := g.Generate(context.Background(), "app:v1", FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Metadata.ToolVersion).To(Equal("unknown"))
	})

	t.Run("invalid JSON content still produces SBOM with zero package count", func(t *testing.T) {
		RegisterTestingT(t)
		// JSON-based format but malformed content: extractPackageCount errors and
		// the count silently stays 0 (err is swallowed by design).
		stdout := `{not valid json`
		fakeSyft(t, "syft 1.41.0", stdout, "", 0)

		g := NewSyftGenerator()
		sbom, err := g.Generate(context.Background(), "app:v1", FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Metadata.PackageCount).To(Equal(0))
	})
}

func TestSyftGeneratorVersion_FakeSyft(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name        string
		versionLine string
		wantVersion string
		wantErr     bool
	}{
		{"Standard syft prefix", "syft 1.41.0", "1.41.0", false},
		{"Syft prefix with v", "syft v1.41.0", "1.41.0", false},
		{"Fallback to any semver", "Application: 2.3.4 (built today)", "2.3.4", false},
		{"No parseable version", "no version here at all", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			fakeSyft(t, tt.versionLine, "", "", 0)
			g := NewSyftGenerator()
			version, err := g.Version(context.Background())
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				Expect(version).To(Equal("unknown"))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(version).To(Equal(tt.wantVersion))
			}
		})
	}

	t.Run("version subcommand exits non-zero", func(t *testing.T) {
		RegisterTestingT(t)
		// A fake syft whose version subcommand fails -> CombinedOutput errors.
		dir := t.TempDir()
		path := filepath.Join(dir, "syft")
		Expect(os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755)).To(Succeed())
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

		g := NewSyftGenerator()
		version, err := g.Version(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get syft version"))
		Expect(version).To(Equal(""))
	})
}

func TestCheckInstalled_FakeSyft(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Installed", func(t *testing.T) {
		RegisterTestingT(t)
		fakeSyft(t, "syft 1.41.0", "", "", 0)
		Expect(CheckInstalled(context.Background())).To(Succeed())
	})

	t.Run("Not installed (version exits non-zero)", func(t *testing.T) {
		RegisterTestingT(t)
		// A fake syft whose version subcommand fails.
		dir := t.TempDir()
		path := filepath.Join(dir, "syft")
		Expect(os.WriteFile(path, []byte("#!/bin/sh\nexit 3\n"), 0o755)).To(Succeed())
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

		err := CheckInstalled(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not installed or not in PATH"))
	})
}

func TestCheckVersion_FakeSyft(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Meets minimum", func(t *testing.T) {
		RegisterTestingT(t)
		fakeSyft(t, "syft 1.41.0", "", "", 0)
		Expect(CheckVersion(context.Background(), "1.0.0")).To(Succeed())
	})

	t.Run("Below minimum errors", func(t *testing.T) {
		RegisterTestingT(t)
		fakeSyft(t, "syft 1.0.0", "", "", 0)
		err := CheckVersion(context.Background(), "2.0.0")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("older than required"))
	})

	t.Run("Version probe failure propagates", func(t *testing.T) {
		RegisterTestingT(t)
		// version subcommand produces no parseable version -> Version returns error.
		fakeSyft(t, "garbage with no semver", "", "", 0)
		err := CheckVersion(context.Background(), "1.0.0")
		Expect(err).To(HaveOccurred())
	})
}
