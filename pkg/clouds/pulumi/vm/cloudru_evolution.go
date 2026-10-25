// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi-tls/sdk/v5/go/tls"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// cloudruEvolutionDriver creates machines in Cloud.ru **Evolution**.
//
// Not the same cloud as cloudruDriver's Cloud.ru Advanced (ex-SberCloud), despite
// one brand and one console: different API, different credentials, different
// regions, and — the reason this file exists at all — no Terraform provider with
// open sources to bridge. Evolution's `evo-terraform` ships as a prebuilt binary, so
// the fork→tfgen→PluginDownloadURL recipe that produced pulumi-sbercloud does not
// apply, and `pulumi package add terraform-provider …` is not an option either: it
// resolves through registry.opentofu.org, which is geo-blocked from the market this
// driver exists to serve.
//
// So the machine is converged by `sc cloudru compute-vm apply|destroy` inside a
// pulumi-command local.Command, exactly as the Container App driver converges an app
// (pkg/clouds/pulumi/cloudru/container_app.go, and the package doc of
// pkg/cmd/cmd_cloudru for why a command and not in-process REST).
//
// Three things this cloud does NOT allow, all measured against the live API on
// 2026-10-24 rather than inferred, and each of which shapes the code below:
//
//   - **No user-data.** `POST /vms` answers `extra_forbidden` to `user_data`, so
//     there is no cloud-init: docker is installed afterwards over SSH
//     (dockerBootstrap), and readiness is the byo gate rather than a marker file.
//   - **No networking at create time.** `subnet_id`, `security_group_ids`,
//     `interfaces`, `networks`, `nics` and `floating_ip` are all refused; the machine
//     lands on the availability zone's default subnet and default security group. A
//     configured `network:` is therefore refused here rather than silently dropped.
//   - **No way to pre-seed the SSH host key**, which follows from the first: the key
//     would have to be written by user-data. The first connection is
//     trust-on-first-use, warned about loudly, and `hostKey:` closes it on the next
//     provision.
//
// Design: forge/docs/design/2026-10-13/sc-russia-market-readiness/ (the 2026-10-24
// amendment), roadmap item sc-cloudru-evolution-vm-driver.
type cloudruEvolutionDriver struct{}

func (cloudruEvolutionDriver) Name() string { return vm.DriverCloudruEvolution }

func (cloudruEvolutionDriver) Capabilities() vm.Capabilities {
	return vm.Drivers[vm.DriverCloudruEvolution].Capabilities
}

func (d cloudruEvolutionDriver) Hosts(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig, params pApi.ProvisionParams) ([]HostFacts, error) {
	name := input.Descriptor.Name
	hostName := fmt.Sprintf("%s-host-0", name)

	// Refused, not ignored, and refused HERE rather than in vm.Validate because the
	// reason is this one cloud's measured behaviour. `POST /vms` rejects every
	// spelling of a subnet or a security group, so a `network:` that looked accepted
	// would describe a placement that silently did not happen — and the operator
	// would be certain it had.
	if cfg.Network != nil {
		return nil, errors.Errorf(
			"`network` is not supported with `provider: %s`: a Cloud.ru Evolution VM create accepts no "+
				"`subnet_id`, `security_group_ids`, `interfaces`, `networks` or `nics` (measured 2026-10-24, the "+
				"API answers `extra_forbidden`), so the machine always lands on the availability zone's default "+
				"subnet and default security group. Remove `network`, or place the machine by hand in the console "+
				"and declare it with `provider: %s` and `hosts:`", vm.DriverCloudruEvolution, vm.DriverByo)
	}

	var account cloudru.EvolutionAccountConfig
	if err := api.ConvertAuth(cfg, &account); err != nil {
		return nil, errors.Wrapf(err,
			"failed to read Cloud.ru Evolution credentials of vm %q; `credentials:` must reference an `auth:` "+
				"entry of type %q", name, cloudru.AuthTypeCloudruEvolutionKey)
	}
	if err := account.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid Cloud.ru Evolution credentials of vm %q", name)
	}

	// The deploy key is a STACK RESOURCE, not an operator secret (vm-matrix.md §3.3,
	// D10): SC generates it, keeps the private half in the checkpoint marked secret,
	// publishes the public half into the VM's `public_key` metadata slot, and rotates
	// it by recreating this resource. An operator never sees it and never stores it.
	//
	// A resource rather than a value computed in Go because it has to survive to the
	// next deploy: a key minted per run would change the VM spec on every provision,
	// and the spec is what triggers the converge.
	deployKey, err := tls.NewPrivateKey(ctx, input.ToResName(name+"-deploy-key"), &tls.PrivateKeyArgs{
		Algorithm: sdk.String("ED25519"),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create deploy key for vm %q", name)
	}

	// Two documents from one spec, for the reason container_app.go states: the
	// request carries the key secret and goes to Stdin (secret, encrypted in the
	// checkpoint), while the trigger has to be something Pulumi may render in a diff
	// — so the trigger is a hash of the spec alone, and the spec holds no credential.
	buildSpec := func(publicKey string) (evolution.VmSpec, error) {
		return d.vmSpec(cfg, hostName, publicKey)
	}

	requestJSON := deployKey.PublicKeyOpenssh.ApplyT(func(publicKey string) (string, error) {
		spec, err := buildSpec(publicKey)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(evolution.Request{
			Credentials: evolution.Credentials{
				KeyID:     account.KeyID,
				Secret:    account.KeySecret,
				ProjectID: account.ProjectID,
			},
			Name:   spec.Name,
			VmSpec: &spec,
		})
		if err != nil {
			return "", errors.Wrapf(err, "failed to encode Cloud.ru Evolution vm request for %q", name)
		}
		return string(encoded), nil
	}).(sdk.StringOutput)

	specHash := deployKey.PublicKeyOpenssh.ApplyT(func(publicKey string) (string, error) {
		spec, err := buildSpec(publicKey)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash Cloud.ru Evolution vm spec for %q", name)
		}
		sum := sha256.Sum256(encoded)
		return hex.EncodeToString(sum[:]), nil
	}).(sdk.StringOutput)

	params.Log.Info(ctx.Context(), "configure Cloud.ru Evolution vm %q (host %q) in project %q...",
		name, hostName, account.ProjectID)

	applyScript := evolutionResultScript("compute-vm", "apply")
	destroyScript := evolutionScript("compute-vm", "destroy")

	vmCmd, err := local.NewCommand(ctx, input.ToResName(hostName+"-compute-vm"), &local.CommandArgs{
		Create: sdk.String(applyScript),
		// Update is the same script, not absent: without it pulumi-command turns any
		// input change into a REPLACEMENT, performed as delete-then-create — which on
		// this resource means destroying the machine, and with it the boot disk and
		// every stack deployed onto it, because one field of the spec moved. `apply` is
		// a find-or-create converge and refuses the changes it cannot make in place
		// (a resize) instead of taking that decision itself.
		Update: sdk.String(applyScript),
		Delete: sdk.StringPtr(destroyScript),
		// The spec hash, and deliberately NOT a per-run nonce (nonce.go): this command
		// CHANGES the cloud, and `apply` cannot tell before it runs whether there is
		// anything to change — it is cheap to re-run, but it is not free, and on a
		// deploy that changed nothing a nonce would re-converge the machine every time.
		// The script text is constant, so the hash is the only thing that can move;
		// everything that changes the resulting machine is inside the spec it covers.
		Triggers: sdk.Array{specHash},
		// Credentials and the spec on stdin, marked secret so the checkpoint holds it
		// encrypted; LoggingNone so neither the request nor the command's own output
		// reaches the Actions log; no AddPreviousOutputInEnv, because the previous
		// run's stdout is a JSON document this script has no use for.
		Stdin:                  sdk.ToSecret(requestJSON).(sdk.StringOutput).ToStringPtrOutput(),
		Logging:                local.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to converge Cloud.ru Evolution vm %q", name)
	}

	address := evolutionResultField(vmCmd.Stdout, func(r evolution.Result) string { return r.Address })

	host := HostFacts{
		Name:    hostName,
		Address: address,
		User:    cfg.EffectiveSshUser(),
		// sshd is on its default port and there is no reason for SC to move it: a
		// non-default port would have to be carried into the security-group rules, the
		// readiness gate and every stack deployed here.
		Port: vm.DefaultSshPort,
		// Marked secret at the single point where it enters Pulumi, as HostFacts
		// requires. tls.PrivateKey already marks its private outputs secret; ToSecret
		// is idempotent and keeps the guarantee local to this line.
		PrivateKey: sdk.ToSecret(deployKey.PrivateKeyOpenssh).(sdk.StringOutput),
		// Empty: an SC-generated key is never an encrypted PEM. Still wrapped and still
		// exported, because the consumer cannot tell which driver produced the host.
		PrivateKeyPassword: secretString(""),
		// Whatever the operator pinned, or nothing. Unlike every other cloud driver
		// this one cannot generate the machine's identity — see the type comment — so
		// HostKeyPinned is a property of the CONFIG here, and the loud warning for the
		// unpinned case is the one vm.go already emits for a byo host without a key.
		HostKey:       sdk.String(strings.TrimSpace(cfg.HostKey)).ToStringOutput(),
		HostKeyPinned: strings.TrimSpace(cfg.HostKey) != "",
		Role:          vm.RoleCompose,
		DependsOn:     []sdk.Resource{vmCmd},
	}

	// Docker, installed over SSH because this cloud has no user-data. In
	// HostFacts.DependsOn rather than left to the stack above it, so that the
	// readiness gate and the compose preflight (which asserts exactly what this
	// installs) cannot run first and fail on a machine that is merely young.
	bootstrap, err := d.dockerBootstrap(ctx, input, host, params)
	if err != nil {
		return nil, err
	}
	host.DependsOn = append(host.DependsOn, bootstrap)

	return []HostFacts{host}, nil
}

// vmSpec turns the cloud-independent VmConfig into the request `sc cloudru
// compute-vm apply` reads.
func (d cloudruEvolutionDriver) vmSpec(cfg *vm.VmConfig, hostName, deployPublicKey string) (evolution.VmSpec, error) {
	size := cfg.EffectiveSize()
	memGiB, err := gibibytes(size.Memory, "size.memory")
	if err != nil {
		return evolution.VmSpec{}, err
	}
	spec := evolution.VmSpec{
		Name:  hostName,
		Cores: size.Cores,
		// Resolved by cores+memory and NEVER by flavor name: the catalogue renames
		// sizes under us — the 2 vCPU / 4 GiB flavor was `low-2-4` on 2026-10-23 and
		// `lowcost10-2-4` a day later, with no `low-2-4` left in the catalogue. A name
		// compiled in here would have failed a deploy on Cloud.ru's schedule.
		MemoryGiB: memGiB,
		Image:     cfg.EffectiveImage(),
		SSHUser:   cfg.EffectiveSshUser(),
		PublicKey: strings.TrimSpace(deployPublicKey),
		// Always: vm.Validate refuses `publicIP: false` on every cloud driver, because
		// every delivery step is an outbound SSH connection from wherever `sc deploy`
		// runs and SC has no bastion.
		PublicIP:     true,
		IngressRules: evolutionIngressRules(cfg),
	}
	if len(cfg.Zones) > 0 && cfg.Zones[0] != "" {
		spec.Zone = cfg.Zones[0]
	}
	if size.Disk != "" {
		disk, err := gibibytes(size.Disk, "size.disk")
		if err != nil {
			return evolution.VmSpec{}, err
		}
		spec.DiskGiB = disk
	}
	// Left empty when unset, which lets the client pick the project's default disk
	// type by name — an SC-invented default here would be a guess about an account's
	// quota, and the type names are not the same across projects.
	spec.DiskType = size.DiskType
	return spec, nil
}

// evolutionIngressRules is the security-group holes SC needs, derived from the same
// port table the Advanced driver opens (ingressPorts) so the two cannot drift. They
// are applied to whichever group the machine actually joined — SC does not get to
// choose the group on this cloud.
func evolutionIngressRules(cfg *vm.VmConfig) []evolution.IngressRule {
	ports := ingressPorts(cfg)
	rules := make([]evolution.IngressRule, 0, len(ports))
	for _, label := range sortedKeys(ports) {
		p := ports[label]
		rules = append(rules, evolution.IngressRule{
			Protocol:    p.protocol,
			Port:        p.port,
			Description: fmt.Sprintf("simple-container %s", label),
		})
	}
	return rules
}

// dockerBootstrap installs Docker Engine and the compose v2 plugin on a machine that
// booted with neither.
//
// This is the whole cost of a cloud with no user-data, and it is placed in the
// driver rather than in the compose template on purpose: `docker` is a property of
// the MACHINE the resource promises (vm.go's doc comment — "a reachable,
// docker-capable machine"), the compose path only ASSERTS it
// (composePreflightScript), and a k3s host would be ready without it. Making the
// resource keep its own promise is what lets both templates stay cloud-independent.
func (cloudruEvolutionDriver) dockerBootstrap(ctx *sdk.Context, input api.ResourceInput, h HostFacts, params pApi.ProvisionParams) (*remote.Command, error) {
	script := dockerBootstrapScript(h.User)

	cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-docker"), &remote.CommandArgs{
		Connection: ConnectionArgs(h),
		Create:     sdk.String(script),
		Update:     sdk.String(script),
		// Guarded, not plain: the script's first act is to check on the machine
		// whether docker and the compose plugin are already there and the login user
		// can reach the daemon, and to exit without touching anything if so. That
		// satisfies all three preconditions in nonce.go — the no-op is observable
		// nowhere, costs one SSH round-trip, and this command carries no Delete (SC
		// does not uninstall docker from a machine it is destroying anyway).
		//
		// The nonce is what makes it repair drift: nothing in the config moves when a
		// machine is rebuilt underneath the stack, so without it the one deploy that
		// could reinstall docker is the one deploy that is skipped.
		Triggers: TriggersForGuardedChange(
			h.Address,
			sdk.String(h.User),
			sdk.String(script),
		),
		// The apt output is noisy and uninteresting, and every command in this package
		// shares one shape so that a new one inherits the safe default rather than
		// relying on whoever adds it to remember.
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn(h.DependsOn))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install docker on host %q of vm %q", h.Name, input.Descriptor.Name)
	}
	params.Log.Info(ctx.Context(),
		"vm %q installs docker on host %q over SSH: Cloud.ru Evolution accepts no user-data, so there is no "+
			"cloud-init to do it at first boot", input.Descriptor.Name, h.Name)
	return cmd, nil
}

// dockerBootstrapScript is the remote half.
//
// apt from download.docker.com, and that choice is measured rather than assumed:
// the smoke machine inside Cloud.ru Evolution reaches `download.docker.com`,
// `archive.ubuntu.com` and `security.ubuntu.com` (verified 2026-10-23, docker-ce
// 5:29.9.2-1~ubuntu.24.04~noble installed that way). Deliberately NOT the
// `get.docker.com` convenience script the Advanced driver's cloud-init uses: that
// one pipes a remote shell script into root over a link this market cannot always
// make, and it re-runs its whole repo setup every time. `registryMirrors` and
// `packageMirror` on the resource are for the accounts where even this does not
// hold; they are rendered into cloud-init today and are a follow-up on this path.
func dockerBootstrapScript(user string) string {
	// `DEBIAN_FRONTEND` and `-o DPkg::Lock::Timeout` both matter on a machine this
	// young: unattended-upgrades holds the dpkg lock for the first minutes after
	// boot, and apt without a lock timeout fails instantly rather than waiting.
	//
	// The lock timeout is NOT enough on its own, which is the live-caught half
	// (2026-10-24, the first VM this driver created end to end). `DPkg::Lock::Timeout`
	// covers the dpkg locks; `apt-get update` also takes `/var/lib/apt/lists/lock`,
	// and THAT one it tries once and abandons:
	//
	//	E: Could not get lock /var/lib/apt/lists/lock. It is held by process 911 (apt-get)
	//
	// Process 911 was the image's own first-boot apt run — which is to say the
	// collision is not an edge case but the normal state of a machine we connect to
	// the second sshd answers. So every apt invocation here retries. `cloud-init
	// status --wait` would be the tidier gate and is not available on this image;
	// polling `fuser` needs psmisc, which is a package install, which needs apt. A
	// retry loop needs nothing that is not already there.
	return strings.Join([]string{
		`set -eu`,
		`export DEBIAN_FRONTEND=noninteractive`,
		`APT="sudo -E apt-get -y -o DPkg::Lock::Timeout=300"`,
		// 40 × 15s = 10 minutes, generously past the ~2 minutes an Ubuntu cloud image's
		// first-boot apt actually takes here. A failure after that is a real one and must
		// surface as a failure: a bootstrap that gave up quietly would hand the compose
		// step a machine with no docker on it.
		`apt_retry() {`,
		`  n=0`,
		`  until $APT "$@"; do`,
		`    n=$((n+1))`,
		`    if [ "$n" -ge 40 ]; then`,
		`      echo "apt is still locked after 10 minutes, giving up on: $*" >&2`,
		`      exit 1`,
		`    fi`,
		`    echo "apt is locked (the image's own first-boot run?), retry $n/40: $*" >&2`,
		`    sleep 15`,
		`  done`,
		`}`,
		// The guard. Checked as the LOGIN user, not as root, because "docker is
		// installed" is not the property the compose path needs — it needs this user
		// to be able to talk to the daemon, which is group membership, and that is the
		// half that silently goes missing when a machine is rebuilt.
		`if command -v docker >/dev/null 2>&1 && docker version >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then`,
		`  echo docker-already-installed`,
		`  exit 0`,
		`fi`,
		`apt_retry update`,
		`apt_retry install ca-certificates curl`,
		`sudo install -m 0755 -d /etc/apt/keyrings`,
		// -fsSL and an explicit output path rather than a pipe into gpg: the key is
		// ASCII-armoured and apt reads it as-is from a .asc, so there is nothing to
		// dearmor and one less process whose failure a pipeline would swallow.
		`sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc`,
		`sudo chmod a+r /etc/apt/keyrings/docker.asc`,
		// VERSION_CODENAME from the machine itself, so the same script serves 22.04 and
		// 24.04 — the image alias is the operator's choice and this must not second-
		// guess it.
		`. /etc/os-release`,
		`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] ` +
			`https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" | ` +
			`sudo tee /etc/apt/sources.list.d/docker.list >/dev/null`,
		`apt_retry update`,
		`apt_retry install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin`,
		`sudo systemctl enable --now docker`,
		// The group, and then a verification AS ROOT. The login shell of THIS session
		// predates the usermod, so `docker version` as the user would fail here even
		// though it will succeed on the next connection — which is every command that
		// follows, since pulumi-command dials afresh for each one.
		fmt.Sprintf(`sudo usermod -aG docker %s`, user),
		fail(`sudo docker version >/dev/null 2>&1`, "docker was installed but the daemon is not answering"),
		fail(`sudo docker compose version >/dev/null 2>&1`, "docker was installed without the compose v2 plugin"),
		`echo docker-installed`,
	}, "\n") + "\n"
}

// evolutionScript renders the shell that runs one `sc cloudru …` subcommand.
//
// Nothing is interpolated: the whole request — credentials, the spec — arrives on
// the command's Stdin, because an argument is readable in `ps` by every other
// process on the runner and an environment variable by every other step of the job.
// The PATH prefix is the one the SC runner image needs for a bare `sc` to resolve.
//
// A second copy of pkg/clouds/pulumi/cloudru/scripts.go's scScript rather than an
// import, for the reason secretString is duplicated three lines below its twin:
// importing pkg/clouds/pulumi/cloudru from here would make the vm package depend on
// a template package that already depends on the docker one, for three lines.
func evolutionScript(args ...string) string {
	return strings.Join([]string{
		"set -eu",
		`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`,
		"sc cloudru " + strings.Join(args, " "),
	}, "\n") + "\n"
}

// evolutionResultScript is evolutionScript for the commands whose stdout this
// program parses, with one addition: the SHELL checks that a result document came
// back, and fails the command if it did not.
//
// The check is here, and not only in parseEvolutionResult, because of how an `sc`
// older than the deploy fails. It does not error — `sc cloudru compute-vm` with an
// unknown subcommand prints the command group's help and exits 0 — so pulumi-command
// records a successful create whose stdout is that help text. From then on the stack
// is wedged in a way that is very hard to read: the program parses the STORED stdout,
// so the next `pulumi preview` fails before it plans anything, which means the stack
// can be neither re-deployed nor destroyed without editing its checkpoint by hand.
// Failing the create instead leaves nothing in state. Live-caught 2026-10-24, on a
// runner whose `sc` predated this subcommand by one build.
//
// The captured output is printed back verbatim so pulumi-command still sees exactly
// what the command wrote; nothing is lost by buffering it, because progress goes to
// stderr and stdout is contractually the single JSON line (cmd_cloudru/cloudru.go).
// Matched loosely (an object mentioning `name`) rather than parsed: the shell's job is
// only to tell a result document from a help screen.
func evolutionResultScript(args ...string) string {
	cmd := "sc cloudru " + strings.Join(args, " ")
	return strings.Join([]string{
		"set -eu",
		`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`,
		"out=$(" + cmd + ")",
		`printf '%s\n' "$out"`,
		`printf '%s\n' "$out" | grep -q '^[[:space:]]*{.*"name"' || {`,
		`  echo '` + cmd + ` printed no result document: this sc is older than the deploy' >&2`,
		"  exit 1",
		"}",
	}, "\n") + "\n"
}

// evolutionResultField reads one field out of the JSON document `sc cloudru
// compute-vm apply` printed, so the stack can use facts only the API knew — the
// floating IP above all, which is what every later SSH connection dials.
//
// Scanned from the END for a parseable object rather than decoded whole: stdout is
// contractually one JSON document, but SC's own logger writes to the same stream and
// an operator debugging a deploy by hand will have the progress lines in front of
// them.
func evolutionResultField(stdout sdk.StringOutput, pick func(evolution.Result) string) sdk.StringOutput {
	return stdout.ApplyT(func(out string) (string, error) {
		// Empty during a preview: the command has not run, and an error here would
		// fail the plan rather than the deploy.
		if strings.TrimSpace(out) == "" {
			return "", nil
		}
		res, err := parseEvolutionResult(out)
		if err != nil {
			return "", err
		}
		return pick(*res), nil
	}).(sdk.StringOutput)
}

func parseEvolutionResult(stdout string) (*evolution.Result, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var res evolution.Result
		if err := json.Unmarshal([]byte(line), &res); err == nil && res.Name != "" {
			return &res, nil
		}
	}
	// Does not echo stdout: LoggingNone keeps the command's output out of the log on
	// purpose, and repeating it in an error would undo that.
	return nil, errors.Errorf("could not find a result document in the output of `sc cloudru compute-vm apply` " +
		"(expected one JSON object on its own line); the `sc` on this runner may be older than this deploy")
}
