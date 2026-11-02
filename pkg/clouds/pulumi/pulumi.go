// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"github.com/simple-container-com/api/pkg/clouds/fs"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

const (
	StateStorageTypeGcpBucket = gcloud.StateStorageTypeGcpBucket
	// Literal string instead of the sc-plugin-aws/api constant so SC
	// core doesn't need to Go-link-time-import the plugin's /api
	// module. The token is a wire-level string that both sides have
	// agreed on since the aws plugin split; a const in the plugin's
	// /api is just one place that spelling lives. See A3 smoke.
	StateStorageTypeS3Bucket      = "s3-bucket"
	StateStorageTypeFileSystem    = fs.StateStorageTypeFileSystem
	SecretsProviderTypePassPhrase = fs.SecretsProviderTypePassphrase
	BackendTypePulumiCloud        = "pulumi-cloud"
	SecretsProviderTypeGcpKms     = "gcp-kms"
	SecretsProviderTypeAwsKms     = "aws-kms"
)
