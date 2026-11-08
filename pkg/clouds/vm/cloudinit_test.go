// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

// The byo install script is a shell program SC runs as root-by-sudo on a machine it
// does not own, so each assertion below is about a way it could do damage or lie
// rather than about its wording.
func TestByoDockerInstallScript(t *testing.T) {
	RegisterTestingT(t)

	script := ByoDockerInstallScript("deploy")

	// Guarded, in both directions: it must ask the machine before downloading
	// anything (this is what makes the command a TriggersForGuardedChange, whose
	// whole premise is that a second run changes nothing), and it must assert
	// afterwards rather than trust the installer's exit code.
	Expect(script).To(HavePrefix("set -eu\n"))
	Expect(script).To(ContainSubstring("if command -v docker >/dev/null 2>&1; then"))
	Expect(script).To(ContainSubstring("curl -fsSL " + DockerInstallURL + " | sudo sh"))
	Expect(strings.Count(script, DockerInstallURL)).To(BeNumerically(">=", 2),
		"the install URL has to appear in the failure message too — it is the host an operator "+
			"has to be told they cannot reach")
	Expect(script).To(HaveSuffix("echo docker-ready\n"))

	// compose v2 is checked separately from docker: an engine with no plugin is the
	// common shape on a hand-built host, and it fails much later and far less
	// legibly (`docker compose` is not a command) if nothing checks here.
	Expect(script).To(ContainSubstring("sudo docker compose version"))
	Expect(script).To(ContainSubstring("docker-compose-plugin"))

	// The daemon, not just the binary. An installed-but-dead docker is what a
	// half-finished hand install leaves behind.
	Expect(script).To(ContainSubstring("sudo systemctl enable --now docker"))
	Expect(script).To(ContainSubstring("sudo docker version >/dev/null 2>&1 ||"))

	// NOT a backtick anywhere. Every message here is inside a double-quoted sh
	// string, where a backtick is command substitution — so quoting a command in
	// prose makes the error path RUN it. Live-caught while writing this script: a
	// hint that read "`docker-compose` v1 is not supported" executed docker-compose.
	Expect(script).ToNot(ContainSubstring("`"))
}

// The login name reaches a sudo command line, so it is quoted. EffectiveUser already
// constrains it, but that constraint lives in another file and this is the line that
// would carry the surprise.
func TestByoDockerInstallScriptQuotesTheUser(t *testing.T) {
	RegisterTestingT(t)

	script := ByoDockerInstallScript("deploy; rm -rf /")

	Expect(script).To(ContainSubstring(`sudo usermod -aG docker 'deploy; rm -rf /'`))
	Expect(script).ToNot(ContainSubstring("docker deploy; rm -rf /"))
	Expect(ByoDockerInstallScript(`o'brien`)).To(ContainSubstring(`sudo usermod -aG docker 'o'\''brien'`))
}

// The unprivileged check runs ONLY on the branch where the user was already in the
// group. `usermod -aG` does not apply to the session that ran it, so asserting
// `docker version` as that user immediately after adding them fails on a host that
// is in fact correctly set up — and pulumi-command opens a fresh login per command,
// so the next thing SC runs picks the group up anyway.
func TestByoDockerInstallScriptChecksTheUserOnlyWhenAlreadyInTheGroup(t *testing.T) {
	RegisterTestingT(t)

	lines := strings.Split(ByoDockerInstallScript("deploy"), "\n")
	var branch, unprivileged, usermod int
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "if id -nG "):
			branch = i
		case strings.HasPrefix(l, "  docker version >/dev/null"):
			unprivileged = i
		case strings.Contains(l, "usermod -aG docker"):
			usermod = i
		}
	}
	Expect(branch).To(BeNumerically(">", 0), "the group-membership branch is gone: %v", lines)
	Expect(unprivileged).To(BeNumerically(">", branch),
		"the unprivileged `docker version` must be inside the already-in-the-group branch")
	Expect(usermod).To(BeNumerically(">", unprivileged),
		"the usermod belongs in the else branch, after the unprivileged check")
}

// The script is executed, with the privileged binaries stubbed, because that is the
// only way to assert the thing that matters about a hostile login name: not that the
// text looks escaped, but that a real sh never evaluates it. Inspection cannot tell
// the two apart (see TestShellQuote_RoundTripsThroughARealShell in the provisioning
// tier for the same argument).
func runByoDockerInstall(t *testing.T, user string) (string, []string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this box")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	Expect(os.MkdirAll(bin, 0o700)).To(Succeed())
	log := filepath.Join(dir, "invocations")

	// docker present, so nothing is downloaded; sudo transparent; the rest record
	// what they were asked to do. systemctl is deliberately absent — the script has
	// to treat a host without it as fine.
	for name, body := range map[string]string{
		"docker":   "#!/bin/sh\nexit 0\n",
		"sudo":     "#!/bin/sh\nexec \"$@\"\n",
		"groupadd": "#!/bin/sh\nprintf 'groupadd %s\\n' \"$*\" >> \"$SC_TEST_LOG\"\n",
		"usermod":  "#!/bin/sh\nfor a in \"$@\"; do printf 'usermod-arg %s\\n' \"$a\" >> \"$SC_TEST_LOG\"; done\n",
	} {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700)).To(Succeed())
	}

	script := filepath.Join(dir, "docker.sh")
	Expect(os.WriteFile(script, []byte(ByoDockerInstallScript(user)), 0o600)).To(Succeed())

	cmd := exec.Command(sh, script)
	// The stubs shadow anything real; the system paths follow because the script
	// legitimately uses id, tr and grep.
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "SC_TEST_LOG=" + log}
	out, runErr := cmd.CombinedOutput()
	Expect(runErr).ToNot(HaveOccurred(), "script failed: %s", out)

	var calls []string
	if b, err := os.ReadFile(log); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if l != "" {
				calls = append(calls, l)
			}
		}
	}
	return string(out), calls
}

func TestByoDockerInstallScript_RunsNothingWhenDockerIsPresent(t *testing.T) {
	RegisterTestingT(t)

	out, calls := runByoDockerInstall(t, "deploy")

	Expect(out).To(ContainSubstring("docker is already installed"))
	Expect(out).To(ContainSubstring("docker-ready"))
	Expect(out).ToNot(ContainSubstring("installing docker engine"))
	// `groupadd -f` is the one unconditional mutation, and it is a no-op on a host
	// that already has the group — which is every host that has docker.
	Expect(calls).To(ContainElement("groupadd -f docker"))
}

// A login name is operator-supplied config. It reaches a sudo command line and two
// printf arguments, and in none of the three may a shell evaluate it.
func TestByoDockerInstallScript_NeverEvaluatesTheUserName(t *testing.T) {
	RegisterTestingT(t)

	for _, user := range []string{
		`x'; touch /tmp/sc-docker-pwned; echo '`,
		`$(touch /tmp/sc-docker-pwned)`,
		"`touch /tmp/sc-docker-pwned`",
		`a b	c`,
	} {
		out, calls := runByoDockerInstall(t, user)

		// The real `id` rejects these names, so the script takes the else branch and
		// hands the name to usermod — as exactly one argument, unchanged.
		Expect(calls).To(ContainElement("usermod-arg "+user),
			"%q did not survive as a single argument; stub saw %v", user, calls)
		Expect(out).To(ContainSubstring("docker-ready"))

		_, err := os.Stat("/tmp/sc-docker-pwned")
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"the script evaluated %q: /tmp/sc-docker-pwned exists", user)
	}
}
