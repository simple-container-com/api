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
	// Use server-side apply patch to avoid validation issues with incomplete deployments
	patchOpts := append(args.Opts,
		sdk.IgnoreChanges([]string{"spec.selector", "spec.template.metadata.labels", "spec.template.spec"}),
	)

	return appsv1.NewDeploymentPatch(ctx, args.PatchName, &appsv1.DeploymentPatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Namespace: sdk.String(args.Namespace),
			Name:      sdk.String(args.ServiceName),
		},
		Spec: &appsv1.DeploymentSpecPatchArgs{
			Template: &v1.PodTemplateSpecPatchArgs{
				Metadata: &metav1.ObjectMetaPatchArgs{
					Annotations: sdk.ToStringMapOutput(args.Annotations),
				},
			},
		},
	}, patchOpts...)
}
