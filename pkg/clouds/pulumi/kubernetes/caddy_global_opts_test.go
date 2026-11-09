// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func TestBuildTrustedProxiesBlock(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("empty proxies returns empty string", func(t *testing.T) {
		block, err := BuildTrustedProxiesBlock(k8s.CaddyConfig{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(block).To(BeEmpty())
	})

	t.Run("valid CIDRs produce servers block", func(t *testing.T) {
		block, err := BuildTrustedProxiesBlock(k8s.CaddyConfig{
			TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12"},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(block).To(ContainSubstring("trusted_proxies static 10.0.0.0/8 172.16.0.0/12"))
		g.Expect(block).To(ContainSubstring("servers {"))
	})

	t.Run("single IP is valid", func(t *testing.T) {
		block, err := BuildTrustedProxiesBlock(k8s.CaddyConfig{
			TrustedProxies: []string{"10.0.0.1"},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(block).To(ContainSubstring("10.0.0.1"))
	})

	t.Run("invalid CIDR returns error", func(t *testing.T) {
		_, err := BuildTrustedProxiesBlock(k8s.CaddyConfig{
			TrustedProxies: []string{"not-a-cidr"},
		})
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid trusted proxy entry"))
	})
}

func TestBuildCaddyfileGlobalOptions(t *testing.T) {
	g := NewGomegaWithT(t)

	storageBlock := "  storage gcs {\n    bucket-name test-bucket\n  }"
	trustedBlock := "  servers {\n    trusted_proxies static 10.0.0.0/8\n  }"

	t.Run("storage only matches previous format", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions(storageBlock, "", "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("{\n  storage gcs {\n    bucket-name test-bucket\n  }\n}"))
	})

	t.Run("storage plus trusted proxies", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions(storageBlock, trustedBlock, "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(ContainSubstring("storage gcs"))
		g.Expect(result).To(ContainSubstring("trusted_proxies static"))
		g.Expect(result).To(HavePrefix("{"))
		g.Expect(result).To(HaveSuffix("}"))
	})

	t.Run("with user prefix appended after", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions(storageBlock, "", "import custom")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(ContainSubstring("storage gcs"))
		g.Expect(result).To(ContainSubstring("import custom"))
	})

	t.Run("empty everything returns empty", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions("", "", "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeEmpty())
	})

	t.Run("only trusted proxies no storage", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions("", trustedBlock, "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(ContainSubstring("trusted_proxies"))
		g.Expect(result).To(HavePrefix("{"))
	})

	// The defect. A Caddyfile accepts one keyless block and it must be first; two is a
	// config-load failure, so the edge never starts. `caddyfilePrefix` is documented as
	// the place to put storage configuration, which IS a global option, so the obvious
	// thing to write there collided with SC's own block.
	t.Run("a user global block is merged, not appended", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions(storageBlock, trustedBlock, "{\n  email ops@example.com\n}")
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(strings.Count(result, "{\n")).To(Equal(3), "one outer block, two nested: %s", result)
		g.Expect(result).To(HavePrefix("{\n"))
		g.Expect(result).To(HaveSuffix("\n}"))
		g.Expect(result).To(ContainSubstring("storage gcs"))
		g.Expect(result).To(ContainSubstring("trusted_proxies static"))
		g.Expect(result).To(ContainSubstring("email ops@example.com"))
	})

	t.Run("a user global block is used as-is when SC has none", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions("", "", "{\n  email ops@example.com\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("{\n  email ops@example.com\n}"))
	})

	t.Run("what follows the user's block stays below the global block", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions(storageBlock, "", strings.Join([]string{
			"{",
			"  email ops@example.com",
			"}",
			"",
			"(logjson) {",
			"  log {",
			"    format json",
			"  }",
			"}",
		}, "\n"))
		g.Expect(err).ToNot(HaveOccurred())

		// A snippet definition is NOT a global option: merging it in would make
		// `(logjson)` a directive inside the global block, which Caddy rejects.
		globalEnd := strings.Index(result, "\n}\n")
		g.Expect(globalEnd).To(BeNumerically(">", 0))
		g.Expect(result[:globalEnd]).To(ContainSubstring("email ops@example.com"))
		g.Expect(result[:globalEnd]).ToNot(ContainSubstring("(logjson)"))
		g.Expect(result[globalEnd:]).To(ContainSubstring("(logjson) {"))
	})

	t.Run("a prefix whose block opens below a comment is still merged", func(t *testing.T) {
		// The shape that makes a naive HasPrefix("{") test wrong, and the one an
		// operator writes: they explain the block above it.
		result, err := BuildCaddyfileGlobalOptions(storageBlock, "",
			"# our ACME account\n{\n  email ops@example.com\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(strings.Count(result, "{\n")).To(Equal(2))
		g.Expect(result).To(HavePrefix("# our ACME account\n"))
		g.Expect(result).To(ContainSubstring("email ops@example.com"))
		g.Expect(result).To(ContainSubstring("storage gcs"))
	})

	t.Run("a prefix with no global block at all is left alone", func(t *testing.T) {
		result, err := BuildCaddyfileGlobalOptions("", "", "(logjson) {\n  log\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("(logjson) {\n  log\n}"))
	})

	t.Run("an unclosed brace is refused, not rendered", func(t *testing.T) {
		// Rendering it would swallow everything SC puts below it, and the only symptom
		// is an edge that never becomes ready.
		_, err := BuildCaddyfileGlobalOptions(storageBlock, "", "{\n  email ops@example.com")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("`caddy.caddyfilePrefix` opens a block that is never closed"))
	})
}

func TestSplitCaddyfilePrefix(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("braces inside a quoted token are not structure", func(t *testing.T) {
		// `header` with a placeholder-looking literal: counting this `}` would end the
		// block early and leave the real contents in Rest.
		parts, err := SplitCaddyfilePrefix("{\n  servers {\n    name \"a{b}c\"\n  }\n}\n\nexample.com {\n  respond 200\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts.Globals).To(ContainSubstring(`name "a{b}c"`))
		g.Expect(parts.Rest).To(Equal("example.com {\n  respond 200\n}"))
	})

	t.Run("a commented-out brace is not structure", func(t *testing.T) {
		parts, err := SplitCaddyfilePrefix("{\n  # }\n  email ops@example.com\n}\n\nexample.com {\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts.Globals).To(ContainSubstring("email ops@example.com"))
		g.Expect(parts.Rest).To(HavePrefix("example.com {"))
	})

	t.Run("a mid-token hash is literal, not a comment", func(t *testing.T) {
		// Caddy's lexer only starts a comment at the start of a token. Treating any `#`
		// as one would comment out the rest of this line INCLUDING the closing brace,
		// and the split would read past the end of the block.
		parts, err := SplitCaddyfilePrefix("{\n  email ops#1@example.com\n}\n\nexample.com {\n}")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts.Globals).To(Equal("  email ops#1@example.com"))
		g.Expect(parts.Rest).To(HavePrefix("example.com {"))
	})

	t.Run("an escaped quote does not end the quoted token", func(t *testing.T) {
		parts, err := SplitCaddyfilePrefix(`{` + "\n" + `  name "a\"}b"` + "\n" + `}`)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts.Globals).To(Equal(`  name "a\"}b"`))
		g.Expect(parts.Rest).To(BeEmpty())
	})

	t.Run("empty prefix", func(t *testing.T) {
		parts, err := SplitCaddyfilePrefix("")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts).To(Equal(CaddyfilePrefixParts{}))
	})

	t.Run("comments only", func(t *testing.T) {
		parts, err := SplitCaddyfilePrefix("# nothing to see\n")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parts.Globals).To(BeEmpty())
		g.Expect(parts.Rest).To(Equal("# nothing to see"))
	})
}

// Every edge whose cloud does not compute a prefix of its own goes through here, and
// until 2026-11-08 none of them rendered one at all (isPulumiOutputSet was a constant
// true, so the dynamic branch always won with an unset output).
func TestStaticCaddyfilePrefix(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("nil config", func(t *testing.T) {
		prefix, err := staticCaddyfilePrefix(CaddyDeployment{}, "caddy", "caddy-staging")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(prefix).To(BeEmpty())
	})

	t.Run("trusted proxies and the user's own block", func(t *testing.T) {
		prefix, err := staticCaddyfilePrefix(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			TrustedProxies:  []string{"10.0.0.0/8"},
			CaddyfilePrefix: lo.ToPtr("{\n  email ops@example.com\n}"),
		}}, "caddy", "caddy-staging")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(prefix).To(ContainSubstring("trusted_proxies static 10.0.0.0/8"))
		g.Expect(prefix).To(ContainSubstring("email ops@example.com"))
		g.Expect(strings.Count(prefix, "{\n")).To(Equal(2))
	})

	t.Run("the certificate store joins the same global block", func(t *testing.T) {
		// One function owns everything above the first site block, so a store, a
		// trusted-proxies block and the operator's own globals have to come out as ONE
		// keyless block or the edge fails to load its config.
		prefix, err := staticCaddyfilePrefix(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			AcmeStorage:     testAcmeStore(),
			TrustedProxies:  []string{"10.0.0.0/8"},
			CaddyfilePrefix: lo.ToPtr("{\n  email ops@example.com\n}"),
		}}, "caddy", "caddy-staging")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(prefix).To(HavePrefix("{\n"))
		g.Expect(prefix).To(HaveSuffix("\n}"))
		g.Expect(prefix).To(ContainSubstring("storage s3 {"))
		g.Expect(prefix).To(ContainSubstring("trusted_proxies static 10.0.0.0/8"))
		g.Expect(prefix).To(ContainSubstring("email ops@example.com"))
		// `storage s3 {`, `servers {` and the outer block — the user's globals are
		// merged in as bare directives, not as a block of their own, and the autosave
		// guard is the bare `persist_config off` rather than a block.
		g.Expect(strings.Count(prefix, "{\n")).To(Equal(3))
	})

	t.Run("an invalid store is refused here, not inside a Pulumi apply", func(t *testing.T) {
		store := testAcmeStore()
		store.EncryptionKey = "too-short"
		_, err := staticCaddyfilePrefix(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			AcmeStorage: store,
		}}, "caddy", "caddy-staging")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("`caddy.acmeStorage.encryptionKey`"))
	})

	t.Run("an invalid CIDR is refused instead of silently dropping the block", func(t *testing.T) {
		// It used to be `trustedBlock, _ :=` inside a closure that could not return an
		// error, so a typo'd CIDR produced an edge that trusts no proxy and logs the
		// load balancer's address as every client's.
		_, err := staticCaddyfilePrefix(CaddyDeployment{CaddyConfig: &k8s.CaddyConfig{
			TrustedProxies: []string{"10.0.0.0/8", "no-a-cidr"},
		}}, "caddy", "caddy-staging")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid trusted proxy entry"))
	})
}

// isPulumiOutputSet answered `true` for every StringOutput in existence, because
// ElementType() is a method on the value type returning a package-level reflect.Type.
func TestIsPulumiOutputSet(t *testing.T) {
	g := NewGomegaWithT(t)

	var unset sdk.StringOutput
	g.Expect(isPulumiOutputSet(unset)).To(BeFalse(), "the zero value of a struct field is not a value")
	g.Expect(isPulumiOutputSet(sdk.String("").ToStringOutput())).To(BeTrue(), "a deliberate empty string is")
	g.Expect(isPulumiOutputSet(sdk.String("{\n  storage gcs {\n  }\n}").ToStringOutput())).To(BeTrue())
	// What an unset output actually is, and why the branch mattered: not "", but an
	// UNKNOWN value — which is what every non-GKE edge was handed as CADDYFILE_PREFIX.
	g.Expect(unset.ElementType()).ToNot(BeNil(), "the check this function used to make")
}
