// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// AuthTypeCloudruAccessKey is the `type:` of an `auth:` entry holding a Cloud.ru
// Advanced key pair.
//
// Named for the credential rather than for the cloud ("cloudru" alone) because
// Cloud.ru Advanced accepts two different ones — this access/secret pair, and a
// Keystone user name + password — and a future second auth type would otherwise
// have no room for a name.
const AuthTypeCloudruAccessKey = "cloudru-access-key"

const (
	// DefaultRegion is Cloud.ru Advanced's primary region, and the one the upstream
	// Terraform provider is tested against.
	DefaultRegion = "ru-moscow-1"
	// authURLPattern builds the Keystone v3 endpoint for a region.
	//
	// The hostname is the legacy ex-SberCloud one, deliberately: upstream's own
	// default is literally the ru-moscow-1 instance of this pattern, so following it
	// is what makes a config with nothing but a key pair work. If Cloud.ru moves the
	// IAM endpoint, the answer is `authUrl` below — not a patched fork.
	authURLPattern = "https://iam.%s.hc.sbercloud.ru/v3"
)

// AccountConfig is the credential blob a `${auth:<name>}` reference resolves to.
//
// Cloud.ru Advanced is OpenStack-descended (by way of Huawei Cloud), so the scoping
// vocabulary is Keystone's, not AWS's: a *project* is the tenancy resources are
// created in and is usually named after the region, and a *domain* is the account
// above it. Both are normally inferable from the key pair, which is why only the
// pair itself is required here.
type AccountConfig struct {
	// AccessKey / SecretKey are the permanent key pair issued in the Cloud.ru
	// console ("My Credentials" → Access Keys). SecretKey is a credential: write it
	// as `${secret:<name>}`.
	AccessKey string `json:"accessKey" yaml:"accessKey"`
	SecretKey string `json:"secretKey" yaml:"secretKey"`
	// SecurityToken accompanies a TEMPORARY key pair. Also a credential.
	SecurityToken string `json:"securityToken,omitempty" yaml:"securityToken,omitempty"`

	Region string `json:"region,omitempty" yaml:"region,omitempty"`
	// ProjectName is the Keystone project (tenant). Defaults to the region, which is
	// how Cloud.ru names the project it creates with an account.
	//
	// There is deliberately no `projectId` counterpart: the upstream provider has no
	// such argument (its scoping inputs are `project_name`, `domain_name`,
	// `domain_id`), so a field here would accept an id and silently drop it.
	ProjectName string `json:"projectName,omitempty" yaml:"projectName,omitempty"`
	// DomainName / DomainID scope the account. Normally derived from the key pair.
	DomainName string `json:"domainName,omitempty" yaml:"domainName,omitempty"`
	DomainID   string `json:"domainId,omitempty" yaml:"domainId,omitempty"`
	// EnterpriseProjectID tags resources into an enterprise project, where the
	// account uses them for cost allocation. Empty means the default one.
	EnterpriseProjectID string `json:"enterpriseProjectId,omitempty" yaml:"enterpriseProjectId,omitempty"`
	// AuthURL overrides the Keystone v3 endpoint derived from the region.
	AuthURL string `json:"authUrl,omitempty" yaml:"authUrl,omitempty"`

	api.Credentials `json:",inline" yaml:",inline"`
}

// WithRegion returns a copy scoped to the given region, and is how a resource's own
// `region:` wins over the credential's default.
//
// A copy rather than a mutation, and the whole reason this exists: region is not one
// input among many on this cloud. It also picks the Keystone endpoint
// (EffectiveAuthURL) and the project the resources are created in
// (EffectiveProjectName), because Cloud.ru names an account's default project after
// its region. Setting the provider's region alone — and leaving a project and an IAM
// endpoint derived from a different one — authenticates against one region's tenancy
// and then creates in another. An explicit `projectName` or `authUrl` in the auth
// entry still wins: both are already non-empty here, so the fallbacks never fire.
func (r *AccountConfig) WithRegion(region string) *AccountConfig {
	out := *r
	if region != "" {
		out.Region = region
	}
	return &out
}

// EffectiveRegion resolves the configured region, falling back to DefaultRegion.
func (r *AccountConfig) EffectiveRegion() string {
	return lo.If(r.Region == "", DefaultRegion).Else(r.Region)
}

// EffectiveProjectName resolves the Keystone project, falling back to the region.
func (r *AccountConfig) EffectiveProjectName() string {
	return lo.If(r.ProjectName == "", r.EffectiveRegion()).Else(r.ProjectName)
}

// EffectiveAuthURL resolves the Keystone v3 endpoint, deriving it from the region
// when it was not given.
func (r *AccountConfig) EffectiveAuthURL() string {
	return lo.If(r.AuthURL == "", fmt.Sprintf(authURLPattern, r.EffectiveRegion())).Else(r.AuthURL)
}

func (r *AccountConfig) ProviderType() string {
	return ProviderType
}

// CredentialsValue serialises the whole account config when no opaque credentials
// blob was supplied, matching yandex.AccountConfig: api.ConvertAuth round-trips this
// string back into an AccountConfig, so every field here must survive JSON.
func (r *AccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

// ProjectIdValue is what `${auth:<name>.projectId}` interpolates to. The Keystone
// project name, not the id: it is what the provider is configured with and what an
// operator sees in the console, and the id is optional here.
func (r *AccountConfig) ProjectIdValue() string {
	return r.EffectiveProjectName()
}

// Validate refuses a config that cannot authenticate, at provision time and naming
// the field, rather than letting the provider fail its first API call with a
// Keystone 401 that says nothing about which half is missing.
func (r *AccountConfig) Validate() error {
	if r.AccessKey == "" {
		return errors.Errorf("`accessKey` is required for auth type %q", AuthTypeCloudruAccessKey)
	}
	if r.SecretKey == "" {
		return errors.Errorf("`secretKey` is required for auth type %q", AuthTypeCloudruAccessKey)
	}
	// Same trap as Evolution, and it belongs to `secrets.yaml` rather than to either
	// cloud — see unexpandedPlaceholder.
	return unexpandedPlaceholder(AuthTypeCloudruAccessKey, map[string]string{
		"accessKey":   r.AccessKey,
		"secretKey":   r.SecretKey,
		"projectName": r.ProjectName,
		"region":      r.Region,
	})
}

func ReadAuthAccessKeyConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AccountConfig{})
}
