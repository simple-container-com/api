package gcloud

import "api/pkg/api"

const ResourceTypeBucket = "gcp-bucket"

type GcpBucket struct {
	Name        string `json:"name,omitempty" yaml:"name"`
	Project     string `json:"project" yaml:"project" json:"project,omitempty"`
	Credentials string `json:"credentials" yaml:"credentials" json:"credentials,omitempty"`
	ProjectId   string `json:"projectId" yaml:"projectId" json:"projectId,omitempty"`
}

func GcpBucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcpBucket{})
}
