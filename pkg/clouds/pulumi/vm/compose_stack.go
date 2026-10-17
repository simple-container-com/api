// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// VmComposeOutput is what a vm-compose deploy hands back.
type VmComposeOutput struct {
	Host   HostFacts
	Images *imagePlan
	// Slot is the slot this deploy brought up. An Output because it is decided from
	// state read off the machine.
	Slot sdk.StringOutput
	// URL is where the stack answers after this deploy. In P5.2 that is always
	// http://<host>:<port> — see the `domain` warning in VmComposeStack.
	URL sdk.StringOutput
	Up  *remote.Command
}

// VmComposeStack deploys one client stack onto one VM as docker compose projects.
//
// The shape of the deploy, and why it is this many commands rather than one script:
//
//	prepare        networks, external volumes, directories — idempotent, no secrets
//	write-env      the env file, 0600, content on stdin
//	write-vol-*    each `textVolumes` entry
//	registry-login only when imageDelivery is `registry`
//	read-state     which slot is serving traffic right now
//	up             write the compose files, bring the slot up, gate on health
//	finalize       record the new active slot, retire the old one, prune images
//	teardown       a no-op that cleans up on destroy
//
// The split is not cosmetic. Each boundary is a point where the deploy must be able
// to stop without having done the next thing: a failed health gate must not record a
// new active slot, and a failed env write must not bring containers up against a
// stale one. Running it as a single script would make every one of those an
// all-or-nothing and would put the env file's content — every secret of the stack —
// into the command string, where Pulumi stores it unencrypted and prints it in a
// diff (compose-on-vm.md §0.7, §7.1).
func VmComposeStack(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != vm.TemplateTypeVmCompose {
		return nil, errors.Errorf("unsupported template type %q", input.Descriptor.Type)
	}
	in, ok := input.Descriptor.Config.Config.(*vm.VmComposeInput)
	if !ok {
		return nil, errors.Errorf("failed to cast config to vm.VmComposeInput for %q (given %T)",
			input.Descriptor.Type, input.Descriptor.Config.Config)
	}
	if err := in.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid `%s` template config for stack %q", vm.TemplateTypeVmCompose, stack.Name)
	}
	if in.Deployment.StackConfig == nil {
		return nil, errors.Errorf("stack %q has no stack config; nothing to deploy", stack.Name)
	}
	if params.ParentStack == nil {
		return nil, errors.Errorf("`%s` template requires a parent stack reference", vm.TemplateTypeVmCompose)
	}

	stackName := input.StackParams.StackName
	env := input.StackParams.Environment
	if err := vm.ValidateNames(stackName, env); err != nil {
		return nil, err
	}
	version := lo.If(input.StackParams.Version != "", input.StackParams.Version).Else("latest")
	stackConfig := in.Deployment.StackConfig

	host, ingressEnabled, err := hostFromParent(ctx, input, params, in.VmResource)
	if err != nil {
		return nil, err
	}

	plan, err := buildImages(ctx, stack, input, params, in, host)
	if err != nil {
		return nil, err
	}
	// Logged after the build, not before: buildImages adds to this list when it has
	// to guess a third-party image reference, and a warning nobody prints is the
	// thing the "support it or refuse it" rule exists to prevent (§2.2).
	for _, w := range in.Warnings {
		params.Log.Warn(ctx.Context(), "stack %q on vm %q: %s", stackName, in.VmResource, w)
	}

	// The ingress host port. In P5.2 it is always published, whatever `ingress.enable`
	// says, because the thing that would otherwise front it does not exist yet.
	hostPort := 0
	if in.Deployment.IngressContainer != nil {
		hostPort = vm.IngressHostPort(stackName, env)
		if ingressEnabled {
			params.Log.Warn(ctx.Context(),
				"vm %q has `ingress.enable: true`, but Caddy and TLS arrive with the next slice (P5.3): "+
					"publishing %q directly on host port %d instead. Expect plain HTTP and no certificate until then",
				in.VmResource, in.Deployment.IngressContainer.Name, hostPort)
		} else {
			params.Log.Info(ctx.Context(), "publishing ingress service %q of stack %q on host port %d",
				in.Deployment.IngressContainer.Name, stackName, hostPort)
		}
	} else {
		params.Log.Warn(ctx.Context(),
			"stack %q declares no ingress service, so nothing is reachable from outside the machine", stackName)
	}
	if stackConfig.Domain != "" {
		// Refusing would be the other defensible choice. Warning wins because the
		// stack is genuinely deployed and genuinely works on the host port — and
		// because the client.yaml that names a domain is the same one P5.3 will
		// serve unchanged, so making operators delete and re-add the field would be
		// churn with no safety gained.
		params.Log.Warn(ctx.Context(),
			"stack %q sets `domain: %s`, but this slice does not serve domains: no DNS record is created and no "+
				"certificate is issued. Reach the stack on http://<host>:%d until Caddy lands (P5.3)",
			stackName, stackConfig.Domain, hostPort)
	}

	conn := connectionArgs(host)
	// Shared by every command: Pulumi's own default is to log a remote command's
	// output, and AddPreviousOutputInEnv would put the last run's stdout into this
	// run's environment — harmless for `echo prepared`, and a way for a secret that
	// surfaced once to be re-injected on the next deploy.
	baseArgs := func(a *remote.CommandArgs) *remote.CommandArgs {
		a.Connection = conn
		a.AddPreviousOutputInEnv = sdk.Bool(false)
		return a
	}

	// Sequenced rather than fanned out. Three commands over one SSH connection
	// writing into one directory is not worth the concurrency, and a linear chain is
	// what makes the failure message name the step that failed.
	var prev []sdk.Resource
	step := func(name, script string, stdin sdk.StringPtrInput, quiet bool, extraTriggers ...sdk.Input) (*remote.Command, error) {
		triggers := sdk.Array{host.Address, sdk.String(hashOf(script))}
		for _, t := range extraTriggers {
			triggers = append(triggers, t)
		}
		args := baseArgs(&remote.CommandArgs{
			Create: sdk.String(script),
			// Update is set on every command in this file, always to the same value
			// as Create. Without it pulumi-command turns any input change into a
			// REPLACEMENT, which it performs by deleting and re-creating — and for a
			// command whose Create brings containers up, that is an outage on every
			// edit (compose-on-vm.md §0.7).
			Update:   sdk.String(script),
			Triggers: triggers,
			Stdin:    stdin,
		})
		if quiet {
			args.Logging = remote.LoggingNone.ToLoggingPtrOutput()
		}
		cmd, err := remote.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-vmc-%s", stackName, name)), args,
			sdk.DependsOn(prev))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create %q command for stack %q on vm %q", name, stackName, in.VmResource)
		}
		prev = []sdk.Resource{cmd}
		return cmd, nil
	}

	if _, err := step("prepare", prepareScript(stackName, env, in.ExternalVolumes()), nil, false); err != nil {
		return nil, err
	}

	// env file. `extra` carries what the stack cannot know about itself: the base
	// variables the cloud provider contributes, and the values resolved from
	// `uses`/`dependencies`. Filtered the same way yandex/serverless_container.go
	// filters them, so an explicit `env:` or `secrets:` in client.yaml still wins
	// over a resolved one of the same name.
	//
	// SIMPLE_CONTAINER_VERSION and SIMPLE_CONTAINER_CLOUD are deliberately absent:
	// pkg/clouds/vm/render.go puts them in each service's compose `environment:`,
	// which takes precedence over `env_file`, and two sources for one variable is how
	// they come to disagree.
	extra := map[string]string{
		api.ComputeEnv.StackName: stackName,
		api.ComputeEnv.StackEnv:  env,
	}
	for name, value := range params.BaseEnvVariables {
		extra[name] = value
	}
	for _, v := range params.ComputeContext.EnvVariables() {
		if stackConfig.Env[v.Name] == "" {
			extra[v.Name] = v.Value
		}
	}
	for _, v := range params.ComputeContext.SecretEnvVariables() {
		if stackConfig.Secrets[v.Name] == "" {
			extra[v.Name] = v.Value
		}
	}
	envContent, err := vm.RenderEnvFile(in, extra)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to render the env file of stack %q", stackName)
	}
	// Hashed, never triggered on directly: a Trigger is stored in the checkpoint in
	// the clear, and this content is every secret the stack has.
	envHash := hashOf(envContent)
	if _, err := step("write-env",
		writeFileScript(vm.EnvFilePath(stackName, env), vm.EnvFileMode),
		secretString(envContent).ToStringPtrOutput(), true,
		sdk.String(envHash),
	); err != nil {
		return nil, err
	}

	for _, f := range vm.TextVolumeFiles(in, stackName, env) {
		file := f
		if _, err := step("write-vol-"+sanitizeResName(file.MountPath),
			writeFileScript(file.Path, file.Mode),
			secretString(file.Content).ToStringPtrOutput(), true,
			sdk.String(hashOf(file.Content)),
		); err != nil {
			return nil, err
		}
	}

	pull := pullBestEffort
	if in.EffectiveImageDelivery() == vm.ImageDeliveryRegistry {
		pull = pullRequired
		if in.RegistryRequiresAuth() {
			if _, err := step("registry-login",
				registryLoginScript(lo.FromPtr(in.DockerRegistryURL), lo.FromPtr(in.DockerRegistryUsername)),
				secretString(lo.FromPtr(in.DockerRegistryPassword)).ToStringPtrOutput(), true,
			); err != nil {
				return nil, err
			}
		} else {
			params.Log.Info(ctx.Context(), "registry %q has no credentials configured: the machine will pull anonymously",
				lo.FromPtr(in.DockerRegistryURL))
		}
	}

	// Which slot is serving traffic. Read from the machine rather than from Pulumi
	// state because the machine is the only place that knows: a stack can be
	// deployed from two checkouts, and a container that was stopped by hand is the
	// machine's truth, not the checkpoint's.
	readState, err := step("read-state", readStateScript(stackName, env), nil, false)
	if err != nil {
		return nil, err
	}

	recreate := in.EffectiveStrategy() == vm.StrategyRecreate
	slotOut := readState.Stdout.ApplyT(func(out string) string {
		active := strings.TrimSpace(out)
		if recreate {
			return lo.If(lo.Contains(vm.Slots, active), active).Else(vm.SlotA)
		}
		// OtherSlot maps an empty or unrecognised active slot to SlotA, which is
		// what a machine that has never been deployed to should get.
		return vm.OtherSlot(active)
	}).(sdk.StringOutput)

	// A published host port is a singleton, so the two slots cannot both be up while
	// one of them holds it: the new slot's `up` fails with "port is already
	// allocated". Until Caddy fronts the stack (P5.3) and the port stops being
	// published, the only correct order is to stop the old slot first and accept a
	// few seconds of downtime. Not a deviation we chose — a consequence of blue-green
	// without a reverse proxy, recorded in the design doc so that P5.3 can delete it.
	stopOtherFirst := hostPort > 0 && !recreate

	// Everything from here needs the slot, and the slot is an Output — so the script
	// is built inside an apply over it and over every image reference.
	services := lo.Keys(plan.Refs)
	sort.Strings(services)
	upInputs := []any{slotOut}
	for _, svc := range services {
		upInputs = append(upInputs, plan.Refs[svc])
	}
	upScriptOut := sdk.All(upInputs...).ApplyT(func(vals []any) (string, error) {
		slot, _ := vals[0].(string)
		images := map[string]string{}
		for i, svc := range services {
			ref, _ := vals[i+1].(string)
			images[svc] = ref
		}
		rendered, err := vm.Render(in, vm.RenderOptions{
			Stack:           stackName,
			Env:             env,
			Slot:            slot,
			Version:         version,
			Images:          images,
			IngressHostPort: hostPort,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to render compose files for stack %q slot %q", stackName, slot)
		}
		return upScript(upSpec{
			Stack:          stackName,
			Env:            env,
			Slot:           slot,
			OtherSlot:      vm.OtherSlot(slot),
			Recreate:       recreate,
			SlotProject:    rendered.SlotProject,
			SlotContent:    rendered.SlotFile,
			SlotGates:      rendered.SlotServices,
			DataProject:    rendered.DataProject,
			DataContent:    rendered.DataFile,
			DataGates:      rendered.DataServices,
			StopOtherFirst: stopOtherFirst,
			Pull:           pull,
		}), nil
	}).(sdk.StringOutput)

	up, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-up"), baseArgs(&remote.CommandArgs{
		Create: upScriptOut,
		Update: upScriptOut,
		// The script itself is the trigger, which covers everything that can change
		// what the deploy would do: the slot, every image reference, the rendered
		// compose files, the host port. The version is listed separately so that a
		// forced redeploy of a byte-identical render still runs.
		Triggers: sdk.Array{host.Address, sdk.String(version), upScriptOut, sdk.String(envHash)},
	}), append(plan.Opts, sdk.DependsOn(prev))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to bring stack %q up on vm %q", stackName, in.VmResource)
	}

	finalizeOut := slotOut.ApplyT(func(slot string) string {
		stopSlot := ""
		if !recreate {
			stopSlot = vm.OtherSlot(slot)
		}
		return finalizeScript(finalizeSpec{
			Stack:      stackName,
			Env:        env,
			Slot:       slot,
			StopSlot:   stopSlot,
			Version:    version,
			PruneRepos: plan.PruneRepos,
			Keep:       in.EffectiveKeepImages(),
		})
	}).(sdk.StringOutput)

	if _, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-finalize"), baseArgs(&remote.CommandArgs{
		Create:   finalizeOut,
		Update:   finalizeOut,
		Triggers: sdk.Array{host.Address, sdk.String(version), finalizeOut},
	}), sdk.DependsOn([]sdk.Resource{up})); err != nil {
		return nil, errors.Wrapf(err, "failed to finalize deploy of stack %q on vm %q", stackName, in.VmResource)
	}

	// Cleanup lives on its own resource whose Create does nothing, and deliberately
	// has NO Triggers. Hanging the teardown off `up`'s Delete would be shorter and
	// is a trap: pulumi-command performs a replacement as create-then-delete, so any
	// input change that Pulumi chose to treat as a replace would tear the stack down
	// moments after bringing it up. A resource with no triggers is never replaced,
	// so its Delete runs exactly when the stack is destroyed.
	if _, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-teardown"), baseArgs(&remote.CommandArgs{
		Create: sdk.String("true"),
		Delete: sdk.String(teardownScript(stackName, env)),
	})); err != nil {
		return nil, errors.Wrapf(err, "failed to register teardown of stack %q on vm %q", stackName, in.VmResource)
	}

	url := sdk.Sprintf("http://%s:%d", host.Address, hostPort)
	ctx.Export(toComposeSlotExport(stackName), slotOut)
	ctx.Export(toComposeURLExport(stackName), url)

	return &api.ResourceOutput{Ref: &VmComposeOutput{
		Host:   host,
		Images: plan,
		Slot:   slotOut,
		URL:    url,
		Up:     up,
	}}, nil
}

func toComposeSlotExport(stackName string) string {
	return fmt.Sprintf("%s-vm-compose-slot", stackName)
}

func toComposeURLExport(stackName string) string {
	return fmt.Sprintf("%s-vm-compose-url", stackName)
}

// hostFromParent reads the facts of the machine out of the parent stack's outputs.
//
// The machine is described in the parent stack and deployed onto from the child, so
// this is the one seam where the two meet. It returns a HostFacts identical in shape
// to the one the driver produced, which is what lets connectionArgs and shipScript be
// the same code on both sides, plus whether the machine declared `ingress.enable`.
func hostFromParent(
	ctx *sdk.Context,
	input api.ResourceInput,
	params pApi.ProvisionParams,
	vmResource string,
) (HostFacts, bool, error) {
	parentRef := params.ParentStack.FullReference
	suffix := lo.If(params.ParentStack.DependsOnResource != nil,
		"--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	// Every GetValueFromStack call creates its own sdk.StackReference, and two
	// references with the same name in one stack is an error — so the field name is
	// part of it.
	refName := func(field string) string {
		return fmt.Sprintf("%s%s-vmc-%s", vmResource, suffix, field)
	}

	countRaw, err := pApi.GetValueFromStack[any](ctx, refName("host-count"), parentRef, ToHostCountExport(vmResource), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read host count of vm %q from parent stack %q", vmResource, parentRef)
	}
	count, ok := toInt(countRaw)
	if !ok || count < 1 {
		// GetValueFromStack returns (zero, nil) for an output that is not there —
		// errors.Wrapf(nil, ...) is nil — so a missing resource arrives here as a
		// successful read of nothing, and this check is the only thing that turns it
		// into a message naming what is actually wrong.
		return HostFacts{}, false, errors.Errorf(
			"parent stack %q has no `type: %s` resource named %q (its host-count output is %v). "+
				"Check `vmResource` in the `%s` template against the resource name in the parent's server.yaml",
			parentRef, vm.ResourceTypeVm, vmResource, countRaw, vm.TemplateTypeVmCompose)
	}
	if count > 1 {
		params.Log.Warn(ctx.Context(),
			"vm %q has %d hosts; this slice deploys onto the first one only. Multi-host compose is not blue-green "+
				"across machines and is not supported yet", vmResource, count)
	}

	address, err := pApi.GetValueFromStack[string](ctx, refName("address"), parentRef, ToHostAddressExport(vmResource, 0), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the address of vm %q from parent stack %q", vmResource, parentRef)
	}
	if address == "" {
		return HostFacts{}, false, errors.Errorf("vm %q in parent stack %q has an empty host address", vmResource, parentRef)
	}
	privateKey, err := pApi.GetValueFromStack[string](ctx, refName("key"), parentRef, ToHostPrivateKeyExport(vmResource, 0), true)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the deploy key of vm %q from parent stack %q", vmResource, parentRef)
	}
	if privateKey == "" {
		return HostFacts{}, false, errors.Errorf(
			"vm %q in parent stack %q exported no deploy key; re-run `sc provision` for the parent stack", vmResource, parentRef)
	}
	keyPassword, err := pApi.GetValueFromStack[string](ctx, refName("key-password"), parentRef,
		ToHostPrivateKeyPasswordExport(vmResource, 0), true)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the deploy key passphrase of vm %q", vmResource)
	}
	// Not an error when empty, and not read back through a check: an unencrypted key
	// is the normal case and has no passphrase.
	user, err := pApi.GetValueFromStack[string](ctx, refName("user"), parentRef, ToHostUserExport(vmResource, 0), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the ssh user of vm %q", vmResource)
	}
	portRaw, err := pApi.GetValueFromStack[any](ctx, refName("port"), parentRef, ToHostPortExport(vmResource, 0), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the ssh port of vm %q", vmResource)
	}
	hostKey, err := pApi.GetValueFromStack[string](ctx, refName("host-key"), parentRef, ToHostKeyExport(vmResource, 0), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the host key of vm %q", vmResource)
	}
	ingressRaw, err := pApi.GetValueFromStack[any](ctx, refName("ingress"), parentRef, ToIngressEnabledExport(vmResource), false)
	if err != nil {
		return HostFacts{}, false, errors.Wrapf(err, "failed to read the ingress setting of vm %q", vmResource)
	}
	// Read through toBool for the same reason the ints are: an sdk.Bool export comes
	// back off a JSON checkpoint, and GetValueFromStack[bool] would panic rather than
	// fail if the decoder handed it anything else. A machine whose ingress setting is
	// unreadable is treated as not having one — this slice publishes a host port
	// either way, and the only thing the flag changes here is the wording of a
	// warning.
	ingressEnabled, _ := toBool(ingressRaw)

	port, ok := toInt(portRaw)
	if !ok || port <= 0 {
		port = vm.DefaultSshPort
	}
	if hostKey == "" {
		// The parent stack warns too, but this is the side that carries the env
		// file: an unverified connection here hands every secret of this stack to
		// whoever answers on that address.
		params.Log.Warn(ctx.Context(),
			"vm %q has no `hostKey`, so this deploy cannot verify the machine it is sending stack %q's env file "+
				"and deploy key to. Pin it in the parent stack with `ssh-keyscan -t ed25519 %s`",
			vmResource, input.StackParams.StackName, address)
	}

	return HostFacts{
		Name:               fmt.Sprintf("%s-host-0", vmResource),
		Address:            sdk.String(address).ToStringOutput(),
		User:               lo.If(user == "", vm.DefaultSshUser).Else(user),
		Port:               port,
		PrivateKey:         secretString(privateKey),
		PrivateKeyPassword: secretString(keyPassword),
		HostKey:            hostKey,
		Role:               vm.RoleCompose,
	}, ingressEnabled, nil
}

// hashOf is the stand-in for content in a Trigger.
//
// Triggers are stored in the checkpoint in the clear and printed in Pulumi's diff,
// so the env file and the rendered textVolumes cannot be triggered on directly. A
// hash changes exactly when the content does and discloses nothing.
func hashOf(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sanitizeResName makes a Pulumi resource name out of a mount path.
var resNameUnsafe = strings.NewReplacer("/", "-", ".", "-", "_", "-", " ", "-")

func sanitizeResName(s string) string {
	return strings.Trim(resNameUnsafe.Replace(s), "-")
}

// toInt and toBool coerce a value read back from a parent stack's outputs.
//
// Needed because secret-ness is not the only thing GetValueFromStack loses. The vm
// resource exports sdk.Int and sdk.Bool, but a stack reference reads them back out
// of a JSON checkpoint, so what arrives is a float64 (or a json.Number, depending on
// the decoder) — and GetValueFromStack[int] would panic on the type assertion
// rather than fail. Reading them as `any` and coercing here is the fix that does not
// require changing export types other stacks may already depend on.
func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case float32:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		return i, err == nil
	default:
		return 0, false
	}
}

func toBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		return t != 0, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(t))
		return b, err == nil
	default:
		return false, false
	}
}
