package gcp

import (
	"fmt"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const MaxInitSQLTimeSec = 30

type CloudsqlInstanceType string

const (
	PostgreSQL CloudsqlInstanceType = "POSTGRES"
	MySQL      CloudsqlInstanceType = "MYSQL"
)

type CloudsqlDbUser struct {
	Database string
	Username string
}

type InitDbUserJobArgs struct {
	User           CloudsqlDbUser
	RootPassword   string
	DBInstance     PostgresDBInstanceArgs
	CloudSQLProxy  *CloudSQLProxy
	Provider       *sdkK8s.Provider
	DBInstanceType CloudsqlInstanceType
}

type InitUserJob struct {
	Name sdk.StringPtrOutput
}

func NewInitDbUserJob(ctx *sdk.Context, stackName string, args InitDbUserJobArgs) (*InitUserJob, error) {
	jobName := fmt.Sprintf("%s-db-user-init", stackName)
	jobCredsName := fmt.Sprintf("%s-creds", jobName)

	// Secret creation
	jobCredsSecret, err := corev1.NewSecret(ctx, jobCredsName, &corev1.SecretArgs{
		Metadata: &v1.ObjectMetaArgs{
			Namespace: sdk.String(stackName),
			Name:      sdk.String(jobCredsName),
		},
		StringData: sdk.StringMap{
			"PGPASSWORD": sdk.String(args.RootPassword),
			"MYSQL_PWD":  sdk.String(args.RootPassword),
		},
	}, sdk.Provider(args.Provider))
	if err != nil {
		return nil, err
	}

	// Job Container creation
	jobContainer := sdk.All(args.User.Database, args.User.Username).ApplyT(func(all []interface{}) (corev1.ContainerArgs, error) {
		database := all[0].(string)
		username := all[1].(string)

		var initScript string
		if args.DBInstanceType == MySQL {
			initScript = `
                set -e;
                apk add --no-cache mysql-client; 
                sleep 20;
                # MySQL-specific logic here
            `
		} else {
			initScript = fmt.Sprintf(`
                set -e;
                apk add --no-cache postgresql-client; 
                sleep 20;
                psql -h localhost -U postgres -d %s -c 'GRANT pg_read_all_data TO "%s";';
                psql -h localhost -U postgres -d %s -c 'GRANT pg_write_all_data TO "%s";';
            `, database, username, database, username)
		}

		return corev1.ContainerArgs{
			Command: sdk.StringArray{
				sdk.String("sh"),
				sdk.String("-c"),
				sdk.String(initScript),
			},
			EnvFrom: corev1.EnvFromSourceArray{
				&corev1.EnvFromSourceArgs{
					SecretRef: &corev1.SecretEnvSourceArgs{
						Name: jobCredsSecret.Metadata.Name().Elem(),
					},
				},
			},
			Image: sdk.String("alpine:latest"),
			Name:  sdk.String("job"),
		}, nil
	}).(corev1.ContainerOutput)

	if err != nil {
		return nil, err
	}

	// Proxy Container creation
	proxyContainer := cloudsqlProxyContainer(args.DBInstance, MaxInitSQLTimeSec)

	// Job creation
	job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
		Metadata: &v1.ObjectMetaArgs{
			Namespace: sdk.String(stackName),
		},
		Spec: &batchv1.JobSpecArgs{
			BackoffLimit: sdk.Int(5),
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					Containers:    corev1.ContainerArray{jobContainer, proxyContainer},
					RestartPolicy: sdk.String("Never"),
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: sdk.String("cloudsql-secret"),
							Secret: &corev1.SecretVolumeSourceArgs{
								SecretName: args.CloudSQLProxy.SqlProxySecret.Metadata.Name(),
							},
						},
					},
				},
			},
		},
	}, sdk.Provider(args.Provider))
	if err != nil {
		return nil, err
	}

	return &InitUserJob{
		Name: job.Metadata.Name(),
	}, nil
}
