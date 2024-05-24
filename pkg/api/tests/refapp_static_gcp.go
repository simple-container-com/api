package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

var RefappStaticGCPServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner: api.ProvisionerDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	Secrets: api.SecretsConfigDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	CiCd: api.CiCdDescriptor{
		Inherit: api.Inherit{Inherit: "common"},
	},
	Templates: map[string]api.StackDescriptor{
		"static-website": {
			Type: gcloud.TemplateTypeStaticWebsite,
			Config: api.Config{Config: &gcloud.TemplateConfig{
				Credentials: gcloud.Credentials{
					Credentials: api.Credentials{
						Credentials: "${auth:gcloud}",
					},
					ServiceAccountConfig: gcloud.ServiceAccountConfig{
						ProjectId: "${auth:gcloud.projectId}",
					},
				},
			}},
			Inherit: api.Inherit{},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template: "static-website",
			},
			"prod": {
				Template: "static-website",
			},
		},
	},
}

var ResolvedRefappStaticGCPServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"static-website": {
			Type: gcloud.TemplateTypeStaticWebsite,
			Config: api.Config{Config: &gcloud.TemplateConfig{
				Credentials: gcloud.Credentials{
					Credentials: api.Credentials{
						Credentials: "<gcloud-service-account-email>",
					},
					ServiceAccountConfig: gcloud.ServiceAccountConfig{
						ProjectId: "test-gcp-project",
					},
				},
			}},
		},
	},
	Variables: map[string]api.VariableDescriptor{},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "static-website",
				Resources: map[string]api.ResourceDescriptor{},
			},
			"prod": {
				Template:  "static-website",
				Resources: map[string]api.ResourceDescriptor{},
			},
		},
	},
}

var RefappStaticGCPClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeStatic,
			ParentStack: "refapp-static-gcp",
			Config: api.Config{
				Config: &api.StackConfigStatic{
					BundleDir: "./bundle",
					Site: api.StaticSiteConfig{
						Domain: "staging.sc-refapp.org",
					},
				},
			},
		},
		"prod": {
			Type:        api.ClientTypeStatic,
			ParentStack: "refapp-static-gcp",
			Config: api.Config{
				Config: &api.StackConfigStatic{
					BundleDir: "./bundle",
					Site: api.StaticSiteConfig{
						Domain: "prod.sc-refapp.org",
					},
				},
			},
		},
	},
}
