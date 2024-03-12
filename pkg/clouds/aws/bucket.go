package aws

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeS3Bucket = "s3-bucket"

type S3Bucket struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Name             string `json:"name,omitempty" yaml:"name"`
	Location         string `json:"location" yaml:"location"`
}

func S3BucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &S3Bucket{})
}
