// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The shell scripts a vm-compose deploy runs on the machine.
//
// Kept in one file, as pure string-building functions, for the same reason
// pkg/clouds/vm/render.go is a pure function: these are the part of the deploy that
// an operator ends up reading when something misbehaves, and being able to produce
// them in a unit test — with no Pulumi, no SSH and no cloud — is what keeps them
// reviewable.
//
// Two conventions hold throughout, and both are load-bearing:
//
//   - **Every value interpolated into a script is shell-quoted** (shellQuote) or
//     base64-encoded (encodeFile). A stack name reaches these functions validated
//     (vm.ValidateNames), but a volume name, a service name or a file path does not,
//     and a quoting bug here is a remote command injection on the operator's machine.
//   - **No secret is ever interpolated.** Secrets arrive on stdin
//     (remote.CommandArgs.Stdin), which pulumi-command encrypts in the checkpoint;
//     a value in the command string is stored in the clear and printed in Pulumi's
//     diff (compose-on-vm.md §0.7).
const (
	// healthWaitAttempts × healthWaitInterval is how long a service gets to report
	// healthy before the deploy gives up and rolls back. Two minutes is chosen
	// against the thing being waited for: a container image that is already on the
	// machine, starting a process that answers a `nc -z` or an HTTP probe. A service
	// that needs longer than this on a cold start should say so in its own
	// healthcheck's `start_period`, which docker honours before the first failure
	// counts — not here, where the number applies to every service of every stack.
	healthWaitAttempts = 24
	healthWaitInterval = 5
)

// shellQuote renders a value as a single POSIX sh word.
//
// The docker package has the same three lines (unexported, build_and_push.go). They
// are duplicated rather than shared for the reason the secretString comment gives:
// the alternative to three duplicated lines is a utility package every provider
// imports for one function.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// encodeFile renders file content as a base64 literal.
//
// Not a heredoc, which is the obvious choice and the wrong one: a heredoc delimiter
// is a string that must not appear at the start of a line in the content, and the
// content here includes a customer's compose file with their command lines and their
// Caddy snippets in it. base64 cannot collide with its own delimiter, has no
// characters sh treats specially, and survives a file with CRLF or a trailing byte
// that is not a newline. The cost is that the file is unreadable in the command
// string — which is also where it would otherwise be readable by anyone with the
// checkpoint, and the file itself lands on the machine in plain text.
func encodeFile(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}

// writeFileScript writes stdin to path with the given mode, atomically.
//
// Used for the env file and the rendered textVolumes — the two deliveries whose
// content is secret and therefore must not be in the command string. `install -m
// <mode> /dev/stdin <path>` would be shorter and is deliberately avoided: GNU
// install stats its source and a pipe is not a regular file, so the one-liner works
// on a workstation (where /dev/stdin may be a file) and fails on a CI runner.
func writeFileScript(filePath, mode string) string {
	return strings.Join([]string{
		"set -eu",
		fmt.Sprintf("install -d -m 0750 %s", shellQuote(path.Dir(filePath))),
		// umask before the file exists: creating it 0644 and chmod'ing afterwards
		// leaves a window in which another user on the machine can read a secret.
		"umask 077",
		fmt.Sprintf("t=%s", shellQuote(filePath+".new")),
		`cat > "$t"`,
		fmt.Sprintf(`chmod %s "$t"`, shellQuote(mode)),
		// Rename, so that a deploy interrupted mid-write cannot leave a truncated
		// env file behind for the next `docker compose up` to read.
		fmt.Sprintf(`mv -f "$t" %s`, shellQuote(filePath)),
	}, "\n") + "\n"
}

// readStateScript prints the slot currently serving traffic, or nothing.
//
// Parsed with grep and cut rather than jq: jq is not installed on a minimal image
// and SC does not install packages on a byo machine. The state file is written by
// finalizeScript below, so the format on both sides is fixed and this is not
// general-purpose JSON parsing.
//
// Printing nothing (rather than failing) is the correct answer for a machine that
// has never been deployed to; the caller turns an empty answer into vm.SlotA.
func readStateScript(stack, env string) string {
	return strings.Join([]string{
		// Deliberately no `set -e`: grep exits non-zero when the file exists but
		// holds no slot, and that is a valid "no active slot", not a failure.
		"set -u",
		fmt.Sprintf("f=%s", shellQuote(vm.StatePath(stack, env))),
		`if [ -f "$f" ]; then`,
		`  grep -o '"activeSlot":"[ab]"' "$f" 2>/dev/null | head -1 | cut -d'"' -f4`,
		`fi`,
	}, "\n") + "\n"
}

// composePreflightScript asserts the machine can actually run a compose deploy.
//
// This used to live in the vm resource's readiness gate (ready.go), where it was
// wrong in one direction only: it also fired for a machine that was never going to
// run docker. A byo host declared for a `k3s-cluster` has containerd and no docker,
// and failed its readiness check on `docker is not installed` before the k3s install
// — which DependsOn that gate — ever ran. Readiness is a property of the machine;
// docker is a property of this deploy path, so the check belongs here, as the first
// step of the chain.
//
// Nothing is lost by checking later: the step runs before anything touches docker,
// and it is still the thing that turns `docker: command not found` four commands
// deeper into a sentence naming what to install. That last sentence is only true
// because the step carries the per-run nonce — its script depends on nothing but the
// ssh user, so with the package's default trigger set it would assert docker on the
// first deploy of a stack and never again (nonce.go, and the call site in
// VmComposeStack).
func composePreflightScript(user string) string {
	return strings.Join([]string{
		`set -eu`,
		fail(`command -v docker >/dev/null 2>&1`,
			"docker is not installed on this host; install Docker Engine with the compose v2 plugin, "+
				"or let SC create the machine with a cloud vm provider"),
		fail(`docker version >/dev/null 2>&1`,
			fmt.Sprintf("user %q cannot talk to the docker daemon; add it to the docker group "+
				"(usermod -aG docker %s) and reconnect", user, user)),
		fail(`docker compose version >/dev/null 2>&1`,
			"the docker compose v2 plugin is missing; `docker-compose` v1 is not supported"),
		`echo preflight-ok`,
	}, "\n") + "\n"
}

// prepareScript creates what both compose projects declare as external: the shared
// ingress network, the stack-env's internal network, the named volumes, and the
// directories under /etc/sc and /var/lib/sc.
//
// External on purpose (compose-on-vm.md §6 rules 1 and 4): a network shared by two
// slots and a data project cannot be any one project's `default`, and a volume
// inside a project is deleted by `docker compose down -v` of the slot that is being
// retired.
func prepareScript(stack, env string, volumes []string) string {
	lines := []string{
		"set -eu",
		ensureNetwork(vm.IngressNetwork),
		ensureNetwork(vm.InternalNetworkName(stack, env)),
	}
	for _, v := range volumes {
		name := vm.ExternalVolumeName(stack, env, v)
		lines = append(
			lines,
			fmt.Sprintf("docker volume inspect %s >/dev/null 2>&1 || docker volume create %s >/dev/null",
				shellQuote(name), shellQuote(name)),
		)
	}
	lines = append(
		lines,
		fmt.Sprintf("install -d -m 0750 %s", shellQuote(vm.StackDir(stack, env))),
		fmt.Sprintf("install -d -m 0750 %s", shellQuote(vm.TextVolumeDir(stack, env))),
		fmt.Sprintf("install -d -m 0750 %s", shellQuote(vm.StateDir+"/state")),
		`echo prepared`,
	)
	return strings.Join(lines, "\n") + "\n"
}

// ensureNetwork is create-if-absent, then assert.
//
// The assert is not redundant with the create. Two stacks deploying onto one machine
// at the same time both find sc-ingress missing and both create it; one of them gets
// "network with name sc-ingress already exists" and must not fail — but a create
// that failed for a real reason (out of address space, a conflicting subnet) must
// not pass either. Swallowing the error and then checking distinguishes them.
func ensureNetwork(name string) string {
	q := shellQuote(name)
	return fmt.Sprintf(
		"docker network inspect %s >/dev/null 2>&1 || docker network create %s >/dev/null 2>&1 || true\n"+
			"docker network inspect %s >/dev/null 2>&1 || { echo \"failed to create docker network %s\" >&2; exit 1; }",
		q, q, q, name,
	)
}

// registryLoginScript authenticates the machine against the image registry. The
// password arrives on stdin, which is what --password-stdin exists for: a password
// in `docker login -p` lands in the machine's shell history and in `ps`.
func registryLoginScript(registryURL, username string) string {
	return strings.Join([]string{
		"set -eu",
		fmt.Sprintf("docker login %s -u %s --password-stdin",
			shellQuote(registryURL), shellQuote(username)),
	}, "\n") + "\n"
}

// shipSpec is one `sc image ship` invocation. Unlike every other script here this
// one runs on the deploy runner, not on the machine.
type shipSpec struct {
	ImageRef string
	Host     string
	User     string
	Port     int
	// HostKey is the pinned host key, or empty — in which case the ship is told to
	// skip the check explicitly. See shipScript for why that is not the default.
	HostKey string
}

// shipScript streams a locally-built image to the machine over SSH.
//
// The deploy key arrives on stdin and is written to a file because that is what
// `--ssh-key` takes — a path. The alternatives are worse: `$SC_SSH_KEY` is also a
// path, and passing the key material as an argument or an environment variable puts
// it in `ps` output and in the runner's process environment, where every other step
// of the job can read it. mktemp under umask 077 plus a trap means the file is
// readable only by the runner user and is gone whether the ship succeeds, fails, or
// is interrupted.
func shipScript(s shipSpec) string {
	args := []string{
		"sc image ship",
		"--image " + shellQuote(s.ImageRef),
		"--host " + shellQuote(s.Host),
		"--user " + shellQuote(s.User),
		fmt.Sprintf("--port %d", s.Port),
		`--ssh-key "$d/key"`,
	}
	if s.HostKey != "" {
		args = append(args, "--host-key "+shellQuote(s.HostKey))
	} else {
		// Explicit, not implicit. `sc image ship` defaults to verifying against
		// ~/.ssh/known_hosts, which does not exist on a CI runner, so without this
		// flag every ship fails — and the fix an operator would reach for under time
		// pressure is to disable verification for good. Naming the insecurity in the
		// command, next to the warning the provisioning tier already logged, keeps
		// "this deploy trusts whoever answers" visible in the run log.
		args = append(args, "--insecure-skip-host-key-check")
	}
	return strings.Join([]string{
		"set -eu",
		// The SC image installs its own tooling outside the default PATH; same
		// prefix as pkg/clouds/pulumi/docker/security_helpers.go.
		`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`,
		"umask 077",
		"d=$(mktemp -d)",
		`trap 'rm -rf "$d"' EXIT INT TERM`,
		`cat > "$d/key"`,
		strings.Join(args, " "),
	}, "\n") + "\n"
}

// pullMode decides how hard `docker compose pull` tries before `up`.
type pullMode string

const (
	// pullRequired is registry delivery: every image must come from the registry, so
	// a pull failure is the deploy's failure and must be reported as one (a registry
	// credential that expired, a digest that was garbage-collected).
	pullRequired pullMode = "required"
	// pullBestEffort is ssh-tar delivery: the stack's own images were streamed onto
	// the machine and exist in no registry, so pulling them fails by design. Any
	// third-party image in the compose file (a mongo, a redis) still has to be
	// pulled, and that is what this mode is for.
	pullBestEffort pullMode = "best-effort"
)

// upSpec is everything upScript needs. Assembled inside a Pulumi apply, because the
// slot is only known once the state file on the machine has been read.
type upSpec struct {
	Stack, Env string
	// Slot is the slot being brought up; OtherSlot is the one it replaces. Under
	// strategy `recreate` they are still different names (OtherSlot is simply the
	// idle project), and Recreate below is what says the deploy replaced containers
	// in place rather than flipping between slots.
	Slot, OtherSlot string
	// Recreate is strategy `recreate`: the deploy brings the SAME project up again,
	// so there is no second copy anywhere and nothing to roll back to.
	Recreate bool

	SlotProject string
	// SlotContent is the rendered compose file, not its path. The path is derived
	// from Stack/Env/Slot, so the file written and the file `docker compose` is
	// pointed at cannot disagree.
	SlotContent string
	// SlotGates are the services to wait for before the deploy is called a success.
	// Every service of the project, not only the ones that declare a healthcheck: a
	// service without one still has to be running, and a crash-looping sidecar that
	// nobody waits for is a deploy that reports green and a stack that does not work.
	SlotGates []string

	// DataProject and DataContent are empty when the stack has no stateful service.
	DataProject string
	DataContent string
	DataGates   []string

	// StopOtherFirst is set when the two slots cannot coexist, which today means
	// exactly one thing: the ingress publishes a fixed host port and the old slot is
	// holding it. See the comment at its only writer in compose_stack.go.
	StopOtherFirst bool

	Pull pullMode
}

// upScript writes the compose files and brings the stack up, gating on health.
//
// The compose files are written here, inside the same script that runs `docker
// compose up`, rather than as their own remote.Command. That is not a shortcut: the
// file on disk is the description of what is running, and two commands can be
// applied one without the other (a write that succeeds and an up that fails leaves
// the machine describing containers it is not running). They are not secret — every
// value in them is either public or a reference to the env file — so there is no
// reason to pay the Stdin round trip that the env file needs.
func upScript(s upSpec) string {
	slotFile := vm.ComposeFilePath(s.Stack, s.Env, s.Slot)
	otherFile := vm.ComposeFilePath(s.Stack, s.Env, s.OtherSlot)
	dataFile := vm.DataComposeFilePath(s.Stack, s.Env)
	otherProject := vm.ProjectName(s.Stack, s.Env, s.OtherSlot)

	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())

	// Rollback is defined before anything can fail, and is the only place that knows
	// whether the previous slot was still up when this deploy started.
	b.WriteString("sc_rollback() {\n")
	fmt.Fprintf(b, "  echo %s >&2\n", shellQuote(fmt.Sprintf("deploy into slot %q of %s/%s failed", s.Slot, s.Stack, s.Env)))
	switch {
	case s.Recreate:
		fmt.Fprintf(b, "  echo %s >&2\n", shellQuote(
			"strategy is `recreate`, so the previous containers were replaced in place: "+
				"there is no second copy to fall back to and this stack-env is DOWN until a deploy succeeds",
		))
	case s.StopOtherFirst:
		fmt.Fprintf(b, "  sc_rm %s\n", shellQuote(s.SlotProject))
		fmt.Fprintf(b, "  if [ -f %s ]; then\n", shellQuote(otherFile))
		fmt.Fprintf(b, "    echo %s >&2\n", shellQuote(fmt.Sprintf("restoring the previous slot %q", s.OtherSlot)))
		fmt.Fprintf(b, "    { docker compose -p %s -f %s up -d || true; } >&2 2>&1\n",
			shellQuote(otherProject), shellQuote(otherFile))
		b.WriteString("  else\n")
		fmt.Fprintf(b, "    echo %s >&2\n", shellQuote(
			"there is no previous slot to restore: this stack-env is DOWN until a deploy succeeds",
		))
		b.WriteString("  fi\n")
	default:
		fmt.Fprintf(b, "  echo %s >&2\n", shellQuote(fmt.Sprintf(
			"slot %q is still serving traffic; the failed slot %q is left running for diagnosis "+
				"(`docker compose -p %s logs`)", s.OtherSlot, s.Slot, s.SlotProject,
		)))
	}
	b.WriteString("}\n\n")

	writeCompose := func(path, content string) {
		fmt.Fprintf(b, "printf %%s %s | base64 -d > %s\n", shellQuote(encodeFile(content)), shellQuote(path+".new"))
		fmt.Fprintf(b, "chmod 0640 %s\n", shellQuote(path+".new"))
		fmt.Fprintf(b, "mv -f %s %s\n", shellQuote(path+".new"), shellQuote(path))
	}

	// The stateful project first, and gated, because `depends_on` does not cross
	// compose projects: a backend in the slot that starts before its mongo gets a
	// connection refused, and whether it survives that is the application's business
	// rather than something SC should leave to chance.
	if s.DataContent != "" {
		writeCompose(dataFile, s.DataContent)
		b.WriteString(pullLine(s.Pull, s.DataProject, dataFile))
		fmt.Fprintf(b, "docker compose -p %s -f %s up -d --remove-orphans\n",
			shellQuote(s.DataProject), shellQuote(dataFile))
		for _, svc := range s.DataGates {
			fmt.Fprintf(b, "sc_wait %s %s %s || { sc_rollback; exit 1; }\n",
				shellQuote(s.DataProject), shellQuote(dataFile), shellQuote(svc))
		}
	}

	writeCompose(slotFile, s.SlotContent)
	b.WriteString(pullLine(s.Pull, s.SlotProject, slotFile))

	if s.StopOtherFirst {
		fmt.Fprintf(b, "echo %s >&2\n", shellQuote(fmt.Sprintf(
			"stopping slot %q before starting %q: the ingress publishes a fixed host port, "+
				"so the two slots cannot run at once (brief downtime; Caddy removes this in P5.3)",
			s.OtherSlot, s.Slot,
		)))
		fmt.Fprintf(b, "sc_rm %s\n", shellQuote(otherProject))
	}
	fmt.Fprintf(b, "docker compose -p %s -f %s up -d --remove-orphans || { sc_rollback; exit 1; }\n",
		shellQuote(s.SlotProject), shellQuote(slotFile))
	for _, svc := range s.SlotGates {
		fmt.Fprintf(b, "sc_wait %s %s %s || { sc_rollback; exit 1; }\n",
			shellQuote(s.SlotProject), shellQuote(slotFile), shellQuote(svc))
	}
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf("slot %q of %s/%s is up and healthy", s.Slot, s.Stack, s.Env)))
	return b.String()
}

func pullLine(mode pullMode, project, file string) string {
	switch mode {
	case pullRequired:
		return fmt.Sprintf("docker compose -p %s -f %s pull\n", shellQuote(project), shellQuote(file))
	case pullBestEffort:
		return fmt.Sprintf("{ docker compose -p %s -f %s pull --ignore-pull-failures || true; } >&2 2>&1\n",
			shellQuote(project), shellQuote(file))
	default:
		return ""
	}
}

// finalizeSpec is everything finalizeScript needs.
type finalizeSpec struct {
	Stack, Env string
	Slot       string
	// StopSlot is the slot to retire once the new one is recorded as active, or
	// empty when there is nothing to retire (strategy `recreate` replaced the
	// containers of this very project in place).
	StopSlot string
	Version  string
	// PruneRepos are the image repositories this stack owns, sorted. Only SC's own
	// repositories: pruning a third-party one would delete the mongo image another
	// stack on the machine is running.
	PruneRepos []string
	Keep       int
}

// finalizeScript records the new active slot, retires the old one, and prunes old
// images.
//
// Separate from upScript, and ordered after it, because every step here is one that
// must not happen if the health gate failed: writing the state file would point a
// rollback at a slot that never served traffic, and stopping the old slot would take
// down the copy that still works.
func finalizeScript(s finalizeSpec) string {
	statePath := vm.StatePath(s.Stack, s.Env)
	// Hand-built rather than json.Marshal'd so that the exact bytes readStateScript
	// greps for are visible next to the grep that reads them.
	state := fmt.Sprintf(`{"stack":%q,"env":%q,"activeSlot":%q,"version":%q}`, s.Stack, s.Env, s.Slot, s.Version)

	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	fmt.Fprintf(b, "t=%s\n", shellQuote(statePath+".new"))
	fmt.Fprintf(b, "printf %%s %s | base64 -d > \"$t\"\n", shellQuote(encodeFile(state)))
	b.WriteString("chmod 0640 \"$t\"\n")
	fmt.Fprintf(b, "mv -f \"$t\" %s\n", shellQuote(statePath))
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf("active slot of %s/%s is now %q at version %s",
		s.Stack, s.Env, s.Slot, s.Version)))

	if s.StopSlot != "" {
		fmt.Fprintf(b, "sc_rm %s\n", shellQuote(vm.ProjectName(s.Stack, s.Env, s.StopSlot)))
	}
	for _, repo := range s.PruneRepos {
		fmt.Fprintf(b, "sc_prune %s %d\n", shellQuote(repo), s.Keep+1)
	}
	return b.String()
}

// teardownScript removes everything a vm-compose deploy put on the machine except
// the data.
//
// It keeps the named volumes and the shared ingress network, deliberately. A
// `sc destroy` that deletes the database along with the containers is a footgun with
// no undo, and the volumes are external precisely so that no compose command can
// take them with it (compose-on-vm.md §6 rule 4). The env file does go, because it
// holds every secret of the stack and there is no reason to leave it on a machine
// that no longer runs it.
func teardownScript(stack, env string) string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	for _, slot := range vm.Slots {
		fmt.Fprintf(b, "sc_rm %s\n", shellQuote(vm.ProjectName(stack, env, slot)))
	}
	fmt.Fprintf(b, "sc_rm %s\n", shellQuote(vm.DataProjectName(stack, env)))
	fmt.Fprintf(b, "rm -rf %s\n", shellQuote(vm.StackDir(stack, env)))
	fmt.Fprintf(b, "rm -f %s\n", shellQuote(vm.StatePath(stack, env)))
	// The ingress route goes unconditionally, whether or not this deploy was the one
	// that wrote it. A site file left behind in conf.d outlives the containers it
	// points at, so the machine's Caddy keeps answering for that domain — with a 502
	// forever, and with a cert it keeps renewing. `rm -f` is a no-op for a stack that
	// was never served by Caddy, and the reload is skipped when there is no Caddy.
	fmt.Fprintf(b, "rm -f %s\n", shellQuote(vm.CaddySitePath(stack, env)))
	fmt.Fprintf(b, "if docker inspect %s >/dev/null 2>&1; then\n", shellQuote(vm.CaddyContainer))
	fmt.Fprintf(b, "  { docker exec %s caddy reload --config %s --adapter caddyfile || true; } >&2 2>&1\n",
		shellQuote(vm.CaddyContainer), shellQuote(vm.CaddyContainerConfigDir+"/Caddyfile"))
	b.WriteString("fi\n")
	fmt.Fprintf(b, "docker network rm %s >/dev/null 2>&1 || true\n",
		shellQuote(vm.InternalNetworkName(stack, env)))
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf(
		"removed the compose projects and configuration of %s/%s; named volumes and %s were kept",
		stack, env, vm.IngressNetwork,
	)))
	return b.String()
}

// routeSpec is everything routeScript needs. Assembled inside an apply, because the
// site file names the slot and the slot is read off the machine.
type routeSpec struct {
	Stack, Env string
	Slot       string
	Domain     string
	// Content is the rendered `conf.d/<stack>-<env>.caddy`, not its path: the path is
	// derived from Stack/Env, so the file written and the file that is validated
	// cannot disagree.
	Content string
}

// routeScript points the machine's Caddy at the slot this deploy just brought up.
//
// This IS the blue-green flip. Both slots are running at this point (the host port is
// not published when Caddy fronts the stack, so nothing forces them apart), the old
// one is still serving traffic, and a single `caddy reload` moves every new connection
// to the new slot without dropping an in-flight one. Nothing else in the deploy
// switches traffic (compose-on-vm.md §8.2, §9.1).
//
// The back-up-and-restore around it is not defensive padding. conf.d is shared by
// every stack on the machine and Caddy loads it as ONE config: a site file this stack
// renders badly does not break this stack, it makes the reload fail for everyone, and
// a validate-then-reload without a restore leaves the bad file on disk for the next
// unrelated deploy — or the next reboot — to load. So: write, validate the whole tree
// as Caddy will actually read it, and put the previous file back before failing.
func routeScript(s routeSpec) string {
	sitePath := vm.CaddySitePath(s.Stack, s.Env)
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	// Named up front so the restore in reloadLines' failure branch can use them.
	fmt.Fprintf(b, "site=%s\n", shellQuote(sitePath))
	fmt.Fprintf(b, "backup=%s\n", shellQuote(sitePath+".prev"))
	// A missing Caddy is reported here rather than let `docker exec` fail obscurely:
	// this script only runs when the parent stack said `ingress.enable: true`, so
	// Caddy not being there means the parent was never provisioned with it.
	fmt.Fprintf(b, "docker inspect %s >/dev/null 2>&1 || { echo %s >&2; exit 1; }\n",
		shellQuote(vm.CaddyContainer),
		shellQuote(fmt.Sprintf(
			"the machine has no %q container, so there is nothing to serve %q. Run `sc provision` for the parent "+
				"stack — the ingress belongs to the `%s` resource, not to this stack",
			vm.CaddyContainer, s.Domain, vm.ResourceTypeVm,
		)))

	fmt.Fprintf(b, "install -d -m 0750 %s\n", shellQuote(vm.CaddyConfDir()))
	// cp rather than mv: the previous route must stay live while the new one is being
	// validated, because a validate failure means traffic keeps going where it was.
	b.WriteString(`if [ -f "$site" ]; then cp -p "$site" "$backup"; else rm -f "$backup"; fi` + "\n")
	fmt.Fprintf(b, "printf %%s %s | base64 -d > \"$site.new\"\n", shellQuote(encodeFile(s.Content)))
	b.WriteString(`chmod 0640 "$site.new"` + "\n")
	b.WriteString(`mv -f "$site.new" "$site"` + "\n")

	// The restore, used by both failure branches below.
	restore := `if [ -f "$backup" ]; then mv -f "$backup" "$site"; else rm -f "$site"; fi` + "\n"

	// Validated inside the container, against the global Caddyfile, because that is
	// the only thing that reads conf.d the way Caddy will: validating the site file on
	// its own would reject every `import` of a snippet the global file defines.
	fmt.Fprintf(b, "if ! docker exec %s caddy validate --config %s --adapter caddyfile >&2 2>&1; then\n",
		shellQuote(vm.CaddyContainer), shellQuote(vm.CaddyContainerConfigDir+"/Caddyfile"))
	fmt.Fprintf(b, "  echo %s >&2\n", shellQuote(fmt.Sprintf(
		"the generated Caddy site for %s/%s is not a valid configuration; the previous route is left in place",
		s.Stack, s.Env,
	)))
	b.WriteString(indentLines(restore, "  "))
	b.WriteString("  exit 1\nfi\n")

	b.WriteString(reloadLines(restore))
	b.WriteString(`rm -f "$backup"` + "\n")
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf("%s now routes to slot %q of %s/%s",
		s.Domain, s.Slot, s.Stack, s.Env)))
	return b.String()
}

// shellHelpers are the three functions the scripts above share.
//
// sc_rm removes a compose project's containers by its docker label rather than by
// running `docker compose down -f <file>`. That is the difference between a teardown
// that works and one that works only when the file happens to be on disk — and the
// file for the slot being retired may predate a rename, a path change, or a machine
// restored from a snapshot. The label is set by compose itself and is the one fact
// that is always true of a running project.
func shellHelpers() string {
	return fmt.Sprintf(`sc_rm() {
  p=$1
  ids=$(docker ps -aq --filter "label=com.docker.compose.project=$p" 2>/dev/null || true)
  if [ -n "$ids" ]; then
    echo "removing containers of compose project $p" >&2
    { docker rm -f $ids || true; } >&2 2>&1
  fi
}

sc_wait() {
  p=$1; f=$2; s=$3; i=0; c=""; st=unknown; h=none
  while [ "$i" -lt %d ]; do
    c=$(docker compose -p "$p" -f "$f" ps -q "$s" 2>/dev/null | head -1)
    if [ -n "$c" ]; then
      st=$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null || echo unknown)
      h=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$c" 2>/dev/null || echo none)
      if [ "$st" = "running" ] && { [ "$h" = "healthy" ] || [ "$h" = "none" ]; }; then
        echo "ready: $p/$s ($st/$h)"
        return 0
      fi
    else
      st=missing; h=none
    fi
    i=$((i+1))
    sleep %d
  done
  echo "FAILED: $p/$s did not become ready within %ds (status=$st health=$h)" >&2
  if [ -n "$c" ]; then
    { echo "---- last 200 log lines of $p/$s ----"; docker logs --tail 200 "$c" || true; } >&2 2>&1
  fi
  return 1
}

sc_prune() {
  repo=$1; keep=$2
  docker images --filter "reference=$repo:*" --format '{{.CreatedAt}}|{{.ID}}' 2>/dev/null \
    | sort -r | tail -n +$((keep+1)) | cut -d'|' -f2 | sort -u | while read -r id; do
        docker rmi "$id" >/dev/null 2>&1 || true
      done
}

`, healthWaitAttempts, healthWaitInterval, healthWaitAttempts*healthWaitInterval)
}
