package aws

import (
	"fmt"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionKmsKey(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error) {
	return nil, fmt.Errorf("not implemented")
}
