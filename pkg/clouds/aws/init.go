package aws

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// gcloud
		SecretsTypeAWSSecretsManager: ReadSecretsConfig,
		TemplateTypeEcsFargate:       ReadTemplateConfig,
		AuthTypeAWSToken:             ReadAuthServiceAccountConfig,

		// bucket
		ResourceTypeS3Bucket: S3BucketReadConfig,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeS3Bucket:  ReadStateStorageConfig,
		SecretsProviderTypeAwsKms: ReadSecretsProviderConfig,
	})
}
