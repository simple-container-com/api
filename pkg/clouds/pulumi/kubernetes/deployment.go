package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type Args struct {
	Namespace              string
	DeploymentName         string
	Annotations            map[string]string
	Input                  api.ResourceInput
	Deployment             k8s.DeploymentConfig
	Images                 []*ContainerImage
	Params                 pApi.ProvisionParams
	ServiceAccountName     *sdk.StringOutput
	KubeProvider           *sdkK8s.Provider
	InitContainers         []corev1.ContainerArgs
	GenerateCaddyfileEntry bool
	ServiceType            *string
	Sidecars               []corev1.ContainerArgs
}

func DeploySimpleContainer(ctx *sdk.Context, args Args, opts ...sdk.ResourceOption) (*SimpleContainer, error) {
	stackName := args.Input.StackParams.StackName
	stackEnv := args.Input.StackParams.Environment
	namespace := lo.If(args.Namespace == "", stackName).Else(args.Namespace)
	deploymentName := lo.If(args.DeploymentName == "", stackName).Else(args.DeploymentName)

	opts = append(opts, sdk.Provider(args.KubeProvider), sdk.DependsOn(args.Params.ComputeContext.Dependencies()))

	replicas := 1
	if args.Deployment.Scale != nil {
		// TODO: support autoscaling
		replicas = args.Deployment.Scale.Replicas
	}

	// secret ENV
	secretEnvs := make(map[string]string)
	for _, v := range args.Params.ComputeContext.SecretEnvVariables() {
		secretEnvs[v.Name] = v.Value
	}
	for k, v := range args.Deployment.StackConfig.Secrets {
		secretEnvs[k] = v
	}

	// env
	contextEnvVars := lo.Filter(args.Params.ComputeContext.EnvVariables(), func(v pApi.ComputeEnvVariable, _ int) bool {
		_, exists := args.Deployment.StackConfig.Env[v.Name]
		return !exists
	})

	// persistent volumes
	pvs := lo.FlatMap(args.Images, func(c *ContainerImage, _ int) []k8s.PersistentVolume {
		return c.Container.Volumes
	})

	containers, err := util.MapErr(args.Images, func(c *ContainerImage, _ int) (corev1.ContainerArgs, error) {
		for _, w := range c.Container.Warnings {
			args.Params.Log.Warn(ctx.Context(), "container %q warning: %s", c.Container.Name, w)
		}
		var env corev1.EnvVarArray
		for _, v := range contextEnvVars {
			env = append(env, corev1.EnvVarArgs{
				Name:  sdk.String(v.Name),
				Value: sdk.String(v.Value),
			})
		}
		for k, v := range args.Deployment.StackConfig.Env {
			env = append(env, corev1.EnvVarArgs{
				Name:  sdk.String(k),
				Value: sdk.String(v),
			})
		}
		for k, v := range c.Container.Env {
			env = append(env, corev1.EnvVarArgs{
				Name:  sdk.String(k),
				Value: sdk.String(v),
			})
		}
		var ports corev1.ContainerPortArray
		var readinessProbe corev1.ProbeArgs
		for _, p := range c.Container.Ports {
			portName := toPortName(p) // TODO: support non-http ports
			ports = append(ports, corev1.ContainerPortArgs{
				Name:          sdk.String(portName),
				ContainerPort: sdk.Int(p),
			})
		}
		if c.Container.ReadinessProbe == nil && len(c.Container.Ports) == 1 {
			readinessProbe = corev1.ProbeArgs{
				TcpSocket: corev1.TCPSocketActionArgs{
					Port: sdk.String(toPortName(c.Container.Ports[0])),
				},
				PeriodSeconds:       sdk.IntPtr(10),
				InitialDelaySeconds: sdk.IntPtr(5),
			}
		} else if c.Container.ReadinessProbe == nil && c.Container.MainPort != nil {
			readinessProbe = corev1.ProbeArgs{
				TcpSocket: corev1.TCPSocketActionArgs{
					Port: sdk.String(toPortName(lo.FromPtr(c.Container.MainPort))),
				},
				PeriodSeconds:       sdk.IntPtr(10),
				InitialDelaySeconds: sdk.IntPtr(5),
			}
		} else if c.Container.ReadinessProbe != nil {
			// TODO: support readiness probe
			return corev1.ContainerArgs{}, errors.Errorf("readiness probe is not supported yet: TODO")
		} else {
			return corev1.ContainerArgs{}, errors.Errorf("container %q has multiple ports and no readiness probe specified", c.Container.Name)
		}

		var startupProbe corev1.ProbeArgs
		if c.Container.StartupProbe == nil && (len(c.Container.Ports) == 1 || c.Container.MainPort != nil) {
			startupProbe = readinessProbe
		}

		var resources corev1.ResourceRequirementsArgs
		return corev1.ContainerArgs{
			Args:            sdk.ToStringArray(c.Container.Args),
			Command:         sdk.ToStringArray(c.Container.Command),
			Env:             env,
			Image:           c.ImageName,
			ImagePullPolicy: sdk.String("IfNotPresent"),
			Lifecycle:       nil, // TODO
			LivenessProbe:   nil, // TODO
			Name:            sdk.String(c.Container.Name),
			Ports:           ports,
			ReadinessProbe:  readinessProbe,
			StartupProbe:    startupProbe,
			Resources:       resources,
		}, nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert GKE containers to k8s containers")
	}
	if args.Deployment.IngressContainer == nil {
		args.Params.Log.Warn(ctx.Context(), "failed to detect ingress container for %q in %q, service won't be exposed", stackName, stackEnv)
	}

	args.Params.Log.Warn(ctx.Context(), "configure simple container deployment for %q in %q", stackName, stackEnv)
	sc, err := NewSimpleContainer(ctx, &SimpleContainerArgs{
		ServiceType:            args.ServiceType,
		Namespace:              namespace,
		Service:                deploymentName,
		Deployment:             deploymentName,
		ScEnv:                  stackEnv,
		IngressContainer:       args.Deployment.IngressContainer,
		Domain:                 args.Deployment.StackConfig.Domain,
		ParentStack:            lo.If(args.Params.ParentStack != nil, lo.ToPtr(lo.FromPtr(args.Params.ParentStack).FullReference)).Else(nil),
		Replicas:               replicas,
		Headers:                args.Deployment.Headers,
		SecretEnvs:             secretEnvs,
		LbConfig:               args.Deployment.StackConfig.LBConfig,
		Volumes:                args.Deployment.TextVolumes,
		PersistentVolumes:      pvs,
		Containers:             containers,
		ServiceAccountName:     args.ServiceAccountName,
		InitContainers:         args.InitContainers,
		GenerateCaddyfileEntry: args.GenerateCaddyfileEntry,
		Annotations:            args.Annotations,
		Sidecars:               args.Sidecars,
		PodDisruption: &k8s.DisruptionBudget{
			MinAvailable: lo.ToPtr(1),
		}, // TODO
		RollingUpdate:   nil, // TODO
		SecurityContext: nil, // TODO
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for stack %q in %q", stackName, args.Input.StackParams.Environment)
	}
	return sc, nil
}

func toPortName(p int) string {
	return fmt.Sprintf("http-%d", p)
}
