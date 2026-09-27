// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"fmt"
	"net/url"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeYandexServiceAccount = "yc-service-account"
	SecretsTypeYandexLockbox     = "yc-lockbox"

	SecretsProviderTypeYandexKms        = "yc-kms"
	StateStorageTypeYandexObjectStorage = "yc-object-storage"
)

const (
	// DefaultRegion is the only generally-available Yandex Cloud region.
	DefaultRegion = "ru-central1"
	// DefaultZone is the default availability zone within DefaultRegion.
	DefaultZone = "ru-central1-a"
	// DefaultStorageEndpoint is the S3-compatible Object Storage endpoint.
	DefaultStorageEndpoint = "https://storage.yandexcloud.net"
)

// AccountConfig is the Yandex Cloud analogue of aws.AccountConfig: the credential
// blob a `${auth:<name>}` reference resolves to, plus the scoping ids every YC API
// call needs.
//
// Two credential shapes coexist deliberately, because YC's own APIs need both:
//
//   - ServiceAccountKey is the authorized-key JSON used for IAM-token exchange. It
//     authenticates the control plane — Serverless Containers, triggers, Lockbox,
//     Managed MongoDB.
//   - AccessKey/SecretAccessKey is a static key pair for the SigV4-compatible
//     services, Object Storage and Message Queue. They are NOT derivable from the
//     service-account key; YC issues them separately for the same service account.
//
// A config that only ever deploys containers can leave the static pair empty; one
// that stores Pulumi state in Object Storage cannot (see StateStorageConfig).
type AccountConfig struct {
	// CloudID is the YC cloud (the billing/organisational root).
	CloudID string `json:"cloudId" yaml:"cloudId"`
	// FolderID is the folder every resource is created in. It is the closest
	// analogue to an AWS account for SC's purposes and is what ProjectIdValue
	// returns, so `${auth:yc.projectId}` interpolates to it.
	FolderID string `json:"folderId" yaml:"folderId"`
	// ServiceAccountKey is the authorized-key JSON document, verbatim.
	ServiceAccountKey string `json:"serviceAccountKey,omitempty" yaml:"serviceAccountKey,omitempty"`
	// AccessKey / SecretAccessKey are the static key pair for the S3-compatible
	// services (Object Storage, Message Queue).
	AccessKey       string `json:"accessKey,omitempty" yaml:"accessKey,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty" yaml:"secretAccessKey,omitempty"`
	Region          string `json:"region,omitempty" yaml:"region,omitempty"`
	Zone            string `json:"zone,omitempty" yaml:"zone,omitempty"`

	api.Credentials `json:",inline" yaml:",inline"`
}

// EffectiveRegion resolves the configured region, falling back to DefaultRegion.
func (r *AccountConfig) EffectiveRegion() string {
	return lo.If(r.Region == "", DefaultRegion).Else(r.Region)
}

// EffectiveZone resolves the configured zone, falling back to DefaultZone.
func (r *AccountConfig) EffectiveZone() string {
	return lo.If(r.Zone == "", DefaultZone).Else(r.Zone)
}

func (r *AccountConfig) ProviderType() string {
	return ProviderType
}

// CredentialsValue serialises the whole account config when no opaque credentials
// blob was supplied, matching aws.AccountConfig: api.ConvertAuth round-trips this
// string back into an AccountConfig, so every field here must survive JSON.
func (r *AccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

func (r *AccountConfig) ProjectIdValue() string {
	return r.FolderID
}

// SecretsConfig is the runtime secret store (Lockbox) descriptor.
type SecretsConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

// StateStorageConfig points Pulumi's DIY state backend at a YC Object Storage
// bucket. No Yandex Pulumi provider is involved: Object Storage is S3-compatible
// and StorageUrl below is consumed by gocloud's s3blob opener.
type StateStorageConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
	BucketName    string `json:"bucketName" yaml:"bucketName"`
	// Endpoint overrides DefaultStorageEndpoint. Rarely needed.
	Endpoint  string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Provision bool   `json:"provision" yaml:"provision"`
}

func (s *StateStorageConfig) IsProvisionEnabled() bool {
	return s.Provision
}

// EffectiveEndpoint resolves the configured endpoint, falling back to
// DefaultStorageEndpoint.
func (s *StateStorageConfig) EffectiveEndpoint() string {
	return lo.If(s.Endpoint == "", DefaultStorageEndpoint).Else(s.Endpoint)
}

// StorageUrl builds the gocloud blob URL Pulumi's DIY backend opens.
//
// Pulumi hands any non-file:// URL to gocloud untouched (diy.massageBlobPath), and
// gocloud's s3blob opener reads `endpoint` and `region` (aws.V2ConfigFromURLParams)
// plus `s3ForcePathStyle` (s3blob.URLOpener.OpenBucketURL) off the query string. So
// an S3-compatible endpoint needs no new backend code at all — only this string.
//
// Path style is forced because YC Object Storage does not serve virtual-hosted
// bucket subdomains for every bucket name.
func (s *StateStorageConfig) StorageUrl() string {
	q := url.Values{}
	q.Set("endpoint", s.EffectiveEndpoint())
	q.Set("region", s.EffectiveRegion())
	q.Set("s3ForcePathStyle", "true")
	return fmt.Sprintf("s3://%s?%s", s.BucketName, q.Encode())
}

// SecretsProviderConfig selects the KMS key Pulumi encrypts stack secrets with.
type SecretsProviderConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
	Provision     bool `json:"provision" yaml:"provision"`

	// KeyName must be a gocloud secrets URL. YC KMS speaks the AWS KMS wire
	// protocol on its own endpoint, so the form is:
	// awskms://<key-id>?endpoint=https://kms.yandex/&region=ru-central1
	KeyName string `json:"keyName" yaml:"keyName"`
}

func (s *SecretsProviderConfig) IsProvisionEnabled() bool {
	return s.Provision
}

func (s *SecretsProviderConfig) KeyUrl() string {
	return s.KeyName
}

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AccountConfig{})
}

func ReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsConfig{})
}

func ReadStateStorageConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &StateStorageConfig{})
}

func ReadSecretsProviderConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsProviderConfig{})
}
