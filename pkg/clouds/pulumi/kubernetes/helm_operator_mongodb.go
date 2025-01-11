package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type MongodbConnectionParams struct {
	InstanceName string `json:"instanceName"`
	Host         string `json:"host"`
	Port         string `json:"port"`
	Password     string `json:"password"`
	Username     string `json:"username"`
	Database     string `json:"database"`
}

func (p MongodbConnectionParams) ConnectionString() string {
	return fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?readPreference=primary&replicaSet=%s", p.Username, p.Password, p.Host, p.Port, p.Database, p.InstanceName)
}

func HelmMongodbOperator(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeHelmMongodbOperator {
		return nil, errors.Errorf("unsupported mongodb operator type %q", input.Descriptor.Type)
	}
	opts := []sdk.ResourceOption{sdk.ResourceOption(sdk.Provider(params.Provider))}
	labels, annotations := labelsAnnotations(input, stack)

	instanceName := toMongodbInstanceName(input, input.Descriptor.Name)

	cfg, operator, err := deployOperatorChart[*k8s.HelmMongodbOperator](ctx, stack, input, params, deployChartCfg{
		name:      "community-operator",
		repo:      lo.ToPtr("https://mongodb.github.io/helm-charts"),
		defaultNS: "operators",
		values: map[string]any{
			"operator": fields{
				"watchNamespace": sdk.String("*"),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	opts = append(opts, sdk.DependsOn([]sdk.Resource{operator}))
	namespace := lo.If(cfg.Namespace() != nil, lo.FromPtr(cfg.Namespace())).Else("default")
	rootPasswordSecretName := fmt.Sprintf("%s-root-password", input.ToResName(input.Descriptor.Name))
	rootPasswordSecretNameScram := fmt.Sprintf("%s-root-password-scram", input.ToResName(input.Descriptor.Name))
	rootPassword, err := random.NewRandomPassword(ctx, fmt.Sprintf("%s-root-generated-password", rootPasswordSecretName), &random.RandomPasswordArgs{
		Length:          sdk.Int(16),
		OverrideSpecial: sdk.String("-_"),
		Special:         sdk.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate root mongodb password")
	}
	rootPasswordSecret, err := corev1.NewSecret(ctx, rootPasswordSecretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(rootPasswordSecretName),
			Namespace:   sdk.String(namespace),
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
		},
		Type: sdk.String("Opaque"),
		StringData: sdk.ToStringMapOutput(map[string]sdk.StringOutput{
			"password": rootPassword.Result,
		}),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb password secret")
	}

	opts = append(opts, sdk.DependsOn([]sdk.Resource{rootPasswordSecret}))

	dbSa, err := corev1.NewServiceAccount(ctx, input.ToResName("mongodb-database"), &corev1.ServiceAccountArgs{
		AutomountServiceAccountToken: sdk.Bool(true),
		Metadata: &metav1.ObjectMetaArgs{
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
			Namespace:   sdk.String(namespace),
			Name:        sdk.String("mongodb-database"),
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb service account")
	}

	opts = append(opts, sdk.DependsOn([]sdk.Resource{dbSa}))

	dbRole, err := rbacv1.NewRole(ctx, input.ToResName("mongodb-database"), &rbacv1.RoleArgs{
		Metadata: metav1.ObjectMetaArgs{
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
			Namespace:   sdk.String(namespace),
			Name:        sdk.String("mongodb-database"),
		},
		Rules: rbacv1.PolicyRuleArray{
			rbacv1.PolicyRuleArgs{
				ApiGroups: sdk.ToStringArray([]string{""}),
				Resources: sdk.ToStringArray([]string{"secrets"}),
				Verbs:     sdk.ToStringArray([]string{"get"}),
			},
			rbacv1.PolicyRuleArgs{
				ApiGroups: sdk.ToStringArray([]string{""}),
				Resources: sdk.ToStringArray([]string{"pods"}),
				Verbs:     sdk.ToStringArray([]string{"patch", "delete", "get"}),
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb role")
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{dbRole}))

	dbRoleBind, err := rbacv1.NewRoleBinding(ctx, input.ToResName("mongodb-database"), &rbacv1.RoleBindingArgs{
		Metadata: metav1.ObjectMetaArgs{
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
			Namespace:   sdk.String(namespace),
			Name:        sdk.String("mongodb-database"),
		},
		RoleRef: rbacv1.RoleRefArgs{
			Name: sdk.String("mongodb-database"),
			Kind: sdk.String("Role"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind: sdk.String("ServiceAccount"),
				Name: sdk.String("mongodb-database"),
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb role binding")
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{dbRoleBind}))

	rootUser := "root"
	rootDatabase := "admin"
	instance, err := apiextensions.NewCustomResource(ctx, instanceName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("mongodbcommunity.mongodb.com/v1"),
		Kind:       sdk.String("MongoDBCommunity"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(instanceName),
			Namespace:   sdk.String(namespace),
			Annotations: sdk.ToStringMap(annotations),
			Labels:      sdk.ToStringMap(labels),
		},
		OtherFields: map[string]any{
			"spec": fields{
				"type":    sdk.String("ReplicaSet"),
				"version": sdk.String(lo.If(cfg.Version != nil, lo.FromPtr(cfg.Version)).Else("6.0.5")),
				"members": sdk.Int(lo.If(cfg.Replicas != nil, lo.FromPtr(cfg.Replicas)).Else(3)),
				"security": fields{
					"authentication": fields{
						"ignoreUnknownUsers": sdk.Bool(true),
						"modes":              sdk.StringArray{sdk.String("SCRAM"), sdk.String("SCRAM-SHA-1")},
					},
				},
				"users": []fields{{
					"name": sdk.String(rootUser),
					"db":   sdk.String(rootDatabase),
					"passwordSecretRef": fields{
						"name": sdk.String(rootPasswordSecretName),
					},
					"roles": []fields{
						{
							"name": sdk.String("clusterAdmin"),
							"db":   sdk.String(rootDatabase),
						},
						{
							"name": sdk.String("userAdminAnyDatabase"),
							"db":   sdk.String(rootDatabase),
						},
					},
					"scramCredentialsSecretName": sdk.String(rootPasswordSecretNameScram),
				}},
				"additionalMongodConfig": fields{
					"storage.wiredTiger.engineConfig.journalCompressor": sdk.String("zlib"),
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create mongodb instance %q", instanceName)
	}

	connectionExport := toMongodbConnectionParamsExport(instanceName)
	params.Log.Info(ctx.Context(), "Exporting mongodb %q connection as %q...", instanceName, connectionExport)
	ctx.Export(connectionExport, rootPassword.Result.ApplyT(func(password string) (sdk.StringMapOutput, error) {
		return objectToStringMapOutput(&MongodbConnectionParams{
			InstanceName: instanceName,
			Host:         fmt.Sprintf("%s-svc.%s.svc.cluster.local", instanceName, namespace),
			Port:         "27017",
			Password:     password,
			Username:     rootUser,
			Database:     rootDatabase,
		})
	}))

	return &api.ResourceOutput{
		Ref: instance,
	}, nil
}

func toMongodbInstanceName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}

func toMongodbConnectionParamsExport(resName string) string {
	return fmt.Sprintf("%s-mongodb-connection", resName)
}
