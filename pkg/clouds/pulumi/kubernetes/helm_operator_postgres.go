package kubernetes

import (
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
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

	labels, annotations := labelsAnnotations(input, stack)

	cfg, operator, err := deployOperatorChart[*k8s.HelmPostgresOperator](ctx, stack, input, params, deployChartCfg{
		name:      "postgres-operator",
		repo:      lo.ToPtr("https://opensource.zalando.com/postgres-operator/charts/postgres-operator"),
		defaultNS: "operators",
		values: map[string]any{
			"configLoadBalancer": map[string]any{
				"custom_service_annotations": annotations,
			},
			"configKubernetes": map[string]any{
				"custom_pod_annotations": annotations,
			},
			"podLabels":      labels,
			"podAnnotations": annotations,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to deploy postgres operator chart")
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{operator}))

	rootUser := "root"
	instanceName := toPostgresInstanceName(input, input.Descriptor.Name)
	namespace := lo.If(cfg.Namespace() != nil, lo.FromPtr(cfg.Namespace())).Else("default")

	instanceSpec := map[string]any{
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
	}
	if len(cfg.PgHbaEntries) > 0 {
		instanceSpec["patroni"] = fields{
			"pg_hba": cfg.PgHbaEntries,
		}
	}
	instance, err := apiextensions.NewCustomResource(ctx, instanceName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("acid.zalan.do/v1"),
		Kind:       sdk.String("postgresql"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(instanceName),
			Namespace:   sdk.String(namespace),
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
		},
		OtherFields: instanceSpec,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create postgres instance %q", instanceName)
	}

	if !ctx.DryRun() {
		rootPassExport := toPostgresRootPasswordExport(instanceName)
		ctx.Export(toPostgresRootUsernameExport(instanceName), sdk.String(rootUser).ToStringOutput())
		secretName := fmt.Sprintf("%s/%s.%s.credentials.postgresql.acid.zalan.do", namespace, rootUser, instanceName)
		opts = append(opts, sdk.DependsOn([]sdk.Resource{instance}))
		rootPassOut, err := waitUntilSecretExists(ctx, params, secretName, func(secret *corev1.Secret) (sdk.Output, error) {
			params.Log.Info(ctx.Context(), "Exporting postgres %q root user's (%q) password as %q...", instanceName, rootUser, rootPassExport)
			return secret.Data.ApplyT(func(data map[string]string) (string, error) {
				password, err := base64.StdEncoding.DecodeString(data["password"])
				return string(password), err
			}), nil
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to wait until secret exists")
		}
		ctx.Export(rootPassExport, rootPassOut)
		ctx.Export(toPostgresRootURLExport(instanceName), rootPassOut.ApplyT(func(rootPass string) string {
			rootURL := fmt.Sprintf("postgresql://%s:%s@%s.%s.svc.cluster.local:%d", rootUser, rootPass, instanceName, namespace, 5432)
			params.Log.Info(ctx.Context(), "Exporting postgres %q root url...", instanceName)
			return rootURL
		}))
		ctx.Export(toPostgresInitSQLExport(instanceName), sdk.ToSecret(sdk.String(lo.FromPtr(cfg.InitSQL))))
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

func toPostgresInitSQLExport(resName string) string {
	return fmt.Sprintf("%s-pg-init-sql", resName)
}
