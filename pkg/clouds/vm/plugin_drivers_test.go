// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"
)

// pluginRow is a plausible registration: a cloud driver that exists, has been run,
// and claims the capabilities of a cloud with cloud-init and per-instance firewalls.
func pluginRow() DriverInfo {
	return DriverInfo{
		Implemented:  true,
		RoadmapItem:  "sc-ru-clouds-beyond-yandex",
		Capabilities: Capabilities{CloudInit: true, PrivateNetwork: true, FloatingIP: true, SecurityGroups: true},
	}
}

// A registered plugin driver has to be indistinguishable from a built-in to every
// consumer of the table — that is the whole point of Phase 1d. Lookup finds it, the
// error-message list names it, and the plugin it came from stays answerable.
func TestRegisterPluginDriver_BehavesLikeABuiltIn(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	Expect(RegisterPluginDriver("sc-plugin-yandex", "yc-vm-plugin", pluginRow())).ToNot(HaveOccurred())

	info, ok := LookupDriver("yc-vm-plugin")
	Expect(ok).To(BeTrue())
	Expect(info.Implemented).To(BeTrue())
	Expect(info.Plugin).To(Equal("sc-plugin-yandex"))
	Expect(PluginForDriver("yc-vm-plugin")).To(Equal("sc-plugin-yandex"))

	// In the list, and still sorted — the list is interpolated into error messages.
	Expect(KnownDrivers()).To(ContainElement("yc-vm-plugin"))
	Expect(KnownDrivers()).To(Equal([]string{
		"byo", "cloudru-evolution-vm", "cloudru-vm", "mws-vm", "openstack-vm",
		"timeweb-vm", "vkcs-vm", "yc-vm", "yc-vm-plugin",
	}))

	// A built-in is not attributed to a plugin.
	Expect(PluginForDriver(DriverByo)).To(BeEmpty())

	ResetPluginDrivers()
	_, ok = LookupDriver("yc-vm-plugin")
	Expect(ok).To(BeFalse())
}

// A plugin may not take over a driver name core ships AN IMPLEMENTATION FOR.
// Resolving this silently would decide which cloud a machine is created in by
// map iteration order. Uses `byo` because it is the one truly in-tree driver;
// every other row in `Drivers` is capability metadata whose implementation is
// (or will be) plugin-owned, and those legitimately coexist with their plugin
// (see TestRegisterPluginDriver_MigratedToPluginAllowsRegistration below).
func TestRegisterPluginDriver_CannotShadowABuiltIn(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	err := RegisterPluginDriver("sc-plugin-rogue", DriverByo, pluginRow())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("SC core provides itself"))

	// And the built-in row is untouched.
	info, ok := LookupDriver(DriverByo)
	Expect(ok).To(BeTrue())
	Expect(info.Plugin).To(BeEmpty())
}

// A row flagged `MigratedToPlugin: true` is NOT a true built-in and MUST allow a
// plugin to register the same name — the row exists only for its capability
// metadata; the plugin is the authoritative implementation.
//
// Live-caught 2026-10-30 on a two-peaks deploy against api v2026.10.54:
// sc-plugin-cloudru advertises `cloudru-evolution-vm`, api's then-strict
// collision check refused the registration, every cloudru deploy went red with
// `plugin %q advertises vm provider %q, which SC core provides itself`.
func TestRegisterPluginDriver_MigratedToPluginAllowsRegistration(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	// Precondition: the row exists and is flagged.
	Expect(Drivers[DriverCloudruEvolution].MigratedToPlugin).To(BeTrue(),
		"DriverCloudruEvolution must stay flagged so sc-plugin-cloudru can register it without colliding")

	// The plugin registers; no error.
	Expect(RegisterPluginDriver("cloudru", DriverCloudruEvolution, pluginRow())).ToNot(HaveOccurred())

	// LookupDriver returns the plugin's row (not the stale built-in metadata),
	// and PluginForDriver names the plugin so dispatch routes to its VmHosts RPC.
	info, ok := LookupDriver(DriverCloudruEvolution)
	Expect(ok).To(BeTrue())
	Expect(info.Plugin).To(Equal("cloudru"))
	Expect(PluginForDriver(DriverCloudruEvolution)).To(Equal("cloudru"))

	// Without a registered plugin, LookupDriver falls back to the built-in
	// metadata so Validate's capability-keyed checks keep working — a config
	// deployed without the plugin pin reads "the plugin is not installed", not
	// "unknown provider".
	ResetPluginDrivers()
	info, ok = LookupDriver(DriverCloudruEvolution)
	Expect(ok).To(BeTrue())
	Expect(info.MigratedToPlugin).To(BeTrue())
	Expect(info.Plugin).To(BeEmpty())
}

// Two plugins claiming one driver name is an operator-fixable configuration error,
// and the message has to name both of them — otherwise "uninstall one" is advice
// with no object.
func TestRegisterPluginDriver_RefusesTwoPluginsForOneDriver(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	Expect(RegisterPluginDriver("sc-plugin-a", "shared-vm", pluginRow())).ToNot(HaveOccurred())

	err := RegisterPluginDriver("sc-plugin-b", "shared-vm", pluginRow())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("sc-plugin-a"))
	Expect(err.Error()).To(ContainSubstring("sc-plugin-b"))

	// The first registration wins and is unchanged.
	Expect(PluginForDriver("shared-vm")).To(Equal("sc-plugin-a"))
}

// The same plugin registering twice is not an error: a plugin is allowed to be
// asked for its schema more than once in a run.
func TestRegisterPluginDriver_SamePluginIsIdempotent(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	Expect(RegisterPluginDriver("sc-plugin-a", "idem-vm", pluginRow())).ToNot(HaveOccurred())
	Expect(RegisterPluginDriver("sc-plugin-a", "idem-vm", pluginRow())).ToNot(HaveOccurred())
	Expect(PluginForDriver("idem-vm")).To(Equal("sc-plugin-a"))
}

func TestRegisterPluginDriver_RefusesEmptyNames(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	Expect(RegisterPluginDriver("", "some-vm", pluginRow())).To(HaveOccurred())
	Expect(RegisterPluginDriver("sc-plugin-a", "", pluginRow())).To(HaveOccurred())
}

// The slice this is all for: a config naming a plugin driver validates, with no
// change to validateCloud. Every refusal in there is keyed off a CAPABILITY and not
// a driver name, so a plugin row carries its own validation with it.
func TestValidate_AcceptsAPluginDriver(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	cfg := &VmConfig{Driver: "plugin-vm", Region: "ru-central1", PublicIP: true}

	// Before registration, it is simply unknown.
	err := cfg.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`unknown vm provider "plugin-vm"`))

	Expect(RegisterPluginDriver("sc-plugin-x", "plugin-vm", pluginRow())).ToNot(HaveOccurred())
	Expect(cfg.Validate()).ToNot(HaveOccurred())

	// And the capability-keyed refusals apply to it. This cloud runs cloud-init, so
	// SC generates the host key itself and a pasted one would be dead config.
	withHostKey := &VmConfig{
		Driver: "plugin-vm", Region: "ru-central1", PublicIP: true,
		HostKey: "ssh-ed25519 AAAA",
	}
	err = withHostKey.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`hostKey` is not valid"))

	// As does the one that catches a machine SC could never deploy to.
	noPublicIP := &VmConfig{Driver: "plugin-vm", Region: "ru-central1"}
	err = noPublicIP.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("requires `publicIP: true`"))
}

// A plugin's own words reach the operator verbatim — both the "not implemented yet"
// refusal and the unverified-driver gate. A plugin that ships a driver it has never
// run must be able to say so in the DSL, for the same reason the in-tree
// `cloudru-vm` row does: "implemented" and "known to work" are different claims.
func TestValidate_QuotesAPluginsOwnRefusals(t *testing.T) {
	RegisterTestingT(t)
	defer ResetPluginDrivers()

	Expect(RegisterPluginDriver("sc-plugin-x", "planned-vm", DriverInfo{
		RoadmapItem: "sc-some-roadmap-item",
	})).ToNot(HaveOccurred())
	err := (&VmConfig{Driver: "planned-vm", Region: "r", PublicIP: true}).Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not implemented yet"))
	Expect(err.Error()).To(ContainSubstring("sc-some-roadmap-item"))

	row := pluginRow()
	row.Unverified = "written against the API docs and never run against a real project"
	Expect(RegisterPluginDriver("sc-plugin-x", "untested-vm", row)).ToNot(HaveOccurred())

	cfg := &VmConfig{Driver: "untested-vm", Region: "r", PublicIP: true}
	err = cfg.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("never run against a real project"))

	cfg.AllowUnverifiedProvider = true
	Expect(cfg.Validate()).ToNot(HaveOccurred())
}
