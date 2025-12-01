package gcp

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	auth "golang.org/x/oauth2/google"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

type GkeAutopilotOutput struct {
	Provider        *sdkK8s.Provider
	Images          []*kubernetes.ContainerImage
	SimpleContainer *kubernetes.SimpleContainer
}

func GkeAutopilotStack(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeGkeAutopilot {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}

	gkeAutopilotInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotInput)
	if !ok {
		return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
	}

	clusterResource := gkeAutopilotInput.GkeAutopilotTemplate.GkeClusterResource
	registryResource := gkeAutopilotInput.GkeAutopilotTemplate.ArtifactRegistryResource

	// Fix for custom stacks: ensure input.StackParams.ParentEnv is set correctly
	environment := input.StackParams.Environment
	if params.ParentStack != nil && params.ParentStack.ParentEnv != "" && params.ParentStack.ParentEnv != environment {
		// This is a custom stack - set ParentEnv so ToResName uses parent environment for resource naming
		input.StackParams.ParentEnv = params.ParentStack.ParentEnv
		params.Log.Info(ctx.Context(), "🔧 Custom stack detected: set input.StackParams.ParentEnv to %q for resource naming", params.ParentStack.ParentEnv)
	}

	clusterName := kubernetes.ToClusterName(input, clusterResource)
	registryName := toArtifactRegistryName(input, registryResource)
	stackName := input.StackParams.StackName
	fullParentReference := params.ParentStack.FullReference

	// Debug logging for custom stacks
	params.Log.Info(ctx.Context(), "🔍 DEBUG: GKE Autopilot stack deployment")
	params.Log.Info(ctx.Context(), "🔍 DEBUG: environment=%q, stackName=%q", environment, stackName)
	params.Log.Info(ctx.Context(), "🔍 DEBUG: input.StackParams.ParentEnv=%q", input.StackParams.ParentEnv)
	params.Log.Info(ctx.Context(), "🔍 DEBUG: params.ParentStack.ParentEnv=%q", params.ParentStack.ParentEnv)
	params.Log.Info(ctx.Context(), "🔍 DEBUG: clusterResource=%q, clusterName=%q", clusterResource, clusterName)
	params.Log.Info(ctx.Context(), "🔍 DEBUG: registryResource=%q, registryName=%q", registryResource, registryName)

	if clusterResource == "" {
		return nil, errors.Errorf("`clusterResource` must be specified for gke autopilot config for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	if registryResource == "" {
		return nil, errors.Errorf("`artifactRegistryResource` must be specified for gke autopilot config for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "Getting kubeconfig for %q from parent stack %q (%q)", clusterName, fullParentReference, suffix)
	kubeConfig, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s%s-stack-kubeconfig", clusterName, suffix), fullParentReference, toKubeconfigExport(clusterName), true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig from parent stack's resources")
	}
	out := &GkeAutopilotOutput{}

	kubeProvider, err := sdkK8s.NewProvider(ctx, input.ToResName(stackName), &sdkK8s.ProviderArgs{
		Kubeconfig: sdk.String(kubeConfig),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q", stackName, input.Descriptor.Name, environment)
	}

	out.Provider = kubeProvider

	params.Log.Info(ctx.Context(), "Getting registry url for %q from parent stack %q (%q)", registryResource, fullParentReference, suffix)
	registryURL, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s%s-stack-registryurl", clusterName, suffix), fullParentReference, toRegistryUrlExport(registryName), false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get registry url from parent stack's %q resources for resource %q", fullParentReference, registryResource)
	}
	if registryURL == "" {
		return nil, errors.Errorf("parent stack's registry url is empty for stack %q", stackName)
	}

	params.Log.Info(ctx.Context(), "Authenticating against registry %q for stack %q", registryURL, stackName)
	gcpCreds, err := getDockerCredentialsWithAuthToken(ctx, input)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to obtain access token for registry %q for stack %q", registryURL, stackName)
	}

	params.Log.Info(ctx.Context(), "Building and pushing images to registry %q for stack %q in %q", registryURL, stackName, environment)
	images, err := kubernetes.BuildAndPushImages(ctx, kubernetes.BuildArgs{
		RegistryURL:      registryURL,
		RegistryUsername: lo.ToPtr(gcpCreds.Username),
		RegistryPassword: lo.ToPtr(gcpCreds.Password),
		Stack:            stack,
		Input:            input,
		Params:           params,
		Deployment:       gkeAutopilotInput.Deployment,
		Opts:             []sdk.ResourceOption{},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker images for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.Images = images

	params.Log.Info(ctx.Context(), "Configure simple container deployment for stack %q in %q", stackName, environment)
	domain := gkeAutopilotInput.Deployment.StackConfig.Domain

	// Debug logging for affinity rules
	params.Log.Info(ctx.Context(), "🔍 DEBUG: gkeAutopilotInput.Deployment.Affinity: %+v", gkeAutopilotInput.Deployment.Affinity)
	if gkeAutopilotInput.Deployment.Affinity != nil {
		params.Log.Info(ctx.Context(), "🔍 DEBUG: GKE Affinity details - NodePool: %v, ComputeClass: %v, ExclusiveNodePool: %v",
			gkeAutopilotInput.Deployment.Affinity.NodePool,
			gkeAutopilotInput.Deployment.Affinity.ComputeClass,
			gkeAutopilotInput.Deployment.Affinity.ExclusiveNodePool)
	} else {
		params.Log.Info(ctx.Context(), "🔍 DEBUG: gkeAutopilotInput.Deployment.Affinity is nil")
	}

	kubeArgs := kubernetes.Args{
		Input:                  input,
		Deployment:             gkeAutopilotInput.Deployment,
		Images:                 images,
		Params:                 params,
		KubeProvider:           kubeProvider,
		ComputeContext:         params.ComputeContext,
		GenerateCaddyfileEntry: domain != "",
		Annotations: map[string]string{
			"pulumi.com/patchForce": "true",
		},
		NodeSelector:   gkeAutopilotInput.Deployment.NodeSelector,
		Affinity:       gkeAutopilotInput.Deployment.Affinity,
		Tolerations:    gkeAutopilotInput.Deployment.Tolerations,
		VPA:            gkeAutopilotInput.Deployment.VPA,            // Pass VPA configuration to Kubernetes deployment
		ReadinessProbe: gkeAutopilotInput.Deployment.ReadinessProbe, // Pass global readiness probe configuration
		LivenessProbe:  gkeAutopilotInput.Deployment.LivenessProbe,  // Pass global liveness probe configuration
	}

	params.Log.Info(ctx.Context(), "🔍 DEBUG: kubeArgs.Affinity passed to DeploySimpleContainer: %+v", kubeArgs.Affinity)

	sc, err := kubernetes.DeploySimpleContainer(ctx, kubeArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for stack %q in %q", stackName, input.StackParams.Environment)
	}
	out.SimpleContainer = sc

	if domain != "" {
		if params.Registrar == nil {
			return nil, errors.Errorf("cannot provision domain %q for stack %q in %q: registrar is not configured", domain, stackName, input.StackParams.Environment)
		}
		clusterIPAddress, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-%s%s-ip", stackName, input.StackParams.Environment, suffix), fullParentReference, kubernetes.ToIngressIpExport(clusterName), false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get cluster IP address from parent stack's resources")
		}

		// Determine if domain should be proxied - defaults to true if not explicitly set to false
		domainProxied := true
		if gkeAutopilotInput.Deployment.StackConfig.DomainProxied != nil {
			domainProxied = *gkeAutopilotInput.Deployment.StackConfig.DomainProxied
		}

		_, err = params.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    domain,
			Type:    "A",
			Value:   clusterIPAddress,
			Proxied: domainProxied,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision domain %q for stack %q in %q", domain, stackName, environment)
		}

		caddyCfgExport := kubernetes.ToCaddyConfigExport(clusterName)
		params.Log.Info(ctx.Context(), "Getting caddy config from parent stack %q (%s)", fullParentReference, caddyCfgExport)
		params.Log.Debug(ctx.Context(), "🔧 DEBUG: clusterName=%q, suffix=%q", clusterName, suffix)
		caddyConfigJson, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-%s%s-caddy-cfg", stackName, input.StackParams.Environment, suffix), fullParentReference, caddyCfgExport, false)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get caddy config from parent stack's resources")
		}
		// DEBUG: Log caddy config JSON content and length for debugging
		params.Log.Debug(ctx.Context(), "🔧 DEBUG [GKE]: Caddy config JSON content: %q", caddyConfigJson)
		params.Log.Debug(ctx.Context(), "🔧 DEBUG [GKE]: Caddy config JSON length: %d", len(caddyConfigJson))
		params.Log.Debug(ctx.Context(), "🔧 DEBUG [GKE]: Caddy config JSON as bytes: %v", []byte(caddyConfigJson))

		var caddyCfg k8s.CaddyConfig
		err = json.Unmarshal([]byte(caddyConfigJson), &caddyCfg)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal caddy config from parent stack: JSON was %q", caddyConfigJson)
		}

		// Attempt to patch caddy deployment annotations (non-critical - skip if it fails)
		// Use deployment name override if specified, otherwise fall back to default
		deploymentName := lo.If(caddyCfg.DeploymentName != nil, lo.FromPtr(caddyCfg.DeploymentName)).Else(input.ToResName("caddy"))
		namespace := lo.If(caddyCfg.Namespace != nil, lo.FromPtr(caddyCfg.Namespace)).Else("caddy")

		_, patchErr := kubernetes.PatchDeployment(ctx, &kubernetes.DeploymentPatchArgs{
			PatchName:   input.ToResName(stackName),
			ServiceName: deploymentName,
			Namespace:   namespace,
			Annotations: map[string]sdk.StringOutput{
				"simple-container.com/caddy-updated-by": sdk.String(stackName).ToStringOutput(),
				"simple-container.com/caddy-updated-at": sdk.String("latest").ToStringOutput(),
				"simple-container.com/caddy-update-hash": sdk.All(sc.CaddyfileEntry).ApplyT(func(entry []any) string {
					sum := md5.Sum([]byte(entry[0].(string)))
					return hex.EncodeToString(sum[:])
				}).(sdk.StringOutput),
			},
			Opts: []sdk.ResourceOption{sdk.Provider(kubeProvider), sdk.DependsOn([]sdk.Resource{sc.Service})},
		})
		if patchErr != nil {
			// Log warning but continue - caddy annotation patch is not critical for deployment
			params.Log.Warn(ctx.Context(), "⚠️  Failed to patch caddy deployment annotations (non-critical): %v", patchErr)
		}
	}

	return &api.ResourceOutput{Ref: out}, nil
}

// authAgainstRegistry - run gcloud auth configure-docker to configure docker/config.json to access repo
// nolint: unused
func authAgainstRegistry(ctx *sdk.Context, authName string, input api.ResourceInput, params pApi.ProvisionParams, registryURL sdk.StringOutput) ([]sdk.ResourceOption, error) {
	authConfig, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert resource input to api.AuthConfig for %q", input.Descriptor.Type)
	}

	var opts []sdk.ResourceOption
	if _, err := exec.LookPath("gcloud"); err == nil {
		env := lo.SliceToMap(os.Environ(), func(env string) (string, string) {
			parts := strings.SplitN(env, "=", 2)
			return parts[0], parts[1]
		})
		env["GOOGLE_CREDENTIALS"] = authConfig.CredentialsValue()
		env["GOOGLE_APPLICATION_CREDENTIALS"] = authConfig.CredentialsValue()
		registryHost := registryURL.ApplyT(func(out any) (string, error) {
			rUrl := out.(string)
			var parsedRegistryURL *url.URL
			if strings.HasPrefix(rUrl, "http") {
				parsedRegistryURL, err = url.Parse(rUrl)
				if err != nil {
					return "", errors.Wrapf(err, "failed to parse registry url %q as is", rUrl)
				}
			} else if parsedRegistryURL, err = url.Parse(fmt.Sprintf("https://%s", rUrl)); err != nil {
				return "", errors.Wrapf(err, "failed to parse registry url %q", rUrl)
			}
			params.Log.Info(ctx.Context(), "extracted registry host for gcloud auth configure-docker: %q", parsedRegistryURL.Host)
			return parsedRegistryURL.Host, nil
		})

		params.Log.Info(ctx.Context(), "configure gcloud auth configure-docker against registry host...")
		configureDockerCmd, err := local.NewCommand(ctx, fmt.Sprintf("%s-%s-%s", input.StackParams.StackName, input.StackParams.Environment, authName), &local.CommandArgs{
			Update:      sdk.Sprintf("gcloud auth configure-docker %s --quiet", registryHost),
			Create:      sdk.Sprintf("gcloud auth configure-docker %s --quiet", registryHost),
			Triggers:    sdk.ArrayInput(sdk.Array{sdk.String(lo.RandomString(5, lo.AllCharset))}),
			Environment: sdk.ToStringMap(env),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to authenticate against docker registry")
		}
		opts = append(opts, sdk.DependsOn([]sdk.Resource{configureDockerCmd}))
	} else {
		return nil, errors.Errorf("command `gcloud` was not found, cannot authenticate against artifact registry")
	}
	return opts, nil
}

type AccessTokenCreds struct {
	Username   string
	Password   string
	AuthHeader string
}

func getDockerCredentialsWithAuthToken(ctx *sdk.Context, input api.ResourceInput) (*AccessTokenCreds, error) {
	authCfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast resource descriptor to api.AuthConfig")
	}
	credentials, err := auth.CredentialsFromJSONWithParams(ctx.Context(), []byte(authCfg.CredentialsValue()), auth.CredentialsParams{
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find default credentials for GCP")
	}
	token, err := credentials.TokenSource.Token()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get GCP token from credentials")
	}

	username := "oauth2accesstoken"
	password := token.AccessToken
	authHeader, err := docker.EncodeDockerAuthHeader(username, password)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate docker auth header")
	}

	return &AccessTokenCreds{
		Username:   username,
		Password:   password,
		AuthHeader: authHeader,
	}, nil
}
