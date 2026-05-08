package aws

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

// Tests for the opt-in `StorageEncrypted` field on MysqlConfig /
// PostgresConfig. Three states matter:
//
//   1. omitted from YAML / JSON → field stays nil → `lo.FromPtr(nil)`
//      collapses to `false`, which preserves pre-2026.5 SC behaviour
//      for stacks created without the field.
//   2. explicit `true` → encrypted instance.
//   3. explicit `false` → still unencrypted (caller asked for it
//      explicitly; we don't second-guess).
//
// The actual replacement-safety guarantee for existing instances comes
// from `pulumi.IgnoreChanges([]{"storageEncrypted"})` on the resource
// opts (see pkg/clouds/pulumi/aws/rds_{mysql,postgres}.go) and is
// covered by integration / e2e tests, not here.

func TestReadRdsMysqlConfig_StorageEncrypted(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *api.Config
		wantSet bool
		wantVal bool
	}{
		{
			name: "omitted → nil (legacy default, encryption off)",
			config: &api.Config{Config: map[string]any{
				"instanceClass": "db.t3.micro",
				"engineVersion": "8.0",
				"username":      "root",
				"password":      "root",
			}},
			wantSet: false,
		},
		{
			name: "explicit true → encrypted",
			config: &api.Config{Config: map[string]any{
				"instanceClass":    "db.t3.micro",
				"engineVersion":    "8.0",
				"username":         "root",
				"password":         "root",
				"storageEncrypted": true,
			}},
			wantSet: true,
			wantVal: true,
		},
		{
			name: "explicit false → still unencrypted",
			config: &api.Config{Config: map[string]any{
				"instanceClass":    "db.t3.micro",
				"engineVersion":    "8.0",
				"username":         "root",
				"password":         "root",
				"storageEncrypted": false,
			}},
			wantSet: true,
			wantVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			out, err := ReadRdsMysqlConfig(tt.config)
			Expect(err).To(BeNil())
			cfg, ok := out.Config.(*MysqlConfig)
			Expect(ok).To(BeTrue())

			if !tt.wantSet {
				Expect(cfg.StorageEncrypted).To(BeNil(),
					"unset field should round-trip as nil so `lo.FromPtr` resolves to false")
			} else {
				Expect(cfg.StorageEncrypted).ToNot(BeNil())
				Expect(*cfg.StorageEncrypted).To(Equal(tt.wantVal))
			}

			// `lo.FromPtr(nil)` is `false` — explicitly assert the
			// resolved Pulumi flag matches the documented contract.
			resolved := lo.FromPtr(cfg.StorageEncrypted)
			expected := tt.wantSet && tt.wantVal
			Expect(resolved).To(Equal(expected),
				"resolved flag passed to `rds.NewInstance` must match nil → false / true → true / false → false")
		})
	}
}

func TestReadRdsPostgresConfig_StorageEncrypted(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *api.Config
		wantSet bool
		wantVal bool
	}{
		{
			name: "omitted → nil (legacy default, encryption off)",
			config: &api.Config{Config: map[string]any{
				"instanceClass": "db.t3.micro",
				"engineVersion": "16",
				"username":      "postgres",
				"password":      "postgres",
			}},
			wantSet: false,
		},
		{
			name: "explicit true → encrypted",
			config: &api.Config{Config: map[string]any{
				"instanceClass":    "db.t3.micro",
				"engineVersion":    "16",
				"username":         "postgres",
				"password":         "postgres",
				"storageEncrypted": true,
			}},
			wantSet: true,
			wantVal: true,
		},
		{
			name: "explicit false → still unencrypted",
			config: &api.Config{Config: map[string]any{
				"instanceClass":    "db.t3.micro",
				"engineVersion":    "16",
				"username":         "postgres",
				"password":         "postgres",
				"storageEncrypted": false,
			}},
			wantSet: true,
			wantVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			out, err := ReadRdsPostgresConfig(tt.config)
			Expect(err).To(BeNil())
			cfg, ok := out.Config.(*PostgresConfig)
			Expect(ok).To(BeTrue())

			if !tt.wantSet {
				Expect(cfg.StorageEncrypted).To(BeNil(),
					"unset field should round-trip as nil so `lo.FromPtr` resolves to false")
			} else {
				Expect(cfg.StorageEncrypted).ToNot(BeNil())
				Expect(*cfg.StorageEncrypted).To(Equal(tt.wantVal))
			}

			resolved := lo.FromPtr(cfg.StorageEncrypted)
			expected := tt.wantSet && tt.wantVal
			Expect(resolved).To(Equal(expected),
				"resolved flag passed to `rds.NewInstance` must match nil → false / true → true / false → false")
		})
	}
}
