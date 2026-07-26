// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const ResourceTypePostgresGcpCloudsql = "gcp-cloudsql-postgres"

type PostgresGcpCloudsqlConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	Version     string  `json:"version" yaml:"version"`
	Project     string  `json:"project" yaml:"project"`
	Tier        *string `json:"tier" yaml:"tier"`
	Region      *string `json:"region" yaml:"region"`
	// Deprecated: prefer DatabaseFlags["max_connections"]; kept for
	// backward compatibility.
	MaxConnections *int `json:"maxConnections" yaml:"maxConnections"`
	// DatabaseFlags sets arbitrary Cloud SQL database flags by name
	// (e.g. cloudsql.iam_authentication: "on"). An explicit max_connections
	// entry here takes precedence over MaxConnections.
	DatabaseFlags         map[string]string       `json:"databaseFlags,omitempty" yaml:"databaseFlags,omitempty"`
	DeletionProtection    *bool                   `json:"deletionProtection" yaml:"deletionProtection"`
	QueryInsightsEnabled  *bool                   `json:"queryInsightsEnabled" yaml:"queryInsightsEnabled"`
	QueryStringLength     *int                    `json:"queryStringLength" yaml:"queryStringLength"`
	UsersProvisionRuntime *ProvisionRuntimeConfig `json:"usersProvisionRuntime" yaml:"usersProvisionRuntime"`
	// Backup configuration
	BackupEnabled               *bool   `json:"backupEnabled,omitempty" yaml:"backupEnabled,omitempty"`
	BackupStartTime             *string `json:"backupStartTime,omitempty" yaml:"backupStartTime,omitempty"`
	PointInTimeRecoveryEnabled  *bool   `json:"pointInTimeRecoveryEnabled,omitempty" yaml:"pointInTimeRecoveryEnabled,omitempty"`
	TransactionLogRetentionDays *int    `json:"transactionLogRetentionDays,omitempty" yaml:"transactionLogRetentionDays,omitempty"`
	RetainedBackups             *int    `json:"retainedBackups,omitempty" yaml:"retainedBackups,omitempty"`
	// High availability
	AvailabilityType *string `json:"availabilityType,omitempty" yaml:"availabilityType,omitempty"` // ZONAL or REGIONAL
	// SSL
	RequireSsl *bool `json:"requireSsl,omitempty" yaml:"requireSsl,omitempty"`
	// PrivateNetwork: when set to a VPC network resource path
	// (projects/{project}/global/networks/{vpc}) the instance is given a
	// private IP on that network. Requires Private Services Access (a
	// servicenetworking peering range) to already exist on the VPC. Adding it
	// while the public IP stays on is a no-cutover step: the instance gains a
	// private IP but the in-cluster proxy keeps using the public endpoint until
	// publicIpEnabled is set false (see UsesPrivateIpProxy).
	PrivateNetwork *string `json:"privateNetwork,omitempty" yaml:"privateNetwork,omitempty"`
	// PublicIpEnabled toggles the instance's public IPv4 address (default true
	// to preserve existing authorized networks). Setting it false requires
	// privateNetwork and switches the in-cluster cloud-sql-proxy to dial the
	// private IP (--private-ip). Do this only once the private path is verified
	// reachable, since the proxy has no public fallback.
	PublicIpEnabled *bool `json:"publicIpEnabled,omitempty" yaml:"publicIpEnabled,omitempty"`
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

// HasPrivateNetwork reports whether a non-empty private VPC network is configured.
func (c *PostgresGcpCloudsqlConfig) HasPrivateNetwork() bool {
	return c.PrivateNetwork != nil && *c.PrivateNetwork != ""
}

// UsesPrivateIpProxy reports whether the in-cluster cloud-sql-proxy should dial
// the instance's private IP. The proxy switches to private only once the public
// IP is disabled, so adding privateNetwork while the public IP stays on does not
// touch the proxy.
func (c *PostgresGcpCloudsqlConfig) UsesPrivateIpProxy() bool {
	return c.PublicIpEnabled != nil && !*c.PublicIpEnabled
}

// Validate checks Cloud SQL config invariants that would otherwise surface as
// opaque failures at the GCP API.
func (c *PostgresGcpCloudsqlConfig) Validate() error {
	if c.AvailabilityType != nil && *c.AvailabilityType != "ZONAL" && *c.AvailabilityType != "REGIONAL" {
		return errors.Errorf("availabilityType must be ZONAL or REGIONAL, got %q", *c.AvailabilityType)
	}
	if c.PrivateNetwork != nil && *c.PrivateNetwork != "" && !strings.HasPrefix(*c.PrivateNetwork, "projects/") {
		return errors.Errorf("privateNetwork must be a full VPC network path like 'projects/{project}/global/networks/{vpc}', got %q", *c.PrivateNetwork)
	}
	// Disabling the public IP without a private network would leave the instance
	// unreachable by the cloud-sql-proxy.
	if c.PublicIpEnabled != nil && !*c.PublicIpEnabled && !c.HasPrivateNetwork() {
		return errors.New("publicIpEnabled: false requires privateNetwork to be set")
	}
	return nil
}

func PostgresqlGcpCloudsqlReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PostgresGcpCloudsqlConfig{})
}
