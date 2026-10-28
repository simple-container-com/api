// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"sort"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k3s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// sortedKeys returns a map's keys in a stable order so SC's template-walking
// code produces the same URNs on every run. Lifted into firewall.go from the
// now-removed in-tree cloudru driver, which was the previous home.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// resolveFirewallPorts is every port SC opens on the machines of one vm resource:
// the base set (SSH, plus 80/443 when the machine runs the ingress), the presets of
// the consumers that name this resource in the same parent stack, and whatever the
// resource's own `ingressPorts` adds.
//
// The inference half exists because a vm resource cannot be asked to know what will
// be put on the machine, and the operator should not have to: the roadmap item this
// closes (sc-cloudru-vm-no-ingress-ports) was filed after a smoke in which
// `sc provision` and `sc deploy` both reported success, `curl` from the node
// answered 200, `readyReplicas: 1` — and the service was reachable from outside only
// after four security-group rules were added by hand. Nothing in any log said a port
// was shut.
//
// It is inference and not guesswork: the consumers are DECLARED, in the same parent
// stack this resource is declared in. A `vm-compose` template names its machine in
// `vmResource`, and so does a `k3s-cluster` resource — so SC is reading a reference
// that already exists, not predicting one. Everything it cannot see that way (the
// port an operator's own service binds on a bare machine, a UDP listener) is what
// `ingressPorts` is for.
func resolveFirewallPorts(
	ctx *sdk.Context,
	stack api.Stack,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	params pApi.ProvisionParams,
) ([]vm.PortRange, error) {
	name := input.Descriptor.Name
	var presets []string

	// `vm-compose` templates live in the parent stack's `templates:` section. A
	// template is shared by every stack that references it, so one match is enough:
	// the host port of each stack is derived by hashing its name into
	// HostPortRangeStart..End (vm.IngressHostPort), and which ports inside that
	// range get used is not knowable here at all — the stacks deploy later, from
	// their own repositories.
	for _, tplName := range sortedKeys(stack.Server.Templates) {
		tpl := stack.Server.Templates[tplName]
		if tpl.Type != vm.TemplateTypeVmCompose {
			continue
		}
		tplCfg, ok := tpl.Config.Config.(*vm.ComposeTemplateConfig)
		if !ok || tplCfg.VmResource != name {
			continue
		}
		params.Log.Info(ctx.Context(),
			"vm %q: opening the `%s` host-port range because template %q deploys onto this machine",
			name, vm.TemplateTypeVmCompose, tplName)
		presets = append(presets, vm.IngressPresetVmCompose)
		break
	}

	// `k3s-cluster` resources sit next to this one, in the same environment of the
	// same parent stack — which is also what K3sClusterConfig.VmResource documents.
	if perEnv, found := stack.Server.Resources.Resources[input.StackParams.Environment]; found {
		for _, resName := range sortedKeys(perEnv.Resources) {
			res := perEnv.Resources[resName]
			if res.Type != k3s.ResourceTypeK3sCluster {
				continue
			}
			clusterCfg, ok := res.Config.Config.(*k3s.K3sClusterConfig)
			if !ok || clusterCfg.VmResource != name {
				continue
			}
			params.Log.Info(ctx.Context(),
				"vm %q: opening the k3s API port and the NodePort range because `%s` resource %q runs on this "+
					"machine", name, k3s.ResourceTypeK3sCluster, resName)
			presets = append(presets, vm.IngressPresetK3s)
			break
		}
	}

	return cfg.FirewallPorts(presets...)
}
