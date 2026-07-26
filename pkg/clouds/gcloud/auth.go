// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"

	StateStorageTypeGcpBucket = "gcp-bucket"
	SecretsProviderTypeGcpKms = "gcp-kms"
)

const (
	// DefaultKeyRotationPeriod is applied when keyRotationPeriod is unset.
	//
	// Cloud KMS bills every ACTIVE key version (ENABLED, DISABLED and
	// DESTROY_SCHEDULED all count; only DESTROYED is free) and rotation never
	// re-encrypts existing ciphertext, so every version a key mints stays
	// load-bearing and billed for the lifetime of the key. That makes the
	// rotation period a direct, compounding cost multiplier: one provisioned
	// key per stack rotating daily adds a billed version per stack per day,
	// forever.
	DefaultKeyRotationPeriod = "7776000s" // 90 days

	// MinKeyRotationPeriodSeconds is the lower bound accepted for an explicit
	// keyRotationPeriod. GCP's own floor is 86400s (1 day), which is far too
	// low to be a sane default for a per-stack provisioned key: a period of a
	// few hours or days passes GCP validation and silently accrues versions.
	// Rejecting anything under 30 days turns that class of typo into a
	// config-parse error instead of an unattributed bill months later.
	MinKeyRotationPeriodSeconds = 2592000 // 30 days
)

type ServiceAccountConfig struct {
	ProjectId string `json:"projectId" yaml:"projectId"`
}

type Credentials struct {
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
}

type CredentialsParsed struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
}

type StateStorageConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	BucketName  string  `json:"bucketName" yaml:"bucketName"`
	Name        string  `json:"name,omitempty" yaml:"name,omitempty"`
	Location    *string `json:"location" yaml:"location"`
	Provision   bool    `json:"provision" yaml:"provision"`
}

// GetBucketName returns the bucket name, supporting both "name" and "bucketName" fields
// Falls back to "name" if "bucketName" is empty, or "bucketName" if "name" is empty
func (s *StateStorageConfig) GetBucketName() string {
	if s.BucketName != "" {
		return s.BucketName
	}
	return s.Name
}

type SecretsProviderConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	// format:
	// "gcpkms://projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s"
	KeyName string `json:"keyName" yaml:"keyName"`

	// only applicable when provision=true
	KeyLocation string `json:"keyLocation" yaml:"keyLocation"`
	// only applicable when provision=true
	KeyRotationPeriod string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`

	// whether to provision key
	Provision bool `json:"provision" yaml:"provision"`
}

func (sa *StateStorageConfig) StorageUrl() string {
	return fmt.Sprintf("gs://%s", sa.GetBucketName())
}

func (sa *StateStorageConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (r *SecretsProviderConfig) IsProvisionEnabled() bool {
	return r.Provision
}

func (r *SecretsProviderConfig) KeyUrl() string {
	return r.KeyName
}

// EffectiveKeyRotationPeriod returns the configured rotation period, or
// DefaultKeyRotationPeriod when unset.
func (r *SecretsProviderConfig) EffectiveKeyRotationPeriod() string {
	if r.KeyRotationPeriod == "" {
		return DefaultKeyRotationPeriod
	}
	return r.KeyRotationPeriod
}

// ValidateKeyRotationPeriod checks an explicitly configured rotation period.
// Only applicable when provision=true; an empty value is valid and means the
// default applies.
func (r *SecretsProviderConfig) ValidateKeyRotationPeriod() error {
	if r.KeyRotationPeriod == "" {
		return nil
	}
	raw := r.KeyRotationPeriod
	if !strings.HasSuffix(raw, "s") {
		return errors.Errorf("keyRotationPeriod %q must be a duration in seconds with an 's' suffix, e.g. %q", raw, DefaultKeyRotationPeriod)
	}
	secs, err := strconv.Atoi(strings.TrimSuffix(raw, "s"))
	if err != nil {
		return errors.Errorf("keyRotationPeriod %q must be a whole number of seconds with an 's' suffix, e.g. %q", raw, DefaultKeyRotationPeriod)
	}
	if secs < MinKeyRotationPeriodSeconds {
		return errors.Errorf("keyRotationPeriod %q is %d seconds, below the minimum of %d (30 days): every rotation mints a key version that Cloud KMS bills for the lifetime of the key, so short periods accrue cost indefinitely",
			raw, secs, MinKeyRotationPeriodSeconds)
	}
	return nil
}

func (r *Credentials) ProviderType() string {
	return ProviderType
}

func (r *Credentials) ProjectIdValue() string {
	return r.ProjectId
}

func (r *Credentials) CredentialsValue() string {
	return r.Credentials.Credentials // just return serialized gcp account json
}

func (r *Credentials) CredentialsParsed() (*CredentialsParsed, error) {
	var key CredentialsParsed
	if err := json.Unmarshal([]byte(r.CredentialsValue()), &key); err != nil {
		return nil, err
	}
	return &key, nil
}

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &Credentials{})
}

func ReadStateStorageConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &StateStorageConfig{})
}

func ReadSecretsProviderConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsProviderConfig{})
}
