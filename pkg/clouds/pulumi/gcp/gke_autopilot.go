package gcp

import (
	"embed"
	"fmt"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

//go:embed embed/caddy/*
var Caddyconfig embed.FS

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

	clusterName := toClusterName(input, input.Descriptor.Name)
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
		caddy, err := deployCaddyService(ctx, input, gkeInput.Caddy, params, kubeconfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create caddy deployment for cluster %q in %q", clusterName, input.StackParams.Environment)
		}
		out.Caddy = caddy
	}

	return &api.ResourceOutput{Ref: out}, nil
}

func deployCaddyService(ctx *sdk.Context, input api.ResourceInput, caddy *gcloud.CaddyConfig, params pApi.ProvisionParams, kubeconfig sdk.StringOutput) (*kubernetes.SimpleContainer, error) {
	params.Log.Info(ctx.Context(), "Configure Caddy deployment for cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	kubeProvider, err := sdkK8s.NewProvider(ctx, fmt.Sprintf("%s-caddy-kubeprovider", input.ToResName(input.Descriptor.Name)), &sdkK8s.ProviderArgs{
		Kubeconfig: kubeconfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q",
			input.StackParams.StackName, input.Descriptor.Name, input.StackParams.Environment)
	}

	var caddyVolumes []k8s.SimpleTextVolume
	caddyfiles, err := Caddyconfig.ReadDir("embed/caddy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read embedded caddy config files")
	}
	for _, caddyfile := range caddyfiles {
		if content, err := Caddyconfig.ReadFile(path.Join("embed/caddy", caddyfile.Name())); err != nil {
			return nil, errors.Wrapf(err, "failed to read caddy config file %q", caddyfile.Name())
		} else {
			caddyVolumes = append(caddyVolumes, k8s.SimpleTextVolume{
				TextVolume: api.TextVolume{
					Content:   string(content),
					Name:      caddyfile.Name(),
					MountPath: filepath.Join("/etc/caddy", caddyfile.Name()),
				},
			})
		}
	}

	caddyContainer := k8s.CloudRunContainer{
		Name:    "caddy",
		Command: []string{"caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"},
		Image: api.ContainerImage{
			Name:     "caddy:latest",
			Platform: api.ImagePlatformLinuxAmd64,
		},
		Secrets: map[string]string{
			"GOOGLE_APPLICATION_CREDENTIALS": "/gcp-credentials.json",
		},
		Ports: []int{443, 80},
	}

	sc, err := kubernetes.DeploySimpleContainer(ctx, kubernetes.Args{
		Input: input,
		Deployment: k8s.DeploymentConfig{
			StackConfig:      &api.StackConfigCompose{},
			Containers:       []k8s.CloudRunContainer{caddyContainer},
			IngressContainer: &caddyContainer,
			Scale: &k8s.Scale{
				Replicas: lo.If(caddy.Replicas != nil, lo.FromPtr(caddy.Replicas)).Else(1),
			},
			TextVolumes: caddyVolumes,
		},
		Images: []*kubernetes.ContainerImage{
			{
				Container: caddyContainer,
				ImageName: sdk.String("simplecontainer/caddy:latest").ToStringOutput(),
			},
		},
		Params:       params,
		KubeProvider: kubeProvider,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for caddy in GKE cluster %q in %q",
			input.Descriptor.Name, input.StackParams.Environment)
	}
	return sc, nil
}

func toClusterName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
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
