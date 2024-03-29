package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ProvisionParams struct {
	Provider  sdk.ProviderResource
	Registrar Registrar
}
