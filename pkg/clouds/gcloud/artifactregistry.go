// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import "github.com/simple-container-com/api/pkg/api"

const (
	ResourceTypeArtifactRegistry      = "gcp-artifact-registry"
	ResourceTypeRemoteDockerImagePush = "gcp-docker-image-push"
)

type ArtifactRegistryConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	Location    string             `json:"location" yaml:"location"`
	Public      *bool              `json:"public,omitempty" yaml:"public,omitempty"`
	Docker      *DockerConfig      `json:"docker,omitempty" yaml:"docker,omitempty"`
	Domain      *string            `json:"domain" yaml:"domain"`
	BasicAuth   *RegistryBasicAuth `json:"basicAuth,omitempty" yaml:"basicAuth,omitempty"`

	// CleanupPolicies declares image retention for the repository.
	//
	// The provider treats cleanupPolicies as authoritative (unlike labels, whose
	// schema documents itself as non-authoritative), so a repository resource
	// that does not declare it will CLEAR any policy set by another tool on the
	// next provision. Leaving this empty therefore means "SC does not manage
	// retention", and SC preserves whatever is configured out of band rather
	// than deleting it. See ManagesCleanupPolicies.
	CleanupPolicies []ArtifactRegistryCleanupPolicy `json:"cleanupPolicies,omitempty" yaml:"cleanupPolicies,omitempty"`

	// CleanupPolicyDryRun evaluates the policies and reports what they would
	// delete without deleting it. Only meaningful alongside CleanupPolicies.
	CleanupPolicyDryRun *bool `json:"cleanupPolicyDryRun,omitempty" yaml:"cleanupPolicyDryRun,omitempty"`
}

// ManagesCleanupPolicies reports whether retention is declared here. When it is
// not, the caller must tell Pulumi to ignore the field rather than send an empty
// value, which is the difference between "not managed" and "delete the policy".
func (c *ArtifactRegistryConfig) ManagesCleanupPolicies() bool {
	return len(c.CleanupPolicies) > 0
}

// ArtifactRegistryCleanupPolicy mirrors a single Artifact Registry cleanup
// policy. Durations are the API's second-suffixed form, e.g. "2592000s".
type ArtifactRegistryCleanupPolicy struct {
	// Name identifies the policy within the repository.
	Name string `json:"name" yaml:"name"`
	// Action is DELETE or KEEP (case-insensitive).
	Action    string                                  `json:"action" yaml:"action"`
	Condition *ArtifactRegistryCleanupPolicyCondition `json:"condition,omitempty" yaml:"condition,omitempty"`
	// MostRecentVersions retains a minimum number of versions. KEEP only.
	MostRecentVersions *ArtifactRegistryCleanupMostRecentVersions `json:"mostRecentVersions,omitempty" yaml:"mostRecentVersions,omitempty"`
}

type ArtifactRegistryCleanupPolicyCondition struct {
	// TagState is TAGGED, UNTAGGED or ANY.
	TagState string `json:"tagState,omitempty" yaml:"tagState,omitempty"`
	// OlderThan / NewerThan are durations, e.g. "2592000s" for 30 days.
	OlderThan           string   `json:"olderThan,omitempty" yaml:"olderThan,omitempty"`
	NewerThan           string   `json:"newerThan,omitempty" yaml:"newerThan,omitempty"`
	TagPrefixes         []string `json:"tagPrefixes,omitempty" yaml:"tagPrefixes,omitempty"`
	PackageNamePrefixes []string `json:"packageNamePrefixes,omitempty" yaml:"packageNamePrefixes,omitempty"`
	VersionNamePrefixes []string `json:"versionNamePrefixes,omitempty" yaml:"versionNamePrefixes,omitempty"`
}

type ArtifactRegistryCleanupMostRecentVersions struct {
	KeepCount           *int     `json:"keepCount,omitempty" yaml:"keepCount,omitempty"`
	PackageNamePrefixes []string `json:"packageNamePrefixes,omitempty" yaml:"packageNamePrefixes,omitempty"`
}

type RegistryBasicAuth struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

type DockerConfig struct {
	ImmutableTags *bool `json:"immutableTags" yaml:"immutableTags"`
}

type RemoteImagePush struct {
	Credentials              `json:",inline" yaml:",inline"`
	RemoteImage              string            `json:"remoteImage" yaml:"remoteImage"`
	Name                     string            `json:"name" yaml:"name"`
	Tag                      string            `json:"tag" yaml:"tag"`
	ArtifactRegistryResource string            `json:"artifactRegistryResource" yaml:"artifactRegistryResource"`
	RegistryCredentials      *string           `json:"registryCredentials" yaml:"registryCredentials"` // TODO: support other registries' creds
	Platform                 api.ImagePlatform `json:"platform" yaml:"platform"`
}

func (i *RemoteImagePush) DependsOnResources() []api.ParentResourceDependency {
	return []api.ParentResourceDependency{
		{Name: i.ArtifactRegistryResource},
	}
}

func ArtifactRegistryConfigReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ArtifactRegistryConfig{})
}

func DockerRemoteImagePushReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &RemoteImagePush{})
}
