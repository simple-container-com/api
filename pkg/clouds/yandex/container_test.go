// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// validTemplateConfig builds a TemplateConfig whose AccountConfig round-trips
// through api.ConvertAuth (which json-Unmarshals CredentialsValue()). With
// Credentials.Credentials empty, CredentialsValue() returns the JSON of the
// AccountConfig itself, so every account field survives.
func validTemplateConfig() *TemplateConfig {
	return &TemplateConfig{
		AccountConfig: AccountConfig{
			CloudID:         "b1gcloud",
			FolderID:        "b1gfolder",
			AccessKey:       "YCAJE",
			SecretAccessKey: "shh",
			Region:          "ru-central1",
		},
		RegistryID:       "crpexample",
		ServiceAccountID: "aje0template",
	}
}

func TestToServerlessContainerConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps account, template and stack config", func(t *testing.T) {
		RegisterTestingT(t)
		stackCfg := &api.StackConfigSingleImage{
			Domain:       "svc.example.com",
			BaseDnsZone:  "example.com",
			Uses:         []string{"mongodb"},
			Dependencies: []api.StackConfigDependencyResource{{Name: "db", Owner: "other", Resource: "mongodb"}},
		}
		out, err := ToServerlessContainerConfig(validTemplateConfig(), stackCfg)
		Expect(err).ToNot(HaveOccurred())

		in, ok := out.(*ServerlessContainerInput)
		Expect(ok).To(BeTrue())
		Expect(in.FolderID).To(Equal("b1gfolder"))
		Expect(in.Region).To(Equal("ru-central1"))
		// Carried across ConvertAuth, which rehydrates credential fields only.
		Expect(in.RegistryID).To(Equal("crpexample"))
		Expect(in.ServiceAccountID).To(Equal("aje0template"))

		Expect(in.StackConfig.Domain).To(Equal("svc.example.com"))
		Expect(in.Uses()).To(ConsistOf("mongodb"))
		Expect(in.OverriddenBaseZone()).To(Equal("example.com"))
		Expect(in.DependsOnResources()).To(HaveLen(1))
	})

	t.Run("per-service identity overrides the template's", func(t *testing.T) {
		RegisterTestingT(t)
		var raw any
		Expect(yaml.Unmarshal([]byte("serviceAccountId: aje0service\n"), &raw)).To(Succeed())

		out, err := ToServerlessContainerConfig(validTemplateConfig(), &api.StackConfigSingleImage{
			CloudExtras: &raw,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.(*ServerlessContainerInput).ServiceAccountID).To(Equal("aje0service"))
	})

	t.Run("schedules are reachable from the stack config", func(t *testing.T) {
		RegisterTestingT(t)
		var raw any
		Expect(yaml.Unmarshal([]byte(
			"schedules:\n  - name: agents\n    expression: \"* * * * ? *\"\n    request: \"{}\"\n",
		), &raw)).To(Succeed())

		out, err := ToServerlessContainerConfig(validTemplateConfig(), &api.StackConfigSingleImage{
			CloudExtras: &raw,
		})
		Expect(err).ToNot(HaveOccurred())
		extras, err := out.(*ServerlessContainerInput).CloudExtras()
		Expect(err).ToNot(HaveOccurred())
		Expect(extras.Schedules).To(HaveLen(1))
		Expect(extras.Schedules[0].Name).To(Equal("agents"))
	})

	t.Run("a malformed schedule fails conversion, not provisioning", func(t *testing.T) {
		RegisterTestingT(t)
		var raw any
		Expect(yaml.Unmarshal([]byte(
			"schedules:\n  - name: agents\n    expression: \"* * * * *\"\n",
		), &raw)).To(Succeed())

		_, err := ToServerlessContainerConfig(validTemplateConfig(), &api.StackConfigSingleImage{
			CloudExtras: &raw,
		})
		Expect(err).To(MatchError(ContainSubstring("has 5 fields; Yandex requires 6")))
	})

	t.Run("wrong template type errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToServerlessContainerConfig("not-a-template", &api.StackConfigSingleImage{})
		Expect(err).To(MatchError(ContainSubstring("not of type *yandex.TemplateConfig")))
	})

	t.Run("nil template errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToServerlessContainerConfig((*TemplateConfig)(nil), &api.StackConfigSingleImage{})
		Expect(err).To(MatchError(ContainSubstring("template config is nil")))
	})

	t.Run("nil stack config errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ToServerlessContainerConfig(validTemplateConfig(), nil)
		Expect(err).To(MatchError(ContainSubstring("stack config cannot be nil")))
	})
}
