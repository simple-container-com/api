// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package tools

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// ---------------------------------------------------------------------------
// command.go — ExecCommand
//
// ExecCommand shells out to a real binary. We exercise it only with the POSIX
// `sh` interpreter (the same binary installer.go relies on for its install
// scripts), so the tests are deterministic and do not touch any of the actual
// security tools (trivy/grype/syft/cosign).
// ---------------------------------------------------------------------------

func TestExecCommand(t *testing.T) {
	RegisterTestingT(t)

	if runtime.GOOS == "windows" {
		t.Skip("ExecCommand tests rely on a POSIX sh")
	}

	ctx := context.Background()

	t.Run("success captures stdout, empty stderr, nil error", func(t *testing.T) {
		RegisterTestingT(t)
		stdout, stderr, err := ExecCommand(ctx, "sh", []string{"-c", "printf hello"}, nil, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		Expect(stdout).To(Equal("hello"))
		Expect(stderr).To(BeEmpty())
	})

	t.Run("non-zero exit returns ExitError with captured stderr", func(t *testing.T) {
		RegisterTestingT(t)
		// cmd.Output() only populates ExitError.Stderr (the stderr return value)
		// because cmd.Stderr was left nil; stdout is still captured.
		stdout, stderr, err := ExecCommand(ctx, "sh",
			[]string{"-c", "printf out; printf err 1>&2; exit 3"}, nil, 5*time.Second)
		Expect(err).To(HaveOccurred())
		Expect(stdout).To(Equal("out"))
		Expect(stderr).To(Equal("err"))
	})

	t.Run("env is appended to the inherited environment", func(t *testing.T) {
		RegisterTestingT(t)
		stdout, _, err := ExecCommand(ctx, "sh",
			[]string{"-c", "printf %s \"$SC_TEST_VAR\""},
			[]string{"SC_TEST_VAR=injected"}, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		Expect(stdout).To(Equal("injected"))
	})

	t.Run("nil env still inherits process environment", func(t *testing.T) {
		RegisterTestingT(t)
		// PATH is virtually always set in the inherited env; with nil env the
		// branch that skips cmd.Env assignment is taken yet the child still
		// inherits it.
		stdout, _, err := ExecCommand(ctx, "sh",
			[]string{"-c", "[ -n \"$PATH\" ] && printf haspath"}, nil, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
		Expect(stdout).To(Equal("haspath"))
	})

	t.Run("non-existent binary returns a non-ExitError error", func(t *testing.T) {
		RegisterTestingT(t)
		// exec.LookPath fails before launch, so err is NOT *exec.ExitError and
		// the function returns ("", "", err) — empty stdout AND stderr.
		stdout, stderr, err := ExecCommand(ctx,
			"sc-nonexistent-binary-xyz", []string{"arg"}, nil, 5*time.Second)
		Expect(err).To(HaveOccurred())
		Expect(stdout).To(BeEmpty())
		Expect(stderr).To(BeEmpty())
	})

	t.Run("timeout cancels a long-running command", func(t *testing.T) {
		RegisterTestingT(t)
		// QUIRK: ExecCommand uses exec.CommandContext, which on cancellation
		// kills only the DIRECT child. `sh -c "sleep 30"` would leave the
		// grandchild `sleep` holding the stdout pipe open, so cmd.Output()
		// blocks for the full 30s despite the 50ms context timeout (observed
		// on linux). Using `exec sleep` makes sh replace itself with sleep so
		// the killed process IS the sleep — the realistic case for the single
		// binaries (trivy/cosign/...) ExecCommand actually runs.
		start := time.Now()
		_, _, err := ExecCommand(ctx, "sh", []string{"-c", "exec sleep 30"}, nil, 50*time.Millisecond)
		Expect(err).To(HaveOccurred())
		// Must return promptly because of the per-call WithTimeout, well before
		// the 30s sleep would have finished.
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
	})

	t.Run("already-cancelled parent context fails fast", func(t *testing.T) {
		RegisterTestingT(t)
		cctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := ExecCommand(cctx, "sh", []string{"-c", "printf hi"}, nil, 5*time.Second)
		Expect(err).To(HaveOccurred())
	})
}

// ---------------------------------------------------------------------------
// registry.go — GetRequiredTools / GetMinVersion / GetInstallURL
// ---------------------------------------------------------------------------

func toolNameSet(tools []ToolMetadata) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func TestRegistryGetRequiredTools(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()

	tests := []struct {
		name       string
		operations []string
		want       []string
	}{
		{"empty operations -> no tools", []string{}, []string{}},
		{"sign maps to cosign", []string{"sign"}, []string{"cosign"}},
		{"signing maps to cosign", []string{"signing"}, []string{"cosign"}},
		{"verify maps to cosign", []string{"verify"}, []string{"cosign"}},
		{"provenance maps to cosign", []string{"provenance"}, []string{"cosign"}},
		{"sbom maps to syft", []string{"sbom"}, []string{"syft"}},
		{"scan maps to grype", []string{"scan"}, []string{"grype"}},
		{"grype maps to grype", []string{"grype"}, []string{"grype"}},
		{"trivy maps to trivy", []string{"trivy"}, []string{"trivy"}},
		{"unknown op ignored", []string{"bogus-op"}, []string{}},
		{
			name:       "multiple ops dedupe cosign and collect distinct tools",
			operations: []string{"sign", "verify", "provenance", "sbom", "scan", "trivy"},
			want:       []string{"cosign", "grype", "syft", "trivy"},
		},
		{
			name:       "duplicate cosign-producing ops collapse to one entry",
			operations: []string{"sign", "signing", "verify", "provenance"},
			want:       []string{"cosign"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := registry.GetRequiredTools(tt.operations)
			Expect(toolNameSet(got)).To(Equal(tt.want))
		})
	}
}

func TestRegistryGetRequiredToolsMissingFromRegistry(t *testing.T) {
	RegisterTestingT(t)

	// When the tool an operation maps to has been removed, GetRequiredTools
	// must silently skip it rather than emit a zero-value ToolMetadata.
	registry := NewToolRegistry()
	registry.Unregister("cosign")

	got := registry.GetRequiredTools([]string{"sign", "sbom"})
	Expect(toolNameSet(got)).To(Equal([]string{"syft"}))
}

func TestRegistryGetMinVersion(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()

	t.Run("known tool returns its pinned min version", func(t *testing.T) {
		RegisterTestingT(t)
		v, err := registry.GetMinVersion("cosign")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("3.0.2"))
	})

	t.Run("unknown tool returns error and empty string", func(t *testing.T) {
		RegisterTestingT(t)
		v, err := registry.GetMinVersion("nonexistent")
		Expect(err).To(HaveOccurred())
		Expect(v).To(BeEmpty())
	})
}

func TestRegistryGetInstallURL(t *testing.T) {
	RegisterTestingT(t)

	registry := NewToolRegistry()

	t.Run("known tool returns its install URL", func(t *testing.T) {
		RegisterTestingT(t)
		url, err := registry.GetInstallURL("syft")
		Expect(err).ToNot(HaveOccurred())
		Expect(url).To(ContainSubstring("github.com/anchore/syft"))
	})

	t.Run("unknown tool returns error and empty string", func(t *testing.T) {
		RegisterTestingT(t)
		url, err := registry.GetInstallURL("nonexistent")
		Expect(err).To(HaveOccurred())
		Expect(url).To(BeEmpty())
	})
}

// ---------------------------------------------------------------------------
// version.go — ValidateVersion, ParseVersion error branches
// ---------------------------------------------------------------------------

func TestValidateVersion(t *testing.T) {
	RegisterTestingT(t)

	checker := NewVersionChecker()

	tests := []struct {
		name      string
		toolName  string
		installed string
		wantErr   bool
		errSubstr string
	}{
		{"installed equals min passes", "cosign", "3.0.2", false, ""},
		{"installed above min passes", "cosign", "4.1.0", false, ""},
		{"installed below min fails", "cosign", "2.9.9", true, "below minimum required"},
		{"syft min boundary passes", "syft", "1.41.0", false, ""},
		{"syft just below min fails", "syft", "1.40.9", true, "below minimum required"},
		{"unknown tool errors", "nonexistent", "1.0.0", true, "not found in registry"},
		{"unparseable installed version errors", "cosign", "not-a-version", true, "failed to parse installed version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := checker.ValidateVersion(tt.toolName, tt.installed)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateVersionNoMinVersion(t *testing.T) {
	RegisterTestingT(t)

	// A registered tool with an empty MinVersion takes the early-return path:
	// any installed version (even garbage) is accepted because there is nothing
	// to compare against.
	checker := NewVersionChecker()
	checker.registry.Register(ToolMetadata{
		Name:       "no-min-tool",
		Command:    "no-min-tool",
		MinVersion: "",
	})
	defer checker.registry.Unregister("no-min-tool")

	Expect(checker.ValidateVersion("no-min-tool", "literally anything")).To(Succeed())
}

func TestValidateVersionUnparseableMinVersion(t *testing.T) {
	RegisterTestingT(t)

	// A registered tool whose MinVersion is itself unparseable surfaces the
	// "failed to parse required version" branch (the installed version parses
	// fine, so we reach the required-version parse).
	checker := NewVersionChecker()
	checker.registry.Register(ToolMetadata{
		Name:       "bad-min-tool",
		Command:    "bad-min-tool",
		MinVersion: "not-semver",
	})
	defer checker.registry.Unregister("bad-min-tool")

	err := checker.ValidateVersion("bad-min-tool", "1.2.3")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse required version"))
}

func TestParseVersionErrorBranches(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		input     string
		errSubstr string
	}{
		{"single component", "1", "invalid version format"},
		{"non-numeric major", "x.2.3", "invalid major version"},
		{"non-numeric minor", "1.y.3", "invalid minor version"},
		{"non-numeric patch", "1.2.z", "invalid patch version"},
		{"empty patch component", "1.2.", "invalid patch version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			v, err := ParseVersion(tt.input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
			Expect(v).To(BeNil())
		})
	}
}

func TestParseVersionPreservesRawAndStripsPrefix(t *testing.T) {
	RegisterTestingT(t)

	// Raw retains the input with the "v" prefix stripped but the pre-release
	// suffix intact, even though Major/Minor/Patch drop the suffix.
	v, err := ParseVersion("v1.2.3-rc.1")
	Expect(err).ToNot(HaveOccurred())
	Expect(v.Raw).To(Equal("1.2.3-rc.1"))
	Expect(v.String()).To(Equal("1.2.3"))
}

// ---------------------------------------------------------------------------
// installer.go — getRequiredTools, CheckAllTools, CheckInstalledWithVersion
// ---------------------------------------------------------------------------

func TestInstallerGetRequiredTools(t *testing.T) {
	RegisterTestingT(t)

	installer := NewToolInstaller()

	// getRequiredTools currently ignores its config argument and always returns
	// the same fixed set; assert that documented behavior across varied inputs.
	for _, cfg := range []interface{}{nil, "anything", 42, struct{ X int }{X: 1}} {
		got := installer.getRequiredTools(cfg)
		Expect(got).To(ConsistOf("cosign", "syft", "grype", "trivy"))
	}
}

func TestInstallerCheckAllTools(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	t.Run("aggregates errors when required tools are unparseable/missing", func(t *testing.T) {
		RegisterTestingT(t)
		// getRequiredTools returns the four real security tools. On a CI box
		// they are almost certainly absent (or version-mismatched), so
		// CheckAllTools should report a non-nil aggregate error. We do not
		// assert success because that depends on host state.
		installer := NewToolInstaller()
		err := installer.CheckAllTools(ctx, nil)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("tool check failed"))
		}
	})
}

func TestCheckInstalledWithVersionNoMinVersion(t *testing.T) {
	RegisterTestingT(t)

	// A tool whose Command resolves on PATH ("sh") and which has NO MinVersion
	// must pass: CheckInstalled succeeds and the version-check block is skipped.
	installer := NewToolInstaller()
	installer.registry.Register(ToolMetadata{
		Name:       "sh-no-min",
		Command:    "sh",
		MinVersion: "",
	})
	defer installer.registry.Unregister("sh-no-min")

	Expect(installer.CheckInstalledWithVersion(context.Background(), "sh-no-min")).To(Succeed())
}

func TestCheckInstalledWithVersionUnknownTool(t *testing.T) {
	RegisterTestingT(t)

	installer := NewToolInstaller()
	err := installer.CheckInstalledWithVersion(context.Background(), "totally-unknown-tool")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not found in registry"))
}

func TestCheckInstalledWithVersionCommandNotOnPath(t *testing.T) {
	RegisterTestingT(t)

	// Command not on PATH -> CheckInstalled fails before any version logic.
	installer := NewToolInstaller()
	installer.registry.Register(ToolMetadata{
		Name:       "ghost-tool",
		Command:    "sc-ghost-binary-xyz",
		MinVersion: "1.0.0",
		InstallURL: "https://example.com",
	})
	defer installer.registry.Unregister("ghost-tool")

	err := installer.CheckInstalledWithVersion(context.Background(), "ghost-tool")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not found in PATH"))
}

func TestInstallScriptUnknownTool(t *testing.T) {
	RegisterTestingT(t)

	// A valid version string passes the regex gate, so we reach the switch and
	// fall through to the default branch for an unrecognized tool name.
	script, err := installScript("unknown-tool", "1.2.3", "/usr/local/bin")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown tool"))
	Expect(script).To(BeEmpty())
}

func TestInstallScriptKnownToolsEmbedVersionAndDir(t *testing.T) {
	RegisterTestingT(t)

	// trivy is the one tool whose script has no OS/arch detection (Linux-only
	// asset name) — assert the version and install dir are both interpolated.
	script, err := installScript("trivy", "0.68.2", "/opt/bin")
	Expect(err).ToNot(HaveOccurred())
	Expect(script).To(ContainSubstring("trivy_0.68.2_Linux-64bit.tar.gz"))
	Expect(script).To(ContainSubstring("/opt/bin/trivy"))

	// cosign downloads a bare binary (no tarball extraction step).
	cosignScript, err := installScript("cosign", "3.0.2", "/opt/bin")
	Expect(err).ToNot(HaveOccurred())
	Expect(cosignScript).To(ContainSubstring("cosign-linux-amd64"))
	Expect(cosignScript).To(ContainSubstring("/opt/bin/cosign"))
}

// ---------------------------------------------------------------------------
// version.go — GetInstalledVersion / CheckAllToolVersions (exec, exercised via
// a fake tool that points at a real POSIX binary with predictable output).
// ---------------------------------------------------------------------------

func TestGetInstalledVersion(t *testing.T) {
	RegisterTestingT(t)

	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX sh")
	}

	ctx := context.Background()

	t.Run("unknown tool errors before exec", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		v, err := checker.GetInstalledVersion(ctx, "no-such-tool")
		Expect(err).To(HaveOccurred())
		Expect(v).To(BeEmpty())
	})

	t.Run("command failure surfaces error with output", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		// `sh -c "exit 7"` -> VersionFlag is the script; cmd fails.
		checker.registry.Register(ToolMetadata{
			Name:        "failing-tool",
			Command:     "sh",
			VersionFlag: "-cprintf nope; exit 7", // single arg passed as VersionFlag
		})
		defer checker.registry.Unregister("failing-tool")
		_, err := checker.GetInstalledVersion(ctx, "failing-tool")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get version"))
	})

	t.Run("output without a version yields extraction error", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		// `false` exits 0? No — use `true`-like via sh printing no version.
		// VersionFlag must be a single argv element, so embed the whole thing.
		checker.registry.Register(ToolMetadata{
			Name:        "noversion-tool",
			Command:     "printf",
			VersionFlag: "no version here",
		})
		defer checker.registry.Unregister("noversion-tool")
		_, err := checker.GetInstalledVersion(ctx, "noversion-tool")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not extract version"))
	})

	t.Run("happy path extracts version from real command output", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		// printf "version 9.8.7" — VersionFlag carries the format string.
		checker.registry.Register(ToolMetadata{
			Name:        "good-tool",
			Command:     "printf",
			VersionFlag: "version 9.8.7",
		})
		defer checker.registry.Unregister("good-tool")
		v, err := checker.GetInstalledVersion(ctx, "good-tool")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal("9.8.7"))
	})
}

func TestCheckAllToolVersions(t *testing.T) {
	RegisterTestingT(t)

	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX printf")
	}

	ctx := context.Background()

	t.Run("all pass when fake tools report satisfying versions", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		checker.registry.Register(ToolMetadata{
			Name: "ok-a", Command: "printf", VersionFlag: "version 5.0.0", MinVersion: "1.0.0",
		})
		checker.registry.Register(ToolMetadata{
			Name: "ok-b", Command: "printf", VersionFlag: "version 2.3.4", MinVersion: "2.0.0",
		})
		defer checker.registry.Unregister("ok-a")
		defer checker.registry.Unregister("ok-b")

		Expect(checker.CheckAllToolVersions(ctx, []string{"ok-a", "ok-b"})).To(Succeed())
	})

	t.Run("empty list passes", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		Expect(checker.CheckAllToolVersions(ctx, []string{})).To(Succeed())
	})

	t.Run("aggregates a get-version failure and a version-too-low failure", func(t *testing.T) {
		RegisterTestingT(t)
		checker := NewVersionChecker()
		// missing-tool: GetInstalledVersion errors (unknown).
		// stale-tool: version parses but is below MinVersion.
		checker.registry.Register(ToolMetadata{
			Name: "stale-tool", Command: "printf", VersionFlag: "version 0.0.1", MinVersion: "9.0.0",
		})
		defer checker.registry.Unregister("stale-tool")

		err := checker.CheckAllToolVersions(ctx, []string{"missing-tool", "stale-tool"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("version check failed"))
		Expect(err.Error()).To(ContainSubstring("missing-tool"))
		Expect(err.Error()).To(ContainSubstring("stale-tool"))
	})
}
