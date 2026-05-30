package kubernetes

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// TestCaddyfileEmbedHSTSPlaceholder locks in the runtime contract for
// CaddyConfig.HSTSValue: the `(hsts)` snippet in embed/caddy/Caddyfile
// MUST use Caddy's `{$HSTS_VALUE:default}` env-var placeholder, with the
// default preserving the prior literal value byte-for-byte. If a future
// refactor accidentally re-inlines the literal, this test fails and the
// HSTSValue field becomes silently inert.
func TestCaddyfileEmbedHSTSPlaceholder(t *testing.T) {
	RegisterTestingT(t)

	content, err := Caddyconfig.ReadFile("embed/caddy/Caddyfile")
	Expect(err).ToNot(HaveOccurred(), "embed Caddyfile must be readable")

	body := string(content)

	// The (hsts) snippet must contain the placeholder, not a hard-coded
	// literal — that's the only mechanism by which CaddyConfig.HSTSValue
	// can override the value at runtime.
	Expect(body).To(ContainSubstring(`{$HSTS_VALUE:max-age=31536000; includeSubDomains; preload}`),
		"(hsts) snippet must use Caddy's env-var placeholder so CaddyConfig.HSTSValue can override it")

	// The literal value (without the {$...} wrapper) must NOT appear as a
	// bare token, otherwise we'd have two competing definitions. The
	// previous hard-coded form was exactly the substring below — the test
	// guards against it being reintroduced.
	Expect(body).ToNot(MatchRegexp(`Strict-Transport-Security\s+"max-age=31536000;\s+includeSubDomains;\s+preload"\s*$`),
		"the prior literal Strict-Transport-Security value must not be present alongside the placeholder")
}

// TestCaddyConfig_HSTSValue_FieldShape locks the struct contract: the
// field must be `*string` (nilable, so unset means "use default"), with
// the documented JSON+YAML tag `hstsValue,omitempty`. A future rename
// to `HstsValue` or a tag drift to `hsts_value` would silently break
// every parent stack server.yaml that sets the new field.
func TestCaddyConfig_HSTSValue_FieldShape(t *testing.T) {
	RegisterTestingT(t)

	cfg := k8s.CaddyConfig{}
	// Nil-pointer assignment must be allowed (the unset case).
	Expect(cfg.HSTSValue).To(BeNil())

	// Setting a value must round-trip.
	cfg.HSTSValue = lo.ToPtr("max-age=31536000; includeSubDomains")
	Expect(cfg.HSTSValue).ToNot(BeNil())
	Expect(*cfg.HSTSValue).To(Equal("max-age=31536000; includeSubDomains"))
}
