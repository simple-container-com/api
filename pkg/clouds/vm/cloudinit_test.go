// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"
)

const testDeployPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest sc-deploy"

func cloudConfig() *VmConfig {
	return &VmConfig{Driver: DriverCloudru, Region: "ru-moscow-1"}
}

// The rendered user-data has to be valid YAML before anything else is true about
// it: cloud-init's answer to a parse error is to do nothing and boot a machine that
// looks fine and has no docker on it.
func TestRenderCloudInitIsValidYaml(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testDeployPublicKey)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(HavePrefix("#cloud-config\n"))

	var parsed struct {
		Users []struct {
			Name              string   `yaml:"name"`
			Groups            []string `yaml:"groups"`
			SshAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
		} `yaml:"users"`
		WriteFiles []struct {
			Path        string `yaml:"path"`
			Permissions string `yaml:"permissions"`
			Content     string `yaml:"content"`
		} `yaml:"write_files"`
		Runcmd [][]string `yaml:"runcmd"`
	}
	Expect(yaml.Unmarshal([]byte(out), &parsed)).To(Succeed())

	Expect(parsed.Users).To(HaveLen(1))
	Expect(parsed.Users[0].Name).To(Equal(DefaultSshUser))
	Expect(parsed.Users[0].SshAuthorizedKeys).To(Equal([]string{testDeployPublicKey}))
	// Deliberately absent: cloud-init's users module runs before runcmd, so the
	// docker group does not exist yet and `groups: [docker]` leaves an account that
	// cannot talk to the daemon. Membership is granted after the install instead.
	Expect(parsed.Users[0].Groups).To(BeEmpty())

	paths := make([]string, 0, len(parsed.WriteFiles))
	for _, f := range parsed.WriteFiles {
		paths = append(paths, f.Path)
	}
	Expect(paths).To(ConsistOf("/etc/docker/daemon.json", "/etc/sysctl.d/99-sc.conf"))

	Expect(parsed.Runcmd).To(HaveLen(3))
	joined := strings.Join(parsed.Runcmd[1], " ")
	Expect(joined).To(ContainSubstring("groupadd -f docker"))
	Expect(joined).To(ContainSubstring("usermod -aG docker " + DefaultSshUser))
	// Explicit, because writing /etc/sysctl.d/99-sc.conf otherwise only takes effect
	// on the next boot — and the first deploy happens on this one.
	Expect(joined).To(ContainSubstring("sysctl --system"))

	// The marker is written last and only on success: its presence is the readiness
	// contract every driver's wait loop reads.
	Expect(strings.Join(parsed.Runcmd[2], " ")).To(ContainSubstring(CloudInitDoneMarker))
}

// The install is skipped when docker is already present: several RU marketplace
// images ship it, and piping get.docker.com over a working install is both slow and
// a dependency on egress that did not need to exist.
func TestRenderCloudInitDockerInstallIsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testDeployPublicKey)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(ContainSubstring("command -v docker"))
	Expect(out).To(ContainSubstring(DefaultDockerInstallScript))
}

func TestRenderCloudInitHonoursSshUserAndExtraKeys(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.SshUser = "deployer"
	cfg.ExtraSshKeys = []string{"ssh-rsa AAAAoncall oncall@example.com", ""}

	out, err := RenderCloudInit(cfg, RoleK3sServer, testDeployPublicKey)
	Expect(err).ToNot(HaveOccurred())

	var parsed struct {
		Users []struct {
			Name              string   `yaml:"name"`
			SshAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
		} `yaml:"users"`
	}
	Expect(yaml.Unmarshal([]byte(out), &parsed)).To(Succeed())
	Expect(parsed.Users[0].Name).To(Equal("deployer"))
	// The deploy key first, the operator's extras after, and the empty entry
	// dropped — an empty line in ssh_authorized_keys is a parse error for sshd.
	Expect(parsed.Users[0].SshAuthorizedKeys).To(Equal([]string{
		testDeployPublicKey, "ssh-rsa AAAAoncall oncall@example.com",
	}))
	Expect(out).To(ContainSubstring("usermod -aG docker deployer"))
	Expect(out).To(ContainSubstring("-o deployer -g deployer " + ConfigDir + " " + StateDir))
}

func TestRenderCloudInitRefusals(t *testing.T) {
	RegisterTestingT(t)

	// byo: not a limitation but a definition — the operator's machine already
	// booted, so there is no first boot to hook into.
	_, err := RenderCloudInit(byoConfig(), RoleCompose, testDeployPublicKey)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("the machine already booted"))

	// A machine with no authorised key is a machine SC cannot deploy to, and finding
	// that out after creation means destroying and recreating it.
	_, err = RenderCloudInit(cloudConfig(), RoleCompose, "  ")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("deploy public key"))

	_, err = RenderCloudInit(cloudConfig(), "", testDeployPublicKey)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("host role"))

	_, err = RenderCloudInit(nil, RoleCompose, testDeployPublicKey)
	Expect(err).To(HaveOccurred())
}

// The log caps are not tidiness. A VM does not rotate container logs for you; an
// uncapped portal fills its disk in a few weeks and fails in a way that reads as
// "the application broke".
func TestDockerDaemonJSON(t *testing.T) {
	RegisterTestingT(t)

	rendered, err := DockerDaemonJSON(nil)
	Expect(err).ToNot(HaveOccurred())

	var daemon map[string]any
	Expect(json.Unmarshal([]byte(rendered), &daemon)).To(Succeed())
	Expect(daemon["log-driver"]).To(Equal("json-file"))
	Expect(daemon["log-opts"]).To(Equal(map[string]any{"max-size": "50m", "max-file": "3"}))
	Expect(daemon).ToNot(HaveKey("registry-mirrors"), "an absent mirror list must not render an empty one")

	rendered, err = DockerDaemonJSON([]string{"https://mirror.example.ru", ""})
	Expect(err).ToNot(HaveOccurred())
	daemon = nil
	Expect(json.Unmarshal([]byte(rendered), &daemon)).To(Succeed())
	Expect(daemon["registry-mirrors"]).To(Equal([]any{"https://mirror.example.ru"}))
}

// daemon.json is embedded into the YAML, and a mirror URL is operator input. A
// quote in it must not be able to produce a file the daemon refuses to parse —
// which it answers by not starting at all, i.e. a machine with no docker.
func TestDockerDaemonJSONSurvivesHostileMirror(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.RegistryMirrors = []string{`https://x.example.ru"}`}

	out, err := RenderCloudInit(cfg, RoleCompose, testDeployPublicKey)
	Expect(err).ToNot(HaveOccurred())

	var parsed struct {
		WriteFiles []struct {
			Path    string `yaml:"path"`
			Content string `yaml:"content"`
		} `yaml:"write_files"`
	}
	Expect(yaml.Unmarshal([]byte(out), &parsed)).To(Succeed())

	var content string
	for _, f := range parsed.WriteFiles {
		if f.Path == "/etc/docker/daemon.json" {
			content = f.Content
		}
	}
	var daemon map[string]any
	Expect(json.Unmarshal([]byte(content), &daemon)).To(Succeed())
	Expect(daemon["registry-mirrors"]).To(Equal([]any{`https://x.example.ru"}`}))
}
