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
		params.Log.Info(ctx.Context(), "Getting caddy config for %q from parent stack %q", caddyResource, fullParentReference)
		caddyConfigJson, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-stack-caddy-cfg", parentStack), fullParentReference, ToCaddyConfigExport(clusterName), false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get caddy config from parent stack's %q resources for resource %q", fullParentReference, caddyResource)
		}
		if caddyConfigJson == "" {
			return nil, errors.Errorf("parent stack's registry url is empty for stack %q", stackName)
		}
		err = json.Unmarshal([]byte(caddyConfigJson), &caddyConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal caddy config from parent stack")
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

	if domain != "" && caddyConfig.UsePrefixes {
		return nil, errors.Errorf("caddy is configured to use prefixes, but domain for service is specified")
	}

	useSSL := kubeRunInput.UseSSL == nil || *kubeRunInput.UseSSL

	kubeArgs := Args{
		Input:                  input,
		Deployment:             kubeRunInput.Deployment,
		UseSSL:                 useSSL,
		Images:                 images,
		Params:                 params,
		KubeProvider:           params.Provider,
		ComputeContext:         params.ComputeContext,
		GenerateCaddyfileEntry: domain != "" || caddyConfig.UsePrefixes,
		Annotations: map[string]string{
			"pulumi.com/patchForce": "true",
		},
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
		clusterIPAddress, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-%s-ip", stackName, input.StackParams.Environment), fullParentReference, ToIngressIpExport(parentStack), false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get cluster IP address from parent stack's resources")
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
				"simple-container.com/caddy-update-hash": sc.CaddyfileEntry.ApplyT(func(entry any) string {
					sum := md5.Sum([]byte(entry.(string)))
					return hex.EncodeToString(sum[:])
				}).(sdk.StringOutput),
			},
			Opts: []sdk.ResourceOption{sdk.Provider(params.Provider), sdk.DependsOn([]sdk.Resource{sc})},
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
