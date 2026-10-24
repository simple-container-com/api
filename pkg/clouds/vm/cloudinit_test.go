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

const (
	testDeployPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest sc-deploy"
	testHostPublicKey   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIhost root@host"
	// Shaped like what tls.PrivateKey.PrivateKeyOpenssh returns, trailing newline
	// included: the newline is the thing the renderer has to handle, because a padded
	// blank line inside a YAML block scalar becomes part of the key file.
	testHostPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n"
)

// cloudConfig is the shared "a cloud driver, minimally configured" fixture.
//
// `AllowUnverifiedProvider` is set because `cloudru-vm` addresses a cloud SC has
// never provisioned on and Validate refuses it without the flag (see the Unverified
// field of DriverInfo). Every test below is about something else; the gate itself is
// asserted in TestValidateRefusesUnverifiedProvider.
func cloudConfig() *VmConfig {
	return &VmConfig{
		Driver:                  DriverCloudru,
		Region:                  "ru-moscow-1",
		PublicIP:                true,
		AllowUnverifiedProvider: true,
	}
}

// evolutionConfig is the same fixture for the one Cloud.ru estate SC has actually
// provisioned on. Two fields differ from cloudConfig() and both are refusals rather
// than defaults, so switching `Driver` by hand leaves the config invalid for a
// reason that has nothing to do with the test doing the switching: Evolution is
// regionless (a `region` is refused) and live-verified (the gate opt-in is refused).
func evolutionConfig() *VmConfig {
	cfg := cloudConfig()
	cfg.Driver = DriverCloudruEvolution
	cfg.Region = ""
	cfg.AllowUnverifiedProvider = false
	return cfg
}

func testKeys() HostKeys {
	return HostKeys{
		DeployPublicKey: testDeployPublicKey,
		HostPrivateKey:  testHostPrivateKey,
		HostPublicKey:   testHostPublicKey,
	}
}

// The rendered user-data has to be valid YAML before anything else is true about
// it: cloud-init's answer to a parse error is to do nothing and boot a machine that
// looks fine and has no docker on it.
func TestRenderCloudInitIsValidYaml(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testKeys())
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

	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testKeys())
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(ContainSubstring("command -v docker"))
	Expect(out).To(ContainSubstring(DefaultDockerInstallScript))
}

func TestRenderCloudInitHonoursSshUserAndExtraKeys(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudConfig()
	cfg.SshUser = "deployer"
	cfg.ExtraSshKeys = []string{"ssh-rsa AAAAoncall oncall@example.com", ""}

	out, err := RenderCloudInit(cfg, RoleK3sServer, testKeys())
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

// `packageMirror` was accepted by the config, documented as the answer to
// unreachable distribution mirrors, and never rendered into anything — a field
// silently dropped, which is the exact failure mode the refuse-don't-ignore table
// exists to prevent, and no test caught it because every test asserted on what IS
// rendered. This one asserts on the field.
//
// It is expressed through cloud-init's apt module rather than a runcmd because the
// module rewrites the sources before any package-installing module runs; by the time
// runcmd runs, the boot has already spent its timeout on the default mirror.
func TestRenderCloudInitHonoursPackageMirror(t *testing.T) {
	RegisterTestingT(t)

	type aptEntry struct {
		Arches []string `yaml:"arches"`
		URI    string   `yaml:"uri"`
	}
	type parsedApt struct {
		Apt *struct {
			Primary  []aptEntry `yaml:"primary"`
			Security []aptEntry `yaml:"security"`
		} `yaml:"apt"`
	}

	// Absent, not empty: a machine with no mirror configured must get no apt block at
	// all, rather than one pointing at "".
	var without parsedApt
	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testKeys())
	Expect(err).ToNot(HaveOccurred())
	Expect(yaml.Unmarshal([]byte(out), &without)).To(Succeed())
	Expect(without.Apt).To(BeNil())
	Expect(out).ToNot(ContainSubstring("apt:"))

	cfg := cloudConfig()
	cfg.PackageMirror = "  https://mirror.yandex.ru/ubuntu  "

	var with parsedApt
	out, err = RenderCloudInit(cfg, RoleCompose, testKeys())
	Expect(err).ToNot(HaveOccurred())
	Expect(yaml.Unmarshal([]byte(out), &with)).To(Succeed())
	Expect(with.Apt).ToNot(BeNil())

	// Both primary AND security, from the same value: a mirror that carries the main
	// archive but leaves security pointing at security.ubuntu.com makes every boot
	// wait on the host this field exists to avoid, and the machine still comes up —
	// just slowly, which is the kind of thing nobody traces back here.
	Expect(with.Apt.Primary).To(Equal([]aptEntry{{Arches: []string{"default"}, URI: "https://mirror.yandex.ru/ubuntu"}}))
	Expect(with.Apt.Security).To(Equal(with.Apt.Primary))

	// The docker install is NOT affected, and the comment on
	// DefaultDockerInstallScript says so: the script curls a host of Docker's own and
	// then installs from Docker's apt repository, neither of which an apt mirror
	// redirects. Asserted here so the two do not drift into implying each other.
	Expect(out).To(ContainSubstring(DefaultDockerInstallScript))
}

func TestRenderCloudInitRefusals(t *testing.T) {
	RegisterTestingT(t)

	// byo: not a limitation but a definition — the operator's machine already
	// booted, so there is no first boot to hook into.
	_, err := RenderCloudInit(byoConfig(), RoleCompose, testKeys())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("the machine already booted"))

	// A machine with no authorised key is a machine SC cannot deploy to, and finding
	// that out after creation means destroying and recreating it.
	_, err = RenderCloudInit(cloudConfig(), RoleCompose, HostKeys{DeployPublicKey: "  "})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("deploy public key"))

	_, err = RenderCloudInit(cloudConfig(), "", testKeys())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("host role"))

	_, err = RenderCloudInit(nil, RoleCompose, testKeys())
	Expect(err).To(HaveOccurred())

	// Half a host key is worse than none: cloud-init writes a private key whose .pub
	// does not match it, sshd refuses to start, and the machine presents as one that
	// never answers SSH at all.
	for _, half := range []HostKeys{
		{DeployPublicKey: testDeployPublicKey, HostPrivateKey: testHostPrivateKey},
		{DeployPublicKey: testDeployPublicKey, HostPublicKey: testHostPublicKey},
	} {
		_, err = RenderCloudInit(cloudConfig(), RoleCompose, half)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("both halves"))
	}
}

// The host key rides in user-data under a YAML block scalar, and the two ways that
// can go wrong both present as a machine that never answers SSH: an unparseable
// document (cloud-init does nothing) or a key file with a line of whitespace in it
// (sshd refuses to start).
func TestRenderCloudInitPinsHostKey(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderCloudInit(cloudConfig(), RoleCompose, testKeys())
	Expect(err).ToNot(HaveOccurred())

	var parsed struct {
		SshKeys struct {
			Ed25519Private string `yaml:"ed25519_private"`
			Ed25519Public  string `yaml:"ed25519_public"`
		} `yaml:"ssh_keys"`
	}
	Expect(yaml.Unmarshal([]byte(out), &parsed)).To(Succeed())

	Expect(parsed.SshKeys.Ed25519Public).To(Equal(testHostPublicKey))
	// Round-trips byte for byte apart from the trailing newline the renderer trims
	// and YAML's block scalar puts back.
	Expect(parsed.SshKeys.Ed25519Private).To(Equal(testHostPrivateKey))
	for i, line := range strings.Split(out, "\n") {
		Expect(line).To(Equal(strings.TrimRight(line, " \t")),
			"line %d has trailing whitespace, which inside a `|` block becomes part of the key file", i+1)
	}

	// And the else-branch: no key pair given, no ssh_keys block, so the machine
	// generates its own and no deploy can verify it.
	out, err = RenderCloudInit(cloudConfig(), RoleCompose, HostKeys{DeployPublicKey: testDeployPublicKey})
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(ContainSubstring("ssh_keys"))
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

	out, err := RenderCloudInit(cfg, RoleCompose, testKeys())
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
