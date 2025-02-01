package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type RedisConnectionParams struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

func HelmRedisOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmRedisOperator {
		return nil, errors.Errorf("unsupported redis operator type %q", input.Descriptor.Type)
	}
	labels, annotations := labelsAnnotations(input, stack)
	opts := []sdk.ResourceOption{sdk.ResourceOption(sdk.Provider(params.Provider))}

	cfg, operator, err := deployOperatorChart[*k8s.HelmRedisOperator](ctx, stack, input, params, deployChartCfg{
		name:      "redis-operator",
		repo:      lo.ToPtr("https://ot-container-kit.github.io/helm-charts"),
		defaultNS: "operators",
		values: map[string]any{
			"redisOperator": map[string]any{
				"podAnnotations": annotations,
				"podLabels":      labels,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{operator}))

	instanceName := toRedisInstanceName(input, input.Descriptor.Name)
	namespace := lo.If(cfg.Namespace() != nil, lo.FromPtr(cfg.Namespace())).Else("default")

	instance, err := helm.NewRelease(ctx, instanceName, &helm.ReleaseArgs{
		Atomic:    sdk.BoolPtr(true),
		Name:      sdk.StringPtr(instanceName),
		Chart:     sdk.String("redis"),
		Namespace: sdk.StringPtr(namespace),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: sdk.StringPtr("https://ot-container-kit.github.io/helm-charts"),
		},
		Values: sdk.ToMap(lo.Assign(cfg.Values(), map[string]any{
			"redisStandalone": fields{
				"name": instanceName,
			},
			"labels": labels,
		})),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create redis instance %q", instanceName)
	}

	connectionExport := toRedisConnectionParamsExport(instanceName)
	connectionOut, err := objectToStringMapOutput(&RedisConnectionParams{
		Host: fmt.Sprintf("%s.%s.svc.cluster.local", instanceName, namespace),
		Port: "6379",
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to export redis connection %q", instanceName)
	}
	params.Log.Info(ctx.Context(), "Exporting redis %q connection as %q...", instanceName, connectionExport)
	ctx.Export(connectionExport, connectionOut)

	return &api.ResourceOutput{
		Ref: instance,
	}, nil
}

func toRedisInstanceName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}

func toRedisConnectionParamsExport(resName string) string {
	return fmt.Sprintf("%s-redis-connection", resName)
}
