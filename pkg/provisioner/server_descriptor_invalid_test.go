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

// The regression this guards: #492 moved `type: cloudflare` out of tree, so a
// server.yaml using it without a `plugins:` pin is rejected by
// ValidatePluginRequirements. That rejection must reach the operator — it carries
// the copy-paste-ready `plugins:` block that tells them how to fix it.
func TestReadServerDescriptor_MissingPluginPinIsInvalidAndActionable(t *testing.T) {
	RegisterTestingT(t)
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
resources:
  registrar:
    type: cloudflare
    config:
      zoneName: example.com
`)

	_, err := (&provisioner{}).readServerDescriptor(root, "dist")

	Expect(err).NotTo(BeNil())
	Expect(errors.Is(err, api.ErrServerDescriptorInvalid)).To(BeTrue())
	Expect(err.Error()).To(ContainSubstring("plugins:"),
		"the fool-protector's remediation block must survive to the operator")
	Expect(err.Error()).To(ContainSubstring("sc-plugin-cloudflare"))
}

// A correctly pinned server.yaml parses, and the provisioner is actually set —
// the absence of which was the symptom the real failure presented as.
func TestReadServerDescriptor_PinnedPluginParses(t *testing.T) {
	RegisterTestingT(t)
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
}
