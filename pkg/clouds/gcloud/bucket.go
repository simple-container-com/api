package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeBucket = "gcp-bucket"

type GcpBucket struct {
	api.AuthConfig
	Name        string `json:"name,omitempty" yaml:"name"`
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
	Location    string `json:"location" yaml:"location"`
}

func GcpBucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcpBucket{})
}

func (r *GcpBucket) CredentialsValue() string {
	return r.Credentials
}

func (r *GcpBucket) ProjectIdValue() string {
	return r.ProjectId
}
