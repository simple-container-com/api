// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
)

const (
	// ContainerInvokerRole is the role that lets a caller invoke a Serverless
	// Container over its HTTPS endpoint.
	ContainerInvokerRole = "serverless.containers.invoker"

	// AllUsersMember is YC's "unauthenticated public" pseudo-member. Granting it
	// the invoker role is the analogue of the AWS function URL's `Principal: "*"`.
	AllUsersMember = "system:allUsers"

	// MemoryAlignmentMB is the granularity YC accepts for container memory.
	MemoryAlignmentMB = 128

	// DefaultMemoryMB matches the AWS lambda default so the same client.yaml sizes
	// the same on both clouds.
	DefaultMemoryMB = 128

	// DefaultTimeoutSeconds matches the AWS lambda default.
	DefaultTimeoutSeconds = 10
)

// DefaultContainerRoles are the roles a Serverless Container cannot run without:
// it pulls its own image out of Container Registry, and reads its own secrets
// out of Lockbox. They are granted on top of whatever CloudExtras.Roles asks for.
var DefaultContainerRoles = []string{
	"container-registry.images.puller",
	"lockbox.payloadViewer",
}

// ycResourceNameRegexp is YC's name rule for folder-scoped resources. It is
// checked here rather than left to the API because a violation otherwise
// surfaces mid-deploy as an opaque gRPC InvalidArgument with no field name.
var ycResourceNameRegexp = regexp.MustCompile(`^[a-z]([a-z0-9-]{1,61}[a-z0-9])?$`)

// ServerlessContainerOutput is what the stack exports for a provisioned container.
type ServerlessContainerOutput struct {
	Container      *sdkYandex.ServerlessContainer
	ServiceAccount *sdkYandex.IamServiceAccount
	Secret         *sdkYandex.LockboxSecret
	SecretVersion  *sdkYandex.LockboxSecretVersion
	Registry       *sdkYandex.ContainerRegistry
	Triggers       []*sdkYandex.FunctionTrigger
}

// ServerlessContainer provisions the Yandex Cloud analogue of an AWS lambda: the
// image is pushed to Container Registry, secrets go into Lockbox and are
// *referenced* (never inlined) by the container, schedules become Timer
// Triggers, and the custom domain is a proxied CNAME plus a host-override rule
// at the registrar — the same mechanism the AWS lambda uses, so no YC DNS or API
// Gateway resource is involved.
func ServerlessContainer(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != yandex.TemplateTypeYandexServerlessContainer {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	if input.StackParams == nil {
		return nil, errors.Errorf("missing deploy params for %q in stack %q", input.Descriptor.Type, stack.Name)
	}
	deployParams := *input.StackParams

	crInput, ok := input.Descriptor.Config.Config.(*yandex.ServerlessContainerInput)
	if !ok {
		return nil, errors.Errorf("failed to convert %q config for %q in stack %q in %q",
			yandex.TemplateTypeYandexServerlessContainer, input.Descriptor.Type, stack.Name, deployParams.Environment)
	}
	if err := api.ConvertAuth(crInput, &crInput.AccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to yandex.AccountConfig")
	}
	if params.Provider == nil {
		return nil, errors.Errorf("provider must not be nil for serverless container in stack %q", stack.Name)
	}
	if crInput.FolderID == "" {
		return nil, errors.Errorf("folderId must be set for serverless container in stack %q", stack.Name)
	}

	stackConfig := crInput.StackConfig
	if stackConfig.Image == nil {
		return nil, errors.Errorf("image must be configured for serverless container in stack %q", stack.Name)
	}

	extras, err := crInput.CloudExtras()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read cloudExtras for serverless container in stack %q", stack.Name)
	}

	containerName := input.ToResName(stack.Name)
	if err := validateYcResourceName(containerName); err != nil {
		return nil, errors.Wrapf(err, "invalid serverless container name for stack %q", stack.Name)
	}

	memoryMb := lo.If(stackConfig.MaxMemory == nil, DefaultMemoryMB).Else(lo.FromPtr(stackConfig.MaxMemory))
	if memoryMb <= 0 || memoryMb%MemoryAlignmentMB != 0 {
		// Rounding here would quietly change what the operator asked for, and the
		// same client.yaml is shared with the AWS deploy where any value is legal.
		return nil, errors.Errorf("maxMemory must be a positive multiple of %d MB for serverless container in stack %q, got %d",
			MemoryAlignmentMB, stack.Name, memoryMb)
	}
	timeoutSeconds := lo.If(stackConfig.Timeout != nil, lo.FromPtr(stackConfig.Timeout)).Else(DefaultTimeoutSeconds)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		sdk.DependsOn(params.ComputeContext.Dependencies()),
	}

	image, err := buildAndPushDockerImage(ctx, stack, params, deployParams, crInput, dockerImage{
		name:       stack.Name,
		dockerfile: stackConfig.Image.Dockerfile,
		context:    stackConfig.Image.Context,
		args:       lo.FromPtr(stackConfig.Image.Build).Args,
		version:    lo.If(deployParams.Version != "", deployParams.Version).Else("latest"),
		platform:   stackConfig.Image.Platform,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image for serverless container in stack %q env %q", stack.Name, deployParams.Environment)
	}
	opts = append(opts, image.addOpts...)

	serviceAccountID, serviceAccount, err := provisionContainerServiceAccount(ctx, stack, params, crInput, extras, containerName, opts)
	if err != nil {
		return nil, err
	}
	if serviceAccount != nil {
		opts = append(opts, sdk.DependsOn([]sdk.Resource{serviceAccount}))
	}

	// SECRETS — one Lockbox secret per stack holding every entry, referenced by
	// the container rather than inlined into its environment. One secret with N
	// entries rather than N secrets is deliberate: Lockbox quotas and billing are
	// per-secret, and the container's Secrets block addresses entries by key.
	secretEnvVariables := lo.Filter(params.ComputeContext.SecretEnvVariables(), func(s pApi.ComputeEnvVariable, _ int) bool {
		return stackConfig.Secrets[s.Name] == ""
	})
	secretValues := map[string]string{}
	for _, v := range secretEnvVariables {
		secretValues[v.Name] = v.Value
	}
	for name, value := range stackConfig.Secrets {
		secretValues[name] = value
	}

	lockboxSecret, lockboxVersion, containerSecrets, err := provisionContainerSecrets(ctx, stack, params, crInput, containerName, secretValues, opts)
	if err != nil {
		return nil, err
	}
	if lockboxVersion != nil {
		opts = append(opts, sdk.DependsOn([]sdk.Resource{lockboxVersion}))
	}

	// ENV VARIABLES — everything that is not a secret. Secret-backed variables are
	// injected by YC itself from the Lockbox reference above, so a name appearing
	// in both places must not also be set here.
	contextEnvVariables := lo.Filter(params.ComputeContext.EnvVariables(), func(v pApi.ComputeEnvVariable, _ int) bool {
		return stackConfig.Env[v.Name] == ""
	})
	envVariables := sdk.StringMap{
		api.ComputeEnv.StackName:    sdk.String(stack.Name),
		api.ComputeEnv.StackEnv:     sdk.String(deployParams.Environment),
		api.ComputeEnv.StackVersion: sdk.String(deployParams.Version),
		// Yandex's Serverless Containers runtime defines exactly PORT and
		// REQUEST_PATH — nothing that names the cloud. So the deployed binary
		// cannot tell where it is running, and go-aws-lambda-sdk reads this
		// variable to decide between serving HTTP and starting the Lambda
		// runtime loop (pkg/service/yandex.go, IsYandexCloudRuntime).
		api.ComputeEnv.CloudProvider: sdk.String(yandex.ProviderType),
	}
	for name, value := range params.BaseEnvVariables {
		envVariables[name] = sdk.String(value)
	}
	for _, envVar := range contextEnvVariables {
		envVariables[envVar.Name] = sdk.String(envVar.Value)
	}
	for name, value := range stackConfig.Env {
		envVariables[name] = sdk.String(value)
	}
	for name := range secretValues {
		delete(envVariables, name)
	}

	containerArgs := &sdkYandex.ServerlessContainerArgs{
		Name:             sdk.String(containerName),
		FolderId:         sdk.String(crInput.FolderID),
		Description:      sdk.String(fmt.Sprintf("%s (%s), deployed by simple-container", stack.Name, deployParams.Environment)),
		Memory:           sdk.Int(memoryMb),
		ExecutionTimeout: sdk.String(fmt.Sprintf("%ds", timeoutSeconds)),
		ServiceAccountId: serviceAccountID,
		Image: sdkYandex.ServerlessContainerImageArgs{
			// The digest ref, not the tag: a tag can be moved after the image was
			// built, scanned and signed.
			Url:         image.deployImageRef,
			Environment: envVariables,
		},
	}
	if len(containerSecrets) > 0 {
		containerArgs.Secrets = containerSecrets
	}
	if extras.Concurrency != nil {
		containerArgs.Concurrency = sdk.IntPtr(lo.FromPtr(extras.Concurrency))
	}
	if extras.ProvisionedInstances != nil {
		containerArgs.ProvisionPolicy = sdkYandex.ServerlessContainerProvisionPolicyArgs{
			MinInstances: sdk.Int(lo.FromPtr(extras.ProvisionedInstances)),
		}
	}

	params.Log.Info(ctx.Context(), "configure serverless container %q for %q in %q...", containerName, stack.Name, deployParams.Environment)
	container, err := sdkYandex.NewServerlessContainer(ctx, containerName, containerArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create serverless container %q", containerName)
	}
	ctx.Export(fmt.Sprintf("%s-%s-container-id", stack.Name, deployParams.Environment), container.ID())
	ctx.Export(fmt.Sprintf("%s-%s-container-url", stack.Name, deployParams.Environment), container.Url)
	opts = append(opts, sdk.DependsOn([]sdk.Resource{container}))

	// Without this the container's endpoint answers 403 to everyone, including the
	// Cloudflare worker that fronts the custom domain. It is the direct analogue of
	// the `Principal: "*"` the AWS lambda grants on its function URL; the service
	// behind it does its own authentication.
	if _, err = sdkYandex.NewServerlessContainerIamBinding(ctx, fmt.Sprintf("%s-invoker", containerName), &sdkYandex.ServerlessContainerIamBindingArgs{
		ContainerId: container.ID().ToStringOutput(),
		Role:        sdk.String(ContainerInvokerRole),
		Members:     sdk.StringArray{sdk.String(AllUsersMember)},
	}, opts...); err != nil {
		return nil, errors.Wrapf(err, "failed to grant %q on container %q", ContainerInvokerRole, containerName)
	}

	if stackConfig.Domain != "" {
		if _, err := provisionDNSForContainer(ctx, stack, params, containerName, stackConfig.Domain, container.Url); err != nil {
			return nil, errors.Wrapf(err, "failed to provision DNS for serverless container %q", containerName)
		}
	}

	var triggers []*sdkYandex.FunctionTrigger
	for _, schedule := range extras.Schedules {
		trigger, err := provisionScheduleForContainer(ctx, stack, params, crInput, containerName, container, serviceAccountID, schedule, opts)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision schedule %q for serverless container %q", schedule.Name, containerName)
		}
		triggers = append(triggers, trigger)
	}

	return &api.ResourceOutput{
		Ref: &ServerlessContainerOutput{
			Container:      container,
			ServiceAccount: serviceAccount,
			Secret:         lockboxSecret,
			SecretVersion:  lockboxVersion,
			Registry:       image.registry,
			Triggers:       triggers,
		},
	}, nil
}

// provisionContainerServiceAccount returns the identity the container runs as.
//
// When the operator names an existing service account, SC does not manage its
// IAM at all — folder role bindings are additive, so two stacks each "owning"
// the same (folder, role, member) triple would let destroying one silently strip
// the grant from the other. A roles list alongside serviceAccountId is therefore
// rejected rather than ignored.
func provisionContainerServiceAccount(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams,
	crInput *yandex.ServerlessContainerInput, extras *yandex.CloudExtras, containerName string, opts []sdk.ResourceOption,
) (sdk.StringInput, *sdkYandex.IamServiceAccount, error) {
	if crInput.ServiceAccountID != "" {
		if len(extras.Roles) > 0 {
			return nil, nil, errors.Errorf(
				"cloudExtras.roles cannot be combined with serviceAccountId %q in stack %q: simple-container does not manage the IAM of a service account it did not create — grant the roles out of band or drop serviceAccountId",
				crInput.ServiceAccountID, stack.Name,
			)
		}
		params.Log.Info(ctx.Context(), "serverless container %q will run as pre-existing service account %q", containerName, crInput.ServiceAccountID)
		return sdk.String(crInput.ServiceAccountID), nil, nil
	}

	saName := fmt.Sprintf("%s-sa", containerName)
	if err := validateYcResourceName(saName); err != nil {
		return nil, nil, errors.Wrapf(err, "invalid service account name for stack %q", stack.Name)
	}
	params.Log.Info(ctx.Context(), "configure service account %q for serverless container %q...", saName, containerName)
	sa, err := sdkYandex.NewIamServiceAccount(ctx, saName, &sdkYandex.IamServiceAccountArgs{
		Name:        sdk.String(saName),
		FolderId:    sdk.String(crInput.FolderID),
		Description: sdk.String(fmt.Sprintf("service account for serverless container %s (stack %s)", containerName, stack.Name)),
	}, opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create service account for serverless container %q", containerName)
	}

	member := sa.ID().ToStringOutput().ApplyT(func(id string) string {
		return fmt.Sprintf("serviceAccount:%s", id)
	}).(sdk.StringOutput)
	for _, role := range lo.Uniq(append(append([]string{}, DefaultContainerRoles...), extras.Roles...)) {
		bindingName := fmt.Sprintf("%s-role-%s", containerName, strings.NewReplacer(".", "-", "_", "-").Replace(role))
		// ResourcemanagerFolderIamMember is additive. ...FolderIamPolicy is
		// authoritative and would wipe every other binding in the folder — never
		// reach for it here.
		if _, err := sdkYandex.NewResourcemanagerFolderIamMember(ctx, bindingName, &sdkYandex.ResourcemanagerFolderIamMemberArgs{
			FolderId: sdk.String(crInput.FolderID),
			Role:     sdk.String(role),
			Member:   member,
		}, append(opts, sdk.DependsOn([]sdk.Resource{sa}))...); err != nil {
			return nil, nil, errors.Wrapf(err, "failed to bind role %q for serverless container %q", role, containerName)
		}
	}
	return sa.ID().ToStringOutput(), sa, nil
}

// provisionContainerSecrets stores every secret of the stack as entries of a
// single Lockbox secret and returns the references the container reads them
// through. A stack with no secrets gets no Lockbox resources at all — a version
// with zero entries is rejected by the API.
func provisionContainerSecrets(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams,
	crInput *yandex.ServerlessContainerInput, containerName string, secretValues map[string]string, opts []sdk.ResourceOption,
) (*sdkYandex.LockboxSecret, *sdkYandex.LockboxSecretVersion, sdkYandex.ServerlessContainerSecretArray, error) {
	if len(secretValues) == 0 {
		return nil, nil, nil, nil
	}

	// Sorted, because map iteration order would otherwise show up as a spurious
	// diff on the version's entries on every single deploy.
	names := lo.Keys(secretValues)
	sort.Strings(names)

	secretName := fmt.Sprintf("%s-secrets", containerName)
	if err := validateYcResourceName(secretName); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "invalid lockbox secret name for stack %q", stack.Name)
	}
	params.Log.Info(ctx.Context(), "configure lockbox secret %q with %d entries for stack %q...", secretName, len(names), stack.Name)
	secret, err := sdkYandex.NewLockboxSecret(ctx, secretName, &sdkYandex.LockboxSecretArgs{
		Name:        sdk.String(secretName),
		FolderId:    sdk.String(crInput.FolderID),
		Description: sdk.String(fmt.Sprintf("secrets for serverless container %s (stack %s)", containerName, stack.Name)),
	}, opts...)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to create lockbox secret %q", secretName)
	}

	entries := sdkYandex.LockboxSecretVersionEntryArray{}
	for _, name := range names {
		entries = append(entries, sdkYandex.LockboxSecretVersionEntryArgs{
			Key:       sdk.String(name),
			TextValue: sdk.ToSecret(sdk.String(secretValues[name])).(sdk.StringOutput).ToStringPtrOutput(),
		})
	}
	version, err := sdkYandex.NewLockboxSecretVersion(ctx, fmt.Sprintf("%s-version", secretName), &sdkYandex.LockboxSecretVersionArgs{
		SecretId: secret.ID().ToStringOutput(),
		Entries:  entries,
	}, append(opts, sdk.DependsOn([]sdk.Resource{secret}))...)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to create lockbox secret version for %q", secretName)
	}

	refs := sdkYandex.ServerlessContainerSecretArray{}
	for _, name := range names {
		refs = append(refs, sdkYandex.ServerlessContainerSecretArgs{
			Id: secret.ID().ToStringOutput(),
			// Pinning the version rather than tracking "latest" is what makes a
			// secret change roll the container: a new version id is a new container
			// revision.
			VersionId:           version.ID().ToStringOutput(),
			Key:                 sdk.String(name),
			EnvironmentVariable: sdk.String(name),
		})
	}
	return secret, version, refs, nil
}

// provisionDNSForContainer points a custom domain at the container's invoke URL.
// It is the AWS lambda's provisionDNSForLambda, unchanged in substance: a proxied
// CNAME plus a host-override rule, which Cloudflare implements as a worker that
// rewrites Host. No Yandex DNS zone and no API Gateway are involved.
func provisionDNSForContainer(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, containerName, domain string, endpointUrl sdk.StringOutput,
) (*api.ResourceOutput, error) {
	params.Log.Info(ctx.Context(), "configure CNAME DNS record %q for stack %q...", domain, stack.Name)

	endpointHost := endpointUrl.ApplyT(func(epUrl string) (string, error) {
		parsed, err := url.Parse(epUrl)
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse URL %q", epUrl)
		}
		return parsed.Host, nil
	}).(sdk.StringOutput)
	record, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
		Name:     domain,
		Type:     "CNAME",
		ValueOut: endpointHost,
		Proxied:  true,
	})
	if err != nil {
		params.Log.Error(ctx.Context(), "failed to create DNS record %q: %s", domain, err.Error())
		return nil, errors.Wrapf(err, "failed to create DNS record %q", domain)
	}
	if _, err := params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
		Name:     containerName,
		FromHost: domain,
		ToHost:   endpointHost,
	}); err != nil {
		params.Log.Error(ctx.Context(), "failed to create override header rule for %q", domain)
		return nil, errors.Wrapf(err, "failed to create override host rule from %q", domain)
	}
	return record, nil
}

// provisionScheduleForContainer turns a cloudExtras schedule into a Timer Trigger.
//
// The payload is POSTed to the container's root path as-is. A schedule written
// for AWS carries an API-Gateway event envelope, which the container will only
// understand once the SDK grows a YC runtime mode — that is a separate slice, and
// a schedule that fires into a handler that cannot decode it is a dead job, not a
// working one.
func provisionScheduleForContainer(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, crInput *yandex.ServerlessContainerInput,
	containerName string, container *sdkYandex.ServerlessContainer, serviceAccountID sdk.StringInput,
	schedule yandex.ContainerSchedule, opts []sdk.ResourceOption,
) (*sdkYandex.FunctionTrigger, error) {
	if err := schedule.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid schedule %q", schedule.Name)
	}
	triggerName := fmt.Sprintf("%s-%s", containerName, schedule.Name)
	if err := validateYcResourceName(triggerName); err != nil {
		return nil, errors.Wrapf(err, "invalid trigger name for schedule %q", schedule.Name)
	}

	serviceAccountIDPtr := serviceAccountID.ToStringOutput().ToStringPtrOutput()
	containerArgs := &sdkYandex.FunctionTriggerContainerArgs{
		Id:               container.ID().ToStringOutput(),
		ServiceAccountId: serviceAccountIDPtr,
	}
	if schedule.RetryAttempts != nil {
		containerArgs.RetryAttempts = sdk.StringPtr(strconv.Itoa(lo.FromPtr(schedule.RetryAttempts)))
	}
	interval, err := schedule.EffectiveRetryInterval()
	if err != nil {
		return nil, errors.Wrapf(err, "invalid retryInterval for schedule %q", schedule.Name)
	}
	if interval > 0 {
		containerArgs.RetryInterval = sdk.StringPtr(fmt.Sprintf("%ds", int(interval.Seconds())))
	}

	triggerArgs := &sdkYandex.FunctionTriggerArgs{
		Name:        sdk.String(triggerName),
		FolderId:    sdk.String(crInput.FolderID),
		Description: sdk.String(fmt.Sprintf("schedule %s for serverless container %s", schedule.Name, containerName)),
		Timer: &sdkYandex.FunctionTriggerTimerArgs{
			CronExpression: sdk.String(schedule.NormalizedExpression()),
			Payload:        lo.If(schedule.Request != "", sdk.StringPtr(schedule.Request)).Else(nil),
		},
		Container: containerArgs,
	}
	if schedule.DLQ != "" {
		triggerArgs.Dlq = &sdkYandex.FunctionTriggerDlqArgs{
			QueueId:          sdk.String(schedule.DLQ),
			ServiceAccountId: serviceAccountID,
		}
	}

	params.Log.Info(ctx.Context(), "configure timer trigger %q (%q) for serverless container %q in stack %q...",
		triggerName, schedule.NormalizedExpression(), containerName, stack.Name)
	trigger, err := sdkYandex.NewFunctionTrigger(ctx, triggerName, triggerArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create timer trigger %q", triggerName)
	}
	return trigger, nil
}

func validateYcResourceName(name string) error {
	if !ycResourceNameRegexp.MatchString(name) {
		return errors.Errorf("%q is not a valid yandex cloud resource name: must be 2-63 characters, start with a lowercase letter, end with a lowercase letter or digit, and contain only lowercase letters, digits and hyphens", name)
	}
	return nil
}
