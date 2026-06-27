// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/util"
)

type PostgresDBInstanceArgs struct {
	Project      string
	InstanceName string
	Region       string
}

type CloudSQLAccount struct {
	ServiceAccount     *serviceaccount.Account
	ServiceAccountKey  *serviceaccount.Key
	CredentialsSecrets sdk.StringMap
}

func NewCloudSQLAccount(ctx *sdk.Context, name string, dbInstance PostgresDBInstanceArgs, provider *gcp.Provider, opts ...sdk.ResourceOption) (*CloudSQLAccount, error) {
	accountName := util.SanitizeGCPServiceAccountName(name)

	opts = append(opts, sdk.Provider(provider))
	serviceAccount, err := serviceaccount.NewAccount(ctx, accountName, &serviceaccount.AccountArgs{
		AccountId:   sdk.String(accountName),
		Project:     sdk.String(dbInstance.Project),
		Description: sdk.String(fmt.Sprintf("Service account to access database %s", dbInstance.InstanceName)),
		DisplayName: sdk.String(fmt.Sprintf("%s-service-account", name)),
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, sdk.Parent(serviceAccount))
	serviceAccountKey, err := serviceaccount.NewKey(ctx, fmt.Sprintf("%s-key", accountName), &serviceaccount.KeyArgs{
		ServiceAccountId: serviceAccount.AccountId,
	}, opts...)
	if err != nil {
		return nil, err
	}

	_, err = projects.NewIAMMember(ctx, fmt.Sprintf("%s-iam", accountName), &projects.IAMMemberArgs{
		Project: sdk.String(dbInstance.Project),
		Member:  serviceAccount.Member,
		Role:    sdk.String("roles/cloudsql.client"),
	}, opts...)
	if err != nil {
		return nil, err
	}

	credentialsSecrets := sdk.StringMap{
		"credentials.json": serviceAccountKey.PrivateKey,
	}

	return &CloudSQLAccount{
		ServiceAccount:     serviceAccount,
		ServiceAccountKey:  serviceAccountKey,
		CredentialsSecrets: credentialsSecrets,
	}, nil
}

type CloudSQLProxyArgs struct {
	Name         string
	DBInstance   PostgresDBInstanceArgs
	GcpProvider  *gcp.Provider
	KubeProvider *sdkK8s.Provider
	Metadata     *metav1.ObjectMetaArgs
	TimeoutSec   int
}

type CloudSQLProxy struct {
	ProxyContainer sdk.Output
	Account        *CloudSQLAccount
	Name           string
	SqlProxySecret *v1.Secret
}

func NewCloudsqlProxy(ctx *sdk.Context, args CloudSQLProxyArgs, opts ...sdk.ResourceOption) (*CloudSQLProxy, error) {
	account, err := NewCloudSQLAccount(ctx, args.Name, args.DBInstance, args.GcpProvider, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, sdk.Provider(args.KubeProvider))
	// args.Metadata.Namespace is the live Namespace.Metadata.Name() Output, threaded
	// through by compute_proc.go (preprocessor uses kubeArgs.NamespaceNameOutput,
	// postprocessor uses sc.Namespace). That means this Secret automatically lands
	// in the same k8s namespace as the consuming pod under both fresh deploys (isolated
	// name) and migrated stacks where #255's IgnoreChanges("metadata.name") keeps the
	// Namespace parent-shared — no IgnoreChanges needed here.
	secretName := util.SanitizeK8sResourceName(args.Name + "-creds")
	sqlProxySecret, err := v1.NewSecret(ctx, secretName, &v1.SecretArgs{
		Metadata: args.Metadata,
		Data:     account.CredentialsSecrets,
	}, opts...)
	if err != nil {
		return nil, err
	}

	proxyContainer := cloudsqlProxyContainer(sqlProxySecret, args.DBInstance, args.TimeoutSec)

	return &CloudSQLProxy{
		ProxyContainer: proxyContainer,
		Account:        account,
		Name:           args.Name,
		SqlProxySecret: sqlProxySecret,
	}, nil
}

// cloudSQLProxyHealthPort is the port the proxy's built-in health-check HTTP server
// listens on when running as a native sidecar (--health-check). Its /startup endpoint
// backs the startup probe that gates the app containers.
const cloudSQLProxyHealthPort = 9090

func cloudsqlProxyContainer(credsSecret *v1.Secret, dbInstance PostgresDBInstanceArgs, timeout int) sdk.Output {
	return sdk.All(credsSecret.Metadata.Name(), dbInstance.Project, dbInstance.Region, dbInstance.InstanceName).ApplyT(func(all []interface{}) v1.ContainerArgs {
		secretName := all[0].(*string)
		project := all[1].(string)
		region := all[2].(string)
		instanceName := all[3].(string)
		return cloudsqlProxyContainerArgs(lo.FromPtr(secretName), project, region, instanceName, timeout)
	}).(v1.ContainerOutput)
}

// cloudsqlProxyCommandArgs returns the proxy entrypoint. timeout == 0 is the long-lived
// runtime proxy (with its health server enabled); timeout > 0 is the init-Job proxy,
// shell-wrapped to self-kill after `timeout`s so a RestartPolicy: Never Job can complete.
func cloudsqlProxyCommandArgs(project, region, instanceName string, timeout int) (string, []string) {
	command := "/cloud-sql-proxy"
	args := []string{
		"--address",
		"0.0.0.0",
		"--structured-logs",
		"--credentials-file=/var/run/secrets/cloudsql/credentials.json",
		fmt.Sprintf("%s:%s:%s", project, region, instanceName),
	}

	if timeout > 0 {
		return "sh", []string{
			"-c",
			fmt.Sprintf(`
                    echo "Starting proxy with timeout %ds..."
                    %s %s &
                    PROXY_PID=$!

                    echo "Waiting %ds until killing proxy..."
                    sleep %d;

                    echo "Killing proxy after %ds"
                    kill -9 $PROXY_PID;
                    exit 0;
                `, timeout, command, strings.Join(args, " "), timeout, timeout, timeout),
		}
	}

	args = append(args,
		"--http-address=0.0.0.0",
		fmt.Sprintf("--http-port=%d", cloudSQLProxyHealthPort),
		"--health-check",
	)
	return command, args
}

// cloudsqlProxyContainerArgs builds the proxy container from already-resolved values.
// timeout == 0 yields a native sidecar (RestartPolicy: Always + startup probe) so the app
// containers don't start before the proxy is listening. timeout > 0 (init-Job) stays an
// ordinary terminating container -- it must NOT be a native sidecar or the Job would hang.
func cloudsqlProxyContainerArgs(secretName, project, region, instanceName string, timeout int) v1.ContainerArgs {
	command, args := cloudsqlProxyCommandArgs(project, region, instanceName, timeout)

	container := v1.ContainerArgs{
		Name:    sdk.String("cloudsql-proxy"),
		Image:   sdk.String("gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.8.1-alpine"),
		Command: sdk.StringArray{sdk.String(command)},
		Args:    sdk.ToStringArray(args),
		SecurityContext: &v1.SecurityContextArgs{
			RunAsNonRoot: sdk.Bool(true),
		},
		Resources: &v1.ResourceRequirementsArgs{
			Limits: sdk.StringMap{
				"memory": sdk.String("300Mi"),
				"cpu":    sdk.String("300m"),
			},
			Requests: sdk.StringMap{
				"memory": sdk.String("200Mi"),
				"cpu":    sdk.String("50m"),
			},
		},
		VolumeMounts: v1.VolumeMountArray{
			&v1.VolumeMountArgs{
				Name:      sdk.String(secretName),
				MountPath: sdk.String("/var/run/secrets/cloudsql"),
				ReadOnly:  sdk.Bool(true),
			},
		},
	}

	if timeout > 0 {
		return container
	}

	container.RestartPolicy = sdk.String("Always")
	container.Ports = v1.ContainerPortArray{
		&v1.ContainerPortArgs{
			Name:          sdk.String("csql-hc"),
			ContainerPort: sdk.Int(cloudSQLProxyHealthPort),
		},
	}
	container.StartupProbe = &v1.ProbeArgs{
		HttpGet: v1.HTTPGetActionArgs{
			Path: sdk.String("/startup"),
			Port: sdk.String("csql-hc"),
		},
		PeriodSeconds:    sdk.IntPtr(2),
		TimeoutSeconds:   sdk.IntPtr(3),
		FailureThreshold: sdk.IntPtr(30),
	}
	container.ReadinessProbe = &v1.ProbeArgs{
		HttpGet: v1.HTTPGetActionArgs{
			Path: sdk.String("/readiness"),
			Port: sdk.String("csql-hc"),
		},
		PeriodSeconds:    sdk.IntPtr(10),
		TimeoutSeconds:   sdk.IntPtr(3),
		FailureThreshold: sdk.IntPtr(3),
	}
	// On a native sidecar a failing readiness probe neither restarts the container nor
	// gates pod readiness — only liveness recovers a proxy that passed startup and then
	// hung (deadlock / pool exhaustion / partial-OOM), where the process stays alive but
	// app DB calls to localhost:5432 fail. /liveness is already served by --health-check.
	// kubelet defers liveness until the startup probe succeeds, so no InitialDelay is needed.
	container.LivenessProbe = &v1.ProbeArgs{
		HttpGet: v1.HTTPGetActionArgs{
			Path: sdk.String("/liveness"),
			Port: sdk.String("csql-hc"),
		},
		PeriodSeconds:    sdk.IntPtr(10),
		TimeoutSeconds:   sdk.IntPtr(3),
		FailureThreshold: sdk.IntPtr(3),
	}
	return container
}
