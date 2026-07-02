// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func renderedFlags(arr sql.DatabaseInstanceSettingsDatabaseFlagArray) []string {
	var got []string
	for _, f := range arr {
		args := f.(sql.DatabaseInstanceSettingsDatabaseFlagArgs)
		got = append(got, string(args.Name.(sdk.String))+"="+string(args.Value.(sdk.String)))
	}
	return got
}

func TestConfiguredDatabaseFlags(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty config yields no flags", func(t *testing.T) {
		RegisterTestingT(t)
		flags := configuredDatabaseFlags(&gcloud.PostgresGcpCloudsqlConfig{})
		Expect(flags).To(BeEmpty())
		Expect(toDatabaseFlagArray(flags)).To(BeNil())
	})

	t.Run("maxConnections maps to max_connections", func(t *testing.T) {
		RegisterTestingT(t)
		flags := configuredDatabaseFlags(&gcloud.PostgresGcpCloudsqlConfig{
			MaxConnections: lo.ToPtr(200),
		})
		Expect(flags).To(Equal(map[string]string{"max_connections": "200"}))
	})

	t.Run("databaseFlags merge with maxConnections", func(t *testing.T) {
		RegisterTestingT(t)
		flags := configuredDatabaseFlags(&gcloud.PostgresGcpCloudsqlConfig{
			MaxConnections: lo.ToPtr(200),
			DatabaseFlags: map[string]string{
				"cloudsql.iam_authentication": "on",
			},
		})
		Expect(flags).To(Equal(map[string]string{
			"max_connections":             "200",
			"cloudsql.iam_authentication": "on",
		}))
	})

	t.Run("explicit max_connections in databaseFlags wins", func(t *testing.T) {
		RegisterTestingT(t)
		flags := configuredDatabaseFlags(&gcloud.PostgresGcpCloudsqlConfig{
			MaxConnections: lo.ToPtr(200),
			DatabaseFlags:  map[string]string{"max_connections": "500"},
		})
		Expect(flags).To(Equal(map[string]string{"max_connections": "500"}))
	})
}

func TestToDatabaseFlagArray(t *testing.T) {
	RegisterTestingT(t)

	t.Run("sorted by flag name for deterministic diffs", func(t *testing.T) {
		RegisterTestingT(t)
		got := renderedFlags(toDatabaseFlagArray(map[string]string{
			"max_connections":             "200",
			"cloudsql.iam_authentication": "on",
			"log_min_duration_statement":  "500",
		}))
		Expect(got).To(Equal([]string{
			"cloudsql.iam_authentication=on",
			"log_min_duration_statement=500",
			"max_connections=200",
		}))
	})
}

func TestMergeDatabaseFlags(t *testing.T) {
	RegisterTestingT(t)

	t.Run("preserves unmanaged flags, overrides configured, sorts all", func(t *testing.T) {
		RegisterTestingT(t)
		existing := []sql.GetDatabaseInstanceSettingDatabaseFlag{
			{Name: "max_connections", Value: "100"},
			{Name: "log_connections", Value: "on"},
		}
		got := renderedFlags(mergeDatabaseFlags(existing, map[string]string{
			"max_connections":             "200",
			"cloudsql.iam_authentication": "on",
		}))
		Expect(got).To(Equal([]string{
			"cloudsql.iam_authentication=on",
			"log_connections=on",
			"max_connections=200",
		}))
	})
}

func TestAdoptIgnoreChanges(t *testing.T) {
	RegisterTestingT(t)

	t.Run("no databaseFlags keeps the legacy no-op on settings.databaseFlags", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(adoptIgnoreChanges(nil)).To(ContainElement("settings.databaseFlags"))
	})

	t.Run("explicit databaseFlags un-ignores settings.databaseFlags", func(t *testing.T) {
		RegisterTestingT(t)
		got := adoptIgnoreChanges(map[string]string{"cloudsql.iam_authentication": "on"})
		Expect(got).ToNot(ContainElement("settings.databaseFlags"))
		// The rest of the protection list must stay intact.
		Expect(got).To(ContainElement("settings.backupConfiguration"))
	})
}
