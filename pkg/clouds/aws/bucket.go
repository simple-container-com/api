package aws

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeS3Bucket = "s3-bucket"

type S3Bucket struct {
	AccountConfig         `json:",inline" yaml:",inline"`
	*api.StaticSiteConfig `json:",inline,omitempty" yaml:",inline,omitempty"`
	Name                  string `json:"name,omitempty" yaml:"name,omitempty"`
	AllowOnlyHttps        bool   `json:"allowOnlyHttps" yaml:"allowOnlyHttps"`
}

func S3BucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &S3Bucket{})
}
