package kubernetes

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type CaddyDeployment struct {
	*k8s.CaddyConfig
	ClusterName     string
	ClusterResource sdk.Resource
}

func CaddyResource(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeCaddy {
		return nil, errors.Errorf("unsupported caddy type %q", input.Descriptor.Type)
	}

	caddyCfg, ok := input.Descriptor.Config.Config.(*k8s.CaddyResource)
	if !ok {
		return nil, errors.Errorf("failed to convert caddy config for %q", input.Descriptor.Type)
	}

	params.Log.Info(ctx.Context(), "Deploying caddy service...")
	caddyService, err := DeployCaddyService(ctx, CaddyDeployment{
		CaddyConfig: caddyCfg.CaddyConfig,
		ClusterName: input.Descriptor.Name,
	}, input, params, sdk.String(caddyCfg.Kubeconfig).ToStringOutput())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to deploy caddy service for stack %q", stack.Name)
	}

	return &api.ResourceOutput{
		Ref: caddyService,
	}, nil
}

func DeployCaddyService(ctx *sdk.Context, caddy CaddyDeployment, input api.ResourceInput, params pApi.ProvisionParams, kubeconfig sdk.StringOutput) (*SimpleContainer, error) {
	params.Log.Info(ctx.Context(), "Configure Caddy deployment for cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	kubeProvider, err := sdkK8s.NewProvider(ctx, fmt.Sprintf("%s-caddy-kubeprovider", input.ToResName(input.Descriptor.Name)), &sdkK8s.ProviderArgs{
		Kubeconfig: kubeconfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q",
			input.StackParams.StackName, input.Descriptor.Name, input.StackParams.Environment)
	}
	deploymentName := input.ToResName("caddy")
	namespace := lo.If(caddy.Namespace != nil, lo.FromPtr(caddy.Namespace)).Else(deploymentName)
	caddyImage := lo.If(caddy.Image != nil, lo.FromPtr(caddy.Image)).Else(fmt.Sprintf("simplecontainer/caddy:%s", build.Version))

	// TODO: provision private bucket for certs storage
	var caddyVolumes []k8s.SimpleTextVolume
	caddyVolumes, err = EmbedFSToTextVolumes(caddyVolumes, Caddyconfig, "embed/caddy", "/etc/caddy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read embedded caddy config files")
	}

	defaultCaddyFileEntryStart := `http:// {`
	defaultCaddyFileEntry := `
  import gzip
  import handle_static
  root * /etc/caddy/pages
  file_server
`
	// if caddy must respect SSL connections only
	useSSL := caddy.UseSSL == nil || *caddy.UseSSL
	if useSSL {
		defaultCaddyFileEntry += "\nimport hsts"
	}

	serviceAccountName := input.ToResName(fmt.Sprintf("%s-caddy-sa", input.Descriptor.Name))
	serviceAccount, err := NewSimpleServiceAccount(ctx, serviceAccountName, &SimpleServiceAccountArgs{
		Name:      serviceAccountName,
		Namespace: namespace,
		Resources: []string{"services"},
	}, sdk.Provider(kubeProvider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to crate service account for caddy")
	}
	caddyContainer := k8s.CloudRunContainer{
		Name:    deploymentName,
		Command: []string{"caddy", "run", "--config", "/tmp/Caddyfile", "--adapter", "caddyfile"},
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
		Image: sdk.String("simplecontainer/kubectl:latest"),
		VolumeMounts: corev1.VolumeMountArray{
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/tmp"),
				Name:      sdk.String("tmp"),
			},
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/etc/caddy/Caddyfile"),
				Name:      sdk.String(ToConfigVolumesName(deploymentName)),
				SubPath:   sdk.String("Caddyfile"),
			},
		},
		Env: corev1.EnvVarArray{
			corev1.EnvVarArgs{
				Name:  sdk.String("DEFAULT_ENTRY_START"),
				Value: sdk.String(defaultCaddyFileEntryStart),
			},
			corev1.EnvVarArgs{
				Name:  sdk.String("DEFAULT_ENTRY"),
				Value: sdk.String(defaultCaddyFileEntry),
			},
			corev1.EnvVarArgs{
				Name:  sdk.String("USE_PREFIXES"),
				Value: sdk.String(fmt.Sprintf("%t", caddy.UsePrefixes)),
			},
		},
		Command: sdk.ToStringArray([]string{"bash", "-c", `
	      set -xe;
	      cp -f /etc/caddy/Caddyfile /tmp/Caddyfile;
	      namespaces=$(kubectl get services --all-namespaces -o jsonpath='{range .items[*]}{.metadata.namespace}{"\n"}{end}' | uniq)
          echo "$DEFAULT_ENTRY_START" >> /tmp/Caddyfile
          if [ "$USE_PREFIXES" == "false" ]; then
            echo "$DEFAULT_ENTRY" >> /tmp/Caddyfile
	        echo "}" >> /tmp/Caddyfile
          fi
	      for ns in $namespaces; do
	          echo $ns
	          kubectl get service -n $ns $ns -o jsonpath='{.metadata.annotations.simple-container\.com/caddyfile-entry}' >> /tmp/Caddyfile || true;
	          echo "" >> /tmp/Caddyfile
	      done
          if [ "$USE_PREFIXES" == "true" ]; then
            echo "$DEFAULT_ENTRY" >> /tmp/Caddyfile
	        echo "}" >> /tmp/Caddyfile
          fi
          echo "" >> /tmp/Caddyfile
	      cat /tmp/Caddyfile
		`}),
	}

	var addOpts []sdk.ResourceOption
	if caddy.ClusterResource != nil {
		addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{caddy.ClusterResource}))
	}
	serviceType := lo.ToPtr("LoadBalancer")
	if lo.FromPtr(caddy.CaddyConfig).ServiceType != nil {
		serviceType = lo.FromPtr(caddy.CaddyConfig).ServiceType
	}
	sc, err := DeploySimpleContainer(ctx, Args{
		ServiceType:        serviceType, // to provision external IP
		ProvisionIngress:   caddy.ProvisionIngress,
		UseSSL:             useSSL,
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
		Images: []*ContainerImage{
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
	}, addOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for caddy in GKE cluster %q in %q",
			input.Descriptor.Name, input.StackParams.Environment)
	}
	clusterName := ToClusterName(input, caddy.ClusterName)
	ctx.Export(ToIngressIpExport(clusterName), sc.Service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
			params.Log.Warn(ctx.Context(), "failed to export ingress IP: load balancer is nil and there is no ingress IP found")
			return ""
		}
		ip := lo.FromPtr(status.LoadBalancer.Ingress[0].Ip)
		params.Log.Info(ctx.Context(), "load balancer ip is %v", ip)
		return ip
	}))
	// Only marshal the CaddyConfig, not the entire CaddyDeployment struct
	// to avoid marshaling ClusterResource which contains Pulumi outputs
	if caddyJson, err := json.Marshal(caddy.CaddyConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to marshal caddy config")
	} else {
		ctx.Export(ToCaddyConfigExport(clusterName), sdk.String(string(caddyJson)))
	}
	return sc, nil
}

func ToIngressIpExport(clusterName string) string {
	return fmt.Sprintf("%s-ingress-ip", clusterName)
}

func ToClusterName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}
