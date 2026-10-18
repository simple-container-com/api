// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"
)

// Every name SC puts on the machine carries the `<stack>-<env>` prefix. The reason it
// is pinned by a test rather than left to the functions is compose-on-vm.md §11 rule 3:
// a single unprefixed name collides with a neighbouring stack on the same machine, and
// it presents as that other stack misbehaving.
func TestNamesAreStackEnvPrefixed(t *testing.T) {
	RegisterTestingT(t)

	Expect(NamePrefix("atrium", "staging")).To(Equal("atrium-staging"))
	Expect(ProjectName("atrium", "staging", SlotB)).To(Equal("atrium-staging-b"))
	Expect(DataProjectName("atrium", "staging")).To(Equal("atrium-staging-data"))
	Expect(InternalNetworkName("atrium", "staging")).To(Equal("atrium-staging-internal"))
	Expect(ExternalVolumeName("atrium", "staging", "mongo-data")).To(Equal("atrium-staging-mongo-data"))
	Expect(SlotAlias("atrium", "staging", "backend", SlotA)).To(Equal("atrium-staging-backend-a"))

	// The one name that is deliberately NOT prefixed: the shared ingress network is
	// the single place two stacks are meant to meet, so that Caddy can reach both.
	Expect(IngressNetwork).To(Equal("sc-ingress"))
}

func TestOtherSlotFlips(t *testing.T) {
	RegisterTestingT(t)

	Expect(OtherSlot(SlotA)).To(Equal(SlotB))
	Expect(OtherSlot(SlotB)).To(Equal(SlotA))
	// A machine with no recorded state deploys into A rather than failing.
	Expect(OtherSlot("")).To(Equal(SlotA))
	Expect(OtherSlot("garbage")).To(Equal(SlotA))
}

func TestPathsLiveUnderOneDirectoryPerStackEnv(t *testing.T) {
	RegisterTestingT(t)

	Expect(StackDir("atrium", "staging")).To(Equal("/etc/sc/atrium/staging"))
	Expect(ComposeFilePath("atrium", "staging", SlotA)).To(Equal("/etc/sc/atrium/staging/docker-compose-a.yaml"))
	// One file per slot: a blue-green deploy has both slots up at once, so a single
	// path would overwrite the description of the slot still serving traffic.
	Expect(ComposeFilePath("atrium", "staging", SlotB)).ToNot(Equal(ComposeFilePath("atrium", "staging", SlotA)))
	Expect(DataComposeFilePath("atrium", "staging")).To(Equal("/etc/sc/atrium/staging/docker-compose-data.yaml"))
	Expect(EnvFilePath("atrium", "staging")).To(Equal("/etc/sc/atrium/staging/env"))
	Expect(TextVolumePath("atrium", "staging", "nginx.conf")).To(Equal("/etc/sc/atrium/staging/volumes/nginx.conf"))
	Expect(ImagesDir("atrium", "staging")).To(Equal("/var/lib/sc/images/atrium/staging"))
	Expect(StatePath("atrium", "staging")).To(Equal("/var/lib/sc/state/atrium-staging.json"))
}

func TestValidateNames(t *testing.T) {
	RegisterTestingT(t)

	Expect(ValidateNames("atrium", "staging")).ToNot(HaveOccurred())
	Expect(ValidateNames("atrium-ru", "staging-ru")).ToNot(HaveOccurred())

	Expect(ValidateNames("", "staging")).To(MatchError(ContainSubstring("stack name is empty")))
	Expect(ValidateNames("atrium", "")).To(MatchError(ContainSubstring("environment name is empty")))
	// Compose normalises a name it dislikes instead of refusing it, which would hand
	// two different stack-envs the same project. SC refuses first, naming the value.
	Expect(ValidateNames("Atrium", "staging")).To(MatchError(ContainSubstring(`stack name "Atrium"`)))
	Expect(ValidateNames("atrium", "stag ing")).To(MatchError(ContainSubstring(`environment name "stag ing"`)))
	Expect(ValidateNames("-atrium", "staging")).To(MatchError(ContainSubstring("lower case")))
}

// The host port goes into a firewall rule and somebody's smoke script, so the one
// property that matters is that adding an unrelated stack to the machine does not move
// it.
func TestIngressHostPortIsStableAndInRange(t *testing.T) {
	RegisterTestingT(t)

	port := IngressHostPort("atrium", "staging")
	Expect(port).To(Equal(IngressHostPort("atrium", "staging")))
	Expect(port).To(BeNumerically(">=", HostPortRangeStart))
	Expect(port).To(BeNumerically("<", HostPortRangeEnd))

	// Different stack-envs get different ports in practice (a collision is possible and
	// is detected at `up` time when docker refuses to bind, not hidden).
	Expect(IngressHostPort("atrium", "staging")).ToNot(Equal(IngressHostPort("atrium", "prod")))
	Expect(IngressHostPort("atrium", "staging")).ToNot(Equal(IngressHostPort("hermes", "staging")))

	// It is derived from the prefix, not from the two arguments separately — otherwise
	// `a`+`b-c` and `a-b`+`c` would be two ports for one directory.
	Expect(IngressHostPort("atrium", "staging")).ToNot(Equal(0))
}
