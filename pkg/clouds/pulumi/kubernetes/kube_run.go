package kubernetes

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type KubeRunOutput struct {
	Images          []*ContainerImage
	SimpleContainer *SimpleContainer
}

func KubeRun(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.TemplateTypeKubernetesCloudrun {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	kubeRunInput, ok := input.Descriptor.Config.Config.(*k8s.KubeRunInput)
	if !ok {
		return nil, errors.Errorf("failed to convert kubernetes run config for %q", input.Descriptor.Type)
	}

	environment := input.StackParams.Environment
	stackName := input.StackParams.StackName
	parentStack := lo.FromPtr(params.ParentStack).StackName
	fullParentReference := params.ParentStack.FullReference

	out := &KubeRunOutput{}

	registryURL := lo.FromPtr(kubeRunInput.DockerRegistryURL)
	if registryURL == "" {
		return nil, errors.Errorf("parent stack's registry url is empty for stack %q", stackName)
	}

	var caddyConfig *k8s.CaddyConfig
	if kubeRunInput.CaddyResource != nil {
		caddyResource := lo.FromPtr(kubeRunInput.CaddyResource)
		clusterName := ToClusterName(input, caddyResource)
		suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
		params.Log.Info(ctx.Context(), "Getting caddy config for %q from parent stack %q (%s)", caddyResource, fullParentReference, suffix)
		caddyCfgExport := ToCaddyConfigExport(clusterName)
		params.Log.Debug(ctx.Context(), "🔧 Reading Caddy config from parent's output: %v", caddyCfgExport)
		caddyConfigJson, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s%s-stack-caddy-cfg", parentStack, suffix), fullParentReference, caddyCfgExport, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get caddy config from parent stack's %q resources for resource %q", fullParentReference, caddyResource)
		}
		if caddyConfigJson == "" {
			return nil, errors.Errorf("parent stack's caddy config JSON is empty for stack %q", stackName)
		}
		// DEBUG: Log caddy config JSON content and length for debugging
		params.Log.Debug(ctx.Context(), "🔧 Caddy config JSON content: %q", caddyConfigJson)
		params.Log.Debug(ctx.Context(), "🔧 Caddy config JSON length: %d", len(caddyConfigJson))
		params.Log.Debug(ctx.Context(), "🔧 Caddy config JSON as bytes: %v", []byte(caddyConfigJson))

		err = json.Unmarshal([]byte(caddyConfigJson), &caddyConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal caddy config from parent stack: JSON was %q", caddyConfigJson)
		}
	}

	params.Log.Info(ctx.Context(), "Building and pushing images to registry %q for stack %q in %q", registryURL, stackName, environment)
	images, err := BuildAndPushImages(ctx, BuildArgs{
		RegistryURL:      registryURL,
		RegistryUsername: kubeRunInput.DockerRegistryUsername,
		RegistryPassword: kubeRunInput.DockerRegistryPassword,
		Stack:            stack,
		Input:            input,
		Params:           params,
		Deployment:       kubeRunInput.Deployment,
		Opts:             []sdk.ResourceOption{},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker images for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.Images = images

	params.Log.Info(ctx.Context(), "Configure simple container deployment for stack %q in %q", stackName, environment)
	domain := kubeRunInput.Deployment.StackConfig.Domain

	if domain != "" && lo.FromPtr(caddyConfig).UsePrefixes {
		return nil, errors.Errorf("caddy is configured to use prefixes, but domain for service is specified")
	}

	useSSL := kubeRunInput.UseSSL == nil || *kubeRunInput.UseSSL

	var nodeSelector map[string]string
	if kubeRunInput.Deployment.StackConfig.CloudExtras != nil {
		if cExtras, err := api.ConvertDescriptor(kubeRunInput.Deployment.StackConfig.CloudExtras, &k8s.CloudExtras{}); err != nil {
			params.Log.Error(ctx.Context(), "failed to convert cloud extras to k8s.CloudExtras: %v", err)
		} else {
			params.Log.Info(ctx.Context(), "using node selector from cloudExtras: %v", cExtras.NodeSelector)
			nodeSelector = cExtras.NodeSelector
		}
	}

	kubeArgs := Args{
		Input:                  input,
		Deployment:             kubeRunInput.Deployment,
		UseSSL:                 useSSL,
		Images:                 images,
		Params:                 params,
		KubeProvider:           params.Provider,
		ComputeContext:         params.ComputeContext,
		GenerateCaddyfileEntry: domain != "" || lo.FromPtr(caddyConfig).UsePrefixes,
		Annotations: map[string]string{
			"pulumi.com/patchForce": "true",
		},
		NodeSelector: nodeSelector,
		Affinity:     kubeRunInput.Deployment.Affinity,
	}

	if kubeRunInput.RegistryRequiresAuth() {
		kubeArgs.ImagePullSecret = lo.ToPtr(kubeRunInput.RegistryCredentials)
	}

	sc, err := DeploySimpleContainer(ctx, kubeArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.SimpleContainer = sc

	if domain != "" {
		if params.Registrar == nil {
			return nil, errors.Errorf("cannot provision domain %q for stack %q in %q: registrar is not configured", domain, stackName, input.StackParams.Environment)
		}
		var clusterIPAddress string

		if lo.FromPtr(kubeRunInput.Deployment.StackConfig.ClusterIPAddress) != "" {
			params.Log.Info(ctx.Context(), "Using specified cluster IP address for domain %q (%q)", domain, kubeRunInput.Deployment.StackConfig.ClusterIPAddress)
			clusterIPAddress = lo.FromPtr(kubeRunInput.Deployment.StackConfig.ClusterIPAddress)
		} else {
			params.Log.Info(ctx.Context(), "Using provisioned cluster IP address for domain %q", domain)
			suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
			clusterIPAddress, err = pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-%s%s-ip", stackName, input.StackParams.Environment, suffix), fullParentReference, ToIngressIpExport(parentStack), false)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get cluster IP address from parent stack's resources")
			}
		}
		_, err = params.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    domain,
			Type:    "A",
			Value:   clusterIPAddress,
			Proxied: true,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision domain %q for stack %q in %q", domain, stackName, environment)
		}
	}

	if caddyConfig != nil {
		_, err = PatchDeployment(ctx, &DeploymentPatchArgs{
			PatchName:   fmt.Sprintf("%s-%s", stackName, environment),
			ServiceName: "caddy",
			Namespace:   lo.If(caddyConfig.Namespace != nil, lo.FromPtr(caddyConfig.Namespace)).Else("caddy"),
			Annotations: map[string]sdk.StringOutput{
				"simple-container.com/caddy-updated-by": sdk.String(stackName).ToStringOutput(),
				"simple-container.com/caddy-updated-at": sdk.String("latest").ToStringOutput(),
				"simple-container.com/caddy-update-hash": sdk.All(sc.CaddyfileEntry).ApplyT(func(entry []any) string {
					sum := md5.Sum([]byte(entry[0].(string)))
					return hex.EncodeToString(sum[:])
				}).(sdk.StringOutput),
			},
			Opts: []sdk.ResourceOption{sdk.Provider(params.Provider), sdk.DependsOn([]sdk.Resource{sc.Service})},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to patch caddy configuration")
		}
	}

	return &api.ResourceOutput{Ref: out}, nil
}

func ToCaddyConfigExport(clusterName string) string {
	return fmt.Sprintf("%s-caddy-config", clusterName)
}
