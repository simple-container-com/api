package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Credentials           `json:",inline" yaml:",inline"`
	Version               string                  `json:"version" yaml:"version"`
	Project               string                  `json:"project" yaml:"project"`
	Tier                  *string                 `json:"tier" yaml:"tier"`
	Region                *string                 `json:"region" yaml:"region"`
	MaxConnections        *int                    `json:"maxConnections" yaml:"maxConnections"`
	DeletionProtection    *bool                   `json:"deletionProtection" yaml:"deletionProtection"`
	QueryInsightsEnabled  *bool                   `json:"queryInsightsEnabled" yaml:"queryInsightsEnabled"`
	QueryStringLength     *int                    `json:"queryStringLength" yaml:"queryStringLength"`
	UsersProvisionRuntime *ProvisionRuntimeConfig `json:"usersProvisionRuntime" yaml:"usersProvisionRuntime"`
	// Resource adoption fields
	Adopt          bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	InstanceName   string `json:"instanceName,omitempty" yaml:"instanceName,omitempty"`
	ConnectionName string `json:"connectionName,omitempty" yaml:"connectionName,omitempty"`
	RootPassword   string `json:"rootPassword,omitempty" yaml:"rootPassword,omitempty"`
}

type ProvisionRuntimeConfig struct {
	Type         string `json:"type" yaml:"type"`                 // type of provisioning runtime
	ResourceName string `json:"resourceName" yaml:"resourceName"` // allows to run init db users jobs on kube jobs (must reference resource name where we can obtain kubeconfig from, e.g. gke-autopilot-cluster)
}

func PostgresqlGcpCloudsqlReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresGcpCloudsqlConfig{})
}
