// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

// provenance.Generate is pure when IncludeGit is false (no git/cosign exec), so
// the full ExecuteProvenance happy path — generate, save-local, summary record —
// is exercisable without external binaries or network. Registry attach is left
// off so cosign is never invoked.

func TestExecuteProvenanceHappyPathNoOutput(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false, // keep Generate hermetic (no git exec)
			Builder:    &BuilderConfig{ID: "gha://acme/builder"},
			Metadata:   &MetadataConfig{IncludeEnv: false, IncludeMaterials: true},
		},
	}, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	stmt, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(stmt).ToNot(BeNil())
	Expect(stmt.ImageRef).To(Equal("registry.example.com/demo@sha256:abc"))
	Expect(stmt.Content).ToNot(BeEmpty())

	// Summary recorded a successful, non-attached provenance.
	Expect(e.Summary.ProvenanceResult).ToNot(BeNil())
	Expect(e.Summary.ProvenanceResult.Attached).To(BeFalse())
}

func TestExecuteProvenanceHappyPathSavesLocal(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "out", "provenance.json")

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Output:     &OutputConfig{Local: outPath, Registry: false},
		},
	})

	stmt, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(stmt).ToNot(BeNil())

	// Statement.Save wrote the predicate content to the configured path (0600).
	info, err := os.Stat(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	data, err := os.ReadFile(outPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(data).To(Equal(stmt.Content))
}

func TestExecuteProvenanceSaveLocalErrorFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Plant a file where a parent directory is needed so Statement.Save's
	// MkdirAll fails. Required=false => fail-open, returns the statement.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
	outPath := filepath.Join(blocker, "child", "provenance.json")

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Required:   false,
			Output:     &OutputConfig{Local: outPath},
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	stmt, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(stmt).ToNot(BeNil()) // fail-open returns the generated statement
}

func TestExecuteProvenanceSaveLocalErrorFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
	outPath := filepath.Join(blocker, "child", "provenance.json")

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Required:   true, // fail-closed: save error becomes a hard error
			Output:     &OutputConfig{Local: outPath},
		},
	})

	_, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("saving provenance locally"))
}

// Registry attach requested but signing disabled: ExecuteProvenance must warn
// and return the statement (fail-open) without invoking cosign.
func TestExecuteProvenanceRegistryAttachRequiresSigningFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e, err := NewSecurityExecutorWithSummary(ctx, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Required:   false,
			Output:     &OutputConfig{Registry: true}, // attach requested
			// Signing nil => attach requires signing.enabled => warn+return.
		},
	}, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	stmt, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(stmt).ToNot(BeNil())
	Expect(e.Summary.ProvenanceResult.Attached).To(BeFalse())
}

func TestExecuteProvenanceRegistryAttachRequiresSigningFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Required:   true, // fail-closed
			Output:     &OutputConfig{Registry: true},
		},
	})

	_, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("signing.enabled"))
}

// Default-metadata path: Metadata nil => IncludeMaterials defaults true,
// IncludeEnv defaults false. Exercises the nil-metadata branch in
// ExecuteProvenance.
func TestExecuteProvenanceNilMetadataDefaults(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	e := newExecutorT(t, &SecurityConfig{
		Enabled: true,
		Provenance: &ProvenanceConfig{
			Enabled:    true,
			Format:     "slsa-v1.0",
			IncludeGit: false,
			Metadata:   nil,
		},
	})

	stmt, err := e.ExecuteProvenance(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(stmt).ToNot(BeNil())
}
