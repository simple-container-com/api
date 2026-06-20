// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestClientDescriptor_Defaults(t *testing.T) {
	RegisterTestingT(t)

	t.Run("HasDefaults", func(t *testing.T) {
		RegisterTestingT(t)
		Expect((&ClientDescriptor{}).HasDefaults()).To(BeFalse())
		Expect((&ClientDescriptor{Defaults: map[string]interface{}{"a": 1}}).HasDefaults()).To(BeTrue())
	})

	t.Run("GetDefaultsSection nil returns empty map", func(t *testing.T) {
		RegisterTestingT(t)
		got := (&ClientDescriptor{}).GetDefaultsSection()
		Expect(got).ToNot(BeNil())
		Expect(got).To(BeEmpty())
	})

	t.Run("GetDefaultsSection returns underlying", func(t *testing.T) {
		RegisterTestingT(t)
		c := &ClientDescriptor{Defaults: map[string]interface{}{"x": "y"}}
		Expect(c.GetDefaultsSection()).To(HaveKeyWithValue("x", "y"))
	})

	t.Run("SetDefaultsSection", func(t *testing.T) {
		RegisterTestingT(t)
		c := &ClientDescriptor{}
		c.SetDefaultsSection(map[string]interface{}{"k": "v"})
		Expect(c.Defaults).To(HaveKeyWithValue("k", "v"))
	})

	t.Run("GetDefaultValue", func(t *testing.T) {
		RegisterTestingT(t)
		c := &ClientDescriptor{Defaults: map[string]interface{}{"present": 42}}
		v, ok := c.GetDefaultValue("present")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(42))

		_, ok = c.GetDefaultValue("absent")
		Expect(ok).To(BeFalse())

		_, ok = (&ClientDescriptor{}).GetDefaultValue("any")
		Expect(ok).To(BeFalse())
	})

	t.Run("SetDefaultValue initialises map", func(t *testing.T) {
		RegisterTestingT(t)
		c := &ClientDescriptor{}
		c.SetDefaultValue("new", "val")
		Expect(c.Defaults).To(HaveKeyWithValue("new", "val"))
		c.SetDefaultValue("new2", "val2")
		Expect(c.Defaults).To(HaveLen(2))
	})
}

func TestResultStringers(t *testing.T) {
	RegisterTestingT(t)

	Expect((&UpdateResult{StackName: "s", Operations: map[string]int{"create": 1}}).String()).
		To(And(ContainSubstring(`"stackName":"s"`), ContainSubstring(`"create":1`)))
	Expect((&PreviewResult{StackName: "p"}).String()).To(ContainSubstring(`"stackName":"p"`))
	Expect((&DestroyResult{Operations: map[string]int{"delete": 2}}).String()).To(ContainSubstring(`"delete":2`))
	Expect((&RefreshResult{Operations: map[string]int{"same": 3}}).String()).To(ContainSubstring(`"same":3`))
}

func TestStackParams_ToProvisionParams(t *testing.T) {
	RegisterTestingT(t)

	p := &StackParams{
		StacksDir:   "/stacks",
		Profile:     "prod",
		StackName:   "web",
		SkipRefresh: true,
		Timeouts:    Timeouts{DeployTimeout: "10m"},
	}
	out := p.ToProvisionParams()
	Expect(out.StacksDir).To(Equal("/stacks"))
	Expect(out.Profile).To(Equal("prod"))
	Expect(out.Stacks).To(Equal([]string{"web"}))
	Expect(out.SkipRefresh).To(BeTrue())
	Expect(out.Timeouts.DeployTimeout).To(Equal("10m"))
}

func TestStackParams_CopyForParentEnv(t *testing.T) {
	RegisterTestingT(t)

	p := &StackParams{
		StackDir:    "/dir",
		Profile:     "prod",
		StackName:   "web",
		Environment: "prod",
		SkipRefresh: true,
		SkipPreview: true,
		Version:     "v1",
		Parent:      true,
	}
	out := p.CopyForParentEnv("staging")
	Expect(out.ParentEnv).To(Equal("staging"))
	Expect(out.StackName).To(Equal("web"))
	Expect(out.Environment).To(Equal("prod"))
	Expect(out.Parent).To(BeTrue())
	// StacksDir is intentionally sourced from StackDir in CopyForParentEnv.
	Expect(out.StacksDir).To(Equal("/dir"))
}

func TestDefaultSecurityDescriptor(t *testing.T) {
	RegisterTestingT(t)

	d := DefaultSecurityDescriptor()
	Expect(d.Enabled).To(BeFalse())
	Expect(d.Signing.Keyless).To(BeTrue())
	Expect(d.SBOM.Format).To(Equal("cyclonedx-json"))
	Expect(d.SBOM.Generator).To(Equal("syft"))
	Expect(d.SBOM.Cache.TTL).To(Equal("24h"))
	Expect(d.Provenance.Format).To(Equal("slsa-v1.0"))
	Expect(d.Scan.FailOn).To(Equal("high"))
	Expect(d.Scan.Tools).To(HaveLen(1))
	Expect(d.Scan.Tools[0].Name).To(Equal("grype"))
	Expect(*d.Scan.Tools[0].Enabled).To(BeTrue())
}
