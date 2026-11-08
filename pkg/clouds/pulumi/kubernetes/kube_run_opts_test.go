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

// ToIngressIpsExport must NOT collide with ToIngressIpExport, and the difference has to
// be more than a plural: both outputs live on the same parent stack at the same time,
// and a reader asking for the wrong one gets either a comma-joined list written into an
// A record's value or a single node of a fleet.
func TestIngressIpsExportIsADistinctOutput(t *testing.T) {
	RegisterTestingT(t)

	Expect(ToIngressIpsExport("ru-cluster--staging")).To(Equal("ru-cluster--staging-ingress-ips"))
	Expect(ToIngressIpsExport("ru-cluster--staging")).ToNot(Equal(ToIngressIpExport("ru-cluster--staging")))
}

// The switch between one A record and N. Empty means the single-address path, so every
// way the parent's output can be useless has to arrive here as empty — including the
// missing output, which is "" and which strings.Split turns into one EMPTY address
// rather than none.
func TestFleetAddresses(t *testing.T) {
	RegisterTestingT(t)

	for name, tc := range map[string]struct {
		joined string
		want   []string
	}{
		"missing output":    {"", []string{}},
		"single address":    {"203.0.113.10", []string{"203.0.113.10"}},
		"three addresses":   {"203.0.113.10,203.0.113.11,203.0.113.12", []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"}},
		"trailing sep":      {"203.0.113.10,203.0.113.11,", []string{"203.0.113.10", "203.0.113.11"}},
		"unresolved middle": {"203.0.113.10,,203.0.113.12", []string{"203.0.113.10", "203.0.113.12"}},
		"separators only":   {",,", []string{}},
		"padded":            {" 203.0.113.10 , 203.0.113.11 ", []string{"203.0.113.10", "203.0.113.11"}},
	} {
		t.Run(name, func(t *testing.T) {
			RegisterTestingT(t)
			// Equal and not ConsistOf: order is the host order of the fleet, and
			// len() == 0 is what the caller reads as "fall back to one record".
			Expect(fleetAddresses(tc.joined)).To(Equal(tc.want))
		})
	}
}

// And the option itself defaults off: every flavour but k3s leaves it empty, which is
// the single-record behaviour KubeRun has always had.
func TestKubeRunOptsIngressIPsExportIsOptional(t *testing.T) {
	RegisterTestingT(t)

	Expect(KubeRunOpts{}.IngressIPsExport).To(BeEmpty())
	Expect(KubeRunOpts{IngressIPExport: "ru-cluster--staging-ingress-ip"}.IngressIPsExport).To(BeEmpty(),
		"setting the single-address override must not opt a flavour into the fleet path")
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
