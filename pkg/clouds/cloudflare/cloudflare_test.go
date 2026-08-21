// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudflare

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestAuthConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	c := &AuthConfig{
		Credentials: api.Credentials{Credentials: "secret-token-value"},
		AccountId:   "acct-12345",
	}

	Expect(c.CredentialsValue()).To(Equal("secret-token-value"))
	Expect(c.ProjectIdValue()).To(Equal("acct-12345"))
	Expect(c.ProviderType()).To(Equal(ProviderType))
	Expect(c.ProviderType()).To(Equal("cloudflare"))
}

func TestAuthConfig_ZeroValues(t *testing.T) {
	RegisterTestingT(t)

	c := &AuthConfig{}
	Expect(c.CredentialsValue()).To(Equal(""))
	Expect(c.ProjectIdValue()).To(Equal(""))
	// ProviderType is invariant regardless of state.
	Expect(c.ProviderType()).To(Equal("cloudflare"))
}

func TestRegistrarConfig_DnsRecords(t *testing.T) {
	RegisterTestingT(t)

	records := []api.DnsRecord{
		{Name: "@", Type: "A", Value: "1.2.3.4"},
		{Name: "www", Type: "CNAME", Value: "example.com"},
	}
	r := &RegistrarConfig{
		ZoneName: "example.com",
		Records:  records,
	}

	got := r.DnsRecords()
	Expect(got).To(HaveLen(2))
	Expect(got).To(Equal(records))
}

func TestRegistrarConfig_DnsRecords_Empty(t *testing.T) {
	RegisterTestingT(t)

	r := &RegistrarConfig{ZoneName: "empty.example.com"}
	Expect(r.DnsRecords()).To(BeEmpty())
}

func TestProviderConstants(t *testing.T) {
	RegisterTestingT(t)

	// Both constants are the config-parsing contract surface.
	Expect(ProviderType).To(Equal("cloudflare"))
	Expect(RegistrarType).To(Equal("cloudflare"))
}

func TestReadRegistrarConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"credentials": "cf-token",
			"accountId":   "acct-12345",
			"zoneName":    "example.com",
			"dnsRecords": []any{
				map[string]any{"name": "www", "type": "CNAME", "value": "example.com"},
			},
		}}
		out, err := ReadRegistrarConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		rc, ok := out.Config.(*RegistrarConfig)
		Expect(ok).To(BeTrue())
		Expect(rc.ZoneName).To(Equal("example.com"))
		Expect(rc.AccountId).To(Equal("acct-12345"))
		Expect(rc.CredentialsValue()).To(Equal("cf-token"))
		Expect(rc.DnsRecords()).To(HaveLen(1))
		Expect(rc.DnsRecords()[0].Type).To(Equal("CNAME"))

		var _ api.RegistrarConfig = rc
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{"zoneName": []int{1, 2, 3}}}
		_, err := ReadRegistrarConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}
