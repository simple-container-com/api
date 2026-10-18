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
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiKubernetes "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
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
	params.Log.Info(ctx.Context(), "configure k3s %s cluster %q on host %q of vm %q",
		version, clusterName, host.Name, cfg.VmResource)

	installScript := sdk.All(host.Address, endpoint).ApplyT(func(args []any) string {
		addr := args[0].(string)
		ep := args[1].(string)
		// The node's own address is a SAN as well as the endpoint, and always: an
		// operator who points `endpoint` at a DNS name still reaches the API server
		// by IP from the console, and a SAN added later costs a certificate
		// re-issue and an API server restart (k3s-all-clouds.md §4).
		sans := lo.Uniq(append([]string{ep, addr}, cfg.TlsSans...))
		return InstallScript(version, installURL, ServerArgs(cfg, sans, disable))
	}).(sdk.StringOutput)

	conn := pulumiVm.ConnectionArgs(host)

	install, err := remote.NewCommand(ctx, input.ToResName(input.Descriptor.Name+"-install"), &remote.CommandArgs{
		Connection: conn,
		Create:     installScript,
		Update:     installScript,
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

	readyScript := NodeReadyScript()
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
	}, sdk.DependsOn([]sdk.Resource{install}))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create node readiness check for cluster %q", clusterName)
	}

	kubeconfigScript := endpoint.ApplyT(KubeconfigScript).(sdk.StringOutput)
	kubeconfig, err := remote.NewCommand(ctx, input.ToResName(input.Descriptor.Name+"-kubeconfig"), &remote.CommandArgs{
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
			input.StackParams.Environment)
	}
	vmOut, ok := out.Ref.(*pulumiVm.VmOutput)
	if !ok {
		return nil, errors.Errorf(
			"%s %q references `vmResource: %s`, which is not a `type: %s` resource (its output is %T)",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, "vm", out.Ref)
	}
	if len(vmOut.Hosts) == 0 || len(vmOut.Ready) != len(vmOut.Hosts) {
		return nil, errors.Errorf("vm resource %q produced no usable hosts for %s %q",
			cfg.VmResource, k3s.ResourceTypeK3sCluster, input.Descriptor.Name)
	}
	// Single-node only, and refused rather than silently partial: a multi-host pool
	// needs the server/agent split — a join token read off the first node, delivered
	// to the others over stdin, and `--server` on each agent. That is the next slice
	// of roadmap item sc-k3s-cluster-resource, and installing a second control plane
	// on every host in the meantime would be a worse outcome than this error.
	if len(vmOut.Hosts) > 1 {
		return nil, errors.Errorf(
			"%s %q runs on vm resource %q, which has %d hosts: a single-node cluster is all this slice "+
				"supports. Multi-node (server + agents, join token) is the next slice of roadmap item "+
				"sc-k3s-cluster-resource",
			k3s.ResourceTypeK3sCluster, input.Descriptor.Name, cfg.VmResource, len(vmOut.Hosts))
	}
	return vmOut, nil
}
