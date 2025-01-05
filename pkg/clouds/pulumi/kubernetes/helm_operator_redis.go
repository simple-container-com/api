package kubernetes

import (
	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func HelmRedisOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmRedisOperator {
		return nil, errors.Errorf("unsupported redis operator type %q", input.Descriptor.Type)
	}

	_, _, err := deployOperatorChart[*k8s.HelmRedisOperator](ctx, stack, input, params, deployChartCfg{
		name:      "redis-operator",
		repo:      lo.ToPtr("https://ot-container-kit.github.io/helm-charts"),
		defaultNS: "operators",
	})
	if err != nil {
		return nil, err
	}

	return &api.ResourceOutput{}, nil
}
