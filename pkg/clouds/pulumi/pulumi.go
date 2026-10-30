// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"github.com/simple-container-com/api/pkg/clouds/fs"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/sc-plugin-aws/api"
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
