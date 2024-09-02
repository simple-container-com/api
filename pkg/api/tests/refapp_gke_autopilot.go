package tests

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

var RefappGkeAutopilotServerResources = map[string]api.ResourceDescriptor{
	"gke-autopilot-res": {
		Type: "gcp-gke-autopilot-cluster",
		Config: api.Config{
			Config: &gcloud.GkeAutopilotResource{
				Credentials:   CommonGcpCredentials,
				GkeMinVersion: "1.26.5-gke.1200",
			},
		},
		Inherit: api.Inherit{},
	},
	"artifact-registry-res": {
		Type: "gcp-artifact-registry",
		Config: api.Config{
			Config: &gcloud.ArtifactRegistryConfig{
				Credentials: CommonGcpCredentials,
			},
		},
		Inherit: api.Inherit{},
	},
}

var RefappGkeAutopilotServerDescriptor = &api.ServerDescriptor{
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
		"stack-per-app-gke": {
			Type: "gcp-gke-autopilot",
			Config: api.Config{
				Config: &gcloud.GkeAutopilotTemplate{
					Credentials:              CommonGcpCredentials,
					GkeClusterResource:       "gke-autopilot-res",
					ArtifactRegistryResource: "artifact-registry-res",
				},
			},
			Inherit: api.Inherit{},
		},
	},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: api.RegistrarDescriptor{
			Inherit: api.Inherit{Inherit: "common"},
		},
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "stack-per-app-gke",
				Resources: RefappGkeAutopilotServerResources,
			},
			"prod": {
				Template:  "stack-per-app-gke",
				Resources: RefappGkeAutopilotServerResources,
			},
		},
	},
}

var ResolvedRefappGkeAutopilotServerResources = map[string]api.ResourceDescriptor{
	"gke-autopilot-res": {
		Type: "gcp-gke-autopilot-cluster",
		Config: api.Config{
			Config: &gcloud.GkeAutopilotResource{
				Credentials:   ResolvedCommonGcpCredentials,
				GkeMinVersion: "1.26.5-gke.1200",
			},
		},
		Inherit: api.Inherit{},
	},
	"artifact-registry-res": {
		Type: "gcp-artifact-registry",
		Config: api.Config{
			Config: &gcloud.ArtifactRegistryConfig{
				Credentials: ResolvedCommonGcpCredentials,
			},
		},
		Inherit: api.Inherit{},
	},
}

var ResolvedRefappGkeAutopilotServerDescriptor = &api.ServerDescriptor{
	SchemaVersion: api.ServerSchemaVersion,
	Provisioner:   ResolvedCommonServerDescriptor.Provisioner,
	Secrets:       ResolvedCommonServerDescriptor.Secrets,
	CiCd:          ResolvedCommonServerDescriptor.CiCd,
	Templates: map[string]api.StackDescriptor{
		"stack-per-app-gke": {
			Type: "gcp-gke-autopilot",
			Config: api.Config{
				Config: &gcloud.GkeAutopilotTemplate{
					Credentials:              ResolvedCommonGcpCredentials,
					ArtifactRegistryResource: "artifact-registry-res",
					GkeClusterResource:       "gke-autopilot-res",
				},
			},
			Inherit: api.Inherit{},
		},
	},
	Variables: map[string]api.VariableDescriptor{},
	Resources: api.PerStackResourcesDescriptor{
		Registrar: ResolvedCommonServerDescriptor.Resources.Registrar,
		Resources: map[string]api.PerEnvResourcesDescriptor{
			"staging": {
				Template:  "stack-per-app-gke",
				Resources: ResolvedRefappGkeAutopilotServerResources,
			},
			"prod": {
				Template:  "stack-per-app-gke",
				Resources: ResolvedRefappGkeAutopilotServerResources,
			},
		},
	},
}

var RefappGkeAutopilotClientDescriptor = &api.ClientDescriptor{
	SchemaVersion: api.ClientSchemaVersion,
	Stacks: map[string]api.StackClientDescriptor{
		"staging": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-gke-autopilot",
			Config: api.Config{
				Config: RefappClientComposeConfigStaging,
			},
		},
		"prod": {
			Type:        api.ClientTypeCloudCompose,
			ParentStack: "refapp-gke-autopilot",
			Config: api.Config{
				Config: RefappClientComposeConfigProd,
			},
		},
	},
}
