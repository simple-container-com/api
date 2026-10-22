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

	// Whether the machine's Caddy serves this stack. One predicate, defined next to
	// the renderer that depends on it, because three things downstream have to agree
	// about the answer: whether a host port is published, whether the two slots may
	// run at once, and what the stack's URL is.
	servesDomain := vm.ServesDomain(in, ingressEnabled)

	// The ingress host port is published only when Caddy is NOT in front. That is not
	// a tidiness choice: a published port is a singleton, and it is the one thing that
	// prevents both slots from running at once (see stopOtherFirst below). Setting it
	// to zero is what makes the deploy blue-green.
	hostPort := 0
	switch {
	case in.Deployment.IngressContainer == nil:
		params.Log.Warn(ctx.Context(),
			"stack %q declares no ingress service, so nothing is reachable from outside the machine", stackName)
	case servesDomain:
		params.Log.Info(ctx.Context(),
			"vm %q serves %q of stack %q on https://%s — no host port is published, and the deploy flips slots "+
				"with a Caddy reload",
			in.VmResource, in.Deployment.IngressContainer.Name, stackName, stackConfig.Domain)
	default:
		hostPort = vm.IngressHostPort(stackName, env)
		params.Log.Info(ctx.Context(), "publishing ingress service %q of stack %q on host port %d",
			in.Deployment.IngressContainer.Name, stackName, hostPort)
	}

	// The two ways a stack can name a domain and not get it. Warned about rather than
	// refused, because the stack is genuinely deployed and genuinely answers on the
	// host port — but said out loud, because "my certificate never appeared" is
	// otherwise a silent failure with no local evidence.
	if stackConfig.Domain != "" && !servesDomain {
		switch {
		case !ingressEnabled:
			params.Log.Warn(ctx.Context(),
				"stack %q sets `domain: %s`, but vm %q does not have `ingress.enable: true`: no certificate is "+
					"issued and nothing listens on 443. Reach the stack on http://<host>:%d, or enable the ingress "+
					"in the parent stack's `type: %s` resource and re-provision",
				stackName, stackConfig.Domain, in.VmResource, hostPort, vm.ResourceTypeVm)
		case in.Deployment.IngressContainer == nil:
			params.Log.Warn(ctx.Context(),
				"stack %q sets `domain: %s` but declares no ingress service, so there is nothing for the machine's "+
					"Caddy to proxy to. Mark the service that serves HTTP with `ingress: true`",
				stackName, stackConfig.Domain)
		}
	}
	if stackConfig.Domain == "" && ingressEnabled && in.Deployment.IngressContainer != nil {
		// The `prefix`-without-`domain` case, among others. On the Kubernetes path a
		// prefix is grafted onto an aggregator site that already owns a domain; on a
		// single VM there is no such site, so there is no host name to attach it to.
		// hostPort is already set by the switch above — this branch only reaches the
		// operator, it does not change the deploy.
		params.Log.Warn(ctx.Context(),
			"vm %q runs Caddy, but stack %q sets no `domain`, so it cannot be served over HTTPS: there is no "+
				"aggregator site on a VM to attach a bare `prefix` to. Publishing on host port %d instead",
			in.VmResource, stackName, hostPort)
	}

	conn := ConnectionArgs(host)
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

	// First, before anything runs `docker`: the preconditions this path needs from
	// the machine. See composePreflightScript for why they are not in the vm
	// resource's readiness gate.
	//
	// runNonce because this command only READS. Its script depends on nothing but
	// the ssh user, so its default trigger set never moves: without the nonce it
	// checks the machine on the first deploy of a stack and is reported unchanged
	// forever after, which is precisely the case it exists to catch (docker removed,
	// the daemon down, the user dropped from the docker group — all of them happen
	// to a host SC does not own, and all of them between deploys, never before the
	// first one).
	if _, err := step("preflight", composePreflightScript(host.User), nil, false, sdk.String(runNonce)); err != nil {
		return nil, err
	}

	// runNonce again, this time for a command that DOES change the machine — every
	// line of prepareScript is inspect-or-create, and the drift it repairs (a pruned
	// `sc-ingress` network, a removed external volume, a deleted stack directory) is
	// invisible to every input this command has. Without the nonce the deploy that
	// would put them back is the one deploy that is skipped, and the failure surfaces
	// one step later as a compose error about a missing network. See
	// TriggersForGuardedChange for the three preconditions this satisfies.
	if _, err := step("prepare", prepareScript(stackName, env, in.ExternalVolumes()), nil, false,
		sdk.String(runNonce)); err != nil {
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
	if _, err := step(
		"write-env",
		writeFileScript(vm.EnvFilePath(stackName, env), vm.EnvFileMode),
		secretString(envContent).ToStringPtrOutput(), true,
		sdk.String(envHash),
	); err != nil {
		return nil, err
	}

	for _, f := range vm.TextVolumeFiles(in, stackName, env) {
		file := f
		if _, err := step(
			"write-vol-"+sanitizeResName(file.MountPath),
			writeFileScript(file.Path, file.Mode),
			secretString(file.Content).ToStringPtrOutput(), true,
			sdk.String(hashOf(file.Content)),
		); err != nil {
			return nil, err
		}
	}

	pull := pullBestEffort
	if in.ShipsExternalImages() {
		// Every image in the file arrived over SSH, so the machine has nothing to
		// fetch. See pullNone for why this is silence rather than best-effort.
		pull = pullNone
	}
	if in.EffectiveImageDelivery() == vm.ImageDeliveryRegistry {
		pull = pullRequired
		if in.RegistryRequiresAuth() {
			if _, err := step(
				"registry-login",
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
	//
	// runNonce is what makes that true. Every other command in this file is
	// triggered by what it would DO, which is right for a command that changes the
	// machine and wrong for the one command that only asks it a question: the
	// address and the script are both invariant, so without a per-run trigger this
	// command runs once, ever, and every later deploy reads its answer out of the
	// checkpoint — the one place the comment above says not to read it from.
	//
	// Live-caught 2026-10-18: the first deploy of the smoke stack read an empty
	// state file (nothing deployed yet) and cached `""`. The machine then recorded
	// `activeSlot: "a"`. The next deploy still saw `""`, so OtherSlot gave it SlotA
	// again — i.e. `strategy: blue-green` would have recreated the slot that was
	// serving, in place, instead of bringing the other one up beside it. Silent, and
	// it converts the one promise of blue-green (no downtime) into a restart.
	readState, err := step("read-state", readStateScript(stackName, env), nil, false, sdk.String(runNonce))
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
	// one of them holds it: the new slot's `up` fails with "port is already allocated",
	// and the only correct order is to stop the old slot first and accept a few seconds
	// of downtime. That is a consequence of blue-green without a reverse proxy, which
	// is why the condition is `hostPort > 0` rather than a flag: when Caddy fronts the
	// stack no port is published, and this relaxes itself.
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
		//
		// But a reference is not a content identity, so plan.ContentIDs is listed
		// too: under `ssh-tar` delivery the reference in that render is a local TAG,
		// which is the same string before and after a rebuild. Without the content
		// ids, a stack deployed at the default version (`latest`) renders byte for
		// byte identically after a code change and this command does not run —
		// see the Triggers of the ship command in compose_images.go.
		Triggers: append(
			sdk.Array{host.Address, sdk.String(version), upScriptOut, sdk.String(envHash)},
			plan.triggerInputs()...),
	}), append(plan.Opts, sdk.DependsOn(prev))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to bring stack %q up on vm %q", stackName, in.VmResource)
	}

	// The flip. Between `up` (both slots running, old one serving) and `finalize`
	// (which retires the old slot) — and it has to be exactly there: routing before
	// the health gate would send traffic to a slot that may never come up, and routing
	// after the old slot is stopped is a gap in which the domain 502s.
	routeDeps := []sdk.Resource{up}
	if servesDomain {
		routeOut := slotOut.ApplyT(func(slot string) (string, error) {
			site, err := vm.RenderCaddySite(in, vm.RenderOptions{
				Stack:   stackName,
				Env:     env,
				Slot:    slot,
				Version: version,
			})
			if err != nil {
				return "", errors.Wrapf(err, "failed to render the Caddy site of stack %q slot %q", stackName, slot)
			}
			return routeScript(routeSpec{
				Stack:   stackName,
				Env:     env,
				Slot:    slot,
				Domain:  stackConfig.Domain,
				Content: site,
			}), nil
		}).(sdk.StringOutput)

		route, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-route"), baseArgs(&remote.CommandArgs{
			Create: routeOut,
			Update: routeOut,
			// The script carries the rendered site file, so it changes whenever the
			// slot, the domain, the upstream port or any lbConfig option does. No
			// Delete: the route is removed by teardownScript along with the rest of
			// the stack, in one place, so that a replacement of THIS resource cannot
			// delete the route moments after writing it.
			//
			// Guarded change on top of that (the no-Delete above is precondition 3 of
			// three; the other two hold: the script writes identical bytes and
			// reloads gracefully). Under `blue-green` the rendered site file names
			// the slot, so it already changed on every deploy and the nonce adds
			// nothing — it is `recreate` and the drift cases that need it. This file
			// is the ONE thing that puts a stack on the machine's ingress, and it
			// lives in a tree the vm resource owns: rebuilding the ingress removes
			// /etc/sc/caddy and every stack's route with it, and so does an operator
			// who cleans up by hand. Without the nonce a `recreate` stack whose route
			// file is gone is served by nobody, and its next deploy — which changes
			// nothing, so it re-runs nothing — reports success and leaves it that
			// way. Live-caught 2026-10-18: a teardown took /etc/sc/caddy with it, the
			// re-provision brought Caddy back with only its catch-all, and the stack's
			// domain answered 503 on :80 and nothing at all on :443.
			Triggers: TriggersForGuardedChange(host.Address, routeOut),
		}), sdk.DependsOn([]sdk.Resource{up}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to route %q to stack %q on vm %q",
				stackConfig.Domain, stackName, in.VmResource)
		}
		routeDeps = []sdk.Resource{route}
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
		Create: finalizeOut,
		Update: finalizeOut,
		// The content ids for the same reason as on `up`, and it matters more here:
		// this is the command that records the active slot and retires the other
		// one. An `up` that ran while this did not leaves the machine serving the new
		// slot with the state file still naming the old one.
		Triggers: append(
			sdk.Array{host.Address, sdk.String(version), finalizeOut},
			plan.triggerInputs()...),
	}), sdk.DependsOn(routeDeps)); err != nil {
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

	// The URL an operator (and `sc deploy`'s own summary) should use. Not derived from
	// whether a domain is configured but from whether it is actually SERVED — a stack
	// whose domain was ignored for one of the reasons warned about above must not
	// export an https:// URL that answers nothing.
	url := sdk.Sprintf("http://%s:%d", host.Address, hostPort)
	if servesDomain {
		url = sdk.String("https://" + stackConfig.Domain).ToStringOutput()
	}
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
			parentRef, vm.ResourceTypeVm, vmResource, countRaw, vm.TemplateTypeVmCompose,
		)
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
			"vm %q in parent stack %q exported no deploy key; re-run `sc provision` for the parent stack", vmResource, parentRef,
		)
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
