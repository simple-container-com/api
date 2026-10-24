// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	testEvolutionKeyID     = "f7b1c0a2-0000-4000-8000-000000000001"
	testEvolutionSecret    = "EVOLUTION-KEY-SECRET"
	testEvolutionProjectID = "11111111-aaaa-4bbb-8ccc-222222222222"
	testEvolutionAddress   = "176.108.247.222"
	// A real `ssh-keyscan -t ed25519` line, which is what an operator pastes: the
	// address prefix is part of it and the driver must not choke on it.
	testPinnedHostKey = "176.108.247.222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPINNED"
)

func evolutionConfig() *vm.VmConfig {
	cfg := &vm.VmConfig{
		Driver: vm.DriverCloudruEvolution,
		// No region, and that is the valid config rather than an omission: this cloud
		// has no region dimension at all, so `region:` is REFUSED here —
		// Capabilities.Regionless, pinned by TestValidateCloudRegionFollowsTheCapability
		// in the vm package.
		PublicIP: true,
	}
	cfg.Credentials.Credentials = `{"keyId":"` + testEvolutionKeyID + `","keySecret":"` + testEvolutionSecret +
		`","projectId":"` + testEvolutionProjectID + `"}`
	return cfg
}

// evolutionMocks fills in the two outputs a real run computes and nothing above
// them can proceed without: the generated deploy key (the VM spec is built inside an
// ApplyT over its public half, so without it the command is never created at all)
// and the apply command's stdout, which is where the machine's address comes from.
func evolutionMocks() *vmMocks {
	m := newVmMocks()
	result, err := json.Marshal(evolution.Result{
		Name: "portal-vm-host-0",
		VmResult: evolution.VmResult{
			VmID:       "0a4fdd94-f09f-44fd-8220-e6a4f8cd34a4",
			Address:    testEvolutionAddress,
			User:       vm.DefaultSshUser,
			Zone:       "ru-central-1a",
			FlavorName: "lowcost10-2-4",
		},
	})
	if err != nil {
		panic(err)
	}
	m.extraOutputs = map[string]resource.PropertyMap{
		tokenTlsPrivateKey: {
			"privateKeyOpenssh": resource.MakeSecret(resource.NewStringProperty(testGeneratedKey)),
			"publicKeyOpenssh":  resource.NewStringProperty(testGeneratedPub),
		},
		tokenLocalCommand: {"stdout": resource.NewStringProperty(string(result))},
	}
	return m
}

// inputsOfNamed is inputsOf keyed by RESOURCE NAME rather than type token: this
// driver creates several commands of the same type (the VM converge, the docker
// bootstrap, readiness) and the one under test has to be picked out by name.
func (m *vmMocks) inputsOfNamed(typeToken, nameSubstring string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken == typeToken && strings.Contains(a.Name, nameSubstring) {
			return a.Inputs
		}
	}
	return nil
}

// The spec is the only thing that decides what machine exists — it is what the
// converge command reads and what the trigger hashes — so every mapping in it is
// asserted here rather than discovered on a cloud that bills for the attempt.
func TestEvolutionVmSpec(t *testing.T) {
	RegisterTestingT(t)

	d := cloudruEvolutionDriver{}

	spec, err := d.vmSpec(evolutionConfig(), "portal-vm-host-0", testGeneratedPub, testPorts(evolutionConfig()))
	Expect(err).ToNot(HaveOccurred())

	Expect(spec.Name).To(Equal("portal-vm-host-0"))
	Expect(spec.Cores).To(Equal(vm.DefaultCores))
	Expect(spec.MemoryGiB).To(Equal(4), "DefaultMemory is 4Gi and the API reports RAM in whole GiB")
	// Never a flavor name. The catalogue renames sizes under us — the 2 vCPU / 4 GiB
	// flavor was `low-2-4` on 2026-10-23 and `lowcost10-2-4` on 2026-10-24, with no
	// `low-2-4` left — so the shape travels and the client resolves it at apply time.
	Expect(spec.Flavor).To(BeEmpty())
	Expect(spec.Image).To(Equal(vm.DefaultImage))
	Expect(spec.SSHUser).To(Equal(vm.DefaultSshUser))
	// Trimmed: tls.PrivateKey's publicKeyOpenssh ends in a newline, and the value goes
	// into an `authorized_keys`-shaped metadata slot where a trailing newline has
	// turned into a second (empty, invalid) key entry before.
	Expect(spec.PublicKey).To(Equal(strings.TrimSpace(testGeneratedPub)))
	Expect(spec.PublicKey).ToNot(HaveSuffix("\n"))
	// Always true: vm.Validate refuses `publicIP: false` on every cloud driver, so a
	// spec that did not ask for one would describe a machine SC could never reach.
	Expect(spec.PublicIP).To(BeTrue())
	// Empty, and that is the point of this cloud: `POST /vms` refuses `subnet_id` and
	// `security_group_ids` outright (measured 2026-10-24), and Hosts() refuses a
	// `network:` rather than filling these in with something that would be rejected.
	Expect(spec.SubnetID).To(BeEmpty())
	Expect(spec.SecurityGroupIDs).To(BeEmpty())
	// Unset unless the operator asked: the client falls back to DefaultVmDiskGiB, and
	// a number chosen here would be a second default to keep in step with it.
	Expect(spec.DiskGiB).To(Equal(0))
	Expect(spec.DiskType).To(BeEmpty())
	Expect(spec.Zone).To(BeEmpty(), "no zone configured means the client picks from /availability-zones")

	// And the configured shape travels whole.
	cfg := evolutionConfig()
	cfg.Zones = []string{"ru-central-1b"}
	cfg.Size = &vm.VmSize{Cores: 4, Memory: "8Gi", Disk: "60Gi", DiskType: "ssd"}
	spec, err = d.vmSpec(cfg, "portal-vm-host-0", testGeneratedPub, testPorts(cfg))
	Expect(err).ToNot(HaveOccurred())
	Expect(spec.Zone).To(Equal("ru-central-1b"))
	Expect(spec.Cores).To(Equal(4))
	Expect(spec.MemoryGiB).To(Equal(8))
	Expect(spec.DiskGiB).To(Equal(60))
	Expect(spec.DiskType).To(Equal("ssd"))

	// A size that is not a whole number of GiB is refused by name rather than
	// rounded, same as on the Advanced driver: the field is in the message because the
	// same value reaches here from two different config keys.
	cfg.Size = &vm.VmSize{Cores: 2, Memory: "1536Mi"}
	_, err = d.vmSpec(cfg, "portal-vm-host-0", testGeneratedPub, testPorts(cfg))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("size.memory"))

	cfg.Size = &vm.VmSize{Cores: 2, Memory: "4Gi", Disk: "half a terabyte"}
	_, err = d.vmSpec(cfg, "portal-vm-host-0", testGeneratedPub, testPorts(cfg))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("size.disk"))
}

// The firewall holes come from the SAME resolved port list the Advanced driver
// opens (vm.VmConfig.FirewallPorts), and that sharing is the assertion: 443/udp was
// added once because Caddy publishes HTTP/3 and advertises it via Alt-Svc, and a
// browser that takes the advertisement while the firewall drops the packets gets a
// stall rather than a fallback. A second, hand-maintained copy of the table is how
// that comes back.
func TestEvolutionIngressRules(t *testing.T) {
	RegisterTestingT(t)

	cfg := evolutionConfig()
	rules := evolutionIngressRules("portal-vm-host-0", testPorts(cfg))
	Expect(rules).To(HaveLen(1), "with no ingress the machine needs SSH and nothing else")
	Expect(rules[0].Protocol).To(Equal("tcp"))
	Expect(rules[0].PortRange).To(Equal(fmt.Sprintf("%d:%d", vm.DefaultSshPort, vm.DefaultSshPort)))
	// The VM's name is in the description, not just the label — on this cloud the
	// machine joins the project's SHARED default group, so a destroy that swept
	// every `simple-container ` rule would close a sibling VM's ports.
	Expect(rules[0].Description).To(Equal("simple-container vm portal-vm-host-0: ssh"))

	cfg.Ingress = &vm.VmIngress{Enable: true, Email: "ops@example.com"}
	rules = evolutionIngressRules("portal-vm-host-0", testPorts(cfg))

	// Sorted, and deterministically: the rules travel inside the spec the converge
	// command hashes, so a reordering would re-converge the machine on a deploy that
	// changed nothing.
	Expect(lo.Map(rules, func(r evolution.IngressRule, _ int) string { return r.Description })).
		To(Equal([]string{
			"simple-container vm portal-vm-host-0: http",
			"simple-container vm portal-vm-host-0: https",
			"simple-container vm portal-vm-host-0: https-h3",
			"simple-container vm portal-vm-host-0: ssh",
		}))
	Expect(lo.Map(rules, func(r evolution.IngressRule, _ int) string {
		return r.Protocol
	})).To(Equal([]string{"tcp", "tcp", "udp", "tcp"}))
	// The pairing that matters: 443 on BOTH protocols.
	Expect(lo.Filter(rules, func(r evolution.IngressRule, _ int) bool { return r.PortRange == "443:443" })).
		To(HaveLen(2))
	for _, r := range rules {
		Expect(r.Description).To(HavePrefix("simple-container vm portal-vm-host-0: "))
		Expect(r.PortRange).ToNot(BeEmpty())
		// Public, all four of them: these are the ingress and SSH.
		Expect(r.Internal).To(BeFalse())
	}

	// And the k3s preset, which is where `Internal` earns its keep: the API port and
	// the NodePort range are public, while the kubelet, flannel's VXLAN and embedded
	// etcd are node-to-node only. A multi-node cluster whose kubelet port were
	// public would be one anybody could `exec` into.
	cfg = evolutionConfig()
	k3sPorts, err := cfg.FirewallPorts(vm.IngressPresetK3s)
	Expect(err).ToNot(HaveOccurred())
	rules = evolutionIngressRules("portal-vm-host-0", k3sPorts)
	byLabel := lo.SliceToMap(rules, func(r evolution.IngressRule) (string, evolution.IngressRule) {
		return strings.TrimPrefix(r.Description, "simple-container vm portal-vm-host-0: "), r
	})
	Expect(byLabel["k3s-api"].Internal).To(BeFalse())
	Expect(byLabel["k3s-api"].PortRange).To(Equal("6443:6443"))
	Expect(byLabel["k3s-nodeport"].Internal).To(BeFalse())
	Expect(byLabel["k3s-nodeport"].PortRange).To(Equal("30000:32767"))
	Expect(byLabel["k3s-kubelet"].Internal).To(BeTrue())
	Expect(byLabel["k3s-flannel-vxlan"].Internal).To(BeTrue())
	Expect(byLabel["k3s-flannel-vxlan"].Protocol).To(Equal("udp"))
	Expect(byLabel["k3s-etcd"].Internal).To(BeTrue())
	Expect(byLabel["k3s-etcd"].PortRange).To(Equal("2379:2380"))
}

// testPorts is the port list the provisioning tier would have resolved for a
// config, without the parent-stack inference (which firewall_test.go covers).
func testPorts(cfg *vm.VmConfig) []vm.PortRange {
	ports, err := cfg.FirewallPorts()
	Expect(err).ToNot(HaveOccurred())
	return ports
}

// The address every later SSH connection dials is parsed out of a command's stdout,
// so the parser is load-bearing. Scanned from the END for a parseable object because
// stdout is contractually one JSON document but SC's own logger shares the stream.
func TestParseEvolutionResult(t *testing.T) {
	RegisterTestingT(t)

	doc := `{"name":"portal-vm-host-0","vmId":"vm-1","address":"` + testEvolutionAddress + `"}`

	res, err := parseEvolutionResult(doc)
	Expect(err).ToNot(HaveOccurred())
	Expect(res.Address).To(Equal(testEvolutionAddress))

	// Progress lines before it, which is what an operator running the command by hand
	// actually sees.
	res, err = parseEvolutionResult("creating disk...\nwaiting for running...\n" + doc + "\n")
	Expect(err).ToNot(HaveOccurred())
	Expect(res.Address).To(Equal(testEvolutionAddress))

	// From the end: an earlier object — a logger that writes JSON lines, a previous
	// invocation's output left in a buffer — must not win over the real result.
	res, err = parseEvolutionResult(`{"name":"stale","address":"203.0.113.9"}` + "\n" + doc)
	Expect(err).ToNot(HaveOccurred())
	Expect(res.Address).To(Equal(testEvolutionAddress))

	// A JSON object with no `name` is not a result document: `{}` parses cleanly and
	// would otherwise hand the stack an empty address, which presents as an SSH dial
	// to "" rather than as the failed converge it is.
	_, err = parseEvolutionResult("{}")
	Expect(err).To(HaveOccurred())
	_, err = parseEvolutionResult("something went wrong")
	Expect(err).To(HaveOccurred())
	// The error must not quote stdout: LoggingNone keeps the command's output out of
	// the log on purpose, and repeating it in an error would undo that.
	Expect(err.Error()).ToNot(ContainSubstring("something went wrong"))
}

// The bootstrap script is the whole cost of a cloud with no user-data, and it runs
// on every deploy. Its shape — guard first, as the login user — is what keeps that
// from being a re-install every time.
func TestDockerBootstrapScript(t *testing.T) {
	RegisterTestingT(t)

	script := dockerBootstrapScript("ubuntu")
	lines := strings.Split(strings.TrimSpace(script), "\n")

	Expect(lines[0]).To(Equal("set -eu"), "a script without `set -e` reports success for a failed install")

	// The guard comes before anything that touches the machine. `apt-get update`
	// appearing above the `exit 0` would mean every deploy pays for an apt run.
	guard := lo.IndexOf(lo.Map(lines, func(l string, _ int) bool {
		return strings.Contains(l, "command -v docker")
	}), true)
	update := lo.IndexOf(lo.Map(lines, func(l string, _ int) bool {
		return strings.Contains(l, "apt_retry update")
	}), true)
	Expect(guard).To(BeNumerically(">=", 0))
	Expect(update).To(BeNumerically(">", guard))

	// Checked as the login user, NOT as root: "docker is installed" is not the
	// property the compose path needs — it needs this user to reach the daemon, which
	// is group membership, and that is the half that silently goes missing when a
	// machine is rebuilt under the stack.
	Expect(script).To(ContainSubstring("if command -v docker >/dev/null 2>&1 && docker version >/dev/null 2>&1 && " +
		"docker compose version >/dev/null 2>&1; then"))
	Expect(script).To(ContainSubstring("echo docker-already-installed"))

	// apt, not the get.docker.com convenience script: measured reachable from inside
	// Evolution on 2026-10-23, and it does not pipe a remote shell script into root.
	Expect(script).To(ContainSubstring("https://download.docker.com/linux/ubuntu/gpg"))
	Expect(script).ToNot(ContainSubstring("get.docker.com"))
	// The codename comes from the machine, so one script serves 22.04 and 24.04 — the
	// image alias is the operator's choice and this must not second-guess it.
	Expect(script).To(ContainSubstring(". /etc/os-release"))
	Expect(script).To(ContainSubstring("${VERSION_CODENAME}"))
	// The compose v2 PLUGIN, which is what `docker compose` is. docker-compose (the
	// python v1 binary) is a different command and the generated compose files use v2
	// syntax.
	Expect(script).To(ContainSubstring("docker-compose-plugin"))

	Expect(script).To(ContainSubstring("sudo usermod -aG docker ubuntu"))
	// Verified as root afterwards, because the login shell of THIS session predates
	// the usermod — `docker version` as the user fails here even though it succeeds on
	// the next connection, and pulumi-command dials afresh for every command.
	Expect(script).To(ContainSubstring("sudo docker version"))
	Expect(script).To(ContainSubstring("sudo docker compose version"))

	// No lock timeout means apt fails instantly against unattended-upgrades, which
	// holds the dpkg lock for the first minutes after boot — i.e. exactly when this
	// runs.
	Expect(script).To(ContainSubstring("DPkg::Lock::Timeout"))
	Expect(script).To(ContainSubstring("DEBIAN_FRONTEND=noninteractive"))

	// And the lock timeout alone is not enough, which is the half that had to be
	// learnt live (2026-10-24, the first VM this driver created): it covers the dpkg
	// locks, while `apt-get update` also takes /var/lib/apt/lists/lock and gives that
	// one up on the first try. The image's own first-boot apt run holds it at exactly
	// the moment sshd starts answering, so EVERY apt call goes through the retry.
	Expect(script).To(ContainSubstring("apt_retry() {"))
	Expect(script).ToNot(MatchRegexp(`(?m)^\s*\$APT (update|install)`),
		"a bare $APT call races the image's first-boot apt and fails on the lists lock")
	// Retries, then fails — a bootstrap that gave up quietly would hand the compose
	// step a machine with no docker on it.
	Expect(script).To(ContainSubstring("exit 1"))
}

// Nothing may be interpolated into the converge command: an argument is readable in
// `ps` by every other process on the runner, and an environment variable by every
// other step of the job. The whole request goes to Stdin.
func TestEvolutionScriptCarriesNoArguments(t *testing.T) {
	RegisterTestingT(t)

	script := evolutionScript("compute-vm", "apply")
	Expect(script).To(ContainSubstring("sc cloudru compute-vm apply"))
	Expect(script).ToNot(ContainSubstring(testEvolutionSecret))
	Expect(script).ToNot(ContainSubstring(testEvolutionProjectID))
	// Constant, which is what lets the spec hash be the only trigger: a script whose
	// text moved with the config would re-converge on every edit of anything.
	Expect(script).To(Equal(evolutionScript("compute-vm", "apply")))
	Expect(evolutionScript("compute-vm", "destroy")).ToNot(Equal(script))
}

// The converge script must fail when `sc` printed no result document, and must fail
// in the SHELL — not later, in the Pulumi program.
//
// An `sc` older than the deploy does not error: `sc cloudru compute-vm` with an
// unknown subcommand prints the command group's help and EXITS 0. Without this check
// pulumi-command records a successful create whose stdout is that help text, and from
// then on the stack is wedged somewhere very hard to read — the program parses the
// stored stdout, so the next `preview` fails before planning anything and the stack
// can be neither re-deployed nor destroyed. Live-caught 2026-10-24.
func TestEvolutionApplyScriptFailsWhenNoResultDocumentCameBack(t *testing.T) {
	RegisterTestingT(t)

	script := evolutionResultScript("compute-vm", "apply")
	Expect(script).To(ContainSubstring("exit 1"), "a help screen must fail the create, not become its output")

	run := func(stdout string) error {
		dir := t.TempDir()
		// A stub `sc` earlier on PATH than any real one, printing what the case says
		// the command printed. The script's own PATH line prepends two directories, so
		// the stub goes in both.
		for _, sub := range []string{".local/bin", "bin"} {
			Expect(os.MkdirAll(filepath.Join(dir, sub), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, sub, "sc"),
				[]byte("#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '"+stdout+"'\n"), 0o755)).To(Succeed())
		}
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(),
			"HOME="+dir,
			"PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"))
		cmd.Stdin = strings.NewReader("{}")
		_, err := cmd.Output()
		return err
	}

	Expect(run(`{"name":"vm-1","address":"1.2.3.4"}`)).To(Succeed(),
		"a real result document passes, and its text reaches stdout unchanged")
	Expect(run("Converge Cloud.ru Evolution resources that no provider covers.")).ToNot(Succeed(),
		"the help screen an older `sc` prints must fail the command")
	Expect(run("")).ToNot(Succeed(), "and so must no output at all")
}

// The whole graph against mocks. Every assertion here is a property that would
// otherwise be learned from a live deploy against a billing account.
func TestEvolutionDriver_BuildsTheMachine(t *testing.T) {
	RegisterTestingT(t)

	cfg := evolutionConfig()
	cfg.HostKey = testPinnedHostKey

	mocks := evolutionMocks()
	out, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())
	Expect(out.Hosts).To(HaveLen(1))

	// pulumi-command's provider and NOTHING else: there is no Pulumi provider for
	// this cloud, which is the reason the driver exists in this shape.
	Expect(mocks.countOf(tokenCommandProvider)).To(Equal(1))
	Expect(mocks.countOf(tokenSbercloudProvider)).To(Equal(0),
		"Evolution is not SberCloud; sharing the brand is not sharing the API")

	// One deploy key, generated by SC. Exactly one — unlike the Advanced driver there
	// is no second key for the machine's host identity, because there is no user-data
	// to publish it in.
	Expect(mocks.countOf(tokenTlsPrivateKey)).To(Equal(1))

	converge := mocks.inputsOfNamed(tokenLocalCommand, "compute-vm")
	Expect(converge).ToNot(BeNil(), "the machine is converged by a local.Command running `sc cloudru`")

	// Update present, and the same script as Create. Without it pulumi-command turns
	// any input change into a REPLACEMENT performed as delete-then-create — which here
	// means destroying the machine, its boot disk and every stack on it because one
	// field of the spec moved.
	Expect(converge["create"].StringValue()).To(ContainSubstring("sc cloudru compute-vm apply"))
	Expect(converge["update"].StringValue()).To(Equal(converge["create"].StringValue()))
	Expect(converge["delete"].StringValue()).To(ContainSubstring("sc cloudru compute-vm destroy"))

	// Triggers present — without them a repeat `sc deploy` does nothing and reports
	// green — and exactly ONE of them: the spec hash. Deliberately not a nonce
	// (nonce.go): this command changes the cloud and cannot prove in advance that
	// there is nothing to change, so a nonce would re-converge the machine on every
	// deploy.
	triggers := converge["triggers"].ArrayValue()
	Expect(triggers).To(HaveLen(1))
	hash, _ := unwrapSecret(triggers[0])
	Expect(hash.StringValue()).To(MatchRegexp(`^[0-9a-f]{64}$`), "the trigger is a sha256 of the spec")

	// And the trigger must not be the credential-bearing request: trigger values are
	// rendered unencrypted in a Pulumi diff.
	Expect(hash.StringValue()).ToNot(ContainSubstring(testEvolutionSecret))

	// The request — credentials and spec — on Stdin, marked secret, so the checkpoint
	// holds it encrypted and no argument list carries it.
	stdin, secret := unwrapSecret(converge["stdin"])
	Expect(secret).To(BeTrue(), "the request carries a service-account key secret and the deploy private key")
	var req evolution.Request
	Expect(json.Unmarshal([]byte(stdin.StringValue()), &req)).To(Succeed())
	Expect(req.KeyID).To(Equal(testEvolutionKeyID))
	Expect(req.Secret).To(Equal(testEvolutionSecret))
	Expect(req.ProjectID).To(Equal(testEvolutionProjectID))
	Expect(req.VmSpec).ToNot(BeNil())
	Expect(req.Name).To(Equal(req.VmSpec.Name), "destroy finds the machine by name, so the two must agree")
	Expect(req.VmSpec.PublicKey).To(Equal(strings.TrimSpace(testGeneratedPub)))

	// No logging, and no previous output in the environment: the request is on stdout's
	// sibling stream and the command's own output is a JSON document with an address in
	// it, neither of which belongs in an Actions log.
	Expect(converge["logging"].StringValue()).To(Equal("none"))
	Expect(converge["addPreviousOutputInEnv"].BoolValue()).To(BeFalse())

	// Docker over SSH, because this cloud has no cloud-init to do it at first boot.
	bootstrap := mocks.inputsOfNamed(tokenRemoteCommand, "-docker")
	Expect(bootstrap).ToNot(BeNil())
	Expect(bootstrap["create"].StringValue()).To(ContainSubstring("docker-compose-plugin"))
	Expect(bootstrap["update"].StringValue()).To(Equal(bootstrap["create"].StringValue()))
	// Guarded change: a nonce (so drift under the stack is repaired) on a command
	// whose first act is to check the machine and exit, and which carries no Delete.
	Expect(bootstrap["triggers"].ArrayValue()).ToNot(BeEmpty())
	Expect(bootstrap["delete"].IsNull() || !bootstrap.HasValue("delete")).To(BeTrue(),
		"SC does not uninstall docker from a machine it is destroying anyway")
	Expect(bootstrap["logging"].StringValue()).To(Equal("none"))

	// The connection dials the address the apply printed — not a value from the
	// config, which does not have one — with the generated key, marked secret.
	conn, connSecret := unwrapSecret(bootstrap["connection"])
	connObj := conn.ObjectValue()
	address, _ := unwrapSecret(connObj["host"])
	Expect(address.StringValue()).To(Equal(testEvolutionAddress))
	// Marked at the object level or at the field level — both are correct, and which
	// one it is depends on how the Output was assembled. What would be wrong is
	// neither: an unwrapped deploy key is printed verbatim in Pulumi's update summary
	// and archived in the CI log.
	key, keySecret := unwrapSecret(connObj["privateKey"])
	Expect(connSecret || keySecret).To(BeTrue())
	Expect(key.StringValue()).To(Equal(testGeneratedKey))
	// Pinned, because the config pinned it: this is the one driver where the host key
	// is a property of the config rather than of a key SC generated.
	hostKey, _ := unwrapSecret(connObj["hostKey"])
	Expect(hostKey.StringValue()).To(Equal(testPinnedHostKey))

	Expect(out.Hosts[0].HostKeyPinned).To(BeTrue())
	Expect(out.Hosts[0].Role).To(Equal(vm.RoleCompose))
	Expect(out.Hosts[0].Port).To(Equal(vm.DefaultSshPort))
}

// Unpinned is the default and must still deploy — trust-on-first-use, warned about —
// but the connection must then carry NO hostKey at all rather than an empty one:
// pulumi-command fails every connection against an empty pin rather than falling
// back to accepting the key.
func TestEvolutionDriver_UnpinnedHostKeyOmitsThePin(t *testing.T) {
	RegisterTestingT(t)

	mocks := evolutionMocks()
	out, err := provisionVm(evolutionConfig(), mocks)
	Expect(err).To(BeNil())
	Expect(out.Hosts[0].HostKeyPinned).To(BeFalse())

	bootstrap := mocks.inputsOfNamed(tokenRemoteCommand, "-docker")
	conn, _ := unwrapSecret(bootstrap["connection"])
	hostKey, _ := unwrapSecret(conn.ObjectValue()["hostKey"])
	Expect(hostKey.IsNull() || hostKey.StringValue() == "").To(BeTrue())
}

// `network:` is refused rather than dropped, and the refusal happens before anything
// is created. `POST /vms` rejects every spelling of a subnet or security group
// (measured 2026-10-24, HTTP 422 `extra_forbidden`), so a config that looked accepted
// would describe a placement that silently did not happen — with the operator certain
// it had.
func TestEvolutionDriver_RefusesAConfiguredNetwork(t *testing.T) {
	RegisterTestingT(t)

	cfg := evolutionConfig()
	cfg.Network = &vm.VmNetwork{ID: "vpc-1", Subnets: map[string]string{"ru-central-1a": "sub-1"}}

	mocks := evolutionMocks()
	_, err := provisionVm(cfg, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`network` is not supported"))
	// The message names every refused spelling and the way out, because "not
	// supported" on its own leaves an operator with a VPC and nothing to do.
	Expect(err.Error()).To(ContainSubstring("security_group_ids"))
	Expect(err.Error()).To(ContainSubstring(vm.DriverByo))
	Expect(mocks.countOf(tokenLocalCommand)).To(Equal(0), "nothing may be created before the refusal")
}

// Credentials of the wrong type are a plausible copy-paste — the two Cloud.ru estates
// share a brand, a console and a credentials page — and the resulting failure
// otherwise surfaces as a 403 `non_authorized` from a cloud the operator did not mean
// to call.
func TestEvolutionDriver_RefusesCredentialsWithoutAProject(t *testing.T) {
	RegisterTestingT(t)

	cfg := evolutionConfig()
	// Cloud.ru ADVANCED credentials: an AK/SK pair, no project id.
	cfg.Credentials.Credentials = `{"accessKey":"` + testCloudruAccess + `","secretKey":"` + testCloudruSecret + `"}`

	_, err := provisionVm(cfg, evolutionMocks())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid Cloud.ru Evolution credentials"))
}
