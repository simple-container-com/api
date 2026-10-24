// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func byoConfig() *VmConfig {
	return &VmConfig{
		Driver: DriverByo,
		Hosts: []VmHost{{
			Address:    "203.0.113.10",
			PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----",
		}},
	}
}

// TestValidate is the refuse-don't-ignore table (compose-on-vm.md §2.2, D4). Each
// case names a field an operator could plausibly write and the message they get
// back. A field accepted and silently dropped is the failure mode this whole table
// exists to prevent: the operator stays certain the setting applied.
func TestValidate(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		mutate   func(*VmConfig)
		errMatch string
	}{
		{
			name:     "valid byo host",
			mutate:   func(*VmConfig) {},
			errMatch: "",
		},
		{
			name:     "no provider",
			mutate:   func(c *VmConfig) { c.Driver = "" },
			errMatch: "requires `provider`",
		},
		{
			name:     "misspelled provider",
			mutate:   func(c *VmConfig) { c.Driver = "cloudru" },
			errMatch: `unknown vm provider "cloudru"`,
		},
		{
			// The message an operator with an MWS subscription hits today. It has to
			// name both the roadmap item and the way out, because "not implemented" on
			// its own leaves them with nothing to do.
			//
			// This used to be the Cloud.ru case; it moved to MWS when `cloudru-vm`
			// landed. Keeping a case here at all is the point — the refusal is the only
			// thing an operator on one of the five remaining clouds ever sees.
			name: "known but unimplemented provider",
			mutate: func(c *VmConfig) {
				c.Driver = DriverMws
				c.Hosts = nil
				c.Region = "ru-central-1"
				c.PublicIP = true
			},
			errMatch: "not implemented yet (roadmap item sc-ru-clouds-beyond-yandex)",
		},
		{
			name:     "byo with region",
			mutate:   func(c *VmConfig) { c.Region = "ru-moscow-1" },
			errMatch: `"region" is not valid with `,
		},
		{
			name:     "byo with size",
			mutate:   func(c *VmConfig) { c.Size = &VmSize{Cores: 2} },
			errMatch: `"size" is not valid with `,
		},
		{
			name: "byo with several creation-time fields names all of them",
			mutate: func(c *VmConfig) {
				c.Image = "ubuntu-2404-lts"
				c.PublicIP = true
				c.Network = &VmNetwork{ID: "net-1"}
			},
			errMatch: `"image", "publicIP", "network"`,
		},
		{
			name:     "byo without hosts",
			mutate:   func(c *VmConfig) { c.Hosts = nil },
			errMatch: "requires at least one entry under `hosts`",
		},
		{
			// Refused rather than deployed-to-the-first-host: a silently idle second
			// machine is exactly the kind of thing nobody notices for a month.
			name: "byo with two hosts",
			mutate: func(c *VmConfig) {
				c.Hosts = append(c.Hosts, VmHost{Address: "203.0.113.11", PrivateKey: "k"})
			},
			errMatch: "supports exactly one host, got 2",
		},
		{
			name:     "host without address",
			mutate:   func(c *VmConfig) { c.Hosts[0].Address = "" },
			errMatch: "hosts[0] requires `address`",
		},
		{
			name:     "host without key",
			mutate:   func(c *VmConfig) { c.Hosts[0].PrivateKey = "" },
			errMatch: "requires `privateKey`",
		},
		{
			name:     "host with out-of-range port",
			mutate:   func(c *VmConfig) { c.Hosts[0].Port = 70000 },
			errMatch: "invalid `port`: 70000",
		},
		{
			name:     "ingress without acme email",
			mutate:   func(c *VmConfig) { c.Ingress = &VmIngress{Enable: true} },
			errMatch: "requires `ingress.email`",
		},
		{
			name:     "ingress with acme email",
			mutate:   func(c *VmConfig) { c.Ingress = &VmIngress{Enable: true, Email: "ops@example.com"} },
			errMatch: "",
		},
		{
			// `ingress.enable: false` is not the same as absent, and neither needs an
			// address — asking for one would make disabling ingress harder than
			// enabling it.
			name:     "ingress disabled needs no email",
			mutate:   func(c *VmConfig) { c.Ingress = &VmIngress{Enable: false} },
			errMatch: "",
		},
		{
			name: "unknown ingress imageDelivery",
			mutate: func(c *VmConfig) {
				c.Ingress = &VmIngress{Enable: true, Email: "ops@example.com", ImageDelivery: "rsync"}
			},
			errMatch: "unknown `ingress.imageDelivery` \"rsync\"",
		},
		{
			name: "ingress shipped from the runner",
			mutate: func(c *VmConfig) {
				c.Ingress = &VmIngress{
					Enable:        true,
					Email:         "ops@example.com",
					ImageDelivery: ImageDeliverySshTar,
				}
			},
			errMatch: "",
		},
		{
			// Refused rather than merged into hosts[0].hostKey: on this path the host
			// key already has a per-host spelling, and a pool of byo hosts does not
			// share one. Accepting both would make whichever lost look applied.
			name:     "byo with a resource-level hostKey",
			mutate:   func(c *VmConfig) { c.HostKey = "ssh-ed25519 AAAAC3Nz" },
			errMatch: "pin it per machine, as `hosts[].hostKey`",
		},
		{
			name:     "byo with a per-host hostKey",
			mutate:   func(c *VmConfig) { c.Hosts[0].HostKey = "203.0.113.10 ssh-ed25519 AAAAC3Nz" },
			errMatch: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			cfg := byoConfig()
			tc.mutate(cfg)

			err := cfg.Validate()
			if tc.errMatch == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.errMatch))
		})
	}
}

// A cloud driver's own invariants are checked before the not-implemented refusal:
// an operator whose config is also wrong in shape should hear about the shape, so
// that the config is already right when the driver lands.
func TestValidateCloudShapeBeforeNotImplemented(t *testing.T) {
	RegisterTestingT(t)

	noRegion := &VmConfig{Driver: DriverCloudru}
	Expect(noRegion.Validate().Error()).To(ContainSubstring("requires `region`"))

	withHosts := &VmConfig{
		Driver: DriverCloudru,
		Region: "ru-moscow-1",
		Hosts:  []VmHost{{Address: "203.0.113.10", PrivateKey: "k"}},
	}
	Expect(withHosts.Validate().Error()).To(ContainSubstring("`hosts` is only valid with `provider: byo`"))
}

// The invariants every machine-creating driver shares. They are checked here, once,
// rather than per driver — `cloudru-vm` is written assuming each of them holds, so a
// hole here is a hole in every driver that follows.
func TestValidateCloud(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		mutate   func(*VmConfig)
		errMatch string
	}{
		{
			name:     "minimal cloud config",
			mutate:   func(*VmConfig) {},
			errMatch: "",
		},
		{
			// Refused at validation rather than discovered as an SSH dial timeout
			// against a machine that was created, billed, and is unreachable: every
			// delivery step is an outbound connection from wherever `sc deploy` runs.
			name:     "no public address",
			mutate:   func(c *VmConfig) { c.PublicIP = false },
			errMatch: "requires `publicIP: true`",
		},
		{
			name:     "one zone",
			mutate:   func(c *VmConfig) { c.Zones = []string{"ru-moscow-1a"} },
			errMatch: "",
		},
		{
			name:     "two zones",
			mutate:   func(c *VmConfig) { c.Zones = []string{"ru-moscow-1a", "ru-moscow-1b"} },
			errMatch: "at most one entry under `zones`, got 2",
		},
		{
			// Half a BYO network is the one shape that cannot be completed by
			// guessing: picking "some subnet in the VPC" places the machine in a zone,
			// or behind a route table, the operator did not choose.
			name:     "network id without subnets",
			mutate:   func(c *VmConfig) { c.Network = &VmNetwork{ID: "vpc-1"} },
			errMatch: "`network.subnets` is empty",
		},
		{
			name:     "subnets without network id",
			mutate:   func(c *VmConfig) { c.Network = &VmNetwork{Subnets: map[string]string{"ru-moscow-1a": "sub-1"}} },
			errMatch: "`network.id` is empty",
		},
		{
			name: "both halves of a byo network",
			mutate: func(c *VmConfig) {
				c.Network = &VmNetwork{ID: "vpc-1", Subnets: map[string]string{"ru-moscow-1a": "sub-1"}}
			},
			errMatch: "",
		},
		{
			// Shared with the byo path, and checked on this one too: the pair drifted
			// apart once already.
			name:     "ingress without acme email",
			mutate:   func(c *VmConfig) { c.Ingress = &VmIngress{Enable: true} },
			errMatch: "requires `ingress.email`",
		},
		{
			// cloudConfig() is `cloudru-vm`, which runs cloud-init and therefore gets
			// an SC-generated host key. A pasted one would never be used, so it is
			// refused rather than stored — the whole point of the field is that on the
			// clouds where it IS accepted, it closes a real exposure, and a config that
			// looked accepted here would read as having closed one that is already shut.
			name:     "hostKey on a cloud that seeds its own",
			mutate:   func(c *VmConfig) { c.HostKey = "ssh-ed25519 AAAAC3Nz" },
			errMatch: "this cloud runs cloud-init",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			cfg := cloudConfig()
			tc.mutate(cfg)

			err := cfg.Validate()
			if tc.errMatch == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.errMatch))
		})
	}
}

// The mirror image of the "hostKey on a cloud that seeds its own" case above, and
// the reason that rule is written against a CAPABILITY rather than a driver name.
//
// Cloud.ru Evolution refuses `user_data` at VM create (measured: HTTP 422
// `extra_forbidden`, 2026-10-24), so SC cannot write a host key into the machine and
// cannot ever learn the real one from the API either. Without this field the only
// options are trust-on-first-use forever or no SSH at all — so here, uniquely, a
// pasted `ssh-keyscan` line is load-bearing config and must validate.
func TestValidateCloudAcceptsHostKeyWhereCloudInitIsImpossible(t *testing.T) {
	RegisterTestingT(t)

	// The premise. If this ever flips — Evolution grows user-data, somebody fills the
	// row in from a newer measurement — the acceptance below becomes dead config and
	// this test is where that gets noticed.
	Expect(Drivers[DriverCloudruEvolution].Capabilities.CloudInit).To(BeFalse())

	cfg := cloudConfig()
	cfg.Driver = DriverCloudruEvolution
	// This cloud has no regions, and cloudConfig() sets one for the region-scoped
	// majority — see TestValidateCloudRegionFollowsTheCapability.
	cfg.Region = ""
	cfg.HostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample"
	Expect(cfg.Validate()).ToNot(HaveOccurred())

	// And the cloud invariants still apply to it — the escape hatch is one field, not
	// a bypass of the path.
	cfg.PublicIP = false
	Expect(cfg.Validate().Error()).To(ContainSubstring("requires `publicIP: true`"))
}

// CredentialsValue must not become a dumping ground for the config: with `byo` the
// config holds host private keys, and this value exists to be handed to other code.
// yandex.AccountConfig serialises itself as a fallback; doing that here would mean
// a key in the credentials blob of every byo stack.
func TestCredentialsValueNeverSerialisesHostKeys(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Hosts[0].PrivateKey = "SUPER-SECRET-KEY-MATERIAL"

	Expect(cfg.CredentialsValue()).To(Equal("{}"))
	Expect(cfg.CredentialsValue()).ToNot(ContainSubstring("SUPER-SECRET"))

	cfg.Credentials = api.Credentials{Credentials: `{"accessKey":"a"}`}
	Expect(cfg.CredentialsValue()).To(Equal(`{"accessKey":"a"}`))
}

func TestEffectiveDefaults(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	Expect(cfg.EffectiveSshUser()).To(Equal(DefaultSshUser))
	Expect(cfg.EffectiveImage()).To(Equal(DefaultImage))
	Expect(cfg.IngressEnabled()).To(BeFalse())
	Expect(cfg.ProviderType()).To(Equal(ProviderType))
	// "n/a" rather than "": `${resource:...projectId}` must not interpolate to
	// something that looks like a real project.
	Expect(cfg.ProjectIdValue()).To(Equal("n/a"))

	// The machine pulls Caddy unless told otherwise — today's behaviour for every
	// existing vm resource, and shipping would make `sc provision` need a local
	// docker daemon it has never needed.
	Expect(cfg.EffectiveIngressImageDelivery()).To(Equal(ImageDeliveryRegistry))
	Expect(cfg.ShipsIngressImage()).To(BeFalse())

	h := cfg.Hosts[0]
	Expect(h.EffectivePort()).To(Equal(DefaultSshPort))
	Expect(h.EffectiveUser("")).To(Equal(DefaultSshUser))
	Expect(h.EffectiveUser("deployer")).To(Equal("deployer"))
	h.User = "ubuntu"
	Expect(h.EffectiveUser("deployer")).To(Equal("ubuntu"))
}

// ShipsIngressImage gates a whole local.Command in the provision graph, so it has to
// be false whenever there is no ingress at all — including the case where somebody
// set the delivery and then disabled the ingress, which is how a half-edited config
// looks.
func TestShipsIngressImageNeedsAnIngress(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Ingress = &VmIngress{Enable: false, ImageDelivery: ImageDeliverySshTar}
	Expect(cfg.ShipsIngressImage()).To(BeFalse())

	cfg.Ingress = &VmIngress{Enable: true, Email: "ops@example.com", ImageDelivery: ImageDeliverySshTar}
	Expect(cfg.ShipsIngressImage()).To(BeTrue())
	Expect(cfg.Validate()).ToNot(HaveOccurred())
}

// A missing `size` must not mean "whatever the cloud lists first". A cloud driver
// picks a flavour by FILTERING on cores and memory, so an unfilled half is a filter
// that matches hundreds of flavours — and a different one on the next deploy, which
// replaces the machine.
func TestEffectiveSize(t *testing.T) {
	RegisterTestingT(t)

	absent := (&VmConfig{}).EffectiveSize()
	Expect(absent).To(Equal(VmSize{Cores: DefaultCores, Memory: DefaultMemory}))

	// Each half defaults on its own, so `size: {memory: 8Gi}` is a legal way to ask
	// for more memory at the default core count.
	Expect((&VmConfig{Size: &VmSize{Memory: "8Gi"}}).EffectiveSize()).
		To(Equal(VmSize{Cores: DefaultCores, Memory: "8Gi"}))
	Expect((&VmConfig{Size: &VmSize{Cores: 8}}).EffectiveSize()).
		To(Equal(VmSize{Cores: 8, Memory: DefaultMemory}))

	// Disk is NOT defaulted: unlike cores and memory it is not a flavour filter, and
	// an empty value lets each image keep its own root-disk size, which is better
	// than one SC invents.
	full := (&VmConfig{Size: &VmSize{Cores: 4, Memory: "16Gi", Disk: "60Gi", DiskType: "SSD"}}).EffectiveSize()
	Expect(full).To(Equal(VmSize{Cores: 4, Memory: "16Gi", Disk: "60Gi", DiskType: "SSD"}))
	Expect(absent.Disk).To(BeEmpty())
	Expect(absent.DiskType).To(BeEmpty())
}

// `region` is required on a region-scoped cloud and REFUSED on a regionless one,
// and the condition is the capability rather than a driver name.
//
// Cloud.ru Evolution is the first cloud with no region at all: one global compute
// endpoint, placement by `ru.AZ-*` availability zone. `provision` against it failed
// on the blanket `requires region` check (live, 2026-10-24) — and the fix is a
// refusal rather than a quiet acceptance, because a `region:` SC dropped would read
// as placement the operator had chosen.
func TestValidateCloudRegionFollowsTheCapability(t *testing.T) {
	RegisterTestingT(t)

	// The premise, so that filling this row in from a newer measurement is noticed
	// here rather than becoming a refusal operators cannot satisfy.
	Expect(Drivers[DriverCloudruEvolution].Capabilities.Regionless).To(BeTrue())
	Expect(Drivers[DriverCloudru].Capabilities.Regionless).To(BeFalse(),
		"Advanced IS region-scoped: the region selects the IAM endpoint too")

	regionless := cloudConfig()
	regionless.Driver = DriverCloudruEvolution
	regionless.Region = ""
	Expect(regionless.Validate()).ToNot(HaveOccurred())

	regionless.Region = "ru-moscow-1"
	err := regionless.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`region` is not valid"))
	Expect(err.Error()).To(ContainSubstring("zones:"), "the refusal has to say what to use instead")

	// And the majority case is untouched.
	scoped := cloudConfig()
	scoped.Driver = DriverCloudru
	scoped.Region = ""
	Expect(scoped.Validate().Error()).To(ContainSubstring("requires `region`"))
}
