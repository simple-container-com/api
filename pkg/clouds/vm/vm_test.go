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
			// The message an operator with a Cloud.ru subscription hits today. It has
			// to name both the roadmap item and the way out, because "not implemented"
			// on its own leaves them with nothing to do.
			name:     "known but unimplemented provider",
			mutate:   func(c *VmConfig) { c.Driver = DriverCloudru; c.Hosts = nil; c.Region = "ru-moscow-1" },
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

	h := cfg.Hosts[0]
	Expect(h.EffectivePort()).To(Equal(DefaultSshPort))
	Expect(h.EffectiveUser("")).To(Equal(DefaultSshUser))
	Expect(h.EffectiveUser("deployer")).To(Equal("deployer"))
	h.User = "ubuntu"
	Expect(h.EffectiveUser("deployer")).To(Equal("ubuntu"))
}
