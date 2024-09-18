package kubernetes

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	policyv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

const (
	AppTypeSimpleContainer = "simple-container"

	AnnotationCaddyfileEntry = "simple-container.com/caddyfile-entry"
	AnnotationParentStack    = "simple-container.com/parent-stack"
	AnnotationDomain         = "simple-container.com/domain"
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
	Deployment             string  `json:"deployment" yaml:"deployment"`
	ParentStack            *string `json:"parentStack" yaml:"parentStack"`
	Replicas               int     `json:"replicas" yaml:"replicas"`
	GenerateCaddyfileEntry bool    `json:"generateCaddyfileEntry" yaml:"generateCaddyfileEntry"`

	// optional properties
	PodDisruption     *k8s.DisruptionBudget        `json:"podDisruption" yaml:"podDisruption"`
	LbConfig          *api.SimpleContainerLBConfig `json:"lbConfig" yaml:"lbConfig"`
	SecretEnvs        map[string]string            `json:"secretEnvs" yaml:"secretEnvs"`
	Annotations       map[string]string            `json:"annotations" yaml:"annotations"`
	NodeSelector      map[string]string            `json:"nodeSelector" yaml:"nodeSelector"`
	IngressContainer  *k8s.CloudRunContainer       `json:"ingressContainer" yaml:"ingressContainer"`
	ServiceType       *string                      `json:"serviceType" yaml:"serviceType"`
	Headers           *k8s.Headers                 `json:"headers" yaml:"headers"`
	Volumes           []k8s.SimpleTextVolume       `json:"volumes" yaml:"volumes"`
	SecretVolumes     []k8s.SimpleTextVolume       `json:"secretVolumes" yaml:"secretVolumes"`
	PersistentVolumes []k8s.PersistentVolume       `json:"persistentVolumes" yaml:"persistentVolumes"`

	// ...
	RollingUpdate      *v1.RollingUpdateDeploymentArgs
	InitContainers     []corev1.ContainerArgs
	Containers         []corev1.ContainerArgs
	SecurityContext    *corev1.PodSecurityContextArgs
	ServiceAccountName *sdk.StringOutput
}

type SimpleContainer struct {
	sdk.ResourceState

	ServicePublicIP    sdk.StringPtrOutput `pulumi:"servicePublicIP"`
	ServiceName        sdk.StringOutput    `pulumi:"serviceName"`
	Namespace          sdk.StringOutput    `pulumi:"namespace"`
	Port               sdk.IntPtrOutput    `pulumi:"port"`
	CaddyfileEntry     sdk.StringOutput    `pulumi:"caddyfileEntry"`
	RequestedResources sdk.Input           `pulumi:"registeredResources"`
	Deployment         *v1.Deployment      `pulumi:"deployment"`
}

func NewSimpleContainer(ctx *sdk.Context, args *SimpleContainerArgs, opts ...sdk.ResourceOption) (*SimpleContainer, error) {
	sc := &SimpleContainer{}
	err := ctx.RegisterComponentResource("pkg:k8s/extensions:simpleContainer", args.Service, sc, opts...)
	if err != nil {
		return nil, err
	}

	appLabels := map[string]string{
		LabelAppType: AppTypeSimpleContainer,
		LabelAppName: args.Service,
		LabelScEnv:   args.ScEnv,
	}

	appAnnotations := map[string]string{
		AnnotationDomain: args.Domain,
		AnnotationEnv:    args.ScEnv,
	}
	var mainPort *int
	if args.IngressContainer != nil && args.IngressContainer.MainPort != nil {
		mainPort = args.IngressContainer.MainPort
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
		StringData: sdk.ToStringMap(secretVolumeToData),
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Volume Mounts
	var volumeMounts corev1.VolumeMountArray
	addVolumeMounts(volumesSecretName, args.SecretVolumes, &volumeMounts)
	addVolumeMounts(volumesCfgName, args.Volumes, &volumeMounts)

	for _, container := range args.Containers {
		// Update container volume mounts and envFrom
		container.VolumeMounts = volumeMounts
		container.EnvFrom = corev1.EnvFromSourceArray{
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
	for _, c := range args.InitContainers {
		initContainers = append(initContainers, c)
	}

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
		_, err := corev1.NewPersistentVolumeClaim(ctx, pv.Name, &corev1.PersistentVolumeClaimArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:        sdk.String(pv.Name),
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Spec: &corev1.PersistentVolumeClaimSpecArgs{
				AccessModes: sdk.StringArray([]sdk.StringInput{sdk.String("ReadWriteOnce")}),
				Resources: &corev1.VolumeResourceRequirementsArgs{
					Requests: sdk.StringMap{
						"storage": sdk.String(pv.Storage),
					},
				},
				StorageClassName: sdk.String("standard"),
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

	// Deployment
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
				Spec: &corev1.PodSpecArgs{
					NodeSelector:       sdk.ToStringMap(args.NodeSelector),
					InitContainers:     initContainers,
					Containers:         containers,
					Volumes:            volumes,
					SecurityContext:    args.SecurityContext,
					ServiceAccountName: args.ServiceAccountName,
				},
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

	if args.GenerateCaddyfileEntry && args.IngressContainer != nil && mainPort != nil {
		caddyfileEntry := `
${proto}://${domain} {
  reverse_proxy http://${service}.${namespace}.svc.cluster.local:${port} {
    header_down Server nginx ${addHeaders}
    import handle_server_error
    ${extraHelpers}
  }
  import hsts
  import gzip
  import handle_static
  import cors
}
`
		if err := placeholders.New().Apply(&caddyfileEntry, placeholders.WithData((placeholders.MapData{
			"proto":     lo.If(lo.FromPtr(args.LbConfig).Https, "https").Else("http"),
			"domain":    args.Domain,
			"service":   args.Service,
			"namespace": args.Namespace,
			"port":      strconv.Itoa(lo.FromPtr(mainPort)),
			"addHeaders": strings.Join(lo.Map(lo.Entries(lo.FromPtr(args.Headers)), func(h lo.Entry[string, string], _ int) string {
				return fmt.Sprintf("header_down %s %s", h.Key, h.Value)
			}), "\n    "),
			"extraHelpers": strings.Join(lo.FromPtr(args.LbConfig).ExtraHelpers, "\n    "),
		}))); err != nil {
			return nil, errors.Wrapf(err, "failed to apply placeholders on caddyfile entry template")
		}
		serviceAnnotations[AnnotationCaddyfileEntry] = caddyfileEntry
	}

	servicePorts := corev1.ServicePortArray{}
	if args.IngressContainer != nil {
		for _, p := range args.IngressContainer.Ports {
			servicePorts = append(servicePorts, corev1.ServicePortArgs{
				Name: sdk.String(toPortName(p)),
				Port: sdk.Int(p),
			})
		}
	}
	service, err := corev1.NewService(ctx, args.Service, &corev1.ServiceArgs{
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

	// Optional Pod Disruption Budget
	if args.PodDisruption != nil {
		_, err := policyv1.NewPodDisruptionBudget(ctx, fmt.Sprintf("%s-pdb", args.Deployment), &policyv1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Namespace:   namespace.Metadata.Name().Elem(),
				Labels:      sdk.ToStringMap(appLabels),
				Annotations: sdk.ToStringMap(appAnnotations),
			},
			Spec: &policyv1.PodDisruptionBudgetSpecArgs{
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: sdk.ToStringMap(appLabels),
				},
				MaxUnavailable: sdk.Int(args.PodDisruption.MaxUnavailable),
				MinAvailable:   sdk.Int(args.PodDisruption.MinAvailable),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
	}

	servicePublicIP := service.Status.ApplyT(func(status *corev1.ServiceStatus) *string {
		if status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
			return nil
		}
		return status.LoadBalancer.Ingress[0].Ip
	}).(sdk.StringPtrOutput)

	sc.ServicePublicIP = servicePublicIP
	sc.ServiceName = service.Metadata.Name().Elem()
	sc.Namespace = namespace.Metadata.Name().Elem()
	if mainPort != nil {
		sc.Port = sdk.IntPtrFromPtr(mainPort).ToIntPtrOutput()
	}
	sc.Deployment = deployment
	err = ctx.RegisterResourceOutputs(sc, sdk.Map{
		"servicePublicIP":     sc.ServicePublicIP,
		"serviceName":         sc.ServiceName,
		"namespace":           sc.Namespace,
		"port":                sc.Port,
		"caddyfileEntry":      sc.CaddyfileEntry,
		"registeredResources": sc.RequestedResources,
		"deploymentName":      sc.Deployment,
	})
	if err != nil {
		return nil, err
	}

	return sc, nil
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
