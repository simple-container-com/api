// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package helpers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

// TestConfigForRegion exercises the per-region aws.Config cache. config.Load
// DefaultConfig only *builds* the config (credential/region providers are
// resolved lazily on first AWS API call), so this makes no network request.
// AWS_EC2_METADATA_DISABLED keeps even the lazy resolver from probing IMDS.
func TestConfigForRegion(t *testing.T) {
	RegisterTestingT(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIATESTTESTTESTTEST")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secretsecretsecretsecretsecretsecretsecr")
	ctx := context.Background()

	t.Run("explicit region is applied and cached", func(t *testing.T) {
		RegisterTestingT(t)
		// Use a distinct region so this assertion is robust to whatever other
		// tests / host env have already populated in the shared cache.
		cfg, err := configForRegion(ctx, "ap-southeast-2")
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.Region).To(Equal("ap-southeast-2"))

		// Second call must come from the cache and yield the same region.
		cfg2, err := configForRegion(ctx, "ap-southeast-2")
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg2.Region).To(Equal("ap-southeast-2"))

		// And the package-level cache now holds the entry under the region key.
		configCacheMu.Lock()
		_, ok := configCache["ap-southeast-2"]
		configCacheMu.Unlock()
		Expect(ok).To(BeTrue())
	})

	t.Run("empty region resolves under the __default__ sentinel key", func(t *testing.T) {
		RegisterTestingT(t)
		// With AWS_REGION unset, an empty-region call falls through to the SDK's
		// default region resolution; the entry is cached under "__default__".
		t.Setenv("AWS_REGION", "us-east-1")
		_, err := configForRegion(ctx, "")
		Expect(err).ToNot(HaveOccurred())

		configCacheMu.Lock()
		_, ok := configCache["__default__"]
		configCacheMu.Unlock()
		Expect(ok).To(BeTrue())
	})

	t.Run("cached default is returned on the second empty-region call", func(t *testing.T) {
		RegisterTestingT(t)
		// Prime then re-fetch; the cache-hit branch returns the stored value.
		first, err := configForRegion(ctx, "")
		Expect(err).ToNot(HaveOccurred())
		second, err := configForRegion(ctx, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(second.Region).To(Equal(first.Region))
	})
}
