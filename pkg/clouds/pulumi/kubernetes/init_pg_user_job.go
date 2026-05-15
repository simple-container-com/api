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

	// IgnoreChanges("metadata.namespace") — symmetric with PR #255's IgnoreChanges("metadata.name")
	// on the Namespace and the matching GCP CSQL fix in pkg/clouds/pulumi/gcp/. compute_proc_postgres.go
	// derives the namespace via kubernetes.GenerateNamespaceName (which suffixes custom-stack stackEnv
	// after #230); for existing stacks whose state predates #230 the previous value was the parent-shared
	// namespace, so the diff schedules an immutable-namespace Replace that fails because the isolated
	// namespace was never created (the Namespace itself is held parent-shared by #255). Suppress the diff
	// so this Secret + the Job below stay co-located with the consuming pod. Fresh stacks still get the
	// isolated namespace on initial create — IgnoreChanges only suppresses *diff*, not *initial value*.
	nsImmutableOpts := append(opts, sdk.IgnoreChanges([]string{"metadata.namespace"}))

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
	}, nsImmutableOpts...)
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
	}, append(nsImmutableOpts, sdk.Provider(kubeProvider))...)
	if err != nil {
		return nil, err
	}

	return &InitUserJob{
		Job: job.ToJobOutput(),
	}, nil
}
