package aws

import (
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterInitStateStore(aws.ProviderType, InitStateStore)
	api.RegisterProvider(aws.ProviderType, Provider)

	api.RegisterResources(map[string]api.ProvisionFunc{
		aws.ResourceTypeS3Bucket:      S3Bucket,
		aws.SecretsProviderTypeAwsKms: KmsKeySecretsProvider,
		aws.TemplateTypeEcsFargate:    EcsFargate,
		aws.TemplateTypeAwsLambda:     Lambda,
		aws.TemplateTypeStaticWebsite: StaticWebsite,
		aws.ResourceTypeRdsPostgres:   RdsPostgres,
		aws.ResourceTypeRdsMysql:      RdsMysql,
		aws.ResourceTypeEcrRepository: EcrRepository,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		aws.ResourceTypeS3Bucket:    S3BucketComputeProcessor,
		aws.ResourceTypeRdsPostgres: RdsPostgresComputeProcessor,
		aws.ResourceTypeRdsMysql:    RdsMysqlComputeProcessor,
	})
}
