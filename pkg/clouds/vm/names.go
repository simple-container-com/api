// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"hash/fnv"
	"regexp"

	"github.com/pkg/errors"
)

// IngressNetwork is the one docker network shared by every stack on the machine.
// Only ingress containers join it, so that Caddy can reach them by name without
// two stacks seeing each other's `mongo` (compose-on-vm.md §6 rule 1, §11 rule 2).
//
// It is created out of band (`docker network create sc-ingress || true` on every
// deploy) and referenced as `external: true`, because it outlives any one compose
// project and must survive a reboot and a `docker compose down` of either slot.
const IngressNetwork = "sc-ingress"

// The ingress (Caddy) names. One Caddy per machine, in its own compose project,
// owned by the `vm` RESOURCE rather than by any stack — several stacks share one
// port 443, so the thing that holds it cannot belong to one of them
// (compose-on-vm.md §3, D15).
const (
	// CaddyProject is the compose project. It carries no stack-env prefix, which is
	// the one documented exception to the rule in NamePrefix: there is exactly one
	// of it per machine and its name is what an operator types.
	CaddyProject = "sc-caddy"
	// CaddyContainer is also `container_name`, because the blue-green flip reloads
	// Caddy with `docker exec sc-caddy …` and a generated container name would make
	// that command depend on the project's numbering.
	CaddyContainer = "sc-caddy"
	// CaddyDataVolume holds the ACME account key and every issued certificate.
	//
	// A named volume, NOT a path inside the config tree, and this is the single most
	// consequential choice in P5.3: Let's Encrypt allows 50 certificates per domain
	// per week, so an ingress that loses /data on every recreate burns through the
	// quota and then serves a site with no certificate at all. The config tree is
	// mounted read-only, which is also incompatible with Caddy writing into it —
	// compose-on-vm.md §3.1 draws `data/` under /etc/sc/caddy, and that part of the
	// drawing is wrong (§8.3 point 3 is the rule).
	CaddyDataVolume = "sc-caddy-data"
	// CaddyConfigVolume is Caddy's own config/autosave directory.
	CaddyConfigVolume = "sc-caddy-config"
	// CaddyContainerConfigDir is where the config tree is mounted INSIDE the
	// container. The paths written into the generated Caddyfile are container paths,
	// so they must not be derived from CaddyDir.
	CaddyContainerConfigDir = "/etc/caddy"
)

// CaddyDir is the ingress configuration tree on the machine, mounted read-only at
// CaddyContainerConfigDir.
func CaddyDir() string {
	return ConfigDir + "/caddy"
}

// CaddyfilePath is the generated global Caddyfile: the ACME configuration, the shared
// snippet library, and an `import` of everything in conf.d.
func CaddyfilePath() string {
	return CaddyDir() + "/Caddyfile"
}

// CaddyConfDir holds one file per stack-env. A deploy writes its own file and reloads;
// it never edits a file another stack owns, which is what lets two stacks deploy
// concurrently without a lock (compose-on-vm.md §8.2).
func CaddyConfDir() string {
	return CaddyDir() + "/conf.d"
}

// CaddyPagesDir holds the error pages the `handle_server_error` snippet serves. Same
// files as the Kubernetes edge — they come from the same embedded asset tree.
func CaddyPagesDir() string {
	return CaddyDir() + "/pages"
}

// CaddyComposePath is the ingress's own compose file.
func CaddyComposePath() string {
	return CaddyDir() + "/docker-compose.yaml"
}

// CaddyEnvPath is the ingress env file (0600): the ACME External Account Binding
// credential, when there is one. It exists even when empty, because compose fails a
// project whose `env_file` is missing.
func CaddyEnvPath() string {
	return CaddyDir() + "/env"
}

// CaddySitePath is one stack-env's file in conf.d.
func CaddySitePath(stack, env string) string {
	return fmt.Sprintf("%s/%s.caddy", CaddyConfDir(), NamePrefix(stack, env))
}

// CaddyDefaultSitePath is the catch-all, written once when the ingress is created.
//
// It answers an unknown Host with a 503 — the same decision, and for the same reason,
// as the Kubernetes edge's default entry (kubernetes/caddy.go): without it `import
// conf.d/*.caddy` on a machine with no stacks yet matches nothing, and Caddy refuses a
// config with no site at all. The `00-` prefix only keeps it first in the glob for a
// human reading the directory; Caddy does not care about order here.
func CaddyDefaultSitePath() string {
	return CaddyConfDir() + "/00-default.caddy"
}

// The ingress env file's variable names. They are referenced from the generated
// Caddyfile as `{$...}` placeholders so that the credential lives only in a 0600 file
// delivered over stdin, and not in a config file an operator may paste into a ticket.
const (
	EnvAcmeEabKeyID  = "SC_ACME_EAB_KEY_ID"
	EnvAcmeEabMacKey = "SC_ACME_EAB_MAC_KEY"
)

// The two blue-green slots. A stack-env occupies one of them at a time; a deploy
// brings up the other and flips Caddy to it (compose-on-vm.md §9.1).
const (
	SlotA = "a"
	SlotB = "b"
)

// Slots is every slot a stack-env can occupy, in flip order.
var Slots = []string{SlotA, SlotB}

// OtherSlot returns the slot a deploy should bring up given the active one. An
// unknown or empty active slot yields SlotA, so a machine with no recorded state
// deploys into A.
func OtherSlot(active string) string {
	if active == SlotA {
		return SlotB
	}
	return SlotA
}

// NamePrefix is the `<stack>-<env>-` prefix that goes on EVERY name SC puts on the
// machine: compose project, external volume, ingress alias, the file in Caddy's
// conf.d, the image directory. compose-on-vm.md §11 rule 3 — one exception breaks a
// neighbouring stack, and it diagnoses as mysticism rather than as a name clash.
func NamePrefix(stack, env string) string {
	return fmt.Sprintf("%s-%s", stack, env)
}

// ProjectName is the compose project of one slot. Compose isolates by project name,
// so two slots are two projects, two sets of containers — and one shared internal
// network and one shared set of data volumes.
func ProjectName(stack, env, slot string) string {
	return fmt.Sprintf("%s-%s", NamePrefix(stack, env), slot)
}

// DataProjectName is the compose project holding the services that must NOT be
// duplicated by blue-green: anything with a writable named volume. Two Mongos on one
// volume corrupt it, so a stateful service lives outside the slots and the flip never
// touches it (compose-on-vm.md §9.1).
func DataProjectName(stack, env string) string {
	return NamePrefix(stack, env) + "-data"
}

// InternalNetworkName is the private network of one stack-env — shared by both slots
// AND by the data project, which is why it is external rather than a compose
// `default`.
//
// Deviation from compose-on-vm.md §6, worth stating: the example renders a per-slot
// `default` network named `<stack>-<env>-<slot>-internal`. That cannot work together
// with §9.1's rule that stateful services live in their own project — a compose
// default network is scoped to its project, so the backend in slot B would not resolve
// `mongo` in the data project. One external network per stack-env satisfies both
// rules; the isolation that matters (§6 rule 1, §11 rule 1) is between stack-envs, and
// that is preserved.
func InternalNetworkName(stack, env string) string {
	return NamePrefix(stack, env) + "-internal"
}

// ExternalVolumeName is the real docker volume behind a named volume written in the
// customer's compose file. It is created out of band and declared `external: true`, so
// that cleaning up the previous slot (`docker compose -p <prefix>-a down -v`) cannot
// delete the data (compose-on-vm.md §6 rule 4).
func ExternalVolumeName(stack, env, volume string) string {
	return fmt.Sprintf("%s-%s", NamePrefix(stack, env), volume)
}

// SlotAlias is the network alias of one service in one slot. Caddy proxies to it on
// the ingress network, and it is also added on the internal network so that a sidecar
// can address one specific slot.
//
// It carries the slot, which is what makes the flip a one-line change in Caddy's
// config, and the stack-env prefix, without which two stacks that both call their
// service `backend` would collide on the shared ingress network.
//
// Services keep their bare compose name as well (compose adds it as an alias on every
// network), which is what lets `MONGO_URI=mongodb://mongo:27017` keep working
// unchanged. During the overlap window of a blue-green deploy the bare name resolves to
// either slot's container; that ambiguity is inherent to running two slots at once.
func SlotAlias(stack, env, service, slot string) string {
	return fmt.Sprintf("%s-%s-%s", NamePrefix(stack, env), service, slot)
}

// StackDir is where rendered configuration for one stack-env lives.
func StackDir(stack, env string) string {
	return fmt.Sprintf("%s/%s/%s", ConfigDir, stack, env)
}

// ComposeFilePath is the generated compose file of one slot.
//
// One file per slot, not the single `docker-compose.yaml` of compose-on-vm.md §3.1: a
// blue-green deploy has both slots up at once (§9.1 steps 1–2), and a single path means
// rendering the new slot overwrites the description of the slot still serving traffic —
// which is exactly the file an operator reaches for when the new slot fails its health
// gate.
func ComposeFilePath(stack, env, slot string) string {
	return fmt.Sprintf("%s/docker-compose-%s.yaml", StackDir(stack, env), slot)
}

// DataComposeFilePath is the generated compose file of the stateful project.
func DataComposeFilePath(stack, env string) string {
	return fmt.Sprintf("%s/docker-compose-data.yaml", StackDir(stack, env))
}

// EnvFilePath is the `env_file` every service of the stack-env reads. Mode 0600,
// owned by the ssh user; values never appear in the generated compose file
// (compose-on-vm.md §7.1).
func EnvFilePath(stack, env string) string {
	return StackDir(stack, env) + "/env"
}

// TextVolumeDir holds the files rendered from `textVolumes`.
func TextVolumeDir(stack, env string) string {
	return StackDir(stack, env) + "/volumes"
}

// TextVolumePath is one rendered `textVolumes` entry on the machine.
func TextVolumePath(stack, env, name string) string {
	return fmt.Sprintf("%s/%s", TextVolumeDir(stack, env), name)
}

// ImagesDir holds the shipped image archives of one stack-env, kept for rollback
// (`keepImages`, compose-on-vm.md §9.3).
func ImagesDir(stack, env string) string {
	return fmt.Sprintf("%s/images/%s/%s", StateDir, stack, env)
}

// StatePath records which slot is active and which version it runs.
func StatePath(stack, env string) string {
	return fmt.Sprintf("%s/state/%s.json", StateDir, NamePrefix(stack, env))
}

// The host-port window SC publishes a stack's ingress on when Caddy is not fronting it
// — either because Caddy is not there yet (P5.2) or because the operator set
// `ingress.enable: false` (compose-on-vm.md §8.4).
const (
	HostPortRangeStart = 20000
	HostPortRangeEnd   = 30000
)

// IngressHostPort derives a stable host port for a stack-env.
//
// Stable matters more than pretty: the port goes into a firewall rule, a bookmark and
// somebody's smoke script, so it must not move when an unrelated stack is added to the
// machine. Hashing the name gives that without SC having to keep a port registry; the
// cost is that two stack-envs on one machine can collide, which the deploy detects when
// docker refuses to bind and reports with both names.
func IngressHostPort(stack, env string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(NamePrefix(stack, env)))
	return HostPortRangeStart + int(h.Sum32()%uint32(HostPortRangeEnd-HostPortRangeStart))
}

// composeProjectName is docker compose's own constraint on a project name. Compose
// normalises a name it dislikes (lowercasing, dropping characters), which would silently
// hand two different stack-envs the same project — so SC refuses instead.
var composeProjectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidateNames refuses a stack or environment name that cannot be part of a docker
// compose project name. It fails at render time, with the offending name, rather than as
// a confusing collision between two stacks on the machine.
func ValidateNames(stack, env string) error {
	for name, value := range map[string]string{"stack": stack, "environment": env} {
		if value == "" {
			return errors.Errorf("%s name is empty", name)
		}
		if !composeProjectName.MatchString(value) {
			return errors.Errorf(
				"%s name %q cannot be used on a VM: a docker compose project name must match %s "+
					"(lower case, starting with a letter or digit)", name, value, composeProjectName.String(),
			)
		}
	}
	return nil
}
