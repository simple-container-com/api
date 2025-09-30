package kubernetes

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

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

	LabelAppType = "appType"
	LabelAppName = "appName"
	LabelScEnv   = "appEnv"
)

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
	Replicas               int     `json:"replicas" yaml:"replicas"`
	GenerateCaddyfileEntry bool    `json:"generateCaddyfileEntry" yaml:"generateCaddyfileEntry"`
	KubeProvider           sdk.ProviderResource

	// optional properties
	PodDisruption     *k8s.DisruptionBudget        `json:"podDisruption" yaml:"podDisruption"`
	LbConfig          *api.SimpleContainerLBConfig `json:"lbConfig" yaml:"lbConfig"`
	SecretEnvs        map[string]string            `json:"secretEnvs" yaml:"secretEnvs"`
	Annotations       map[string]string            `json:"annotations" yaml:"annotations"`
	NodeSelector      map[string]string            `json:"nodeSelector" yaml:"nodeSelector"`
	Affinity          *k8s.AffinityRules           `json:"affinity" yaml:"affinity"`
	IngressContainer  *k8s.CloudRunContainer       `json:"ingressContainer" yaml:"ingressContainer"`
	ServiceType       *string                      `json:"serviceType" yaml:"serviceType"`
	ProvisionIngress  bool                         `json:"provisionIngress" yaml:"provisionIngress"`
	Headers           *k8s.Headers                 `json:"headers" yaml:"headers"`
	Volumes           []k8s.SimpleTextVolume       `json:"volumes" yaml:"volumes"`
	SecretVolumes     []k8s.SimpleTextVolume       `json:"secretVolumes" yaml:"secretVolumes"`
	PersistentVolumes []k8s.PersistentVolume       `json:"persistentVolumes" yaml:"persistentVolumes"`

	Log logger.Logger
	// ...
	RollingUpdate        *v1.RollingUpdateDeploymentArgs
	InitContainers       []corev1.ContainerArgs
	Containers           []corev1.ContainerArgs
	SecurityContext      *corev1.PodSecurityContextArgs
	ServiceAccountName   *sdk.StringOutput
	Sidecars             []corev1.ContainerArgs
	SidecarOutputs       []corev1.ContainerOutput
	InitContainerOutputs []corev1.ContainerOutput
	VolumeOutputs        []corev1.VolumeOutput
	SecretVolumeOutputs  []any
	ComputeContext       pApi.ComputeContext
	ImagePullSecret      *docker.RegistryCredentials
	UseSSL               bool
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

	appLabels := map[string]string{
		LabelAppType: AppTypeSimpleContainer,
		LabelAppName: args.Service,
		LabelScEnv:   args.ScEnv,
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
	namespace, err := corev1.NewNamespace(ctx, args.Namespace, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(args.Namespace),
			Labels:      sdk.ToStringMap(appLabels),
			Annotations: sdk.ToStringMap(appAnnotations),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

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

	volumesCfgName := ToConfigVolumesName(args.Deployment)
	envSecretName := ToEnvConfigName(args.Deployment)
	volumesSecretName := ToSecretVolumesName(args.Deployment)
	imagePullSecretName := ToImagePullSecretName(args.Deployment)

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
			EmptyDir: corev1.EmptyDirVolumeSourceArgs{},
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
		_, err := corev1.NewPersistentVolumeClaim(ctx, pv.Name, &corev1.PersistentVolumeClaimArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(pv.Name),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Spec: &corev1.PersistentVolumeClaimSpecArgs{
				AccessModes:      sdk.StringArray(accessModes),
				StorageClassName: sdk.StringPtrFromPtr(pv.StorageClassName),
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
			Name: sdk.String(pv.Name),
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{
				ClaimName: sdk.String(pv.Name),
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMountArgs{
			Name:      sdk.String(pv.Name),
			MountPath: sdk.String(pv.MountPath),
		})
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
	podSpecArgs := &corev1.PodSpecArgs{
		NodeSelector: sdk.ToStringMap(args.NodeSelector),
		Affinity:     convertAffinityRulesToKubernetes(args.Affinity),
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
	if imagePullSecret != nil {
		podSpecArgs.ImagePullSecrets = corev1.LocalObjectReferenceArray{
			corev1.LocalObjectReferenceArgs{
				Name: imagePullSecret.Metadata.Name(),
			},
		}
	}
	deployment, err := v1.NewDeployment(ctx, args.Deployment, &v1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        sdk.String(args.Deployment),
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
	if args.GenerateCaddyfileEntry && mainPort != nil {
		if args.Domain != "" {
			caddyfileEntry = `
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
			caddyfileEntry = `
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
			"service":   args.Service,
			"namespace": args.Namespace,
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
		if err := placeholders.New().Apply(&caddyfileEntry, placeholders.WithData(placeholdersMap)); err != nil {
			return nil, errors.Wrapf(err, "failed to apply placeholders on caddyfile entry template")
		}
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
	var service *corev1.Service
	if len(lo.FromPtr(args.IngressContainer).Ports) > 0 {
		service, err = corev1.NewService(ctx, args.Service, &corev1.ServiceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(args.Service),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(serviceAnnotations),
			},
			Spec: &corev1.ServiceSpecArgs{
				Selector: sdk.ToStringMap(appLabels),
				Ports:    servicePorts,
				Type:     serviceType,
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
		ingressAnnotations := lo.Assign(serviceAnnotations)
		if args.UseSSL {
			ingressAnnotations["ingress.kubernetes.io/ssl-redirect"] = "false" // do not need ssl redirect from kube
		}
		_, err = networkv1.NewIngress(ctx, args.Service, &networkv1.IngressArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(args.Service),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(ingressAnnotations),
			},
			Spec: &networkv1.IngressSpecArgs{
				Rules: networkv1.IngressRuleArray{
					networkv1.IngressRuleArgs{
						Http: networkv1.HTTPIngressRuleValueArgs{
							Paths: networkv1.HTTPIngressPathArray{
								networkv1.HTTPIngressPathArgs{
									Backend: networkv1.IngressBackendArgs{
										Service: networkv1.IngressServiceBackendArgs{
											Name: sdk.String(args.Service),
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
		_, err := policyv1.NewPodDisruptionBudget(ctx, fmt.Sprintf("%s-pdb", args.Deployment), &policyv1.PodDisruptionBudgetArgs{
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

	err = ctx.RegisterComponentResource("simple-container.com:k8s:SimpleContainer", args.Service, sc, opts...)
	if err != nil {
		return nil, err
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
