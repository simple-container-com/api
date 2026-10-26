// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// TestProviderConfigsRegistered pins the one type string a server.yaml may name.
// It is the whole public surface of this provider: a rename turns every existing
// parent stack into "unknown resource type".
func TestProviderConfigsRegistered(t *testing.T) {
	RegisterTestingT(t)

	Expect(api.GetRegisteredProviderConfigs()).To(HaveKey(ResourceTypeVm),
		"provider config %q must be registered", ResourceTypeVm)
}

// TestReadVmConfigAcceptsEmptyConfig guards the constraint that decided where
// validation lives. cmd/schema-gen discovers resources by driving every registered
// config reader with an EMPTY api.Config and silently skipping the readers that
// error — so a reader that validates drops `vm` out of the generated schema
// entirely, with no failure anywhere to explain it. Validation therefore runs in
// the provisioning tier, and this test is what keeps someone from "fixing" that.
func TestReadVmConfigAcceptsEmptyConfig(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadVmConfig(&api.Config{Config: make(map[string]any)})
	Expect(err).ToNot(HaveOccurred())

	cfg, ok := out.Config.(*VmConfig)
	Expect(ok).To(BeTrue())
	// And the same config does not survive the real gate.
	Expect(cfg.Validate()).To(HaveOccurred())
}

func TestReadVmConfigReadsByoHost(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadVmConfig(&api.Config{Config: map[string]any{
		"provider": DriverByo,
		"hosts": []any{map[string]any{
			"address":    "203.0.113.10",
			"user":       "ubuntu",
			"port":       2222,
			"privateKey": "-----BEGIN OPENSSH PRIVATE KEY-----",
		}},
		"ingress": map[string]any{"enable": true, "email": "ops@example.com"},
	}})
	Expect(err).ToNot(HaveOccurred())

	cfg, ok := out.Config.(*VmConfig)
	Expect(ok).To(BeTrue())
	Expect(cfg.Validate()).ToNot(HaveOccurred())
	Expect(cfg.Hosts).To(HaveLen(1))
	Expect(cfg.Hosts[0].Address).To(Equal("203.0.113.10"))
	Expect(cfg.Hosts[0].EffectiveUser(cfg.SshUser)).To(Equal("ubuntu"))
	Expect(cfg.Hosts[0].EffectivePort()).To(Equal(2222))
	Expect(cfg.IngressEnabled()).To(BeTrue())
}

// Every driver named by a const must have a Drivers row, and vice versa: the row
// is what Validate uses to tell "unknown provider" from "not implemented yet", and
// a const without a row produces the wrong one of those two messages.
//
// Scope note: this covers the drivers SC CORE ships. Plugin-advertised drivers live
// in a second table (plugin_drivers.go) precisely so that this invariant stays
// checkable — merged into one map it would start covering whatever plugins happened
// to be installed on the box running the test. The KnownDrivers assertion below is
// therefore the no-plugins-registered case, which is the state every other test in
// this package leaves behind (they defer ResetPluginDrivers).
func TestDriversCoverEveryNamedDriver(t *testing.T) {
	RegisterTestingT(t)

	named := []string{
		DriverByo, DriverYandex, DriverOpenstack, DriverVkcs,
		DriverMws, DriverCloudru, DriverCloudruEvolution, DriverTimeweb,
	}
	for _, name := range named {
		Expect(Drivers).To(HaveKey(name), "driver %q has no Drivers row", name)
	}
	Expect(Drivers).To(HaveLen(len(named)))
	Expect(KnownDrivers()).To(ConsistOf(named))
	// Sorted, because the list is interpolated into error messages and an unstable
	// order makes those messages unassertable.
	Expect(KnownDrivers()).To(Equal([]string{
		"byo", "cloudru-evolution-vm", "cloudru-vm", "mws-vm", "openstack-vm", "timeweb-vm", "vkcs-vm", "yc-vm",
	}))

	// The drivers that exist today. This list is the reminder: landing a driver means
	// filling in its capability row from the cloud's own API, and an implemented
	// driver left with the zeroed row would make shared code branch on "this cloud
	// has no private network, no public address and no firewall".
	implemented := []string{DriverByo, DriverCloudru, DriverCloudruEvolution}
	for name, info := range Drivers {
		if lo.Contains(implemented, name) {
			Expect(info.Implemented).To(BeTrue(), "driver %q is implemented but its row says otherwise", name)
			// byo is the exception and is genuinely all-false: there is no cloud
			// behind it, so SC cannot create a network, hand out an address or write
			// a firewall rule. Every cloud driver has to claim something.
			if name != DriverByo {
				Expect(info.Capabilities).ToNot(Equal(Capabilities{}),
					"driver %q is implemented with an empty capability row", name)
			}
			continue
		}
		Expect(info.Implemented).To(BeFalse(), "driver %q claims to be implemented", name)
		Expect(info.RoadmapItem).ToNot(BeEmpty(), "driver %q must name the item that lands it", name)
	}
}
