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
	// Use strategic merge patch to only update pod template annotations
	// This avoids Kubernetes validation errors that require full deployment spec
	ssaOpts := []sdk.ResourceOption{
		sdk.ReplaceOnChanges([]string{}), // Don't replace, just update
	}

	// Combine SSA options with user-provided options
	allOpts := append(ssaOpts, args.Opts...)

	// Only patch the pod template annotations - this is the minimal patch needed
	// to trigger a rolling restart without requiring selector/containers validation
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
