package kubernetes

import (
	"fmt"

	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/db"
	"github.com/simple-container-com/api/pkg/util"
)

type DatabaseUser struct {
	Database string
	Username string
	Password sdk.StringOutput
}

type InitDbUserJobArgs struct {
	User         DatabaseUser
	InstanceName string
	RootUser     string
	RootPassword string
	KubeProvider sdk.ProviderResource
	Namespace    string
	Host         string
	Port         string
	InitSQL      string
	Opts         []sdk.ResourceOption
}

type InitUserJob struct {
	Job sdk.Output
}

func NewPostgresInitDbUserJob(ctx *sdk.Context, stackName string, args InitDbUserJobArgs) (*InitUserJob, error) {
	jobName := util.TrimStringMiddle(fmt.Sprintf("%s-pg-db-user-init", stackName), 60, "-")
	jobCredsName := util.TrimStringMiddle(fmt.Sprintf("%s-creds", jobName), 60, "-")

	opts := args.Opts
	opts = append(opts, sdk.Provider(args.KubeProvider))

	// Secret creation
	jobCredsSecret, err := corev1.NewSecret(ctx, jobCredsName, &corev1.SecretArgs{
		Metadata: &v1.ObjectMetaArgs{
			Namespace: sdk.String(args.Namespace),
			Name:      sdk.String(jobCredsName),
		},
		StringData: sdk.StringMap{
			"PGPASSWORD":  sdk.String(args.RootPassword),
			"DB_NAME":     sdk.String(args.User.Database),
			"DB_USER":     sdk.String(args.User.Username),
			"DB_PASSWORD": args.User.Password,
			"PGHOST":      sdk.String(args.Host),
			"PGPORT":      sdk.String(args.Port),
			"PGUSER":      sdk.String(args.RootUser),
			"PGDATABASE":  sdk.String("postgres"),
			"INIT_SQL":    sdk.String(args.InitSQL),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	// Job Container creation
	jobContainer := corev1.ContainerArgs{
		Command: sdk.StringArray{
			sdk.String("sh"),
			sdk.String("-c"),
			sdk.String(db.PSQL_DB_INIT_SH),
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
	}

	kubeProvider := args.KubeProvider
	namespace := args.Namespace

	// Job creation
	job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
		Metadata: &v1.ObjectMetaArgs{
			Name:      sdk.String(jobName),
			Namespace: sdk.String(namespace),
			Annotations: sdk.StringMap{
				"pulumi.com/patchForce": sdk.String("true"),
			},
		},
		Spec: &batchv1.JobSpecArgs{
			BackoffLimit: sdk.Int(5),
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					Containers:    corev1.ContainerArray{jobContainer},
					RestartPolicy: sdk.String("Never"),
				},
			},
		},
	}, append(opts, sdk.Provider(kubeProvider))...)
	if err != nil {
		return nil, err
	}

	return &InitUserJob{
		Job: job.ToJobOutput(),
	}, nil
}
