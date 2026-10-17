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

	testVmResource   = "portal-vm"
	testComposeStack = "atrium"
	testHostKeyValue = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI-TEST"
	testEnvSecret    = "s3cr3t-mongo-uri"
	testRegistryPass = "s3cr3t-registry-password"
)

// composeMocks serves the parent stack's outputs and records what was created.
type composeMocks struct {
	mu      sync.Mutex
	created []pulumi.MockResourceArgs
	// outputs is the parent stack's export map. Overridden per test to model a parent
	// that was never provisioned, or one with no host key.
	outputs resource.PropertyMap
}

func newComposeMocks() *composeMocks {
	return &composeMocks{outputs: resource.PropertyMap{
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

func (f *composeFixture) withRegistryAuth() *composeFixture {
	f.in.DockerRegistryUsername = lo.ToPtr("ci")
	f.in.DockerRegistryPassword = lo.ToPtr(testRegistryPass)
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

// The URL and the slot are what the operator reads off the deploy, and in this slice
// the URL is a host port because Caddy has not landed yet.
func TestVmCompose_ExportsThePortAndStartsAtSlotA(t *testing.T) {
	RegisterTestingT(t)

	f := newComposeFixture()
	_, err := f.run()
	Expect(err).To(BeNil())

	Expect(f.url).To(Equal(fmt.Sprintf("http://%s:%d", testHostAddress, vm.IngressHostPort(testComposeStack, testEnv))))

	// read-state returns nothing under the mocks, which is the case a first deploy
	// hits for real: no state file on the machine yet, so slot a.
	Expect(f.slot).To(Equal(vm.SlotA))
}
