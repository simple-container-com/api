// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
)

// The default has to stay exactly what kubernetes-cloudrun has always asked for. The
// extraction that introduced KubeRunOpts moved this lookup behind a method, and the
// cheap way to get that wrong is to "fix" the default to the spelling the exporters
// actually use (ToIngressIpExport of the cluster resource + env suffix, which is what
// caddy.go and adopt_gke_autopilot.go export and what GkeAutopilotStack reads). That
// may well be the right fix, but it changes what a live fleet looks up and needs its
// own smoke — so it is not this change, and this test is what says so.
func TestKubeRunOptsIngressIPExportDefaultsToTheParentStackName(t *testing.T) {
	RegisterTestingT(t)

	Expect(KubeRunOpts{}.ingressIPExport("my-parent")).To(Equal("my-parent-ingress-ip"))
	Expect(KubeRunOpts{}.ingressIPExport("my-parent")).To(Equal(ToIngressIpExport("my-parent")))
}

// And an explicitly-set name must win outright, with no suffixing or decoration: the
// caller passes a fully-formed output name (k3s passes
// ToIngressIpExport(<cluster>--<env>)), because the caller is the only side that knows
// how its own cluster exported it.
func TestKubeRunOptsIngressIPExportOverrideIsUsedVerbatim(t *testing.T) {
	RegisterTestingT(t)

	opts := KubeRunOpts{IngressIPExport: "ru-cluster--staging-ingress-ip"}
	Expect(opts.ingressIPExport("my-parent")).To(Equal("ru-cluster--staging-ingress-ip"))
}
