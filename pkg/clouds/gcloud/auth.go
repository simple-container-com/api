package gcloud

import (
	"encoding/json"
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"

	StateStorageTypeGcpBucket = "gcp-bucket"
	SecretsProviderTypeGcpKms = "gcp-kms"
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
