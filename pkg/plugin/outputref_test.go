// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// This test lives in package `plugin` rather than `plugin_test` (the convention in
// roundtrip_test.go) because it drives resolveStringOutput with an injected
// monitor. The engine's getResource reply shape is the thing under test, and
// pulumi's own WithMocks monitor does not speak it.
package plugin

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	pulumiplugin "github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

const testURN = "urn:pulumi:e2e::proj::sc:test:Key::host-key"

// fakeMonitor answers getResource and nothing else. Embedding the client
// interface satisfies it without stubbing 16 methods; anything this test does not
// expect to be called nil-panics, which is the correct outcome for a seam test.
type fakeMonitor struct {
	pulumirpc.ResourceMonitorClient

	// invoke sees every request, so a test can assert on what core sent.
	invoke func(*pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error)
}

func (f *fakeMonitor) Invoke(
	_ context.Context, in *pulumirpc.ResourceInvokeRequest, _ ...grpc.CallOption,
) (*pulumirpc.InvokeResponse, error) {
	return f.invoke(in)
}

// engineReply builds the reply the REAL engine sends: the {urn, id, provider,
// state} envelope, marshalled with exactly the options source_eval.go hardcodes
// for a getResource invoke (KeepUnknowns + KeepSecrets + KeepResources, and
// NOT KeepOutputValues).
func engineReply(t *testing.T, outputs resource.PropertyMap) *pulumirpc.InvokeResponse {
	t.Helper()
	ret, err := pulumiplugin.MarshalProperties(
		resource.PropertyMap{
			"urn":      resource.NewProperty(testURN),
			"id":       resource.NewProperty("some-cloud-id"),
			"provider": resource.NewProperty(""),
			"state":    resource.NewProperty(outputs),
		},
		pulumiplugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true, KeepResources: true},
	)
	Expect(err).ToNot(HaveOccurred())
	return &pulumirpc.InvokeResponse{Return: ret}
}

func monitorReturning(t *testing.T, outputs resource.PropertyMap) *fakeMonitor {
	t.Helper()
	return &fakeMonitor{
		invoke: func(in *pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error) {
			Expect(in.GetTok()).To(Equal(getResourceTok))
			args, err := pulumiplugin.UnmarshalProperties(
				in.GetArgs(), pulumiplugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(args["urn"].StringValue()).To(Equal(testURN))
			return engineReply(t, outputs), nil
		},
	}
}

func resolve(t *testing.T, outputs resource.PropertyMap, property string) (internals.UnsafeAwaitOutputResult, error) {
	t.Helper()
	out, err := resolveStringOutput(
		context.Background(), monitorReturning(t, outputs),
		OutputRef{URN: testURN, Property: property}, nil,
	)
	if err != nil {
		return internals.UnsafeAwaitOutputResult{}, err
	}
	return internals.UnsafeAwaitOutput(context.Background(), out)
}

// TestResolveStringOutput_PlainString is the baseline: a non-secret known
// property comes back as a resolved, non-secret StringOutput.
func TestResolveStringOutput_PlainString(t *testing.T) {
	RegisterTestingT(t)

	res, err := resolve(t, resource.PropertyMap{
		"publicIp": resource.NewProperty("10.0.0.7"),
	}, "publicIp")

	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeTrue())
	Expect(res.Secret).To(BeFalse())
	Expect(res.Value).To(Equal("10.0.0.7"))
}

// TestResolveStringOutput_SecretSurvivesTheRoundTrip answers design-doc §3.3
// question (a) — "does getResource preserve the secret flag?" — in a test rather
// than by reading the engine.
//
// It matters because the property this carries in Phase 1d is an SSH private key.
// If the flag were dropped, the key would be a plain output: printed in a preview
// diff, written unencrypted into the checkpoint, and archived in a CI log.
func TestResolveStringOutput_SecretSurvivesTheRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	res, err := resolve(t, resource.PropertyMap{
		"privateKeyOpenssh": resource.MakeSecret(resource.NewProperty("-----BEGIN OPENSSH PRIVATE KEY-----")),
	}, "privateKeyOpenssh")

	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeTrue())
	Expect(res.Secret).To(BeTrue(), "the secret flag must survive getResource — a private key rides this path")
	Expect(res.Value).To(Equal("-----BEGIN OPENSSH PRIVATE KEY-----"))
}

// TestResolveStringOutput_UnknownStaysUnknown pins the preview behaviour. An
// address that does not exist yet must NOT resolve to "" — core would preview a
// remote.Command against the empty string and report a diff that is not the one
// that will happen.
func TestResolveStringOutput_UnknownStaysUnknown(t *testing.T) {
	RegisterTestingT(t)

	res, err := resolve(t, resource.PropertyMap{
		"publicIp": resource.MakeComputed(resource.NewProperty("")),
	}, "publicIp")

	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeFalse())
	Expect(res.Value).To(BeNil())
}

// TestResolveStringOutput_UnknownSecretStaysBoth covers the shape a previewed
// generated key actually arrives in: the engine collapses an unknown secret output
// to Secret(Computed), and both bits have to survive.
func TestResolveStringOutput_UnknownSecretStaysBoth(t *testing.T) {
	RegisterTestingT(t)

	res, err := resolve(t, resource.PropertyMap{
		"privateKeyOpenssh": resource.MakeSecret(resource.MakeComputed(resource.NewProperty(""))),
	}, "privateKeyOpenssh")

	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeFalse())
	Expect(res.Secret).To(BeTrue())
}

// TestResolveStringOutput_OutputValueEnvelope covers flattenPropertyValue's Output
// branch. The engine does not currently send this (no KeepOutputValues on an
// invoke return) — the point is that if that marshalling option ever changes, the
// resolver does not start reporting known secrets as unknown plaintext.
func TestResolveStringOutput_OutputValueEnvelope(t *testing.T) {
	RegisterTestingT(t)

	outputs := resource.PropertyMap{
		"token": resource.NewOutputProperty(resource.Output{
			Element: resource.NewProperty("t0ken"),
			Known:   true,
			Secret:  true,
		}),
	}
	// Marshal WITH KeepOutputValues so the envelope reaches the resolver intact.
	ret, err := pulumiplugin.MarshalProperties(
		resource.PropertyMap{"state": resource.NewProperty(outputs)},
		pulumiplugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true, KeepOutputValues: true},
	)
	Expect(err).ToNot(HaveOccurred())

	monitor := &fakeMonitor{invoke: func(*pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error) {
		return &pulumirpc.InvokeResponse{Return: ret}, nil
	}}
	out, err := resolveStringOutput(
		context.Background(), monitor, OutputRef{URN: testURN, Property: "token"}, nil,
	)
	Expect(err).ToNot(HaveOccurred())

	res, err := internals.UnsafeAwaitOutput(context.Background(), out)
	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeTrue())
	Expect(res.Secret).To(BeTrue())
	Expect(res.Value).To(Equal("t0ken"))
}

// TestResolveStringOutput_NullIsEmptyNotMissing distinguishes the two cases that
// both look like "" to a careless implementation. A null property is a real answer
// — it is what privateKeyPassword is on every key SC generates — whereas a
// property that is not there at all is a plugin bug.
func TestResolveStringOutput_NullIsEmptyNotMissing(t *testing.T) {
	RegisterTestingT(t)

	res, err := resolve(t, resource.PropertyMap{
		"privateKeyPassword": resource.NewNullProperty(),
	}, "privateKeyPassword")

	Expect(err).ToNot(HaveOccurred())
	Expect(res.Known).To(BeTrue())
	Expect(res.Value).To(Equal(""))
}

// TestResolveStringOutput_MissingPropertyErrors is the typo guard. Silently
// resolving to "" here would configure SSH with no key and fail much later,
// somewhere else.
func TestResolveStringOutput_MissingPropertyErrors(t *testing.T) {
	RegisterTestingT(t)

	_, err := resolve(t, resource.PropertyMap{
		"privateKeyOpenssh": resource.NewProperty("key"),
	}, "privateKeyPem")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`no output property "privateKeyPem"`))
	// The message must name what IS there — a property-name typo across a plugin
	// boundary is otherwise a guessing game.
	Expect(err.Error()).To(ContainSubstring("privateKeyOpenssh"))
}

// TestResolveStringOutput_NonStringErrors keeps a structural mismatch loud. A
// HostFacts field is a string; a plugin pointing one at an object or a number is a
// contract violation, not something to coerce.
func TestResolveStringOutput_NonStringErrors(t *testing.T) {
	RegisterTestingT(t)

	_, err := resolve(t, resource.PropertyMap{
		"port": resource.NewProperty(22.0),
	}, "port")

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not a string"))
}

// TestResolveStringOutput_EmptyURNErrors — an unset OutputValue.ref is a plugin
// that forgot to fill the field, and must not reach the engine as a lookup for "".
func TestResolveStringOutput_EmptyURNErrors(t *testing.T) {
	RegisterTestingT(t)

	_, err := resolveStringOutput(
		context.Background(),
		&fakeMonitor{invoke: func(*pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error) {
			t.Error("must not invoke the monitor for an empty URN")
			return nil, nil
		}},
		OutputRef{Property: "publicIp"}, nil,
	)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("empty URN"))
}

// TestResolveStringOutput_UnknownResourceErrorNamesTheCause wraps the engine's own
// "unknown resource <urn>" — which on its own reads like a corrupt state file —
// with the explanation that is almost always the real one: the plugin returned a
// URN it never awaited.
func TestResolveStringOutput_UnknownResourceErrorNamesTheCause(t *testing.T) {
	RegisterTestingT(t)

	_, err := resolveStringOutput(
		context.Background(),
		&fakeMonitor{invoke: func(*pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error) {
			return nil, errors.New("unknown resource " + testURN)
		}},
		OutputRef{URN: testURN, Property: "publicIp"}, nil,
	)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("await"))
	Expect(err.Error()).To(ContainSubstring("unknown resource"))
}

// TestResolveStringOutput_CheckFailuresSurface — the invoke can succeed at the
// transport level and still report failures in the body.
func TestResolveStringOutput_CheckFailuresSurface(t *testing.T) {
	RegisterTestingT(t)

	_, err := resolveStringOutput(
		context.Background(),
		&fakeMonitor{invoke: func(*pulumirpc.ResourceInvokeRequest) (*pulumirpc.InvokeResponse, error) {
			return &pulumirpc.InvokeResponse{Failures: []*pulumirpc.CheckFailure{
				{Property: "urn", Reason: "malformed"},
			}}, nil
		}},
		OutputRef{URN: testURN, Property: "publicIp"}, nil,
	)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("urn: malformed"))
}

// TestStateFromGetResourceReturn_BothShapes pins the shape tolerance. The engine
// wraps outputs in a `state` object; pulumi's own WithMocks monitor returns the
// property map flat. Both have to work, or half the mechanism is untestable
// without a deploy.
func TestStateFromGetResourceReturn_BothShapes(t *testing.T) {
	RegisterTestingT(t)

	enveloped := resource.PropertyMap{
		"urn":   resource.NewProperty(testURN),
		"state": resource.NewProperty(resource.PropertyMap{"publicIp": resource.NewProperty("1.2.3.4")}),
	}
	state, err := stateFromGetResourceReturn(enveloped, testURN)
	Expect(err).ToNot(HaveOccurred())
	Expect(state["publicIp"].StringValue()).To(Equal("1.2.3.4"))

	flat := resource.PropertyMap{"publicIp": resource.NewProperty("1.2.3.4")}
	state, err = stateFromGetResourceReturn(flat, testURN)
	Expect(err).ToNot(HaveOccurred())
	Expect(state["publicIp"].StringValue()).To(Equal("1.2.3.4"))

	_, err = stateFromGetResourceReturn(
		resource.PropertyMap{"state": resource.NewProperty("not-an-object")}, testURN,
	)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not an object"))
}

// TestResolveStringOutput_ThroughRealContext is the end-to-end: it runs inside a
// real pulumi.Context (with pulumi's own mock monitor), registers a resource with
// a secret output the way a plugin would, and then resolves a property of it via
// the PUBLIC entry point.
//
// What this adds over the fake-monitor tests: it exercises monitorFromContext —
// the reflect+unsafe read of pulumi.Context.state.monitor that MonitorProxy also
// depends on. If a Pulumi upgrade renames that field, this test is the one that
// says so.
func TestResolveStringOutput_ThroughRealContext(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var key sdk.CustomResourceState
		if err := ctx.RegisterResource("sc:test:Key", "host-key", sdk.Map{
			"privateKeyOpenssh": sdk.ToSecret(sdk.String("PRIVATE")),
			"publicKeyOpenssh":  sdk.String("ssh-ed25519 AAAA"),
		}, &key); err != nil {
			return err
		}
		urn, err := internals.UnsafeAwaitOutput(ctx.Context(), key.URN())
		Expect(err).ToNot(HaveOccurred())
		urnStr := string(urn.Value.(sdk.URN))

		// The plain property, with the registered resource as a dependency.
		pub, err := ResolveStringOutput(ctx,
			OutputRef{URN: urnStr, Property: "publicKeyOpenssh"}, []sdk.Resource{&key})
		Expect(err).ToNot(HaveOccurred())
		pubRes, err := internals.UnsafeAwaitOutput(ctx.Context(), pub)
		Expect(err).ToNot(HaveOccurred())
		Expect(pubRes.Value).To(Equal("ssh-ed25519 AAAA"))

		// And the secret one. ToSecret at register time has to come back secret.
		priv, err := ResolveStringOutput(ctx,
			OutputRef{URN: urnStr, Property: "privateKeyOpenssh"}, nil)
		Expect(err).ToNot(HaveOccurred())
		privRes, err := internals.UnsafeAwaitOutput(ctx.Context(), priv)
		Expect(err).ToNot(HaveOccurred())
		Expect(privRes.Value).To(Equal("PRIVATE"))
		Expect(privRes.Secret).To(BeTrue())

		return nil
	}, sdk.WithMocks("proj", "e2e", outputRefMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

// TestRehydrateResource_ThroughRealContext proves the DependsOn half: a URN alone
// is enough to get an sdk.Resource handle back, with the type token and name read
// out of the URN so core never needs to know a plugin's resource types.
func TestRehydrateResource_ThroughRealContext(t *testing.T) {
	RegisterTestingT(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		var key sdk.CustomResourceState
		if err := ctx.RegisterResource("sc:test:Key", "host-key", nil, &key); err != nil {
			return err
		}
		urn, err := internals.UnsafeAwaitOutput(ctx.Context(), key.URN())
		Expect(err).ToNot(HaveOccurred())
		urnStr := string(urn.Value.(sdk.URN))

		rehydrated, err := RehydrateResource(ctx, urnStr)
		Expect(err).ToNot(HaveOccurred())
		got, err := internals.UnsafeAwaitOutput(ctx.Context(), rehydrated.URN())
		Expect(err).ToNot(HaveOccurred())
		Expect(string(got.Value.(sdk.URN))).To(Equal(urnStr),
			"a rehydrated handle must be byte-identical to the plugin's resource, or it is a new resource")

		_, err = RehydrateResource(ctx, "not-a-urn")
		Expect(err).To(HaveOccurred())
		return nil
	}, sdk.WithMocks("proj", "e2e", outputRefMocks{}))
	Expect(err).ToNot(HaveOccurred())
}

type outputRefMocks struct{}

func (outputRefMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	// Echo the inputs back as outputs, which is what a key resource does.
	return args.Name + "-id", args.Inputs, nil
}

func (outputRefMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}
