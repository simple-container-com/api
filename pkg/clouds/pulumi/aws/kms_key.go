package aws

import (
	"fmt"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
)

func ProvisionKmsKey(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	return nil, fmt.Errorf("not implemented")
}
