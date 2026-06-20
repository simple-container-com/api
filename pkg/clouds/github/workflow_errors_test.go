package github

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGenerateWorkflows_MkdirAllError(t *testing.T) {
	RegisterTestingT(t)

	// outputPath points at an existing regular file, so MkdirAll fails.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "iam-a-file")
	Expect(os.WriteFile(filePath, []byte("x"), 0o644)).To(Succeed())

	wg := NewWorkflowGenerator(deterministicConfig(), "s", filePath, false)
	err := wg.GenerateWorkflows()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("create output directory"))
}

func TestGenerateWorkflows_WriteFileError(t *testing.T) {
	RegisterTestingT(t)

	// Pre-create a directory where the workflow file should be written, so
	// os.WriteFile fails with "is a directory".
	dir := t.TempDir()
	cfg := deterministicConfig()
	cfg.WorkflowGeneration.Templates = []string{"deploy"}
	collision := filepath.Join(dir, "deploy-s.yml")
	Expect(os.MkdirAll(collision, 0o755)).To(Succeed())

	wg := NewWorkflowGenerator(cfg, "s", dir, false)
	err := wg.GenerateWorkflows()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("write workflow file"))
}

func TestWorkflowGenerator_SyncWorkflows_RemoveError(t *testing.T) {
	RegisterTestingT(t)

	wg := NewWorkflowGenerator(deterministicConfig(), "s", t.TempDir(), false)
	// Removing a file that does not exist surfaces an error.
	plan := &SyncPlan{FilesToRemove: []string{"does-not-exist.yml"}}
	err := wg.SyncWorkflows(plan)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("remove workflow"))
}

func TestWorkflowGenerator_ValidateWorkflows_UnreadableFile(t *testing.T) {
	RegisterTestingT(t)

	// A workflow path that exists but is a directory cannot be read as a file,
	// driving the InvalidFiles branch of ValidateWorkflows.
	dir := t.TempDir()
	cfg := deterministicConfig()
	cfg.WorkflowGeneration.Templates = []string{"deploy"}
	Expect(os.MkdirAll(filepath.Join(dir, "deploy-s.yml"), 0o755)).To(Succeed())

	wg := NewWorkflowGenerator(cfg, "s", dir, false)
	res, err := wg.ValidateWorkflows()
	Expect(err).ToNot(HaveOccurred())
	Expect(res.IsValid).To(BeFalse())
	Expect(res.InvalidFiles).To(HaveKey("deploy-s.yml"))
}
