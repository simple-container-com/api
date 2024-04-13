package mongodb

import (
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterProvider(mongodb.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		mongodb.ResourceTypeMongodbAtlas: Cluster,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		mongodb.ResourceTypeMongodbAtlas: ClusterComputeProcessor,
	})
}
