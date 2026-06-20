package k8s

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// kubeconfigYAML returns a representative inline kubeconfig string used across
// the config-reader tests.
const kubeconfigYAML = "apiVersion: v1\nclusters: []\n"

// TestKubernetesConfig_Getters exercises the trivial provider/credential getters
// on KubernetesConfig.
func TestKubernetesConfig_Getters(t *testing.T) {
	RegisterTestingT(t)

	cfg := &KubernetesConfig{Kubeconfig: kubeconfigYAML}

	Expect(cfg.ProviderType()).To(Equal(ProviderType))
	Expect(cfg.ProviderType()).To(Equal("kubernetes"))
	Expect(cfg.ProjectIdValue()).To(Equal("n/a"))
	Expect(cfg.CredentialsValue()).To(Equal(kubeconfigYAML))
}

// TestKubernetesConfig_CredentialsValue_Empty verifies the credential getter
// faithfully returns an empty kubeconfig rather than substituting a default.
func TestKubernetesConfig_CredentialsValue_Empty(t *testing.T) {
	RegisterTestingT(t)

	cfg := &KubernetesConfig{}
	Expect(cfg.CredentialsValue()).To(Equal(""))
}

func TestReadKubernetesConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("parses kubeconfig field", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{"kubeconfig": kubeconfigYAML}}

		out, err := ReadKubernetesConfig(in)

		Expect(err).ToNot(HaveOccurred())
		kc, ok := out.Config.(*KubernetesConfig)
		Expect(ok).To(BeTrue())
		Expect(kc.Kubeconfig).To(Equal(kubeconfigYAML))
	})

	t.Run("absent kubeconfig yields empty string", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{}}

		out, err := ReadKubernetesConfig(in)

		Expect(err).ToNot(HaveOccurred())
		kc, ok := out.Config.(*KubernetesConfig)
		Expect(ok).To(BeTrue())
		Expect(kc.Kubeconfig).To(Equal(""))
	})
}

func TestReadTemplateConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("parses cloudrun template fields", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{
			"kubeconfig":    kubeconfigYAML,
			"caddyResource": "my-caddy",
			"useSSL":        false,
		}}

		out, err := ReadTemplateConfig(in)

		Expect(err).ToNot(HaveOccurred())
		tpl, ok := out.Config.(*CloudrunTemplate)
		Expect(ok).To(BeTrue())
		Expect(tpl.Kubeconfig).To(Equal(kubeconfigYAML))
		Expect(tpl.CaddyResource).ToNot(BeNil())
		Expect(*tpl.CaddyResource).To(Equal("my-caddy"))
		Expect(tpl.UseSSL).ToNot(BeNil())
		Expect(*tpl.UseSSL).To(BeFalse())
	})

	t.Run("omitted optional fields are nil", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{"kubeconfig": kubeconfigYAML}}

		out, err := ReadTemplateConfig(in)

		Expect(err).ToNot(HaveOccurred())
		tpl, ok := out.Config.(*CloudrunTemplate)
		Expect(ok).To(BeTrue())
		Expect(tpl.CaddyResource).To(BeNil())
		Expect(tpl.UseSSL).To(BeNil())
	})
}

func TestCaddyReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("parses inline kubeconfig and caddy config", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{
			"kubeconfig":     kubeconfigYAML,
			"enable":         true,
			"usePrefixes":    true,
			"trustedProxies": []string{"10.0.0.0/8", "192.168.0.0/16"},
		}}

		out, err := CaddyReadConfig(in)

		Expect(err).ToNot(HaveOccurred())
		res, ok := out.Config.(*CaddyResource)
		Expect(ok).To(BeTrue())
		Expect(res.KubernetesConfig).ToNot(BeNil())
		Expect(res.KubernetesConfig.Kubeconfig).To(Equal(kubeconfigYAML))
		Expect(res.CaddyConfig).ToNot(BeNil())
		Expect(res.CaddyConfig.Enable).ToNot(BeNil())
		Expect(*res.CaddyConfig.Enable).To(BeTrue())
		Expect(res.CaddyConfig.UsePrefixes).To(BeTrue())
		Expect(res.CaddyConfig.TrustedProxies).To(ConsistOf("10.0.0.0/8", "192.168.0.0/16"))
	})

	// Regression: CaddyReadConfig normalizes an absent/empty trustedProxies
	// slice to nil (yaml.Unmarshal into inline pointer structs can produce
	// []string{} for absent fields).
	t.Run("normalizes empty trustedProxies to nil", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{
			"kubeconfig":     kubeconfigYAML,
			"enable":         true,
			"trustedProxies": []string{},
		}}

		out, err := CaddyReadConfig(in)

		Expect(err).ToNot(HaveOccurred())
		res, ok := out.Config.(*CaddyResource)
		Expect(ok).To(BeTrue())
		Expect(res.CaddyConfig).ToNot(BeNil())
		Expect(res.CaddyConfig.TrustedProxies).To(BeNil())
	})

	t.Run("malformed config returns conversion error", func(t *testing.T) {
		RegisterTestingT(t)
		// A scalar config value cannot unmarshal into the CaddyResource struct,
		// so ConvertConfig errors and CaddyReadConfig returns it.
		in := &api.Config{Config: "not-a-mapping"}

		_, err := CaddyReadConfig(in)
		Expect(err).To(HaveOccurred())
	})

	// Quirk: when ONLY kubeconfig is supplied (no caddy-specific keys at all),
	// the inline *CaddyConfig pointer stays nil — yaml.Unmarshal never allocates
	// it. The normalization branch in CaddyReadConfig is therefore skipped.
	t.Run("absent caddy fields leave CaddyConfig nil", func(t *testing.T) {
		RegisterTestingT(t)
		in := &api.Config{Config: map[string]any{"kubeconfig": kubeconfigYAML}}

		out, err := CaddyReadConfig(in)

		Expect(err).ToNot(HaveOccurred())
		res, ok := out.Config.(*CaddyResource)
		Expect(ok).To(BeTrue())
		Expect(res.CaddyConfig).To(BeNil())
	})
}
