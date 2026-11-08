// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"strings"

	"github.com/pkg/errors"
)

// ResourceRef is a reference from one parent-stack resource's config field to an
// output of a SIBLING resource — a resource of the same server.yaml, the same
// stack and the same environment.
//
// Why this exists. A `k3s-cluster` produces the cluster's kubeconfig, and a
// `kubernetes-caddy` needs a kubeconfig to deploy the edge into that cluster, but
// the field took a literal string only. So the single most obvious server.yaml
// anyone would write — a cluster and its front door, side by side — could not be
// provisioned in one pass. The documented workaround was to provision twice and
// carry the value across by hand:
//
//  1. provision with the Caddy resource absent, read the kubeconfig from the
//     stack outputs,
//  2. paste it into `secrets.yaml` and provision again with
//     `kubeconfig: ${secret:K3S_KUBECONFIG}`.
//
// That works, and it is what all three k3s smokes of the 2026-11-08 fleet stand
// did. As a documented path it is poor in three separate ways: it makes a
// one-command provision a two-command one, it puts a cluster-admin credential in
// the operator's hands and in `secrets.yaml` purely as plumbing, and the pasted
// copy can go stale against the cluster with nothing to notice — a control-plane
// rebuild or a `tlsSans` change reissues the certificate, and the next deploy
// fails with an x509 error pointing at the Caddy resource rather than at the stale
// string. An output only a human can carry between two resources is a missing edge
// in the dependency graph, not a configuration style.
//
// See forge/docs/roadmap/items/sc-k3s-caddy-needs-literal-kubeconfig.md.
type ResourceRef struct {
	// ResourceName is the sibling resource, named exactly as the `resources:`
	// block of server.yaml names it.
	ResourceName string
	// Key is which of that resource's outputs to read (e.g. `kubeconfig`). The
	// set of valid keys belongs to the resource that publishes them, not here.
	Key string
}

// ResourceRefPrefix is the placeholder context this reference uses.
//
// It is deliberately the same `${resource:<name>.<key>}` spelling a CLIENT stack
// already uses to read a provisioned parent resource's outputs (see
// pkg/clouds/pulumi/api/compute_context.go) — one syntax, one meaning, two tiers.
// The RESOLUTION is necessarily different: a client-stack placeholder is
// substituted as a string before anything runs, while here the value does not
// exist until the sibling has been provisioned in this very update, so it can only
// ever be a Pulumi Output.
//
// That is also why the reference survives placeholder resolution untouched.
// `pkg/provisioner/placeholders` registers no `resource` extension, and
// pkg/template's calcValue returns an unknown context verbatim rather than
// erroring (strict mode is off on the provision path) — so the literal string
// arrives at the provisioning tier for ParseResourceRef to read. Registering a
// `resource` extension in that package would silently break every reference here.
const ResourceRefPrefix = "${resource:"

// ParseResourceRef reads the `${resource:<name>.<key>}` form.
//
// Whole-string match, not a substring: a field is either a reference or a literal
// value. Half a kubeconfig with a reference spliced into it is not a thing anyone
// means, and treating it as one would turn a typo into a provider built from a
// corrupt credential.
//
// The boolean is "this is a reference" and the error is "it is a reference and it
// is malformed" — two different answers, because a literal value that merely
// starts with a brace must pass through untouched while `${resource:no-dot}` has
// to be a refusal rather than a string that silently reaches a provider.
func ParseResourceRef(value string) (ResourceRef, bool, error) {
	v := strings.TrimSpace(value)
	if !strings.HasPrefix(v, ResourceRefPrefix) || !strings.HasSuffix(v, "}") {
		return ResourceRef{}, false, nil
	}
	body := strings.TrimSuffix(strings.TrimPrefix(v, ResourceRefPrefix), "}")
	// SplitN(…, 2) and not Split: an output key may itself contain a dot, and the
	// resource name may not (server.yaml map keys are plain names), so the FIRST
	// dot is the boundary.
	parts := strings.SplitN(body, ".", 2)
	if len(parts) != 2 {
		return ResourceRef{}, true, errors.Errorf(
			"%q is not a valid resource reference: expected ${resource:<resource-name>.<output>}, "+
				"e.g. ${resource:my-k3s.kubeconfig}", value,
		)
	}
	name, key := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if name == "" || key == "" {
		return ResourceRef{}, true, errors.Errorf(
			"%q is not a valid resource reference: both the resource name and the output must be non-empty", value,
		)
	}
	return ResourceRef{ResourceName: name, Key: key}, true, nil
}

// String renders the reference back to the form an operator wrote, so an error
// message can quote what is in their server.yaml rather than a struct dump.
func (r ResourceRef) String() string {
	return ResourceRefPrefix + r.ResourceName + "." + r.Key + "}"
}
