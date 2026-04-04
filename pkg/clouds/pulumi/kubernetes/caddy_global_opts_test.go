package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"

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
		result := BuildCaddyfileGlobalOptions(storageBlock, "", "")
		g.Expect(result).To(Equal("{\n  storage gcs {\n    bucket-name test-bucket\n  }\n}"))
	})

	t.Run("storage plus trusted proxies", func(t *testing.T) {
		result := BuildCaddyfileGlobalOptions(storageBlock, trustedBlock, "")
		g.Expect(result).To(ContainSubstring("storage gcs"))
		g.Expect(result).To(ContainSubstring("trusted_proxies static"))
		g.Expect(result).To(HavePrefix("{"))
		g.Expect(result).To(HaveSuffix("}"))
	})

	t.Run("with user prefix appended after", func(t *testing.T) {
		result := BuildCaddyfileGlobalOptions(storageBlock, "", "import custom")
		g.Expect(result).To(ContainSubstring("storage gcs"))
		g.Expect(result).To(ContainSubstring("import custom"))
	})

	t.Run("empty everything returns empty", func(t *testing.T) {
		result := BuildCaddyfileGlobalOptions("", "", "")
		g.Expect(result).To(BeEmpty())
	})

	t.Run("only trusted proxies no storage", func(t *testing.T) {
		result := BuildCaddyfileGlobalOptions("", trustedBlock, "")
		g.Expect(result).To(ContainSubstring("trusted_proxies"))
		g.Expect(result).To(HavePrefix("{"))
	})
}
