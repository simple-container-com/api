package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api/logger"
)

type ProvisionParams struct {
	Provider  sdk.ProviderResource
	Registrar Registrar
	Log       logger.Logger
}
