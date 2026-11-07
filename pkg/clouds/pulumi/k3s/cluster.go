// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package k3s provisions a Kubernetes cluster SC installs itself, over SSH, onto the
// machines of a `vm` resource. It is the Kubernetes story for every cloud that has no
// managed Kubernetes — most of the Russian market — and it is deliberately thin: the
// only thing it has to get exactly right is the NAME it exports the kubeconfig under,
// because that is what makes the entire existing layer above kubeconfig
// (`kubernetes-cloudrun`, the Caddy edge, the helm operators) work against it
// unchanged.
//
// Design: forge/docs/design/2026-10-13/sc-russia-market-readiness/k3s-all-clouds.md,
// roadmap item sc-k3s-cluster-resource (slice K.0).
package k3s

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// K3sOutput is what a provisioned k3s-cluster hands to anything in the same parent
// stack that depends on it.
type K3sOutput struct {
	// Kubeconfig is the admin kubeconfig, pointed at Endpoint and marked secret.
	Kubeconfig sdk.StringOutput
	// Endpoint is the host the API server is reached on.
	Endpoint sdk.StringOutput
	// Ready is the gate: it succeeds only once the cluster's own API reports a Ready
	// node. Anything that applies manifests must DependsOn it.
	Ready *remote.Command
}

// K3sCluster stands up the control plane on the hosts of the vm resource it names.
func K3sCluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	cfg, ok := input.Descriptor.Config.Config.(*k3s.K3sClusterConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to k3s.K3sClusterConfig for %q (given %T)",
			input.Descriptor.Name, input.Descriptor.Config.Config)
	}
	// Validated again rather than trusted: Provider is the normal path in, but this
	// ProvisionFunc is reachable on its own and validation is a pure check.
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid %s resource %q in stack %q",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, stack.Name)
	}

	vmOut, err := hostsOf(cfg, input, stack, params)
	if err != nil {
		return nil, err
	}
	roles, err := rolesOf(cfg, input, vmOut)
	if err != nil {
		return nil, err
	}
	// Host 0 is always a server, and the FIRST one: it is where SC installs before
	// anything else, where it reads the join token, and where it reads the
	// kubeconfig. rolesOf guarantees it (a byo pool that declares `k3s-agent` on its
	// first host is refused there, with the reason).
	host := vmOut.Hosts[0]

	// The cluster name, and therefore the export name, is derived exactly as
	// gcp-gke-autopilot derives it (gke_autopilot.go:68). The template side resolves
	// the same string from the resource NAME it was given
	// (gke_autopilot_stack.go:76), which is what makes the two halves meet without
	// either knowing the other exists.
	clusterName := pulumiKubernetes.ToClusterName(input, input.Descriptor.Name)

	// Endpoint defaults to the node's own address, which is only knowable as an
	// Output: a cloud driver learns it when the instance is created. Everything
	// derived from it — the SAN list, the kubeconfig URL, and so the scripts
	// themselves — is therefore an Output too.
	endpoint := host.Address.ApplyT(func(addr string) string {
		return lo.If(cfg.Endpoint == "", addr).Else(cfg.Endpoint)
	}).(sdk.StringOutput)

	disable := disableSet(cfg, vmOut)
	if err := validateIngressPorts(cfg, input, vmOut, disable); err != nil {
		return nil, err
	}
	if vmOut.IngressEnabled && lo.Contains(disable, componentServiceLB) {
		// Said once, here, because it is a composition an operator can reasonably
		// want and will not otherwise understand the consequences of: the machine's
		// own Caddy (installed by the vm resource) holds ports 80 and 443 for the
		// compose stacks on it, so klipper-lb cannot have them.
		params.Log.Info(ctx.Context(),
			"%s %q runs on a host whose compose ingress owns ports 80/443, so k3s is installed without "+
				"servicelb: a `type: LoadBalancer` Service in this cluster will stay pending. Deploy HTTP "+
				"workloads onto this cluster only if the vm resource %q sets `ingress.enable: false`",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource)
	}

	version := cfg.EffectiveVersion()
	installURL := cfg.EffectiveInstallScriptURL()
	serverCount := lo.CountBy(roles, func(r vm.HostRole) bool { return r == vm.RoleK3sServer })
	params.Log.Info(ctx.Context(),
		"configure k3s %s cluster %q on %d host(s) of vm %q: %d server(s), %d agent(s)",
		version, clusterName, len(vmOut.Hosts), cfg.VmResource, serverCount, len(vmOut.Hosts)-serverCount)

	// The SAN list covers the endpoint and every SERVER's own address — and always:
	// an operator who points `endpoint` at a DNS name still reaches the API server by
	// IP from the console, and a SAN added later costs a certificate re-issue and an
	// API server restart (k3s-all-clouds.md §4).
	//
	// Agents are deliberately NOT in it. They serve no API, so their addresses would
	// buy nothing — and they would cost the one thing worth protecting here: the SAN
	// list is part of every server's argument vector, so adding a fourth agent to a
	// working cluster would change the args marker on all three control-plane nodes
	// and reinstall them. Scaling the workers must not restart the control plane.
	sanInputs := []any{endpoint}
	for i, h := range vmOut.Hosts {
		if roles[i] == vm.RoleK3sServer {
			sanInputs = append(sanInputs, h.Address)
		}
	}
	sans := sdk.All(sanInputs...).ApplyT(func(args []any) []string {
		return lo.Uniq(append(lo.Map(args, func(a any, _ int) string { return a.(string) }), cfg.TlsSans...))
	}).(sdk.StringArrayOutput)

	// Where every other node registers. Host 0's own address and not `endpoint`: the
	// endpoint may be a name that does not resolve yet (the registrar writes it in
	// the same provision) or a load balancer whose health checks only pass once the
	// nodes it balances are up — either way a join that waits for it deadlocks.
	joinURL := host.Address.ApplyT(func(addr string) string {
		return fmt.Sprintf("https://%s:%d", addr, k3s.APIPort)
	}).(sdk.StringOutput)

	installScript := sans.ApplyT(func(sans []string) string {
		// `--cluster-init` only when this cluster has more than one server: embedded
		// etcd on a single node is a strictly worse sqlite (see JoinSpec).
		args := ServerArgs(cfg, sans, disable, JoinSpec{ClusterInit: serverCount > 1})
		return InstallScript(version, installURL, args, ServerUnit, "")
	}).(sdk.StringOutput)

	conn := pulumiVm.ConnectionArgs(host)

	// NOTE on the resource name: host 0's install keeps the un-suffixed name it has
	// had since the single-node slice, and the joining nodes take `-install-<i>`.
	// That is what makes this change additive for an existing single-node stack —
	// the URN of the one command it already has is byte-identical, so Pulumi updates
	// it rather than replacing a control plane.
	install, err := remote.NewCommand(ctx, input.ToResName(input.Descriptor.Name+"-install"), &remote.CommandArgs{
		Connection: conn,
		Create:     installScript,
		Update:     installScript,
		// No Delete here — see uninstallOnDestroy, and TriggersForGuardedChange's
		// third precondition: this command is replaced on every deploy by its own
		// nonce, and pulumi-command performs a replacement as create-then-delete, so
		// a Delete on this resource would uninstall the control plane seconds after
		// installing it.
		//
		// Without Triggers the command does not re-run once it has succeeded, so a
		// changed version or SAN list would be reported green having done nothing.
		// The script text covers every input that changes what gets installed,
		// because it is rendered from all of them.
		//
		// The per-run nonce on top of that, because InstallScript's first act is to
		// ask the machine whether it is already running this version with these
		// arguments and exit 0 if it is (the ArgsMarkerPath guard) — the
		// guarded-change case in pkg/clouds/pulumi/vm/nonce.go. What it buys is the
		// one deploy that matters: a node whose k3s died, was stopped, or was
		// reinstalled by hand has drifted without any input here moving, so a
		// change-only trigger set skips the install and leaves the node-ready read
		// below to fail the provision with no way to repair it but editing the
		// config. Live-caught 2026-10-18, against a node stopped out of band: install
		// skipped, "no k3s node became Ready within 300s".
		Triggers:               pulumiVm.TriggersForGuardedChange(installScript, host.Address),
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn([]sdk.Resource{vmOut.Ready[0]}))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install k3s on host %q for cluster %q", host.Name, clusterName)
	}

	if err := uninstallOnDestroy(ctx, input, host, ServerUnit); err != nil {
		return nil, err
	}

	joined, err := joinNodes(ctx, input, cfg, params, joinArgs{
		clusterName: clusterName,
		vmOut:       vmOut,
		roles:       roles,
		disable:     disable,
		sans:        sans,
		joinURL:     joinURL,
		version:     version,
		installURL:  installURL,
		firstServer: install,
		firstScript: installScript,
	})
	if err != nil {
		return nil, err
	}

	// Counts the WHOLE fleet, so the gate is not satisfied by host 0 alone. See
	// NodeReadyScript.
	readyScript := NodeReadyScript(len(vmOut.Hosts))
	ready, err := remote.NewCommand(ctx, input.ToResName(input.Descriptor.Name+"-node-ready"), &remote.CommandArgs{
		Connection: conn,
		Create:     sdk.String(readyScript),
		Update:     sdk.String(readyScript),
		// A read, so it takes the per-run nonce: the install script and this script
		// are both invariant between two deploys that change nothing, which made
		// "the node is Ready" an answer out of the checkpoint from the second deploy
		// of a cluster onwards — including for the deploys that go on to schedule
		// workloads on it (pkg/clouds/pulumi/vm/nonce.go).
		Triggers:               pulumiVm.TriggersForRead(installScript, sdk.String(readyScript)),
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn(append([]sdk.Resource{install}, joined...)))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create node readiness check for cluster %q", clusterName)
	}

	kubeconfigScript := endpoint.ApplyT(KubeconfigScript).(sdk.StringOutput)
	kubeconfig, err := remote.NewCommand(
		ctx, input.ToResName(input.Descriptor.Name+"-kubeconfig"), &remote.CommandArgs{
			Connection: conn,
			Create:     kubeconfigScript,
			Update:     kubeconfigScript,
			// A read, and the one whose staleness is worst: this command's stdout IS the
			// exported kubeconfig. With an invariant trigger set it ran once, so every
			// later deploy exported the credential captured at cluster creation — and a
			// k3s whose certificates were rotated (the installer does it at expiry) or
			// which was re-installed hands every downstream `kubernetes-*` stack a
			// kubeconfig that no longer authenticates, from a deploy that reported
			// success. Re-reading costs one SSH round-trip; an unchanged file yields an
			// unchanged output, so nothing downstream churns for it.
			Triggers:               pulumiVm.TriggersForRead(installScript, kubeconfigScript),
			Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
			AddPreviousOutputInEnv: sdk.Bool(false),
		},
		// Ordered after readiness, not after the install: the kubeconfig is written
		// the moment the server process starts, so reading it earlier would export a
		// credential for a cluster that cannot yet schedule anything. This way the
		// existence of the export implies a Ready node.
		sdk.DependsOn([]sdk.Resource{ready}),
		// The stdout of THIS command is the cluster's admin credential, and
		// pulumi-command does not mark stdout secret. Without this the kubeconfig —
		// client certificate and key included — is stored in plaintext in the Pulumi
		// checkpoint and printed in the update summary that GitHub Actions archives,
		// which is precisely how a Yandex service-account key reached a
		// world-readable log on 2026-09-30 (forge
		// docs/roadmap/items/yc-provider-key-in-deploy-log.md). Logging: LoggingNone
		// above covers the command's own log line; this covers the resource property.
		// The export is wrapped as well — two different places, both required.
		sdk.AdditionalSecretOutputs([]string{"stdout"}),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read kubeconfig of cluster %q", clusterName)
	}

	kubeconfigValue := pulumiKubernetes.SecretKubeconfigOutput(kubeconfig.Stdout)

	// THE line this whole slice exists for. The name must match
	// gcp-gke-autopilot's byte for byte (hence the shared helper), because that is
	// what every consumer of a cluster already reads — get it right and
	// `kubernetes-cloudrun`, the Caddy edge and the helm operators run on k3s with no
	// changes at all.
	ctx.Export(pulumiKubernetes.ToKubeconfigExport(clusterName), kubeconfigValue)

	// The other half of the same contract, and the one a child stack only discovers it
	// needed once a workload declares a `domain`: both KubeRun (kube_run.go:181) and
	// GkeAutopilotStack (gke_autopilot_stack.go:206) read ToIngressIpExport to decide
	// what the DNS A record points at. On a managed cluster that is a reserved
	// load-balancer address; here the node *is* the edge, so it is the machine's own
	// address — specifically host.Address and not `endpoint`, because an operator who
	// sets `endpoint` to a DNS name would otherwise have SC write that name into the
	// value of an A record.
	//
	// Exported unconditionally. A cluster with no domain-bearing workload today gets
	// one tomorrow, and the cost of the export missing is paid in the CHILD stack's
	// deploy, a repo away from this line.
	ctx.Export(pulumiKubernetes.ToIngressIpExport(clusterName), host.Address)

	return &api.ResourceOutput{Ref: &K3sOutput{
		Kubeconfig: kubeconfigValue,
		Endpoint:   endpoint,
		Ready:      ready,
	}}, nil
}

// rolesOf decides which host runs what, and is the one place that decision is made.
//
// Two sources, and which one wins is not a preference:
//
//   - The vm resource's own `hosts[].role`, when it declares any. A byo pool is
//     heterogeneous by definition — the operator knows which box has the disk, the
//     memory and the stable address — and SC has no way to learn that. A `servers:`
//     that disagrees with it is refused rather than silently losing, because either
//     outcome of a silent resolution is a machine in a role its owner did not pick.
//   - Otherwise `servers:`, assigned BY INDEX: hosts 0..servers-1 are the control
//     plane, the rest are agents. Deterministic and index-stable, so raising the
//     agent count never converts an existing agent into an etcd member — a change
//     that is not undone by putting the number back.
//
// The vm driver resolves an undeclared role to vm.RoleCompose (VmHost.EffectiveRole),
// so "no k3s role anywhere" is the signal that nothing was declared. That also means
// a pool explicitly marked `role: compose` and then handed to a k3s cluster is read
// as undeclared and takes the index assignment; the config tier refuses mixing the
// two role families, so the only thing lost is the chance to object to an all-compose
// pool being used as a cluster, which is a stack that works.
func rolesOf(cfg *k3s.K3sClusterConfig, input api.ResourceInput, vmOut *pulumiVm.VmOutput) ([]vm.HostRole, error) {
	declared := lo.Map(vmOut.Hosts, func(h pulumiVm.HostFacts, _ int) vm.HostRole { return h.Role })
	if lo.SomeBy(declared, func(r vm.HostRole) bool { return r.IsK3s() }) {
		servers := lo.CountBy(declared, func(r vm.HostRole) bool { return r == vm.RoleK3sServer })
		if cfg.Servers != 0 && cfg.Servers != servers {
			return nil, errors.Errorf(
				"%s %q sets `servers: %d`, but vm resource %q declares %d host(s) with `role: %s`. Remove one of "+
					"the two: on a pool whose hosts name their own roles, the roles are the operator's choice and "+
					"this field can only contradict it",
				k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.Servers, cfg.VmResource, servers,
				vm.RoleK3sServer,
			)
		}
		if declared[0] != vm.RoleK3sServer {
			// Not an arbitrary convention: SC installs on host 0 first, reads the join
			// token there and reads the kubeconfig there, so an agent in that slot
			// would have to be joined to a cluster that does not exist yet.
			return nil, errors.Errorf(
				"%s %q runs on vm resource %q whose first host declares `role: %s`; the FIRST host must be a %q — "+
					"it is the node SC installs first, takes the join token from and exports the kubeconfig of. "+
					"Reorder `hosts` so a server comes first",
				k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, declared[0], vm.RoleK3sServer,
			)
		}
		return declared, nil
	}

	servers := cfg.EffectiveServers()
	if servers > len(vmOut.Hosts) {
		return nil, errors.Errorf(
			"%s %q sets `servers: %d`, but vm resource %q has %d host(s): every server needs a machine of its "+
				"own. Either add hosts to %q or lower `servers`",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, servers, cfg.VmResource, len(vmOut.Hosts),
			cfg.VmResource,
		)
	}
	return lo.Map(vmOut.Hosts, func(_ pulumiVm.HostFacts, i int) vm.HostRole {
		return lo.If(i < servers, vm.RoleK3sServer).Else(vm.RoleK3sAgent)
	}), nil
}

// joinArgs is what every node after the first needs in order to join. A struct
// because it is nine values that are all computed once for the cluster, and a
// nine-parameter function is a call site nobody can read.
type joinArgs struct {
	clusterName string
	vmOut       *pulumiVm.VmOutput
	roles       []vm.HostRole
	disable     []string
	sans        sdk.StringArrayOutput
	joinURL     sdk.StringOutput
	version     string
	installURL  string
	// firstServer and firstScript are host 0's install command and the script it
	// ran: the dependency every join is ordered after, and the trigger input that
	// makes a join re-run when the control plane itself was reinstalled.
	firstServer *remote.Command
	firstScript sdk.StringOutput
}

// joinNodes installs k3s on every host but the first, and returns the commands so the
// readiness gate can wait on all of them.
//
// Nothing happens here for a single-host cluster — not even the token read. That keeps
// a single-node stack's resource set exactly what it was before this slice, which is
// what makes the change additive for one, and saves an SSH round-trip per deploy that
// could not be used for anything.
func joinNodes(
	ctx *sdk.Context, input api.ResourceInput, cfg *k3s.K3sClusterConfig, params pApi.ProvisionParams, a joinArgs,
) ([]sdk.Resource, error) {
	if len(a.vmOut.Hosts) < 2 {
		return nil, nil
	}

	token, err := readJoinToken(ctx, input, a)
	if err != nil {
		return nil, err
	}

	created := make([]sdk.Resource, 0, len(a.vmOut.Hosts)-1)
	// Servers are chained — server i waits for server i-1 — rather than installed in
	// parallel. Each one adds itself as an etcd member, and etcd serialises member
	// changes anyway: two joins racing means one of them gets a rejected proposal and
	// fails its provision, on a cluster that is otherwise fine.
	previous := sdk.Resource(a.firstServer)
	for i, role := range a.roles {
		if i == 0 || role != vm.RoleK3sServer {
			continue
		}
		cmd, err := installNode(ctx, input, cfg, a, i, token, []sdk.Resource{a.vmOut.Ready[i], previous})
		if err != nil {
			return nil, err
		}
		previous = cmd
		created = append(created, cmd)
	}
	// Agents, in parallel with each other: an agent registers with the API server and
	// touches no datastore state that another agent could conflict with. They wait on
	// the LAST server, so an agent never registers against a control plane that is
	// still gaining members.
	lastServer := previous
	for i, role := range a.roles {
		if i == 0 || role != vm.RoleK3sAgent {
			continue
		}
		cmd, err := installNode(ctx, input, cfg, a, i, token, []sdk.Resource{a.vmOut.Ready[i], lastServer})
		if err != nil {
			return nil, err
		}
		created = append(created, cmd)
	}

	params.Log.Info(ctx.Context(), "cluster %q joins %d host(s) to the control plane on host %q",
		a.clusterName, len(created), a.vmOut.Hosts[0].Name)
	return created, nil
}

// readJoinToken reads the cluster's join secret off host 0, once the server is up.
func readJoinToken(ctx *sdk.Context, input api.ResourceInput, a joinArgs) (sdk.StringOutput, error) {
	script := TokenReadScript()
	cmd, err := remote.NewCommand(
		ctx, input.ToResName(input.Descriptor.Name+"-join-token"), &remote.CommandArgs{
			Connection: pulumiVm.ConnectionArgs(a.vmOut.Hosts[0]),
			Create:     sdk.String(script),
			Update:     sdk.String(script),
			// A read, so it takes the per-run nonce. Without it the token is answered out
			// of the checkpoint forever, and a control plane that was reinstalled — which
			// mints a new one — would have every join authenticate with the old value.
			Triggers:               pulumiVm.TriggersForRead(a.firstScript, sdk.String(script)),
			Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
			AddPreviousOutputInEnv: sdk.Bool(false),
		},
		sdk.DependsOn([]sdk.Resource{a.firstServer}),
		// This stdout admits a machine to the control plane, and pulumi-command does
		// not mark stdout secret. Same two protections as the kubeconfig read below,
		// for the same reason — see the comment there, and
		// forge docs/roadmap/items/yc-provider-key-in-deploy-log.md.
		sdk.AdditionalSecretOutputs([]string{"stdout"}),
	)
	if err != nil {
		return sdk.StringOutput{}, errors.Wrapf(err,
			"failed to read the k3s join token from host %q for cluster %q", a.vmOut.Hosts[0].Name, a.clusterName)
	}
	// Marked again at the value level: AdditionalSecretOutputs covers the property in
	// the checkpoint, this covers everything derived from it — including the install
	// scripts of every other node, which embed it.
	return sdk.ToSecret(cmd.Stdout).(sdk.StringOutput), nil
}

// installNode installs k3s on host i, as the role rolesOf gave it.
func installNode(
	ctx *sdk.Context, input api.ResourceInput, cfg *k3s.K3sClusterConfig, a joinArgs,
	i int, token sdk.StringOutput, deps []sdk.Resource,
) (*remote.Command, error) {
	host := a.vmOut.Hosts[i]
	role := a.roles[i]
	unit := UnitFor(role)

	// The argument vector is rendered WITHOUT the token (see ServerArgs) and is
	// therefore safe to use as a trigger: the token is in the script, and a trigger
	// set holding it would put a cluster credential in every preview diff.
	nodeArgs := sdk.All(a.sans, a.joinURL).ApplyT(func(args []any) string {
		sans := args[0].([]string)
		join := JoinSpec{ServerURL: args[1].(string)}
		if role == vm.RoleK3sAgent {
			return AgentArgs(join)
		}
		return ServerArgs(cfg, sans, a.disable, join)
	}).(sdk.StringOutput)

	script := sdk.All(nodeArgs, token).ApplyT(func(args []any) string {
		return InstallScript(a.version, a.installURL, args[0].(string), unit, strings.TrimSpace(args[1].(string)))
	}).(sdk.StringOutput)

	cmd, err := remote.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-install-%d", input.Descriptor.Name, i)),
		&remote.CommandArgs{
			Connection: pulumiVm.ConnectionArgs(host),
			Create:     script,
			Update:     script,
			// Guarded (the install's own args-marker check) and nonce-triggered for
			// the same reason host 0's is: a node whose k3s was stopped out of band
			// has drifted with no input here moving, and the readiness gate would
			// then fail a provision that had no way to repair it. Deliberately NOT
			// triggered on `script`, which holds the token — `nodeArgs` plus the
			// control plane's own script covers every input that changes what gets
			// installed, and a token rotation already reinstalls the control plane
			// this depends on.
			Triggers:               pulumiVm.TriggersForGuardedChange(nodeArgs, a.firstScript, host.Address),
			Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
			AddPreviousOutputInEnv: sdk.Bool(false),
		}, sdk.DependsOn(deps))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to join host %q as %s to cluster %q", host.Name, role, a.clusterName)
	}
	if err := uninstallOnDestroy(ctx, input, host, unit); err != nil {
		return nil, err
	}
	return cmd, nil
}

// uninstallOnDestroy registers the one thing that removes k3s from a machine when the
// cluster resource goes away.
//
// Its own resource with NO triggers, which is not a style choice: the install commands
// are replaced on every deploy by their per-run nonce, and pulumi-command performs a
// replacement as create-then-delete — so an uninstall living on an install would run
// moments after it, tearing down what it had just built. That exact fault was live on
// the vm ingress until 2026-10-18 (see TriggersForGuardedChange's third precondition
// and pkg/clouds/pulumi/vm/caddy.go's teardown). A resource with no triggers is never
// replaced, so this Delete runs on destroy and at no other time.
//
// Without it a destroyed cluster leaves k3s running on every machine: holding ports
// 80/443 against whatever is deployed there next, and — for the agents — retrying
// forever against a control plane that no longer answers.
func uninstallOnDestroy(ctx *sdk.Context, input api.ResourceInput, host pulumiVm.HostFacts, unit string) error {
	if _, err := remote.NewCommand(ctx, input.ToResName(host.Name+"-k3s-uninstall"), &remote.CommandArgs{
		Connection:             pulumiVm.ConnectionArgs(host),
		Create:                 sdk.String("true"),
		Delete:                 sdk.String(UninstallScript(unit)),
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}); err != nil {
		return errors.Wrapf(err, "failed to register the k3s uninstall of host %q", host.Name)
	}
	return nil
}

// componentServiceLB is k3s's bundled load-balancer controller (klipper-lb). It binds
// the node's ports 80 and 443 for every `type: LoadBalancer` Service, which is what
// makes a single-node cluster reachable without a cloud load balancer — and what
// makes it incompatible with a machine that is also running SC's compose ingress.
const componentServiceLB = "servicelb"

// componentTraefik is k3s's bundled ingress controller. SC never uses it: the edge on
// every Kubernetes path SC manages is its own Caddy deployment
// (pkg/clouds/pulumi/kubernetes/caddy.go), and leaving traefik running would put a
// second ingress controller on the same ports with no one owning the result.
const componentTraefik = "traefik"

// disableSet decides which packaged components are turned off.
//
// An explicitly configured `disable` wins, including an explicit empty list — that is
// the escape hatch for an operator who wants traefik after all, and distinguishing
// "unset" (nil) from "empty" (set to nothing) is the only way to offer it.
func disableSet(cfg *k3s.K3sClusterConfig, vmOut *pulumiVm.VmOutput) []string {
	if cfg.Disable != nil {
		return cfg.Disable
	}
	disable := []string{componentTraefik}
	if vmOut.IngressEnabled {
		disable = append(disable, componentServiceLB)
	}
	return disable
}

// validateIngressPorts refuses a cluster whose k3s would fight the machine's own
// Caddy for ports 80 and 443.
//
// The default `disable` set already avoids this (disableSet drops traefik always and
// servicelb whenever the vm resource has an ingress), so the only way to reach this
// error is an explicit `disable:` that puts one of them back on an ingressed host —
// which an operator does on purpose, usually without knowing the machine already has
// something on :80.
//
// A refusal rather than a warning, and that is the point of roadmap item
// sc-vm-k3s-port80-race-not-validated: the two components bind the same port and
// race, so the symptom is whichever lost reporting a bind failure in a log nobody is
// reading, or — worse — traffic reaching the wrong backend from a provision that
// exited 0. On one host that is a confusing afternoon; on a fleet it is a confusing
// afternoon that only reproduces on some of the nodes.
func validateIngressPorts(
	cfg *k3s.K3sClusterConfig, input api.ResourceInput, vmOut *pulumiVm.VmOutput, disable []string,
) error {
	if !vmOut.IngressEnabled {
		return nil
	}
	conflicting := lo.Filter([]string{componentTraefik, componentServiceLB}, func(c string, _ int) bool {
		return !lo.Contains(disable, c)
	})
	if len(conflicting) == 0 {
		return nil
	}
	return errors.Errorf(
		"%s %q runs on vm resource %q, whose Caddy ingress owns ports 80 and 443 on every host — but this "+
			"cluster leaves %s enabled, which binds the same ports. They do not share: whichever starts "+
			"second fails to bind, and SC cannot tell you which that was. Either add %s to the cluster's "+
			"`disable` list, or set `ingress.enable: false` on %q and let k3s own the edge",
		k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, strings.Join(conflicting, " and "),
		strings.Join(conflicting, " and "), cfg.VmResource,
	)
}

// hostsOf resolves the vm resource this cluster runs on out of the outputs of the
// resources provisioned before it.
//
// The map is populated by provision.go because K3sClusterConfig implements
// api.WithParentDependencies (pkg/clouds/pulumi/provision.go:145-172). That is the
// whole reason a cluster can learn its nodes' addresses and readiness gates without a
// StackReference into the stack that is being provisioned right now — which would not
// be slow, it would be a cycle.
func hostsOf(cfg *k3s.K3sClusterConfig, input api.ResourceInput, stack api.Stack, params pApi.ProvisionParams) (*pulumiVm.VmOutput, error) {
	out, found := params.ResourceOutputs[cfg.VmResource]
	if !found || out == nil {
		return nil, errors.Errorf(
			"%s %q references `vmResource: %s`, which is not a resource of stack %q in env %q. It must be a "+
				"`type: vm` resource in the SAME parent stack and environment — a cluster cannot depend on a "+
				"machine described somewhere else",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, stack.Name,
			input.StackParams.Environment,
		)
	}
	vmOut, ok := out.Ref.(*pulumiVm.VmOutput)
	if !ok {
		return nil, errors.Errorf(
			"%s %q references `vmResource: %s`, which is not a `type: %s` resource (its output is %T)",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, "vm", out.Ref,
		)
	}
	if len(vmOut.Hosts) == 0 || len(vmOut.Ready) != len(vmOut.Hosts) {
		return nil, errors.Errorf("vm resource %q produced no usable hosts for %s %q",
			cfg.VmResource, k3s.ResourceTypeK3sCluster, input.Descriptor.Name)
	}
	// Any host count from here. Which machine runs what is rolesOf's decision, not
	// this function's — it only establishes that the pool exists and that every host
	// has a readiness resource to be ordered after.
	return vmOut, nil
}
