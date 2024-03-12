package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeBucket = "gcp-bucket"

type GcpBucket struct {
	Credentials `json:",inline" yaml:",inline"`
	Name        string `json:"name,omitempty" yaml:"name"`
	Location    string `json:"location" yaml:"location"`
}

func GcpBucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcpBucket{})
}
