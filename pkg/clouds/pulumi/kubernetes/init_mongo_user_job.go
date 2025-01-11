package kubernetes

import (
	"fmt"

	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/util"
)

func NewMongodbInitDbUserJob(ctx *sdk.Context, stackName string, args InitDbUserJobArgs) (*InitUserJob, error) {
	jobName := util.TrimStringMiddle(fmt.Sprintf("%s-mongo-db-user-init", stackName), 60, "-")
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
			"ROOT_PASSWORD": sdk.String(args.RootPassword),
			"DB_NAME":       sdk.String(args.User.Database),
			"DB_USER":       sdk.String(args.User.Username),
			"DB_PASSWORD":   args.User.Password,
			"HOST":          sdk.String(args.Host),
			"PORT":          sdk.String(args.Port),
			"ROOT_USER":     sdk.String(args.RootUser),
			"ROOT_DATABASE": sdk.String("admin"),
			"REPLICA_SET":   sdk.String(args.InstanceName),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	createUserScript := `
set -e;
mongosh "mongodb://${ROOT_USER}:${ROOT_PASSWORD}@${HOST}/${DB_NAME}?authSource=${ROOT_DATABASE}&readPreference=primary&replicaSet=${REPLICA_SET}" \
	--eval "db.createUser({user:'${DB_USER}',pwd:'${DB_PASSWORD}',roles:[{db: '${DB_NAME}', role: 'dbAdmin'}, {db: '${DB_NAME}', role: 'readWrite'}, {db: 'local', role: 'read'}]})"
`
	// Job Container creation
	jobContainer := corev1.ContainerArgs{
		Command: sdk.StringArray{
			sdk.String("sh"),
			sdk.String("-c"),
			sdk.String(createUserScript),
		},
		EnvFrom: corev1.EnvFromSourceArray{
			&corev1.EnvFromSourceArgs{
				SecretRef: &corev1.SecretEnvSourceArgs{
					Name: jobCredsSecret.Metadata.Name().Elem(),
				},
			},
		},
		Image: sdk.String("alpine/mongosh:latest"),
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
			BackoffLimit: sdk.Int(3),
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
