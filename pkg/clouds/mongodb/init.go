package mongodb

import (
	"github.com/simple-container-com/api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// mongodb
		ResourceTypeMongodbAtlas: ReadAtlasConfig,
	})
}
