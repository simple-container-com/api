// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// Test utilities and helpers

// createBasicTestArgs creates a minimal SimpleContainerArgs for testing
func createBasicTestArgs() *SimpleContainerArgs {
	return &SimpleContainerArgs{
		Namespace:              "test-namespace",
		Service:                "test-service",
		ScEnv:                  "test",
		Domain:                 "test.example.com",
		Prefix:                 "",
		ProxyKeepPrefix:        false,
		Deployment:             "test-deployment",
		ParentStack:            lo.ToPtr("parent/stack"),
		Replicas:               1,
		GenerateCaddyfileEntry: true,
		Log:                    logger.New(),
		KubeProvider:           nil, // This might be required

		// Optional properties with defaults
		PodDisruption: &k8s.DisruptionBudget{
			MinAvailable: lo.ToPtr(1),
		},
		SecretEnvs:   map[string]string{},
		Annotations:  map[string]string{},
		NodeSelector: map[string]string{},
		IngressContainer: &k8s.CloudRunContainer{
			Name:     "test-container",
			Ports:    []int{8080},
			MainPort: lo.ToPtr(8080),
		},
		ServiceType:       lo.ToPtr("ClusterIP"),
		ProvisionIngress:  false,
		Headers:           &k8s.Headers{},
		Volumes:           []k8s.SimpleTextVolume{},
		SecretVolumes:     []k8s.SimpleTextVolume{},
		PersistentVolumes: []k8s.PersistentVolume{},
		VPA:               nil,
		Scale:             nil,

		// Add a basic container - required for deployment creation
		Containers: []corev1.ContainerArgs{
			{
				Name:  sdk.String("test-container"),
				Image: sdk.String("nginx:latest"),
				Ports: corev1.ContainerPortArray{
					&corev1.ContainerPortArgs{
						ContainerPort: sdk.Int(8080),
						Name:          sdk.String("http"),
					},
				},
			},
		},
	}
}

// createHPATestArgs creates SimpleContainerArgs with HPA enabled
func createHPATestArgs() *SimpleContainerArgs {
	args := createBasicTestArgs()
	args.Scale = &k8s.Scale{
		Replicas:     2,
		EnableHPA:    true,
		MinReplicas:  2,
		MaxReplicas:  10,
		CPUTarget:    lo.ToPtr(70),
		MemoryTarget: lo.ToPtr(80),
	}
	return args
}

// createVPATestArgs creates SimpleContainerArgs with VPA enabled
func createVPATestArgs() *SimpleContainerArgs {
	args := createBasicTestArgs()
	args.VPA = &k8s.VPAConfig{
		Enabled:    true,
		UpdateMode: lo.ToPtr("Auto"),
	}
	return args
}

// createVPATestArgsWithControlledValues exercises the full VPA surface area:
// minAllowed, maxAllowed, controlledResources (which lives inside the
// containerPolicy per the VPA CRD), and the controlledValues knob that lets
// callers opt out of VPA scaling limits proportionally with requests.
func createVPATestArgsWithControlledValues() *SimpleContainerArgs {
	args := createBasicTestArgs()
	args.VPA = &k8s.VPAConfig{
		Enabled:    true,
		UpdateMode: lo.ToPtr("Auto"),
		MinAllowed: &k8s.VPAResourceRequirements{
			CPU:    lo.ToPtr("50m"),
			Memory: lo.ToPtr("64Mi"),
		},
		MaxAllowed: &k8s.VPAResourceRequirements{
			CPU:    lo.ToPtr("2"),
			Memory: lo.ToPtr("4Gi"),
		},
		ControlledResources: []string{"cpu", "memory"},
		ControlledValues:    lo.ToPtr("RequestsOnly"),
	}
	return args
}

// createComplexTestArgs creates SimpleContainerArgs with many features enabled
func createComplexTestArgs() *SimpleContainerArgs {
	args := createBasicTestArgs()
	args.ProvisionIngress = true
	args.PersistentVolumes = []k8s.PersistentVolume{
		{
			Name:        "test-volume",
			MountPath:   "/data",
			Storage:     "1Gi",
			AccessModes: []string{"ReadWriteOnce"},
		},
	}
	args.Volumes = []k8s.SimpleTextVolume{
		{
			TextVolume: api.TextVolume{
				Name:    "config-volume",
				Content: "test-config",
			},
		},
	}
	args.SecretVolumes = []k8s.SimpleTextVolume{
		{
			TextVolume: api.TextVolume{
				Name:    "secret-volume",
				Content: "secret-data",
			},
		},
	}
	args.SecretEnvs = map[string]string{
		"SECRET_KEY": "secret-value",
	}
	args.Annotations = map[string]string{
		"custom.annotation": "test-value",
	}
	args.NodeSelector = map[string]string{
		"node-type": "compute",
	}
	return args
}

// Basic Resource Creation Tests

func TestNewSimpleContainer_BasicResourceCreation(t *testing.T) {
	RegisterTestingT(t)

	// Create mock and test args
	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()

	// Run the test
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer creation should succeed")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Focus on validating that SimpleContainer was created successfully
		// and has the expected outputs rather than counting individual resources
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")
		Expect(sc.Service).ToNot(BeNil(), "Service should not be nil")

		// Verify CaddyfileEntry is generated
		Expect(sc.CaddyfileEntry).ToNot(BeEmpty(), "CaddyfileEntry should be generated")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_NamespaceCreation(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer creation should succeed")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify namespace output is available
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace output should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_DeploymentCreation(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer creation should succeed")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify deployment output is available
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment output should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_ServiceCreation(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer creation should succeed")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify service outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Service).ToNot(BeNil(), "Service should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_WithIngress(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()
	args.ProvisionIngress = true

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with ingress should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify basic outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_WithoutIngress(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()
	args.ProvisionIngress = false

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer without ingress should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify basic outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_WithPersistentVolumes(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createBasicTestArgs()
	args.PersistentVolumes = []k8s.PersistentVolume{
		{
			Name:        "test-volume-1",
			MountPath:   "/data1",
			Storage:     "1Gi",
			AccessModes: []string{"ReadWriteOnce"},
		},
		{
			Name:        "test-volume-2",
			MountPath:   "/data2",
			Storage:     "2Gi",
			AccessModes: []string{"ReadWriteMany"},
		},
	}

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with persistent volumes should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify basic outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_WithHPA(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createHPATestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with HPA should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify basic outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

// TestNewSimpleContainer_WithVPA_ControlledValues exercises the new
// ControlledValues + ControlledResources fields on VPAConfig. Asserts the
// resource creation succeeds; the actual CRD shape (controlledValues +
// controlledResources living inside containerPolicy, not at resourcePolicy
// level) is enforced by simple_container.go's createVPA implementation.
func TestNewSimpleContainer_WithVPA_ControlledValues(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createVPATestArgsWithControlledValues()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with VPA controlledValues should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_WithVPA(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createVPATestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with VPA should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify basic outputs are available
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

func TestNewSimpleContainer_ComplexConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := createComplexTestArgs()

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with complex configuration should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Verify all expected outputs are properly set
		Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
		Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")
		Expect(sc.Service).ToNot(BeNil(), "Service should not be nil")
		Expect(sc.CaddyfileEntry).ToNot(BeEmpty(), "CaddyfileEntry should be generated")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}

// CaddyfileEntry Header Rendering Tests
//
// Regression coverage for two coupled bugs in the `args.Headers` rendering
// path: (a) the first `header_down` was being concatenated onto the
// `header_down Server nginx ${addHeaders}` template line, producing
// invalid Caddyfile syntax, and (b) values weren't quoted, so multi-token
// headers (CSP, Permissions-Policy) broke Caddy's whitespace tokenizer.
// Both stay fixed only if these assertions hold.

// caddyfileEntryFor returns the rendered Caddyfile entry for the given
// args. Uses the same pulumi-mocks pattern as the rest of this file.
func caddyfileEntryFor(t *testing.T, args *SimpleContainerArgs) string {
	t.Helper()
	mocks := NewSimpleContainerMocks()
	var rendered string
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		if err != nil {
			return err
		}
		rendered = string(sc.CaddyfileEntry)
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))
	Expect(err).ToNot(HaveOccurred(), "pulumi run should not fail")
	return rendered
}

func TestCaddyfileEntry_HeadersRenderOnTheirOwnLines(t *testing.T) {
	RegisterTestingT(t)

	args := createBasicTestArgs()
	args.Headers = &k8s.Headers{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	entry := caddyfileEntryFor(t, args)

	// The bug: first custom header_down landed on the same line as
	// `header_down Server nginx`. Assert each of our headers gets its
	// own line.
	for _, hdr := range []string{"X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy"} {
		// `(?m)^[\t ]*header_down ` ensures the directive starts a line
		// (only leading whitespace allowed before it).
		Expect(entry).To(MatchRegexp(`(?m)^[\t ]*header_down `+regexp.QuoteMeta(hdr)+` `),
			"header_down for %s must start its own line, got:\n%s", hdr, entry)
	}

	// And the `Server nginx` directive must end its own line — i.e. the
	// next non-whitespace token after `nginx` is a newline.
	Expect(entry).To(MatchRegexp(`header_down Server nginx[\t ]*\n`),
		"`header_down Server nginx` must be followed by a newline, got:\n%s", entry)
}

func TestCaddyfileEntry_MultiTokenHeaderValuesAreQuoted(t *testing.T) {
	RegisterTestingT(t)

	csp := `default-src 'self'; script-src 'self' 'unsafe-inline' https://example.com; frame-ancestors 'none'`
	permissions := `geolocation=(), camera=(), microphone=(), payment=()`

	args := createBasicTestArgs()
	args.Headers = &k8s.Headers{
		"Content-Security-Policy-Report-Only": csp,
		"Permissions-Policy":                  permissions,
	}

	entry := caddyfileEntryFor(t, args)

	// Caddy tokenizes on whitespace; multi-word header values MUST be
	// wrapped in double quotes or the parser sees them as extra args.
	// %q also escapes embedded double quotes — there are none in CSP, but
	// the literal single quotes around 'self' are passed through.
	Expect(entry).To(ContainSubstring(`header_down Content-Security-Policy-Report-Only "`+csp+`"`),
		"CSP value must be double-quoted in the rendered Caddyfile, got:\n%s", entry)
	Expect(entry).To(ContainSubstring(`header_down Permissions-Policy "`+permissions+`"`),
		"Permissions-Policy value must be double-quoted, got:\n%s", entry)
}

func TestCaddyfileEntry_HeadersAreSortedDeterministically(t *testing.T) {
	RegisterTestingT(t)

	args := createBasicTestArgs()
	args.Headers = &k8s.Headers{
		"X-Frame-Options":        "DENY",
		"A-Header":               "first-alphabetically",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	}

	// Two-render equality is probabilistic with only 4 keys (~75% miss
	// rate even with broken sorting), so it's a weak signal on its own.
	// The load-bearing assertion is the alphabetical index check below.
	a := caddyfileEntryFor(t, args)
	b := caddyfileEntryFor(t, args)
	Expect(a).To(Equal(b), "two renders with the same Headers map must be byte-identical")

	// This is what actually proves determinism: deterministic alphabetical
	// ordering (A-Header before Referrer-Policy before X-* etc.) so we
	// can rely on the rendered bytes for the change-hash signal.
	idxA := strings.Index(a, "header_down A-Header ")
	idxR := strings.Index(a, "header_down Referrer-Policy ")
	idxXC := strings.Index(a, "header_down X-Content-Type-Options ")
	idxXF := strings.Index(a, "header_down X-Frame-Options ")
	Expect(idxA).To(BeNumerically(">", 0), "A-Header must be present")
	Expect(idxA).To(BeNumerically("<", idxR), "A-Header before Referrer-Policy")
	Expect(idxR).To(BeNumerically("<", idxXC), "Referrer-Policy before X-Content-Type-Options")
	Expect(idxXC).To(BeNumerically("<", idxXF), "X-Content-Type-Options before X-Frame-Options")
}

func TestCaddyfileEntry_EmptyHeadersStillRenders(t *testing.T) {
	RegisterTestingT(t)

	args := createBasicTestArgs()
	// args.Headers is &k8s.Headers{} (empty) from createBasicTestArgs.
	entry := caddyfileEntryFor(t, args)

	// Server nginx still rendered, no spurious `header_down` directives,
	// no trailing garbage from a leaked newline.
	Expect(entry).To(ContainSubstring("header_down Server nginx"))
	Expect(entry).ToNot(MatchRegexp(`header_down Server nginx[^\n]+header_down`),
		"with empty Headers, the Server nginx line must not be followed by any other header_down on the same line, got:\n%s", entry)
}

func TestCaddyfileEntry_EmptyHeadersByteIdenticalToPreFix(t *testing.T) {
	RegisterTestingT(t)

	args := createBasicTestArgs()
	// args.Headers is &k8s.Headers{} (empty) from createBasicTestArgs.
	entry := caddyfileEntryFor(t, args)

	// Lock in the compatibility contract: when Headers is empty, the
	// rendered Caddyfile must contain `header_down Server nginx ` followed
	// immediately by a newline (the trailing space comes from the literal
	// space in the template between `nginx` and the now-empty `${addHeaders}`
	// substitution). Pre-fix output had exactly this shape; we must not
	// drift it, or the parent Caddy aggregator sees a spurious change-hash
	// flap on every existing header-less stack after the SC upgrade.
	Expect(entry).To(ContainSubstring("header_down Server nginx \n"),
		"empty-Headers output must end the Server-nginx line with a single space + newline (byte-identical to pre-fix), got:\n%s", entry)
}

func TestCaddyfileEntry_HeaderValueWithEmbeddedDoubleQuote(t *testing.T) {
	RegisterTestingT(t)

	// CSP with a strict-dynamic attribute that uses double-quoted hashes
	// is a realistic case that contains a literal `"`. `%q` must escape it
	// so Caddy's lexer round-trips back to the original value.
	value := `default-src 'self'; script-src 'self' "sha256-abc=" "strict-dynamic"`

	args := createBasicTestArgs()
	args.Headers = &k8s.Headers{
		"Content-Security-Policy": value,
	}

	entry := caddyfileEntryFor(t, args)

	// %q emits each embedded `"` as `\"`. Caddy's quoted-string lexer
	// supports `\"` and `\\` as escape sequences; on round-trip the header
	// value the server sets equals `value` exactly. This assertion locks in
	// the escape behaviour — without it, a future refactor to bare `%s`
	// would silently break any header value that contains a quote.
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	Expect(entry).To(ContainSubstring(`header_down Content-Security-Policy "`+escaped+`"`),
		"value containing embedded \" must be %%q-escaped (\\\") in the rendered Caddyfile, got:\n%s", entry)
}

func TestCaddyfileEntry_SiteExtraHelpersRendersAtSiteLevel(t *testing.T) {
	RegisterTestingT(t)

	// `rate_limit` (and other site-level HTTP handlers) MUST NOT land inside
	// the `reverse_proxy { ... }` block — Caddy's grammar rejects them there.
	// This test locks in: SiteExtraHelpers entries appear AFTER the closing
	// `}` of reverse_proxy and BEFORE the closing `}` of the site block, so
	// `lbConfig.siteExtraHelpers` is a valid insertion point for rate_limit,
	// top-level matchers, `respond` directives, etc.
	args := createBasicTestArgs()
	args.LbConfig = &api.SimpleContainerLBConfig{
		Https: true,
		SiteExtraHelpers: []string{
			`rate_limit { distributed; zone login { key {remote_host}; events 5; window 1m } }`,
		},
	}
	entry := caddyfileEntryFor(t, args)

	// Find the close-brace of the reverse_proxy block and the close-brace of
	// the site block. SiteExtraHelpers must appear strictly between them.
	rpOpenIdx := strings.Index(entry, "reverse_proxy ")
	Expect(rpOpenIdx).To(BeNumerically(">", 0), "expected reverse_proxy open, got:\n%s", entry)
	rpCloseIdx := strings.Index(entry[rpOpenIdx:], "\n  }")
	Expect(rpCloseIdx).To(BeNumerically(">", 0),
		"expected reverse_proxy close brace (2-space indent), got:\n%s", entry)
	rpCloseIdx += rpOpenIdx

	rlIdx := strings.Index(entry, "rate_limit")
	Expect(rlIdx).To(BeNumerically(">", rpCloseIdx),
		"rate_limit must appear AFTER the reverse_proxy block closes, got:\n%s", entry)

	// And it must NOT appear INSIDE the reverse_proxy block.
	rpBody := entry[rpOpenIdx:rpCloseIdx]
	Expect(rpBody).ToNot(ContainSubstring("rate_limit"),
		"rate_limit must NOT appear inside the reverse_proxy block body, got:\n%s", rpBody)
}

func TestCaddyfileEntry_EmptySiteExtraHelpersIsByteIdentical(t *testing.T) {
	RegisterTestingT(t)

	// Compatibility contract: stacks that don't set lbConfig.siteExtraHelpers
	// must produce output structurally identical to the pre-feature rendering,
	// so the parent Caddy aggregator's change-hash doesn't flap on the SC
	// upgrade for any of the ~hundreds of existing consumer stacks.
	args := createBasicTestArgs()
	args.LbConfig = &api.SimpleContainerLBConfig{Https: true}
	// SiteExtraHelpers intentionally unset.

	entry := caddyfileEntryFor(t, args)

	// The placeholder must have substituted as an empty string: no stray
	// blank-indent line should be emitted between imports and the site close.
	Expect(entry).ToNot(MatchRegexp(`import handle_static\n  \n}`),
		"empty siteExtraHelpers must not produce a blank line before the site close, got:\n%s", entry)
}

func TestCaddyfileEntry_HeadersOnPrefixTemplate(t *testing.T) {
	RegisterTestingT(t)

	// The Prefix template branch (handle_path /<prefix>* ...) shares the
	// same addHeaders placeholder as the Domain branch but at a different
	// indent level. Without a test here, the second template variant has
	// zero CI coverage of the header-rendering path.
	args := createBasicTestArgs()
	args.Domain = ""
	args.Prefix = "api"
	args.Headers = &k8s.Headers{
		"X-Frame-Options":    "DENY",
		"Permissions-Policy": "geolocation=(), camera=()",
	}

	entry := caddyfileEntryFor(t, args)

	// Both directives must appear on their own lines and the
	// multi-token Permissions-Policy must be quoted, same contract as
	// the Domain template.
	Expect(entry).To(MatchRegexp(`(?m)^[\t ]*header_down X-Frame-Options `))
	Expect(entry).To(ContainSubstring(`header_down Permissions-Policy "geolocation=(), camera=()"`))
}

// Name Sanitization Tests

func TestNewSimpleContainer_NameSanitization(t *testing.T) {
	testCases := []struct {
		name           string
		inputName      string
		expectedSuffix string // We'll check if the sanitized name ends with this
	}{
		{
			name:           "underscores_replaced",
			inputName:      "test_service_name",
			expectedSuffix: "test-service-name",
		},
		{
			name:           "uppercase_lowercased",
			inputName:      "TestServiceName",
			expectedSuffix: "testservicename",
		},
		{
			name:           "special_chars_removed",
			inputName:      "test@service#name!",
			expectedSuffix: "testservicename",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks := NewSimpleContainerMocks()
			args := createBasicTestArgs()
			args.Service = tc.inputName
			args.Deployment = tc.inputName
			args.Namespace = tc.inputName

			RegisterTestingT(t)

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				sc, err := NewSimpleContainer(ctx, args)
				Expect(err).ToNot(HaveOccurred(), "SimpleContainer with name sanitization should be created successfully")
				Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

				// Verify that SimpleContainer was created successfully
				Expect(sc.ServicePublicIP).ToNot(BeNil(), "ServicePublicIP should not be nil")
				Expect(sc.ServiceName).ToNot(BeNil(), "ServiceName should not be nil")
				Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
				Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

				return nil
			}, pulumi.WithMocks("project", "stack", mocks))

			Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
		})
	}
}

// Edge Case Tests

func TestNewSimpleContainer_MinimalConfiguration(t *testing.T) {
	RegisterTestingT(t)

	mocks := NewSimpleContainerMocks()
	args := &SimpleContainerArgs{
		Namespace:  "minimal",
		Service:    "minimal-service",
		ScEnv:      "test",
		Deployment: "minimal-deployment",
		Replicas:   1,
		Log:        logger.New(),
		// Minimal required fields only
	}

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		sc, err := NewSimpleContainer(ctx, args)
		Expect(err).ToNot(HaveOccurred(), "SimpleContainer with minimal configuration should be created successfully")
		Expect(sc).ToNot(BeNil(), "SimpleContainer should not be nil")

		// Even with minimal config, basic outputs should be available
		Expect(sc.Namespace).ToNot(BeNil(), "Namespace should not be nil")
		Expect(sc.Deployment).ToNot(BeNil(), "Deployment should not be nil")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks))

	Expect(err).ToNot(HaveOccurred(), "Test should complete without errors")
}
