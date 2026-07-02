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

	t.Run("array is sorted by flag name for deterministic diffs", func(t *testing.T) {
		RegisterTestingT(t)
		arr := toDatabaseFlagArray(map[string]string{
			"max_connections":             "200",
			"cloudsql.iam_authentication": "on",
			"log_min_duration_statement":  "500",
		})
		Expect(arr).To(HaveLen(3))
		var names []string
		for _, f := range arr {
			args, ok := f.(sql.DatabaseInstanceSettingsDatabaseFlagArgs)
			Expect(ok).To(BeTrue())
			names = append(names, string(args.Name.(sdk.String)))
		}
		Expect(names).To(Equal([]string{
			"cloudsql.iam_authentication",
			"log_min_duration_statement",
			"max_connections",
		}))
	})
}
