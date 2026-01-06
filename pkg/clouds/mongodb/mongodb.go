package mongodb

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeMongodbAtlas = "mongodb-atlas"

type AtlasConfig struct {
	Admins         []string                      `json:"admins" yaml:"admins"`
	Developers     []string                      `json:"developers" yaml:"developers"`
	InstanceSize   string                        `json:"instanceSize" yaml:"instanceSize"`
	OrgId          string                        `json:"orgId" yaml:"orgId"`
	ProjectId      string                        `json:"projectId" yaml:"projectId"`
	ProjectName    string                        `json:"projectName" yaml:"projectName"`
	Region         string                        `json:"region" yaml:"region"`
	PrivateKey     string                        `json:"privateKey" yaml:"privateKey"`
	PublicKey      string                        `json:"publicKey" yaml:"publicKey"`
	CloudProvider  string                        `json:"cloudProvider" yaml:"cloudProvider"`
	Backup         *AtlasBackup                  `json:"backup,omitempty" yaml:"backup,omitempty"`
	NetworkConfig  *AtlasNetworkConfig           `json:"networkConfig,omitempty" yaml:"networkConfig,omitempty"`
	ExtraProviders map[string]api.AuthDescriptor `json:"extraProviders,omitempty" yaml:"extraProviders,omitempty"`
	DiskSizeGB     *float64                      `json:"diskSizeGB,omitempty" yaml:"diskSizeGB,omitempty"`
	NumShards      *int                          `json:"numShards,omitempty" yaml:"numShards,omitempty"`
	// Resource adoption fields
	Adopt       bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"`
	// Deletion protection
	DeletionProtection bool `json:"deletionProtection,omitempty" yaml:"deletionProtection,omitempty"`
	// Naming strategy version control
	NamingStrategyVersion *int `json:"namingStrategyVersion,omitempty" yaml:"namingStrategyVersion,omitempty"` // Default 2 (new), use 1 for legacy
}

type AtlasNetworkConfig struct {
	PrivateLinkEndpoint *PrivateLinkEndpoint `json:"privateLinkEndpoint,omitempty" yaml:"privateLinkEndpoint,omitempty"`
	AllowAllIps         *bool                `json:"allowAllIps,omitempty" yaml:"allowAllIps,omitempty"`
	AllowCidrs          *[]string            `json:"allowCidrs,omitempty" yaml:"allowCidrs,omitempty"` // format "0.0.0.0/0"
}

type PrivateLinkEndpoint struct {
	ProviderName string `json:"providerName" yaml:"providerName"`
}

type AtlasBackup struct {
	// Basic configuration (backwards compatible)
	Every     string `json:"every,omitempty" yaml:"every,omitempty"`         // e.g. 2h
	Retention string `json:"retention,omitempty" yaml:"retention,omitempty"` // e.g. 24h

	// Advanced multi-tier backup configuration
	Advanced *AtlasAdvancedBackup `json:"advanced,omitempty" yaml:"advanced,omitempty"`
}

// AtlasAdvancedBackup provides sophisticated backup scheduling with multiple retention tiers
type AtlasAdvancedBackup struct {
	// Individual schedule policies (can be combined)
	Hourly  *AtlasBackupPolicy `json:"hourly,omitempty" yaml:"hourly,omitempty"`
	Daily   *AtlasBackupPolicy `json:"daily,omitempty" yaml:"daily,omitempty"`
	Weekly  *AtlasBackupPolicy `json:"weekly,omitempty" yaml:"weekly,omitempty"`
	Monthly *AtlasBackupPolicy `json:"monthly,omitempty" yaml:"monthly,omitempty"`

	// Point-in-Time Recovery configuration
	PointInTimeRecovery *AtlasPointInTimeRecovery `json:"pointInTimeRecovery,omitempty" yaml:"pointInTimeRecovery,omitempty"`

	// Export configuration for cross-region/project backups
	Export *AtlasBackupExport `json:"export,omitempty" yaml:"export,omitempty"`
}

// AtlasBackupPolicy defines a backup schedule with retention
type AtlasBackupPolicy struct {
	// Schedule frequency
	Every int `json:"every" yaml:"every"` // e.g. 1 for every 1 hour/day/week/month

	// Retention duration (unit inferred from backup type context)
	RetainFor int    `json:"retainFor" yaml:"retainFor"`           // e.g. 2 for retain for 2 days/weeks/months
	Unit      string `json:"unit,omitempty" yaml:"unit,omitempty"` // "days", "weeks", "months" (optional, defaults based on backup type)

	// Weekly-specific configuration (not supported in current provider)
	DayOfWeek *int `json:"dayOfWeek,omitempty" yaml:"dayOfWeek,omitempty"` // 1=Sunday, 7=Saturday

	// Monthly-specific configuration (not supported in current provider)
	DayOfMonth *int `json:"dayOfMonth,omitempty" yaml:"dayOfMonth,omitempty"` // 1-31
}

// AtlasPointInTimeRecovery configures continuous oplog streaming
type AtlasPointInTimeRecovery struct {
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Oplog retention window
	OplogSizeGB            *float64 `json:"oplogSizeGB,omitempty" yaml:"oplogSizeGB,omitempty"`                       // GB of oplog to retain
	OplogMinRetentionHours *int     `json:"oplogMinRetentionHours,omitempty" yaml:"oplogMinRetentionHours,omitempty"` // Minimum oplog retention in hours
}

// AtlasBackupExport configures cross-region or cross-project backup exports
type AtlasBackupExport struct {
	// Export frequency (typically less frequent than main backups)
	FrequencyType string `json:"frequencyType" yaml:"frequencyType"` // "daily", "weekly", "monthly"

	// Export destinations
	ExportBucketId  string  `json:"exportBucketId" yaml:"exportBucketId"`                       // Atlas-managed cloud storage bucket
	ExportBucketUrl *string `json:"exportBucketUrl,omitempty" yaml:"exportBucketUrl,omitempty"` // Custom S3/GCS bucket URL
}

func ReadAtlasConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AtlasConfig{})
}

func (r *AtlasConfig) CredentialsValue() string {
	return r.PrivateKey
}

func (r *AtlasConfig) ProjectIdValue() string {
	return r.ProjectId
}

func (r *AtlasConfig) DependencyProviders() map[string]api.AuthDescriptor {
	return r.ExtraProviders
}

func (r *AtlasConfig) ProviderType() string {
	return ProviderType
}
