package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

type GkeAutopilotOut struct {
	Cluster *container.Cluster
	Caddy   *kubernetes.SimpleContainer
}

func GkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	gkeInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotResource)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	containerServiceName := fmt.Sprintf("projects/%s/services/container.googleapis.com", gkeInput.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, containerServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", containerServiceName)
	}
	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	location := gkeInput.Location

	if location == "" {
		return nil, errors.Errorf("`location` must be specified for GKE cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	}

	clusterName := kubernetes.ToClusterName(input, input.Descriptor.Name)
	params.Log.Info(ctx.Context(), "Configuring GKE Autopilot cluster %q in %q", clusterName, input.StackParams.Environment)
	timeouts := sdk.CustomTimeouts{
		Create: "10m",
		Update: "5m",
		Delete: "10m",
	}
	if gkeInput.Timeouts != nil {
		timeouts.Delete = lo.If(gkeInput.Timeouts.Delete != "", gkeInput.Timeouts.Delete).Else(timeouts.Delete)
		timeouts.Update = lo.If(gkeInput.Timeouts.Update != "", gkeInput.Timeouts.Update).Else(timeouts.Update)
		timeouts.Create = lo.If(gkeInput.Timeouts.Create != "", gkeInput.Timeouts.Create).Else(timeouts.Create)
	}
	out := GkeAutopilotOut{}
	cluster, err := container.NewCluster(ctx, clusterName, &container.ClusterArgs{
		EnableAutopilot:  sdk.Bool(true),
		Location:         sdk.String(location),
		Name:             sdk.String(clusterName),
		MinMasterVersion: sdk.String(gkeInput.GkeMinVersion),
		ReleaseChannel: &container.ClusterReleaseChannelArgs{
			Channel: sdk.String("STABLE"),
		},
		IpAllocationPolicy: &container.ClusterIpAllocationPolicyArgs{},
		// because we are using autopilot verticalPodAutoscaling is handled by the GCP
	}, append(opts, sdk.IgnoreChanges([]string{"verticalPodAutoscaling"}), sdk.Timeouts(&timeouts))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create cluster %q in %q", clusterName, input.StackParams.Environment)
	}
	out.Cluster = cluster
	kubeconfig := generateKubeconfig(cluster, gkeInput)
	ctx.Export(toKubeconfigExport(clusterName), kubeconfig)

	if gkeInput.Caddy != nil {
		caddyConfig := kubernetes.CaddyDeployment{
			CaddyConfig:     gkeInput.Caddy,
			ClusterName:     clusterName,
			ClusterResource: cluster,
		}
		caddy, err := kubernetes.DeployCaddyService(ctx, caddyConfig, input, params, kubeconfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create caddy deployment for cluster %q in %q", clusterName, input.StackParams.Environment)
		}
		out.Caddy = caddy
	}

	return &api.ResourceOutput{Ref: out}, nil
}

func toKubeconfigExport(clusterName string) string {
	return fmt.Sprintf("%s-kubeconfig", clusterName)
}

func generateKubeconfig(cluster *container.Cluster, gkeInput *gcloud.GkeAutopilotResource) sdk.StringOutput {
	return sdk.All(cluster.Project, cluster.Name, cluster.Endpoint, cluster.MasterAuth).ApplyT(func(args []any) (string, error) {
		project := args[0].(string)
		name := args[1].(string)
		endpoint := args[2].(string)
		masterAuth := args[3].(container.ClusterMasterAuth)

		context := fmt.Sprintf("%s_%s_%s", project, gkeInput.Zone, name)

		kubeconfig := `
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: ${clusterCaCertificate}
    server: https://${endpoint}
  name: ${context}
contexts:
- context:
    cluster: ${context}
    user: ${context}
  name: ${context}
current-context: ${context}
kind: Config
preferences: {}
users:
- name: ${context}
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: gke-gcloud-auth-plugin
      installHint: Install gke-gcloud-auth-plugin for use with kubectl by following
        https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke
      provideClusterInfo: true
`

		if err := placeholders.New().Apply(&kubeconfig, placeholders.WithData((placeholders.MapData{
			"clusterCaCertificate": lo.FromPtr(masterAuth.ClusterCaCertificate),
			"endpoint":             endpoint,
			"context":              context,
		}))); err != nil {
			return "", errors.Wrapf(err, "failed to apply placeholders on kubeconfig template")
		}

		return kubeconfig, nil
	}).(sdk.StringOutput)
}
