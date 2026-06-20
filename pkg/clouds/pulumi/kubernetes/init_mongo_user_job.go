// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

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
	// Sanitize stack name to comply with Kubernetes RFC 1123 requirements (no underscores)
	sanitizedStackName := SanitizeK8sName(stackName)
	jobName := util.TrimStringMiddle(fmt.Sprintf("%s-mongo-db-user-init", sanitizedStackName), 60, "-")
	jobCredsName := util.TrimStringMiddle(fmt.Sprintf("%s-creds", jobName), 60, "-")

	opts := args.Opts
	opts = append(opts, sdk.Provider(args.KubeProvider))

	// Secret creation
	jobCredsSecret, err := corev1.NewSecret(ctx, jobCredsName, &corev1.SecretArgs{
		Metadata: &v1.ObjectMetaArgs{
			Namespace: args.Namespace,
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
	// Idempotent user provisioning: db.createUser errors with code 51003 (DuplicateKey)
	// if the user already exists, which would break any re-run of this Job — including
	// the Replace that happens when a consumer follows #255's documented opt-in
	// namespace migration (`pulumi stack export | jq 'del(...Namespace urn...)' |
	// pulumi stack import`). Use createUser-or-updateUser semantics so the Job
	// succeeds on both the first and any subsequent run. Matches the idempotency
	// guarantees the postgres init scripts already provide via `IF NOT EXISTS`
	// guards (pkg/clouds/pulumi/db/constants.go) and `GRANT` idempotency
	// (pkg/clouds/pulumi/gcp/init_pg_user_job.go).
	//
	// Credentials read from process.env inside the mongosh eval rather than shell
	// interpolation. Pulling them via env (a) bypasses shell quoting entirely so
	// passwords containing spaces/quotes/$ don't bash-word-split or break JS
	// parsing, (b) keeps the secret out of `ps`/strace visibility on the
	// command line. The connection URI itself still has to be shell-interpolated
	// because mongosh consumes it as its first positional argument, but the user
	// password is the higher-risk value and is now end-to-end env-bound.
	createUserScript := `
set -e;
mongosh "mongodb://${ROOT_USER}:${ROOT_PASSWORD}@${HOST}/${DB_NAME}?authSource=${ROOT_DATABASE}&readPreference=primary&replicaSet=${REPLICA_SET}" --eval '
  var dbName = process.env.DB_NAME;
  var dbUser = process.env.DB_USER;
  var dbPwd  = process.env.DB_PASSWORD;
  var roles = [
    {db: dbName, role: "dbAdmin"},
    {db: dbName, role: "readWrite"},
    {db: "local", role: "read"}
  ];
  if (db.getUser(dbUser) === null) {
    db.createUser({user: dbUser, pwd: dbPwd, roles: roles});
  } else {
    db.updateUser(dbUser, {pwd: dbPwd, roles: roles});
  }
'
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

	// Job creation
	job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
		Metadata: &v1.ObjectMetaArgs{
			Name:      sdk.String(jobName),
			Namespace: args.Namespace,
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
