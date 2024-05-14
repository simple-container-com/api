package aws

import (
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws/helpers"
)

const ProviderType = "aws"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// gcloud
		SecretsTypeAWSSecretsManager: ReadSecretsConfig,
		TemplateTypeEcsFargate:       ReadTemplateConfig,
		TemplateTypeStaticWebsite:    ReadTemplateConfig,
		AuthTypeAWSToken:             ReadAuthServiceAccountConfig,

		// bucket
		ResourceTypeS3Bucket: S3BucketReadConfig,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeS3Bucket:  ReadStateStorageConfig,
		SecretsProviderTypeAwsKms: ReadSecretsProviderConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeEcsFargate: ToEcsFargateConfig,
	})

	api.RegisterCloudStaticSiteConverter(api.CloudStaticSiteConfigRegister{
		TemplateTypeStaticWebsite: ToStaticSiteConfig,
	})

	api.RegisterCloudHelper(api.CloudHelpersRegisterMap{
		helpers.CHCloudwatchAlertLambda: helpers.NewLambdaHelper,
	})
}
