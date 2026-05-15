package kubernetes

import (
	"embed"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	policyv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

//go:embed embed/caddy/*
var Caddyconfig embed.FS

const (
	AppTypeSimpleContainer = "simple-container"

	AnnotationCaddyfileEntry = "simple-container.com/caddyfile-entry"
	AnnotationParentStack    = "simple-container.com/parent-stack"
	AnnotationDomain         = "simple-container.com/domain"
	AnnotationPrefix         = "simple-container.com/prefix"
	AnnotationPort           = "simple-container.com/port"
	AnnotationEnv            = "simple-container.com/env"

	// Standard Kubernetes labels - using hyphens instead of dots for GCP compatibility
	// Kubernetes allows dots in label prefixes, but GCP labels do not
	LabelAppType     = "simple-container-com/app-type"
	LabelAppName     = "simple-container-com/app-name"
	LabelScEnv       = "simple-container-com/env"
	LabelParentEnv   = "simple-container-com/parent-env"
	LabelParentStack = "simple-container-com/parent-stack"
	LabelClientStack = "simple-container-com/client-stack"
	LabelCustomStack = "simple-container-com/custom-stack"
)

// sanitizeK8sResourceName converts a name to be RFC 1123 compliant for Kubernetes resources
// Replaces underscores with hyphens and ensures it starts/ends with alphanumeric characters
func sanitizeK8sResourceName(name string) string {
	// Replace underscores with hyphens
	sanitized := strings.ReplaceAll(name, "_", "-")
	// Remove any invalid characters (keep only a-z, 0-9, -, .)
	reg := regexp.MustCompile(`[^a-z0-9\-\.]`)
	sanitized = reg.ReplaceAllString(strings.ToLower(sanitized), "")
	// Ensure it starts and ends with alphanumeric (trim leading/trailing hyphens and dots)
	sanitized = strings.Trim(sanitized, "-.")
	return sanitized
}

// sanitizeK8sLabelValue sanitizes a value to be valid for Kubernetes labels
// Kubernetes label values must be empty or consist of alphanumeric characters, '-', '_' or '.',
// and must start and end with an alphanumeric character
func sanitizeK8sLabelValue(value string) string {
	if value == "" {
		return value
	}

	// Replace forward slashes with hyphens (common in stack paths like "/demo/root")
	sanitized := strings.ReplaceAll(value, "/", "-")
	// Replace other invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_\.]`)
	sanitized = reg.ReplaceAllString(sanitized, "-")
	// Remove leading/trailing hyphens, underscores, or dots to ensure alphanumeric start/end
	sanitized = strings.Trim(sanitized, "-_.")

	// If the result is empty after sanitization, provide a default
	if sanitized == "" {
		sanitized = "unknown"
	}

	return sanitized
}

type SimpleContainerArgs struct {
	// required properties
	Namespace              string  `json:"namespace" yaml:"namespace"`
	Service                string  `json:"service" yaml:"service"`
	ScEnv                  string  `json:"scEnv" yaml:"scEnv"`
	Domain                 string  `json:"domain" yaml:"domain"`
	Prefix                 string  `json:"prefix" yaml:"prefix"`
	ProxyKeepPrefix        bool    `json:"proxyKeepPrefix" yaml:"proxyKeepPrefix"`
	Deployment             string  `json:"deployment" yaml:"deployment"`
	ParentStack            *string `json:"parentStack" yaml:"parentStack"`
	ParentEnv              *string `json:"parentEnv" yaml:"parentEnv"`
	Replicas               int     `json:"replicas" yaml:"replicas"`
	GenerateCaddyfileEntry bool    `json:"generateCaddyfileEntry" yaml:"generateCaddyfileEntry"`
	KubeProvider           sdk.ProviderResource

	// optional properties
	PodDisruption         *k8s.DisruptionBudget        `json:"podDisruption" yaml:"podDisruption"`
	LbConfig              *api.SimpleContainerLBConfig `json:"lbConfig" yaml:"lbConfig"`
	SecretEnvs            map[string]string            `json:"secretEnvs" yaml:"secretEnvs"`
	Annotations           map[string]string            `json:"annotations" yaml:"annotations"`
	NodeSelector          map[string]string            `json:"nodeSelector" yaml:"nodeSelector"`
	Affinity              *k8s.AffinityRules           `json:"affinity" yaml:"affinity"`
	PriorityClassName     *string                      `json:"priorityClassName" yaml:"priorityClassName"` // Kubernetes PriorityClass for pod scheduling and preemption
	IngressContainer      *k8s.CloudRunContainer       `json:"ingressContainer" yaml:"ingressContainer"`
	ServiceType           *string                      `json:"serviceType" yaml:"serviceType"`
	ExternalTrafficPolicy *string                      `json:"externalTrafficPolicy" yaml:"externalTrafficPolicy"`
	ProvisionIngress      bool                         `json:"provisionIngress" yaml:"provisionIngress"`
	Headers               *k8s.Headers                 `json:"headers" yaml:"headers"`
	Volumes               []k8s.SimpleTextVolume       `json:"volumes" yaml:"volumes"`
	SecretVolumes         []k8s.SimpleTextVolume       `json:"secretVolumes" yaml:"secretVolumes"`
	PersistentVolumes     []k8s.PersistentVolume       `json:"persistentVolumes" yaml:"persistentVolumes"`
	EphemeralVolumes      []k8s.GenericEphemeralVolume `json:"ephemeralVolumes" yaml:"ephemeralVolumes"` // Generic ephemeral volumes for large temp storage
	VPA                   *k8s.VPAConfig               `json:"vpa" yaml:"vpa"`
	Scale                 *k8s.Scale                   `json:"scale" yaml:"scale"`

	Log logger.Logger
	// ...
	RollingUpdate                 *v1.RollingUpdateDeploymentArgs
	InitContainers                []corev1.ContainerArgs
	Containers                    []corev1.ContainerArgs
	SecurityContext               *corev1.PodSecurityContextArgs
	ServiceAccountName            *sdk.StringOutput
	Sidecars                      []corev1.ContainerArgs
	SidecarOutputs                []corev1.ContainerOutput
	InitContainerOutputs          []corev1.ContainerOutput
	VolumeOutputs                 []corev1.VolumeOutput
	SecretVolumeOutputs           []any
	ComputeContext                pApi.ComputeContext
	ImagePullSecret               *docker.RegistryCredentials
	UseSSL                        bool
	EphemeralSize                 string
	TerminationGracePeriodSeconds *int

	// NamespaceNameOutput is the live k8s name of the Namespace resource — set by
	// NewSimpleContainer right after the Namespace is created and before
	// RunPreProcessors fires. Pre/post-processors (e.g. CSQL sidecar in
	// pkg/clouds/pulumi/gcp/compute_proc.go) must use this Output instead of
	// recomputing the namespace via kubernetes.GenerateNamespaceName(stackName,
	// stackEnv, parentEnv): the Namespace carries IgnoreChanges("metadata.name")
	// (see #255), so its k8s name is the *state* name (parent-shared for
	// migrated stacks, isolated for fresh stacks), not whatever
	// GenerateNamespaceName would derive. Consuming this Output keeps
	// downstream resources in lock-step with the Namespace through both modes.
	NamespaceNameOutput sdk.StringOutput
}

type SimpleContainer struct {
	sdk.ResourceState

	ServicePublicIP sdk.StringOutput    `pulumi:"servicePublicIP"`
	ServiceName     sdk.StringPtrOutput `pulumi:"serviceName"`
	Namespace       sdk.StringOutput    `pulumi:"namespace"`
	Port            sdk.IntPtrOutput    `pulumi:"port"`
	CaddyfileEntry  sdk.String          `pulumi:"caddyfileEntry"`
	Service         *corev1.Service     `pulumi:"service"`
	Deployment      *v1.Deployment      `pulumi:"deployment"`
}

func NewSimpleContainer(ctx *sdk.Context, args *SimpleContainerArgs, opts ...sdk.ResourceOption) (*SimpleContainer, error) {
	sc := &SimpleContainer{}

	// Sanitize deployment and service names early to comply with Kubernetes RFC 1123 requirements
	sanitizedDeployment := sanitizeK8sName(args.Deployment)
	sanitizedService := sanitizeK8sName(args.Service)

	// Extract parentEnv for resource naming
	var parentEnv string
	if args.ParentEnv != nil {
		parentEnv = lo.FromPtr(args.ParentEnv)
	}

	appLabels := map[string]string{
		LabelAppType: AppTypeSimpleContainer,
		LabelAppName: sanitizedService,
		LabelScEnv:   args.ScEnv,
	}

	// Add parentEnv labels for custom stacks
	if args.ParentEnv != nil && lo.FromPtr(args.ParentEnv) != "" && lo.FromPtr(args.ParentEnv) != args.ScEnv {
		appLabels[LabelParentEnv] = sanitizeK8sLabelValue(lo.FromPtr(args.ParentEnv))
		appLabels[LabelCustomStack] = "true"
	}

	// Add parent-stack and client-stack labels if provided
	if args.ParentStack != nil && *args.ParentStack != "" {
		appLabels[LabelParentStack] = sanitizeK8sLabelValue(*args.ParentStack)
	}
	// Note: client-stack is typically same as parent-stack in nested scenarios
	// but can be different in more complex hierarchies
	if args.ParentStack != nil && *args.ParentStack != "" {
		appLabels[LabelClientStack] = sanitizeK8sLabelValue(*args.ParentStack)
	}

	appAnnotations := map[string]string{
		AnnotationDomain: args.Domain,
		AnnotationPrefix: args.Prefix,
		AnnotationEnv:    args.ScEnv,
	}
	var mainPort *int
	if args.IngressContainer != nil && args.IngressContainer.MainPort != nil {
		mainPort = args.IngressContainer.MainPort
	} else if len(lo.FromPtr(args.IngressContainer).Ports) == 1 {
		mainPort = lo.ToPtr(lo.FromPtr(args.IngressContainer).Ports[0])
	}
	if mainPort != nil {
		appAnnotations[AnnotationPort] = strconv.Itoa(*mainPort)
	}
	if args.ParentStack != nil {
		appAnnotations[AnnotationParentStack] = lo.FromPtr(args.ParentStack)
	}
	// apply provided annotations
	for k, v := range args.Annotations {
		appAnnotations[k] = v
	}

	// Namespace
	// Sanitize namespace name to comply with Kubernetes RFC 1123 requirements
	sanitizedNamespace := sanitizeK8sName(args.Namespace)
	// Use deployment name as Pulumi resource name to ensure uniqueness across environments
	// while keeping the actual K8s namespace name as specified by the user.
	//
	// Namespace-handling has two protections against the destroy/Replace cascade
	// hazard discovered in pre-PR-230 deploys (see PR #230 and the consumer-side
	// outages tracked in #255):
	//
	// 1. RetainOnDelete(true). In legacy deploys, sub-env client stacks
	//    (parentEnv=<prod> with stackEnv=tenant-a/tenant-b/...) shared one
	//    physical K8s namespace because metadata.Name was derived from
	//    stackName, not stackEnv. Each stack tracked its own Pulumi Namespace
	//    resource at a unique URN, but they all pointed at the same physical
	//    namespace. Destroying any single sub-env stack would cascade-delete
	//    the shared namespace and wipe every sibling. RetainOnDelete keeps
	//    Pulumi from issuing the k8s DELETE on destroy.
	//
	// 2. IgnoreChanges("metadata.name"). PR #230 changed GenerateNamespaceName
	//    to isolate custom stacks (stackName-stackEnv) instead of sharing the
	//    parent's namespace. That works for fresh deploys, but for any consumer
	//    whose Pulumi state predates #230, the next `pulumi up` saw a diff
	//    between state's metadata.Name="<stackName>" and program's
	//    metadata.Name="<stackName>-<stackEnv>", and scheduled a Replace.
	//    Replace = create-new + delete-old, and `RetainOnDelete` on the new
	//    resource is non-retroactive — Pulumi reads delete-time options from
	//    the OLD resource's state, which predates the flag. The k8s DELETE on
	//    the shared namespace went through and cascade-killed the parent
	//    stack's running resources.
	//
	//    IgnoreChanges("metadata.name") suppresses the diff entirely. No
	//    Replace is scheduled, no delete fires. The resource state retains
	//    whatever metadata.Name it had (new for fresh deploys, legacy shared
	//    for migrated consumers). Other resources (Service, Deployment, …)
	//    that reference namespace.Metadata.Name().Elem() follow whichever
	//    name is in effect — fresh deploys land in the isolated namespace,
	//    migrated consumers continue using the shared one. Combined with
	//    RetainOnDelete this keeps both modes safe.
	//
	//    Downstream Secret/Job resources consume the live namespace via
	//    SimpleContainerArgs.NamespaceNameOutput (set a few lines below from
	//    namespace.Metadata.Name().Elem()) rather than recomputing it via
	//    GenerateNamespaceName. The three pre/post-processor call sites that
	//    consume this Output live in:
	//
	//      pkg/clouds/pulumi/gcp/compute_proc.go                 (GCP CSQL sidecar + init proxies)
	//      pkg/clouds/pulumi/kubernetes/compute_proc_postgres.go (on-cluster postgres init Job)
	//      pkg/clouds/pulumi/kubernetes/compute_proc_mongodb.go  (on-cluster mongo init Job)
	//
	//    From there the namespace flows into NewCloudsqlProxy /
	//    NewPostgresInitDbUserJob / NewMongodbInitDbUserJob as an
	//    sdk.StringInput, so the leaf Secret/Job ObjectMeta.Namespace tracks
	//    the live Output. That keeps them in lock-step with whatever
	//    metadata.Name is in state — fresh stacks get the isolated namespace,
	//    migrated stacks stay parent-shared — without needing
	//    IgnoreChanges("metadata.namespace") on every individual Secret/Job.
	//    Migrating an existing custom stack from parent-shared to isolated
	//    namespace is therefore automatic via `pulumi stack export | jq
	//    'del(... namespace urn ...)' | pulumi stack import` (forget the
	//    Namespace resource, then `pulumi up` creates a fresh Namespace at
	//    the isolated name and the downstream consumers follow it). See PR #258.
	namespaceResourceName := fmt.Sprintf("%s-ns", sanitizedDeployment)
	namespace, err := corev1.NewNamespace(ctx, namespaceResourceName, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(sanitizedNamespace),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
	}, append(opts, sdk.RetainOnDelete(true), sdk.IgnoreChanges([]string{"metadata.name"}))...)
	if err != nil {
		return nil, err
	}

	// Expose the live Namespace name to pre/post-processors. See the comment on
	// NamespaceNameOutput in SimpleContainerArgs: GCP CSQL and K8s on-cluster
	// init Jobs need this Output (not GenerateNamespaceName) to land in the
	// same namespace as the consuming pod under both fresh and migrated state.
	args.NamespaceNameOutput = namespace.Metadata.Name().Elem()

	// run pre-processors after namespace is created, but before deployment is created
	if args.ComputeContext != nil {
		if err := args.ComputeContext.RunPreProcessors(args, args); err != nil {
			return nil, err
		}
	}

	// Volumes and Secrets
	volumeToData := make(map[string]string)
	for _, volume := range args.Volumes {
		content := base64.StdEncoding.EncodeToString([]byte(volume.Content))
		volumeToData[volume.Name] = content
	}

	secretVolumeToData := make(map[string]string)
	for _, secretVolume := range args.SecretVolumes {
		secretVolumeToData[secretVolume.Name] = secretVolume.Content
	}

	// Generate resource names with parentEnv-aware logic
	baseResourceName := generateDeploymentName(sanitizedService, args.ScEnv, parentEnv)
	volumesCfgName := generateConfigVolumesName(sanitizedService, args.ScEnv, parentEnv)
	envSecretName := generateSecretName(sanitizedService, args.ScEnv, parentEnv)
	volumesSecretName := generateSecretVolumesName(sanitizedService, args.ScEnv, parentEnv)
	imagePullSecretName := generateImagePullSecretName(sanitizedService, args.ScEnv, parentEnv)

	var imagePullSecret *corev1.Secret
	if args.ImagePullSecret != nil {
		args.Log.Info(ctx.Context(), "Creating imagePullSecret for service %s", args.Service)

		imagePullSecretString, err := args.ImagePullSecret.ToImagePullSecret()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert pull secret to string")
		}
		imagePullSecret, err = corev1.NewSecret(ctx, imagePullSecretName, &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(imagePullSecretName),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Type: sdk.String("kubernetes.io/dockerconfigjson"),
			Data: sdk.ToStringMap(map[string]string{
				".dockerconfigjson": imagePullSecretString,
			}),
		}, opts...)
		if err != nil {
			return nil, err
		}
	}

	// ConfigMap
	volumesConfigMap, err := corev1.NewConfigMap(ctx, volumesCfgName, &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(volumesCfgName),
			Namespace:   namespace.Metadata.Name().Elem(),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
		BinaryData: sdk.ToStringMap(volumeToData),
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Secrets
	envSecret, err := corev1.NewSecret(ctx, envSecretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(envSecretName),
			Namespace:   namespace.Metadata.Name().Elem(),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
		StringData: sdk.ToStringMap(args.SecretEnvs),
	}, opts...)
	if err != nil {
		return nil, err
	}

	volumesSecret, err := corev1.NewSecret(ctx, volumesSecretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(volumesSecretName),
			Namespace:   namespace.Metadata.Name().Elem(),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
		StringData: sdk.All(args.SecretVolumeOutputs...).ApplyT(func(vols []any) map[string]string {
			for _, va := range vols {
				vol := va.(k8s.SimpleTextVolume)
				secretVolumeToData[vol.Name] = vol.Content
			}
			return secretVolumeToData
		}).(sdk.StringMapOutput),
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Volume Mounts
	var volumeMounts corev1.VolumeMountArray
	addVolumeMounts(volumesSecretName, args.SecretVolumes, &volumeMounts)
	addVolumeMounts(volumesCfgName, args.Volumes, &volumeMounts)
	addVolumeMountsFromOutputs(volumesSecretName, args.SecretVolumeOutputs, &volumeMounts)

	// Volumes
	emptyDirArgs := corev1.EmptyDirVolumeSourceArgs{}
	if args.EphemeralSize != "" {
		emptyDirArgs.SizeLimit = sdk.StringPtr(args.EphemeralSize)
	}
	volumes := corev1.VolumeArray{
		corev1.VolumeArgs{
			Name: sdk.String(volumesCfgName),
			ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
				Name: volumesConfigMap.Metadata.Name(),
			},
		},
		corev1.VolumeArgs{
			Name: sdk.String(volumesSecretName),
			Secret: &corev1.SecretVolumeSourceArgs{
				SecretName: volumesSecret.Metadata.Name(),
			},
		},
		corev1.VolumeArgs{
			Name:     sdk.String("tmp"),
			EmptyDir: emptyDirArgs,
		},
	}
	volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
		Name:      sdk.String("tmp"),
		MountPath: sdk.String("/tmp"),
	})

	// Persistent volumes
	for _, pv := range args.PersistentVolumes {
		accessModes := []sdk.StringInput{sdk.String("ReadWriteOnce")}
		if len(pv.AccessModes) > 0 {
			accessModes = lo.Map(pv.AccessModes, func(am string, _ int) sdk.StringInput {
				return sdk.String(am)
			})
		}
		// Sanitize volume name for Kubernetes RFC 1123 compliance (no underscores allowed)
		sanitizedName := sanitizeK8sResourceName(pv.Name)
		if sanitizedName != pv.Name {
			args.Log.Info(ctx.Context(), "📝 Sanitized volume name %q -> %q for Kubernetes RFC 1123 compliance", pv.Name, sanitizedName)
		}

		// Use default storage class for GKE when none specified
		storageClass := pv.StorageClassName
		if storageClass == nil {
			// For GKE, use the default standard storage class if available
			defaultSC := "standard-rwo"
			storageClass = &defaultSC
			args.Log.Info(ctx.Context(), "📦 Using default storage class %q for volume %q", defaultSC, sanitizedName)
		}
		_, err := corev1.NewPersistentVolumeClaim(ctx, sanitizedName, &corev1.PersistentVolumeClaimArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(sanitizedName),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Spec: &corev1.PersistentVolumeClaimSpecArgs{
				AccessModes:      sdk.StringArray(accessModes),
				StorageClassName: sdk.StringPtrFromPtr(storageClass),
				Resources: &corev1.VolumeResourceRequirementsArgs{
					Requests: sdk.StringMap{
						"storage": sdk.String(pv.Storage),
					},
				},
			},
		}, opts...)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, corev1.VolumeArgs{
			Name: sdk.String(sanitizedName),
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{
				ClaimName: sdk.String(sanitizedName),
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
			Name:      sdk.String(sanitizedName),
			MountPath: sdk.String(pv.MountPath),
		})
	}

	// Generic ephemeral volumes
	// These use the generic ephemeral volume feature which creates a PVC for each pod
	// and deletes it when the pod is deleted. This allows for larger temporary storage
	// than the 10GB limit on GKE Autopilot regular ephemeral storage.
	for _, ev := range args.EphemeralVolumes {
		// Sanitize volume name for Kubernetes RFC 1123 compliance (no underscores allowed)
		sanitizedName := sanitizeK8sResourceName(ev.Name)
		if sanitizedName != ev.Name {
			args.Log.Info(ctx.Context(), "📝 Sanitized ephemeral volume name %q -> %q for Kubernetes RFC 1123 compliance", ev.Name, sanitizedName)
		}

		// Set default storage class if not specified
		storageClass := ev.StorageClassName
		if storageClass == nil {
			// Use the default standard-rwo storage class for GKE
			defaultSC := "standard-rwo"
			storageClass = &defaultSC
			args.Log.Info(ctx.Context(), "📦 Using default storage class %q for ephemeral volume %q", defaultSC, sanitizedName)
		}

		// Create the generic ephemeral volume with volumeClaimTemplate
		// This creates a PVC for each pod and deletes it when the pod is deleted
		// Note: metadata.name cannot be set in volumeClaimTemplate - Kubernetes generates it automatically
		volumes = append(volumes, corev1.VolumeArgs{
			Name: sdk.String(sanitizedName),
			Ephemeral: &corev1.EphemeralVolumeSourceArgs{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimTemplateArgs{
					Metadata: &metav1.ObjectMetaArgs{
						// Name is intentionally omitted - Kubernetes generates it automatically
						Labels:      sdk.ToStringMap(appLabels),
						Annotations: sdk.ToStringMap(appAnnotations),
					},
					Spec: &corev1.PersistentVolumeClaimSpecArgs{
						AccessModes: sdk.StringArray{
							sdk.String("ReadWriteOnce"),
						},
						StorageClassName: sdk.StringPtrFromPtr(storageClass),
						Resources: &corev1.VolumeResourceRequirementsArgs{
							Requests: sdk.StringMap{
								"storage": sdk.String(ev.Size),
							},
						},
					},
				},
			},
		})

		// Add the volume mount
		volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
			Name:      sdk.String(sanitizedName),
			MountPath: sdk.String(ev.MountPath),
		})

		args.Log.Info(ctx.Context(), "✨ Added generic ephemeral volume %q at %q with size %q (storage class: %q)",
			sanitizedName, ev.MountPath, ev.Size, lo.FromPtr(storageClass))
	}

	var strategy v1.DeploymentStrategyArgs
	if args.RollingUpdate == nil {
		strategy = v1.DeploymentStrategyArgs{
			RollingUpdate: v1.RollingUpdateDeploymentArgs{
				MaxSurge:       sdk.Int(1),
				MaxUnavailable: sdk.Int(0),
			},
		}
	} else {
		strategy = v1.DeploymentStrategyArgs{
			RollingUpdate: lo.FromPtr(args.RollingUpdate),
		}
	}

	for i := range args.Containers {
		// Update container volume mounts and envFrom
		args.Containers[i].VolumeMounts = volumeMounts
		args.Containers[i].EnvFrom = corev1.EnvFromSourceArray{
			corev1.EnvFromSourceArgs{
				SecretRef: &corev1.SecretEnvSourceArgs{
					Name: envSecret.Metadata.Name(),
				},
			},
		}
	}
	containers := corev1.ContainerArray{}
	initContainers := corev1.ContainerArray{}
	for _, c := range args.Containers {
		containers = append(containers, c)
	}
	for _, c := range args.Sidecars {
		containers = append(containers, c)
	}
	for _, c := range args.InitContainers {
		initContainers = append(initContainers, c)
	}

	sidecarOutputs := lo.Map(args.SidecarOutputs, func(o corev1.ContainerOutput, _ int) any { return o })
	volumeOutputs := lo.Map(args.VolumeOutputs, func(o corev1.VolumeOutput, _ int) any { return o })
	initContainerOutputs := lo.Map(args.InitContainerOutputs, func(o corev1.ContainerOutput, _ int) any { return o })
	// Deployment
	args.Log.Info(ctx.Context(), "🔍 DEBUG: About to convert affinity rules - args.Affinity: %+v", args.Affinity)
	convertedAffinity := convertAffinityRulesToKubernetes(args.Affinity)
	args.Log.Info(ctx.Context(), "🔍 DEBUG: Converted affinity result: %+v", convertedAffinity)

	podSpecArgs := &corev1.PodSpecArgs{
		NodeSelector: sdk.ToStringMap(args.NodeSelector),
		Affinity:     convertedAffinity,
		TerminationGracePeriodSeconds: func() sdk.IntPtrInput {
			if args.TerminationGracePeriodSeconds != nil {
				return sdk.IntPtr(*args.TerminationGracePeriodSeconds)
			}
			return nil
		}(),
		InitContainers: sdk.All(initContainerOutputs...).ApplyT(func(scOuts []any) (corev1.ContainerArray, error) {
			for _, c := range scOuts {
				initContainers = append(initContainers, c.(corev1.ContainerInput))
			}
			return initContainers, nil
		}).(corev1.ContainerArrayOutput),
		Containers: sdk.All(sidecarOutputs...).ApplyT(func(scOuts []any) (corev1.ContainerArray, error) {
			for _, c := range scOuts {
				containers = append(containers, c.(corev1.ContainerInput))
			}
			return containers, nil
		}).(corev1.ContainerArrayOutput),
		Volumes: sdk.All(volumeOutputs...).ApplyT(func(vOuts []any) (corev1.VolumeArray, error) {
			for _, v := range vOuts {
				volumes = append(volumes, v.(corev1.VolumeInput))
			}
			return volumes, nil
		}).(corev1.VolumeArrayOutput),
		SecurityContext:    args.SecurityContext,
		ServiceAccountName: args.ServiceAccountName,
	}

	// Set optional fields if provided
	if args.PriorityClassName != nil {
		podSpecArgs.PriorityClassName = sdk.String(*args.PriorityClassName)
	}
	if imagePullSecret != nil {
		podSpecArgs.ImagePullSecrets = corev1.LocalObjectReferenceArray{
			corev1.LocalObjectReferenceArgs{
				Name: imagePullSecret.Metadata.Name(),
			},
		}
	}
	deployment, err := v1.NewDeployment(ctx, sanitizedDeployment, &v1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(sanitizedDeployment),
			Namespace:   namespace.Metadata.Name().Elem(),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
		Spec: &v1.DeploymentSpecArgs{
			Strategy: strategy,
			Replicas: sdk.Int(args.Replicas),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: sdk.ToStringMap(appLabels),
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels:      sdk.ToStringMap(appLabels),
					Annotations: sdk.ToStringMap(appAnnotations),
				},
				Spec: podSpecArgs,
			},
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Expose service
	serviceType := sdk.String("ClusterIP")
	if args.ServiceType != nil {
		serviceType = sdk.String(lo.FromPtr(args.ServiceType))
	}

	serviceAnnotations := lo.Assign(appAnnotations)

	var caddyfileEntry string
	var caddyfileEntryAnnotation sdk.StringInput
	if args.GenerateCaddyfileEntry && mainPort != nil {
		// The unsubstituted template — used for both the initial sync render
		// (sc.CaddyfileEntry static export, change-hash signal) and the
		// deferred re-render inside ApplyT below (live-namespace annotation
		// on the Service). Single source of truth so any template tweak
		// updates both paths.
		var caddyfileEntryTemplate string
		if args.Domain != "" {
			caddyfileEntryTemplate = `
${proto}://${domain} {
  reverse_proxy http://${service}.${namespace}.svc.cluster.local:${port} {
    header_down Server nginx ${addHeaders}
    import handle_server_error
    ${extraHelpers}
  }
  ${imports}
}
`
		} else if args.Prefix != "" {
			caddyfileEntryTemplate = `
  handle_path /${prefix}* {${additionalProxyConfig}
    reverse_proxy http://${service}.${namespace}.svc.cluster.local:${port} {
      header_down Server nginx ${addHeaders}
      import handle_server_error
      ${extraHelpers}
    }
  }
`
		}
		imports := []string{
			"import gzip", "import handle_static",
		}
		if args.UseSSL {
			imports = append(imports, "import hsts")
		}
		placeholdersMap := placeholders.MapData{
			"proto":     lo.If(lo.FromPtr(args.LbConfig).Https, "https").Else("http"),
			"domain":    args.Domain,
			"prefix":    args.Prefix,
			"service":   sanitizedService,
			"namespace": sanitizedNamespace,
			"port":      strconv.Itoa(lo.FromPtr(mainPort)),
			"addHeaders": strings.Join(lo.Map(lo.Entries(lo.FromPtr(args.Headers)), func(h lo.Entry[string, string], _ int) string {
				return fmt.Sprintf("header_down %s %s", h.Key, h.Value)
			}), "\n    "),
			"extraHelpers": strings.Join(lo.FromPtr(args.LbConfig).ExtraHelpers, "\n    "),
			"imports":      strings.Join(imports, "\n    "),
		}
		if args.ProxyKeepPrefix {
			placeholdersMap["additionalProxyConfig"] = fmt.Sprintf("\n    rewrite * /%s{uri}", args.Prefix)
		} else {
			placeholdersMap["additionalProxyConfig"] = ""
		}
		// Apply placeholders synchronously so the static representation
		// (used for sc.CaddyfileEntry change-hash + log lines) is populated.
		// `namespace` here is sanitizedNamespace — for fresh deploys that
		// matches the live k8s namespace, but for migrated stacks with
		// IgnoreChanges("metadata.name") suppressing the rename the live
		// namespace stays at the legacy value. The annotation that lands
		// on the Service is computed from the live namespace Output below.
		caddyfileEntry = caddyfileEntryTemplate
		if err := placeholders.New().Apply(&caddyfileEntry, placeholders.WithData(placeholdersMap)); err != nil {
			return nil, errors.Wrapf(err, "failed to apply placeholders on caddyfile entry template")
		}

		// Build the actual annotation as an Output that resolves namespace
		// from the live Namespace resource's metadata.name. On migrated
		// stacks this is the legacy shared name (because of IgnoreChanges),
		// which is also where the Service is created, so reverse_proxy
		// http://${service}.${namespace}.svc.cluster.local resolves to
		// real cluster DNS. On fresh deploys it equals sanitizedNamespace
		// so the byte output matches the legacy code path.
		//
		// Render failures inside ApplyT are returned as errors (not silently
		// fallen back to the statically-rendered template) — falling back
		// would re-introduce the migrated-stack 502 bug this PR is fixing.
		staticEntry := caddyfileEntry
		caddyfileEntryAnnotation = namespace.Metadata.Name().ApplyT(func(nsPtr *string) (string, error) {
			liveNS := sanitizedNamespace
			if nsPtr != nil && *nsPtr != "" {
				liveNS = *nsPtr
			}
			if liveNS == sanitizedNamespace {
				// Fresh deploy or no migration: static template is correct verbatim.
				return staticEntry, nil
			}
			// Migrated stack: re-render with the live (legacy) namespace.
			localMap := make(placeholders.MapData, len(placeholdersMap))
			for k, v := range placeholdersMap {
				localMap[k] = v
			}
			localMap["namespace"] = liveNS
			rendered := caddyfileEntryTemplate
			if err := placeholders.New().Apply(&rendered, placeholders.WithData(localMap)); err != nil {
				return "", errors.Wrapf(err, "failed to re-render caddyfile entry for live namespace %q", liveNS)
			}
			return rendered, nil
		}).(sdk.StringOutput)
		serviceAnnotations[AnnotationCaddyfileEntry] = caddyfileEntry
	}

	servicePorts := corev1.ServicePortArray{}
	if args.IngressContainer != nil {
		for _, p := range lo.FromPtr(args.IngressContainer).Ports {
			servicePorts = append(servicePorts, corev1.ServicePortArgs{
				Name: sdk.String(toPortName(p)),
				Port: sdk.Int(p),
			})
		}
	}
	// Build the Pulumi-input annotation map. The caddyfile-entry value, if
	// any, is an Output that resolves the namespace placeholder against the
	// live Namespace resource (so IgnoreChanges'd migrated stacks point at
	// the legacy shared namespace, fresh deploys point at the per-stackEnv
	// namespace). Everything else is a static string.
	serviceAnnotationsInput := sdk.StringMap{}
	for k, v := range serviceAnnotations {
		serviceAnnotationsInput[k] = sdk.String(v)
	}
	if caddyfileEntryAnnotation != nil {
		serviceAnnotationsInput[AnnotationCaddyfileEntry] = caddyfileEntryAnnotation
	}
	var service *corev1.Service
	if len(lo.FromPtr(args.IngressContainer).Ports) > 0 {
		service, err = corev1.NewService(ctx, sanitizedService, &corev1.ServiceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(sanitizedService),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: serviceAnnotationsInput,
			},
			Spec: &corev1.ServiceSpecArgs{
				Selector:              sdk.ToStringMap(appLabels),
				Ports:                 servicePorts,
				Type:                  serviceType,
				ExternalTrafficPolicy: lo.If(args.ExternalTrafficPolicy != nil, sdk.StringPtr(lo.FromPtr(args.ExternalTrafficPolicy))).Else(nil),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
	}

	// Optional ingress for service
	if args.ProvisionIngress {
		if mainPort == nil {
			return nil, errors.Errorf("cannot provision ingress when no main port is specified")
		}
		// Mirror the Service-side annotation map (Pulumi-input with the
		// live-namespace caddyfile-entry Output) and overlay the
		// Ingress-only ssl-redirect tweak.
		ingressAnnotationsInput := sdk.StringMap{}
		for k, v := range serviceAnnotations {
			ingressAnnotationsInput[k] = sdk.String(v)
		}
		if caddyfileEntryAnnotation != nil {
			ingressAnnotationsInput[AnnotationCaddyfileEntry] = caddyfileEntryAnnotation
		}
		if args.UseSSL {
			ingressAnnotationsInput["ingress.kubernetes.io/ssl-redirect"] = sdk.String("false") // do not need ssl redirect from kube
		}
		_, err = networkv1.NewIngress(ctx, sanitizedService, &networkv1.IngressArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(sanitizedService),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: ingressAnnotationsInput,
			},
			Spec: &networkv1.IngressSpecArgs{
				Rules: networkv1.IngressRuleArray{
					networkv1.IngressRuleArgs{
						Http: networkv1.HTTPIngressRuleValueArgs{
							Paths: networkv1.HTTPIngressPathArray{
								networkv1.HTTPIngressPathArgs{
									Backend: networkv1.IngressBackendArgs{
										Service: networkv1.IngressServiceBackendArgs{
											Name: sdk.String(sanitizedService),
											Port: networkv1.ServiceBackendPortArgs{
												Number: sdk.Int(*mainPort),
											},
										},
									},
									Path:     sdk.String("/"),
									PathType: sdk.String("Prefix"),
								},
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision ingress for service")
		}
	}

	// Optional Pod Disruption Budget
	if args.PodDisruption != nil {
		pdbArgs := policyv1.PodDisruptionBudgetSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: sdk.ToStringMap(appLabels),
			},
		}
		if args.PodDisruption.MinAvailable != nil {
			pdbArgs.MinAvailable = sdk.IntPtrFromPtr(args.PodDisruption.MinAvailable)
		} else if args.PodDisruption.MaxUnavailable != nil {
			pdbArgs.MaxUnavailable = sdk.IntPtrFromPtr(args.PodDisruption.MaxUnavailable)
		}
		_, err := policyv1.NewPodDisruptionBudget(ctx, fmt.Sprintf("%s-pdb", sanitizedDeployment), &policyv1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Spec: &pdbArgs,
		}, opts...)
		if err != nil {
			return nil, err
		}
	}

	sc.Service = service
	sc.CaddyfileEntry = sdk.String(caddyfileEntry)
	if service != nil {
		sc.ServicePublicIP = service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
			if status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
				args.Log.Warn(ctx.Context(), "failed to read load balancer IP: there is no ingress IP found")
				return ""
			}
			ip := lo.FromPtr(status.LoadBalancer.Ingress[0].Ip)
			args.Log.Info(ctx.Context(), "load balancer ip is %v", ip)
			return ip
		}).(sdk.StringOutput)
		sc.ServiceName = service.Metadata.Name().Elem().ToStringPtrOutput()
	}
	sc.Namespace = namespace.Metadata.Name().Elem()
	if mainPort != nil {
		sc.Port = sdk.IntPtrFromPtr(mainPort).ToIntPtrOutput()
	}

	sc.Deployment = deployment
	// run post-processors after sc is created
	if args.ComputeContext != nil {
		if err := args.ComputeContext.RunPostProcessors(sc, sc); err != nil {
			return nil, err
		}
	}

	err = ctx.RegisterComponentResource("simple-container.com:k8s:SimpleContainer", sanitizedService, sc, opts...)
	if err != nil {
		return nil, err
	}

	// Create VPA if enabled. Pass the live namespace name (Pulumi Output)
	// rather than the program-computed sanitizedNamespace string, so the
	// VPA lands in the same namespace as its target Deployment on migrated
	// stacks (where IgnoreChanges("metadata.name") keeps the namespace at
	// the legacy shared value).
	if args.VPA != nil && args.VPA.Enabled {
		if err := createVPA(ctx, args, baseResourceName, namespace.Metadata.Name().Elem(), appLabels, appAnnotations, opts...); err != nil {
			return nil, errors.Wrapf(err, "failed to create VPA for deployment %s", baseResourceName)
		}
	}

	// Create HPA if enabled (validation already done in deployment.go)
	if args.Scale != nil && args.Scale.EnableHPA {
		hpaArgs := &HPAArgs{
			Name:         baseResourceName, // Uses parentEnv-aware name
			Deployment:   deployment,
			MinReplicas:  args.Scale.MinReplicas,
			MaxReplicas:  args.Scale.MaxReplicas,
			CPUTarget:    args.Scale.CPUTarget,
			MemoryTarget: args.Scale.MemoryTarget,
			Namespace:    namespace,
			Labels:       appLabels,
			Annotations:  appAnnotations,
			Opts:         opts,
		}

		hpa, err := CreateHPA(ctx, hpaArgs)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create HPA for deployment %s", sanitizedDeployment)
		}

		args.Log.Info(ctx.Context(), "✅ Created HPA %s with min=%d, max=%d replicas",
			hpa.Metadata.Name(), args.Scale.MinReplicas, args.Scale.MaxReplicas)
	}

	err = ctx.RegisterResourceOutputs(sc, sdk.Map{
		"servicePublicIP": sc.ServicePublicIP,
		"serviceName":     sc.ServiceName,
		"namespace":       sc.Namespace,
		"port":            sc.Port,
		"caddyfileEntry":  sc.CaddyfileEntry,
		"deployment":      sc.Deployment,
	})
	if err != nil {
		return nil, err
	}

	return sc, nil
}

func createVPA(ctx *sdk.Context, args *SimpleContainerArgs, deploymentName string, namespace sdk.StringInput, labels, annotations map[string]string, opts ...sdk.ResourceOption) error {
	vpaName := fmt.Sprintf("%s-vpa", deploymentName)

	// Build VPA spec content
	vpaSpec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deploymentName,
		},
	}

	// Add update policy if specified
	if args.VPA.UpdateMode != nil {
		vpaSpec["updatePolicy"] = map[string]interface{}{
			"updateMode": lo.FromPtr(args.VPA.UpdateMode),
		}
	}

	// Add resource policy if specified
	if args.VPA.MinAllowed != nil || args.VPA.MaxAllowed != nil || len(args.VPA.ControlledResources) > 0 {
		resourcePolicy := map[string]interface{}{}

		// Add controlled resources
		if len(args.VPA.ControlledResources) > 0 {
			resourcePolicy["controlledResources"] = args.VPA.ControlledResources
		}

		// Add container policies
		containerPolicy := map[string]interface{}{
			"containerName": "*",
		}

		if args.VPA.MinAllowed != nil {
			minAllowed := map[string]interface{}{}
			if args.VPA.MinAllowed.CPU != nil {
				minAllowed["cpu"] = lo.FromPtr(args.VPA.MinAllowed.CPU)
			}
			if args.VPA.MinAllowed.Memory != nil {
				minAllowed["memory"] = lo.FromPtr(args.VPA.MinAllowed.Memory)
			}
			if args.VPA.MinAllowed.EphemeralStorage != nil {
				minAllowed["ephemeral-storage"] = lo.FromPtr(args.VPA.MinAllowed.EphemeralStorage)
			}
			if len(minAllowed) > 0 {
				containerPolicy["minAllowed"] = minAllowed
			}
		}

		if args.VPA.MaxAllowed != nil {
			maxAllowed := map[string]interface{}{}
			if args.VPA.MaxAllowed.CPU != nil {
				maxAllowed["cpu"] = lo.FromPtr(args.VPA.MaxAllowed.CPU)
			}
			if args.VPA.MaxAllowed.Memory != nil {
				maxAllowed["memory"] = lo.FromPtr(args.VPA.MaxAllowed.Memory)
			}
			if args.VPA.MaxAllowed.EphemeralStorage != nil {
				maxAllowed["ephemeral-storage"] = lo.FromPtr(args.VPA.MaxAllowed.EphemeralStorage)
			}
			if len(maxAllowed) > 0 {
				containerPolicy["maxAllowed"] = maxAllowed
			}
		}

		resourcePolicy["containerPolicies"] = []interface{}{containerPolicy}
		vpaSpec["resourcePolicy"] = resourcePolicy
	}

	// Build the complete VPA resource with proper spec nesting
	spec := kubernetes.UntypedArgs{
		"spec": vpaSpec,
	}

	// Merge common labels with VPA-specific labels
	vpaLabels := make(map[string]string)
	for k, v := range labels {
		vpaLabels[k] = v
	}
	// Add VPA-specific labels
	vpaLabels["app.kubernetes.io/component"] = "vpa"
	vpaLabels["app.kubernetes.io/managed-by"] = "simple-container"

	// Use common annotations (VPA doesn't typically need specific annotations)
	vpaAnnotations := make(map[string]string)
	for k, v := range annotations {
		vpaAnnotations[k] = v
	}

	// Create VPA custom resource
	_, err := apiextensions.NewCustomResource(ctx, vpaName, &apiextensions.CustomResourceArgs{
		ApiVersion: sdk.String("autoscaling.k8s.io/v1"),
		Kind:       sdk.String("VerticalPodAutoscaler"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(vpaName),
			Namespace:   namespace,
			Labels:      sdk.ToStringMap(vpaLabels),
			Annotations: sdk.ToStringMap(vpaAnnotations),
		},
		OtherFields: spec,
	}, opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to create VPA resource")
	}

	args.Log.Info(ctx.Context(), "Created VPA %s for deployment %s", vpaName, deploymentName)
	return nil
}

func ToImagePullSecretName(deploymentName string) string {
	return fmt.Sprintf("%s-docker-config", deploymentName)
}

func ToSecretVolumesName(deploymentName string) string {
	return fmt.Sprintf("%s-secret-volumes", deploymentName)
}

func ToEnvConfigName(deploymentName string) string {
	return fmt.Sprintf("%s-env", deploymentName)
}

func ToConfigVolumesName(deploymentName string) string {
	return fmt.Sprintf("%s-cfg-volumes", deploymentName)
}

// Helper functions for volume mounts
func addVolumeMounts(volumeName string, volumes []k8s.SimpleTextVolume, volumeMounts *corev1.VolumeMountArray) {
	for _, volume := range volumes {
		*volumeMounts = append(*volumeMounts, corev1.VolumeMountArgs{
			Name:      sdk.String(volumeName),
			MountPath: sdk.String(volume.MountPath),
			SubPath:   sdk.String(volume.Name),
		})
	}
}

func addVolumeMountsFromOutputs(volumeName string, volumes []any, volumeMounts *corev1.VolumeMountArray) {
	for _, vol := range volumes {
		volOut := vol.(sdk.Output)
		*volumeMounts = append(*volumeMounts, volOut.ApplyT(func(vol any) corev1.VolumeMount {
			sv := vol.(k8s.SimpleTextVolume)
			return corev1.VolumeMount{
				Name:      volumeName,
				MountPath: sv.MountPath,
				SubPath:   lo.ToPtr(sv.Name),
			}
		}).(corev1.VolumeMountOutput))
	}
}

// convertAffinityRulesToKubernetes converts Simple Container affinity rules to Kubernetes affinity
func convertAffinityRulesToKubernetes(affinity *k8s.AffinityRules) *corev1.AffinityArgs {
	if affinity == nil {
		return nil
	}

	kubeAffinity := &corev1.AffinityArgs{}

	// Convert node affinity
	if affinity.NodeAffinity != nil {
		kubeAffinity.NodeAffinity = convertNodeAffinity(affinity.NodeAffinity)
	}

	// Convert pod affinity
	if affinity.PodAffinity != nil {
		kubeAffinity.PodAffinity = convertPodAffinity(affinity.PodAffinity)
	}

	// Convert pod anti-affinity
	if affinity.PodAntiAffinity != nil {
		kubeAffinity.PodAntiAffinity = convertPodAntiAffinity(affinity.PodAntiAffinity)
	}

	// Handle Space Pay specific rules for exclusive node pool
	if affinity.ExclusiveNodePool != nil && *affinity.ExclusiveNodePool && affinity.NodePool != nil {
		// Create node affinity to require the specific node pool
		if kubeAffinity.NodeAffinity == nil {
			kubeAffinity.NodeAffinity = &corev1.NodeAffinityArgs{}
		}

		nodePoolRequirement := corev1.NodeSelectorRequirementArgs{
			Key:      sdk.String("cloud.google.com/gke-nodepool"),
			Operator: sdk.String("In"),
			Values:   sdk.StringArray{sdk.String(*affinity.NodePool)},
		}

		// Create a new node affinity with the exclusive node pool requirement
		nodeAffinityArgs := &corev1.NodeAffinityArgs{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelectorArgs{
				NodeSelectorTerms: corev1.NodeSelectorTermArray{
					corev1.NodeSelectorTermArgs{
						MatchExpressions: corev1.NodeSelectorRequirementArray{nodePoolRequirement},
					},
				},
			},
		}

		// Override existing node affinity with exclusive node pool requirement
		kubeAffinity.NodeAffinity = nodeAffinityArgs
	}

	return kubeAffinity
}

// convertNodeAffinity converts Simple Container node affinity to Kubernetes node affinity
func convertNodeAffinity(nodeAffinity *k8s.NodeAffinity) *corev1.NodeAffinityArgs {
	if nodeAffinity == nil {
		return nil
	}

	kubeNodeAffinity := &corev1.NodeAffinityArgs{}

	// Convert required node affinity
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		kubeNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = convertNodeSelector(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution)
	}

	// Convert preferred node affinity
	if len(nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
		preferredTerms := make(corev1.PreferredSchedulingTermArray, len(nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution))
		for i, term := range nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			preferredTerms[i] = corev1.PreferredSchedulingTermArgs{
				Weight:     sdk.Int(int(term.Weight)),
				Preference: convertNodeSelectorTerm(term.Preference),
			}
		}
		kubeNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = preferredTerms
	}

	return kubeNodeAffinity
}

// convertNodeSelector converts Simple Container node selector to Kubernetes node selector
func convertNodeSelector(nodeSelector *k8s.NodeSelector) *corev1.NodeSelectorArgs {
	if nodeSelector == nil {
		return nil
	}

	terms := make(corev1.NodeSelectorTermArray, len(nodeSelector.NodeSelectorTerms))
	for i, term := range nodeSelector.NodeSelectorTerms {
		terms[i] = convertNodeSelectorTerm(term)
	}

	return &corev1.NodeSelectorArgs{
		NodeSelectorTerms: terms,
	}
}

// convertNodeSelectorTerm converts Simple Container node selector term to Kubernetes node selector term
func convertNodeSelectorTerm(term k8s.NodeSelectorTerm) corev1.NodeSelectorTermArgs {
	kubeTerm := corev1.NodeSelectorTermArgs{}

	// Convert match expressions
	if len(term.MatchExpressions) > 0 {
		matchExpressions := make(corev1.NodeSelectorRequirementArray, len(term.MatchExpressions))
		for i, expr := range term.MatchExpressions {
			values := make(sdk.StringArray, len(expr.Values))
			for j, val := range expr.Values {
				values[j] = sdk.String(val)
			}
			matchExpressions[i] = corev1.NodeSelectorRequirementArgs{
				Key:      sdk.String(expr.Key),
				Operator: sdk.String(expr.Operator),
				Values:   values,
			}
		}
		kubeTerm.MatchExpressions = matchExpressions
	}

	// Convert match fields
	if len(term.MatchFields) > 0 {
		matchFields := make(corev1.NodeSelectorRequirementArray, len(term.MatchFields))
		for i, field := range term.MatchFields {
			values := make(sdk.StringArray, len(field.Values))
			for j, val := range field.Values {
				values[j] = sdk.String(val)
			}
			matchFields[i] = corev1.NodeSelectorRequirementArgs{
				Key:      sdk.String(field.Key),
				Operator: sdk.String(field.Operator),
				Values:   values,
			}
		}
		kubeTerm.MatchFields = matchFields
	}

	return kubeTerm
}

// convertPodAffinity converts Simple Container pod affinity to Kubernetes pod affinity
func convertPodAffinity(podAffinity *k8s.PodAffinity) *corev1.PodAffinityArgs {
	if podAffinity == nil {
		return nil
	}

	kubePodAffinity := &corev1.PodAffinityArgs{}

	// Convert required pod affinity
	if len(podAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
		requiredTerms := make(corev1.PodAffinityTermArray, len(podAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
		for i, term := range podAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			requiredTerms[i] = convertPodAffinityTerm(term)
		}
		kubePodAffinity.RequiredDuringSchedulingIgnoredDuringExecution = requiredTerms
	}

	// Convert preferred pod affinity
	if len(podAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
		preferredTerms := make(corev1.WeightedPodAffinityTermArray, len(podAffinity.PreferredDuringSchedulingIgnoredDuringExecution))
		for i, term := range podAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			preferredTerms[i] = corev1.WeightedPodAffinityTermArgs{
				Weight:          sdk.Int(int(term.Weight)),
				PodAffinityTerm: convertPodAffinityTerm(term.PodAffinityTerm),
			}
		}
		kubePodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = preferredTerms
	}

	return kubePodAffinity
}

// convertPodAntiAffinity converts Simple Container pod anti-affinity to Kubernetes pod anti-affinity
func convertPodAntiAffinity(podAntiAffinity *k8s.PodAffinity) *corev1.PodAntiAffinityArgs {
	if podAntiAffinity == nil {
		return nil
	}

	kubePodAntiAffinity := &corev1.PodAntiAffinityArgs{}

	// Convert required pod anti-affinity
	if len(podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
		requiredTerms := make(corev1.PodAffinityTermArray, len(podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
		for i, term := range podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			requiredTerms[i] = convertPodAffinityTerm(term)
		}
		kubePodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = requiredTerms
	}

	// Convert preferred pod anti-affinity
	if len(podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
		preferredTerms := make(corev1.WeightedPodAffinityTermArray, len(podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution))
		for i, term := range podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			preferredTerms[i] = corev1.WeightedPodAffinityTermArgs{
				Weight:          sdk.Int(int(term.Weight)),
				PodAffinityTerm: convertPodAffinityTerm(term.PodAffinityTerm),
			}
		}
		kubePodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = preferredTerms
	}

	return kubePodAntiAffinity
}

// convertPodAffinityTerm converts Simple Container pod affinity term to Kubernetes pod affinity term
func convertPodAffinityTerm(term k8s.PodAffinityTerm) corev1.PodAffinityTermArgs {
	kubeTerm := corev1.PodAffinityTermArgs{
		TopologyKey: sdk.String(term.TopologyKey),
	}

	// Convert label selector
	if term.LabelSelector != nil {
		kubeTerm.LabelSelector = convertLabelSelector(term.LabelSelector)
	}

	// Convert namespaces
	if len(term.Namespaces) > 0 {
		namespaces := make(sdk.StringArray, len(term.Namespaces))
		for i, ns := range term.Namespaces {
			namespaces[i] = sdk.String(ns)
		}
		kubeTerm.Namespaces = namespaces
	}

	return kubeTerm
}

// convertLabelSelector converts Simple Container label selector to Kubernetes label selector
func convertLabelSelector(labelSelector *k8s.LabelSelector) *metav1.LabelSelectorArgs {
	if labelSelector == nil {
		return nil
	}

	kubeLabelSelector := &metav1.LabelSelectorArgs{}

	// Convert match labels
	if len(labelSelector.MatchLabels) > 0 {
		kubeLabelSelector.MatchLabels = sdk.ToStringMap(labelSelector.MatchLabels)
	}

	// Convert match expressions
	if len(labelSelector.MatchExpressions) > 0 {
		matchExpressions := make(metav1.LabelSelectorRequirementArray, len(labelSelector.MatchExpressions))
		for i, expr := range labelSelector.MatchExpressions {
			values := make(sdk.StringArray, len(expr.Values))
			for j, val := range expr.Values {
				values[j] = sdk.String(val)
			}
			matchExpressions[i] = metav1.LabelSelectorRequirementArgs{
				Key:      sdk.String(expr.Key),
				Operator: sdk.String(expr.Operator),
				Values:   values,
			}
		}
		kubeLabelSelector.MatchExpressions = matchExpressions
	}

	return kubeLabelSelector
}
