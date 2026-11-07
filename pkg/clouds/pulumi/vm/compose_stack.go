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
	// Hosts is every machine of the fleet the stack was deployed onto, in fleet
	// order. Always at least one entry.
	Hosts  []VmComposeHostOutput
	Images *imagePlan

	// Host, Slot, URL and Up are the FIRST host's, and are what the single-machine
	// path returned before a vm resource could hold more than one. Kept because a
	// fleet of one is still the ordinary case; anything that has to account for
	// every machine must read Hosts.
	Host HostFacts
	Slot sdk.StringOutput
	URL  sdk.StringOutput
	Up   *remote.Command
}

// VmComposeHostOutput is one machine's share of a vm-compose deploy.
type VmComposeHostOutput struct {
	Host HostFacts
	// Slot is the slot this deploy brought up ON THIS MACHINE. An Output because it
	// is decided from state read off the machine — and per machine, because each
	// machine's blue-green history is its own: two hosts of one fleet can legitimately
	// be serving from different slots, and forcing them into step would mean reading
	// one machine's state and acting on another's.
	Slot sdk.StringOutput
	// URL is where this machine answers. The fleet's domain when Caddy fronts it (so
	// the same string on every host), otherwise this machine's own address and the
	// published host port.
	URL sdk.StringOutput
	Up  *remote.Command
	// last is the final resource of this host's chain. The next host is ordered after
	// it, which is what makes a fleet deploy roll rather than fan out.
	last sdk.Resource
}

// hostResSuffix is what makes a per-host resource name unique WITHOUT renaming the
// resources of a fleet of one.
//
// Host 0 gets no suffix, so every stack already deployed onto a single-host vm
// resource keeps byte-identical resource names — and so its URNs, its checkpoint
// history and its running containers. Adding a second machine APPENDS resources
// instead of replacing the first machine's: a fleet is grown, not rebuilt.
func hostResSuffix(i int) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("-%d", i)
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

	hosts, parentVm, err := hostsFromParent(ctx, input, params, in.VmResource)
	if err != nil {
		return nil, err
	}

	plan, err := buildImages(ctx, stack, input, params, in)
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
	servesDomain := vm.ServesDomain(in, parentVm.IngressEnabled)

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

	// A host port the machine's firewall does not let through. This is the warning
	// the whole port-resolution seam exists to make possible: before it, a stack
	// deployed onto a shut port was a green deploy, a `curl` that answered 200 FROM
	// the machine, and nothing anywhere saying why the service was unreachable from
	// outside (roadmap item sc-cloudru-vm-no-ingress-ports).
	//
	// A warning rather than a refusal, deliberately: the parent stack and this one
	// are provisioned separately, so the port may well be opened by a re-provision a
	// minute from now, and failing the deploy would make the remedy harder to reach
	// than the problem. Silent when the parent exported nothing — SC either does not
	// manage that firewall or has not been re-provisioned since this was added, and
	// in neither case does it know the answer.
	if hostPort > 0 && len(parentVm.OpenPorts) > 0 && !vm.PortsReachable(parentVm.OpenPorts, hostPort, vm.ProtocolTcp) {
		params.Log.Warn(ctx.Context(),
			"stack %q publishes host port %d on vm %q, but the ports SC opened on that machine are %s — so the "+
				"stack will answer on the machine and be unreachable from outside. Add `ingressPorts: [{preset: %s}]` "+
				"to the `type: %s` resource in the parent stack and re-provision it",
			stackName, hostPort, in.VmResource, vm.FormatPortSpec(parentVm.OpenPorts),
			vm.IngressPresetVmCompose, vm.ResourceTypeVm)
	}

	// The two ways a stack can name a domain and not get it. Warned about rather than
	// refused, because the stack is genuinely deployed and genuinely answers on the
	// host port — but said out loud, because "my certificate never appeared" is
	// otherwise a silent failure with no local evidence.
	if stackConfig.Domain != "" && !servesDomain {
		switch {
		case !parentVm.IngressEnabled:
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
	if stackConfig.Domain == "" && parentVm.IngressEnabled && in.Deployment.IngressContainer != nil {
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

	// The env file is rendered ONCE for the fleet: every machine runs the same stack
	// with the same resolved values, and a per-host render would be a second place for
	// them to come from.
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

	// `scale` on this path is the fleet's size, and the fleet is declared in the
	// PARENT stack — so this is the first place both numbers are in hand. Said out
	// loud either way: a stack that asks for three replicas and gets one machine is a
	// capacity surprise, and silence about it is what the "support it or refuse it"
	// rule exists to prevent (compose-on-vm.md §2.2).
	if s := stackConfig.Scale; s != nil && s.Max > 1 {
		if s.Max == len(hosts) {
			params.Log.Info(ctx.Context(),
				"stack %q asks for %d replicas and vm %q has %d host(s): one replica of each service per machine",
				stackName, s.Max, in.VmResource, len(hosts))
		} else {
			params.Log.Warn(ctx.Context(),
				"stack %q sets `scale.max: %d`, but vm %q has %d host(s) and this path runs one replica of each "+
					"service per machine — so the stack gets %d, not %d. Add hosts to the `type: %s` resource in "+
					"the parent stack, or move the stack to a k3s/Kubernetes parent, which is what scaling WITHIN "+
					"a machine needs",
				stackName, s.Max, in.VmResource, len(hosts), len(hosts), s.Max, vm.ResourceTypeVm)
		}
	}

	if len(hosts) > 1 {
		params.Log.Info(ctx.Context(),
			"vm %q has %d hosts: stack %q is deployed onto every one of them, one machine at a time. Blue-green "+
				"is per machine — two machines' slots are independent, and a release that fails its health gate "+
				"stops at the machine it failed on with the rest of the fleet still serving the previous one",
			in.VmResource, len(hosts), stackName)
	}

	shared := composeDeploy{
		ctx: ctx, stack: stack, input: input, params: params, in: in,
		stackName: stackName, env: env, version: version, stackConfig: stackConfig,
		plan: plan, hostPort: hostPort, servesDomain: servesDomain,
		recreate: in.EffectiveStrategy() == vm.StrategyRecreate,
		envHash:  envHash, envContent: envContent,
	}

	// Rolling, not fanned out. The machines are independent and Pulumi would happily
	// deploy them at once — but a fleet exists so that a bad release does not take
	// every replica with it, and a parallel deploy hands it all of them before the
	// first health gate has answered. Ordering each host after the previous one's
	// last command means a failure stops the deploy with the untouched machines still
	// serving the version that worked.
	deploys := make([]VmComposeHostOutput, 0, len(hosts))
	var after []sdk.Resource
	for i, h := range hosts {
		out, err := deployComposeToHost(shared, h, i, after)
		if err != nil {
			return nil, err
		}
		after = []sdk.Resource{out.last}
		deploys = append(deploys, *out)
	}

	// Host 0's exports keep the names they have always had (hostResSuffix is empty at
	// index 0), so nothing reading a single-host stack's outputs breaks; the rest are
	// indexed, and the count is exported so a consumer can enumerate them without
	// guessing how far to count.
	for i, d := range deploys {
		ctx.Export(toComposeSlotExport(stackName)+hostResSuffix(i), d.Slot)
		ctx.Export(toComposeURLExport(stackName)+hostResSuffix(i), d.URL)
	}
	ctx.Export(toComposeHostCountExport(stackName), sdk.Int(len(deploys)))

	return &api.ResourceOutput{Ref: &VmComposeOutput{
		Hosts:  deploys,
		Images: plan,
		Host:   deploys[0].Host,
		Slot:   deploys[0].Slot,
		URL:    deploys[0].URL,
		Up:     deploys[0].Up,
	}}, nil
}

// composeDeploy is everything a per-host deploy needs that is NOT about the host.
//
// A struct because the alternative is a seventeen-parameter function whose arguments
// are all strings and bools, and because the split is the point: what is in here is
// decided ONCE for the whole fleet (the images, the rendered env file, whether Caddy
// fronts the stack, which host port is published), and what is passed alongside it is
// the machine. A field that turned out to be per-host would have to move out of here,
// which is exactly the review question worth having.
type composeDeploy struct {
	ctx          *sdk.Context
	stack        api.Stack
	input        api.ResourceInput
	params       pApi.ProvisionParams
	in           *vm.VmComposeInput
	stackName    string
	env          string
	version      string
	stackConfig  *api.StackConfigCompose
	plan         *imagePlan
	hostPort     int
	servesDomain bool
	recreate     bool
	envHash      string
	envContent   string
}

// deployComposeToHost runs the whole deploy against ONE machine of the fleet.
//
// `after` orders this host behind the previous one's last command; it is empty for
// the first host. The returned output carries that last command so the next host can
// be ordered behind this one.
func deployComposeToHost(
	d composeDeploy, host HostFacts, idx int, after []sdk.Resource,
) (*VmComposeHostOutput, error) {
	ctx, stack, input, params, in := d.ctx, d.stack, d.input, d.params, d.in
	stackName, env, version, stackConfig := d.stackName, d.env, d.version, d.stackConfig
	plan, hostPort, servesDomain, recreate := d.plan, d.hostPort, d.servesDomain, d.recreate
	envHash, envContent := d.envHash, d.envContent
	// The suffix every resource name on this host carries. Empty at index 0.
	sfx := hostResSuffix(idx)

	// Where this deploy's images come from, for THIS machine: the build happened once
	// for the fleet, the ship happens here.
	shipOpts, err := shipImagesTo(ctx, input, params, stack, plan, version, host, idx)
	if err != nil {
		return nil, err
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
	//
	// The chain starts at `after`, which is the previous machine's last command — so
	// the same linearity that orders the steps within a host also orders the hosts.
	prev := after
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
		cmd, err := remote.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-vmc-%s%s", stackName, name, sfx)), args,
			sdk.DependsOn(prev))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create %q command for stack %q on host %q", name, stackName, host.Name)
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

	up, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-up"+sfx), baseArgs(&remote.CommandArgs{
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
			plan.triggerInputs()...,
		),
	}), append(append(plan.Opts, shipOpts...), sdk.DependsOn(prev))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to bring stack %q up on host %q", stackName, host.Name)
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

		route, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-route"+sfx), baseArgs(&remote.CommandArgs{
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
			return nil, errors.Wrapf(err, "failed to route %q to stack %q on host %q",
				stackConfig.Domain, stackName, host.Name)
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

	finalize, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-finalize"+sfx), baseArgs(&remote.CommandArgs{
		Create: finalizeOut,
		Update: finalizeOut,
		// The content ids for the same reason as on `up`, and it matters more here:
		// this is the command that records the active slot and retires the other
		// one. An `up` that ran while this did not leaves the machine serving the new
		// slot with the state file still naming the old one.
		Triggers: append(
			sdk.Array{host.Address, sdk.String(version), finalizeOut},
			plan.triggerInputs()...,
		),
	}), sdk.DependsOn(routeDeps))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to finalize deploy of stack %q on host %q", stackName, host.Name)
	}

	// Cleanup lives on its own resource whose Create does nothing, and deliberately
	// has NO Triggers. Hanging the teardown off `up`'s Delete would be shorter and
	// is a trap: pulumi-command performs a replacement as create-then-delete, so any
	// input change that Pulumi chose to treat as a replace would tear the stack down
	// moments after bringing it up. A resource with no triggers is never replaced,
	// so its Delete runs exactly when the stack is destroyed.
	if _, err := remote.NewCommand(ctx, input.ToResName(stackName+"-vmc-teardown"+sfx), baseArgs(&remote.CommandArgs{
		Create: sdk.String("true"),
		Delete: sdk.String(teardownScript(stackName, env)),
	})); err != nil {
		return nil, errors.Wrapf(err, "failed to register teardown of stack %q on host %q", stackName, host.Name)
	}

	// The URL an operator (and `sc deploy`'s own summary) should use. Not derived from
	// whether a domain is configured but from whether it is actually SERVED — a stack
	// whose domain was ignored for one of the reasons warned about above must not
	// export an https:// URL that answers nothing.
	url := sdk.Sprintf("http://%s:%d", host.Address, hostPort)
	if servesDomain {
		url = sdk.String("https://" + stackConfig.Domain).ToStringOutput()
	}
	// Exported by the caller, which is the only place that knows this host's index
	// in the fleet.
	return &VmComposeHostOutput{
		Host: host,
		Slot: slotOut,
		URL:  url,
		Up:   up,
		// finalize and not `up`: the next machine must not start until this one has
		// recorded its active slot and retired the old one, or a failure between the
		// two would leave a machine serving a slot its state file does not name while
		// the roll moves on.
		last: finalize,
	}, nil
}

func toComposeSlotExport(stackName string) string {
	return fmt.Sprintf("%s-vm-compose-slot", stackName)
}

func toComposeURLExport(stackName string) string {
	return fmt.Sprintf("%s-vm-compose-url", stackName)
}

// toComposeHostCountExport is how many machines this stack was deployed onto.
//
// Exported so that a consumer of the slot/URL exports can enumerate them instead of
// probing `-1`, `-2`, … until one is missing — the same reason the vm resource itself
// exports a host count (ToHostCountExport).
func toComposeHostCountExport(stackName string) string {
	return fmt.Sprintf("%s-vm-compose-host-count", stackName)
}

// parentVmFacts is what a child stack learns about the MACHINE (as opposed to the
// host it connects to) from the parent stack's outputs.
//
// A struct rather than two more return values because both fields answer the same
// question — "what did the parent stack decide, that this deploy has to live with"
// — and a third bare bool in that signature is the kind of thing that gets passed
// in the wrong order.
type parentVmFacts struct {
	// IngressEnabled is whether the machine runs Caddy.
	IngressEnabled bool
	// OpenPorts is what SC opened in the machine's firewall, as exported by the
	// parent. Empty when the parent does not know — a machine whose packet
	// filtering SC does not manage, or a parent stack provisioned before the
	// export existed.
	OpenPorts []vm.PortRange
}

// hostsFromParent reads the facts of every machine of a vm resource out of the
// parent stack's outputs.
//
// The machines are described in the parent stack and deployed onto from the child, so
// this is the one seam where the two meet. Each HostFacts comes back identical in
// shape to the one the driver produced, which is what lets ConnectionArgs and
// shipScript be the same code on both sides. Alongside them come the two facts about
// the RESOURCE that this stack's deploy has to agree with: whether it declared
// `ingress.enable`, and which ports SC opened in its firewall. Both are per-resource
// rather than per-host — SC runs the same ingress and opens the same ports on every
// machine of one vm resource — which is why they are read once and not in the loop.
func hostsFromParent(
	ctx *sdk.Context,
	input api.ResourceInput,
	params pApi.ProvisionParams,
	vmResource string,
) ([]HostFacts, parentVmFacts, error) {
	r := parentReader{
		ctx:        ctx,
		params:     params,
		vmResource: vmResource,
		parentRef:  params.ParentStack.FullReference,
		suffix: lo.If(params.ParentStack.DependsOnResource != nil,
			"--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else(""),
	}

	count, err := r.hostCount()
	if err != nil {
		return nil, parentVmFacts{}, err
	}
	facts, err := r.vmFacts()
	if err != nil {
		return nil, parentVmFacts{}, err
	}

	hosts := make([]HostFacts, 0, count)
	for i := 0; i < count; i++ {
		host, err := r.host(input, i)
		if err != nil {
			return nil, parentVmFacts{}, err
		}
		hosts = append(hosts, host)
	}
	return hosts, facts, nil
}

// parentReader reads one vm resource's exports out of the parent stack.
//
// A type rather than a few closures because the reference NAMES have to be derived in
// exactly one place: every GetValueFromStack call creates its own sdk.StackReference,
// two references with the same name in one stack is an error, and the names now vary
// by host index as well as by field.
type parentReader struct {
	ctx        *sdk.Context
	params     pApi.ProvisionParams
	vmResource string
	parentRef  string
	suffix     string
}

// refName is the stack-reference name for one field of one host.
//
// Host 0 keeps the un-indexed name it has always had. A renamed StackReference would
// be harmless in itself — it reads, it owns nothing — but a stable name is what keeps
// a single-host stack's preview empty, and an empty preview is the only cheap way an
// operator can see that a release changed nothing.
func (r parentReader) refName(i int, field string) string {
	return fmt.Sprintf("%s%s-vmc%s-%s", r.vmResource, r.suffix, hostResSuffix(i), field)
}

// hostCount is how many machines the vm resource has, and the check that it exists at
// all.
func (r parentReader) hostCount() (int, error) {
	countRaw, err := pApi.GetValueFromStack[any](r.ctx, r.refName(0, "host-count"), r.parentRef,
		ToHostCountExport(r.vmResource), false)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read host count of vm %q from parent stack %q", r.vmResource, r.parentRef)
	}
	count, ok := toInt(countRaw)
	if !ok || count < 1 {
		// GetValueFromStack returns (zero, nil) for an output that is not there —
		// errors.Wrapf(nil, ...) is nil — so a missing resource arrives here as a
		// successful read of nothing, and this check is the only thing that turns it
		// into a message naming what is actually wrong.
		return 0, errors.Errorf(
			"parent stack %q has no `type: %s` resource named %q (its host-count output is %v). "+
				"Check `vmResource` in the `%s` template against the resource name in the parent's server.yaml",
			r.parentRef, vm.ResourceTypeVm, r.vmResource, countRaw, vm.TemplateTypeVmCompose,
		)
	}
	return count, nil
}

// vmFacts reads what the parent decided for the resource as a whole.
func (r parentReader) vmFacts() (parentVmFacts, error) {
	ingressRaw, err := pApi.GetValueFromStack[any](r.ctx, r.refName(0, "ingress"), r.parentRef,
		ToIngressEnabledExport(r.vmResource), false)
	if err != nil {
		return parentVmFacts{}, errors.Wrapf(err, "failed to read the ingress setting of vm %q", r.vmResource)
	}
	// Read through toBool for the same reason the ints are: an sdk.Bool export comes
	// back off a JSON checkpoint, and GetValueFromStack[bool] would panic rather than
	// fail if the decoder handed it anything else. A machine whose ingress setting is
	// unreadable is treated as not having one — this slice publishes a host port
	// either way, and the only thing the flag changes here is the wording of a
	// warning.
	ingressEnabled, _ := toBool(ingressRaw)

	// What the parent actually opened. Read as a string (see vm.FormatPortSpec for
	// why it crosses the boundary as one) and empty on an older parent stack or on a
	// machine whose firewall SC does not manage — in both cases the caller must stay
	// quiet rather than guess, since "I cannot see the firewall" and "the port is
	// shut" are different statements and only one of them is actionable.
	openPortsSpec, err := pApi.GetValueFromStack[string](r.ctx, r.refName(0, "open-ports"), r.parentRef,
		ToOpenPortsExport(r.vmResource), false)
	if err != nil {
		return parentVmFacts{}, errors.Wrapf(err, "failed to read the open ports of vm %q", r.vmResource)
	}

	return parentVmFacts{IngressEnabled: ingressEnabled, OpenPorts: vm.ParsePortSpec(openPortsSpec)}, nil
}

// host reads one machine of the fleet.
func (r parentReader) host(input api.ResourceInput, i int) (HostFacts, error) {
	address, err := pApi.GetValueFromStack[string](r.ctx, r.refName(i, "address"), r.parentRef,
		ToHostAddressExport(r.vmResource, i), false)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the address of host %d of vm %q from parent stack %q",
			i, r.vmResource, r.parentRef)
	}
	if address == "" {
		// Reachable on a fleet in a way it was not on a single machine: the host count
		// says N, so the parent provisioned N machines, and an empty address at index i
		// means its exports are incomplete rather than absent.
		return HostFacts{}, errors.Errorf(
			"host %d of vm %q in parent stack %q has an empty address; re-run `sc provision` for the parent stack",
			i, r.vmResource, r.parentRef,
		)
	}
	privateKey, err := pApi.GetValueFromStack[string](r.ctx, r.refName(i, "key"), r.parentRef,
		ToHostPrivateKeyExport(r.vmResource, i), true)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the deploy key of host %d of vm %q from parent stack %q",
			i, r.vmResource, r.parentRef)
	}
	if privateKey == "" {
		return HostFacts{}, errors.Errorf(
			"host %d of vm %q in parent stack %q exported no deploy key; re-run `sc provision` for the parent stack",
			i, r.vmResource, r.parentRef,
		)
	}
	keyPassword, err := pApi.GetValueFromStack[string](r.ctx, r.refName(i, "key-password"), r.parentRef,
		ToHostPrivateKeyPasswordExport(r.vmResource, i), true)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the deploy key passphrase of host %d of vm %q", i, r.vmResource)
	}
	// Not an error when empty, and not read back through a check: an unencrypted key
	// is the normal case and has no passphrase.
	user, err := pApi.GetValueFromStack[string](r.ctx, r.refName(i, "user"), r.parentRef,
		ToHostUserExport(r.vmResource, i), false)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the ssh user of host %d of vm %q", i, r.vmResource)
	}
	portRaw, err := pApi.GetValueFromStack[any](r.ctx, r.refName(i, "port"), r.parentRef,
		ToHostPortExport(r.vmResource, i), false)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the ssh port of host %d of vm %q", i, r.vmResource)
	}
	hostKey, err := pApi.GetValueFromStack[string](r.ctx, r.refName(i, "host-key"), r.parentRef,
		ToHostKeyExport(r.vmResource, i), false)
	if err != nil {
		return HostFacts{}, errors.Wrapf(err, "failed to read the host key of host %d of vm %q", i, r.vmResource)
	}

	port, ok := toInt(portRaw)
	if !ok || port <= 0 {
		port = vm.DefaultSshPort
	}
	if hostKey == "" {
		// The parent stack warns too, but this is the side that carries the env
		// file: an unverified connection here hands every secret of this stack to
		// whoever answers on that address.
		r.params.Log.Warn(r.ctx.Context(),
			"host %d of vm %q has no `hostKey`, so this deploy cannot verify the machine it is sending stack %q's "+
				"env file and deploy key to. Pin it in the parent stack with `ssh-keyscan -t ed25519 %s`",
			i, r.vmResource, input.StackParams.StackName, address)
	}

	return HostFacts{
		Name:               fmt.Sprintf("%s-host-%d", r.vmResource, i),
		Address:            sdk.String(address).ToStringOutput(),
		User:               lo.If(user == "", vm.DefaultSshUser).Else(user),
		Port:               port,
		PrivateKey:         secretString(privateKey),
		PrivateKeyPassword: secretString(keyPassword),
		// Lifted into an Output only to satisfy HostFacts, which carries one because a
		// cloud driver GENERATES the host key and therefore only knows it once the
		// resource exists. On this side it is already a resolved string: it came out of
		// the parent stack's exports.
		HostKey:       sdk.String(hostKey).ToStringOutput(),
		HostKeyPinned: hostKey != "",
		Role:          vm.RoleCompose,
	}, nil
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
