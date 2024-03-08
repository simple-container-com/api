package aws

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeS3Bucket = "s3-bucket"

type S3Bucket struct {
	api.AuthConfig
	Name        string `json:"name,omitempty" yaml:"name"`
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
	Location    string `json:"location" yaml:"location"`
}

func S3BucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &S3Bucket{})
}

func (r *S3Bucket) CredentialsValue() string {
	return r.Credentials
}

func (r *S3Bucket) ProjectIdValue() string {
	return r.ProjectId
}
