package pulumi

import (
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/fs"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	StateStorageTypeGcpBucket     = gcloud.StateStorageTypeGcpBucket
	StateStorageTypeS3Bucket      = aws.StateStorageTypeS3Bucket
	StateStorageTypeFileSystem    = fs.StateStorageTypeFileSystem
	SecretsProviderTypePassPhrase = fs.SecretsProviderTypePassphrase
	BackendTypePulumiCloud        = "pulumi-cloud"
	SecretsProviderTypeGcpKms     = "gcp-kms"
	SecretsProviderTypeAwsKms     = "aws-kms"
)
