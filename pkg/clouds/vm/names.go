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
