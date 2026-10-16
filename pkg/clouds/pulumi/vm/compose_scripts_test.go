// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The scripts are pure functions of their inputs, which is the whole reason they live
// in their own file — so these tests need no Pulumi, no SSH and no docker, and they
// assert the two invariants the package comment names: everything interpolated is
// quoted, and no secret is ever interpolated.

const (
	testStack = "atrium"
	testEnv   = "staging"
	// hostileName is what an unvalidated name can look like. Stack and env reach the
	// scripts through vm.ValidateNames, but a volume name, a service name and a
	// registry URL do not, and a quoting bug in any of them is remote command
	// execution — on the machine for the remote scripts, and on the operator's own
	// runner for shipScript.
	hostileName = `x'; touch /tmp/sc-pwned; echo '`
)

// shellCheck runs `sh -n` over a generated script.
//
// A syntax check is a weak assertion about behaviour and a strong one about quoting:
// hostileName closes a quote, and every script below that interpolates it unquoted
// either fails to parse or parses as something other than one word. Asserting on the
// text alone would not catch it — `touch /tmp/sc-pwned` appears in the script either
// way, correctly quoted or not.
func shellCheck(t *testing.T, script string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this box; skipping the syntax check")
	}
	f := filepath.Join(t.TempDir(), "script.sh")
	Expect(os.WriteFile(f, []byte(script), 0o600)).To(Succeed())

	out, err := exec.Command(sh, "-n", f).CombinedOutput()
	Expect(err).To(BeNil(), "generated script is not valid POSIX sh: %s\n--- script ---\n%s", out, script)
}

// decodedBlobs returns every base64 literal the script carries, decoded. The compose
// files and the state file are delivered this way, so this is how a test reads what
// the machine would end up with.
var blobRe = regexp.MustCompile(`printf %s '([A-Za-z0-9+/=]+)'`)

func decodedBlobs(script string) []string {
	var out []string
	for _, m := range blobRe.FindAllStringSubmatch(script, -1) {
		b, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			continue
		}
		out = append(out, string(b))
	}
	return out
}

// Asserted by round-tripping through a real shell rather than by inspecting the
// quoted text. Inspection is what makes this kind of test pass while the bug ships:
// the escape sequence a correct quoter emits (`'\”`) contains a quote followed by
// whatever came next, so "must not contain `'; touch`" is false for the CORRECT
// output. What actually matters is that sh reads the result back as one word equal to
// the input, and only sh can say that.
func TestShellQuote_RoundTripsThroughARealShell(t *testing.T) {
	RegisterTestingT(t)

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this box")
	}
	for _, in := range []string{
		hostileName,
		`$(whoami)`,
		"`id`",
		`a b	c`,
		`back\slash`,
		`"double"`,
		`'`,
		``,
	} {
		out, err := exec.Command(sh, "-c", "printf %s "+shellQuote(in)).Output()
		Expect(err).To(BeNil(), "sh refused the quoting of %q", in)
		Expect(string(out)).To(Equal(in), "%q did not survive quoting as a single word", in)
	}
}

// The env file and the textVolumes are the two deliveries whose content is secret.
// Their content must reach the machine on stdin and must NOT be in the script.
func TestWriteFileScript_CarriesNoContentAndIsAtomic(t *testing.T) {
	RegisterTestingT(t)

	script := writeFileScript(vm.EnvFilePath(testStack, testEnv), vm.EnvFileMode)
	shellCheck(t, script)

	Expect(script).To(ContainSubstring(`cat > "$t"`), "content comes from stdin")
	// umask before the file is created, not chmod after: the window between create
	// and chmod is a window in which another user on the machine can read a secret.
	Expect(strings.Index(script, "umask 077")).To(BeNumerically("<", strings.Index(script, `cat > "$t"`)))
	Expect(script).To(ContainSubstring(`mv -f "$t" `+shellQuote(vm.EnvFilePath(testStack, testEnv))),
		"written to a temp name and renamed, so an interrupted deploy leaves no truncated env file")
	Expect(script).To(ContainSubstring(`chmod '0600' "$t"`))
}

func TestWriteFileScript_QuotesAHostilePath(t *testing.T) {
	RegisterTestingT(t)

	// A textVolume's mountPath is customer input and becomes part of the path.
	script := writeFileScript("/etc/sc/"+hostileName+"/f", vm.TextVolumeMode)

	shellCheck(t, script)
	Expect(script).NotTo(MatchRegexp(`(?m)^touch /tmp/sc-pwned`))
}

func TestPrepareScript_CreatesBothNetworksAndEveryVolume(t *testing.T) {
	RegisterTestingT(t)

	script := prepareScript(testStack, testEnv, []string{"mongo-data", hostileName})
	shellCheck(t, script)

	Expect(script).To(ContainSubstring(shellQuote(vm.IngressNetwork)),
		"the shared ingress network is external to every project")
	Expect(script).To(ContainSubstring(shellQuote(vm.InternalNetworkName(testStack, testEnv))))
	Expect(script).To(ContainSubstring("docker volume create " + shellQuote(vm.ExternalVolumeName(testStack, testEnv, "mongo-data"))))
	Expect(script).To(ContainSubstring(shellQuote(vm.ExternalVolumeName(testStack, testEnv, hostileName))))
	// Create-then-assert, so that two stacks deploying at once do not both fail on
	// "network already exists" while a genuine failure still fails.
	Expect(script).To(ContainSubstring("|| true"))
	Expect(script).To(ContainSubstring("failed to create docker network"))
}

func TestRegistryLoginScript_PasswordOnlyOnStdin(t *testing.T) {
	RegisterTestingT(t)

	script := registryLoginScript("registry.example.ru/forge", "robot$ci")
	shellCheck(t, script)

	Expect(script).To(ContainSubstring("--password-stdin"))
	Expect(script).NotTo(ContainSubstring("-p "))
	Expect(script).To(ContainSubstring(`-u 'robot$ci'`), "a username with a shell metacharacter stays one word")
}

func TestShipScript_KeyOnStdinAndAlwaysRemoved(t *testing.T) {
	RegisterTestingT(t)

	script := shipScript(shipSpec{
		ImageRef: "sc-local/atrium-staging-app:2026.10.16-abc1234",
		Host:     "203.0.113.10",
		User:     vm.DefaultSshUser,
		Port:     vm.DefaultSshPort,
		HostKey:  "ssh-ed25519 AAAAC3Nz",
	})
	shellCheck(t, script)

	Expect(script).To(ContainSubstring(`cat > "$d/key"`), "the deploy key arrives on stdin")
	Expect(script).To(ContainSubstring("umask 077"))
	// EXIT INT TERM, not just EXIT: a cancelled workflow is the case where a key left
	// on a shared runner matters most.
	Expect(script).To(ContainSubstring(`trap 'rm -rf "$d"' EXIT INT TERM`))
	Expect(script).To(ContainSubstring("--host-key 'ssh-ed25519 AAAAC3Nz'"))
	Expect(script).NotTo(ContainSubstring("--insecure-skip-host-key-check"))
}

// Without a pinned host key the ship must say so in the command rather than inherit
// `sc image ship`'s default, which verifies against a ~/.ssh/known_hosts that does
// not exist on a CI runner — every ship would fail, and the fix reached for under
// time pressure is to disable verification permanently.
func TestShipScript_SkipsVerificationExplicitlyWhenUnpinned(t *testing.T) {
	RegisterTestingT(t)

	script := shipScript(shipSpec{ImageRef: "sc-local/a:1", Host: "h", User: "sc", Port: 22})

	shellCheck(t, script)
	Expect(script).To(ContainSubstring("--insecure-skip-host-key-check"))
}

func blueGreenUp() upSpec {
	return upSpec{
		Stack:       testStack,
		Env:         testEnv,
		Slot:        vm.SlotB,
		OtherSlot:   vm.SlotA,
		SlotProject: vm.ProjectName(testStack, testEnv, vm.SlotB),
		SlotContent: "services:\n  app:\n    image: sc-local/atrium-staging-app:1\n",
		SlotGates:   []string{"app"},
		Pull:        pullBestEffort,
	}
}

func TestUpScript_BlueGreenLeavesTheServingSlotAlone(t *testing.T) {
	RegisterTestingT(t)

	script := upScript(blueGreenUp())
	shellCheck(t, script)

	Expect(script).NotTo(ContainSubstring("sc_rm "+shellQuote(vm.ProjectName(testStack, testEnv, vm.SlotA))),
		"the whole point of blue-green: slot a keeps serving until the new slot is healthy")
	Expect(script).To(ContainSubstring("sc_wait " + shellQuote(vm.ProjectName(testStack, testEnv, vm.SlotB))))
	Expect(script).To(ContainSubstring("still serving traffic"), "the rollback message names what is still up")
	Expect(script).To(ContainSubstring("{ sc_rollback; exit 1; }"))
}

// The compose file is delivered base64-encoded rather than as a heredoc, because a
// heredoc delimiter is a string that must not appear at the start of a line in a
// customer's compose file.
func TestUpScript_DeliversComposeFilesEncoded(t *testing.T) {
	RegisterTestingT(t)

	s := blueGreenUp()
	s.SlotContent = "services:\n  app:\n    command: [\"sh\", \"-c\", \"echo 'EOF' && exit 0\"]\n"
	script := upScript(s)

	shellCheck(t, script)
	Expect(script).NotTo(ContainSubstring("echo 'EOF'"), "content is encoded, never interpolated")
	Expect(decodedBlobs(script)).To(ContainElement(s.SlotContent))
	Expect(script).To(ContainSubstring("mv -f " + shellQuote(vm.ComposeFilePath(testStack, testEnv, vm.SlotB)+".new")))
}

func TestUpScript_StatefulProjectComesUpAndIsGatedFirst(t *testing.T) {
	RegisterTestingT(t)

	s := blueGreenUp()
	s.DataProject = vm.DataProjectName(testStack, testEnv)
	s.DataContent = "services:\n  mongo:\n    image: mongo:7\n"
	s.DataGates = []string{"mongo"}
	script := upScript(s)

	shellCheck(t, script)
	// depends_on does not cross compose projects, so the ordering here is the only
	// thing that keeps the backend from starting before its database.
	dataWait := strings.Index(script, "sc_wait "+shellQuote(s.DataProject))
	slotUp := strings.Index(script, "docker compose -p "+shellQuote(s.SlotProject)+" -f "+shellQuote(vm.ComposeFilePath(testStack, testEnv, vm.SlotB))+" up")
	Expect(dataWait).To(BeNumerically(">", 0))
	Expect(dataWait).To(BeNumerically("<", slotUp))
}

// A published host port is a singleton, so until Caddy fronts the stack the old slot
// has to go first — and the rollback has to be able to bring it back.
func TestUpScript_StopOtherFirstWarnsAndCanRestore(t *testing.T) {
	RegisterTestingT(t)

	s := blueGreenUp()
	s.StopOtherFirst = true
	script := upScript(s)
	shellCheck(t, script)

	otherProject := vm.ProjectName(testStack, testEnv, vm.SlotA)
	Expect(script).To(ContainSubstring("sc_rm " + shellQuote(otherProject)))
	Expect(script).To(ContainSubstring("brief downtime"), "downtime is announced, not discovered")
	Expect(script).To(ContainSubstring("restoring the previous slot"))
	Expect(script).To(ContainSubstring(shellQuote(vm.ComposeFilePath(testStack, testEnv, vm.SlotA))),
		"rollback brings slot a back up from the file that is still on disk")
	// Stopped before the new slot is started, not after: the point is the port.
	Expect(strings.LastIndex(script, "sc_rm "+shellQuote(otherProject))).
		To(BeNumerically("<", strings.Index(script, "up -d --remove-orphans || { sc_rollback")))
}

func TestUpScript_RecreateSaysThereIsNothingToRollBackTo(t *testing.T) {
	RegisterTestingT(t)

	s := blueGreenUp()
	s.Slot, s.OtherSlot = vm.SlotA, vm.SlotB
	s.SlotProject = vm.ProjectName(testStack, testEnv, vm.SlotA)
	s.Recreate = true
	script := upScript(s)

	shellCheck(t, script)
	Expect(script).To(ContainSubstring("is DOWN until a deploy succeeds"))
	Expect(script).NotTo(ContainSubstring("still serving traffic"))
}

// ssh-tar delivery streams the stack's own images onto the machine, so they exist in
// no registry and pulling them fails by design — while a third-party mongo still has
// to be pulled. Registry delivery is the opposite: a pull failure is the deploy's.
func TestUpScript_PullModeMatchesDelivery(t *testing.T) {
	RegisterTestingT(t)

	s := blueGreenUp()
	Expect(upScript(s)).To(ContainSubstring("pull --ignore-pull-failures || true"))

	s.Pull = pullRequired
	registry := upScript(s)
	Expect(registry).To(ContainSubstring(" pull\n"))
	Expect(registry).NotTo(ContainSubstring("--ignore-pull-failures"))
}

// finalizeScript writes the state file and readStateScript reads it. They are the two
// halves of one format, so the test asserts they agree rather than asserting each
// against a literal.
func TestFinalizeScript_WritesStateThatReadStateCanParse(t *testing.T) {
	RegisterTestingT(t)

	script := finalizeScript(finalizeSpec{
		Stack: testStack, Env: testEnv,
		Slot: vm.SlotB, StopSlot: vm.SlotA,
		Version:    "2026.10.16-abc1234",
		PruneRepos: []string{"sc-local/atrium-staging-app"},
		Keep:       3,
	})
	shellCheck(t, script)

	blobs := decodedBlobs(script)
	Expect(blobs).To(HaveLen(1))
	state := blobs[0]

	reader := readStateScript(testStack, testEnv)
	shellCheck(t, reader)
	// The exact pattern the reader greps for, applied to what the writer wrote.
	pattern := regexp.MustCompile(`"activeSlot":"[ab]"`)
	Expect(pattern.FindString(state)).To(Equal(`"activeSlot":"b"`))
	Expect(reader).To(ContainSubstring(`grep -o '"activeSlot":"[ab]"'`))
	Expect(reader).To(ContainSubstring(shellQuote(vm.StatePath(testStack, testEnv))))

	Expect(script).To(ContainSubstring("sc_rm "+shellQuote(vm.ProjectName(testStack, testEnv, vm.SlotA))),
		"the old slot is retired only after the new one is recorded as active")
	// keep=3 means four tags survive: the three previous versions plus the one just
	// deployed. Off by one here deletes the image a rollback needs.
	Expect(script).To(ContainSubstring("sc_prune 'sc-local/atrium-staging-app' 4"))
}

// A `sc destroy` that takes the database with it is a footgun with no undo.
func TestTeardownScript_KeepsVolumesAndTheSharedNetwork(t *testing.T) {
	RegisterTestingT(t)

	script := teardownScript(testStack, testEnv)
	shellCheck(t, script)

	Expect(script).NotTo(ContainSubstring("docker volume rm"))
	Expect(script).NotTo(ContainSubstring("down -v"))
	Expect(script).NotTo(ContainSubstring("docker network rm "+shellQuote(vm.IngressNetwork)),
		"sc-ingress is shared with every other stack on the machine")
	Expect(script).To(ContainSubstring("docker network rm " + shellQuote(vm.InternalNetworkName(testStack, testEnv))))
	for _, slot := range vm.Slots {
		Expect(script).To(ContainSubstring("sc_rm " + shellQuote(vm.ProjectName(testStack, testEnv, slot))))
	}
	// The env file holds every secret of the stack; there is no reason to leave it on
	// a machine that no longer runs it.
	Expect(script).To(ContainSubstring("rm -rf " + shellQuote(vm.StackDir(testStack, testEnv))))
}

func TestShellHelpers_WaitGatesOnRunningAndHealthy(t *testing.T) {
	RegisterTestingT(t)

	h := shellHelpers()
	shellCheck(t, "set -eu\n"+h)

	// A container with no healthcheck reports health "none" and must still pass —
	// otherwise every service without one would hang the deploy for 120s.
	Expect(h).To(ContainSubstring(`[ "$h" = "healthy" ] || [ "$h" = "none" ]`))
	Expect(h).To(ContainSubstring(`[ "$st" = "running" ]`))
	Expect(h).To(ContainSubstring("docker logs --tail 200"), "a failed gate prints why")
	// Removal is by compose label, not by `docker compose down -f <file>`: the file
	// for a slot being retired may predate a rename or a restored snapshot.
	Expect(h).To(ContainSubstring(`--filter "label=com.docker.compose.project=$p"`))
}
