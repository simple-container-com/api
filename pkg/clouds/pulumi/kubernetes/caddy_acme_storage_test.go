// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func testAcmeStore() *caddyassets.AcmeStorage {
	return &caddyassets.AcmeStorage{
		Bucket:        "sc-acme",
		Endpoint:      "https://storage.yandexcloud.net",
		AccessKey:     "YCAJEaccesskey",
		SecretKey:     "YCNsecretkeyvalue",
		EncryptionKey: strings.Repeat("e", caddyassets.AcmeStorageEncryptionKeyLen),
	}
}

func TestCaddyAcmeStorageBlock(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("no store renders nothing", func(t *testing.T) {
		block, err := caddyAcmeStorageBlock(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{}}, "caddy", "caddy-staging")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(block).To(BeEmpty())

		// A nil CaddyConfig is the shape every lo.FromPtr in this file guards against.
		block, err = caddyAcmeStorageBlock(CaddyDeployment{}, "caddy", "caddy-staging")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(block).To(BeEmpty())
	})

	t.Run("renders at the global-options indent, with no credential in it", func(t *testing.T) {
		store := testAcmeStore()
		block, err := caddyAcmeStorageBlock(
			CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{AcmeStorage: store}}, "caddy", "caddy-staging",
		)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(block).To(ContainSubstring(caddyGlobalOptsIndent + "storage s3 {"))
		g.Expect(block).To(ContainSubstring(strings.Repeat(caddyGlobalOptsIndent, 2) + "bucket sc-acme"))
		// Trailing newline trimmed: BuildCaddyfileGlobalOptions joins blocks with "\n"
		// and a blank line in a generated global block reads as a bug.
		g.Expect(block).To(HaveSuffix(caddyGlobalOptsIndent + "}"))

		// The whole reason the credentials go through a Secret.
		g.Expect(block).ToNot(ContainSubstring(store.AccessKey))
		g.Expect(block).ToNot(ContainSubstring(store.SecretKey))
		g.Expect(block).ToNot(ContainSubstring(store.EncryptionKey))

		// Every name the block asks Caddy to substitute must be one the deployment
		// actually delivers. These are the two halves of the same contract, and
		// nothing else connects them.
		for name := range store.CredentialEnv() {
			g.Expect(block).To(ContainSubstring("{$"+name+"}"), "placeholder for %s", name)
		}
		g.Expect(strings.Count(block, "{$")).To(Equal(len(store.CredentialEnv())))

		// It composes with the other global-options block at the same indent.
		full, err := BuildCaddyfileGlobalOptions(block, "  servers {\n    trusted_proxies static 10.0.0.0/8\n  }", "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(full).To(HavePrefix("{\n"))
		g.Expect(full).To(HaveSuffix("\n}"))
		g.Expect(full).ToNot(ContainSubstring("\n\n"))
	})

	t.Run("validation failures name the operator-facing field", func(t *testing.T) {
		_, err := caddyAcmeStorageBlock(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			AcmeStorage: &caddyassets.AcmeStorage{Bucket: "sc-acme", Region: "ru-central1", AccessKey: "only-one-half"},
		}}, "caddy", "caddy-staging")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("`caddy.acmeStorage.accessKey`"))

		// Set but bucketless is still refused rather than quietly skipped: it is the
		// shape an operator reaches by commenting out one line, and serving without
		// the shared store is the failure the field exists to prevent.
		_, err = caddyAcmeStorageBlock(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			AcmeStorage: &caddyassets.AcmeStorage{Region: "ru-central1"},
		}}, "caddy", "caddy-staging")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("`caddy.acmeStorage` is set without `bucket`"))
	})

	t.Run("refused on a cluster that renders its own storage block", func(t *testing.T) {
		// GKE: gcp/gke_autopilot.go provisions a GCS bucket and puts `storage gcs`
		// into CaddyfilePrefixOut. Caddy accepts one `storage` directive, so SC would
		// have to drop one of the two silently — and the one it dropped would be the
		// one the operator was reading.
		_, err := caddyAcmeStorageBlock(CaddyDeployment{
			CaddyConfig:        &k8s.CaddyConfig{AcmeStorage: testAcmeStore()},
			CaddyfilePrefixOut: sdk.String("{\n  storage gcs {\n    bucket-name b\n  }\n}").ToStringOutput(),
		}, "caddy", "caddy-staging")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("already provisions its own certificate store"))
	})
}

// The exported caddy config is a PLAIN stack output read by every child stack. It was
// credential-free until `acmeStorage` existed; this is what keeps it that way.
func TestExportedCaddyConfigStripsStoreCredentials(t *testing.T) {
	g := NewGomegaWithT(t)

	store := testAcmeStore()
	deployment := CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{AcmeStorage: store}}

	exported := exportedCaddyConfig(deployment)
	g.Expect(exported.AcmeStorage.AccessKey).To(BeEmpty())
	g.Expect(exported.AcmeStorage.SecretKey).To(BeEmpty())
	g.Expect(exported.AcmeStorage.EncryptionKey).To(BeEmpty())
	// Not secret, and what a reader of the output needs to know which store is in use.
	g.Expect(exported.AcmeStorage.Bucket).To(Equal("sc-acme"))

	// The live config must be untouched — the deployment still has to authenticate.
	// AcmeStorage is a pointer, so stripping in place would have disabled the store it
	// was configuring, one apply after the export looked correct.
	g.Expect(store.AccessKey).To(Equal("YCAJEaccesskey"))
	g.Expect(store.SecretKey).To(Equal("YCNsecretkeyvalue"))
	g.Expect(deployment.CaddyConfig.AcmeStorage.SecretKey).To(Equal("YCNsecretkeyvalue"))

	// And the pre-existing behaviour it rides on: a derived name is exported so
	// children patch the right deployment.
	named := exportedCaddyConfig(CaddyDeployment{
		CaddyConfig: &k8s.CaddyConfig{AcmeStorage: testAcmeStore()},
		ObjectsName: "caddy-custom--staging",
	})
	g.Expect(lo.FromPtr(named.DeploymentName)).To(Equal("caddy-custom--staging"))
	g.Expect(named.AcmeStorage.SecretKey).To(BeEmpty())
}
