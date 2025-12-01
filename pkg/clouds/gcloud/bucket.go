package gcloud

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeBucket = "gcp-bucket"

type GcpBucket struct {
	Credentials `json:",inline" yaml:",inline"`
	Name        string `json:"name,omitempty" yaml:"name"`
	Location    string `json:"location" yaml:"location"`
	// Resource adoption fields
	Adopt      bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
	BucketName string `json:"bucketName,omitempty" yaml:"bucketName,omitempty"`
}

// GetBucketName returns the bucket name, supporting both "name" and "bucketName" fields
// Falls back to "name" if "bucketName" is empty, or "bucketName" if "name" is empty
func (b *GcpBucket) GetBucketName() string {
	if b.BucketName != "" {
		return b.BucketName
	}
	return b.Name
}

func GcpBucketReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcpBucket{})
}
