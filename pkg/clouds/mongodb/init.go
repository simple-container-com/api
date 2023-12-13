package mongodb

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// mongodb
		ResourceTypeMongodbAtlas: ReadAtlasConfig,
	})
}
