// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestConfigFilePath(t *testing.T) {
	cases := []struct {
		name    string
		workDir string
		profile string
		want    string
	}{
		{
			name:    "default profile under cwd",
			workDir: "/tmp/proj",
			profile: "dev",
			want:    "/tmp/proj/.sc/cfg.dev.yaml",
		},
		{
			name:    "production profile",
			workDir: "/var/work",
			profile: "prod",
			want:    "/var/work/.sc/cfg.prod.yaml",
		},
		{
			name:    "empty workdir",
			workDir: "",
			profile: "default",
			want:    ".sc/cfg.default.yaml",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(ConfigFilePath(tc.workDir, tc.profile)).To(Equal(tc.want))
		})
	}
}

func TestConfigFile_ToYaml(t *testing.T) {
	RegisterTestingT(t)

	cf := &ConfigFile{
		ProjectName:      "my-project",
		PublicKeyPath:    "/path/to/public.key",
		StacksDir:        ".sc/stacks",
		ParentRepository: "github.com/acme/parent",
	}

	y, err := cf.ToYaml()
	Expect(err).ToNot(HaveOccurred())
	yStr := string(y)
	Expect(yStr).To(ContainSubstring("projectName: my-project"))
	Expect(yStr).To(ContainSubstring("publicKeyPath: /path/to/public.key"))
	Expect(yStr).To(ContainSubstring("stacksDir: .sc/stacks"))
	Expect(yStr).To(ContainSubstring("parentRepository: github.com/acme/parent"))
}

func TestConfigFile_WriteAndRead_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	Expect(os.MkdirAll(filepath.Join(dir, ScConfigDirectory), 0o755)).To(Succeed())

	original := &ConfigFile{
		ProjectName:      "roundtrip-project",
		PublicKeyPath:    "/keys/pub",
		StacksDir:        "stacks/",
		ParentRepository: "git@github.com:acme/parent.git",
	}

	Expect(original.WriteConfigFile(dir, "ci")).To(Succeed())

	got, err := ReadConfigFile(dir, "ci")
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	Expect(got.ProjectName).To(Equal("roundtrip-project"))
	Expect(got.PublicKeyPath).To(Equal("/keys/pub"))
	Expect(got.StacksDir).To(Equal("stacks/"))
	Expect(got.ParentRepository).To(Equal("git@github.com:acme/parent.git"))
}

func TestReadConfigFile_FromEnvVariable(t *testing.T) {
	RegisterTestingT(t)

	yamlBlob := `projectName: env-driven
privateKeyPath: /env/path/priv
stacksDir: env-stacks/
`
	t.Setenv(ScConfigEnvVariable, yamlBlob)

	// workDir / profile are ignored when the env var is set.
	got, err := ReadConfigFile("/nonexistent", "any-profile")
	Expect(err).ToNot(HaveOccurred())
	Expect(got.ProjectName).To(Equal("env-driven"))
	Expect(got.PrivateKeyPath).To(Equal("/env/path/priv"))
	Expect(got.StacksDir).To(Equal("env-stacks/"))
}

func TestReadConfigFile_FromEnvVariable_InvalidYaml(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv(ScConfigEnvVariable, "not: valid: yaml: [unbalanced")

	_, err := ReadConfigFile("/nonexistent", "any-profile")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(ScConfigEnvVariable))
}

func TestReadConfigFile_MissingFile(t *testing.T) {
	RegisterTestingT(t)

	// Make sure the env var is not set so we fall through to the file path.
	t.Setenv(ScConfigEnvVariable, "")

	dir := t.TempDir() // empty — no .sc/cfg.dev.yaml inside

	_, err := ReadConfigFile(dir, "dev")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("profile does not exist"))
	Expect(err.Error()).To(ContainSubstring("dev"))
}

func TestConfigDirectoryConstants(t *testing.T) {
	RegisterTestingT(t)

	Expect(ScConfigDirectory).To(Equal(".sc"))
	Expect(EnvConfigFileTemplate).To(Equal("cfg.%s.yaml"))
	Expect(ScConfigEnvVariable).To(Equal("SIMPLE_CONTAINER_CONFIG"))
	Expect(ScContainerResourceTypeEnvVariable).To(Equal("SIMPLE_CONTAINER_RESOURCE_TYPE"))
}

func TestUnmarshalDescriptor_HappyPath(t *testing.T) {
	RegisterTestingT(t)

	yamlBlob := []byte("projectName: u-test\nstacksDir: s/\n")
	got, err := UnmarshalDescriptor[ConfigFile](yamlBlob)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	Expect(got.ProjectName).To(Equal("u-test"))
	Expect(got.StacksDir).To(Equal("s/"))
}

func TestUnmarshalDescriptor_InvalidYaml(t *testing.T) {
	RegisterTestingT(t)

	_, err := UnmarshalDescriptor[ConfigFile]([]byte("not: a: valid: yaml: [unbalanced"))
	Expect(err).To(HaveOccurred())
}

func TestReadDescriptor_HappyPath(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.yaml")
	Expect(os.WriteFile(p, []byte("projectName: read-test\n"), 0o644)).To(Succeed())

	got, err := ReadDescriptor(p, &ConfigFile{})
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	Expect(got.ProjectName).To(Equal("read-test"))
}

func TestReadDescriptor_MissingFile(t *testing.T) {
	RegisterTestingT(t)

	_, err := ReadDescriptor("/no/such/path-9c4f2e/cfg.yaml", &ConfigFile{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to read"))
}

func TestReadDescriptor_InvalidYaml(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	Expect(os.WriteFile(p, []byte("not: valid: yaml: [unbalanced"), 0o644)).To(Succeed())

	_, err := ReadDescriptor(p, &ConfigFile{})
	Expect(err).To(HaveOccurred())
	Expect(strings.Contains(err.Error(), "unmarshal") ||
		strings.Contains(err.Error(), "yaml")).To(BeTrue())
}
