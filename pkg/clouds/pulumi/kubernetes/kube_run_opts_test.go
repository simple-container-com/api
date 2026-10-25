// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// Without caddyResource the default stays exactly what kubernetes-cloudrun has always
// asked for: the bare parent stack name.
func TestKubeRunOptsIngressIPExportDefaultsToTheParentStackName(t *testing.T) {
	RegisterTestingT(t)

	Expect(KubeRunOpts{}.ingressIPExport("my-parent", "")).To(Equal("my-parent-ingress-ip"))
	Expect(KubeRunOpts{}.ingressIPExport("my-parent", "")).To(Equal(ToIngressIpExport("my-parent")))
}

// With caddyResource the child reads the address of that Caddy, under the name
// caddy.go exports it. Nothing exports the bare parent stack name, so that lookup
// only ever produced an empty IP; and a parent with two Caddies in one environment
// has one address per Caddy.
func TestKubeRunOptsIngressIPExportFollowsTheCaddyResource(t *testing.T) {
	RegisterTestingT(t)

	input := api.ResourceInput{StackParams: &api.StackParams{Environment: "work-dev", ParentEnv: "likeclaw-us"}}
	Expect(KubeRunOpts{}.ingressIPExport("root", ToClusterName(input, "caddy-std"))).To(Equal("caddy-std--likeclaw-us-ingress-ip"))
}

// And an explicitly-set name must win outright, with no suffixing or decoration: the
// caller passes a fully-formed output name (k3s passes
// ToIngressIpExport(<cluster>--<env>)), because the caller is the only side that knows
// how its own cluster exported it.
func TestKubeRunOptsIngressIPExportOverrideIsUsedVerbatim(t *testing.T) {
	RegisterTestingT(t)

	opts := KubeRunOpts{IngressIPExport: "ru-cluster--staging-ingress-ip"}
	Expect(opts.ingressIPExport("my-parent", "")).To(Equal("ru-cluster--staging-ingress-ip"))
	Expect(opts.ingressIPExport("my-parent", "caddy-std--likeclaw-us")).To(Equal("ru-cluster--staging-ingress-ip"))
}

// A cloud-compose client stack that omits `size:` leaves StackConfig.Size nil, and
// reading the ephemeral size straight through it used to panic the deploy with a bare
// SIGSEGV before any resource was touched.
func TestStackConfigFacts_NilSizeIsNotADereference(t *testing.T) {
	RegisterTestingT(t)

	domain, ephemeral := stackConfigFacts(&api.StackConfigCompose{Domain: "app.example.ru"})
	Expect(domain).To(Equal("app.example.ru"))
	Expect(ephemeral).To(BeEmpty())
}

func TestStackConfigFacts_NilConfigIsNotADereference(t *testing.T) {
	RegisterTestingT(t)

	domain, ephemeral := stackConfigFacts(nil)
	Expect(domain).To(BeEmpty())
	Expect(ephemeral).To(BeEmpty())
}

func TestStackConfigFacts_SizeIsCarriedWhenPresent(t *testing.T) {
	RegisterTestingT(t)

	domain, ephemeral := stackConfigFacts(&api.StackConfigCompose{
		Size: &api.StackConfigComposeSize{Ephemeral: "2Gi"},
	})
	Expect(domain).To(BeEmpty())
	Expect(ephemeral).To(Equal("2Gi"))
}
