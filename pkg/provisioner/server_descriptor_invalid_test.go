// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

// writeStack lays out <root>/<name>/server.yaml and returns the root.
func writeStack(t *testing.T, name, serverYaml string) string {
	t.Helper()
	root := t.TempDir()
	dir := path.Join(root, name)
	RegisterTestingT(t)
	Expect(os.MkdirAll(dir, 0o755)).To(BeNil())
	if serverYaml != "" {
		Expect(os.WriteFile(path.Join(dir, api.ServerDescriptorFileName), []byte(serverYaml), 0o600)).To(BeNil())
	}
	return root
}

// A stack directory with no server.yaml is a legitimate client-only stack, so the
// error must stay ignorable under ReadOpts.IgnoreServerMissing.
func TestReadServerDescriptor_MissingIsNotInvalid(t *testing.T) {
	RegisterTestingT(t)
	root := writeStack(t, "client-only", "")

	_, err := (&provisioner{}).readServerDescriptor(root, "client-only")

	Expect(err).NotTo(BeNil())
	Expect(errors.Is(err, api.ErrServerDescriptorInvalid)).To(BeFalse(),
		"an absent server.yaml must remain ignorable under IgnoreServerMissing")
}

// Malformed YAML is NOT the same thing as an absent file: it must be fatal, or it
// resurfaces later as the unrelated "provisioner is not set for stack %q".
func TestReadServerDescriptor_MalformedIsInvalid(t *testing.T) {
	RegisterTestingT(t)
	root := writeStack(t, "broken", "provisioner: [this is not a mapping\n")

	_, err := (&provisioner{}).readServerDescriptor(root, "broken")

	Expect(err).NotTo(BeNil())
	Expect(errors.Is(err, api.ErrServerDescriptorInvalid)).To(BeTrue())
}

// unpinnedCloudflareStack is a server.yaml that predates the #492 plugin split:
// it uses `type: cloudflare`, which is now owned by sc-plugin-cloudflare, and
// carries no `plugins:` block.
const unpinnedCloudflareStack = `schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    organization: acme
    state-storage:
      type: pulumi-cloud
      config:
        credentials: "token"
    secrets-provider:
      type: pulumi-cloud
      config:
        credentials: "token"
resources:
  registrar:
    type: cloudflare
    config:
      zoneName: example.com
`

// The backwards-compatibility guarantee: an un-pinned server.yaml that predates
// #492 must still PARSE, with the pin resolved and injected for the operator.
// Anything else makes the upgrade a flag day.
//
// The resolver is stubbed because auto-injection otherwise calls api.github.com —
// which would make this test depend on runner egress and on whatever happens to
// be the latest release. (That is exactly what the previous version of this test
// did, silently, once auto-injection landed.)
func TestReadServerDescriptor_MissingPluginPinIsAutoInjected(t *testing.T) {
	RegisterTestingT(t)
	defer api.SetLatestVersionResolver(func(repo string) (string, error) {
		Expect(repo).To(Equal("simple-container-com/sc-plugin-cloudflare"))
		return "v9.9.9-stub", nil
	})()
	root := writeStack(t, "dist", unpinnedCloudflareStack)

	desc, err := (&provisioner{}).readServerDescriptor(root, "dist")

	Expect(err).To(BeNil())
	Expect(desc).NotTo(BeNil())
	Expect(desc.Plugins).To(HaveKey("cloudflare"),
		"the un-pinned plugin must be injected into the parsed descriptor")
	Expect(desc.Plugins["cloudflare"].Source).NotTo(BeNil())
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v9.9.9-stub"))
	Expect(desc.Provisioner.GetProvisioner()).NotTo(BeNil(),
		"a parsed server.yaml must carry a usable provisioner")
}

// When resolution itself fails — air-gapped, rate-limited, repo gone — parsing
// IS fatal, and the rejection must reach the operator: it carries the
// copy-paste-ready `plugins:` block that tells them how to fix it by hand.
// Swallowing it leaves stack.Server zero and resurfaces much later as the
// unrelated "provisioner is not set for stack %q", which is how the #492 split
// broke the release pipeline's own `publish sc tool` step.
func TestReadServerDescriptor_UnresolvablePluginPinIsInvalidAndActionable(t *testing.T) {
	RegisterTestingT(t)
	defer api.SetLatestVersionResolver(func(string) (string, error) {
		return "", errors.New("dial tcp: lookup api.github.com: no such host")
	})()
	root := writeStack(t, "dist", unpinnedCloudflareStack)

	_, err := (&provisioner{}).readServerDescriptor(root, "dist")

	Expect(err).NotTo(BeNil())
	Expect(errors.Is(err, api.ErrServerDescriptorInvalid)).To(BeTrue())
	Expect(err.Error()).To(ContainSubstring("plugins:"),
		"the fool-protector's remediation block must survive to the operator")
	Expect(err.Error()).To(ContainSubstring("sc-plugin-cloudflare"))
	Expect(err.Error()).To(ContainSubstring("no such host"),
		"the operator needs to know WHY resolution failed")
}

// A correctly pinned server.yaml parses, and the provisioner is actually set —
// the absence of which was the symptom the real failure presented as. It must
// also make NO resolver call: that a pinned (reproducible) steady-state deploy
// costs zero network round-trips is the reason pinning is the documented
// recommendation, so it is asserted rather than assumed.
func TestReadServerDescriptor_PinnedPluginParses(t *testing.T) {
	RegisterTestingT(t)
	resolverCalled := false
	defer api.SetLatestVersionResolver(func(string) (string, error) {
		resolverCalled = true
		return "", errors.New("resolver must not be consulted for a pinned plugin")
	})()
	root := writeStack(t, "dist", `schemaVersion: 1.0
provisioner:
  type: pulumi
  config:
    organization: acme
    state-storage:
      type: pulumi-cloud
      config:
        credentials: "token"
    secrets-provider:
      type: pulumi-cloud
      config:
        credentials: "token"
plugins:
  cloudflare:
    source:
      type: github-release
      repository: simple-container-com/sc-plugin-cloudflare
      version: v2026.10.26-5eddde3
resources:
  registrar:
    type: cloudflare
    config:
      zoneName: example.com
`)

	desc, err := (&provisioner{}).readServerDescriptor(root, "dist")

	Expect(err).To(BeNil())
	Expect(desc).NotTo(BeNil())
	Expect(desc.Provisioner.GetProvisioner()).NotTo(BeNil(),
		"a parsed server.yaml must carry a usable provisioner")
	Expect(resolverCalled).To(BeFalse(),
		"a declared plugin must not trigger a GitHub release lookup")
	Expect(desc.Plugins["cloudflare"].Source.Version).To(Equal("v2026.10.26-5eddde3"),
		"the operator's pin must survive parsing unmodified")
}
