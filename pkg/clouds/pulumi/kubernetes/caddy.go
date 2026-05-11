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
	// Use the same naming convention as the patch operation for consistency
	deploymentName := GenerateCaddyDeploymentName(input.StackParams.Environment)
	namespace := lo.If(caddy.Namespace != nil, lo.FromPtr(caddy.Namespace)).Else("caddy")
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
	// Default catch-all serves a hard 503 instead of a static "welcome" page.
	// Rationale: when all Services with a `simple-container.com/caddyfile-entry`
	// annotation for a given Host vanish (e.g. a cascade-deletion from a
	// namespace Replace gone wrong), the request used to fall through to a
	// `file_server /etc/caddy/pages` block and respond with HTTP 200 + "Default
	// page". External monitoring saw healthy 200s while every backend was gone.
	// 503 + Retry-After makes the absence of routes loud: CDNs fail over,
	// uptime checks alert, oncall sees it.
	defaultCaddyFileEntry := `
  import gzip
  header Cache-Control "no-store"
  header Retry-After "60"
  respond "<!doctype html><meta charset=utf-8><title>503 Service Unavailable</title><style>body{font:20px Helvetica,sans-serif;color:#333;text-align:center;padding:120px}h1{font-size:48px}code{background:#eee;padding:2px 6px;border-radius:3px}</style><h1>503 Service Unavailable</h1><p>No backend route is configured for this host.</p><p>If you are an operator, verify the Service has the <code>simple-container.com/caddyfile-entry</code> annotation and that Caddy has been rolled.</p>" 503 {
    close
  }
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
				// Use dynamic Caddyfile prefix from cloud provider (e.g., GCS storage config with trusted proxies baked in)
				envVars = append(envVars, corev1.EnvVarArgs{
					Name:  sdk.String("CADDYFILE_PREFIX"),
					Value: caddy.CaddyfilePrefixOut.ToStringOutput(),
				})
			} else {
				// Build static Caddyfile prefix from config (non-GKE path)
				trustedBlock, _ := BuildTrustedProxiesBlock(lo.FromPtrOr(caddy.CaddyConfig, k8s.CaddyConfig{}))
				userPrefix := lo.FromPtrOr(caddy.CaddyfilePrefix, "")
				if prefix := BuildCaddyfileGlobalOptions("", trustedBlock, userPrefix); prefix != "" {
					envVars = append(envVars, corev1.EnvVarArgs{
						Name:  sdk.String("CADDYFILE_PREFIX"),
						Value: sdk.String(prefix),
					})
				}
			}

			return envVars
		}(),
		Command: sdk.ToStringArray([]string{"bash", "-c", `
	      set -xeo pipefail;
	      cp -f /etc/caddy/Caddyfile /tmp/Caddyfile;

	      # Inject custom Caddyfile prefix at the top (e.g., GCS storage configuration)
	      if [ -n "$CADDYFILE_PREFIX" ]; then
	        echo "$CADDYFILE_PREFIX" >> /tmp/Caddyfile
	        echo "" >> /tmp/Caddyfile
	      fi

	      # List Services carrying the caddyfile-entry annotation. We also pull
	      # creationTimestamp so we can dedup by site-address with the newest
	      # Service winning — during a Pulumi Replace of a namespace (or Service),
	      # the old and new Services transiently coexist and both carry the same
	      # annotation; without dedup that produced two "http://<domain> { ... }"
	      # blocks and Caddy aborted with "ambiguous site definition".
	      # pipefail is critical here: a flaky kubectl piped into sort would
	      # otherwise yield services="" and the init-container would silently
	      # emit a Caddyfile with only the default block — every domain would
	      # then serve the welcome page from /etc/caddy/pages on the next pod
	      # restart, masquerading as healthy 200s.
	      raw_services=$(kubectl get services --all-namespaces -o jsonpath='{range .items[?(@.metadata.annotations.simple-container\.com/caddyfile-entry)]}{.metadata.creationTimestamp}{" "}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}')
	      services=$(printf '%s' "$raw_services" | sort -r)
          echo "$DEFAULT_ENTRY_START" >> /tmp/Caddyfile
          if [ "$USE_PREFIXES" == "false" ]; then
            echo "$DEFAULT_ENTRY" >> /tmp/Caddyfile
	        echo "}" >> /tmp/Caddyfile
          fi
	      # Dedup state: first non-blank, non-comment line of each annotation is
	      # the site address (e.g. "http://support-payhey.pay.space {") or the
	      # "handle_path /<prefix>*" matcher for prefix routing. Whitespace is
	      # trimmed both sides so an indentation difference can't pass through as
	      # a distinct key. Already-seen keys are skipped — most-recently-created
	      # Service wins via sort -r.
	      seen=$(mktemp)
	      trap 'rm -f "$seen"' EXIT
	      # Process each service that has Caddyfile entry annotation
	      printf '%s\n' "$services" | while read ts ns service; do
	          if [ -n "$ns" ] && [ -n "$service" ]; then
	              entry=$(kubectl get service -n "$ns" "$service" -o jsonpath='{.metadata.annotations.simple-container\.com/caddyfile-entry}' 2>/dev/null || true)
	              if [ -z "$entry" ]; then
	                  continue
	              fi
	              key=$(printf '%s\n' "$entry" | awk '
	                  /^[[:space:]]*$/ { next }
	                  /^[[:space:]]*#/ { next }
	                  { sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, ""); print; exit }
	              ')
	              if [ -n "$key" ] && grep -qFx -- "$key" "$seen" 2>/dev/null; then
	                  echo "Skipping duplicate caddyfile-entry '$key' from $ns/$service (older Service)"
	                  continue
	              fi
	              [ -n "$key" ] && printf '%s\n' "$key" >> "$seen"
	              echo "Processing service: $service in namespace: $ns"
	              printf '%s\n' "$entry" >> /tmp/Caddyfile
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
		ServiceType:                   serviceType, // to provision external IP
		ExternalTrafficPolicy:         lo.FromPtr(caddy.CaddyConfig).ExternalTrafficPolicy,
		ProvisionIngress:              caddy.ProvisionIngress,
		UseSSL:                        useSSL,
		Namespace:                     namespace,
		DeploymentName:                deploymentName,
		Input:                         input,
		ServiceAccountName:            lo.ToPtr(serviceAccount.Name),
		Deployment:                    deploymentConfig,
		SecretVolumes:                 caddy.SecretVolumes,       // Cloud credentials volumes (e.g., GCP service account)
		SecretVolumeOutputs:           caddy.SecretVolumeOutputs, // Pulumi outputs for secret volumes
		SecretEnvs:                    secretEnvs,                // Secret environment variables
		VPA:                           caddy.VPA,                 // Vertical Pod Autoscaler configuration for Caddy
		TerminationGracePeriodSeconds: lo.FromPtr(caddy.CaddyConfig).TerminationGracePeriodSeconds,
		PreStopSleepSeconds:           lo.FromPtr(caddy.CaddyConfig).PreStopSleepSeconds,
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
