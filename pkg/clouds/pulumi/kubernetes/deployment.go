package kubernetes

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type Args struct {
	Namespace              string
	DeploymentName         string
	Annotations            map[string]string
	NodeSelector           map[string]string
	Affinity               *k8s.AffinityRules
	Tolerations            []k8s.Toleration
	Input                  api.ResourceInput
	Deployment             k8s.DeploymentConfig
	Images                 []*ContainerImage
	Params                 pApi.ProvisionParams
	ServiceAccountName     *sdk.StringOutput
	KubeProvider           sdk.ProviderResource
	InitContainers         []corev1.ContainerArgs
	GenerateCaddyfileEntry bool
	ServiceType            *string
	Sidecars               []corev1.ContainerArgs
	ComputeContext         pApi.ComputeContext
	SecretVolumes          []k8s.SimpleTextVolume
	SecretVolumeOutputs    []any
	SecretEnvs             map[string]string
	ImagePullSecret        *docker.RegistryCredentials
	ProvisionIngress       bool
	UseSSL                 bool
	VPA                    *k8s.VPAConfig     // Vertical Pod Autoscaler configuration
	ReadinessProbe         *k8s.CloudRunProbe // Global readiness probe configuration
	LivenessProbe          *k8s.CloudRunProbe // Global liveness probe configuration
}

func DeploySimpleContainer(ctx *sdk.Context, args Args, opts ...sdk.ResourceOption) (*SimpleContainer, error) {
	stackName := args.Input.StackParams.StackName
	stackEnv := args.Input.StackParams.Environment

	// Extract parentEnv from ParentStack if available
	var parentEnv string
	if args.Params.ParentStack != nil {
		parentEnv = args.Params.ParentStack.ParentEnv
	}

	// Determine namespace - always use stack name as namespace (service name)
	namespace := lo.If(args.Namespace == "", stackName).Else(args.Namespace)

	// Generate deployment name with environment suffix for custom stacks
	baseDeploymentName := lo.If(args.DeploymentName == "", stackName).Else(args.DeploymentName)
	deploymentName := generateDeploymentName(baseDeploymentName, stackEnv, parentEnv)

	args.Params.Log.Info(ctx.Context(), "📦 Deploying to namespace=%q, deployment=%q (stackEnv=%q, parentEnv=%q, isCustomStack=%v)",
		namespace, deploymentName, stackEnv, parentEnv, isCustomStack(stackEnv, parentEnv))

	opts = append(opts, sdk.Provider(args.KubeProvider), sdk.DependsOn(args.Params.ComputeContext.Dependencies()))

	replicas := 1
	if args.Deployment.Scale != nil {
		// TODO: support autoscaling
		replicas = args.Deployment.Scale.Replicas
	}

	// secret ENV
	contextSecretEnvs := lo.SliceToMap(args.Params.ComputeContext.SecretEnvVariables(), func(v pApi.ComputeEnvVariable) (string, string) {
		return v.Name, v.Value
	})
	secretEnvs := lo.Assign(contextSecretEnvs, args.Deployment.StackConfig.Secrets)

	// env
	contextEnvVars := lo.Filter(args.Params.ComputeContext.EnvVariables(), func(v pApi.ComputeEnvVariable, _ int) bool {
		_, exists := args.Deployment.StackConfig.Env[v.Name]
		return !exists
	})
	envVars := lo.SliceToMap(contextEnvVars, func(v pApi.ComputeEnvVariable) (string, string) {
		return v.Name, v.Value
	})

	// persistent volumes
	pvs := lo.FlatMap(args.Images, func(c *ContainerImage, _ int) []k8s.PersistentVolume {
		return c.Container.Volumes
	})

	containers, err := util.MapErr(args.Images, func(c *ContainerImage, _ int) (corev1.ContainerArgs, error) {
		for _, w := range c.Container.Warnings {
			args.Params.Log.Warn(ctx.Context(), "container %q warning: %s", c.Container.Name, w)
		}
		// Merge all env sources (container, context, stack config) and exclude secret env keys
		containerEnvVars := lo.Assign(c.Container.Env, envVars, args.Deployment.StackConfig.Env)
		containerEnvVars = lo.OmitByKeys(containerEnvVars, lo.Keys(secretEnvs))

		// Convert to Kubernetes env var array with sorted keys to ensure consistent ordering
		var env corev1.EnvVarArray
		envKeys := lo.Keys(containerEnvVars)
		sort.Strings(envKeys)
		for _, k := range envKeys {
			env = append(env, corev1.EnvVarArgs{
				Name:  sdk.String(k),
				Value: sdk.String(containerEnvVars[k]),
			})
		}
		var ports corev1.ContainerPortArray
		var readinessProbe *corev1.ProbeArgs
		for _, p := range c.Container.Ports {
			portName := toPortName(p) // TODO: support non-http ports
			ports = append(ports, corev1.ContainerPortArgs{
				Name:          sdk.String(portName),
				ContainerPort: sdk.Int(p),
			})
		}
		cReadyProbe := c.Container.ReadinessProbe
		// Use global readiness probe if container doesn't have one AND it's the ingress container
		// This prevents applying HTTP/TCP probes to worker containers that don't expose ports
		isIngressContainer := args.Deployment.IngressContainer != nil && args.Deployment.IngressContainer.Name == c.Container.Name
		if cReadyProbe == nil && args.ReadinessProbe != nil && isIngressContainer {
			cReadyProbe = args.ReadinessProbe
		}

		if cReadyProbe == nil && len(c.Container.Ports) == 1 {
			readinessProbe = &corev1.ProbeArgs{
				TcpSocket: corev1.TCPSocketActionArgs{
					Port: sdk.String(toPortName(c.Container.Ports[0])),
				},
				PeriodSeconds:       sdk.IntPtr(10),
				InitialDelaySeconds: sdk.IntPtr(5),
			}
		} else if cReadyProbe == nil && c.Container.MainPort != nil {
			readinessProbe = &corev1.ProbeArgs{
				TcpSocket: corev1.TCPSocketActionArgs{
					Port: sdk.String(toPortName(lo.FromPtr(c.Container.MainPort))),
				},
				PeriodSeconds:       sdk.IntPtr(10),
				InitialDelaySeconds: sdk.IntPtr(5),
			}
		} else if cReadyProbe != nil {
			readinessProbe = toProbeArgs(c, cReadyProbe)
		} else if len(c.Container.Ports) > 1 {
			return corev1.ContainerArgs{}, errors.Errorf("container %q has multiple ports and no readiness probe specified", c.Container.Name)
		}

		// Handle liveness probe
		var livenessProbe *corev1.ProbeArgs
		cLivenessProbe := c.Container.LivenessProbe
		// Use global liveness probe if container doesn't have one AND it's the ingress container
		// This prevents applying HTTP/TCP probes to worker containers that don't expose ports
		if cLivenessProbe == nil && args.LivenessProbe != nil && isIngressContainer {
			cLivenessProbe = args.LivenessProbe
		}

		if cLivenessProbe != nil {
			livenessProbe = toProbeArgs(c, cLivenessProbe)
		}

		var startupProbe *corev1.ProbeArgs
		if c.Container.StartupProbe == nil && (len(c.Container.Ports) == 1 || c.Container.MainPort != nil) {
			startupProbe = readinessProbe
		} else if c.Container.StartupProbe != nil && (len(c.Container.Ports) == 1 || c.Container.MainPort != nil) {
			startupProbe = toProbeArgs(c, c.Container.StartupProbe)
		}

		var resources corev1.ResourceRequirementsArgs
		if c.Container.Resources != nil {
			args.Params.Log.Info(ctx.Context(), "container %q configure resources: %v", c.Container.Name, c.Container.Resources)

			resources.Limits = sdk.ToStringMap(c.Container.Resources.Limits)
			resources.Requests = sdk.ToStringMap(c.Container.Resources.Requests)
		}

		return corev1.ContainerArgs{
			Args:            sdk.ToStringArray(c.Container.Args),
			Command:         sdk.ToStringArray(c.Container.Command),
			Env:             env,
			Image:           c.ImageName,
			ImagePullPolicy: sdk.String(lo.If(c.Container.ImagePullPolicy != nil, lo.FromPtr(c.Container.ImagePullPolicy)).Else("IfNotPresent")),
			Lifecycle:       nil, // TODO
			LivenessProbe:   livenessProbe,
			Name:            sdk.String(c.Container.Name),
			Ports:           ports,
			ReadinessProbe:  readinessProbe,
			StartupProbe:    startupProbe,
			Resources:       resources,
		}, nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert containers to k8s containers")
	}
	if args.Deployment.IngressContainer == nil {
		args.Params.Log.Warn(ctx.Context(), "failed to detect ingress container for %q in %q, service won't be exposed", stackName, stackEnv)
	}

	// Merge secret environment variables from Args with those from Deployment config
	mergedSecretEnvs := lo.Assign(secretEnvs, args.SecretEnvs)

	args.Params.Log.Warn(ctx.Context(), "configure simple container deployment for %q in %q", stackName, stackEnv)
	sc, err := NewSimpleContainer(ctx, &SimpleContainerArgs{
		KubeProvider:           args.KubeProvider,
		ComputeContext:         args.ComputeContext,
		ServiceType:            args.ServiceType,
		UseSSL:                 args.UseSSL,
		ProvisionIngress:       args.ProvisionIngress,
		Namespace:              namespace,
		Service:                deploymentName,
		Deployment:             deploymentName,
		ScEnv:                  stackEnv,
		IngressContainer:       args.Deployment.IngressContainer,
		Domain:                 lo.FromPtr(args.Deployment.StackConfig).Domain,
		Prefix:                 lo.FromPtr(args.Deployment.StackConfig).Prefix,
		ProxyKeepPrefix:        lo.FromPtr(args.Deployment.StackConfig).ProxyKeepPrefix,
		ParentStack:            lo.If(args.Params.ParentStack != nil, lo.ToPtr(lo.FromPtr(args.Params.ParentStack).FullReference)).Else(nil),
		ParentEnv:              lo.If(parentEnv != "", lo.ToPtr(parentEnv)).Else(nil),
		Replicas:               replicas,
		Headers:                args.Deployment.Headers,
		SecretEnvs:             mergedSecretEnvs,
		LbConfig:               args.Deployment.StackConfig.LBConfig,
		Volumes:                args.Deployment.TextVolumes,
		PersistentVolumes:      pvs,
		Containers:             containers,
		ServiceAccountName:     args.ServiceAccountName,
		InitContainers:         args.InitContainers,
		GenerateCaddyfileEntry: args.GenerateCaddyfileEntry,
		Annotations:            args.Annotations,
		NodeSelector:           args.NodeSelector,
		Affinity:               args.Affinity,
		Sidecars:               args.Sidecars,
		VPA:                    args.VPA,              // Pass VPA configuration to SimpleContainer
		Scale:                  args.Deployment.Scale, // Pass Scale configuration to SimpleContainer
		PodDisruption: lo.If(args.Deployment.DisruptionBudget != nil, args.Deployment.DisruptionBudget).Else(&k8s.DisruptionBudget{
			MinAvailable: lo.ToPtr(1),
		}),
		RollingUpdate:       lo.If(args.Deployment.RollingUpdate != nil, toRollingUpdateArgs(args.Deployment.RollingUpdate)).Else(nil),
		SecurityContext:     nil, // TODO
		Log:                 args.Params.Log,
		SecretVolumes:       args.SecretVolumes,
		SecretVolumeOutputs: args.SecretVolumeOutputs,
		ImagePullSecret:     args.ImagePullSecret,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for stack %q in %q", stackName, args.Input.StackParams.Environment)
	}

	// Validate HPA configuration before passing to SimpleContainer
	if args.Deployment.Scale != nil && args.Deployment.Scale.EnableHPA {
		// Get resources from the first container (assuming all containers have similar resource requirements)
		var containerResources *k8s.Resources
		if len(args.Deployment.Containers) > 0 {
			containerResources = args.Deployment.Containers[0].Resources
		}

		// Validate HPA configuration
		if err := ValidateHPAConfiguration(args.Deployment.Scale, containerResources); err != nil {
			return nil, errors.Wrapf(err, "invalid HPA configuration for deployment %s", stackName)
		}
	}

	return sc, nil
}

func toRollingUpdateArgs(update *k8s.RollingUpdate) *v1.RollingUpdateDeploymentArgs {
	return &v1.RollingUpdateDeploymentArgs{
		MaxUnavailable: lo.If(lo.FromPtr(update).MaxUnavailable != nil, sdk.IntPtrFromPtr(lo.FromPtr(update).MaxUnavailable)).Else(nil),
		MaxSurge:       lo.If(lo.FromPtr(update).MaxSurge != nil, sdk.IntPtrFromPtr(lo.FromPtr(update).MaxSurge)).Else(nil),
	}
}

func toProbeArgs(c *ContainerImage, probe *k8s.CloudRunProbe) *corev1.ProbeArgs {
	// Determine the port for the probe:
	// 1. Use probe's HttpGet.Port if specified
	// 2. Fall back to container's MainPort if available
	// 3. Fall back to first container port as last resort
	var probePort int
	if probe.HttpGet.Port > 0 {
		probePort = probe.HttpGet.Port
	} else if c.Container.MainPort != nil && *c.Container.MainPort > 0 {
		probePort = *c.Container.MainPort
	} else if len(c.Container.Ports) > 0 {
		probePort = c.Container.Ports[0]
	}

	probeArgs := &corev1.ProbeArgs{
		PeriodSeconds:       sdk.IntPtrFromPtr(lo.If(probe.Interval != nil, lo.ToPtr(int(lo.FromPtr(probe.Interval).Seconds()))).Else(nil)),
		InitialDelaySeconds: sdk.IntPtrFromPtr(probe.InitialDelaySeconds),
		FailureThreshold:    sdk.IntPtrFromPtr(probe.FailureThreshold),
		SuccessThreshold:    sdk.IntPtrFromPtr(probe.SuccessThreshold),
		TimeoutSeconds:      sdk.IntPtrFromPtr(probe.TimeoutSeconds),
	}

	// Use HttpGet probe if path is specified, otherwise fall back to TcpSocket
	if probe.HttpGet.Path != "" {
		httpGetArgs := &corev1.HTTPGetActionArgs{
			Path: sdk.String(probe.HttpGet.Path),
			Port: sdk.Int(probePort),
		}

		// Add HTTP headers if specified
		if len(probe.HttpGet.HTTPHeaders) > 0 {
			httpHeaders := make(corev1.HTTPHeaderArray, 0, len(probe.HttpGet.HTTPHeaders))
			for _, header := range probe.HttpGet.HTTPHeaders {
				httpHeaders = append(httpHeaders, corev1.HTTPHeaderArgs{
					Name:  sdk.String(header.Name),
					Value: sdk.String(header.Value),
				})
			}
			httpGetArgs.HttpHeaders = httpHeaders
		}

		probeArgs.HttpGet = httpGetArgs
	} else {
		probeArgs.TcpSocket = corev1.TCPSocketActionArgs{
			Port: sdk.String(toPortName(probePort)),
		}
	}

	return probeArgs
}

func toPortName(p int) string {
	return fmt.Sprintf("http-%d", p)
}
