// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package caddy

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

func fullStore() *AcmeStorage {
	return &AcmeStorage{
		Bucket:        "sc-acme",
		Endpoint:      "https://storage.yandexcloud.net",
		AccessKey:     "YCAJEaccesskey",
		SecretKey:     "YCNsecretkeyvalue",
		EncryptionKey: strings.Repeat("e", AcmeStorageEncryptionKeyLen),
	}
}

func renderStore(s *AcmeStorage, indent string) string {
	b := &strings.Builder{}
	WriteStorageBlock(b, s, indent, "`docker logs sc-caddy`")
	return b.String()
}

// The placeholders the block asks Caddy to substitute and the names the caller is told
// to deliver are two halves of one contract, and nothing but this test connects them.
// They are in different files on both edges (a Caddyfile and an env file; a Caddyfile
// and a Secret), and a disagreement between them presents as a credential that is
// present everywhere a human would look and empty where Caddy reads it.
func TestCredentialEnvMatchesTheRenderedPlaceholders(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("every delivered name is a placeholder, and vice versa", func(t *testing.T) {
		store := fullStore()
		block := renderStore(store, "\t")
		env := store.CredentialEnv()

		g.Expect(env).To(HaveLen(3))
		for _, name := range lo.Keys(env) {
			g.Expect(block).To(ContainSubstring(fmt.Sprintf("{$%s}", name)), "placeholder for %s", name)
		}
		// No placeholder is rendered that nothing delivers: a `{$NAME}` with no value
		// expands to the empty string rather than failing, so Caddy would authenticate
		// with "" and report a credentials error.
		g.Expect(strings.Count(block, "{$")).To(Equal(len(env)))
	})

	t.Run("no credential VALUE is ever rendered", func(t *testing.T) {
		store := fullStore()
		block := renderStore(store, "\t")

		g.Expect(block).ToNot(ContainSubstring(store.AccessKey))
		g.Expect(block).ToNot(ContainSubstring(store.SecretKey))
		g.Expect(block).ToNot(ContainSubstring(store.EncryptionKey))
	})

	t.Run("access and secret are delivered as a pair", func(t *testing.T) {
		// Validate refuses one without the other; this is the shape the module cares
		// about once the config is legal. A secret with no access key would otherwise
		// be delivered alone and silently unused.
		env := (&AcmeStorage{Bucket: "b", Region: "us-east-1"}).CredentialEnv()
		g.Expect(env).To(BeEmpty())

		env = (&AcmeStorage{Bucket: "b", Region: "us-east-1", AccessKey: "a", SecretKey: "s"}).CredentialEnv()
		g.Expect(env).To(HaveKeyWithValue(EnvAcmeS3AccessKey, "a"))
		g.Expect(env).To(HaveKeyWithValue(EnvAcmeS3SecretKey, "s"))
		g.Expect(env).ToNot(HaveKey(EnvAcmeS3EncryptionKey))

		// Nil-safe: both edges call this through a possibly-absent store.
		var absent *AcmeStorage
		g.Expect(absent.CredentialEnv()).To(BeEmpty())
	})
}

// The module's parser reads each subdirective with `if !d.Args(&value) { continue }`, so
// a bare boolean is not an error and is not true — it is silently skipped, and the only
// symptom is every request to the store failing.
func TestWriteStorageBlockNeverRendersABareBoolean(t *testing.T) {
	g := NewGomegaWithT(t)

	for _, store := range []*AcmeStorage{
		{Bucket: "b", Region: "us-east-1"},
		{Bucket: "b", Endpoint: "http://10.0.0.4:9000"},
		{Bucket: "b", Endpoint: "https://s3.example.com", PathStyle: lo.ToPtr(false), Insecure: true},
		fullStore(),
	} {
		for _, line := range strings.Split(renderStore(store, "\t"), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
				continue
			}
			switch fields[0] {
			case "use_path_style", "insecure":
				g.Expect(fields).To(HaveLen(2), "bare boolean %q is skipped by the parser", line)
				g.Expect(fields[1]).To(BeElementOf("true", "false"))
			}
		}
	}
}

// Both edges nest their global options differently, and the block has to come out at the
// caller's own indent unit — a tab on the vm path, two spaces next to the Kubernetes
// edge's `servers { trusted_proxies ... }`. Hardcoding either produces a file that parses
// and that a reviewer mistrusts.
func TestWriteStorageBlockIndentsInTheCallersUnit(t *testing.T) {
	g := NewGomegaWithT(t)

	for _, indent := range []string{"\t", "  ", ""} {
		block := renderStore(fullStore(), indent)

		g.Expect(block).To(ContainSubstring(indent+"storage s3 {\n"), "outer line at indent %q", indent)
		g.Expect(block).To(ContainSubstring(strings.Repeat(indent, 2)+"bucket sc-acme\n"), "inner line at indent %q", indent)
		// Not "ends in a newline by accident": the vm path appends more directives
		// straight after this block, and the Kubernetes path trims it precisely because
		// it is there.
		g.Expect(block).To(HaveSuffix(indent + "}\n"))
	}
}

func TestAcmeStorageDefaults(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("path style follows the endpoint", func(t *testing.T) {
		// The useful default, and the one that differs from the module's: virtual-host
		// addressing needs the bucket to resolve as a DNS label, which minio never
		// serves.
		g.Expect((&AcmeStorage{Bucket: "b", Endpoint: "http://10.0.0.4:9000"}).UsePathStyle()).To(BeTrue())
		g.Expect((&AcmeStorage{Bucket: "b", Region: "eu-west-1"}).UsePathStyle()).To(BeFalse())
		g.Expect((&AcmeStorage{Bucket: "b", Endpoint: "http://m:9000", PathStyle: lo.ToPtr(false)}).UsePathStyle()).To(BeFalse())
		g.Expect((&AcmeStorage{Bucket: "b", Region: "eu-west-1", PathStyle: lo.ToPtr(true)}).UsePathStyle()).To(BeTrue())
	})

	t.Run("prefix and region", func(t *testing.T) {
		g.Expect((&AcmeStorage{}).StoragePrefix()).To(Equal(DefaultAcmeStoragePrefix))
		g.Expect((&AcmeStorage{Prefix: "fleet-a"}).StoragePrefix()).To(Equal("fleet-a"))
		g.Expect((&AcmeStorage{}).StorageRegion()).To(Equal(DefaultAcmeStorageRegion))
		g.Expect((&AcmeStorage{Region: "ru-central1"}).StorageRegion()).To(Equal("ru-central1"))
	})

	t.Run("enabled needs a bucket", func(t *testing.T) {
		var absent *AcmeStorage
		g.Expect(absent.Enabled()).To(BeFalse())
		g.Expect((&AcmeStorage{Region: "eu-west-1"}).Enabled()).To(BeFalse())
		g.Expect((&AcmeStorage{Bucket: "b"}).Enabled()).To(BeTrue())
	})
}

// Each refusal has to name something the operator can grep for in the file they are
// holding, and the two edges spell the same struct differently. A hardcoded path would
// send a Kubernetes operator looking for `ingress.acmeStorage`, which their file does
// not contain.
func TestValidateNamesTheCallersField(t *testing.T) {
	g := NewGomegaWithT(t)

	for _, tc := range []struct {
		name  string
		store *AcmeStorage
		// what the message must mention beyond the field path
		mentions string
	}{
		{"no bucket", &AcmeStorage{Region: "eu-west-1"}, "`bucket`"},
		{"access key without secret", &AcmeStorage{Bucket: "b", Region: "eu-west-1", AccessKey: "a"}, "default chain"},
		{"secret without access key", &AcmeStorage{Bucket: "b", Region: "eu-west-1", SecretKey: "s"}, "default chain"},
		{"neither endpoint nor region", &AcmeStorage{Bucket: "b"}, "real AWS S3"},
		{"endpoint without a scheme", &AcmeStorage{Bucket: "b", Endpoint: "storage.yandexcloud.net"}, "no scheme"},
		{
			"wrong-length encryption key",
			&AcmeStorage{Bucket: "b", Region: "eu-west-1", EncryptionKey: "too-short"},
			"secretbox needs exactly 32",
		},
		{"insecure over plain http", &AcmeStorage{Bucket: "b", Endpoint: "http://m:9000", Insecure: true}, "no TLS to skip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, field := range []string{"ingress.acmeStorage", "caddy.acmeStorage"} {
				err := tc.store.Validate(field)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("`" + field))
				g.Expect(err.Error()).To(ContainSubstring(tc.mentions))
			}
		})
	}

	t.Run("accepts the legal shapes", func(t *testing.T) {
		var absent *AcmeStorage
		g.Expect(absent.Validate("ingress.acmeStorage")).ToNot(HaveOccurred())
		g.Expect(fullStore().Validate("caddy.acmeStorage")).ToNot(HaveOccurred())
		// Real AWS with an instance role: no endpoint, no credential, region given.
		g.Expect((&AcmeStorage{Bucket: "b", Region: "eu-west-1"}).Validate("caddy.acmeStorage")).ToNot(HaveOccurred())
		// minio on the fleet's own network, encrypted at rest.
		g.Expect((&AcmeStorage{
			Bucket: "b", Endpoint: "http://10.0.0.4:9000", AccessKey: "a", SecretKey: "s",
			EncryptionKey: strings.Repeat("k", AcmeStorageEncryptionKeyLen),
		}).Validate("ingress.acmeStorage")).ToNot(HaveOccurred())
	})
}

func TestToken(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(Token("sc-acme")).To(Equal("sc-acme"))
	g.Expect(Token("https://storage.yandexcloud.net")).To(Equal("https://storage.yandexcloud.net"))
	// Anything Caddy's parser would read as structure.
	g.Expect(Token("two words")).To(Equal(`"two words"`))
	g.Expect(Token("a{b}")).To(Equal(`"a{b}"`))
	g.Expect(Token(`a"b`)).To(Equal(`"a\"b"`))
}
