// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

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
			// Accepted since multi-node (roadmap item sc-vm-multi-node-clusters): a byo
			// pool with no role declared anywhere is a compose fleet, which is what the
			// provisioning tier has looped over all along. This case used to assert the
			// opposite — "supports exactly one host, got 2" — and the role-set rules
			// that replaced it are in TestValidateHostRoles.
			name: "byo with two hosts",
			mutate: func(c *VmConfig) {
				c.Hosts = append(c.Hosts, VmHost{Address: "203.0.113.11", PrivateKey: "k"})
			},
			errMatch: "",
		},
		{
			// Two entries for one machine: the same cloud-init, the same ingress and —
			// in a k3s pool — the same node name, so the cluster reports N-1 members.
			name: "byo with a repeated address",
			mutate: func(c *VmConfig) {
				c.Hosts = append(c.Hosts, VmHost{Address: c.Hosts[0].Address, PrivateKey: "k"})
			},
			errMatch: `lists "203.0.113.10" more than once`,
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

// The shared ACME store's refusals. Every one of them fails LATE and quietly if it is
// not caught here: a wrong store is not a config error Caddy can report, it is a fleet
// whose certificates never issue, and the only evidence is in `docker logs sc-caddy` on
// whichever machine the CA happened to dial.
func TestValidateAcmeStorage(t *testing.T) {
	RegisterTestingT(t)

	withStore := func(mutate func(*VmAcmeStorage)) *VmConfig {
		cfg := byoConfig()
		cfg.Ingress = &VmIngress{
			Enable: true,
			Email:  "ops@example.com",
			AcmeStorage: &VmAcmeStorage{
				Bucket:    "sc-acme",
				Endpoint:  "https://storage.yandexcloud.net",
				AccessKey: "YCAJE...",
				SecretKey: "YCN...",
			},
		}
		mutate(cfg.Ingress.AcmeStorage)
		return cfg
	}

	cases := []struct {
		name     string
		mutate   func(*VmAcmeStorage)
		errMatch string
	}{
		{
			name:     "a complete store",
			mutate:   func(*VmAcmeStorage) {},
			errMatch: "",
		},
		{
			name:     "no bucket",
			mutate:   func(s *VmAcmeStorage) { s.Bucket = "" },
			errMatch: "is set without `bucket`",
		},
		{
			// One without the other is the worst shape of all: the module only uses
			// static credentials when BOTH are set, so half a pair silently falls
			// through to the AWS default chain.
			name:     "access key without secret",
			mutate:   func(s *VmAcmeStorage) { s.SecretKey = "" },
			errMatch: "`.secretKey` must be set together",
		},
		{
			name:     "secret without access key",
			mutate:   func(s *VmAcmeStorage) { s.AccessKey = "" },
			errMatch: "`.secretKey` must be set together",
		},
		{
			name: "neither endpoint nor region",
			mutate: func(s *VmAcmeStorage) {
				s.Endpoint = ""
				s.Region = ""
			},
			errMatch: "has neither `endpoint` nor `region`",
		},
		{
			// No endpoint means real AWS S3, which is legitimate — but then the region
			// is the only thing that routes the request.
			name: "region alone is enough",
			mutate: func(s *VmAcmeStorage) {
				s.Endpoint = ""
				s.Region = "eu-central-1"
			},
			errMatch: "",
		},
		{
			// The module hands this straight to the SDK as BaseEndpoint, which needs a
			// scheme. Without one the SDK fails per request, not at load.
			name:     "endpoint without a scheme",
			mutate:   func(s *VmAcmeStorage) { s.Endpoint = "storage.yandexcloud.net" },
			errMatch: "has no scheme",
		},
		{
			name:     "encryption key of the wrong length",
			mutate:   func(s *VmAcmeStorage) { s.EncryptionKey = "too-short" },
			errMatch: "secretbox needs exactly 32",
		},
		{
			name:     "encryption key of exactly 32 bytes",
			mutate:   func(s *VmAcmeStorage) { s.EncryptionKey = strings.Repeat("k", 32) },
			errMatch: "",
		},
		{
			name: "insecure against a plaintext endpoint",
			mutate: func(s *VmAcmeStorage) {
				s.Endpoint = "http://10.0.0.5:9000"
				s.Insecure = true
			},
			errMatch: "there is no TLS to skip verifying",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			err := withStore(tc.mutate).Validate()
			if tc.errMatch == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.errMatch))
		})
	}
}

// The three defaults SC computes rather than leaving to the module. `use_path_style` is
// the one that matters: the module's own default is false, which is right for AWS and
// wrong for every other S3 implementation — so SC derives it from whether a custom
// endpoint was given, and an operator can still override either way.
func TestAcmeStorageDefaults(t *testing.T) {
	RegisterTestingT(t)

	aws := &VmAcmeStorage{Bucket: "b", Region: "eu-central-1"}
	Expect(aws.UsePathStyle()).To(BeFalse())
	Expect(aws.StorageRegion()).To(Equal("eu-central-1"))
	Expect(aws.StoragePrefix()).To(Equal(DefaultAcmeStoragePrefix))

	minio := &VmAcmeStorage{Bucket: "b", Endpoint: "http://10.0.0.5:9000"}
	Expect(minio.UsePathStyle()).To(BeTrue())
	// us-east-1 and not empty: the SDK requires a region even where the store ignores
	// it, and the module would otherwise default it itself, out of SC's sight.
	Expect(minio.StorageRegion()).To(Equal(DefaultAcmeStorageRegion))

	Expect((&VmAcmeStorage{Endpoint: "http://x", PathStyle: lo.ToPtr(false)}).UsePathStyle()).To(BeFalse())
	Expect((&VmAcmeStorage{PathStyle: lo.ToPtr(true)}).UsePathStyle()).To(BeTrue())
	Expect((&VmAcmeStorage{Prefix: "certs"}).StoragePrefix()).To(Equal("certs"))

	// Absent on a single machine, which is the common case and must stay zero-config.
	plain := ingressConfig()
	Expect(plain.AcmeStorageEnabled()).To(BeFalse())
	// Present but with no bucket is not "enabled": validation refuses it, and the
	// renderer must not emit a half-written block in the meantime.
	plain.Ingress.AcmeStorage = &VmAcmeStorage{}
	Expect(plain.AcmeStorageEnabled()).To(BeFalse())
}

// TestValidateHostRoles is the role set of a byo pool, which is what replaced the
// old "exactly one host" refusal (roadmap item sc-vm-multi-node-clusters, phase 1a).
//
// Every refused shape here is one that otherwise deploys and half-works: a machine
// left out of the cluster, two services fighting over :443, or an etcd membership
// that tolerates no failures while reading as HA.
func TestValidateHostRoles(t *testing.T) {
	RegisterTestingT(t)

	host := func(address, role string) VmHost {
		return VmHost{Address: address, PrivateKey: "k", Role: role}
	}

	// k3sHost also names the address the machine's SIBLINGS dial it on. Every valid
	// multi-node k3s shape has to: without it k3s advertises its default-route
	// address, which on a single-NIC VPS is the public one (VmHost.PrivateAddress).
	// The refused shapes below deliberately do NOT use this helper — the addressing
	// check runs after the topology checks, so a pool that is the wrong shape says so
	// rather than complaining about addresses it would never have used.
	k3sHost := func(address, private, role string) VmHost {
		h := host(address, role)
		h.PrivateAddress = private
		return h
	}

	cases := []struct {
		name     string
		hosts    []VmHost
		errMatch string
	}{
		{
			// The pre-existing shape: one host, no role. Still valid, still inferred.
			name:  "single host, no role",
			hosts: []VmHost{host("203.0.113.10", "")},
		},
		{
			name: "three hosts, no roles — an inferred compose fleet",
			hosts: []VmHost{
				host("203.0.113.10", ""), host("203.0.113.11", ""), host("203.0.113.12", ""),
			},
		},
		{
			name: "explicit compose fleet",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleCompose)), host("203.0.113.11", string(RoleCompose)),
			},
		},
		{
			name: "one server, two agents — the default k3s topology",
			hosts: []VmHost{
				k3sHost("203.0.113.10", "10.0.0.10", string(RoleK3sServer)),
				k3sHost("203.0.113.11", "10.0.0.11", string(RoleK3sAgent)),
				k3sHost("203.0.113.12", "10.0.0.12", string(RoleK3sAgent)),
			},
		},
		{
			name: "three servers — embedded-etcd HA",
			hosts: []VmHost{
				k3sHost("203.0.113.10", "10.0.0.10", string(RoleK3sServer)),
				k3sHost("203.0.113.11", "10.0.0.11", string(RoleK3sServer)),
				k3sHost("203.0.113.12", "10.0.0.12", string(RoleK3sServer)),
			},
		},
		{
			// A single-node cluster has no node-to-node traffic, so it is exempt — and
			// has to be, or every k3s config written before `privateAddress` existed
			// would stop validating.
			name:  "single k3s server needs no private address",
			hosts: []VmHost{host("203.0.113.10", string(RoleK3sServer))},
		},
		{
			// The whole point of the refusal: this shape otherwise DEPLOYS, green, with
			// etcd and an unencrypted VXLAN overlay on the public internet.
			name: "multi-node k3s with no private addresses",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)),
				host("203.0.113.11", string(RoleK3sAgent)),
			},
			errMatch: "a multi-node k3s pool requires `privateAddress` on every entry under `hosts`, " +
				"and hosts[0] (203.0.113.10), hosts[1] (203.0.113.11) name none",
		},
		{
			// All-or-none, same as `role`: one machine left on its default route is one
			// machine advertising a public node-ip, which is the whole failure.
			name: "multi-node k3s with one private address missing",
			hosts: []VmHost{
				k3sHost("203.0.113.10", "10.0.0.10", string(RoleK3sServer)),
				host("203.0.113.11", string(RoleK3sAgent)),
			},
			errMatch: "and hosts[1] (203.0.113.11) names none",
		},
		{
			// Two nodes claiming one node-ip: the second overwrites the first in the
			// cluster, and the symptom appears nowhere near the config.
			name: "multi-node k3s with a repeated private address",
			hosts: []VmHost{
				k3sHost("203.0.113.10", "10.0.0.10", string(RoleK3sServer)),
				k3sHost("203.0.113.11", "10.0.0.10", string(RoleK3sAgent)),
			},
			errMatch: "`hosts` gives \"10.0.0.10\" as the `privateAddress` of more than one machine",
		},
		{
			// A compose fleet is exempt: those machines never talk to each other. The
			// only shared state is the ACME store, reached outbound over HTTPS.
			name: "compose fleet needs no private addresses",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleCompose)),
				host("203.0.113.11", string(RoleCompose)),
				host("203.0.113.12", string(RoleCompose)),
			},
		},
		{
			name:     "unknown role",
			hosts:    []VmHost{host("203.0.113.10", "k3s-master")},
			errMatch: `unknown ` + "`role`" + `: "k3s-master"; expected one of "compose", "k3s-server", "k3s-agent"`,
		},
		{
			// The shape the all-or-none rule exists for: host 1's role would have to be
			// guessed, and the guess decides whether it joins the cluster at all.
			name: "half-declared pool",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)), host("203.0.113.11", ""),
			},
			errMatch: "either every entry under `hosts` names a `role` or none does; hosts[1] (203.0.113.11) names none",
		},
		{
			name: "half-declared pool names every host that is missing one",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)), host("203.0.113.11", ""), host("203.0.113.12", ""),
			},
			errMatch: "hosts[1] (203.0.113.11), hosts[2] (203.0.113.12) name none",
		},
		{
			name: "compose mixed with k3s",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)), host("203.0.113.11", string(RoleCompose)),
			},
			errMatch: `mixes "compose" with k3s roles`,
		},
		{
			name: "agents with no server",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sAgent)), host("203.0.113.11", string(RoleK3sAgent)),
			},
			errMatch: `declares 2 k3s agent(s) and no "k3s-server"`,
		},
		{
			// Two members tolerate zero failures, so they are strictly worse than one.
			name: "two servers",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)), host("203.0.113.11", string(RoleK3sServer)),
			},
			errMatch: "needs an odd number (1, 3, 5)",
		},
		{
			name: "four servers",
			hosts: []VmHost{
				host("203.0.113.10", string(RoleK3sServer)), host("203.0.113.11", string(RoleK3sServer)),
				host("203.0.113.12", string(RoleK3sServer)), host("203.0.113.13", string(RoleK3sServer)),
			},
			errMatch: "needs an odd number (1, 3, 5)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			cfg := byoConfig()
			cfg.Hosts = tc.hosts

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

// EffectiveRole's fallback is what keeps every config written before `role:` existed
// producing the same HostFacts — pkg/clouds/pulumi/vm/byo.go hardcoded RoleCompose.
func TestEffectiveRole(t *testing.T) {
	RegisterTestingT(t)

	Expect(VmHost{}.EffectiveRole()).To(Equal(RoleCompose))
	Expect(VmHost{Role: string(RoleK3sServer)}.EffectiveRole()).To(Equal(RoleK3sServer))
	Expect(VmHost{Role: string(RoleK3sAgent)}.EffectiveRole()).To(Equal(RoleK3sAgent))

	Expect(RoleK3sServer.IsK3s()).To(BeTrue())
	Expect(RoleK3sAgent.IsK3s()).To(BeTrue())
	Expect(RoleCompose.IsK3s()).To(BeFalse())
	Expect(VmHost{}.EffectiveRole().IsK3s()).To(BeFalse())
}

// EffectivePrivateAddress falls back to `address`, which is what keeps every
// single-host and compose-fleet config written before `privateAddress` existed
// producing the same HostFacts. A multi-node k3s pool cannot reach the fallback —
// validateHostRoles refuses one that names no private addresses.
func TestEffectivePrivateAddress(t *testing.T) {
	RegisterTestingT(t)

	Expect(VmHost{Address: "203.0.113.10"}.EffectivePrivateAddress()).To(Equal("203.0.113.10"))
	Expect(VmHost{Address: "203.0.113.10", PrivateAddress: "10.0.0.10"}.EffectivePrivateAddress()).
		To(Equal("10.0.0.10"))
}

// NodeCount is the single place the pool's size is decided, so that two drivers
// can never disagree about whether `zones: [a, b]` means one machine or two.
func TestNodeCount(t *testing.T) {
	RegisterTestingT(t)

	// byo: the machines exist, so the count is `len(hosts)` and `nodes` is refused.
	Expect(byoConfig().NodeCount()).To(Equal(1))
	threeHosts := byoConfig()
	threeHosts.Hosts = append(threeHosts.Hosts,
		VmHost{Address: "203.0.113.11", PrivateKey: "k"}, VmHost{Address: "203.0.113.12", PrivateKey: "k"})
	Expect(threeHosts.NodeCount()).To(Equal(3))

	// Cloud: zones alone are a count, `nodes` alone is a count, and neither is 1.
	Expect(cloudConfig().NodeCount()).To(Equal(1))

	zonesOnly := cloudConfig()
	zonesOnly.Zones = []string{"ru-moscow-1a", "ru-moscow-1b", "ru-moscow-1c"}
	Expect(zonesOnly.NodeCount()).To(Equal(3))

	nodesOnly := cloudConfig()
	nodesOnly.Nodes = 3
	Expect(nodesOnly.NodeCount()).To(Equal(3))
	Expect(nodesOnly.ZoneForNode(0)).To(BeEmpty(), "no zones listed: the driver picks")

	// More machines than zones: round-robin, and `nodes` wins over `len(zones)`.
	both := cloudConfig()
	both.Nodes = 5
	both.Zones = []string{"ru-moscow-1a", "ru-moscow-1b"}
	Expect(both.NodeCount()).To(Equal(5))
	Expect(lo.Map(lo.Range(both.NodeCount()), func(i int, _ int) string { return both.ZoneForNode(i) })).
		To(Equal([]string{"ru-moscow-1a", "ru-moscow-1b", "ru-moscow-1a", "ru-moscow-1b", "ru-moscow-1a"}))

	// Index stability is the whole reason the assignment is i%len(zones) rather
	// than anything smarter: growing the pool must not move an existing machine,
	// because a machine cannot change zone without being destroyed.
	grown := cloudConfig()
	grown.Nodes = 7
	grown.Zones = both.Zones
	for i := range both.NodeCount() {
		Expect(grown.ZoneForNode(i)).To(Equal(both.ZoneForNode(i)), "machine %d moved zone when the pool grew", i)
	}
}

// The zone count is checked against the cloud's own measured capability — and NOT
// checked when that capability is still zero, which Capabilities.MultiAZInRegion
// defines as "not measured yet" rather than "none".
func TestValidateNodeCountAgainstZoneCapability(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.Zones = []string{"ru-moscow-1a", "ru-moscow-1b", "ru-moscow-1c"}

	unmeasured := DriverInfo{Capabilities: Capabilities{MultiAZInRegion: 0}}
	Expect(cfg.validateNodeCount(unmeasured)).ToNot(HaveOccurred())

	roomy := DriverInfo{Capabilities: Capabilities{MultiAZInRegion: 3}}
	Expect(cfg.validateNodeCount(roomy)).ToNot(HaveOccurred())

	tight := DriverInfo{Capabilities: Capabilities{MultiAZInRegion: 2}}
	err := cfg.validateNodeCount(tight)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`zones` lists 3 entries but `provider: cloudru-vm` has 2 usable zone(s)"))
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
			// Accepted since multi-node (roadmap item sc-vm-multi-node-clusters): two
			// zones means two machines, one each. This case asserted the refusal that
			// used to stand here; the pool-shape rules are in TestValidateNodeCount.
			name:     "two zones",
			mutate:   func(c *VmConfig) { c.Zones = []string{"ru-moscow-1a", "ru-moscow-1b"} },
			errMatch: "",
		},
		{
			name:     "negative node count",
			mutate:   func(c *VmConfig) { c.Nodes = -1 },
			errMatch: "`nodes` must be positive, got -1",
		},
		{
			// A zone nobody is placed in: the operator wanted either more machines or
			// fewer zones, and guessing which silently picks one.
			name: "fewer nodes than zones",
			mutate: func(c *VmConfig) {
				c.Nodes = 2
				c.Zones = []string{"ru-moscow-1a", "ru-moscow-1b", "ru-moscow-1c"}
			},
			errMatch: "`nodes` is 2 but `zones` lists 3",
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

	cfg := evolutionConfig()
	cfg.HostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample"
	Expect(cfg.Validate()).ToNot(HaveOccurred())

	// And the cloud invariants still apply to it — the escape hatch is one field, not
	// a bypass of the path.
	cfg.PublicIP = false
	Expect(cfg.Validate().Error()).To(ContainSubstring("requires `publicIP: true`"))
}

// The same capability, read the other way round. `extraSshKeys` is honoured by
// writing the keys into user-data, so a cloud that accepts no user-data must refuse
// the field rather than take it and drop it — the failure mode otherwise is an
// operator who believes they can still log in during an incident and discovers in
// the incident that they cannot.
func TestValidateCloudRefusesExtraSshKeysWithoutCloudInit(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.ExtraSshKeys = []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample ops@example.com"}

	// cloudConfig() is `cloudru-vm`, which does run cloud-init: accepted there.
	Expect(Drivers[cfg.Driver].Capabilities.CloudInit).To(BeTrue())
	Expect(cfg.Validate()).ToNot(HaveOccurred())

	// And refused on Evolution, which accepts no user-data to write them into.
	noCloudInit := evolutionConfig()
	noCloudInit.ExtraSshKeys = cfg.ExtraSshKeys
	Expect(noCloudInit.Validate().Error()).To(ContainSubstring("`extraSshKeys` is not valid"))

	noCloudInit.ExtraSshKeys = nil
	Expect(noCloudInit.Validate()).ToNot(HaveOccurred())
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

	regionless := evolutionConfig()
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

// The verified/unverified gate, both halves. "Implemented" and "known to work" are
// different claims, and before this gate `main` offered a Cloud.ru Advanced driver
// that was indistinguishable in the DSL from the Evolution one that had actually
// provisioned a machine end to end.
func TestValidateRefusesUnverifiedProvider(t *testing.T) {
	RegisterTestingT(t)

	// The gate keys off the capability row, never off a driver name — so a driver
	// that gets its first live smoke is released by clearing one field here, and
	// one that is added tomorrow is gated without touching Validate.
	Expect(Drivers[DriverCloudru].Unverified).ToNot(BeEmpty())
	Expect(Drivers[DriverCloudruEvolution].Unverified).To(BeEmpty(),
		"Evolution is the one Cloud.ru estate SC has provisioned on")
	for name, drv := range Drivers {
		if drv.Unverified != "" {
			Expect(drv.RoadmapItem).ToNot(BeEmpty(),
				"the refusal for %q points the operator at an item to follow", name)
		}
	}

	// Refused by default: the driver compiles, and that is all anyone knows.
	gated := cloudConfig()
	gated.AllowUnverifiedProvider = false
	err := gated.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("has never provisioned a machine on the cloud it addresses"))
	Expect(err.Error()).To(ContainSubstring("cloudru-evolution-vm"),
		"the refusal names the driver to use instead, which is the whole reason it is worth refusing")
	Expect(err.Error()).To(ContainSubstring(Drivers[DriverCloudru].RoadmapItem))

	// And accepted with the opt-in, because this is a gate and not a removal: an
	// operator willing to debug the driver is how it stops being unverified.
	gated.AllowUnverifiedProvider = true
	Expect(gated.Validate()).ToNot(HaveOccurred())

	// The other half. Left unrefused, the flag would spread into every example and
	// read as a standing exception to a gate the driver is not subject to — and the
	// day that driver did need gating, the config would already be opted in.
	verified := evolutionConfig()
	verified.AllowUnverifiedProvider = true
	err = verified.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`allowUnverifiedProvider` is not valid"))
	Expect(err.Error()).To(ContainSubstring("live-verified"))
}

// `ingress.originTls: false` and the ACME settings are alternatives, not a pair. Each
// refusal below replaces a belief an operator would otherwise keep: that the fleet
// still holds a certificate, or that a CA override still applies. Neither is visible
// until the thing in front of the machine goes away.
func TestValidateOriginTls(t *testing.T) {
	RegisterTestingT(t)

	noTls := func(mutate func(*VmIngress)) *VmConfig {
		cfg := byoConfig()
		cfg.Ingress = &VmIngress{Enable: true, OriginTls: lo.ToPtr(false)}
		mutate(cfg.Ingress)
		return cfg
	}

	// The whole point of the field: `email` stops being required, because there is no
	// ACME account to name.
	Expect(noTls(func(*VmIngress) {}).Validate()).ToNot(HaveOccurred())
	Expect(noTls(func(*VmIngress) {}).OriginTlsEnabled()).To(BeFalse())

	for _, tc := range []struct {
		name     string
		mutate   func(*VmIngress)
		errMatch string
	}{
		{
			name:     "email",
			mutate:   func(i *VmIngress) { i.Email = "ops@example.com" },
			errMatch: "`ingress.email` is set together with `ingress.originTls: false`",
		},
		{
			name:     "acmeCa",
			mutate:   func(i *VmIngress) { i.AcmeCa = "https://acme.example.com/directory" },
			errMatch: "`ingress.acmeCa` is set together with `ingress.originTls: false`",
		},
		{
			name: "acmeEab",
			mutate: func(i *VmIngress) {
				i.AcmeEabKeyID = "kid"
				i.AcmeEabMacKey = "mac"
			},
			errMatch: "`ingress.acmeEabKeyId` is set together with `ingress.originTls: false`",
		},
		{
			name: "acmeStorage",
			mutate: func(i *VmIngress) {
				i.AcmeStorage = &VmAcmeStorage{Bucket: "sc-acme", Region: "eu-central-1"}
			},
			// Spelled out at length in the message because these are the two
			// alternative front doors for a fleet and the operator has asked for both.
			errMatch: "These are the two alternative front doors for a fleet, not a pair",
		},
		{
			// imageDelivery is checked BEFORE the origin-TLS branch, so a resource
			// that is wrong in two ways still hears about the one that is wrong on
			// every path.
			name:     "imageDelivery is still validated",
			mutate:   func(i *VmIngress) { i.ImageDelivery = "carrier-pigeon" },
			errMatch: "unknown `ingress.imageDelivery`",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := noTls(tc.mutate).Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.errMatch))
		})
	}
}

// Backwards compatibility, asserted rather than assumed: the operator's standing
// requirement is that nothing already deployed changes behaviour. Every way of NOT
// saying `originTls: false` has to read as "this machine terminates TLS", because that
// is what every vm resource written before the field did.
func TestOriginTlsDefaultsToOn(t *testing.T) {
	RegisterTestingT(t)

	unset := byoConfig()
	unset.Ingress = &VmIngress{Enable: true, Email: "ops@example.com"}
	Expect(unset.OriginTlsEnabled()).To(BeTrue())

	explicit := byoConfig()
	explicit.Ingress = &VmIngress{Enable: true, Email: "ops@example.com", OriginTls: lo.ToPtr(true)}
	Expect(explicit.OriginTlsEnabled()).To(BeTrue())
	Expect(explicit.Validate()).ToNot(HaveOccurred())

	// No ingress at all: there is no Caddy to ask, and a caller that reads this must
	// not take "false" for "an edge terminates" — it would render an http:// site for
	// a machine that serves no sites.
	none := byoConfig()
	none.Ingress = nil
	Expect(none.OriginTlsEnabled()).To(BeTrue())

	disabled := byoConfig()
	disabled.Ingress = &VmIngress{Enable: false, OriginTls: lo.ToPtr(false)}
	Expect(disabled.OriginTlsEnabled()).To(BeTrue())
}
