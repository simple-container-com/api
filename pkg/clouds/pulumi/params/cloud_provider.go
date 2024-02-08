package params

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ProvisionParams struct {
	Provider sdk.ProviderResource
	// todo: more
}

type ProviderInput struct {
	Name     string
	Resource any
}

type ProviderOutput struct {
	Provider sdk.ProviderResource
}
