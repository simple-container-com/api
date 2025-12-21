package kubernetes

import (
	"fmt"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DeploymentPatchArgs struct {
	PatchName    string
	ServiceName  string
	Namespace    string
	Annotations  map[string]sdk.StringOutput
	KubeProvider *sdkK8s.Provider  // Main Kubernetes provider (for dependencies)
	Kubeconfig   *sdk.StringOutput // Optional: Kubeconfig for creating patch-specific provider
	Opts         []sdk.ResourceOption
}

func PatchDeployment(ctx *sdk.Context, args *DeploymentPatchArgs) (*apiextensions.CustomResource, error) {
	var patchProvider sdk.ProviderResource

	// If Kubeconfig is provided, create a dedicated SSA-enabled provider for patches
	// This isolates patch resources from regular resources
	if args.Kubeconfig != nil {
		patchProviderName := fmt.Sprintf("%s-patch-provider", args.PatchName)
		dedicatedProvider, err := sdkK8s.NewProvider(ctx, patchProviderName, &sdkK8s.ProviderArgs{
			Kubeconfig:            *args.Kubeconfig,
			EnableServerSideApply: sdk.BoolPtr(true), // Required for DeploymentPatch resources
		}, sdk.Parent(args.KubeProvider)) // Make it a child of the main provider
		if err != nil {
			return nil, err
		}
		patchProvider = dedicatedProvider
	} else {
		// Use the existing provider (assumes SSA is already enabled or will be enabled)
		patchProvider = args.KubeProvider
	}

	// NOTE: DeploymentPatch requires Server-Side Apply mode
	// SSA allows partial updates without requiring the complete deployment spec
	patchOpts := []sdk.ResourceOption{
		sdk.Provider(patchProvider),      // Use dedicated or existing provider
		sdk.RetainOnDelete(true),         // Don't delete the deployment if patch is removed
		sdk.ReplaceOnChanges([]string{}), // Don't replace, just update
		sdk.DeleteBeforeReplace(false),   // Never delete before replacing
	}

	// Combine patch options with user-provided options
	// Note: Provider option is set first, so if user provides another provider it will be ignored
	allOpts := append(patchOpts, args.Opts...)

	// Use CustomResource with OtherFields to bypass DeploymentPatch validation logic
	// This forces Pulumi to send a raw PATCH request to Kubernetes without schema validation
	// The OtherFields map is sent directly to the API with SSA enabled
	otherFields := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": args.Annotations,
				},
			},
		},
	}

	return apiextensions.NewCustomResource(ctx, args.PatchName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("apps/v1"),
		Kind:       sdk.String("Deployment"),
		Metadata: &metav1.ObjectMetaArgs{
			Namespace: sdk.String(args.Namespace),
			Name:      sdk.String(args.ServiceName),
			Annotations: sdk.StringMap{
				"pulumi.com/patchForce": sdk.String("true"), // Force SSA to resolve conflicts
			},
		},
		OtherFields: otherFields,
	}, allOpts...)
}
