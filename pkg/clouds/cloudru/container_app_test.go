// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// Every name ToAppName produces must satisfy ValidateAppName, because the
// provisioner calls them in exactly that order and a name the API rejects comes
// back as a bare 400.
func TestToAppNameAlwaysValid(t *testing.T) {
	RegisterTestingT(t)

	for _, in := range []string{
		"atrium--staging",
		"forge-conductor--staging-ru",
		"sc",
		"9lives--prod",
		"Atrium__Staging",
		"a-b",
		strings.Repeat("verylongstackname", 5) + "--staging",
	} {
		out := ToAppName(in)
		Expect(ValidateAppName(out)).To(Succeed(), "ToAppName(%q) = %q", in, out)
	}
}

func TestToAppNameNormalisation(t *testing.T) {
	RegisterTestingT(t)

	// The common case is a pass-through: a client stack is `<stack>--<env>` and the
	// doubled hyphen is legal.
	Expect(ToAppName("atrium--staging")).To(Equal("atrium--staging"))
	// Underscores and upper case are normalised, not rejected.
	Expect(ToAppName("Atrium_Staging")).To(Equal("atrium-staging"))
	// A leading digit is illegal and is dropped rather than failing the deploy.
	Expect(ToAppName("9lives--prod")).To(Equal("lives--prod"))
	// Padded, because the minimum is 5 and `sc` is a legal stack name.
	Expect(ToAppName("sc")).To(Equal("sc000"))
	// Truncation keeps the prefix and must not leave a trailing hyphen.
	long := ToAppName(strings.Repeat("ab", 30) + "--staging")
	Expect(len(long)).To(Equal(MaxAppNameLen))
	Expect(long).NotTo(HaveSuffix("-"))
	// Nothing salvageable: empty, which ValidateAppName then refuses loudly.
	Expect(ToAppName("---")).To(Equal(""))
	Expect(ValidateAppName("")).NotTo(Succeed())
}

func TestContainerAppExtrasDefaults(t *testing.T) {
	RegisterTestingT(t)

	extras := &ContainerAppExtras{}
	Expect(extras.Validate()).To(Succeed())
	Expect(extras.EffectiveContainerPort()).To(Equal(DefaultContainerPort))
	Expect(extras.EffectiveCPU()).To(Equal(DefaultCPU))
	Expect(extras.EffectiveIdleTimeout()).To(Equal(DefaultIdleTimeout))
	// Never defaulted to true: it runs the container as root, and the images that
	// need it need it because they were built to run as root.
	Expect(extras.EffectivePrivileged()).To(BeFalse())
	// The default has to clear the platform minimum, or the default config is the
	// one combination that CrashLoops behind a green deploy.
	Expect(DefaultContainerPort).To(BeNumerically(">", evolution.MinContainerPort))
}

// The failure this refuses is a runtime CrashLoop behind a successful deploy, so
// it has to be caught at config-parse time or not at all.
func TestContainerAppExtrasRefusesLowPortWithoutPrivileged(t *testing.T) {
	RegisterTestingT(t)

	extras := &ContainerAppExtras{ContainerPort: lo.ToPtr(80)}
	err := extras.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("privileged"))

	extras.Privileged = lo.ToPtr(true)
	Expect(extras.Validate()).To(Succeed())
}

func TestContainerAppExtrasValidation(t *testing.T) {
	RegisterTestingT(t)

	Expect((&ContainerAppExtras{Concurrency: lo.ToPtr(0)}).Validate()).NotTo(Succeed())
	Expect((&ContainerAppExtras{Concurrency: lo.ToPtr(1)}).Validate()).To(Succeed())
	// A container name is checked against the SAME regex as the app, which means a
	// container called `app` is a 400 purely on length.
	Expect((&ContainerAppExtras{ContainerName: "app"}).Validate()).NotTo(Succeed())
	Expect((&ContainerAppExtras{ContainerName: "atrium"}).Validate()).To(Succeed())
	Expect((&ContainerAppExtras{ContainerPort: lo.ToPtr(70000)}).Validate()).NotTo(Succeed())
}

func TestReadContainerAppExtrasAbsentBlock(t *testing.T) {
	RegisterTestingT(t)

	extras, err := ReadContainerAppExtras(nil)
	Expect(err).To(BeNil())
	Expect(extras.EffectiveContainerPort()).To(Equal(DefaultContainerPort))
}

func TestToContainerAppConfig(t *testing.T) {
	RegisterTestingT(t)

	tpl := &ContainerAppTemplateConfig{
		EvolutionAccountConfig: EvolutionAccountConfig{
			KeyID:     "key-id",
			KeySecret: "key-secret",
			ProjectID: "project-id",
		},
		RegistryName: "sc-smoke",
	}
	stackCfg := &api.StackConfigSingleImage{
		Image: &api.ContainerImage{Context: "."},
	}

	out, err := ToContainerAppConfig(tpl, stackCfg)
	Expect(err).To(BeNil())
	input, ok := out.(*ContainerAppInput)
	Expect(ok).To(BeTrue())
	// RegistryName is carried over explicitly: ConvertAuth rehydrates only
	// credential fields, so a template field that is not one is silently lost
	// unless it is copied here.
	Expect(input.RegistryName).To(Equal("sc-smoke"))
	Expect(input.ProjectID).To(Equal("project-id"))
	Expect(input.ProviderType()).To(Equal(ProviderType))
}

// A cloudExtras error is a config error, and the operator is looking at the config
// at the moment the converter runs — not half an hour later, mid-provision.
func TestToContainerAppConfigRejectsBadExtras(t *testing.T) {
	RegisterTestingT(t)

	var extras any = map[string]any{"containerPort": 80}
	_, err := ToContainerAppConfig(&ContainerAppTemplateConfig{}, &api.StackConfigSingleImage{
		Image:       &api.ContainerImage{Context: "."},
		CloudExtras: &extras,
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("privileged"))
}
