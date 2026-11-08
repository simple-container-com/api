// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	ycApi "github.com/simple-container-com/sc-plugin-yandex/api"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

type nestedSite struct {
	Domain        string `json:"domain" yaml:"domain"`
	IndexDocument string `json:"indexDocument" yaml:"indexDocument"`
}

type nestedHost struct {
	Zone string `json:"zone" yaml:"zone"`
}

type nestedInput struct {
	FolderID string       `json:"folderId" yaml:"folderId"`
	Public   bool         `json:"public" yaml:"public"`
	DiskSize int          `json:"diskSize" yaml:"diskSize"`
	Site     nestedSite   `json:"site" yaml:"site"`
	Hosts    []nestedHost `json:"hosts" yaml:"hosts"`
	Tags     []string     `json:"tags" yaml:"tags"`
	Missing  *nestedSite  `json:"missing" yaml:"missing"`
}

// decodeLikePlugin is what a plugin does with ProvisionRequest.inputs:
// InputsToPlain → YAML → its own config struct.
func decodeLikePlugin[T any](t *testing.T, inputs map[string]*scpluginv1.InputValue, out *T) {
	t.Helper()
	b, err := yaml.Marshal(scPlugin.InputsToPlain(inputs))
	Expect(err).ToNot(HaveOccurred())
	Expect(yaml.Unmarshal(b, out)).To(Succeed())
}

// Before nested values travelled, a plugin received only the top-level scalars
// of its config; nested blocks and lists vanished without an error.
func TestSerializeConfigToInputs_NestedRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	in := nestedInput{
		FolderID: "b1g",
		Public:   true,
		DiskSize: 10,
		Site:     nestedSite{Domain: "a.example.ru", IndexDocument: "index.html"},
		Hosts:    []nestedHost{{Zone: "ru-central1-a"}, {Zone: "ru-central1-b"}},
		Tags:     []string{"x", "y"},
	}
	inputs := serializeConfigToInputs(&in)
	Expect(inputs).To(HaveKey("site"))
	Expect(inputs).To(HaveKey("hosts"))
	Expect(inputs).NotTo(HaveKey("missing"), "a JSON null is omitted, not sent as an empty value")

	var out nestedInput
	decodeLikePlugin(t, inputs, &out)
	Expect(out).To(Equal(in))
}

func TestSerializeConfigToInputs_NilAndScalars(t *testing.T) {
	RegisterTestingT(t)

	Expect(serializeConfigToInputs(nil)).To(BeEmpty())
	inputs := serializeConfigToInputs(map[string]any{"s": "v", "b": false, "n": 2})
	Expect(inputs["s"].GetStringValue()).To(Equal("v"))
	Expect(inputs["b"].GetBoolValue()).To(BeFalse())
	Expect(inputs["n"].GetDoubleValue()).To(Equal(2.0))
}

func TestSerializeConfigToInputs_ListKeepsNullPositions(t *testing.T) {
	RegisterTestingT(t)

	inputs := serializeConfigToInputs(map[string]any{"l": []any{"a", nil, "c"}})
	plain := scPlugin.InputsToPlain(inputs)
	Expect(plain["l"]).To(Equal([]any{"a", nil, "c"}))
}

// The yc-serverless-container deploy input is the template's account config plus
// the client stack's whole stackConfig. The plugin used to receive only
// folderId-level scalars — no image, env, secrets, uses, domain or cloudExtras.
func TestSerializeConfigToInputs_ServerlessContainerInputRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	var extras any = map[string]any{"serviceAccountId": "aje-sa", "public": true}
	in := ycApi.ServerlessContainerInput{
		AccountConfig: ycApi.AccountConfig{
			FolderID: "b1g-folder",
		},
		RegistryID:       "crp-registry",
		ServiceAccountID: "aje-sa",
		StackConfig: api.StackConfigSingleImage{
			Image: &api.ContainerImage{
				Dockerfile: "Dockerfile",
				Context:    ".",
				Build:      &api.ContainerImageBuild{Args: map[string]string{"FOO": "bar"}},
			},
			Domain:      "app.example.ru",
			Env:         map[string]string{"LOG_LEVEL": "debug"},
			Secrets:     map[string]string{"API_KEY": "${secret:api-key}"},
			Timeout:     lo.ToPtr(30),
			MaxMemory:   lo.ToPtr(512),
			Uses:        []string{"mongodb", "bucket"},
			CloudExtras: &extras,
		},
	}

	inputs := serializeConfigToInputs(&in)
	Expect(inputs).To(HaveKey("stackConfig"), "stackConfig must cross the bus as a nested map")
	Expect(inputs["stackConfig"].GetMapValue()).ToNot(BeNil())

	var out ycApi.ServerlessContainerInput
	decodeLikePlugin(t, inputs, &out)

	Expect(out.FolderID).To(Equal("b1g-folder"))
	Expect(out.RegistryID).To(Equal("crp-registry"))
	Expect(out.ServiceAccountID).To(Equal("aje-sa"))
	sc := out.StackConfig
	Expect(sc.Image).ToNot(BeNil())
	Expect(sc.Image.Dockerfile).To(Equal("Dockerfile"))
	Expect(sc.Image.Context).To(Equal("."))
	Expect(sc.Image.Build).ToNot(BeNil())
	Expect(sc.Image.Build.Args).To(HaveKeyWithValue("FOO", "bar"))
	Expect(sc.Domain).To(Equal("app.example.ru"))
	Expect(sc.Env).To(HaveKeyWithValue("LOG_LEVEL", "debug"))
	Expect(sc.Secrets).To(HaveKeyWithValue("API_KEY", "${secret:api-key}"))
	Expect(sc.Timeout).To(Equal(lo.ToPtr(30)))
	Expect(sc.MaxMemory).To(Equal(lo.ToPtr(512)))
	Expect(sc.Uses).To(Equal([]string{"mongodb", "bucket"}))
	Expect(sc.CloudExtras).ToNot(BeNil())
	Expect(*sc.CloudExtras).To(Equal(map[string]any{"serviceAccountId": "aje-sa", "public": true}))

	ce, err := out.CloudExtras()
	Expect(err).ToNot(HaveOccurred())
	Expect(ce).ToNot(BeNil())
}

type stubProvisionClient struct {
	scpluginv1.SCCloudPluginClient
	gotReq *scpluginv1.ProvisionRequest
}

func (s *stubProvisionClient) Provision(_ context.Context, req *scpluginv1.ProvisionRequest, _ ...grpc.CallOption) (*scpluginv1.ProvisionResponse, error) {
	s.gotReq = req
	return &scpluginv1.ProvisionResponse{Urn: "urn:pulumi:test::proj::yandex:index/serverlessContainer:ServerlessContainer::app"}, nil
}

type fixedComputeContext struct {
	pApi.ComputeContext
	plain, secret []pApi.ComputeEnvVariable
}

func (f *fixedComputeContext) EnvVariables() []pApi.ComputeEnvVariable       { return f.plain }
func (f *fixedComputeContext) SecretEnvVariables() []pApi.ComputeEnvVariable { return f.secret }

// A compute template's env comes from the consuming stack's `uses:` resources
// via params.ComputeContext. The resource sidecar must forward it — before, it
// ignored ProvisionParams and the plugin saw no MONGO_* at all.
func TestResourceSidecar_ForwardsConfigAndComputeEnv(t *testing.T) {
	RegisterTestingT(t)
	withActiveSession(t)

	stub := &stubProvisionClient{}
	sidecar := (&preEnginePluginBundle{}).makeResourceSidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-yandex", ycApi.TemplateTypeYandexServerlessContainer,
	)
	cfg := &ycApi.ServerlessContainerInput{
		AccountConfig: ycApi.AccountConfig{FolderID: "b1g"},
		StackConfig: api.StackConfigSingleImage{
			Image: &api.ContainerImage{Dockerfile: "Dockerfile"},
			Uses:  []string{"mongodb"},
		},
	}
	cc := &fixedComputeContext{
		plain:  []pApi.ComputeEnvVariable{{Name: "MONGO_DB", Value: "app", ResourceName: "mongodb", ResourceType: "mongodb-atlas", StackName: "backend"}},
		secret: []pApi.ComputeEnvVariable{{Name: "MONGO_URI", Value: "mongo-uri-placeholder", ResourceName: "mongodb", ResourceType: "mongodb-atlas", StackName: "backend"}},
	}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := sidecar(ctx, api.Stack{Name: "backend"}, api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Type: ycApi.TemplateTypeYandexServerlessContainer, Name: "backend", Config: api.Config{Config: cfg}},
			StackParams: &api.StackParams{StackName: "backend", Environment: "staging-ru"},
		}, pApi.ProvisionParams{ComputeContext: cc})
		return err
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).ToNot(HaveOccurred())

	Expect(stub.gotReq).ToNot(BeNil())
	Expect(stub.gotReq.GetInputs()).To(HaveKey("stackConfig"))
	env := stub.gotReq.GetComputeEnvVars()
	Expect(env).To(HaveLen(2))
	Expect(env[0].GetName()).To(Equal("MONGO_DB"))
	Expect(env[0].GetSecret()).To(BeFalse())
	Expect(env[1].GetName()).To(Equal("MONGO_URI"))
	Expect(env[1].GetSecret()).To(BeTrue(), "a var from SecretEnvVariables is secret even when its entry's flag is unset")
	Expect(env[1].GetResourceType()).To(Equal("mongodb-atlas"))
	Expect(env[1].GetStackName()).To(Equal("backend"))
}

// TestResourceSidecar_PacksDependencyOutputs — a sibling resource provisioned
// earlier in the same stack publishes outputs onto its `api.ResourceOutput.Ref`
// as `*api.PluginResourceOutput` (post-#569). When THIS resource's sidecar
// fires with `ProvisionParams.ResourceOutputs` carrying such Refs, the
// `DependencyOutputsJson` field on the outbound `ProvisionRequest` must be
// populated — `runtime.DependencyOutputsFromProvisionRequest` decodes it on
// the plugin side and the body reads `params.ResourceOutputs[dep].Ref` as
// `*api.PluginResourceOutput`, which is exactly the shape in-tree bodies
// already type-assert on.
//
// Live-caught 2026-11-08: `gcp-docker-image-push` read
// `params.ResourceOutputs[registryResource].Ref` and errored with
// `expected registry resource %q to be present in outputs for %q in %q`
// because this sidecar previously left `DependencyOutputsJson` empty. See
// forge/docs/roadmap/items/sc-plugin-dependency-outputs-not-populated.md.
func TestResourceSidecar_PacksDependencyOutputs(t *testing.T) {
	RegisterTestingT(t)
	withActiveSession(t)

	stub := &stubProvisionClient{}
	sidecar := (&preEnginePluginBundle{}).makeResourceSidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-yandex", ycApi.TemplateTypeYandexServerlessContainer,
	)
	cfg := &ycApi.ServerlessContainerInput{
		AccountConfig: ycApi.AccountConfig{FolderID: "b1g"},
		StackConfig: api.StackConfigSingleImage{
			Image: &api.ContainerImage{Dockerfile: "Dockerfile"},
		},
	}
	// Mix: one plugin-owned sibling (packable), one in-tree Ref (string — NOT
	// packable, silently dropped since the plugin can't unmarshal a Go struct).
	resourceOutputs := map[string]*api.ResourceOutput{
		"my-registry": {Ref: &api.PluginResourceOutput{
			PluginName: "yc",
			Urn:        "urn:pulumi:parent::stack::yandex:index:ContainerRegistry::my-registry",
			Outputs: map[string]api.PluginOutputValue{
				"url": {Value: "cr.yandex/my-registry", Secret: false},
			},
		}},
		"legacy-intree": {Ref: "plain-string-ref"},
	}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := sidecar(ctx, api.Stack{Name: "backend"}, api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Type: ycApi.TemplateTypeYandexServerlessContainer, Name: "backend", Config: api.Config{Config: cfg}},
			StackParams: &api.StackParams{StackName: "backend", Environment: "staging-ru"},
		}, pApi.ProvisionParams{ResourceOutputs: resourceOutputs})
		return err
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).ToNot(HaveOccurred())

	Expect(stub.gotReq).ToNot(BeNil())
	raw := stub.gotReq.GetDependencyOutputsJson()
	Expect(raw).ToNot(BeEmpty(), "sibling-published outputs must cross the bus as DependencyOutputsJson")

	// Decode and verify shape — same path the plugin side uses.
	var decoded map[string]*api.PluginResourceOutput
	Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
	Expect(decoded).To(HaveKey("my-registry"))
	Expect(decoded).ToNot(HaveKey("legacy-intree"), "in-tree Refs are dropped — plugin can't unmarshal unknown Go struct shapes")
	Expect(decoded["my-registry"].PluginName).To(Equal("yc"))
	Expect(decoded["my-registry"].Outputs["url"].Value).To(Equal("cr.yandex/my-registry"))
}

// TestResourceSidecar_EmptyResourceOutputs — the common case: a resource with
// no sibling deps. DependencyOutputsJson stays empty (older-plugin-compatible),
// and the plugin side's `params.ResourceOutputs` is nil — matching the
// pre-field behaviour exactly.
func TestResourceSidecar_EmptyResourceOutputs(t *testing.T) {
	RegisterTestingT(t)
	withActiveSession(t)

	stub := &stubProvisionClient{}
	sidecar := (&preEnginePluginBundle{}).makeResourceSidecar(
		scPlugin.NewClientWithStub(stub), "sc-plugin-yandex", ycApi.TemplateTypeYandexServerlessContainer,
	)
	cfg := &ycApi.ServerlessContainerInput{
		AccountConfig: ycApi.AccountConfig{FolderID: "b1g"},
		StackConfig: api.StackConfigSingleImage{
			Image: &api.ContainerImage{Dockerfile: "Dockerfile"},
		},
	}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := sidecar(ctx, api.Stack{Name: "backend"}, api.ResourceInput{
			Descriptor:  &api.ResourceDescriptor{Type: ycApi.TemplateTypeYandexServerlessContainer, Name: "backend", Config: api.Config{Config: cfg}},
			StackParams: &api.StackParams{StackName: "backend", Environment: "staging-ru"},
		}, pApi.ProvisionParams{})
		return err
	}, sdk.WithMocks("project", "stack", &refMocks{}))
	Expect(err).ToNot(HaveOccurred())
	Expect(stub.gotReq.GetDependencyOutputsJson()).To(BeEmpty())
}

func TestComputeContextToProto_Nil(t *testing.T) {
	RegisterTestingT(t)
	Expect(computeContextToProto(nil)).To(BeNil())
	Expect(computeContextToProto(&fixedComputeContext{})).To(BeNil())
}
