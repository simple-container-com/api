package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

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
	ClusterName        string
	ClusterResource    sdk.Resource
	CaddyfilePrefixOut sdk.StringOutput // Dynamic Caddyfile prefix for cloud storage (e.g., GCS config from GCP)
	// Cloud-specific configuration (following SimpleContainer patterns)
	SecretEnvs          map[string]string      // Secret environment variables (e.g., GOOGLE_APPLICATION_CREDENTIALS=/etc/gcp/credentials.json)
	SecretVolumes       []k8s.SimpleTextVolume // Secret volumes to mount (e.g., GCP credentials)
	SecretVolumeOutputs []any                  // Pulumi outputs for secret volumes
}

// isPulumiOutputSet checks if a Pulumi StringOutput has been initialized with a value
// This is safer than directly checking ElementType() on potentially uninitialized outputs
func isPulumiOutputSet(output sdk.StringOutput) bool {
	// Check if the output's element type is set (indicates it was initialized)
	return output.ElementType() != nil
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

	// Generate volume names using the same logic as SimpleContainer to ensure consistency
	// This fixes the volume mount name mismatch issue for custom stacks
	// Use the deploymentName (which includes environment suffix) as the service name
	parentEnv := input.StackParams.ParentEnv
	stackEnv := input.StackParams.Environment
	serviceName := sanitizeK8sName(deploymentName)
	volumesCfgName := generateConfigVolumesName(serviceName, stackEnv, parentEnv)

	// Prepare Caddy volumes (embedded config)
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

	// Build Caddy container configuration
	caddyContainer := k8s.CloudRunContainer{
		Name:    deploymentName,
		Command: []string{"caddy", "run", "--config", "/tmp/Caddyfile", "--adapter", "caddyfile"},
		Image: api.ContainerImage{
			Name:     caddyImage,
			Platform: api.ImagePlatformLinuxAmd64,
		},
		Ports:     []int{443, 80},
		MainPort:  lo.ToPtr(80),
		Resources: caddy.Resources, // Use custom resources if specified, otherwise defaults will be applied
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
				Name:      sdk.String(volumesCfgName),
				SubPath:   sdk.String("Caddyfile"),
			},
		},
		Env: func() corev1.EnvVarArray {
			envVars := corev1.EnvVarArray{
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
			}

			// Add Caddyfile prefix - prefer dynamic output over static config
			if isPulumiOutputSet(caddy.CaddyfilePrefixOut) {
				// Use dynamic Caddyfile prefix from cloud provider (e.g., GCS storage config)
				envVars = append(envVars, corev1.EnvVarArgs{
					Name:  sdk.String("CADDYFILE_PREFIX"),
					Value: caddy.CaddyfilePrefixOut.ToStringOutput(),
				})
			} else if caddy.CaddyfilePrefix != nil {
				// Use static Caddyfile prefix from config
				envVars = append(envVars, corev1.EnvVarArgs{
					Name:  sdk.String("CADDYFILE_PREFIX"),
					Value: sdk.String(lo.FromPtr(caddy.CaddyfilePrefix)),
				})
			}

			return envVars
		}(),
		Command: sdk.ToStringArray([]string{"bash", "-c", `
	      set -xe;
	      cp -f /etc/caddy/Caddyfile /tmp/Caddyfile;
	      
	      # Inject custom Caddyfile prefix at the top (e.g., GCS storage configuration)
	      if [ -n "$CADDYFILE_PREFIX" ]; then
	        echo "$CADDYFILE_PREFIX" >> /tmp/Caddyfile
	        echo "" >> /tmp/Caddyfile
	      fi
	      
	      # Get all services with Simple Container annotations across all namespaces
	      services=$(kubectl get services --all-namespaces -o jsonpath='{range .items[?(@.metadata.annotations.simple-container\.com/caddyfile-entry)]}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}')
          echo "$DEFAULT_ENTRY_START" >> /tmp/Caddyfile
          if [ "$USE_PREFIXES" == "false" ]; then
            echo "$DEFAULT_ENTRY" >> /tmp/Caddyfile
	        echo "}" >> /tmp/Caddyfile
          fi
	      # Process each service that has Caddyfile entry annotation
	      echo "$services" | while read ns service; do
	          if [ -n "$ns" ] && [ -n "$service" ]; then
	              echo "Processing service: $service in namespace: $ns"
	              kubectl get service -n $ns $service -o jsonpath='{.metadata.annotations.simple-container\.com/caddyfile-entry}' >> /tmp/Caddyfile || true;
	              echo "" >> /tmp/Caddyfile
	          fi
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

	// Prepare deployment config
	deploymentConfig := k8s.DeploymentConfig{
		StackConfig:      &api.StackConfigCompose{Env: make(map[string]string)},
		Containers:       []k8s.CloudRunContainer{caddyContainer},
		IngressContainer: &caddyContainer,
		Scale: &k8s.Scale{
			Replicas: lo.If(caddy.Replicas != nil, lo.FromPtr(caddy.Replicas)).Else(1),
		},
		TextVolumes: caddyVolumes,
	}

	// Prepare secret environment variables (e.g., GOOGLE_APPLICATION_CREDENTIALS for GCP)
	secretEnvs := make(map[string]string)
	if len(caddy.SecretEnvs) > 0 {
		// Log secret environment variable names (not values for security)
		secretEnvNames := make([]string, 0, len(caddy.SecretEnvs))
		for k, v := range caddy.SecretEnvs {
			secretEnvs[k] = v
			secretEnvNames = append(secretEnvNames, k)
		}
		params.Log.Info(ctx.Context(), "🔐 Adding %d secret environment variables to Caddy: %v", len(caddy.SecretEnvs), secretEnvNames)
	}

	sc, err := DeploySimpleContainer(ctx, Args{
		ServiceType:         serviceType, // to provision external IP
		ProvisionIngress:    caddy.ProvisionIngress,
		UseSSL:              useSSL,
		Namespace:           namespace,
		DeploymentName:      deploymentName,
		Input:               input,
		ServiceAccountName:  lo.ToPtr(serviceAccount.Name),
		Deployment:          deploymentConfig,
		SecretVolumes:       caddy.SecretVolumes,       // Cloud credentials volumes (e.g., GCP service account)
		SecretVolumeOutputs: caddy.SecretVolumeOutputs, // Pulumi outputs for secret volumes
		SecretEnvs:          secretEnvs,                // Secret environment variables
		VPA:                 caddy.VPA,                 // Vertical Pod Autoscaler configuration for Caddy
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
	// Smart cluster name handling: avoid double environment suffix
	clusterName := caddy.ClusterName
	env := input.StackParams.Environment
	if env != "" && !strings.HasSuffix(clusterName, "--"+env) {
		// ClusterName doesn't have environment suffix, add it (Case 2: CaddyResource)
		clusterName = ToClusterName(input, caddy.ClusterName)
	}
	// Otherwise, use clusterName as-is (Case 1: GKE Autopilot - already has suffix)
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
