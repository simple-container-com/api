// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

// resourceProvisionOrder orders the resources of one environment so that every
// resource is provisioned after the siblings its config names through
// api.WithParentDependencies.
//
// What this replaced, and why it had to. The loop used to split the resources
// into two buckets — "no dependencies" and "has dependencies" — provision the
// first bucket while accumulating outputs, then provision the second bucket
// WITHOUT accumulating them. Two consequences, both silent:
//
//   - a dependency between two dependency-having resources was not ordered at
//     all. Both sat in the second bucket, and Go randomises map iteration, so
//     which one ran first changed between runs of the same unchanged config.
//   - a dependency-having resource's output was discarded, so nothing could ever
//     read it. That is what made `kubernetes-caddy` unable to reference a
//     `k3s-cluster`'s kubeconfig: the cluster depends on its vm resource, so its
//     own output never reached the map a sibling reads
//     (forge/docs/roadmap/items/sc-k3s-caddy-needs-literal-kubeconfig.md).
//
// Kahn's algorithm over a stable, sorted ready set: the order is a pure function
// of the config, so an unchanged server.yaml produces the same order on every
// run. That matters beyond tidiness — Pulumi resource names are derived from
// provisioning order in a few places, and a non-deterministic order is a diff
// that appears and disappears.
//
// A dependency naming a resource that is not in this environment contributes NO
// edge rather than an error. The name may belong to another environment's block
// of the same server.yaml, or to nothing at all; either way the resource that
// needs it reports the miss itself, with the list of what WAS available (see
// pApi.ResolveResourceRef). Failing here instead would break every stack whose
// dependency is satisfied some other way — through a parent stack reference, say
// — for the sake of an error someone else already raises better.
func resourceProvisionOrder(resources map[string]api.ResourceDescriptor) ([]string, error) {
	names := make([]string, 0, len(resources))
	for name := range resources {
		names = append(names, name)
	}
	sort.Strings(names)

	// dependsOn[name] is the set of siblings `name` must follow.
	dependsOn := make(map[string]map[string]struct{}, len(names))
	// blocks[name] is the set of siblings that must follow `name`.
	blocks := make(map[string][]string, len(names))
	for _, name := range names {
		dependsOn[name] = make(map[string]struct{})
	}
	for _, name := range names {
		withDeps, ok := resources[name].Config.Config.(api.WithParentDependencies)
		if !ok {
			continue
		}
		for _, dep := range withDeps.DependsOnResources() {
			if dep.Name == "" || dep.Name == name {
				// Self-reference: not an ordering, and treating it as a cycle
				// would turn a harmless config quirk into a refusal.
				continue
			}
			if _, inEnv := resources[dep.Name]; !inEnv {
				continue
			}
			if _, already := dependsOn[name][dep.Name]; already {
				continue
			}
			dependsOn[name][dep.Name] = struct{}{}
			blocks[dep.Name] = append(blocks[dep.Name], name)
		}
	}

	ready := make([]string, 0, len(names))
	for _, name := range names {
		if len(dependsOn[name]) == 0 {
			ready = append(ready, name)
		}
	}

	order := make([]string, 0, len(names))
	for len(ready) > 0 {
		// Sorted every round, not just at the start: a resource becomes ready
		// when its last dependency is placed, and the round it joins in is not
		// the round its name would sort into.
		sort.Strings(ready)
		name := ready[0]
		ready = ready[1:]
		order = append(order, name)
		for _, blocked := range blocks[name] {
			delete(dependsOn[blocked], name)
			if len(dependsOn[blocked]) == 0 {
				ready = append(ready, blocked)
			}
		}
	}

	if len(order) != len(names) {
		return nil, errors.Errorf(
			"circular dependency between resources %s: each of them declares a dependency on another, so none can "+
				"be provisioned first. Break the cycle by giving one of them a literal value instead of a "+
				"${resource:...} reference",
			quotedList(unplaced(names, order)),
		)
	}
	return order, nil
}

// unplaced returns the names the sort could not place, sorted — the members of
// the cycle, plus anything downstream of it.
func unplaced(names, order []string) []string {
	placed := make(map[string]struct{}, len(order))
	for _, n := range order {
		placed[n] = struct{}{}
	}
	out := make([]string, 0, len(names)-len(order))
	for _, n := range names {
		if _, ok := placed[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}

func quotedList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "\""+n+"\"")
	}
	return strings.Join(quoted, ", ")
}
