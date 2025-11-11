package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type RabbitmqConnectionParams struct {
	ConnectionString string `json:"connection_string"`
	DefaultUserConf  string `json:"default_user.conf"`
	Host             string `json:"host"`
	Password         string `json:"password"`
	Port             string `json:"port"`
	Provider         string `json:"provider"`
	Type             string `json:"type"`
	Username         string `json:"username"`
}

func HelmRabbitmqOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmRabbitmqOperator {
		return nil, errors.Errorf("unsupported rabbitmq operator type %q", input.Descriptor.Type)
	}

	labels, annotations := labelsAnnotations(input, stack)
	opts := []sdk.ResourceOption{sdk.ResourceOption(sdk.Provider(params.Provider))}

	cfg, operator, err := deployOperatorChart[*k8s.HelmRabbitmqOperator](ctx, stack, input, params, deployChartCfg{
		name:      "oci://registry-1.docker.io/bitnamicharts/rabbitmq-cluster-operator",
		defaultNS: "operators",
		version:   lo.ToPtr("4.4.1"),
		values: fields{
			"commonAnnotations": annotations,
			"commonLabels":      labels,
			// Use bitnamilegacy repositories for compatibility
			"clusterOperator": fields{
				"image": fields{
					"repository": "bitnamilegacy/rabbitmq-cluster-operator",
				},
			},
			"msgTopologyOperator": fields{
				"image": fields{
					"repository": "bitnamilegacy/rmq-messaging-topology-operator",
				},
			},
			"rabbitmqImage": fields{
				"repository": "bitnamilegacy/rabbitmq",
			},
			"credentialUpdaterImage": fields{
				"repository": "bitnamilegacy/rmq-default-credential-updater",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{operator}))

	instanceName := toRabbitmqInstanceName(input, input.Descriptor.Name)
	namespace := lo.If(cfg.Namespace() != nil, lo.FromPtr(cfg.Namespace())).Else("default")
	instance, err := apiextensions.NewCustomResource(ctx, instanceName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("rabbitmq.com/v1beta1"),
		Kind:       sdk.String("RabbitmqCluster"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(instanceName),
			Namespace:   sdk.String(namespace),
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
		},
		OtherFields: map[string]any{
			"spec": fields{
				"replicas": sdk.Int(lo.If(cfg.Replicas != nil, lo.FromPtr(cfg.Replicas)).Else(1)),
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rabbitmq instance %q", instanceName)
	}

	if !ctx.DryRun() {
		connectionExport := toRabbitmqConnectionParamsExport(instanceName)
		secretName := fmt.Sprintf("%s/%s-default-user", namespace, instanceName)
		opts = append(opts, sdk.DependsOn([]sdk.Resource{instance}))
		if err := exportSecretValues(ctx, secretName, connectionExport, params, &RabbitmqConnectionParams{}, opts...); err != nil {
			return nil, errors.Wrapf(err, "failed to export rabbitmq connection secret")
		}
	}

	return &api.ResourceOutput{
		Ref: instance,
	}, nil
}

func toRabbitmqInstanceName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}

func toRabbitmqConnectionParamsExport(resName string) string {
	return fmt.Sprintf("%s-rabbitmq-connection", resName)
}
