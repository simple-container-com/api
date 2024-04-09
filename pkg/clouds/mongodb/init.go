package mongodb

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "mongodb-atlas"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// mongodb
		ResourceTypeMongodbAtlas: ReadAtlasConfig,
	})
}
