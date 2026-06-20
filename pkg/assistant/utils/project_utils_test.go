// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package utils

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

// CheckAndWarnExistingSimpleContainerProject writes to stdout when an
// existing SC project is found. The tests don't capture stdout; they
// pin the function's contract via the error return + the
// forceOverwrite / skipConfirmation / interactive flag matrix.

func TestCheckAndWarn_EmptyProject_NoError(t *testing.T) {
	RegisterTestingT(t)

	// Fresh tmp dir with no SC artifacts → returns nil regardless of flags.
	dir := t.TempDir()

	err := CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).ToNot(HaveOccurred())
}

func TestCheckAndWarn_ExistingClientYAML_NonInteractive_Errors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "client.yaml"), []byte("stack: test"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	// Non-interactive + no force-overwrite + no skip-confirm = blocked.
	err = CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("existing Simple Container project"))
}

func TestCheckAndWarn_ExistingClientYAML_ForceOverwrite_Succeeds(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "client.yaml"), []byte("stack: test"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	// forceOverwrite=true short-circuits the warning + always returns nil.
	err = CheckAndWarnExistingSimpleContainerProject(dir, true, false, false)
	Expect(err).ToNot(HaveOccurred())
}

func TestCheckAndWarn_ExistingClientYAML_SkipConfirmation_Succeeds(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "client.yaml"), []byte("stack: test"), 0o644)
	Expect(err).ToNot(HaveOccurred())

	// skipConfirmation=true is the chat / automation path — no warning, no error.
	err = CheckAndWarnExistingSimpleContainerProject(dir, false, true, false)
	Expect(err).ToNot(HaveOccurred())
}

func TestCheckAndWarn_StacksSubdir_NonInteractive_Errors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	stacksDir := filepath.Join(dir, ".sc", "stacks", "prod")
	Expect(os.MkdirAll(stacksDir, 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(stacksDir, "client.yaml"), []byte("a: b"), 0o644)).To(Succeed())

	err := CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).To(HaveOccurred())
}

func TestCheckAndWarn_ServerYAML_NonInteractive_Errors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	Expect(os.WriteFile(filepath.Join(dir, "server.yaml"), []byte(""), 0o644)).To(Succeed())

	err := CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).To(HaveOccurred())
}

func TestCheckAndWarn_SecretsYAML_NonInteractive_Errors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	Expect(os.WriteFile(filepath.Join(dir, "secrets.yaml"), []byte(""), 0o644)).To(Succeed())

	err := CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).To(HaveOccurred())
}

func TestCheckAndWarn_StacksDirOnly_NonInteractive_Errors(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	Expect(os.MkdirAll(filepath.Join(dir, ".sc", "stacks"), 0o755)).To(Succeed())

	err := CheckAndWarnExistingSimpleContainerProject(dir, false, false, false)
	Expect(err).To(HaveOccurred())
}

func TestCheckAndWarn_EmptyPathDefaultsToDot(t *testing.T) {
	RegisterTestingT(t)

	// Switch cwd to a clean tmp dir so the "." default resolves to it.
	dir := t.TempDir()
	orig, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() { _ = os.Chdir(orig) })
	Expect(os.Chdir(dir)).To(Succeed())

	err = CheckAndWarnExistingSimpleContainerProject("", false, false, false)
	Expect(err).ToNot(HaveOccurred())
}
