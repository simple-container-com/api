// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestConvertDescriptor_UnmarshalError(t *testing.T) {
	RegisterTestingT(t)
	// `from` marshals fine, but the YAML re-unmarshals into an incompatible
	// target type (string -> int), surfacing the decode error.
	target := struct {
		N int `yaml:"n"`
	}{}
	_, err := ConvertDescriptor(map[string]any{"n": "not-an-int"}, &target)
	Expect(err).To(HaveOccurred())
}

func TestAuthToString_MarshalError(t *testing.T) {
	RegisterTestingT(t)

	// Happy path is exercised in mapping_read_more_test.go; cover the marshal-failure branch.
	ch := make(chan int)
	Expect(AuthToString(&ch)).To(ContainSubstring("<ERROR:"))
}

func TestRegisterCloudConverters(t *testing.T) {
	RegisterTestingT(t)

	// These registration helpers have no public getter; calling them must not
	// panic and must merge into the package-level maps.
	Expect(func() {
		RegisterCloudComposeConverter(CloudComposeConfigRegister{})
		RegisterCloudSingleImageConverter(CloudSingleImageConfigRegister{})
		RegisterCloudStaticSiteConverter(CloudStaticSiteConfigRegister{})
		RegisterProvisioner(ProvisionerRegisterMap{})
	}).ToNot(Panic())
}

func TestStacksMap_ResolveInheritance_Templates(t *testing.T) {
	RegisterTestingT(t)

	base := Stack{
		Name: "base",
		Server: ServerDescriptor{
			Templates: map[string]StackDescriptor{
				"web":   {Type: "aws-ecs", Config: Config{Config: "base-web"}},
				"other": {Type: "gcp-run", Config: Config{Config: "base-other"}},
			},
		},
	}
	child := Stack{
		Name: "child",
		Server: ServerDescriptor{
			Templates: map[string]StackDescriptor{
				// no slash -> inherit the same template name from "base"
				"web": {Inherit: Inherit{Inherit: "base"}},
				// slash form -> inherit "other" from "base" under a new local name
				"renamed": {Inherit: Inherit{Inherit: "base/other"}},
			},
		},
	}
	m := StacksMap{"base": base, "child": child}

	resolved := *m.ResolveInheritance()
	Expect(resolved["child"].Server.Templates["web"].Type).To(Equal("aws-ecs"))
	Expect(resolved["child"].Server.Templates["renamed"].Type).To(Equal("gcp-run"))
}

func TestDetectPerStackResourcesType_InheritedSkips(t *testing.T) {
	RegisterTestingT(t)

	t.Run("inherited per-env resources are skipped", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{
			Resources: map[string]PerEnvResourcesDescriptor{
				"prod": {Inherit: Inherit{Inherit: "base"}},
			},
		}
		out, err := DetectPerStackResourcesType(p)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).ToNot(BeNil())
	})

	t.Run("inherited per-env resources with resources defined errors", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{
			Resources: map[string]PerEnvResourcesDescriptor{
				"prod": {
					Inherit:   Inherit{Inherit: "base"},
					Resources: map[string]ResourceDescriptor{"db": {Type: "x"}},
				},
			},
		}
		_, err := DetectPerStackResourcesType(p)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inherited, but resources are defined"))
	})

	t.Run("inherited individual resource is skipped", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{
			Resources: map[string]PerEnvResourcesDescriptor{
				"prod": {Resources: map[string]ResourceDescriptor{
					"db": {Inherit: Inherit{Inherit: "base"}},
				}},
			},
		}
		out, err := DetectPerStackResourcesType(p)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).ToNot(BeNil())
	})

	t.Run("inherited resource with type defined errors", func(t *testing.T) {
		RegisterTestingT(t)
		p := &PerStackResourcesDescriptor{
			Resources: map[string]PerEnvResourcesDescriptor{
				"prod": {Resources: map[string]ResourceDescriptor{
					"db": {Type: "x", Inherit: Inherit{Inherit: "base"}},
				}},
			},
		}
		_, err := DetectPerStackResourcesType(p)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("inherited, but type is defined"))
	})
}

func TestReadServerConfigs_PropagatesDetectError(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	// Valid provisioner so detection advances past the first step, then an
	// unknown CiCd type makes a later Detect* step fail — exercising the
	// error-propagation legs of ReadServerConfigs.
	d := ServerDescriptor{
		Provisioner: ProvisionerDescriptor{Type: testProviderType},
		CiCd:        CiCdDescriptor{Type: "ghost-cicd"},
	}
	_, err := ReadServerConfigs(&d)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown cicd type"))
}

func TestDetectAuthType_UnknownProvider(t *testing.T) {
	RegisterTestingT(t)
	d := &SecretsDescriptor{Auth: map[string]AuthDescriptor{
		"a": {Type: "ghost-auth"},
	}}
	_, err := DetectAuthType(d)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown auth type"))
}

func TestDetectTemplatesType_InheritedSkip(t *testing.T) {
	RegisterTestingT(t)
	d := &ServerDescriptor{
		Templates: map[string]StackDescriptor{
			"t": {Inherit: Inherit{Inherit: "base"}},
		},
	}
	out, err := DetectTemplatesType(d)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
}

func TestDetectRegistrarType_InheritedAndEmpty(t *testing.T) {
	RegisterTestingT(t)

	// Inherited registrar short-circuits.
	out, err := DetectRegistrarType(&PerStackResourcesDescriptor{
		Registrar: RegistrarDescriptor{Inherit: Inherit{Inherit: "base"}},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())

	// Empty type is skipped (no registrar configured).
	out, err = DetectRegistrarType(&PerStackResourcesDescriptor{})
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
}
