package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/storage"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
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
		// Provision GCS bucket and service account for Caddy ACME certificate storage
		bucket, credentialsJSON, err := provisionCaddyACMEStorage(ctx, clusterName, gkeInput.ProjectId, location, opts, params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision ACME storage for Caddy in cluster %q", clusterName)
		}

		// Build Caddyfile prefix with GCS storage configuration
		caddyfilePrefix := bucket.Name.ApplyT(func(bucketName string) string {
			return fmt.Sprintf(`{
  storage gcs {
    bucket %s
  }
}`, bucketName)
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
		caddyConfig := kubernetes.CaddyDeployment{
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

		caddy, err := kubernetes.DeployCaddyService(ctx, caddyConfig, input, params, kubeconfig)
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

	// Return bucket and service account key (credentials JSON)
	return bucket, sa.ServiceAccountKey.PrivateKey, nil
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
