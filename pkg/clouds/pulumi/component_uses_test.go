// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

const (
	usesTestResType  = "test-uses/notifier"
	usesTestPlainTyp = "test-uses/plain"
)

// usesTestCall records one invocation of the fake compute processor —
// everything the real notifier plugin reads off its inputs.
type usesTestCall struct {
	stackName  string
	parentInfo *pApi.ParentInfo
	outs       pApi.ResourcesOutputs
}

// registerUsesTestProcessor installs a compute processor for
// usesTestResType that behaves like the notifier's: it publishes a URL
// that is the same for everyone and a token derived from WHO IS ASKING
// (the consumer's stack name — the notifier mints it into the JWT's
// `tenant` claim, see sc-component-notifier/plugin/internal/compute.go).
//
// The globals it writes to are package-level maps populated at plugin
// load, so they are saved and restored rather than appended to.
func registerUsesTestProcessor(t *testing.T) *[]usesTestCall {
	t.Helper()
	prevProcessors := pApi.ComputeProcessorFuncByType
	prevPluginTypes := pApi.PluginResourceTypes
	t.Cleanup(func() {
		pApi.ComputeProcessorFuncByType = prevProcessors
		pApi.PluginResourceTypes = prevPluginTypes
	})
	pApi.PluginResourceTypes = map[string]struct{}{}
	// Plugin-dispatched types skip initProvider in getProvisionParams,
	// which is what lets this run without a cloud provider. A type that
	// is NOT registered here makes getProvisionParams fail, and
	// processResourceDependency swallows that failure as a warning — so
	// the processor would never be called and these tests would pass
	// vacuously.
	pApi.RegisterPluginType(usesTestResType)
	pApi.RegisterPluginType(usesTestPlainTyp)

	var calls []usesTestCall
	pApi.RegisterComputeProcessor(map[string]pApi.ComputeProcessorFunc{
		usesTestResType: func(_ *sdk.Context, _ api.Stack, input api.ResourceInput,
			collector pApi.ComputeContextCollector, params pApi.ProvisionParams,
		) (*api.ResourceOutput, error) {
			consumer := input.StackParams.StackName
			calls = append(calls, usesTestCall{
				stackName:  consumer,
				parentInfo: params.ParentStack,
				outs:       params.ResourceOutputs,
			})
			collector.AddEnvVariableIfNotExist("SC_NOTIFY_URL", "https://notify.example.com",
				input.Descriptor.Type, input.Descriptor.Name, consumer)
			collector.AddSecretEnvVariableIfNotExist("SC_NOTIFY_TOKEN", "token-for:"+consumer,
				input.Descriptor.Type, input.Descriptor.Name, consumer)
			collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
				"url":   "https://notify.example.com",
				"token": "token-for:" + consumer,
			})
			return &api.ResourceOutput{}, nil
		},
	})
	return &calls
}

func usesTestStack(env string, resources map[string]api.ResourceDescriptor) api.Stack {
	return api.Stack{
		Name: "infra",
		Server: api.ServerDescriptor{
			Resources: api.PerStackResourcesDescriptor{
				Resources: map[string]api.PerEnvResourcesDescriptor{
					env: {Resources: resources},
				},
			},
		},
	}
}

func usesTestComponent(uses any) api.ResourceDescriptor {
	raw := map[string]any{"deployTo": "lambda"}
	if uses != nil {
		raw["uses"] = uses
	}
	return api.ResourceDescriptor{
		Type:   usesTestPlainTyp,
		Config: api.Config{Config: &api.PluginFieldConfig{Raw: raw}},
	}
}

func usesTestProvisioner() *pulumi {
	return &pulumi{
		logger:         logger.New(),
		provisionerCfg: &ProvisionerConfig{Organization: "org"},
	}
}

// Hazard 1, the one a smoke cannot see.
//
// The provision program shares ONE collector across every resource in an
// environment. `AddResourceTplExtension` overwrites by resource name
// while `AddEnvVariableIfNotExist` is first-write-wins, so two consumers
// of the same resource on that collector would disagree about whose
// credential is whose — in OPPOSITE directions for the env var and the
// placeholder. With one publisher deployed everything looks right; the
// second publisher silently authenticates as the first, and an alert
// attributed to the wrong tenant is indistinguishable from a correct one.
//
// Hence a fresh collector per consumer. This test is the proof that the
// identity actually differs per consumer AND that nothing consumer-scoped
// reaches the shared collector.
func TestProcessComponentUses_PerConsumerIsolation(t *testing.T) {
	RegisterTestingT(t)
	calls := registerUsesTestProcessor(t)

	const env = "staging"
	stack := usesTestStack(env, map[string]api.ResourceDescriptor{
		"my-notifier": {Type: usesTestResType, Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{}}}},
		// List form and bare-string form, so both reach the fan-out.
		"my-backup":      usesTestComponent([]any{"my-notifier"}),
		"my-observatory": usesTestComponent("my-notifier"),
	})
	outs := pApi.ResourcesOutputs{"my-notifier": &api.ResourceOutput{}}
	shared := pApi.NewComputeContextCollector(context.Background(), logger.New(), stack.Name, env)
	p := usesTestProvisioner()

	consumers := map[string]pApi.ComputeContextCollector{}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		for _, resName := range []string{"my-backup", "my-observatory"} {
			consumer, err := p.processComponentUses(ctx, stack, &api.ConfigFile{ProjectName: "forge"},
				env, resName, stack.Server.Resources.Resources[env].Resources[resName], shared, outs)
			Expect(err).To(BeNil())
			Expect(consumer).NotTo(BeNil(), "resource %q declares uses: but got no consumer context", resName)
			consumers[resName] = consumer
		}
		return nil
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).To(BeNil())

	// The processor really ran, once per consumer — otherwise everything
	// below is vacuously true.
	Expect(*calls).To(HaveLen(2))

	tokens := map[string]string{}
	for resName, consumer := range consumers {
		secrets := consumer.SecretEnvVariables()
		Expect(secrets).To(HaveLen(1))
		Expect(secrets[0].Name).To(Equal("SC_NOTIFY_TOKEN"))
		tokens[resName] = secrets[0].Value

		plain := consumer.EnvVariables()
		Expect(plain).To(HaveLen(1))
		Expect(plain[0].Name).To(Equal("SC_NOTIFY_URL"))
	}

	// The identity is `<parentStack>--<resourceName>`: unique per
	// consumer, distinct from a same-named client stack, and readable in
	// an alert. Passing stack.Name alone would give every component in
	// the parent stack one shared tenant.
	Expect(tokens["my-backup"]).To(Equal("token-for:infra--my-backup"))
	Expect(tokens["my-observatory"]).To(Equal("token-for:infra--my-observatory"))
	Expect(tokens["my-backup"]).NotTo(Equal(tokens["my-observatory"]))

	// And the shared collector carries no consumer-scoped value at all.
	Expect(shared.SecretEnvVariables()).To(BeEmpty(),
		"a per-consumer token reached the env-wide collector — the next consumer would inherit it")
	Expect(shared.EnvVariables()).To(BeEmpty())
	// A per-consumer placeholder on the shared collector is the same leak
	// in the other direction: last writer wins there, so the FIRST
	// consumer would resolve to the second's token.
	var probe struct {
		Token string
	}
	probe.Token = "${resource:my-notifier.token}"
	Expect(shared.ResolvePlaceholders(&probe)).To(Succeed())
	Expect(probe.Token).To(Equal("${resource:my-notifier.token}"),
		"the shared collector resolved a consumer-scoped token, so it holds one consumer's credential for everyone")

	// Each consumer's own collector does resolve it — that is D9.
	probe.Token = "${resource:my-notifier.token}"
	Expect(consumers["my-observatory"].ResolvePlaceholders(&probe)).To(Succeed())
	Expect(probe.Token).To(Equal("token-for:infra--my-observatory"))
}

// What the fan-out must tell the provider, beyond the consumer's name:
// that this is a `uses:` relationship, and which outputs the used
// resource produced. The second one is why `resourceOutputs` is threaded
// through at all — the notifier's deployed endpoint URL is read from
// `_endpoint_url` in the outputs, and the client-stack path has no
// equivalent (it passes nil and providers fall back to the declared
// ingress hostname).
func TestProcessComponentUses_PassesParentInfoAndOutputs(t *testing.T) {
	RegisterTestingT(t)
	calls := registerUsesTestProcessor(t)

	const env = "staging"
	stack := usesTestStack(env, map[string]api.ResourceDescriptor{
		"my-notifier": {Type: usesTestResType, Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{}}}},
		"my-backup":   usesTestComponent([]any{"my-notifier"}),
	})
	outs := pApi.ResourcesOutputs{"my-notifier": &api.ResourceOutput{}}
	shared := pApi.NewComputeContextCollector(context.Background(), logger.New(), stack.Name, env)
	p := usesTestProvisioner()

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := p.processComponentUses(ctx, stack, &api.ConfigFile{ProjectName: "forge"},
			env, "my-backup", stack.Server.Resources.Resources[env].Resources["my-backup"], shared, outs)
		return err
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).To(BeNil())

	Expect(*calls).To(HaveLen(1))
	got := (*calls)[0]
	Expect(got.parentInfo).NotTo(BeNil())
	Expect(got.parentInfo.UsesResource).To(BeTrue(),
		"the provider cannot tell a `uses:` consumer from a `dependencies:` one")
	Expect(got.parentInfo.StackName).To(Equal("infra"))
	// `<org>/<project>/<parentStack>` — the same expansion
	// provisionProgram uses for the template outputs it exports, so a
	// provider reading it on the `uses:` path sees the reference shape it
	// already sees everywhere else.
	Expect(got.parentInfo.FullReference).To(Equal("org/forge/infra"))
	Expect(got.outs).To(HaveKey("my-notifier"),
		"the used resource's outputs did not reach the provider — it cannot find the endpoint it is wiring")
}

// The overwhelmingly common case: no `uses:`, nothing happens, and the
// nil return is what keeps every pre-existing dispatch byte-identical.
func TestProcessComponentUses_NoUsesIsANoOp(t *testing.T) {
	RegisterTestingT(t)
	calls := registerUsesTestProcessor(t)

	const env = "staging"
	stack := usesTestStack(env, map[string]api.ResourceDescriptor{
		"my-notifier": {Type: usesTestResType, Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{}}}},
		"my-backup":   usesTestComponent(nil),
		// A typed (non-plugin) config: `uses:` is a Component
		// declaration, and an in-tree type expresses dependencies
		// through its own DependsOnResources.
		"typed": {Type: usesTestPlainTyp, Config: api.Config{Config: fakePlain{}}},
	})
	shared := pApi.NewComputeContextCollector(context.Background(), logger.New(), stack.Name, env)
	p := usesTestProvisioner()

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		for _, resName := range []string{"my-backup", "typed", "my-notifier"} {
			consumer, err := p.processComponentUses(ctx, stack, &api.ConfigFile{ProjectName: "forge"},
				env, resName, stack.Server.Resources.Resources[env].Resources[resName], shared, nil)
			Expect(err).To(BeNil())
			Expect(consumer).To(BeNil(), "resource %q has no uses: but got a consumer context", resName)
		}
		return nil
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).To(BeNil())
	Expect(*calls).To(BeEmpty())
}

// A used resource whose type registers no compute processor cannot
// publish anything. Warned, not fatal — matching what the client-stack
// path does with the same shape — but the consumer context still comes
// back non-nil, because the operator asked for wiring.
func TestProcessComponentUses_UsedResourceWithoutProcessor(t *testing.T) {
	RegisterTestingT(t)
	registerUsesTestProcessor(t)

	const env = "staging"
	stack := usesTestStack(env, map[string]api.ResourceDescriptor{
		"my-bucket": {Type: usesTestPlainTyp, Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{}}}},
		"my-backup": usesTestComponent([]any{"my-bucket"}),
	})
	shared := pApi.NewComputeContextCollector(context.Background(), logger.New(), stack.Name, env)
	p := usesTestProvisioner()

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		consumer, err := p.processComponentUses(ctx, stack, &api.ConfigFile{ProjectName: "forge"},
			env, "my-backup", stack.Server.Resources.Resources[env].Resources["my-backup"], shared, nil)
		Expect(err).To(BeNil())
		Expect(consumer).NotTo(BeNil())
		Expect(consumer.EnvVariables()).To(BeEmpty())
		Expect(consumer.SecretEnvVariables()).To(BeEmpty())
		return nil
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).To(BeNil())
}

// Hazard 5. Both refusals are loud because neither has any other
// symptom: nothing is injected, the component deploys, and the failure
// surfaces much later as a service missing an env var it never knew to
// expect.
func TestResolveUsedSiblings_Refusals(t *testing.T) {
	siblings := map[string]api.ResourceDescriptor{
		"my-notifier": {Type: usesTestResType},
		"my-backup":   {Type: usesTestPlainTyp},
		"my-bucket":   {Type: usesTestPlainTyp},
	}
	tests := []struct {
		name      string
		resName   string
		used      []string
		wantErr   []string
		wantNames []string
	}{
		{
			name:      "resolves in declaration order",
			resName:   "my-backup",
			used:      []string{"my-notifier", "my-bucket"},
			wantNames: []string{"my-notifier", "my-bucket"},
		},
		{
			name:    "self-use",
			resName: "my-backup",
			used:    []string{"my-backup"},
			wantErr: []string{"cannot consume itself"},
		},
		{
			name:    "unknown name lists what is available",
			resName: "my-backup",
			used:    []string{"my-notify"},
			wantErr: []string{
				`no resource named "my-notify" is declared in env "staging"`,
				"my-backup, my-bucket, my-notifier",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := resolveUsedSiblings(siblings, "staging", tt.resName, tt.used)
			if len(tt.wantErr) > 0 {
				Expect(err).NotTo(BeNil())
				for _, want := range tt.wantErr {
					Expect(err.Error()).To(ContainSubstring(want))
				}
				return
			}
			Expect(err).To(BeNil())
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.name)
				// A descriptor declared without an explicit `name:` is
				// keyed only by its map key; a provider that derives a
				// resource name from it would otherwise get "".
				Expect(s.res.Name).To(Equal(s.name))
			}
			Expect(names).To(Equal(tt.wantNames))
		})
	}
}

// Hazard 4 — the `SC_` namespace guard (D8), which is the property that
// makes auto-injection safe rather than merely convenient. A plugin that
// could inject `NOTIFY_URL` could inject `DATABASE_URL` or `PATH`: the
// application keeps starting, reads the plugin's value, and nothing in
// the diff says why.
func TestMergeConsumerEnvIntoComponentRaw(t *testing.T) {
	tests := []struct {
		name        string
		merged      map[string]any
		plain       []pApi.ComputeEnvVariable
		secret      []pApi.ComputeEnvVariable
		wantErr     []string
		wantEnv     map[string]any
		wantSecrets map[string]any
	}{
		{
			name:        "plain and secret land in their own keys",
			merged:      map[string]any{},
			plain:       []pApi.ComputeEnvVariable{{Name: "SC_NOTIFY_URL", Value: "https://notify"}},
			secret:      []pApi.ComputeEnvVariable{{Name: "SC_NOTIFY_TOKEN", Value: "tok"}},
			wantEnv:     map[string]any{"SC_NOTIFY_URL": "https://notify"},
			wantSecrets: map[string]any{"SC_NOTIFY_TOKEN": "tok"},
		},
		{
			// An injected value is a default the operator never typed.
			// Overwriting a line they DID type would make the config file
			// stop describing the deployment.
			name: "operator-written keys win",
			merged: map[string]any{
				"env":     map[string]any{"SC_NOTIFY_URL": "https://my-own"},
				"secrets": map[string]any{"SC_NOTIFY_TOKEN": "${secret:mine}"},
			},
			plain:       []pApi.ComputeEnvVariable{{Name: "SC_NOTIFY_URL", Value: "https://notify"}},
			secret:      []pApi.ComputeEnvVariable{{Name: "SC_NOTIFY_TOKEN", Value: "tok"}},
			wantEnv:     map[string]any{"SC_NOTIFY_URL": "https://my-own"},
			wantSecrets: map[string]any{"SC_NOTIFY_TOKEN": "${secret:mine}"},
		},
		{
			name:   "unprefixed name refused, naming the publisher",
			merged: map[string]any{},
			plain: []pApi.ComputeEnvVariable{{
				Name: "NOTIFY_URL", Value: "https://notify",
				ResourceName: "my-notifier", ResourceType: usesTestResType,
			}},
			wantErr: []string{
				`resource "my-notifier"`,
				usesTestResType,
				`inject env variable "NOTIFY_URL"`,
				"^SC_[A-Z0-9_]+$",
				"rename it to SC_NOTIFY_URL",
			},
		},
		{
			// The guard has to hold on the secret side too, which is the
			// side that reaches SecretsManager.
			name:   "unprefixed secret name refused",
			merged: map[string]any{},
			secret: []pApi.ComputeEnvVariable{{
				Name: "DATABASE_URL", ResourceName: "my-notifier", ResourceType: usesTestResType,
			}},
			wantErr: []string{`inject env variable "DATABASE_URL"`},
		},
		{
			name:    "lowercase prefix is not the namespace",
			merged:  map[string]any{},
			plain:   []pApi.ComputeEnvVariable{{Name: "sc_notify_url", ResourceName: "my-notifier"}},
			wantErr: []string{`inject env variable "sc_notify_url"`},
		},
		{
			// `SC_` alone is a prefix, not a name.
			name:    "bare prefix refused",
			merged:  map[string]any{},
			plain:   []pApi.ComputeEnvVariable{{Name: "SC_", ResourceName: "my-notifier"}},
			wantErr: []string{`inject env variable "SC_"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			consumer := &fakeConsumerContext{plain: tt.plain, secret: tt.secret}
			err := mergeConsumerEnvIntoComponentRaw(tt.merged, consumer)
			if len(tt.wantErr) > 0 {
				Expect(err).NotTo(BeNil())
				for _, want := range tt.wantErr {
					Expect(err.Error()).To(ContainSubstring(want))
				}
				return
			}
			Expect(err).To(BeNil())
			if tt.wantEnv == nil {
				Expect(tt.merged).NotTo(HaveKey("env"))
			} else {
				Expect(tt.merged["env"]).To(Equal(tt.wantEnv))
			}
			if tt.wantSecrets == nil {
				Expect(tt.merged).NotTo(HaveKey("secrets"))
			} else {
				Expect(tt.merged["secrets"]).To(Equal(tt.wantSecrets))
			}
		})
	}
}

// A nil consumer context is the no-`uses:` path, and it must leave the
// component's raw config untouched rather than creating empty `env:` /
// `secrets:` maps that a template would then render.
func TestMergeConsumerEnvIntoComponentRaw_NilConsumer(t *testing.T) {
	RegisterTestingT(t)
	merged := map[string]any{"deployTo": "lambda"}
	Expect(mergeConsumerEnvIntoComponentRaw(merged, nil)).To(Succeed())
	Expect(merged).To(Equal(map[string]any{"deployTo": "lambda"}))
}

// Hazard 2 — the fail-open literal.
//
// `ResolvePlaceholders` returns the input unchanged on a miss, so an
// unresolved `${resource:my-notifier.token}` is DEPLOYED as that literal
// string, where it reads as a value: a publisher authenticating with the
// characters `${resource` and failing with an error that names nothing
// relevant. The operator wrote a reference; they get the value or an
// error.
func TestRefuseUnresolvedUsesPlaceholders(t *testing.T) {
	tests := []struct {
		name    string
		merged  map[string]any
		used    []string
		wantErr []string
	}{
		{
			name:   "resolved values pass",
			merged: map[string]any{"env": map[string]any{"U": "https://notify"}},
			used:   []string{"my-notifier"},
		},
		{
			name:   "no uses: declared means no guard",
			merged: map[string]any{"env": map[string]any{"U": "${resource:my-notifier.token}"}},
			used:   nil,
		},
		{
			name:   "unresolved token naming a used resource",
			merged: map[string]any{"env": map[string]any{"U": "${resource:my-notifier.tokne}"}},
			used:   []string{"my-notifier"},
			wantErr: []string{
				`component "my-backup"`,
				"${resource:my-notifier.tokne}",
				"fails open",
			},
		},
		{
			// Deliberately narrow: a ComponentDefaults
			// `${resource:<self>.<key>}` is MEANT to survive this pass and
			// resolve later, at a consumer's own deploy. A blanket scan of
			// `merged` would refuse the notifier's own config.
			name:   "a token naming a resource outside uses: is left alone",
			merged: map[string]any{"env": map[string]any{"U": "${resource:my-backup.self-url}"}},
			used:   []string{"my-notifier"},
		},
		{
			// yaml.v2 decodes a nested mapping into map[any]any, and a
			// reference sits under a nested list as readily as at the top
			// level. Missing either shape would make the guard silently
			// depend on the decoder.
			name: "nested map[any]any and []any leaves are caught",
			merged: map[string]any{
				"config": map[any]any{
					"channels": []any{
						map[string]any{"url": "${resource:my-notifier.url}"},
					},
				},
			},
			used:    []string{"my-notifier"},
			wantErr: []string{"${resource:my-notifier.url}"},
		},
		{
			name: "every offender is reported, sorted, once",
			merged: map[string]any{
				"env": map[string]any{
					"A": "${resource:my-notifier.token}",
					"B": "${resource:my-notifier.token}",
					"C": "${resource:my-notifier.url}",
				},
			},
			used: []string{"my-notifier"},
			wantErr: []string{
				"${resource:my-notifier.token}, ${resource:my-notifier.url}",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := refuseUnresolvedUsesPlaceholders(tt.merged, "my-backup", tt.used)
			if len(tt.wantErr) > 0 {
				Expect(err).NotTo(BeNil())
				for _, want := range tt.wantErr {
					Expect(err.Error()).To(ContainSubstring(want))
				}
				return
			}
			Expect(err).To(BeNil())
		})
	}
}

// Hazard 3 — ordering, at the layer that decides it.
//
// This is the failure `plugin_field_deps.go` documents with this exact
// pair: `my-backup` < `my-notifier` alphabetically, so without a
// dependency edge the consumer provisions FIRST and the fail-open
// resolver hands it the literal placeholder. Renaming the notifier
// `alerts` would have "fixed" it, which is the signature of a landmine.
func TestResourceProvisionOrder_UsesOrdersConsumerAfterProvider(t *testing.T) {
	order, err := resourceProvisionOrder(map[string]api.ResourceDescriptor{
		"my-backup": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{
			"uses": []any{"my-notifier"},
		}}}},
		"my-notifier": {Config: api.Config{Config: &api.PluginFieldConfig{Raw: map[string]any{}}}},
	})
	if err != nil {
		t.Fatalf("resourceProvisionOrder: %v", err)
	}
	if indexOf(order, "my-notifier") > indexOf(order, "my-backup") {
		t.Fatalf("order = %v, want my-notifier before my-backup (alphabetical order is the trap)", order)
	}
}

// fakeConsumerContext is a pApi.ComputeContext that reports exactly the
// env vars a test hands it — the merge function reads nothing else.
type fakeConsumerContext struct {
	plain  []pApi.ComputeEnvVariable
	secret []pApi.ComputeEnvVariable
}

func (f *fakeConsumerContext) EnvVariables() []pApi.ComputeEnvVariable       { return f.plain }
func (f *fakeConsumerContext) SecretEnvVariables() []pApi.ComputeEnvVariable { return f.secret }
func (f *fakeConsumerContext) Dependencies() []sdk.Resource                  { return nil }
func (f *fakeConsumerContext) Outputs() []sdk.Output                         { return nil }
func (f *fakeConsumerContext) ResolvePlaceholders(any) error                 { return nil }

func (f *fakeConsumerContext) GetPreProcessors(any) ([]pApi.PreProcessor, bool) { return nil, false }

func (f *fakeConsumerContext) GetPostProcessors(any) ([]pApi.PostProcessor, bool) {
	return nil, false
}

func (f *fakeConsumerContext) RunPreProcessors(any, any) error  { return nil }
func (f *fakeConsumerContext) RunPostProcessors(any, any) error { return nil }
