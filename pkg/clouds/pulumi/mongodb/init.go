package mongodb

import (
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	pApi.RegisterProvider(mongodb.ProviderType, Provider)
	pApi.RegisterResources(map[string]pApi.ProvisionFunc{
		mongodb.ResourceTypeMongodbAtlas: Cluster,
	})
	pApi.RegisterComputeProcessor(map[string]pApi.ComputeProcessorFunc{
		mongodb.ResourceTypeMongodbAtlas: ClusterComputeProcessor,
	})
}
