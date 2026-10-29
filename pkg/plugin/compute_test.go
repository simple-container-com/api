// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"testing"

	. "github.com/onsi/gomega"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestComputeResultBuilder — the shape test: what a ComputeProcessor plugin
// author writes, and what comes out on the wire. Each env var carries its
// metadata (resource_name/type/stack_name) so the SC-core sidecar can route
// the var to the right scope; each tpl extension carries its kind so the
// sidecar picks between AddResourceTplExtension and
// AddDependencyTplExtension.
func TestComputeResultBuilder_BuildsTheWireShape(t *testing.T) {
	RegisterTestingT(t)

	resp, err := NewComputeResult().
		Urn("urn:pulumi:test::proj::mongodbatlas:index/cluster:Cluster::primary").
		// An opaque placeholder, not a real connection URI shape: the
		// secret-flag plumbing is what this test asserts, and TruffleHog
		// pattern-matches live-looking URI samples even inside test
		// fixtures (verified unverifiable finding on PR #511, 2026-10-29).
		SecretEnvVar("MONGO_URI", "placeholder-secret-value", "mongodb-atlas", "primary", "test-stack").
		EnvVar("MONGO_DB", "mydb", "mongodb-atlas", "primary", "test-stack").
		ResourceTplExtension("primary", map[string]string{
			"connectionString": "mongodb+srv://cluster.example.com",
			"database":         "mydb",
		}).
		DependencyTplExtension("backend", "primary", map[string]string{
			"env": "test",
		}).
		Build()
	Expect(err).ToNot(HaveOccurred())

	Expect(resp.GetUrn()).To(Equal("urn:pulumi:test::proj::mongodbatlas:index/cluster:Cluster::primary"))
	Expect(resp.GetEnvVars()).To(HaveLen(2))

	Expect(resp.GetEnvVars()[0].GetName()).To(Equal("MONGO_URI"))
	Expect(resp.GetEnvVars()[0].GetSecret()).To(BeTrue(), "SecretEnvVar must set the secret flag")
	Expect(resp.GetEnvVars()[0].GetResourceName()).To(Equal("primary"))
	Expect(resp.GetEnvVars()[0].GetResourceType()).To(Equal("mongodb-atlas"))
	Expect(resp.GetEnvVars()[0].GetStackName()).To(Equal("test-stack"))

	Expect(resp.GetEnvVars()[1].GetName()).To(Equal("MONGO_DB"))
	Expect(resp.GetEnvVars()[1].GetSecret()).To(BeFalse(), "a plain EnvVar must not be flagged secret")

	Expect(resp.GetTplExtensions()).To(HaveLen(2))

	Expect(resp.GetTplExtensions()[0].GetKind()).To(Equal(scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_RESOURCE))
	Expect(resp.GetTplExtensions()[0].GetResName()).To(Equal("primary"))
	Expect(resp.GetTplExtensions()[0].GetDepName()).To(Equal(""), "resource-kind extensions carry no dep_name")
	Expect(resp.GetTplExtensions()[0].GetValues()).To(HaveKeyWithValue("connectionString", "mongodb+srv://cluster.example.com"))
	Expect(resp.GetTplExtensions()[0].GetValues()).To(HaveKeyWithValue("database", "mydb"))

	Expect(resp.GetTplExtensions()[1].GetKind()).To(Equal(scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_DEPENDENCY))
	Expect(resp.GetTplExtensions()[1].GetResName()).To(Equal("primary"))
	Expect(resp.GetTplExtensions()[1].GetDepName()).To(Equal("backend"))
}

// TestComputeResultBuilder_RefusesEmptyIdentifiers — the author errors are
// raised locally so the plugin gets them without a gRPC round-trip, with
// the author's problem named. SC core's equivalent refusals (an env var
// with no name is a silent drop) land as a shape-breaking failure at the
// consuming stack with no mention of the plugin.
func TestComputeResultBuilder_RefusesEmptyIdentifiers(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewComputeResult().EnvVar("", "val", "t", "r", "s").Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("env var with no name"))

	_, err = NewComputeResult().SecretEnvVar("", "val", "t", "r", "s").Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("secret env var with no name"))

	_, err = NewComputeResult().ResourceTplExtension("", map[string]string{"k": "v"}).Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("resource tpl extension with no resource name"))

	_, err = NewComputeResult().DependencyTplExtension("", "r", nil).Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("dependency tpl extension"))

	_, err = NewComputeResult().DependencyTplExtension("d", "", nil).Build()
	Expect(err).To(HaveOccurred())
}

// TestComputeResultBuilder_FirstErrorSurvivesTheRestOfTheChain — a setter
// that fails does not stop the chain. Build is where the error surfaces.
// Same pattern as VmHostBuilder: lets a handler read as one expression.
func TestComputeResultBuilder_FirstErrorSurvivesTheRestOfTheChain(t *testing.T) {
	RegisterTestingT(t)

	_, err := NewComputeResult().
		EnvVar("", "oops", "t", "r", "s").
		EnvVar("GOOD", "value", "t", "r", "s").
		Build()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("env var with no name"), "the FIRST error must win, not the last")
}

// TestComputeResultBuilder_EmptyBuildIsFine — a compute processor that
// chooses to emit nothing (because the resource was adopted, or the stack
// has no consumers) returns a valid empty response. SC core's sidecar
// treats this as the v1 shape (no-op) and the deploy continues.
func TestComputeResultBuilder_EmptyBuildIsFine(t *testing.T) {
	RegisterTestingT(t)

	resp, err := NewComputeResult().Build()
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.GetEnvVars()).To(BeEmpty())
	Expect(resp.GetTplExtensions()).To(BeEmpty())
	Expect(resp.GetUrn()).To(Equal(""))
}
