package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type fields map[string]any

func HelmPostgresOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmPostgresOperator {
		return nil, errors.Errorf("unsupported postgres operator type %q", input.Descriptor.Type)
	}
	opts := []sdk.ResourceOption{sdk.ResourceOption(sdk.Provider(params.Provider))}

	cfg, err := deployOperatorChart[*k8s.HelmPostgresOperator](ctx, input, params, deployChartCfg{
		name:      "postgres-operator",
		repo:      "https://opensource.zalando.com/postgres-operator/charts/postgres-operator",
		defaultNS: "operators",
	})
	if err != nil {
		return nil, err
	}

	annotations := map[string]string{
		"pulumi.com/patchForce": "true",
		AnnotationEnv:           input.StackParams.Environment,
	}
	labels := map[string]string{
		LabelAppName: stack.Name,
		LabelAppType: AppTypeSimpleContainer,
		LabelScEnv:   input.StackParams.Environment,
	}

	rootUser := "root"
	instanceName := toPostgresInstanceName(input, input.Descriptor.Name)
	namespace := lo.If(cfg.Namespace() != nil, lo.FromPtr(cfg.Namespace())).Else("default")
	instance, err := apiextensions.NewCustomResource(ctx, instanceName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("acid.zalan.do/v1"),
		Kind:       sdk.String("postgresql"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(instanceName),
			Namespace:   sdk.String(namespace),
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
		},
		OtherFields: map[string]any{
			"spec": fields{
				"teamId": sdk.String(instanceName),
				"volume": fields{
					"size": sdk.String(lo.If(cfg.VolumeSize != nil, lo.FromPtr(cfg.VolumeSize)).Else("5Gi")),
				},
				"numberOfInstances": sdk.Int(lo.If(cfg.NumberOfInstances != nil, lo.FromPtr(cfg.NumberOfInstances)).Else(1)),
				"users": fields{
					// user: [<roles>,...]
					rootUser: sdk.ToArray([]any{
						"superuser",
						"createdb",
					}),
				},
				"postgresql": fields{
					"version": sdk.String(lo.If(cfg.Version != nil, lo.FromPtr(cfg.Version)).Else("15")),
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create postgres instance %q", instanceName)
	}

	if !ctx.DryRun() {
		rootPassExport := toPostgresRootPasswordExport(instanceName)
		ctx.Export(toPostgresRootUsernameExport(instanceName), sdk.String(rootUser).ToStringOutput())
		rootPassOut := instance.OtherFields.ApplyT(func(otherFields map[string]any) (sdk.StringOutput, error) {
			params.Log.Info(ctx.Context(), "got other fields values: %v", otherFields)
			secretName := fmt.Sprintf("%s/%s.%s.credentials.postgresql.acid.zalan.do", namespace, rootUser, instanceName)
			return waitUntilSecretExists(ctx, params, secretName, func(secret *corev1.Secret) (sdk.StringOutput, error) {
				rootPassword := secret.Data.ApplyT(func(data map[string]string) (string, error) {
					password, err := base64.StdEncoding.DecodeString(data["password"])
					return string(password), err
				}).(sdk.StringOutput)
				params.Log.Info(ctx.Context(), "Exporting postgres %q root user's (%q) password as %q...", instanceName, rootUser, rootPassExport)
				return rootPassword, nil
			})
		})
		ctx.Export(rootPassExport, rootPassOut)
		ctx.Export(toPostgresRootURLExport(instanceName), rootPassOut.ApplyT(func(rootPass string) string {
			rootURL := fmt.Sprintf("postgresql://%s:%s@%s.%s.svc.cluster.local:%d", rootUser, rootPass, instanceName, namespace, 5432)
			params.Log.Info(ctx.Context(), "Exporting postgres %q root url...", instanceName)
			return rootURL
		}))
	}

	return &api.ResourceOutput{
		Ref: instance,
	}, nil
}

func toPostgresInstanceName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}

func toPostgresRootUsernameExport(resName string) string {
	return fmt.Sprintf("%s-pg-root-username", resName)
}

func toPostgresRootPasswordExport(resName string) string {
	return fmt.Sprintf("%s-pg-root-password", resName)
}

func toPostgresRootURLExport(resName string) string {
	return fmt.Sprintf("%s-pg-root-url", resName)
}

func waitUntilSecretExists(ctx *sdk.Context, params pApi.ProvisionParams, secretName string, callback func(secret *corev1.Secret) (sdk.StringOutput, error)) (sdk.StringOutput, error) {
	if !ctx.DryRun() {
		withTimeout, cancel := context.WithTimeout(ctx.Context(), 30*time.Second)
		defer cancel()
		ticker := time.NewTicker(1 * time.Second)
		idx := 0
		for {
			select {
			case <-withTimeout.Done():
				params.Log.Error(ctx.Context(), "failed to wait until postgres instance secrets exist")
				return sdk.StringOutput{}, errors.Errorf("timeout while waiting until postgres instance secret %q exists", secretName)
			case <-ticker.C:
				secret, err := corev1.GetSecret(ctx, fmt.Sprintf("%s-get-%d", secretName, idx), sdk.ID(secretName), nil, sdk.Provider(params.Provider))
				idx++
				if err != nil {
					params.Log.Warn(ctx.Context(), "still waiting for secret %q to exist: %s", secretName, err.Error())
					continue
				}
				return callback(secret)
			}
		}
	}
	return sdk.StringOutput{}, errors.Errorf("failed to wait until secret exists")
}

func HelmRabbitmqOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmRabbitmqOperator {
		return nil, errors.Errorf("unsupported rabbitmq operator type %q", input.Descriptor.Type)
	}

	_, err := deployOperatorChart[*k8s.HelmRabbitmqOperator](ctx, input, params, deployChartCfg{
		name:      "rabbitmq-cluster-operator",
		repo:      "https://charts.bitnami.com/bitnami",
		defaultNS: "operators",
	})
	if err != nil {
		return nil, err
	}

	return &api.ResourceOutput{}, nil
}

func HelmRedisOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmRedisOperator {
		return nil, errors.Errorf("unsupported redis operator type %q", input.Descriptor.Type)
	}

	_, err := deployOperatorChart[*k8s.HelmRedisOperator](ctx, input, params, deployChartCfg{
		name:      "redis-operator",
		repo:      "https://ot-container-kit.github.io/helm-charts",
		defaultNS: "operators",
	})
	if err != nil {
		return nil, err
	}

	return &api.ResourceOutput{}, nil
}

func HelmMongodbOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmMongodbOperator {
		return nil, errors.Errorf("unsupported mongodb operator type %q", input.Descriptor.Type)
	}

	_, err := deployOperatorChart[*k8s.HelmMongodbOperator](ctx, input, params, deployChartCfg{
		name:      "community-operator",
		repo:      "https://mongodb.github.io/helm-charts",
		defaultNS: "operators",
	})
	if err != nil {
		return nil, err
	}

	return &api.ResourceOutput{}, nil
}

type deployChartCfg struct {
	name      string
	repo      string
	defaultNS string
}

func deployOperatorChart[T k8s.HelmOperatorChart](ctx *sdk.Context, input api.ResourceInput, params pApi.ProvisionParams, cfg deployChartCfg) (T, error) {
	chartCfg, ok := input.Descriptor.Config.Config.(T)
	if !ok {
		return chartCfg, errors.Errorf("failed to convert chart's %q config for %q", cfg.name, input.Descriptor.Type)
	}

	params.Log.Info(ctx.Context(), "Deploying %q helm chart...", cfg.name)

	releaseName := input.ToResName(input.Descriptor.Name)
	namespace := input.ToResName(lo.If(cfg.defaultNS != "", cfg.defaultNS).Else("operators"))
	if chartCfg.OperatorNamespace() != nil {
		namespace = lo.FromPtr(chartCfg.OperatorNamespace())
	}
	_, err := helm.NewRelease(ctx, releaseName, &helm.ReleaseArgs{
		Chart:           sdk.String(cfg.name),
		Name:            sdk.String(releaseName),
		Atomic:          sdk.BoolPtr(true),
		CreateNamespace: sdk.BoolPtr(true),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: sdk.String(cfg.repo),
		},
		Namespace: sdk.String(namespace),
		Values:    sdk.ToMap(chartCfg.Values()),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return chartCfg, errors.Wrapf(err, "failed to install %q helm chart", cfg.name)
	}
	return chartCfg, nil
}
