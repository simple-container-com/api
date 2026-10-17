// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"fmt"
)

// ToKubeconfigExport is the stack-output name a cluster resource publishes its
// kubeconfig under, and the name every consumer of a cluster reads back.
//
// It lives here, in the cloud-independent Kubernetes package, because it is the one
// string that decides whether a NEW cluster implementation gets the whole existing
// layer above kubeconfig for free. `gcp-gke-autopilot` invented this name and three
// places in pkg/clouds/pulumi/gcp read it back (gke_autopilot_stack.go:76,
// compute_proc.go:95, adopt_gke_autopilot.go:153); pkg/clouds/pulumi/k3s exports
// under exactly the same name, so `kubernetes-cloudrun`, the Caddy edge and the helm
// operators work against a self-hosted k3s cluster with no changes at all.
//
// A second spelling of this format string is therefore not a duplication to clean up
// later — it is a cluster whose kubeconfig nothing can find, reported as a missing
// stack output in the child deploy rather than in the parent provision that caused
// it. Keep this the only definition.
func ToKubeconfigExport(clusterName string) string {
	return fmt.Sprintf("%s-kubeconfig", clusterName)
}
