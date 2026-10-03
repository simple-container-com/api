// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// TestProviderConfigsRegistered pins the type strings a server.yaml may name. They
// are the provider's entire public surface: a rename here silently turns an existing
// stack into "unknown template type".
func TestProviderConfigsRegistered(t *testing.T) {
	RegisterTestingT(t)

	registered := api.GetRegisteredProviderConfigs()
	for _, typ := range []string{
		AuthTypeYandexServiceAccount,
		SecretsTypeYandexLockbox,
		TemplateTypeYandexServerlessContainer,
		ResourceTypeObjectStorageBucket,
		RegistrarTypeYandexDns,
	} {
		Expect(registered).To(HaveKey(typ), "provider config %q must be registered", typ)
	}

	fields := api.GetRegisteredProvisionerFieldConfigs()
	for _, typ := range []string{
		StateStorageTypeYandexObjectStorage,
		SecretsProviderTypeYandexKms,
	} {
		Expect(fields).To(HaveKey(typ), "provisioner field config %q must be registered", typ)
	}
}

// TestPrepareCloudSingleImageForDeploy walks the real path SC takes for a
// `yc-serverless-container` stack: read the server.yaml template through the
// registered config reader, then convert it with the registered single-image
// converter. Asserting the registrations individually would miss a converter
// registered under a type string no config reader knows.
func TestPrepareCloudSingleImageForDeploy(t *testing.T) {
	RegisterTestingT(t)

	// The template as it appears in server.yaml under `templates:`.
	tpl := api.StackDescriptor{
		Type: TemplateTypeYandexServerlessContainer,
		Config: api.Config{Config: map[string]any{
			"cloudId":          "b1gcloud",
			"folderId":         "b1gfolder",
			"accessKey":        "YCAJE",
			"secretAccessKey":  "shh",
			"registryId":       "crpexample",
			"serviceAccountId": "aje0template",
		}},
	}

	clientCfg := &api.StackConfigSingleImage{
		Domain:      "svc.example.com",
		BaseDnsZone: "example.com",
	}

	out, err := api.PrepareCloudSingleImageForDeploy(
		context.Background(), t.TempDir(), "svc", tpl, clientCfg, "forge/infra",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Type).To(Equal(TemplateTypeYandexServerlessContainer))
	Expect(out.ParentStack).To(Equal("forge/infra"))

	in, ok := out.Config.Config.(*ServerlessContainerInput)
	Expect(ok).To(BeTrue())
	Expect(in.FolderID).To(Equal("b1gfolder"))
	Expect(in.RegistryID).To(Equal("crpexample"))
	Expect(in.StackConfig.Domain).To(Equal("svc.example.com"))
}
