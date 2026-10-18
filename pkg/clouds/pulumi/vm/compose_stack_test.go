// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// These tests assert what reaches Pulumi, which is a different thing from what the
// scripts say: the three pulumi-command semantics in compose-on-vm.md §0.7 are
// properties of the resource inputs, not of the shell. Each of the three has already
// cost the fleet a deploy somewhere, and each is invisible in a passing deploy:
//
//   - a command with no Triggers silently no-ops on the next deploy and reports green
//   - a command with no Update is REPLACED on any input change, delete before create
//   - a secret in the command string is stored in the checkpoint in the clear
const (
	tokenStackReference = "pulumi:pulumi:StackReference"
	tokenLocalCommand   = "command:local:Command"
	tokenDockerImage    = "docker:index/image:Image"

	testVmResource   = "portal-vm"
	testComposeStack = "atrium"
	testHostKeyValue = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI-TEST"
	testEnvSecret    = "s3cr3t-mongo-uri"
	testRegistryPass = "s3cr3t-registry-password"
	testDomain       = "atrium.example.ru"
)

// composeMocks serves the parent stack's outputs and records what was created.
type composeMocks struct {
	mu      sync.Mutex
	created []pulumi.MockResourceArgs
	// outputs is the parent stack's export map. Overridden per test to model a parent
	// that was never provisioned, or one with no host key.
	outputs resource.PropertyMap
	// repoDigest is what a built docker.Image reports back. Under SkipPush that is
	// the LOCAL image id, which is the only output of a build that moves when the
	// build's result does — the image name does not. Varied by a test to model the
	// same stack rebuilt after a code change.
	repoDigest string
}

const testRepoDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func newComposeMocks() *composeMocks {
	return &composeMocks{repoDigest: testRepoDigest, outputs: resource.PropertyMap{
		resource.PropertyKey(ToHostCountExport(testVmResource)):      resource.NewNumberProperty(1),
		resource.PropertyKey(ToHostAddressExport(testVmResource, 0)): resource.NewStringProperty(testHostAddress),
		resource.PropertyKey(ToHostUserExport(testVmResource, 0)):    resource.NewStringProperty(vm.DefaultSshUser),
		// A number, not an int — this is the shape a stack reference hands back after
		// a round trip through the checkpoint's JSON, and the reason hostFromParent
		// reads it as `any` and coerces. GetValueFromStack[int] would panic here.
		resource.PropertyKey(ToHostPortExport(testVmResource, 0)): resource.NewNumberProperty(22),
		resource.PropertyKey(ToHostPrivateKeyExport(testVmResource, 0)): resource.MakeSecret(
			resource.NewStringProperty(testPrivateKey)),
		resource.PropertyKey(ToHostPrivateKeyPasswordExport(testVmResource, 0)): resource.MakeSecret(
			resource.NewStringProperty("")),
		resource.PropertyKey(ToHostKeyExport(testVmResource, 0)):     resource.NewStringProperty(testHostKeyValue),
		resource.PropertyKey(ToIngressEnabledExport(testVmResource)): resource.NewBoolProperty(false),
	}}
}

func (m *composeMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, args)
	if args.TypeToken == tokenStackReference {
		return args.Name + "-id", resource.PropertyMap{
			"outputs": resource.NewObjectProperty(m.outputs),
		}, nil
	}
	if args.TypeToken == tokenDockerImage {
		// repoDigest is an OUTPUT of the build, so it has to be added here: echoing
		// the inputs back (what every other resource gets) would leave it empty, and
		// a test that asserts on triggers would then be asserting about "" on both
		// sides of the comparison it is making.
		state := args.Inputs.Copy()
		state["repoDigest"] = resource.NewStringProperty(m.repoDigest)
		return args.Name + "-id", state, nil
	}
	return args.Name + "-id", args.Inputs, nil
}

func (m *composeMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// commands returns the recorded remote commands keyed by the suffix of their logical
// name, so a test can name the step it is asserting about.
func (m *composeMocks) commands() map[string]resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]resource.PropertyMap{}
	for _, a := range m.created {
		if a.TypeToken != tokenRemoteCommand {
			continue
		}
		// `<stack>-vmc-<step>--<env>`: ToResName appends the environment, so both
		// ends come off to leave the step.
		name := a.Name
		if i := strings.Index(name, "-vmc-"); i >= 0 {
			name = name[i+len("-vmc-"):]
		}
		name = strings.TrimSuffix(name, "--"+testEnv)
		out[name] = a.Inputs
	}
	return out
}

// localCommands returns the recorded LOCAL commands — the image ships — keyed by the
// same suffix convention. A separate map from commands() because the ship runs on the
// runner, not on the machine, and an assertion that means to be about one of those two
// places must not silently be about the other.
func (m *composeMocks) localCommands() map[string]resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]resource.PropertyMap{}
	for _, a := range m.created {
		if a.TypeToken != tokenLocalCommand {
			continue
		}
		name := strings.TrimSuffix(a.Name, "--"+testEnv)
		if i := strings.LastIndex(name, "-"); i >= 0 {
			name = name[i+1:]
		}
		out[name] = a.Inputs
	}
	return out
}

// dependsOn returns the URNs one step was registered with. Asserting ordering off the
// recorded sequence would be asserting about goroutine scheduling — independent
// resources register in whatever order their inputs resolve. The dependency edge is
// the thing the deploy actually honours.
func (m *composeMocks) dependsOn(step string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken != tokenRemoteCommand || a.RegisterRPC == nil {
			continue
		}
		if strings.Contains(a.Name, "-vmc-"+step) {
			return a.RegisterRPC.GetDependencies()
		}
	}
	return nil
}

func composeInput() *vm.VmComposeInput {
	port := 8080
	// The converters set IngressContainer from the compose label (or, for
	// single-image, from the only service there is); this fixture builds the input
	// directly, so it has to be set here too — leaving it nil is the
	// "nothing is reachable from outside the machine" case, which publishes no
	// host port and would make the URL assertions vacuous.
	app := k8s.CloudRunContainer{
		Name:     "app",
		Image:    api.ContainerImage{Name: "ghcr.io/simple-container-com/atrium:1"},
		MainPort: &port,
	}
	return &vm.VmComposeInput{
		ComposeTemplateConfig: vm.ComposeTemplateConfig{
			VmResource: testVmResource,
			// registry delivery, so the test exercises the login step without needing
			// a real build context: the container below is a third-party image, which
			// buildImages passes through without creating a docker.Image.
			//
			// The URL with no credentials is a public registry, which is a supported
			// configuration — withRegistryAuth() below adds the private case.
			ImageDelivery:       vm.ImageDeliveryRegistry,
			RegistryCredentials: docker.RegistryCredentials{DockerRegistryURL: lo.ToPtr("registry.example.ru")},
		},
		Deployment: k8s.DeploymentConfig{
			StackConfig: &api.StackConfigCompose{
				Env:     map[string]string{"LOG_LEVEL": "info"},
				Secrets: map[string]string{"MONGO_URI": testEnvSecret},
			},
			Containers:       []k8s.CloudRunContainer{app},
			IngressContainer: &app,
		},
	}
}

// withSshTarBuild models the DEFAULT delivery mode, which the registry fixture above
// does not reach: a service SC builds itself and streams to the machine over SSH.
// Giving the container a build context is what sends buildImages down that branch —
// a container with only an `image:` is a third-party image and is never built.
func (f *composeFixture) withSshTarBuild() *composeFixture {
	f.in.ImageDelivery = vm.ImageDeliverySshTar
	f.in.RegistryCredentials = docker.RegistryCredentials{}
	built := api.ContainerImage{Dockerfile: "Dockerfile", Context: "."}
	f.in.Deployment.Containers[0].Image = built
	f.in.Deployment.IngressContainer.Image = built
	return f
}

func (f *composeFixture) withRegistryAuth() *composeFixture {
	f.in.DockerRegistryUsername = lo.ToPtr("ci")
	f.in.DockerRegistryPassword = lo.ToPtr(testRegistryPass)
	return f
}

// withIngress models the machine an operator provisioned with `ingress.enable: true`
// and a stack that declares a domain. Both halves are required — the ingress is a
// property of the vm RESOURCE (read back off the parent stack) and the domain a
// property of this stack — and getting one without the other is a supported state
// that produces a warning, not a route.
func (f *composeFixture) withIngress() *composeFixture {
	f.mocks.outputs[resource.PropertyKey(ToIngressEnabledExport(testVmResource))] = resource.NewBoolProperty(true)
	f.in.Deployment.StackConfig.Domain = testDomain
	return f
}

type composeFixture struct {
	in    *vm.VmComposeInput
	mocks *composeMocks
	// Resolved inside the Pulumi run, because an Output can only be awaited while
	// the context that produced it is alive.
	url  string
	slot string
}

func newComposeFixture() *composeFixture {
	return &composeFixture{in: composeInput(), mocks: newComposeMocks()}
}

func (f *composeFixture) run() (*VmComposeOutput, error) {
	var out *VmComposeOutput
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params := pApi.ProvisionParams{
			Log:            logger.New(),
			ComputeContext: pApi.NewComputeContextCollector(context.Background(), logger.New(), testComposeStack, testEnv),
			ParentStack:    &pApi.ParentInfo{StackName: "infra", FullReference: "organization/forge/infra--" + testEnv},
			BaseEnvVariables: map[string]string{
				"SIMPLE_CONTAINER_HELPERS_IMAGE": "ignored",
			},
		}
		input := api.ResourceInput{
			Descriptor: &api.ResourceDescriptor{
				Type:   vm.TemplateTypeVmCompose,
				Name:   testComposeStack,
				Config: api.Config{Config: f.in},
			},
			StackParams: &api.StackParams{
				StackName:   testComposeStack,
				Environment: testEnv,
				Version:     "2026.10.16-abc1234",
			},
		}
		res, err := VmComposeStack(ctx, api.Stack{Name: testComposeStack}, input, params)
		if err != nil {
			return err
		}
		var ok bool
		out, ok = res.Ref.(*VmComposeOutput)
		if !ok {
			return errors.Errorf("VmComposeStack().Ref is %T, not a *VmComposeOutput", res.Ref)
		}
		url, err := internals.UnsafeAwaitOutput(ctx.Context(), out.URL)
		if err != nil {
			return err
		}
		f.url, _ = url.Value.(string)
		slot, err := internals.UnsafeAwaitOutput(ctx.Context(), out.Slot)
		if err != nil {
			return err
		}
		f.slot, _ = slot.Value.(string)
		return nil
	}, pulumi.WithMocks("project", testComposeStack, f.mocks))
	return out, err
}

// THE assertion of this file. A remote.Command without Triggers does not re-run when
// its script changes identity but not text (a new image tag inside an apply, a new
// slot), and pulumi-command reports the unchanged resource as a successful deploy.
func TestVmCompose_EveryCommandHasTriggersAndAnExplicitUpdate(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withRegistryAuth()
	_, err := f.run()
	Expect(err).To(BeNil())

	cmds := f.mocks.commands()
	Expect(cmds).To(HaveKey("prepare"))
	Expect(cmds).To(HaveKey("write-env"))
	Expect(cmds).To(HaveKey("registry-login"))
	Expect(cmds).To(HaveKey("read-state"))
	Expect(cmds).To(HaveKey("up"))
	Expect(cmds).To(HaveKey("finalize"))
	Expect(cmds).To(HaveKey("teardown"))

	for name, inputs := range cmds {
		if name == "teardown" {
			// Deliberately the exception: a teardown whose Create does nothing must
			// never be replaced, because pulumi-command performs a replacement as
			// create-then-delete and the delete would tear down the stack that was
			// just brought up.
			Expect(inputs["triggers"].V).To(BeNil(), "teardown must not be replaceable")
			Expect(inputs["delete"].StringValue()).NotTo(BeEmpty())
			continue
		}
		triggers, _ := unwrapSecret(inputs["triggers"])
		Expect(triggers.ArrayValue()).NotTo(BeEmpty(), "command %q has no triggers and will silently no-op", name)
		update, _ := unwrapSecret(inputs["update"])
		create, _ := unwrapSecret(inputs["create"])
		Expect(update.StringValue()).NotTo(BeEmpty(), "command %q has no update and will be replaced via delete+create", name)
		Expect(update.StringValue()).To(Equal(create.StringValue()), "command %q diverges between create and update", name)
	}
}

// Every secret this template handles — the env file, the registry password, the
// deploy key — must reach the machine on stdin and must not be anywhere in the
// command string, the triggers, or the environment.
func TestVmCompose_NoSecretIsEverInACommandString(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withRegistryAuth()
	_, err := f.run()
	Expect(err).To(BeNil())

	for name, inputs := range f.mocks.commands() {
		for _, field := range []resource.PropertyKey{"create", "update", "delete", "triggers", "environment"} {
			pv, ok := inputs[field]
			if !ok {
				continue
			}
			plain, _ := unwrapSecret(pv)
			rendered := plain.String()
			for _, secret := range []string{testEnvSecret, testRegistryPass, "TEST-KEY-MATERIAL"} {
				Expect(rendered).NotTo(ContainSubstring(secret),
					"command %q leaks a secret in %q, where Pulumi stores it unencrypted and prints it in a diff",
					name, field)
			}
		}
	}

	cmds := f.mocks.commands()
	envStdin, secret := unwrapSecret(cmds["write-env"]["stdin"])
	Expect(secret).To(BeTrue(), "the env file's content must be marked secret")
	Expect(envStdin.StringValue()).To(ContainSubstring("MONGO_URI=" + testEnvSecret))
	Expect(envStdin.StringValue()).To(ContainSubstring("LOG_LEVEL=info"))
	// Resolved from BaseEnvVariables and from SC itself, not from the client.yaml.
	Expect(envStdin.StringValue()).To(ContainSubstring(api.ComputeEnv.StackName + "=" + testComposeStack))

	loginStdin, secret := unwrapSecret(cmds["registry-login"]["stdin"])
	Expect(secret).To(BeTrue())
	Expect(loginStdin.StringValue()).To(Equal(testRegistryPass))

	// Logging is disabled on exactly the commands that carry a secret on stdin: a
	// `docker login` that fails prints the password-stdin value in some versions, and
	// an env-file write has nothing worth logging.
	for _, name := range []string{"write-env", "registry-login"} {
		logging, _ := unwrapSecret(cmds[name]["logging"])
		Expect(logging.StringValue()).To(Equal("none"), "command %q must not log", name)
	}
}

// The env file is secret, so it cannot be a trigger — but it still has to cause a
// re-run when it changes, or a rotated secret never reaches the machine.
func TestVmCompose_EnvFileTriggersOnItsHashNotItsContent(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	_, err := f.run()
	Expect(err).To(BeNil())
	before, _ := unwrapSecret(f.mocks.commands()["write-env"]["triggers"])

	g := newComposeFixture()
	g.in.Deployment.StackConfig.Secrets["MONGO_URI"] = "a-different-secret"
	_, err = g.run()
	Expect(err).To(BeNil())
	after, _ := unwrapSecret(g.mocks.commands()["write-env"]["triggers"])

	Expect(after.String()).NotTo(Equal(before.String()), "a rotated secret must re-run the env write")
	Expect(after.String()).NotTo(ContainSubstring("a-different-secret"))
}

// A parent stack that was never provisioned, or a `vmResource` that does not name one
// of its resources, reads back as a successful fetch of nothing: GetValueFromStack
// wraps a nil error and returns the zero value. Without an explicit check the deploy
// would proceed to SSH into the empty string.
func TestVmCompose_MissingVmResourceFailsWithAUsefulMessage(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	f.mocks.outputs = resource.PropertyMap{}

	_, err := f.run()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("has no `type: vm` resource named"))
	Expect(err.Error()).To(ContainSubstring("vmResource"))
	Expect(f.mocks.commands()).To(BeEmpty(), "nothing may be created against a machine that was not found")
}

func TestVmCompose_PinnedHostKeyReachesTheConnection(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	_, err := f.run()
	Expect(err).To(BeNil())

	conn, _ := unwrapSecret(f.mocks.commands()["prepare"]["connection"])
	hostKey, _ := unwrapSecret(conn.ObjectValue()["hostKey"])
	Expect(hostKey.StringValue()).To(Equal(testHostKeyValue))
}

// The URL and the slot are what the operator reads off the deploy. The fixture's
// parent has no ingress, which is the `ingress.enable: false` escape hatch: the stack
// publishes a host port and is served over plain HTTP.
func TestVmCompose_ExportsThePortAndStartsAtSlotA(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.url).To(Equal(fmt.Sprintf("http://%s:%d", testHostAddress, vm.IngressHostPort(testComposeStack, testEnv))))

	// read-state returns nothing under the mocks, which is the case a first deploy
	// hits for real: no state file on the machine yet, so slot a.
	Expect(f.slot).To(Equal(vm.SlotA))

	// No route is written when the machine has no ingress — there would be no Caddy to
	// reload, and the command would fail every deploy.
	Expect(f.mocks.commands()).ToNot(HaveKey("route"))
}

// With Caddy in front, the stack stops publishing a host port altogether: the proxy
// reaches it over the shared docker network, and NOT publishing is what makes the flip
// possible — two slots cannot bind the same host port, so `stopOtherFirst` only relaxes
// to blue-green when nothing is published.
func TestVmCompose_WithIngressPublishesNoHostPortAndServesHttps(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.url).To(Equal("https://" + testDomain))

	cmds := f.mocks.commands()
	Expect(cmds).To(HaveKey("route"))

	// The compose project the machine brings up must not contain a published port. The
	// rendered file travels inside the `up` script, so this reads it back out of there
	// rather than re-rendering it.
	up, _ := unwrapSecret(cmds["up"]["create"])
	blobs := decodedBlobs(up.StringValue())
	// Guard: without this the loop below passes on an empty slice, which is exactly
	// what a renamed delivery mechanism would produce.
	Expect(blobs).To(ContainElement(ContainSubstring("services:")))
	hostPortMapping := fmt.Sprintf("%d:", vm.IngressHostPort(testComposeStack, testEnv))
	for _, blob := range blobs {
		Expect(blob).ToNot(ContainSubstring(hostPortMapping),
			"a published host port makes the second slot unschedulable and breaks the flip")
	}
}

// The ordering is the whole no-502 property, and it is only expressible as a
// dependency: the route must be written after BOTH slots are up (earlier and it points
// at a container that may never start) and before finalize (later and the old slot is
// already gone, which is the 502 this slice exists to avoid).
func TestVmCompose_RouteIsSequencedBetweenUpAndFinalize(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.mocks.dependsOn("route")).To(ContainElement(ContainSubstring("-vmc-up")))
	Expect(f.mocks.dependsOn("finalize")).To(ContainElement(ContainSubstring("-vmc-route")))
	// And finalize no longer waits on `up` directly — if it did, Pulumi would be free
	// to retire the old slot concurrently with the route being written.
	Expect(f.mocks.dependsOn("finalize")).ToNot(ContainElement(ContainSubstring("-vmc-up")))
}

// The route command carries the slot, so it must re-run on every deploy that flips —
// and it must have an Update, because a replacement would delete the route it just
// wrote. Checked separately from the blanket assertion at the top of this file, which
// runs without an ingress and so never sees this command.
func TestVmCompose_RouteHasTriggersAndAnUpdate(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	_, err := f.run()
	Expect(err).To(BeNil())

	route := f.mocks.commands()["route"]
	create, _ := unwrapSecret(route["create"])
	update, _ := unwrapSecret(route["update"])
	triggers, _ := unwrapSecret(route["triggers"])

	Expect(update.StringValue()).To(Equal(create.StringValue()))
	Expect(triggers.ArrayValue()).ToNot(BeEmpty())
	// The slot is in the script, so the content IS the trigger: a flip from a to b
	// changes it and the route is rewritten.
	Expect(create.StringValue()).To(ContainSubstring(vm.CaddySitePath(testComposeStack, testEnv)))
	// No Delete: the route is removed exactly once, by the teardown command, so a
	// replacement of this command cannot delete a route moments after writing it.
	Expect(route["delete"].V).To(BeNil())
}

// A domain declared against a machine with no ingress is the most likely misconfig in
// this slice, and the one that would otherwise present as "my domain doesn't work"
// with nothing in the deploy output. It is a warning, not an error: the stack still
// deploys and is still reachable on its host port.
func TestVmCompose_DomainWithoutIngressStillPublishesTheHostPort(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	f.in.Deployment.StackConfig.Domain = testDomain
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.url).To(Equal(fmt.Sprintf("http://%s:%d", testHostAddress, vm.IngressHostPort(testComposeStack, testEnv))))
	Expect(f.mocks.commands()).ToNot(HaveKey("route"))
}

// The mirror case: an ingress is there but the stack declares no domain. There is no
// aggregator site on a VM to graft a `prefix` onto, so there is nothing for Caddy to
// serve this stack as — it falls back to the host port.
func TestVmCompose_IngressWithoutADomainFallsBackToTheHostPort(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	f.in.Deployment.StackConfig.Domain = ""
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.url).To(Equal(fmt.Sprintf("http://%s:%d", testHostAddress, vm.IngressHostPort(testComposeStack, testEnv))))
	Expect(f.mocks.commands()).ToNot(HaveKey("route"))
}

// Teardown removes the route whether or not this stack was ever served by Caddy: the
// conf.d file outlives the containers it points at, and a stale one answers for the
// domain with a 502 forever while renewing a certificate for it.
func TestVmCompose_TeardownRemovesTheRoute(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	_, err := f.run()
	Expect(err).To(BeNil())

	del, _ := unwrapSecret(f.mocks.commands()["teardown"]["delete"])
	Expect(del.StringValue()).To(ContainSubstring(vm.CaddySitePath(testComposeStack, testEnv)))
}

// A trigger set that is non-empty is not the same thing as a trigger set that
// DISCRIMINATES, and the test above only asserts the former. This one asserts the
// latter for the default delivery mode: rebuild the same stack, at the same version,
// after a code change, and every command whose job depends on the image content must
// see a different trigger set than it did before.
//
// Live-caught 2026-10-18. The ssh-tar ship was triggered by the local tag, the deploy
// version, the host address and the image NAME — four strings that are identical on
// every deploy of a given stack, the version included, because `sc deploy` with no
// `--version` uses the constant `latest`. So a redeploy after an edit built the new
// image on the runner, shipped nothing, restarted nothing, and reported
// `1 updated, 10 unchanged` with the machine still serving the previous build.
func TestVmCompose_ShipAndUpTriggerOnImageContentNotItsTag(t *testing.T) {
	RegisterTestingT(t)

	before := newComposeFixture().withSshTarBuild().withIngress()
	_, err := before.run()
	Expect(err).To(BeNil())

	after := newComposeFixture().withSshTarBuild().withIngress()
	// The ONLY difference: the same Dockerfile produced a different image. Same
	// stack, same version, same host, same env — so the local tag, which is what the
	// compose file references, is byte for byte what it was.
	after.mocks.repoDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	_, err = after.run()
	Expect(err).To(BeNil())

	ship := before.mocks.localCommands()["ship"]
	Expect(ship).ToNot(BeNil(), "the ssh-tar fixture did not produce a ship command")
	shipAfter := after.mocks.localCommands()["ship"]

	// First establish that the tag really is the same, or the assertions below would
	// pass for the uninteresting reason.
	Expect(ship["create"].String()).To(Equal(shipAfter["create"].String()),
		"the ship SCRIPT is expected to be identical — it addresses the image by tag; "+
			"if this fails the test no longer proves what it claims")

	for _, c := range []struct {
		name   string
		inputs [2]resource.PropertyMap
	}{
		{"ship", [2]resource.PropertyMap{ship, shipAfter}},
		{"up", [2]resource.PropertyMap{before.mocks.commands()["up"], after.mocks.commands()["up"]}},
		{"finalize", [2]resource.PropertyMap{before.mocks.commands()["finalize"], after.mocks.commands()["finalize"]}},
	} {
		a, _ := unwrapSecret(c.inputs[0]["triggers"])
		b, _ := unwrapSecret(c.inputs[1]["triggers"])
		Expect(a.String()).ToNot(Equal(b.String()),
			"command %q has the same triggers before and after a rebuild, so it will not re-run "+
				"and the deploy will report success while the machine serves the old image", c.name)
	}
}

// The state read is the one command that must run on every deploy, and the only one.
//
// Live-caught 2026-10-18: triggered by the host address and the script hash — both
// invariant — it ran once and every later deploy took the active slot out of the
// checkpoint instead of off the machine. The first deploy had cached `""` (nothing
// deployed yet), so OtherSlot kept returning SlotA and `strategy: blue-green` would
// have recreated the serving slot in place rather than flipping to the other one.
func TestVmCompose_OnlyTheStateReadRunsOnEveryDeploy(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withIngress()
	_, err := f.run()
	Expect(err).To(BeNil())

	cmds := f.mocks.commands()
	triggers, _ := unwrapSecret(cmds["read-state"]["triggers"])
	Expect(triggers.String()).To(ContainSubstring(runNonce),
		"read-state is not triggered per run, so it will answer later deploys from the checkpoint")

	for name, inputs := range cmds {
		if name == "read-state" {
			continue
		}
		other, _ := unwrapSecret(inputs["triggers"])
		// A nonce on a command that CHANGES the machine would make every deploy a
		// full redeploy of everything — including, on a stack with a published host
		// port, a stop-start of the serving container for a deploy that changed
		// nothing.
		Expect(other.String()).ToNot(ContainSubstring(runNonce),
			"command %q is triggered per run; only the state read may be", name)
	}
}

// The ship is a LOCAL command, so it is invisible to the secret sweep above, which
// walks remote commands only — and it is the command that carries the deploy key.
func TestVmCompose_TheShipKeepsTheDeployKeyOffTheCommandLine(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture().withSshTarBuild()
	_, err := f.run()
	Expect(err).To(BeNil())

	ship := f.mocks.localCommands()["ship"]
	Expect(ship).ToNot(BeNil())
	for _, field := range []resource.PropertyKey{"create", "update", "triggers", "environment"} {
		plain, _ := unwrapSecret(ship[field])
		Expect(plain.String()).ToNot(ContainSubstring("TEST-KEY-MATERIAL"),
			"the ship leaks the deploy key in %q, where Pulumi stores it in the clear", field)
	}
	stdin, secret := unwrapSecret(ship["stdin"])
	Expect(secret).To(BeTrue(), "the deploy key must reach the ship marked secret")
	Expect(stdin.StringValue()).To(Equal(testPrivateKey))
	logging, _ := unwrapSecret(ship["logging"])
	Expect(logging.StringValue()).To(Equal("none"),
		"the ship streams a customer image over a connection authenticated by the key on stdin")
}
