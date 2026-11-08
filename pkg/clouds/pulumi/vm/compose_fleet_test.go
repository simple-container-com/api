// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// A vm resource can hold more than one machine, and a vm-compose stack is deployed onto
// every one of them. What makes that safe is not the loop — it is four properties of
// what reaches Pulumi, none of which a passing deploy would show:
//
//   - host 0's resource names are byte-identical to a single-host deploy's, so growing
//     a fleet APPENDS machines instead of replacing the one already serving
//   - the machines are deployed one at a time, so a release that fails its health gate
//     stops with the rest of the fleet still on the version that worked
//   - an image is built once for the fleet and shipped once per machine
//   - every machine is actually a different machine (a loop that read index 0 N times
//     would deploy the same host three times and look entirely green doing it)

// fleetHostAddress is machine i's address in the fixture's parent stack. Distinct per
// host so that a deploy which read the wrong index cannot pass.
func fleetHostAddress(i int) string {
	return fmt.Sprintf("10.20.30.%d", 40+i)
}

// withHosts grows the parent stack's vm resource to n machines.
//
// Host 0 is left exactly as newComposeMocks wrote it, which is the point: the
// single-host fixture and the fleet fixture agree about machine 0 down to the address,
// so a name that differs between the two runs is a real difference and not an artefact
// of the fixture.
func (f *composeFixture) withHosts(n int) *composeFixture {
	f.mocks.outputs[resource.PropertyKey(ToHostCountExport(testVmResource))] = resource.NewNumberProperty(float64(n))
	for i := 1; i < n; i++ {
		f.mocks.outputs[resource.PropertyKey(ToHostAddressExport(testVmResource, i))] = resource.NewStringProperty(fleetHostAddress(i))
		f.mocks.outputs[resource.PropertyKey(ToHostUserExport(testVmResource, i))] = resource.NewStringProperty(vm.DefaultSshUser)
		f.mocks.outputs[resource.PropertyKey(ToHostPortExport(testVmResource, i))] = resource.NewNumberProperty(22)
		f.mocks.outputs[resource.PropertyKey(ToHostPrivateKeyExport(testVmResource, i))] = resource.MakeSecret(resource.NewStringProperty(testPrivateKey))
		f.mocks.outputs[resource.PropertyKey(ToHostPrivateKeyPasswordExport(testVmResource, i))] = resource.MakeSecret(resource.NewStringProperty(""))
		f.mocks.outputs[resource.PropertyKey(ToHostKeyExport(testVmResource, i))] = resource.NewStringProperty(testHostKeyValue)
	}
	return f
}

// resourceNames returns the logical names of every recorded resource of one type, with
// ToResName's environment suffix removed. Unlike commands()/localCommands() it keeps
// the whole name rather than the step, because what these tests assert about IS the
// name.
func (m *composeMocks) resourceNames(token string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, a := range m.created {
		if a.TypeToken != token {
			continue
		}
		out = append(out, strings.TrimSuffix(a.Name, "--"+testEnv))
	}
	return out
}

// connectionHost is the machine a remote command was pointed at.
func (m *composeMocks) connectionHost(step string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken != tokenRemoteCommand || !strings.Contains(a.Name, "-vmc-"+step+"--") {
			continue
		}
		// The whole `connection` arrives secret-wrapped — it carries the deploy key,
		// and Pulumi marks the containing object and not only the leaf — so it has to
		// be peeled at both levels. A helper that only looked at the object would read
		// every connection as absent and make these assertions vacuous.
		conn, _ := unwrapSecret(a.Inputs["connection"])
		if !conn.IsObject() {
			return ""
		}
		host, _ := unwrapSecret(conn.ObjectValue()["host"])
		if !host.IsString() {
			return ""
		}
		return host.StringValue()
	}
	return ""
}

// The naming contract, stated as code: host 0 is un-suffixed so that an existing
// single-host stack sees no rename, and every later host carries its index. A rename
// here is not cosmetic — a renamed remote.Command is a NEW resource whose Create runs
// and whose predecessor's Delete tears down the containers it just started.
func TestHostResSuffix_HostZeroIsUnsuffixed(t *testing.T) {
	RegisterTestingT(t)

	Expect(hostResSuffix(0)).To(BeEmpty())
	Expect(hostResSuffix(1)).To(Equal("-1"))
	Expect(hostResSuffix(2)).To(Equal("-2"))
	Expect(hostResSuffix(11)).To(Equal("-11"))
}

func TestVmCompose_FleetDeploysOntoEveryHostAndLeavesHostZerosNamesAlone(t *testing.T) {
	RegisterTestingT(t)

	single := newComposeFixture()
	_, err := single.run()
	Expect(err).ToNot(HaveOccurred())
	singleSteps := lo.Keys(single.mocks.commands())
	Expect(singleSteps).ToNot(BeEmpty())

	fleet := newComposeFixture().withHosts(3)
	out, err := fleet.run()
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Hosts).To(HaveLen(3))

	steps := fleet.mocks.commands()
	// Every step the single-host deploy ran is present under the identical name. This
	// is the assertion that protects a running stack from being torn down by the
	// upgrade that adds its second machine.
	for _, step := range singleSteps {
		Expect(steps).To(HaveKey(step), "host 0's %q was renamed by growing the fleet", step)
	}
	// And the other two machines added their own, suffixed.
	for _, step := range []string{"preflight-1", "prepare-1", "up-1", "finalize-1", "up-2", "finalize-2"} {
		Expect(steps).To(HaveKey(step))
	}
	Expect(steps).ToNot(HaveKey("up-3"))

	// Three machines and not the same machine three times.
	Expect(fleet.mocks.connectionHost("up")).To(Equal(testHostAddress))
	Expect(fleet.mocks.connectionHost("up-1")).To(Equal(fleetHostAddress(1)))
	Expect(fleet.mocks.connectionHost("up-2")).To(Equal(fleetHostAddress(2)))

	// Same thing read off the returned output, which is what a caller sees: Host/Slot/
	// URL stay host 0's, and Hosts carries the fleet.
	Expect(out.Host.Name).To(Equal(testVmResource + "-host-0"))
	Expect(lo.Map(out.Hosts, func(h VmComposeHostOutput, _ int) string { return h.Host.Name })).To(Equal(
		[]string{testVmResource + "-host-0", testVmResource + "-host-1", testVmResource + "-host-2"},
	))
	for _, h := range out.Hosts {
		Expect(h.Host.Role).To(Equal(vm.RoleCompose))
	}
}

// The roll. A fleet exists so that a bad release does not take every replica with it,
// and Pulumi would happily deploy all three machines at once — the dependency edge is
// the only thing that makes the deploy stop at the machine that failed.
//
// The edge is on the previous host's `finalize` rather than its `up`: between those two
// a machine is serving a slot its state file does not yet name, and moving on from
// there would leave the fleet in that state if the next machine failed.
func TestVmCompose_FleetRollsOneMachineAtATime(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withHosts(3)
	_, err := f.run()
	Expect(err).ToNot(HaveOccurred())

	// Host 1's first step — `preflight`, the head of each machine's chain — waits for
	// host 0 to have finished, and host 2's for host 1.
	Expect(f.mocks.dependsOn("preflight-1")).To(ContainElement(
		ContainSubstring("-vmc-finalize--" + testEnv),
	))
	Expect(f.mocks.dependsOn("preflight-2")).To(ContainElement(
		ContainSubstring("-vmc-finalize-1--" + testEnv),
	))
	// And host 0 waits for nothing: it is the start of the roll, not the end of one.
	Expect(f.mocks.dependsOn("preflight")).ToNot(ContainElement(ContainSubstring("-vmc-finalize")))
}

// An image is a property of the code and a ship is a property of the machine, so a
// fleet shares the first and not the second. Registering the build per host would be N
// identical docker builds under N resource names — Pulumi would reject the duplicates,
// and if it did not, the deploy would pay for the same build three times.
func TestVmCompose_FleetBuildsOnceAndShipsToEveryMachine(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withSshTarBuild().withHosts(3)
	_, err := f.run()
	Expect(err).ToNot(HaveOccurred())

	Expect(f.mocks.resourceNames(tokenDockerImage)).To(HaveLen(1),
		"one build for the fleet, not one per machine")

	ships := f.mocks.resourceNames(tokenLocalCommand)
	Expect(ships).To(ContainElement(testComposeStack + "-app-ship"))
	Expect(ships).To(ContainElement(testComposeStack + "-app-ship-1"))
	Expect(ships).To(ContainElement(testComposeStack + "-app-ship-2"))

	// Each machine's containers wait for the image to have reached THAT machine. An
	// `up` ordered behind another host's ship would start containers against an image
	// the local docker does not have.
	Expect(f.mocks.dependsOn("up-2")).To(ContainElement(ContainSubstring("-app-ship-2--" + testEnv)))
}
