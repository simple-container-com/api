package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeBucket = "gcp-bucket"

type GcpBucket struct {
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	api.Credentials      `json:",inline" yaml:",inline"`
	Name                 string `json:"name,omitempty" yaml:"name"`
	Location             string `json:"location" yaml:"location"`
}

func GcpBucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcpBucket{})
}

func (r *GcpBucket) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *GcpBucket) ProjectIdValue() string {
	return r.ProjectId
}
