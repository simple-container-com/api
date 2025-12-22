package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
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

type deploymentPatchInputs struct {
	Kubeconfig  string
	Namespace   string
	ServiceName string
	Annotations map[string]string
}

func patchDeploymentWithK8sClient(ctx context.Context, inputs deploymentPatchInputs) error {
	// Create Kubernetes client from kubeconfig
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(inputs.Kubeconfig))
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Build the patch payload - only the annotations we want to update
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": inputs.Annotations,
				},
			},
		},
	}

	// Marshal to JSON
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	// Apply the patch using Strategic Merge Patch
	// This is a true partial update that doesn't require full deployment spec
	patchOptions := metav1.PatchOptions{
		FieldManager: "simple-container",
		Force:        ptr.To(true), // Force ownership of fields
	}

	_, err = clientSet.AppsV1().Deployments(inputs.Namespace).Patch(
		ctx,
		inputs.ServiceName,
		types.StrategicMergePatchType,
		patchBytes,
		patchOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to patch deployment %s/%s: %w", inputs.Namespace, inputs.ServiceName, err)
	}

	return nil
}

func PatchDeployment(ctx *sdk.Context, args *DeploymentPatchArgs) (*sdk.StringOutput, error) {
	// Use Pulumi's Apply to execute the native Kubernetes client patch
	// This bypasses Pulumi's DeploymentPatch validation entirely

	// Convert map[string]StringOutput to StringMapOutput for proper resolution
	annotationsOutput := sdk.ToStringMapOutput(args.Annotations)

	// Apply the patch when all outputs are resolved
	// Use ApplyTWithContext to get access to Pulumi's context
	result := sdk.All(args.Kubeconfig, annotationsOutput).ApplyTWithContext(ctx.Context(), func(goCtx context.Context, vals []interface{}) (string, error) {
		kubeconfigStr, ok := vals[0].(string)
		if !ok || kubeconfigStr == "" {
			return "", fmt.Errorf("kubeconfig is required for native Kubernetes client patching")
		}

		annotations, ok := vals[1].(map[string]string)
		if !ok {
			return "", fmt.Errorf("failed to resolve annotations: got type %T", vals[1])
		}

		inputs := deploymentPatchInputs{
			Kubeconfig:  kubeconfigStr,
			Namespace:   args.Namespace,
			ServiceName: args.ServiceName,
			Annotations: annotations,
		}

		// Use Pulumi's context with timeout to respect cancellation and prevent hanging
		patchCtx, cancel := context.WithTimeout(goCtx, 30*time.Second)
		defer cancel()

		if err := patchDeploymentWithK8sClient(patchCtx, inputs); err != nil {
			return "", err
		}

		return fmt.Sprintf("%s/%s patched", args.Namespace, args.ServiceName), nil
	}).(sdk.StringOutput)

	return &result, nil
}
