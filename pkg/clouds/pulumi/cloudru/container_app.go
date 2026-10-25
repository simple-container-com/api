// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
)

// ContainerAppOutput is what the stack exports for a provisioned Container App.
type ContainerAppOutput struct {
	Registry *local.Command
	Image    *docker.Image
	App      *local.Command
}

// ContainerApp provisions the Cloud.ru Evolution analogue of an AWS lambda or a
// Yandex Serverless Container: one `type: single-image` client stack becomes one
// Container App on a public HTTPS URL, scaling to zero.
//
// Three resources, in order, because each needs the previous one to exist:
//
//  1. a local.Command running `sc cloudru registry ensure` — a Container App
//     resolves its image from the calling project's own Artifact Registry and from
//     NOWHERE else (Docker Hub, `docker.io/…` and Cloud.ru's own quickstart image
//     all answer `Failed to resolve image`), so the registry is part of the deploy;
//  2. the shared docker build+push — `docker login <registry>.cr.cloud.ru` takes
//     the same IAM key pair as the REST API, so this is an ordinary
//     pDocker.BuildAndPushImage with no Cloud.ru-specific code at all;
//  3. a local.Command triplet running `sc cloudru container-app apply|destroy`.
//
// Why the app is a command rather than in-process REST, which would be shorter: an
// in-process call runs during `pulumi preview`, where nothing is supposed to
// change, and it sits outside Pulumi's resource model, so nothing downstream could
// depend on it and a repeat deploy would have no way to know it already happened.
// Same reasoning, verbatim, as the vm driver's shipImage. See the package doc of
// pkg/cmd/cmd_cloudru.
func ContainerApp(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != cloudru.TemplateTypeCloudruContainerApp {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.StackParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.StackParams

	caInput, ok := input.Descriptor.Config.Config.(*cloudru.ContainerAppInput)
	if !ok {
		return nil, errors.Errorf("failed to convert %q config for %q in stack %q in %q",
			cloudru.TemplateTypeCloudruContainerApp, input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	if err := api.ConvertAuth(caInput, &caInput.EvolutionAccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to cloudru.EvolutionAccountConfig")
	}
	if err := caInput.EvolutionAccountConfig.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid cloud.ru evolution credentials for stack %q", stack.Name)
	}
	if params.Provider == nil {
		return nil, errors.Errorf("provider must not be nil for container app in stack %q", stack.Name)
	}

	stackConfig := caInput.StackConfig
	if stackConfig.Image == nil {
		return nil, errors.Errorf("image must be configured for container app in stack %q", stack.Name)
	}
	extras, err := caInput.CloudExtras()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read cloudExtras for container app in stack %q", stack.Name)
	}

	appName := cloudru.ToAppName(input.ToResName(stack.Name))
	if err := cloudru.ValidateAppName(appName); err != nil {
		return nil, errors.Wrapf(err, "invalid container app name for stack %q", stack.Name)
	}
	containerName := lo.If(extras.ContainerName != "", extras.ContainerName).Else(appName)
	if err := cloudru.ValidateAppName(containerName); err != nil {
		return nil, errors.Wrapf(err, "invalid container name for stack %q", stack.Name)
	}

	// Derived from the stack name WITHOUT the environment, so every environment of
	// a stack shares one registry — a registry per environment would multiply a
	// project-scoped resource by the number of environments for no benefit, since
	// the image tag already carries the version and the repository carries the env.
	// `registryName:` on the template overrides this, which is how several stacks
	// share one.
	registryName := caInput.RegistryName
	if registryName == "" {
		registryName = cloudru.ToAppName(stack.Name)
	}
	// Cloud.ru does not document the Artifact Registry name grammar and the API
	// answers a violation with a bare 400. The Container App rule is used as a
	// conservative stand-in: it is stricter than anything a registry is likely to
	// enforce, and the one live registry (`sc-smoke`) satisfies it.
	if err := cloudru.ValidateAppName(registryName); err != nil {
		return nil, errors.Wrapf(err, "invalid cloud.ru artifact registry name for stack %q (set `registryName` on the template to override)", stack.Name)
	}

	creds := evolution.Credentials{
		KeyID:     caInput.KeyID,
		Secret:    caInput.KeySecret,
		ProjectID: caInput.ProjectID,
	}

	registryCmd, err := ensureRegistry(ctx, stack, input, params, creds, registryName)
	if err != nil {
		return nil, err
	}

	image, err := buildAndPushImage(ctx, stack, params, deployParams, caInput, creds, registryName, appName, registryCmd)
	if err != nil {
		return nil, err
	}

	// buildSpec is called twice — once for the request the command reads, once for
	// the hash that triggers it. The spec itself cannot be the trigger: trigger
	// values are stored and rendered unencrypted in a Pulumi diff, and this spec's
	// env block holds the stack's secrets inline (see envVars below).
	buildSpec := func(imageRef string) (evolution.AppSpec, error) {
		return containerAppSpec(ctx, stack, deployParams, params, caInput, extras, appName, containerName, imageRef)
	}

	requestJSON := image.DeployImageRef.ApplyT(func(imageRef string) (string, error) {
		spec, err := buildSpec(imageRef)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(evolution.Request{
			Credentials: creds,
			Name:        appName,
			Spec:        &spec,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to encode cloud.ru container app request for stack %q", stack.Name)
		}
		return string(encoded), nil
	}).(sdk.StringOutput)

	specHash := image.DeployImageRef.ApplyT(func(imageRef string) (string, error) {
		spec, err := buildSpec(imageRef)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash cloud.ru container app spec for stack %q", stack.Name)
		}
		sum := sha256.Sum256(encoded)
		return hex.EncodeToString(sum[:]), nil
	}).(sdk.StringOutput)

	applyScript := scScript("cloudru", "container-app", "apply")
	destroyScript := scScript("cloudru", "container-app", "destroy")

	appOpts := []sdk.ResourceOption{sdk.Provider(params.Provider)}
	appOpts = append(appOpts, image.AddOpts...)
	appOpts = append(appOpts, sdk.DependsOn(params.ComputeContext.Dependencies()))

	params.Log.Info(ctx.Context(), "configure cloud.ru container app %q (container %q) for stack %q in %q...",
		appName, containerName, stack.Name, deployParams.Environment)
	appCmd, err := local.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-container-app", stack.Name)), &local.CommandArgs{
		Create: sdk.String(applyScript),
		// Update is the same script, not absent: without it pulumi-command treats
		// any input change as a replacement and runs Delete before Create, which
		// would take the public URL down on every redeploy. `apply` is a
		// create-or-patch, and a patch publishes a new revision while the old one
		// keeps serving — blue-green at no cost.
		Update: sdk.String(applyScript),
		Delete: sdk.StringPtr(destroyScript),
		// The one trigger that earns its place, and the reason there are no others:
		// the script text is a constant, so without a trigger over the *content* a
		// redeploy at the same version would not re-run at all and would report green
		// while Cloud.ru kept serving the previous revision. That exact failure was
		// live-caught on the vm driver on 2026-10-18 (see shipImage), and it is
		// expensive because the deploy is green — the next place anybody looks is
		// their own Dockerfile.
		//
		// The hash covers the image digest as well as every other field of the spec,
		// so a code change, an env change and a scaling change each re-run it.
		Triggers: sdk.Array{specHash},
		// Credentials, the stack's secrets and the spec, all on stdin. Marked secret
		// so the state file holds it encrypted; LoggingNone so neither the request
		// nor the command's own output reaches the Actions log; and no
		// AddPreviousOutputInEnv, because the previous run's stdout is a JSON
		// document this script has no use for.
		Stdin:                  sdk.ToSecret(requestJSON).(sdk.StringOutput).ToStringPtrOutput(),
		Logging:                local.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, appOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to configure cloud.ru container app %q for stack %q", appName, stack.Name)
	}

	// Stdin is a plain resource input, so it is persisted in state and the Delete
	// above reads the same document — which is why one request shape serves both
	// subcommands and `destroy` needs no separate plumbing.
	ctx.Export(fmt.Sprintf("%s-%s-container-app-url", stack.Name, deployParams.Environment), resultField(appCmd.Stdout, func(r evolution.Result) string { return r.URL }))
	ctx.Export(fmt.Sprintf("%s-%s-container-app-id", stack.Name, deployParams.Environment), resultField(appCmd.Stdout, func(r evolution.Result) string { return r.ID }))
	ctx.Export(fmt.Sprintf("%s-%s-container-app-revision", stack.Name, deployParams.Environment), resultField(appCmd.Stdout, func(r evolution.Result) string { return r.Revision }))

	if stackConfig.Domain != "" {
		// Deliberately a warning on the deploy rather than a silent omission or a
		// refusal. The custom domain IS sent to the API (configuration.ingress.
		// customDomains below), but SC does not create the DNS record for it on this
		// cloud yet — the registrar seam that does that for a lambda and for a YC
		// container has not been exercised against a Container App. Untested as of
		// 2026-10-22.
		params.Log.Warn(ctx.Context(), "cloud.ru container app %q binds custom domain %q, but simple-container does not create its DNS record on this cloud yet: "+
			"point the record at the app's public host yourself (see the %s-%s-container-app-url stack output)",
			appName, stackConfig.Domain, stack.Name, deployParams.Environment)
	}

	return &api.ResourceOutput{
		Ref: &ContainerAppOutput{
			Registry: registryCmd,
			Image:    image.Image,
			App:      appCmd,
		},
	}, nil
}

// ensureRegistry makes the project's Artifact Registry exist before the push.
//
// No Delete, on purpose. A registry is shared by every environment of a stack (and
// by every stack, when the template names one), it holds images other revisions may
// still be rolling back to, and Cloud.ru's delete is not scoped to the images this
// stack pushed. Destroying one stack must not strip a sibling's images, so the
// registry outlives `sc destroy` and is the operator's to remove.
func ensureRegistry(
	ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams,
	creds evolution.Credentials, registryName string,
) (*local.Command, error) {
	encoded, err := json.Marshal(evolution.Request{Credentials: creds, RegistryName: registryName})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encode cloud.ru registry request for stack %q", stack.Name)
	}
	script := scScript("cloudru", "registry", "ensure")

	params.Log.Info(ctx.Context(), "configure cloud.ru artifact registry %q (%s) for stack %q...",
		registryName, evolution.RegistryHost(registryName), stack.Name)
	cmd, err := local.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-registry", stack.Name)), &local.CommandArgs{
		Create: sdk.String(script),
		Update: sdk.String(script),
		// `ensure` is idempotent, so re-running it costs one list call; what the
		// trigger buys is re-running it at all when the name or the project changes.
		// Neither value is a credential.
		Triggers: sdk.Array{
			sdk.String(registryName),
			sdk.String(creds.ProjectID),
		},
		Stdin:                  sdk.ToSecret(sdk.String(string(encoded))).(sdk.StringOutput).ToStringPtrOutput(),
		Logging:                local.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to configure cloud.ru artifact registry %q for stack %q", registryName, stack.Name)
	}
	return cmd, nil
}

// buildAndPushImage pushes the stack's image into the project's Artifact Registry.
//
// Nothing here is Cloud.ru-specific beyond the host and the credential: `docker
// login <registry>.cr.cloud.ru` takes the same IAM Key ID and secret as the REST
// API, so there is deliberately no separate registry credential to configure and no
// REST call in the push path.
func buildAndPushImage(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams,
	caInput *cloudru.ContainerAppInput, creds evolution.Credentials, registryName, appName string, registryCmd *local.Command,
) (*pDocker.ImageOut, error) {
	stackConfig := caInput.StackConfig
	host := evolution.RegistryHost(registryName)

	// The repository is a path segment under the registry host, as on Yandex and
	// unlike ECR, so RepositoryUrlWithImage is false and the name is appended.
	//
	// The name is the app name, which cloudru.ToAppName has already collapsed to
	// single hyphens. That collapse is load-bearing here, and the question the
	// previous version of this comment left open is now ANSWERED: Cloud.ru's
	// registry behaves exactly like Yandex's, rejecting a separator run on push
	// with `invalid repository name` (live smoke 2026-10-23 — the first SC deploy
	// ever attempted on this driver, which got this far and no further). The
	// 2026-10-22 skeleton missed it only because it pushed `react-hello-world`,
	// which has no run.
	repositoryName := appName

	out, err := pDocker.BuildAndPushImage(ctx, stack, params, deployParams, pDocker.Image{
		Name:                   repositoryName,
		Dockerfile:             stackConfig.Image.Dockerfile,
		Context:                stackConfig.Image.Context,
		Args:                   lo.FromPtr(stackConfig.Image.Build).Args,
		Version:                lo.If(deployParams.Version != "", deployParams.Version).Else("latest"),
		RepositoryUrlWithImage: false,
		RepositoryUrl:          sdk.String(host).ToStringOutput(),
		Platform:               lo.If(stackConfig.Image.Platform != "", lo.ToPtr(string(stackConfig.Image.Platform))).Else(nil),
		// Only the registry dependency. The rest of this driver's options carry
		// sdk.Provider(<the command provider>), which a docker.Image has no use for.
		ProviderOptions: []sdk.ResourceOption{sdk.DependsOn([]sdk.Resource{registryCmd})},
		Registry: docker.RegistryArgs{
			Server:   sdk.String(host),
			Username: sdk.String(creds.KeyID),
			Password: sdk.ToSecret(sdk.String(creds.Secret)).(sdk.StringOutput).ToStringPtrOutput(),
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q (from %q) to %q for stack %q env %q",
			repositoryName, stackConfig.Image.Context, host, stack.Name, deployParams.Environment)
	}
	return out, nil
}

// containerAppSpec renders the desired state of the app.
func containerAppSpec(
	ctx *sdk.Context,
	stack api.Stack, deployParams api.StackParams, params pApi.ProvisionParams,
	caInput *cloudru.ContainerAppInput, extras *cloudru.ContainerAppExtras,
	appName, containerName, imageRef string,
) (evolution.AppSpec, error) {
	stackConfig := caInput.StackConfig

	memoryMB := lo.If(stackConfig.MaxMemory == nil, cloudru.DefaultMemoryMB).Else(lo.FromPtr(stackConfig.MaxMemory))
	if memoryMB <= 0 {
		return evolution.AppSpec{}, errors.Errorf("maxMemory must be positive for container app in stack %q, got %d", stack.Name, memoryMB)
	}
	timeoutSeconds := lo.If(stackConfig.Timeout != nil, lo.FromPtr(stackConfig.Timeout)).Else(cloudru.DefaultTimeoutSeconds)
	maxInstances := lo.If(stackConfig.Max > 0, stackConfig.Max).Else(cloudru.DefaultMaxInstances)
	if stackConfig.Min > maxInstances {
		return evolution.AppSpec{}, errors.Errorf("min (%d) cannot exceed max (%d) for container app in stack %q",
			stackConfig.Min, maxInstances, stack.Name)
	}

	port := extras.EffectiveContainerPort()

	ingress := &evolution.AppIngress{PubliclyAccessible: lo.ToPtr(true)}
	if stackConfig.Domain != "" {
		if extras.CertificateID == "" {
			// There is no ACME on this path. The platform's own wildcard covers only
			// `*.containerapps.ru`, so a custom domain cannot get a certificate from
			// anywhere except Cloud.ru's certificate manager — which means the id is
			// not an optional extra, it is the only way the domain can serve TLS.
			return evolution.AppSpec{}, errors.Errorf("`domain: %s` on stack %q also needs `cloudExtras.certificateId`: "+
				"Cloud.ru's platform certificate covers only *.%s and there is no ACME on this path, so a custom domain "+
				"serves TLS from a certificate uploaded to Cloud.ru's certificate manager",
				stackConfig.Domain, stack.Name, evolution.PublicDomainSuffix)
		}
		ingress.CustomDomains = []evolution.CustomDomainBinding{{
			Domain:        stackConfig.Domain,
			CertificateID: extras.CertificateID,
		}}
	}

	scaling := &evolution.AppScaling{
		MinInstanceCount: stackConfig.Min,
		MaxInstanceCount: maxInstances,
	}
	if extras.Concurrency != nil {
		scaling.Rule = &evolution.AppScalingRule{
			Type:  evolution.ScalingRuleConcurrency,
			Value: map[string]int{"soft": lo.FromPtr(extras.Concurrency)},
		}
	}

	return evolution.AppSpec{
		ProjectID:   caInput.ProjectID,
		Name:        appName,
		Description: fmt.Sprintf("%s (%s), deployed by simple-container", stack.Name, deployParams.Environment),
		Configuration: &evolution.AppConfig{
			Ingress:    ingress,
			Privileged: lo.If(extras.EffectivePrivileged(), lo.ToPtr(true)).Else(nil),
		},
		Template: &evolution.AppTemplate{
			Timeout:     fmt.Sprintf("%ds", timeoutSeconds),
			IdleTimeout: extras.EffectiveIdleTimeout(),
			Protocol:    evolution.ProtocolHTTP1,
			Scaling:     scaling,
			Containers: []evolution.AppContainer{{
				Name:  containerName,
				Image: imageRef,
				Resources: &evolution.AppResources{
					CPU:    extras.EffectiveCPU(),
					Memory: fmt.Sprintf("%dMi", memoryMB),
				},
				ContainerPort: port,
				Env:           envVars(ctx, stackConfig, params, deployParams, stack, port),
			}},
		},
	}, nil
}

// envVars renders the container's environment.
//
// Secrets are INLINED into the revision spec, unlike every other cloud SC deploys
// to. Cloud.ru's `AppEnv.Type: "secret"` makes the value a reference into Cloud.ru
// Secret Management, which SC does not provision, so there is nothing to reference
// yet; until it does, a stack's secrets are visible in the app's revision view to
// anyone with read access to the project. That is a real difference from the Lockbox
// reference a YC container gets and from an AWS SecretsManager ARN, and it is the
// reason the whole request travels on Stdin with LoggingNone.
//
// Sorted by name, because map iteration order would otherwise change the spec hash
// on every run and publish a fresh revision for no reason.
func envVars(
	ctx *sdk.Context,
	stackConfig api.StackConfigSingleImage, params pApi.ProvisionParams,
	deployParams api.StackParams, stack api.Stack, port int,
) []evolution.AppEnv {
	values := map[string]string{
		api.ComputeEnv.StackName:    stack.Name,
		api.ComputeEnv.StackEnv:     deployParams.Environment,
		api.ComputeEnv.StackVersion: deployParams.Version,
		// PORT is NOT set here, and must not be: Cloud.ru reserves it exactly like YC
		// does, and refuses the whole revision with
		//   fieldViolations[0]: field "name", "The container env variable name 'PORT'
		//   is reserved. Give another name for the variable; input=PORT"
		// Live-caught 2026-10-23 on the first Container Apps deploy; the comment that
		// used to sit here asserted the opposite ("Cloud.ru injects nothing that names
		// the port") and was never verified against the API.
		//
		// Nothing is lost by dropping it. The port the platform routes to travels in
		// the app spec as ContainerPort (see the AppContainer literal above), so the
		// platform already knows it, and the runtime defines PORT itself.
		//
		// api.ComputeEnv.CloudProvider is deliberately NOT set. go-aws-lambda-sdk
		// reads it to choose between serving HTTP and starting a cloud's runtime
		// loop, and it has no branch for this cloud — an unrecognised value is a
		// coin-flip over which path an SC-built service takes. A Container App wants
		// the plain HTTP server, which is what the variable's absence selects.
	}
	for name, value := range params.BaseEnvVariables {
		values[name] = value
	}
	for _, v := range params.ComputeContext.EnvVariables() {
		values[v.Name] = v.Value
	}
	for _, v := range params.ComputeContext.SecretEnvVariables() {
		values[v.Name] = v.Value
	}
	// The client stack's own blocks win over anything a `uses:` dependency
	// contributed, same precedence as every other driver.
	for name, value := range stackConfig.Env {
		values[name] = value
	}
	for name, value := range stackConfig.Secrets {
		values[name] = value
	}

	// A reserved name reaching the API costs the whole revision, so it is dropped
	// here rather than refused at validation.
	//
	// Refusing would be the usual SC rule (support it or reject it; a silently
	// ignored field is a future incident), and this is the deliberate exception:
	// a client.yaml that shares one `env:` YAML anchor across an AWS stack — where
	// PORT is correct and load-bearing — and this one CANNOT remove the inherited
	// key, because a YAML merge key can add but never delete. That is not
	// hypothetical; it is what the YC stacks hit, and the workaround there was to
	// write the whole env block out twice. An error would make a legitimate
	// multi-cloud stack unshippable, so the drop is logged loudly instead of being
	// silent.
	for _, name := range cloudru.ReservedEnvNames {
		if _, found := values[name]; !found {
			continue
		}
		params.Log.Warn(ctx.Context(), "dropping env variable %q from Container App %q: the name is reserved by the "+
			"Cloud.ru runtime and a revision that sets it is refused outright. The port is carried by the app spec's "+
			"containerPort (%d), so nothing is lost; set a differently-named variable if the image needs the value.",
			name, stack.Name, port)
		delete(values, name)
	}

	names := lo.Keys(values)
	sort.Strings(names)
	out := make([]evolution.AppEnv, 0, len(names))
	for _, name := range names {
		out = append(out, evolution.AppEnv{Name: name, Value: values[name]})
	}
	return out
}

// resultField reads one field out of the JSON document `sc cloudru container-app
// apply` printed, so the stack can export facts only the API knew.
//
// Scanned from the END for a parseable object rather than decoding the whole
// stream: stdout is contractually one JSON document, but SC's own logger can write
// to the same stream, and an operator debugging a deploy by hand will have the
// progress lines in front of them.
func resultField(stdout sdk.StringOutput, pick func(evolution.Result) string) sdk.StringOutput {
	return stdout.ApplyT(func(out string) (string, error) {
		// Empty during a preview: the command has not run and there is nothing to
		// parse. An error here would fail the plan rather than the deploy.
		if strings.TrimSpace(out) == "" {
			return "", nil
		}
		res, err := parseResult(out)
		if err != nil {
			return "", err
		}
		return pick(*res), nil
	}).(sdk.StringOutput)
}

func parseResult(stdout string) (*evolution.Result, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var res evolution.Result
		if err := json.Unmarshal([]byte(line), &res); err == nil && res.Name != "" {
			return &res, nil
		}
	}
	// Does not echo stdout: LoggingNone keeps the command's output out of the log on
	// purpose, and repeating it in an error would undo that.
	return nil, errors.Errorf("could not find a result document in the output of `sc cloudru container-app apply` " +
		"(expected one JSON object on its own line); the `sc` on this runner may be older than this deploy")
}
