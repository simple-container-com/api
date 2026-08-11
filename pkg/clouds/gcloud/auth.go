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

	// GCP's own documented bounds for rotationPeriod: at least 24h, at most
	// 876,000h. These are NOT waivable by AllowShortKeyRotation — a value
	// outside them is rejected by the KMS API, and by then the KeyRing has
	// already been created and can never be deleted. Failing here keeps that
	// class of error away from any side effect.
	GcpMinKeyRotationPeriodSeconds = 86400      // 24h
	GcpMaxKeyRotationPeriodSeconds = 3153600000 // 876,000h
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

	// NoncurrentVersionRetentionDays bounds how long overwritten state
	// generations are kept when the bucket has object versioning enabled.
	//
	// It matters beyond storage cost. Pulumi's cloud secrets manager re-wraps
	// each stack's data key on every state write, and a KMS symmetric encrypt
	// always uses the key's current primary version, so each retained
	// generation depends on whichever key version was primary when it was
	// written. With versioning on and no lifecycle rule, that set of
	// load-bearing key versions grows without limit and no key version can
	// ever be proven unreferenced.
	//
	// Zero disables the rule. Nil means DefaultNoncurrentVersionRetentionDays.
	NoncurrentVersionRetentionDays *int `json:"noncurrentVersionRetentionDays,omitempty" yaml:"noncurrentVersionRetentionDays,omitempty"`
}

// DefaultNoncurrentVersionRetentionDays is the rollback horizon applied when
// noncurrentVersionRetentionDays is unset. Long enough to recover from a bad
// deployment, short enough that the dependency set stays bounded.
const DefaultNoncurrentVersionRetentionDays = 30

// EffectiveNoncurrentVersionRetentionDays resolves the configured retention,
// falling back to the default when unset. A configured zero disables the rule.
func (s *StateStorageConfig) EffectiveNoncurrentVersionRetentionDays() int {
	if s.NoncurrentVersionRetentionDays == nil {
		return DefaultNoncurrentVersionRetentionDays
	}
	return *s.NoncurrentVersionRetentionDays
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
	KeyRotationPeriod string `json:"keyRotationPeriod,omitempty" yaml:"keyRotationPeriod,omitempty"`
	// AllowShortKeyRotation opts out of the MinKeyRotationPeriodSeconds floor.
	// Rotating faster than 30 days is a legitimate compliance choice; it is
	// gated only because every rotation mints a permanently billed key version,
	// so the common case of a mistyped period should fail loudly. Setting this
	// makes the short period a deliberate, reviewable decision.
	AllowShortKeyRotation bool `json:"allowShortKeyRotation,omitempty" yaml:"allowShortKeyRotation,omitempty"`

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

// Validate checks the secrets-provider config. Follows the same shape as the
// other GCP configs in this package (PostgresGcpCloudsqlConfig.Validate,
// ExternalEgressIpConfig.Validate) so there is one convention to learn.
//
// keyRotationPeriod only applies when the key is provisioned here; a BYO key
// referenced by keyName ignores it, so a stale value must not block those
// consumers. An empty value is valid and means DefaultKeyRotationPeriod.
func (r *SecretsProviderConfig) Validate() error {
	if !r.Provision || r.KeyRotationPeriod == "" {
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
	// GCP's own bounds first, and never waivable: outside them the KMS API
	// rejects the key AFTER the KeyRing exists.
	if secs < GcpMinKeyRotationPeriodSeconds {
		return errors.Errorf("keyRotationPeriod %q is %d seconds; GCP requires at least %d (24h)",
			raw, secs, GcpMinKeyRotationPeriodSeconds)
	}
	if secs > GcpMaxKeyRotationPeriodSeconds {
		return errors.Errorf("keyRotationPeriod %q is %d seconds; GCP allows at most %d (876,000h)",
			raw, secs, GcpMaxKeyRotationPeriodSeconds)
	}
	if secs < MinKeyRotationPeriodSeconds && !r.AllowShortKeyRotation {
		return errors.Errorf("keyRotationPeriod %q (%ds) is below the %ds (30 day) minimum; set allowShortKeyRotation: true to override",
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
	out, err := api.ConvertConfig(config, &SecretsProviderConfig{})
	if err != nil {
		return out, err
	}
	// Validate here rather than only in the provisioner: the secrets-provider
	// stack is Up'd only when its URL export is absent, so a provisioner-only
	// check never runs for an already-provisioned stack and a bad value would
	// sit unnoticed until a DR rebuild.
	if sp, ok := out.Config.(*SecretsProviderConfig); ok {
		if err := sp.Validate(); err != nil {
			return out, err
		}
	}
	return out, nil
}
