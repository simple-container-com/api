// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// TestRegistrarConfigIsZonesAware is what makes a stack able to route a domain to this
// registrar without constructing it first. The interface is optional, so dropping the
// method would not fail to compile — it would silently stop the registrar from ever
// being chosen.
func TestRegistrarConfigIsZonesAware(t *testing.T) {
	RegisterTestingT(t)

	var cfg any = &RegistrarConfig{ZoneName: "simple-forge.ru"}
	aware, ok := cfg.(api.RegistrarZonesAware)
	Expect(ok).To(BeTrue(), "*RegistrarConfig must implement api.RegistrarZonesAware")
	Expect(aware.Zones()).To(Equal([]string{"simple-forge.ru"}))

	Expect((&RegistrarConfig{}).Zones()).To(BeNil(), "an unconfigured registrar claims no zone")
}

// TestEffectiveZoneResourceName covers the trap that cost a live lookup: YC's
// lookup-by-name takes the *resource* name, and the zone serving `simple-forge.ru.` is
// a resource named `simple-forge-ru`.
func TestEffectiveZoneResourceName(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name     string
		cfg      RegistrarConfig
		expected string
	}{
		{"derived from the zone", RegistrarConfig{ZoneName: "simple-forge.ru"}, "simple-forge-ru"},
		{"trailing dot dropped", RegistrarConfig{ZoneName: "simple-forge.ru."}, "simple-forge-ru"},
		{"deeper zone", RegistrarConfig{ZoneName: "dev.simple-forge.ru"}, "dev-simple-forge-ru"},
		{
			"explicit wins",
			RegistrarConfig{ZoneName: "simple-forge.ru", ZoneResourceName: "forge-public"},
			"forge-public",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Expect(tc.cfg.EffectiveZoneResourceName()).To(Equal(tc.expected))
		})
	}
}

// TestEffectiveRecordTtl pins that some TTL is always produced: Yandex Cloud DNS has no
// "automatic" sentinel, so a zero would be sent verbatim and rejected.
func TestEffectiveRecordTtl(t *testing.T) {
	RegisterTestingT(t)

	Expect((&RegistrarConfig{}).EffectiveRecordTtl()).To(Equal(DefaultRecordTtl))
	Expect((&RegistrarConfig{RecordTtl: -1}).EffectiveRecordTtl()).To(Equal(DefaultRecordTtl))
	Expect((&RegistrarConfig{RecordTtl: 60}).EffectiveRecordTtl()).To(Equal(60))
}

func TestRegistrarConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	Expect((&RegistrarConfig{}).Validate()).To(MatchError(ContainSubstring("zoneName is required")))

	noFolder := &RegistrarConfig{ZoneName: "simple-forge.ru"}
	Expect(noFolder.Validate()).To(MatchError(ContainSubstring("requires folderId")))

	byFolder := &RegistrarConfig{ZoneName: "simple-forge.ru"}
	byFolder.FolderID = "b1gfolder"
	Expect(byFolder.Validate()).ToNot(HaveOccurred())

	// An explicit zone id makes the folder unnecessary: nothing has to be searched.
	byZoneID := &RegistrarConfig{ZoneName: "simple-forge.ru", ZoneID: "dns8cs728kosp7ms1s3u"}
	Expect(byZoneID.Validate()).ToNot(HaveOccurred())
}

// TestReadRegistrarConfig walks the path a server.yaml actually takes, so a wrong yaml
// tag is caught here rather than as an empty zone at deploy time.
func TestReadRegistrarConfig(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadRegistrarConfig(&api.Config{Config: map[string]any{
		"folderId":      "b1gfolder",
		"zoneName":      "simple-forge.ru",
		"certificateId": "fpqu9ukb7q53rmraipgs",
		"recordTtl":     600,
		"dnsRecords": []any{
			map[string]any{"name": "txt.simple-forge.ru", "type": "TXT", "value": "hello"},
		},
	}})
	Expect(err).ToNot(HaveOccurred())

	cfg, ok := out.Config.(*RegistrarConfig)
	Expect(ok).To(BeTrue())
	Expect(cfg.FolderID).To(Equal("b1gfolder"))
	Expect(cfg.ZoneName).To(Equal("simple-forge.ru"))
	Expect(cfg.CertificateID).To(Equal("fpqu9ukb7q53rmraipgs"))
	Expect(cfg.EffectiveRecordTtl()).To(Equal(600))
	Expect(cfg.DnsRecords()).To(HaveLen(1))
	Expect(cfg.DnsRecords()[0].Type).To(Equal("TXT"))
}
