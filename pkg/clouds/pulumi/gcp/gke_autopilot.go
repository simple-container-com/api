package gcp

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/storage"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

type GkeAutopilotOut struct {
	Cluster *container.Cluster
	Caddy   *pulumiKubernetes.SimpleContainer

	// Cloud NAT resources (optional)
	StaticIp *compute.Address
	Router   *compute.Router
	Nat      *compute.RouterNat
}

func GkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported resource type %q", input.Descriptor.Type)
	}

	gkeInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotResource)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	// Handle resource adoption - exit early if adopting
	if gkeInput.Adopt {
		return AdoptGkeAutopilot(ctx, stack, input, params)
	}

	containerServiceName := fmt.Sprintf("projects/%s/services/container.googleapis.com", gkeInput.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, containerServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", containerServiceName)
	}
	var opts []sdk.ResourceOption
	if params.Provider != nil {
		opts = append(opts, sdk.Provider(params.Provider))
	}

	location := gkeInput.Location

	if location == "" {
		return nil, errors.Errorf("`location` must be specified for GKE cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	}

	clusterName := pulumiKubernetes.ToClusterName(input, input.Descriptor.Name)
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

	// Setup Cloud NAT if external egress IP is enabled
	if gkeInput.ExternalEgressIp != nil && gkeInput.ExternalEgressIp.Enabled {
		if err := gkeInput.ExternalEgressIp.Validate(); err != nil {
			return nil, errors.Wrapf(err, "invalid external egress IP configuration")
		}

		if err := setupCloudNAT(ctx, gkeInput, clusterName, location, cluster, &out, opts, params); err != nil {
			return nil, errors.Wrapf(err, "failed to setup Cloud NAT for cluster %q", clusterName)
		}

		// Create Egress NAT Policy to allow Cloud NAT for external traffic
		if err := createEgressNATPolicy(ctx, gkeInput, clusterName, cluster, opts, params); err != nil {
			return nil, errors.Wrapf(err, "failed to create Egress NAT Policy for cluster %q", clusterName)
		}
	}

	if gkeInput.Caddy != nil {
		// Provision GCS bucket and service account for Caddy ACME certificate storage
		bucket, credentialsJSON, err := provisionCaddyACMEStorage(ctx, clusterName, gkeInput.ProjectId, location, opts, params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision ACME storage for Caddy in cluster %q", clusterName)
		}

		// Build Caddyfile prefix with GCS storage configuration
		// Merge with user-provided prefix if it exists
		caddyfilePrefix := bucket.Name.ApplyT(func(bucketName string) string {
			gcsStorageConfig := fmt.Sprintf(`{
  storage gcs {
    bucket-name %s
  }
}`, bucketName)

			// If user provided custom prefix, merge it
			if gkeInput.Caddy.CaddyfilePrefix != nil && *gkeInput.Caddy.CaddyfilePrefix != "" {
				// User prefix first, then GCS storage config
				return fmt.Sprintf("%s\n\n%s", gcsStorageConfig, *gkeInput.Caddy.CaddyfilePrefix)
			}

			return gcsStorageConfig
		}).(sdk.StringOutput)

		// Prepare GCP credentials as a secret volume output (Pulumi output)
		// SimpleContainer will create the Kubernetes Secret automatically
		// Note: With SubPath mounting, the file is mounted directly at MountPath (not MountPath/Name)
		credentialsMountPath := "/etc/gcp-credentials/credentials.json"
		gcpCredentialsVolume := credentialsJSON.ApplyT(func(creds string) interface{} {
			return k8s.SimpleTextVolume{
				TextVolume: api.TextVolume{
					Name:      "credentials.json",   // Filename within the Kubernetes secret
					Content:   creds,                // GCP service account JSON
					MountPath: credentialsMountPath, // File will be mounted directly here (SubPath behavior)
				},
			}
		})

		params.Log.Info(ctx.Context(), "🔐 Preparing GCP credentials secret volume for Caddy ACME storage at %s", credentialsMountPath)

		// Build Caddy deployment configuration with GCS storage
		caddyConfig := pulumiKubernetes.CaddyDeployment{
			CaddyConfig:        gkeInput.Caddy,
			ClusterName:        clusterName,
			ClusterResource:    cluster,
			CaddyfilePrefixOut: caddyfilePrefix, // Caddyfile with GCS storage config
			// Pass GCP credentials as secret volume output (SimpleContainer will create the K8s Secret)
			SecretVolumeOutputs: []any{gcpCredentialsVolume},
			// Set environment variable pointing to mounted credentials
			SecretEnvs: map[string]string{
				"GOOGLE_APPLICATION_CREDENTIALS": credentialsMountPath,
			},
		}

		caddy, err := pulumiKubernetes.DeployCaddyService(ctx, caddyConfig, input, params, kubeconfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create caddy deployment for cluster %q in %q", clusterName, input.StackParams.Environment)
		}
		out.Caddy = caddy
	}

	return &api.ResourceOutput{Ref: out}, nil
}

// convertClusterLocationToBucketLocation converts GKE cluster location to GCS bucket location
// GKE locations can be zones (us-central1-a) or regions (us-central1)
// GCS buckets should use regions for single-region buckets
func convertClusterLocationToBucketLocation(clusterLocation string) string {
	// Check if location is a zone (has zone suffix like -a, -b, -c)
	// Zones follow pattern: {region}-{zone} (e.g., us-central1-a)
	// Regions follow pattern: {continent}-{area}{number} (e.g., us-central1, europe-west1)

	// Count dashes to determine if it's a zone or region
	// Region: 2 dashes (us-central1, europe-west1)
	// Zone: 3 dashes (us-central1-a, europe-west1-b)
	lastDashIndex := -1
	dashCount := 0
	for i := len(clusterLocation) - 1; i >= 0; i-- {
		if clusterLocation[i] == '-' {
			dashCount++
			if lastDashIndex == -1 {
				lastDashIndex = i
			}
		}
	}

	// If it's a zone (has zone suffix), extract the region part
	if dashCount >= 2 && lastDashIndex > 0 && lastDashIndex < len(clusterLocation)-1 {
		// Check if the part after the last dash is a single letter (zone indicator)
		zonePart := clusterLocation[lastDashIndex+1:]
		if len(zonePart) == 1 && zonePart >= "a" && zonePart <= "z" {
			// It's a zone, return the region part (everything before the last dash)
			return clusterLocation[:lastDashIndex]
		}
	}

	// It's already a region or a multi-region location, return as-is
	return clusterLocation
}

// provisionCaddyACMEStorage provisions a GCS bucket and service account for Caddy ACME certificate storage
func provisionCaddyACMEStorage(ctx *sdk.Context, clusterName, projectID, clusterLocation string, opts []sdk.ResourceOption, params pApi.ProvisionParams) (*storage.Bucket, sdk.StringOutput, error) {
	bucketName := fmt.Sprintf("%s-caddy-acme", clusterName)

	// Convert cluster location to GCS bucket location
	// GKE location can be a zone (e.g., "us-central1-a") or region (e.g., "us-central1")
	// GCS buckets need a region or multi-region location
	bucketLocation := convertClusterLocationToBucketLocation(clusterLocation)

	params.Log.Info(ctx.Context(), "📦 Provisioning GCS bucket %q in location %q for Caddy ACME certificate storage", bucketName, bucketLocation)

	// Provision GCS bucket for ACME data
	bucket, err := storage.NewBucket(ctx, bucketName, &storage.BucketArgs{
		Name:     sdk.String(bucketName),
		Location: sdk.String(bucketLocation),
		LifecycleRules: storage.BucketLifecycleRuleArray{
			&storage.BucketLifecycleRuleArgs{
				Action: &storage.BucketLifecycleRuleActionArgs{
					Type: sdk.String("Delete"),
				},
				Condition: &storage.BucketLifecycleRuleConditionArgs{
					Age: sdk.Int(90), // Delete old certificate data after 90 days
				},
			},
		},
	}, opts...)
	if err != nil {
		return nil, sdk.StringOutput{}, errors.Wrapf(err, "failed to provision GCS bucket for Caddy ACME storage")
	}

	params.Log.Info(ctx.Context(), "🔐 Creating service account for Caddy GCS bucket access")

	// Create service account for Caddy to access GCS bucket
	saName := fmt.Sprintf("%s-caddy-sa", clusterName)
	sa, err := NewServiceAccount(ctx, saName, ServiceAccountArgs{
		Project:     projectID,
		Description: "Service account for Caddy to access GCS bucket for ACME certificate storage",
		Roles: []string{
			"roles/storage.objectAdmin", // Full access to bucket objects
		},
	}, opts...)
	if err != nil {
		return nil, sdk.StringOutput{}, errors.Wrapf(err, "failed to create service account for Caddy")
	}

	params.Log.Info(ctx.Context(), "✅ GCS bucket and service account provisioned successfully")

	// Decode base64-encoded service account key to get actual JSON credentials
	// GCP's ServiceAccountKey.PrivateKey is base64-encoded, but we need the raw JSON
	credentialsJSON := sa.ServiceAccountKey.PrivateKey.ApplyT(func(base64Key string) (string, error) {
		decoded, err := base64.StdEncoding.DecodeString(base64Key)
		if err != nil {
			return "", errors.Wrapf(err, "failed to decode service account key")
		}
		return string(decoded), nil
	}).(sdk.StringOutput)

	return bucket, credentialsJSON, nil
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

// setupCloudNAT creates Cloud NAT resources for static egress IP
func setupCloudNAT(
	ctx *sdk.Context,
	gkeInput *gcloud.GkeAutopilotResource,
	clusterName string,
	location string,
	cluster *container.Cluster,
	out *GkeAutopilotOut,
	opts []sdk.ResourceOption,
	params pApi.ProvisionParams,
) error {
	// Extract region from location (handle both regional and zonal locations)
	region := extractRegionFromLocation(location)

	params.Log.Info(ctx.Context(), "🌐 Setting up Cloud NAT for static egress IP in region %s", region)

	// Validate that cluster and NAT will be in the same region
	cluster.Location.ApplyT(func(clusterLocation string) error {
		clusterRegion := extractRegionFromLocation(clusterLocation)
		if clusterRegion != region {
			params.Log.Warn(ctx.Context(), "⚠️ Region mismatch: cluster in %s, NAT in %s", clusterRegion, region)
		} else {
			params.Log.Info(ctx.Context(), "✅ Cluster and NAT both in region %s", region)
		}
		return nil
	})

	// Step 1: Create or reference static IP
	staticIp, err := createOrReferenceStaticIp(ctx, gkeInput.ExternalEgressIp, clusterName, region, opts, params)
	if err != nil {
		return errors.Wrap(err, "failed to create or reference static IP")
	}
	out.StaticIp = staticIp

	// Step 2: Create Cloud Router (using cluster's VPC network)
	router, err := createCloudRouter(ctx, clusterName, region, cluster, opts, params)
	if err != nil {
		return errors.Wrap(err, "failed to create Cloud Router")
	}
	out.Router = router

	// Step 3: Create Cloud NAT (configured for cluster's specific subnet)
	nat, err := createCloudNat(ctx, clusterName, router, staticIp, region, cluster, opts, params)
	if err != nil {
		return errors.Wrap(err, "failed to create Cloud NAT")
	}
	out.Nat = nat

	// Export static IP address for external reference
	ctx.Export(fmt.Sprintf("%s-egress-ip-address", clusterName), staticIp.Address)
	ctx.Export(fmt.Sprintf("%s-egress-ip-name", clusterName), staticIp.Name)

	params.Log.Info(ctx.Context(), "✅ Cloud NAT configured successfully with static egress IP")

	return nil
}

// createOrReferenceStaticIp creates a new static IP or references an existing one
func createOrReferenceStaticIp(
	ctx *sdk.Context,
	config *gcloud.ExternalEgressIpConfig,
	clusterName string,
	region string,
	opts []sdk.ResourceOption,
	params pApi.ProvisionParams,
) (*compute.Address, error) {
	if config.Existing != "" {
		// Use existing static IP
		params.Log.Info(ctx.Context(), "🔗 Using existing static IP: %s", config.Existing)

		// Parse the existing IP reference to extract name
		// Format: projects/{project}/regions/{region}/addresses/{name}
		parts := strings.Split(config.Existing, "/")
		if len(parts) != 6 {
			return nil, errors.Errorf("invalid existing static IP reference format: %s", config.Existing)
		}
		addressName := parts[5]

		return compute.GetAddress(ctx, addressName, sdk.ID(config.Existing), nil, opts...)
	} else {
		// Create new static IP automatically
		staticIpName := fmt.Sprintf("%s-egress-ip", clusterName)
		params.Log.Info(ctx.Context(), "📍 Creating static IP address: %s", staticIpName)

		return compute.NewAddress(ctx, staticIpName, &compute.AddressArgs{
			Name:        sdk.String(staticIpName),
			Region:      sdk.String(region),
			AddressType: sdk.String("EXTERNAL"),
			Description: sdk.String(fmt.Sprintf("Static egress IP for GKE cluster %s", clusterName)),
		}, opts...)
	}
}

// createCloudRouter creates a Cloud Router for NAT
func createCloudRouter(
	ctx *sdk.Context,
	clusterName string,
	region string,
	cluster *container.Cluster,
	opts []sdk.ResourceOption,
	params pApi.ProvisionParams,
) (*compute.Router, error) {
	routerName := fmt.Sprintf("%s-router", clusterName)
	params.Log.Info(ctx.Context(), "🔀 Creating Cloud Router: %s", routerName)

	return compute.NewRouter(ctx, routerName, &compute.RouterArgs{
		Name:    sdk.String(routerName),
		Region:  sdk.String(region),
		Network: cluster.Network.ToStringPtrOutput().Elem(), // Use cluster's actual VPC network
		Bgp: &compute.RouterBgpArgs{
			Asn: sdk.Int(64512), // Private ASN
		},
		Description: sdk.String(fmt.Sprintf("Cloud Router for GKE cluster %s NAT", clusterName)),
	}, opts...)
}

// createCloudNat creates a Cloud NAT gateway
func createCloudNat(
	ctx *sdk.Context,
	clusterName string,
	router *compute.Router,
	staticIp *compute.Address,
	region string,
	cluster *container.Cluster,
	opts []sdk.ResourceOption,
	params pApi.ProvisionParams,
) (*compute.RouterNat, error) {
	natName := fmt.Sprintf("%s-nat", clusterName)
	params.Log.Info(ctx.Context(), "🌐 Creating Cloud NAT gateway: %s", natName)

	// Create array of static IP references for NAT
	natIps := sdk.StringArray{staticIp.SelfLink}

	// Configure NAT for specific GKE cluster subnet instead of all subnets
	natArgs := &compute.RouterNatArgs{
		Name:   sdk.String(natName),
		Router: router.Name,
		Region: sdk.String(region),

		// NAT configuration - use our specific static IP (not random GCP-assigned IPs)
		NatIpAllocateOption: sdk.String("MANUAL_ONLY"), // Use only the IPs we specify in NatIps
		NatIps:              natIps,                    // Our static IP address

		// Port allocation - production-ready defaults
		MinPortsPerVm: sdk.Int(64),
		MaxPortsPerVm: sdk.Int(65536),

		// Logging configuration - errors only for cost optimization
		LogConfig: &compute.RouterNatLogConfigArgs{
			Enable: sdk.Bool(true),
			Filter: sdk.String("ERRORS_ONLY"),
		},

		// Enable endpoint independent mapping for better performance
		EnableEndpointIndependentMapping: sdk.Bool(true),
	}

	// Configure NAT to target primary IP ranges only (avoid secondary ranges)
	params.Log.Info(ctx.Context(), "🎯 Configuring NAT for primary IP ranges only")

	// Use ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES to avoid conflicts with secondary ranges
	natArgs.SourceSubnetworkIpRangesToNat = sdk.String("ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES")

	params.Log.Info(ctx.Context(), "🔧 NAT configured for primary IP ranges only (avoiding pod/service ranges)")

	// Log cluster subnetwork for debugging purposes
	cluster.Subnetwork.ApplyT(func(subnetwork string) error {
		if subnetwork != "" {
			params.Log.Info(ctx.Context(), "📍 GKE cluster using subnetwork: %s", subnetwork)
		} else {
			params.Log.Info(ctx.Context(), "📍 GKE cluster using default subnetwork")
		}
		return nil
	})

	// Add additional logging to help debug NAT configuration
	params.Log.Info(ctx.Context(), "🔍 NAT Configuration Details:")
	params.Log.Info(ctx.Context(), "   - IP Allocation: MANUAL_ONLY (using static IP %s)", staticIp.Name.ToStringOutput())
	params.Log.Info(ctx.Context(), "   - Source Ranges: ALL_SUBNETWORKS_ALL_PRIMARY_IP_RANGES")
	params.Log.Info(ctx.Context(), "   - Port Range: %d-%d per VM", 64, 65536)
	params.Log.Info(ctx.Context(), "")
	params.Log.Info(ctx.Context(), "🔍 Troubleshooting Steps if egress IP is still wrong:")
	params.Log.Info(ctx.Context(), "   1. Check GCP Console → VPC Network → Cloud NAT")
	params.Log.Info(ctx.Context(), "   2. Verify NAT gateway is 'Active' and using your static IP")
	params.Log.Info(ctx.Context(), "   3. Check if there are multiple NAT gateways in the same region")
	params.Log.Info(ctx.Context(), "   4. Restart pods to pick up new NAT configuration")
	params.Log.Info(ctx.Context(), "   5. Run: kubectl delete pods --all -n <namespace>")

	// Export the static IP for easy reference
	staticIp.Address.ApplyT(func(addr string) error {
		params.Log.Info(ctx.Context(), "📍 Expected egress IP: %s", addr)
		return nil
	})

	return compute.NewRouterNat(ctx, natName, natArgs, opts...)
}

// createEgressNATPolicy creates an Egress NAT Policy to allow Cloud NAT for external traffic
func createEgressNATPolicy(
	ctx *sdk.Context,
	gkeInput *gcloud.GkeAutopilotResource,
	clusterName string,
	cluster *container.Cluster,
	opts []sdk.ResourceOption,
	params pApi.ProvisionParams,
) error {
	policyName := fmt.Sprintf("%s-cloud-nat-policy", clusterName)
	params.Log.Info(ctx.Context(), "🔧 Creating Egress NAT Policy for Cloud NAT compatibility")

	// Create Kubernetes provider using the cluster's kubeconfig
	kubeconfig := generateKubeconfig(cluster, gkeInput)
	k8sProvider, err := kubernetes.NewProvider(ctx, fmt.Sprintf("%s-k8s-provider", clusterName), &kubernetes.ProviderArgs{
		Kubeconfig: kubeconfig,
	}, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to create Kubernetes provider for Egress NAT Policy")
	}

	// Create the Egress NAT Policy using Pulumi's CustomResource
	egressNATPolicy, err := apiextensions.NewCustomResource(ctx, policyName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("networking.gke.io/v1"),
		Kind:       sdk.String("EgressNATPolicy"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: sdk.String(policyName),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": sdk.All(cluster.ClusterIpv4Cidr, cluster.ServicesIpv4Cidr).ApplyT(func(args []interface{}) (map[string]interface{}, error) {
				podCIDR := args[0].(string)
				serviceCIDR := args[1].(string)

				params.Log.Info(ctx.Context(), "📋 Creating Egress NAT Policy with:")
				params.Log.Info(ctx.Context(), "   - Pod CIDR (NoSNAT): %s", podCIDR)
				params.Log.Info(ctx.Context(), "   - Service CIDR (NoSNAT): %s", serviceCIDR)
				params.Log.Info(ctx.Context(), "   - External traffic: Will use Cloud NAT SNAT")

				return map[string]interface{}{
					"action": "NoSNAT",
					"destinations": []map[string]interface{}{
						{"cidr": podCIDR},     // Pod IP range
						{"cidr": serviceCIDR}, // Service IP range
					},
				}, nil
			}),
		},
	}, sdk.Provider(k8sProvider), sdk.Parent(cluster))
	if err != nil {
		params.Log.Warn(ctx.Context(), "⚠️ Failed to create Egress NAT Policy via Pulumi: %v", err)
		params.Log.Info(ctx.Context(), "")
		params.Log.Info(ctx.Context(), "🚨 MANUAL ACTION REQUIRED:")
		params.Log.Info(ctx.Context(), "Your Cloud NAT is configured correctly, but you need to manually")
		params.Log.Info(ctx.Context(), "create an Egress NAT Policy to enable it for external traffic.")
		params.Log.Info(ctx.Context(), "")

		// Provide fallback manual instructions
		cluster.ClusterIpv4Cidr.ApplyT(func(podCIDR string) error {
			cluster.ServicesIpv4Cidr.ApplyT(func(serviceCIDR string) error {
				params.Log.Info(ctx.Context(), "Run this command:")
				params.Log.Info(ctx.Context(), "kubectl apply -f - << 'EOF'")
				params.Log.Info(ctx.Context(), "apiVersion: networking.gke.io/v1")
				params.Log.Info(ctx.Context(), "kind: EgressNATPolicy")
				params.Log.Info(ctx.Context(), "metadata:")
				params.Log.Info(ctx.Context(), "  name: %s", policyName)
				params.Log.Info(ctx.Context(), "spec:")
				params.Log.Info(ctx.Context(), "  action: NoSNAT")
				params.Log.Info(ctx.Context(), "  destinations:")
				params.Log.Info(ctx.Context(), "  - cidr: %s  # Pod IP range", podCIDR)
				params.Log.Info(ctx.Context(), "  - cidr: %s  # Service IP range", serviceCIDR)
				params.Log.Info(ctx.Context(), "EOF")
				return nil
			})
			return nil
		})

		return nil // Don't fail the entire deployment
	}

	params.Log.Info(ctx.Context(), "✅ Egress NAT Policy created successfully via Pulumi")
	params.Log.Info(ctx.Context(), "💡 Restart pods to use the new NAT configuration:")
	params.Log.Info(ctx.Context(), "   kubectl delete pods --all -n default")

	// Export the policy for reference
	ctx.Export(fmt.Sprintf("%s-egress-nat-policy", clusterName), egressNATPolicy.ID())

	return nil
}

// extractRegionFromLocation extracts region from GKE location (handles both regional and zonal)
func extractRegionFromLocation(location string) string {
	// Handle both regional (us-central1) and zonal (us-central1-a) locations

	// Special handling for complex region names that don't follow simple pattern
	complexRegions := map[string]string{
		"asia-southeast1-a":         "asia-southeast1",
		"asia-southeast1-b":         "asia-southeast1",
		"asia-southeast1-c":         "asia-southeast1",
		"asia-southeast2-a":         "asia-southeast2",
		"asia-southeast2-b":         "asia-southeast2",
		"asia-southeast2-c":         "asia-southeast2",
		"asia-northeast1-a":         "asia-northeast1",
		"asia-northeast1-b":         "asia-northeast1",
		"asia-northeast1-c":         "asia-northeast1",
		"asia-northeast2-a":         "asia-northeast2",
		"asia-northeast2-b":         "asia-northeast2",
		"asia-northeast2-c":         "asia-northeast2",
		"asia-northeast3-a":         "asia-northeast3",
		"asia-northeast3-b":         "asia-northeast3",
		"asia-northeast3-c":         "asia-northeast3",
		"australia-southeast1-a":    "australia-southeast1",
		"australia-southeast1-b":    "australia-southeast1",
		"australia-southeast1-c":    "australia-southeast1",
		"australia-southeast2-a":    "australia-southeast2",
		"australia-southeast2-b":    "australia-southeast2",
		"australia-southeast2-c":    "australia-southeast2",
		"europe-southwest1-a":       "europe-southwest1",
		"europe-southwest1-b":       "europe-southwest1",
		"europe-southwest1-c":       "europe-southwest1",
		"northamerica-northeast1-a": "northamerica-northeast1",
		"northamerica-northeast1-b": "northamerica-northeast1",
		"northamerica-northeast1-c": "northamerica-northeast1",
		"northamerica-northeast2-a": "northamerica-northeast2",
		"northamerica-northeast2-b": "northamerica-northeast2",
		"northamerica-northeast2-c": "northamerica-northeast2",
		"southamerica-east1-a":      "southamerica-east1",
		"southamerica-east1-b":      "southamerica-east1",
		"southamerica-east1-c":      "southamerica-east1",
		"southamerica-west1-a":      "southamerica-west1",
		"southamerica-west1-b":      "southamerica-west1",
		"southamerica-west1-c":      "southamerica-west1",
	}

	// Check if it's a known complex region zone
	if region, exists := complexRegions[location]; exists {
		return region
	}

	// For zones like us-central1-a, extract us-central1
	if strings.Count(location, "-") >= 2 {
		lastDash := strings.LastIndex(location, "-")
		if lastDash > 0 {
			zonePart := location[lastDash+1:]
			// Check if the part after the last dash is a single letter (zone indicator)
			if len(zonePart) == 1 && zonePart >= "a" && zonePart <= "z" {
				return location[:lastDash]
			}
		}
	}

	// Return as-is for regional clusters (already in correct format)
	return location
}
