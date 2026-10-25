// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	tokenCommandProvider = "pulumi:providers:command"
	tokenRemoteCommand   = "command:remote:Command"

	testHostAddress = "203.0.113.10"
	testPrivateKey  = "-----BEGIN OPENSSH PRIVATE KEY-----\nTEST-KEY-MATERIAL\n-----END OPENSSH PRIVATE KEY-----"
)

// vmMocks records what the provisioner actually asked Pulumi to create, so the
// assertions land on the inputs that reach each resource rather than on the helpers
// that compute them.
type vmMocks struct {
	mu      sync.Mutex
	created []pulumi.MockResourceArgs
	// extraOutputs supplements a mocked resource's state, by type token, with
	// properties a real provider COMPUTES rather than echoes — an address, a
	// generated key. Without them the output never resolves and anything that reads
	// it (an ApplyT rendering cloud-init, say) silently never runs.
	extraOutputs map[string]resource.PropertyMap
	// callResults answers a datasource invoke, by token. The default of echoing the
	// arguments back is right for a resource and useless for a datasource, whose
	// whole purpose is to return something the caller did not send.
	callResults map[string]resource.PropertyMap
	// invoked records the arguments each datasource was asked with — for a driver
	// that resolves an image and a flavour by FILTERING, the filter is the behaviour
	// worth asserting.
	invoked map[string]resource.PropertyMap
}

func newVmMocks() *vmMocks { return &vmMocks{} }

func (m *vmMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, args)

	state := args.Inputs
	if extra, ok := m.extraOutputs[args.TypeToken]; ok {
		state = make(resource.PropertyMap, len(args.Inputs)+len(extra))
		for k, v := range args.Inputs {
			state[k] = v
		}
		for k, v := range extra {
			state[k] = v
		}
	}
	return args.Name + "-id", state, nil
}

func (m *vmMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	m.mu.Lock()
	if m.invoked == nil {
		m.invoked = map[string]resource.PropertyMap{}
	}
	m.invoked[args.Token] = args.Args
	result, mocked := m.callResults[args.Token]
	m.mu.Unlock()

	if mocked {
		return result, nil
	}
	return args.Args, nil
}

func (m *vmMocks) callArgsOf(token string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.invoked[token]
}

func (m *vmMocks) inputsOf(typeToken string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			return a.Inputs
		}
	}
	return nil
}

// allInputsOf is inputsOf for the cases where the COUNT is not the assertion — a
// driver that creates several of one type (security-group rules) needs every one of
// them checked, because three rules of the right shape plus one of the wrong shape
// passes a count.
func (m *vmMocks) allInputsOf(typeToken string) []resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []resource.PropertyMap
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			out = append(out, a.Inputs)
		}
	}
	return out
}

func (m *vmMocks) countOf(typeToken string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, a := range m.created {
		if a.TypeToken == typeToken {
			n++
		}
	}
	return n
}

// unwrapSecret peels secret and output wrappers off a property, reporting whether
// anything on the way down was marked secret. Needed because the connection object
// may be marked at the object level (an Output built from a secret field) or at the
// field level, and both are correct — what would be wrong is neither.
func unwrapSecret(pv resource.PropertyValue) (resource.PropertyValue, bool) {
	secret := false
	for {
		switch {
		case pv.IsSecret():
			secret = true
			pv = pv.SecretValue().Element
		case pv.IsOutput():
			o := pv.OutputValue()
			secret = secret || o.Secret
			pv = o.Element
		default:
			return pv, secret
		}
	}
}

func byoConfig() *vm.VmConfig {
	return &vm.VmConfig{
		Driver: vm.DriverByo,
		Hosts: []vm.VmHost{{
			Address:    testHostAddress,
			PrivateKey: testPrivateKey,
		}},
	}
}

func resourceInput(cfg *vm.VmConfig) api.ResourceInput {
	return api.ResourceInput{
		Descriptor: &api.ResourceDescriptor{
			Type:   vm.ResourceTypeVm,
			Name:   "portal-vm",
			Config: api.Config{Config: cfg},
		},
		StackParams: &api.StackParams{Environment: "test"},
	}
}

// provisionVm walks the real path: Provider() first (provision.go's initProvider
// runs before any ProvisionFunc), then Vm() with that provider in ProvisionParams.
func provisionVm(cfg *vm.VmConfig, mocks *vmMocks) (*VmOutput, error) {
	var out *VmOutput
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		params := pApi.ProvisionParams{Log: logger.New()}
		input := resourceInput(cfg)

		prov, err := Provider(ctx, api.Stack{Name: "test-stack"}, input, params)
		if err != nil {
			return err
		}
		provider, ok := prov.Ref.(pulumi.ProviderResource)
		if !ok {
			return errors.Errorf("Provider().Ref is %T, not a pulumi.ProviderResource", prov.Ref)
		}
		params.Provider = provider

		res, err := Vm(ctx, api.Stack{Name: "test-stack"}, input, params)
		if err != nil {
			return err
		}
		out, ok = res.Ref.(*VmOutput)
		if !ok {
			return errors.Errorf("Vm().Ref is %T, not a *VmOutput", res.Ref)
		}
		return nil
	}, pulumi.WithMocks("project", "stack", mocks))
	return out, err
}

// Provider().Ref must satisfy pulumi.ProviderResource — provision.go hard-casts it
// and fails the whole stack if it does not.
func TestProvider_ReturnsProviderResource(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(byoConfig(), mocks)

	Expect(err).To(BeNil())
	Expect(mocks.countOf(tokenCommandProvider)).To(Equal(1),
		"a byo host has no cloud API in play, only SSH, so the provider is pulumi-command's")
}

// Provider is the earliest point at which a vm config is seen, so it is where an
// operator's typo has to surface. Nothing may be created before it does.
func TestProvider_ValidatesBeforeCreatingAnything(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Region = "ru-moscow-1" // meaningless for byo: SC did not create the machine

	mocks := newVmMocks()
	_, err := provisionVm(cfg, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`"region" is not valid with`))
	Expect(mocks.countOf(tokenCommandProvider)).To(Equal(0))
	Expect(mocks.countOf(tokenRemoteCommand)).To(Equal(0))
}

func TestProvider_RefusesUnimplementedDriver(t *testing.T) {
	RegisterTestingT(t)

	// MWS rather than Cloud.ru since `cloudru-vm` landed. The refusal has to keep
	// being exercised: it is what an operator on one of the five remaining clouds
	// sees, and driverFor has no entry for them — so without this check the failure
	// would be an "unsupported driver" from one layer deeper, after a provider had
	// already been built.
	mocks := newVmMocks()
	_, err := provisionVm(&vm.VmConfig{Driver: vm.DriverMws, Region: "ru-central-1", PublicIP: true}, mocks)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not implemented yet"))
	Expect(mocks.countOf(tokenCommandProvider)).To(Equal(0))
}

// THE assertion of this package, and it is deliberately made against the driver
// rather than against the command's recorded inputs.
//
// The reason: pulumi-command's own remote.NewCommand wraps the WHOLE Connection in
// ToSecret (sdk@v1.2.1 remote/command.go:184), so an assertion on what reaches
// `command:remote:Command` passes whether or not SC marked anything — verified by
// mutation on 2026-10-15: dropping ToSecret from byo.go's secretString left that
// assertion green. HostFacts.PrivateKey is read by more than the readiness command
// (the vm-compose template next, k3s after), so it has to be secret at the point it
// enters Pulumi, which is here.
//
// Why it matters: a credential is an ordinary Pulumi property, and an unmarked one
// lands in the preview and update summaries and from there in a CI log — which is
// how a real service-account private key reached a world-readable GitHub Actions
// log on 2026-09-30.
func TestByoDriver_MarksHostKeysSecretWhereTheyEnterPulumi(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Hosts[0].PrivateKeyPassword = "test-passphrase"

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		hosts, err := byoDriver{}.Hosts(ctx, resourceInput(cfg), cfg, pApi.ProvisionParams{Log: logger.New()})
		if err != nil {
			return err
		}
		Expect(hosts).To(HaveLen(1))

		key, err := internals.UnsafeAwaitOutput(ctx.Context(), hosts[0].PrivateKey)
		if err != nil {
			return err
		}
		Expect(key.Secret).To(BeTrue(),
			"the deploy private key entered Pulumi unmarked — it will be printed verbatim in the "+
				"preview and update summaries and archived in CI logs")
		// ...and arrives intact. Masking is the engine's job, so a secret still
		// carries the plaintext; that is not the leak. The leak is being unmarked.
		Expect(key.Value).To(Equal(testPrivateKey))

		// The passphrase is a second, independent credential: a key file alone is
		// useless without it, which is exactly why both halves travel together and
		// both have to be marked.
		pass, err := internals.UnsafeAwaitOutput(ctx.Context(), hosts[0].PrivateKeyPassword)
		if err != nil {
			return err
		}
		Expect(pass.Secret).To(BeTrue())
		Expect(pass.Value).To(Equal("test-passphrase"))

		// The address is not a credential, and masking it makes a diff unreadable for
		// nothing.
		addr, err := internals.UnsafeAwaitOutput(ctx.Context(), hosts[0].Address)
		if err != nil {
			return err
		}
		Expect(addr.Secret).To(BeFalse())
		Expect(addr.Value).To(Equal(testHostAddress))
		return nil
	}, pulumi.WithMocks("project", "stack", newVmMocks()))
	Expect(err).To(BeNil())
}

// Pins the upstream behaviour the test above works around: pulumi-command marks the
// whole connection secret itself. It is load-bearing — every connection SC builds
// carries a key — so an SDK upgrade that drops it has to fail here rather than in a
// CI log.
func TestVm_ConnectionIsSecretAtTheCommand(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(byoConfig(), mocks)
	Expect(err).To(BeNil())

	inputs := mocks.inputsOf(tokenRemoteCommand)
	Expect(inputs).NotTo(BeNil(), "no readiness command was created")
	Expect(inputs["connection"].ContainsSecrets()).To(BeTrue(),
		"pulumi-command no longer marks the SSH connection secret; mark it in ConnectionArgs")

	conn, _ := unwrapSecret(inputs["connection"])
	key, _ := unwrapSecret(conn.ObjectValue()["privateKey"])
	Expect(key.StringValue()).To(Equal(testPrivateKey))
}

// An unencrypted deploy key must send NO password, not an empty one.
//
// The provider passes whatever it receives to ssh.ParsePrivateKeyWithPassphrase,
// which answers `ssh: key is not password protected` for a key that has no
// passphrase — so an empty string fails every connection to the host, readiness
// included, with a message that reads like the key itself is wrong. Live-caught on
// the first byo smoke, 2026-10-18.
func TestVm_UnencryptedKeySendsNoKeyPassword(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(byoConfig(), mocks)
	Expect(err).To(BeNil())

	inputs := mocks.inputsOf(tokenRemoteCommand)
	Expect(inputs).NotTo(BeNil(), "no readiness command was created")
	conn, _ := unwrapSecret(inputs["connection"])

	pass, present := conn.ObjectValue()["privateKeyPassword"]
	if present {
		unwrapped, _ := unwrapSecret(pass)
		Expect(unwrapped.IsNull()).To(BeTrue(),
			"privateKeyPassword must be absent or null for an unencrypted key, got %v", unwrapped)
	}
}

// The other half of the same contract: a key that IS an encrypted PEM must still
// have its passphrase delivered, and delivered as a secret.
func TestVm_EncryptedKeySendsItsPassword(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Hosts[0].PrivateKeyPassword = "hunter2"

	mocks := newVmMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	inputs := mocks.inputsOf(tokenRemoteCommand)
	Expect(inputs).NotTo(BeNil(), "no readiness command was created")
	conn, _ := unwrapSecret(inputs["connection"])

	pass, present := conn.ObjectValue()["privateKeyPassword"]
	Expect(present).To(BeTrue(), "privateKeyPassword is missing for an encrypted key")
	unwrapped, _ := unwrapSecret(pass)
	Expect(unwrapped.StringValue()).To(Equal("hunter2"))
}

func TestVm_ConnectionCarriesHostFacts(t *testing.T) {
	RegisterTestingT(t)

	cfg := byoConfig()
	cfg.Hosts[0].User = "ubuntu"
	cfg.Hosts[0].Port = 2222

	mocks := newVmMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	conn, _ := unwrapSecret(mocks.inputsOf(tokenRemoteCommand)["connection"])
	obj := conn.ObjectValue()

	host, _ := unwrapSecret(obj["host"])
	Expect(host.StringValue()).To(Equal(testHostAddress))
	user, _ := unwrapSecret(obj["user"])
	Expect(user.StringValue()).To(Equal("ubuntu"))
	port, _ := unwrapSecret(obj["port"])
	Expect(port.NumberValue()).To(Equal(float64(2222)))

	// Explicit, not defaulted: pulumi-command retries a dial a fixed number of times
	// and gives up, and the default is too impatient for a machine that is still
	// booting (vm-matrix.md §3.4).
	dialLimit, _ := unwrapSecret(obj["dialErrorLimit"])
	Expect(dialLimit.NumberValue()).To(Equal(float64(dialErrorLimit)))
	perDial, _ := unwrapSecret(obj["perDialTimeout"])
	Expect(perDial.NumberValue()).To(Equal(float64(perDialTimeoutSeconds)))
}

// Three pulumi-command semantics that all fail *green* (vm-matrix.md §0.7). Each one
// is an invariant of every command in this package, so they are asserted together:
// the point is the shape a new command inherits, not this command in particular.
func TestVm_CommandHasSafeDefaults(t *testing.T) {
	RegisterTestingT(t)

	mocks := newVmMocks()
	_, err := provisionVm(byoConfig(), mocks)
	Expect(err).To(BeNil())

	inputs := mocks.inputsOf(tokenRemoteCommand)

	// Without Triggers the command never re-runs once it has succeeded: a later
	// `sc deploy` reports green having checked nothing, including against a host
	// that was rebuilt in the meantime.
	//
	// Non-empty is NOT the property that matters, and asserting only that is how
	// this shipped broken: the host address, user, port and script are all invariant
	// between two deploys of one stack, so the four of them together behaved exactly
	// like no triggers. The gate only reads the machine, so it takes the per-run
	// nonce (nonce.go), and that is what is asserted.
	triggers, _ := unwrapSecret(inputs["triggers"])
	Expect(triggers.IsArray()).To(BeTrue(), "the readiness command has no Triggers")
	Expect(triggers.ArrayValue()).ToNot(BeEmpty())
	Expect(triggers.String()).To(ContainSubstring(runNonce),
		"the readiness gate is not triggered per run, so after the first deploy of a stack it "+
			"reports ready without asking the host")

	// Update, same value as Create. Without it a trigger change is a REPLACEMENT,
	// and with a per-run nonce above that means replacing this resource on every
	// deploy — while the commands that DependsOn it are gated on it.
	update, _ := unwrapSecret(inputs["update"])
	create, _ := unwrapSecret(inputs["create"])
	Expect(update.StringValue()).To(Equal(create.StringValue()),
		"the readiness command has no explicit Update, so every deploy replaces it")

	logging, _ := unwrapSecret(inputs["logging"])
	Expect(logging.StringValue()).To(Equal("none"))

	addPrev, _ := unwrapSecret(inputs["addPreviousOutputInEnv"])
	Expect(addPrev.BoolValue()).To(BeFalse())

	// Secrets ride Stdin, never Environment: Environment inlines the value into the
	// command line and depends on sshd's AcceptEnv to arrive at all.
	Expect(inputs).ToNot(HaveKey(resource.PropertyKey("environment")))
}

// A byo host has already booted, so readiness is a precondition check — never an
// install on a machine SC does not own. Each check has to name what the operator
// must go and do, because the alternative is a bare `exit 1` from a remote command.
func TestReadinessScript_ByoChecksPreconditions(t *testing.T) {
	RegisterTestingT(t)

	script := readinessScript(false, "ubuntu")

	Expect(script).To(ContainSubstring("sudo -n true"))
	Expect(script).To(ContainSubstring("install -d -m 0750 -o ubuntu -g ubuntu " + vm.ConfigDir + " " + vm.StateDir))
	// Must NOT install anything: the host belongs to the operator.
	Expect(script).ToNot(ContainSubstring("get.docker.com"))
	Expect(script).ToNot(ContainSubstring("apt-get"))
	// Every check is a guard with a message, so a failure explains itself.
	Expect(strings.Count(script, ">&2")).To(BeNumerically(">=", 1))
}

// The gate is a property of the MACHINE, and a k3s node is a ready machine with no
// docker on it. Pinned because the whole k3s byo path hangs off it: the k3s install
// DependsOn this command, so a docker check here fails a cluster that was never
// going to run docker (live-caught 2026-10-18). The checks live in
// composePreflightScript instead.
func TestReadinessScript_ByoDoesNotRequireDocker(t *testing.T) {
	RegisterTestingT(t)

	script := readinessScript(false, "ubuntu")

	Expect(script).ToNot(ContainSubstring("docker"))
}

// For a machine SC created, readiness is the marker cloud-init writes last — not
// SSH answering, which it does well before docker exists.
func TestReadinessScript_CloudInitWaitsForTheMarker(t *testing.T) {
	RegisterTestingT(t)

	script := readinessScript(true, "sc")

	Expect(script).To(ContainSubstring(vm.CloudInitDoneMarker))
	// The only thing that distinguishes "did not finish in time" from "cloud-init
	// failed"; without it the next person debugging this is guessing.
	Expect(script).To(ContainSubstring("cloud-init status --long >&2"))
	Expect(script).To(ContainSubstring("exit 1"))
}

func TestVm_ExportsHostFacts(t *testing.T) {
	RegisterTestingT(t)

	out, err := provisionVm(byoConfig(), newVmMocks())
	Expect(err).To(BeNil())

	Expect(out.Hosts).To(HaveLen(1))
	Expect(out.Ready).To(HaveLen(1), "every host must carry a readiness gate for templates to DependsOn")
	Expect(out.Hosts[0].Role).To(Equal(vm.RoleCompose))

	// Stable names, read back through the same helpers by whatever consumes them.
	Expect(ToHostCountExport("portal-vm")).To(Equal("portal-vm-vm-host-count"))
	Expect(ToHostAddressExport("portal-vm", 0)).To(Equal("portal-vm-vm-host-0-address"))
	Expect(ToHostPrivateKeyExport("portal-vm", 0)).To(Equal("portal-vm-vm-host-0-private-key"))
	Expect(ToIngressEnabledExport("portal-vm")).To(Equal("portal-vm-vm-ingress-enabled"))
}

// Vm is reachable on its own, so it re-checks what Provider checked: a nil provider
// panics inside the resource constructors, and an unvalidated config would get as
// far as SSH before failing.
func TestVm_RefusesWithoutProvider(t *testing.T) {
	RegisterTestingT(t)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := Vm(ctx, api.Stack{Name: "test-stack"}, resourceInput(byoConfig()),
			pApi.ProvisionParams{Log: logger.New()})
		return err
	}, pulumi.WithMocks("project", "stack", newVmMocks()))

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("provider must not be nil"))
}

func TestDriverFor(t *testing.T) {
	RegisterTestingT(t)

	drv, err := driverFor(byoConfig())
	Expect(err).ToNot(HaveOccurred())
	Expect(drv.Name()).To(Equal(vm.DriverByo))
	// A byo host already booted: there is no user-data to run, so readiness is a
	// precondition check rather than a wait.
	Expect(drv.Capabilities().CloudInit).To(BeFalse())

	_, err = driverFor(&vm.VmConfig{Driver: vm.DriverMws})
	Expect(err).To(HaveOccurred())
}
