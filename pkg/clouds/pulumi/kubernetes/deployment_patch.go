package kubernetes

import (
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DeploymentPatchArgs struct {
	PatchName   string
	ServiceName string
	Namespace   string
	Annotations map[string]sdk.StringOutput
	Opts        []sdk.ResourceOption
}

func PatchDeployment(ctx *sdk.Context, args *DeploymentPatchArgs) (*appsv1.DeploymentPatch, error) {
	// Add SSA options to handle field manager conflicts
	// This forces Pulumi to take ownership of conflicting fields
	ssaOpts := []sdk.ResourceOption{
		sdk.ReplaceOnChanges([]string{}), // Don't replace, just update
	}

	// Combine SSA options with user-provided options
	allOpts := append(ssaOpts, args.Opts...)

	return appsv1.NewDeploymentPatch(ctx, args.PatchName, &appsv1.DeploymentPatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Namespace: sdk.String(args.Namespace),
			Name:      sdk.String(args.ServiceName),
			Annotations: sdk.StringMap{
				"pulumi.com/patchForce": sdk.String("true"), // Force SSA to resolve conflicts
			},
		},
		Spec: &appsv1.DeploymentSpecPatchArgs{
			Template: &v1.PodTemplateSpecPatchArgs{
				Metadata: &metav1.ObjectMetaPatchArgs{
					Annotations: sdk.ToStringMapOutput(args.Annotations),
				},
			},
		},
	}, allOpts...)
}
