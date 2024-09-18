package gcp

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
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
		caddy, err := deployCaddyService(ctx, input, gkeInput, lo.FromPtr(gkeInput.Caddy), params, kubeconfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create caddy deployment for cluster %q in %q", clusterName, input.StackParams.Environment)
		}
		out.Caddy = caddy
	}

	return &api.ResourceOutput{Ref: out}, nil
}

func deployCaddyService(ctx *sdk.Context, input api.ResourceInput, gkeInput *gcloud.GkeAutopilotResource, caddy gcloud.CaddyConfig, params pApi.ProvisionParams, kubeconfig sdk.StringOutput) (*kubernetes.SimpleContainer, error) {
	params.Log.Info(ctx.Context(), "Configure Caddy deployment for cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	kubeProvider, err := sdkK8s.NewProvider(ctx, fmt.Sprintf("%s-caddy-kubeprovider", input.ToResName(input.Descriptor.Name)), &sdkK8s.ProviderArgs{
		Kubeconfig: kubeconfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q",
			input.StackParams.StackName, input.Descriptor.Name, input.StackParams.Environment)
	}
	deploymentName := "caddy"
	namespace := lo.If(caddy.Namespace != nil, lo.FromPtr(caddy.Namespace)).Else(deploymentName)
	caddyImage := lo.If(caddy.Image != nil, lo.FromPtr(caddy.Image)).Else(fmt.Sprintf("simplecontainer/caddy:%s", build.Version))

	// TODO: provision private bucket for certs storage
	var caddyVolumes []k8s.SimpleTextVolume
	caddyVolumes, err = kubernetes.EmbedFSToTextVolumes(caddyVolumes, Caddyconfig, "embed/caddy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read embedded caddy config files")
	}

	serviceAccountName := input.ToResName(fmt.Sprintf("%s-caddy-sa", input.Descriptor.Name))
	serviceAccount, err := kubernetes.NewSimpleServiceAccount(ctx, serviceAccountName, &kubernetes.SimpleServiceAccountArgs{
		Name:      serviceAccountName,
		Namespace: namespace,
		Resources: []string{"services"},
	}, sdk.Provider(kubeProvider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to crate service account for caddy")
	}
	caddyContainer := k8s.CloudRunContainer{
		Name:    deploymentName,
		Command: []string{deploymentName, "run", "--config", "/tmp/Caddyfile", "--adapter", "caddyfile"},
		Image: api.ContainerImage{
			Name:     caddyImage,
			Platform: api.ImagePlatformLinuxAmd64,
		},
		Secrets: map[string]string{
			"GOOGLE_APPLICATION_CREDENTIALS": "/gcp-credentials.json",
		},
		Ports:    []int{443, 80},
		MainPort: lo.ToPtr(80),
	}
	initContainer := corev1.ContainerArgs{
		Name:  sdk.String("generate-caddyfile"),
		Image: sdk.String("bitnami/kubectl:latest"),
		VolumeMounts: corev1.VolumeMountArray{
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/tmp"),
				Name:      sdk.String("tmp"),
			},
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/etc/caddy/Caddyfile"),
				Name:      sdk.String(kubernetes.ToConfigVolumesName(deploymentName)),
				SubPath:   sdk.String("Caddyfile"),
			},
		},
		Command: sdk.ToStringArray([]string{"bash", "-c", `
	      set -xe;
	      cp -f /etc/caddy/Caddyfile /tmp/Caddyfile;
	      namespaces=$(kubectl get services --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{"\n"}{end}' | uniq)
	      for ns in $namespaces; do
	          echo $ns
	          kubectl get service -n $ns $ns -o jsonpath='{.metadata.annotations.simple-container\.com/caddyfile-entry}' >> /tmp/Caddyfile || true;
	          echo "" >> /tmp/Caddyfile
	      done
	      cat /tmp/Caddyfile
		`}),
	}

	sc, err := kubernetes.DeploySimpleContainer(ctx, kubernetes.Args{
		ServiceType:        lo.ToPtr("LoadBalancer"), // to provision external IP
		Namespace:          namespace,
		DeploymentName:     deploymentName,
		Input:              input,
		ServiceAccountName: lo.ToPtr(serviceAccount.Name),
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
				ImageName: sdk.String(caddyImage).ToStringOutput(),
			},
		},
		Params:                 params,
		InitContainers:         []corev1.ContainerArgs{initContainer},
		KubeProvider:           kubeProvider,
		GenerateCaddyfileEntry: false,
		Annotations: map[string]string{
			"pulumi.com/patchForce": "true",
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for caddy in GKE cluster %q in %q",
			input.Descriptor.Name, input.StackParams.Environment)
	}
	clusterName := toClusterName(input, input.Descriptor.Name)
	ctx.Export(toIngressIpExport(clusterName), sc.ServicePublicIP)
	if caddyJson, err := json.Marshal(caddy); err != nil {
		return nil, errors.Wrapf(err, "failed to marshal caddy config")
	} else {
		ctx.Export(toCaddyConfigExport(clusterName), sdk.String(string(caddyJson)))
	}
	return sc, nil
}

func toIngressIpExport(clusterName string) string {
	return fmt.Sprintf("%s-ingress-ip", clusterName)
}

func toCaddyConfigExport(clusterName string) string {
	return fmt.Sprintf("%s-caddy-config", clusterName)
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
