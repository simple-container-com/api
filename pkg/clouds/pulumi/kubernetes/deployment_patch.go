package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DeploymentPatchArgs struct {
	PatchName   string
	ServiceName string
	Namespace   string
	// Annotations are applied to spec.template.metadata — changes here trigger a pod rolling update.
	// Use only for values that should restart pods when changed (e.g. content hashes).
	Annotations map[string]sdk.StringOutput
	// DeploymentAnnotations are applied to metadata only — changes do NOT trigger pod restarts.
	// Use for informational labels (e.g. caddy-updated-at, caddy-updated-by).
	DeploymentAnnotations map[string]sdk.StringOutput
	KubeProvider          *sdkK8s.Provider  // Main Kubernetes provider (for dependencies)
	Kubeconfig            *sdk.StringOutput // Optional: Kubeconfig for creating patch-specific provider
	Opts                  []sdk.ResourceOption
}

type deploymentPatchInputs struct {
	Kubeconfig            string
	Namespace             string
	ServiceName           string
	Annotations           map[string]string
	DeploymentAnnotations map[string]string
}

// buildPodTemplatePatch returns the JSON patch that targets spec.template.metadata.annotations.
// Changes here cause a rolling restart of pods.
func buildPodTemplatePatch(annotations map[string]string) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": annotations,
				},
			},
		},
	})
}

// buildDeploymentMetadataPatch returns the JSON patch that targets metadata.annotations.
// Changes here do NOT trigger pod restarts.
func buildDeploymentMetadataPatch(annotations map[string]string) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": annotations,
		},
	})
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

	patchOptions := metav1.PatchOptions{
		FieldManager: "simple-container",
	}

	// Patch spec.template.metadata.annotations — triggers rolling restart when values change.
	if len(inputs.Annotations) > 0 {
		patchBytes, err := buildPodTemplatePatch(inputs.Annotations)
		if err != nil {
			return fmt.Errorf("failed to marshal pod-template annotations patch: %w", err)
		}

		_, err = clientSet.AppsV1().Deployments(inputs.Namespace).Patch(
			ctx,
			inputs.ServiceName,
			types.StrategicMergePatchType,
			patchBytes,
			patchOptions,
		)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "❌ PATCH ERROR: failed to patch deployment pod-template annotations %s/%s: %v\n", inputs.Namespace, inputs.ServiceName, err)
			return fmt.Errorf("failed to patch deployment %s/%s: %w", inputs.Namespace, inputs.ServiceName, err)
		}
	}

	// Patch metadata.annotations — informational only, does NOT trigger pod restarts.
	if len(inputs.DeploymentAnnotations) > 0 {
		patchBytes, err := buildDeploymentMetadataPatch(inputs.DeploymentAnnotations)
		if err != nil {
			return fmt.Errorf("failed to marshal deployment annotations patch: %w", err)
		}

		_, err = clientSet.AppsV1().Deployments(inputs.Namespace).Patch(
			ctx,
			inputs.ServiceName,
			types.StrategicMergePatchType,
			patchBytes,
			patchOptions,
		)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "❌ PATCH ERROR: failed to patch deployment metadata annotations %s/%s: %v\n", inputs.Namespace, inputs.ServiceName, err)
			return fmt.Errorf("failed to patch deployment metadata annotations %s/%s: %w", inputs.Namespace, inputs.ServiceName, err)
		}
	}

	return nil
}

func PatchDeployment(ctx *sdk.Context, args *DeploymentPatchArgs) (*sdk.StringOutput, error) {
	// Use Pulumi's Apply to execute the native Kubernetes client patch
	// This bypasses Pulumi's DeploymentPatch validation entirely

	// Convert map[string]StringOutput to StringMapOutput for proper resolution
	annotationsOutput := sdk.ToStringMapOutput(args.Annotations)
	deploymentAnnotationsOutput := sdk.ToStringMapOutput(args.DeploymentAnnotations)

	// Apply the patch when all outputs are resolved
	// Use ApplyTWithContext to get access to Pulumi's context
	result := sdk.All(args.Kubeconfig, annotationsOutput, deploymentAnnotationsOutput).ApplyTWithContext(ctx.Context(), func(goCtx context.Context, vals []interface{}) (string, error) {
		kubeconfigStr, ok := vals[0].(string)
		if !ok || kubeconfigStr == "" {
			return "", fmt.Errorf("kubeconfig is required for native Kubernetes client patching")
		}

		annotations, ok := vals[1].(map[string]string)
		if !ok {
			return "", fmt.Errorf("failed to resolve annotations: got type %T", vals[1])
		}

		deploymentAnnotations, ok := vals[2].(map[string]string)
		if !ok {
			return "", fmt.Errorf("failed to resolve deployment annotations: got type %T", vals[2])
		}

		inputs := deploymentPatchInputs{
			Kubeconfig:            kubeconfigStr,
			Namespace:             args.Namespace,
			ServiceName:           args.ServiceName,
			Annotations:           annotations,
			DeploymentAnnotations: deploymentAnnotations,
		}

		// Create a context that respects parent cancellation but allows extra time for patch to complete
		// We use a channel to listen for parent context cancellation, then give the patch operation
		// additional time (5 seconds) to complete before actually cancelling
		patchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Monitor parent context for cancellation
		go func() {
			<-goCtx.Done()
			// Parent context was cancelled, but give patch 5 more seconds to complete
			time.Sleep(5 * time.Second)
			cancel()
		}()

		if err := patchDeploymentWithK8sClient(patchCtx, inputs); err != nil {
			return "", err
		}

		return fmt.Sprintf("%s/%s patched", args.Namespace, args.ServiceName), nil
	}).(sdk.StringOutput)

	return &result, nil
}
