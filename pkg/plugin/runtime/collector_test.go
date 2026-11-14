// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestNewProvisionCollector_SeedsIncomingPools confirms trap 14 (the
// incoming/outgoing split): compute_env_vars from the request end up in
// EnvVariables() / SecretEnvVariables(), bucketed by Secret flag.
func TestNewProvisionCollector_SeedsIncomingPools(t *testing.T) {
	t.Parallel()
	req := &scpluginv1.ProvisionRequest{
		ComputeEnvVars: []*scpluginv1.ComputeEnvVariable{
			{Name: "MONGO_URI", Value: "mongodb://...", Secret: false, ResourceType: "mongodb", ResourceName: "db", StackName: "parent"},
			{Name: "MONGO_PASSWORD", Value: "secret", Secret: true, ResourceType: "mongodb", ResourceName: "db", StackName: "parent"},
			{Name: "", Value: "wire-artefact-drops"},
			nil,
		},
	}
	c := NewProvisionCollector(req)

	plain := c.EnvVariables()
	if len(plain) != 1 || plain[0].Name != "MONGO_URI" {
		t.Fatalf("EnvVariables = %+v, want [MONGO_URI]", plain)
	}
	secrets := c.SecretEnvVariables()
	if len(secrets) != 1 || secrets[0].Name != "MONGO_PASSWORD" || !secrets[0].Secret {
		t.Fatalf("SecretEnvVariables = %+v, want [MONGO_PASSWORD secret=true]", secrets)
	}
}

// TestNewProvisionCollector_NilRequest handles the compute-only call site
// (Provision for a static template with no uses:) — nil request is valid.
func TestNewProvisionCollector_NilRequest(t *testing.T) {
	t.Parallel()
	c := NewProvisionCollector(nil)
	if len(c.EnvVariables()) != 0 || len(c.SecretEnvVariables()) != 0 {
		t.Fatal("nil request should leave both pools empty")
	}
}

// TestRunComputeProcess_WithIncomingEnvVars_SeedsIncomingPools covers
// slice 1d.7 auto-discover: SC core packs the shared collector's env
// vars into ComputeProcessRequest.compute_env_vars; a plugin that opts
// into `WithIncomingEnvVars(req)` sees them on `EnvVariables()` /
// `SecretEnvVariables()` on the collector its handler receives.
//
// Mirrors TestNewProvisionCollector_SeedsIncomingPools — same wire
// shape, same wire-artefact filtering, different proto message.
func TestRunComputeProcess_WithIncomingEnvVars_SeedsIncomingPools(t *testing.T) {
	t.Parallel()
	req := &scpluginv1.ComputeProcessRequest{
		ComputeEnvVars: []*scpluginv1.ComputeEnvVariable{
			{Name: "MONGO_URI", Value: "mongodb+srv://u:p@h/", Secret: true, ResourceType: "mongodb", ResourceName: "the-mongo", StackName: "parent"},
			{Name: "MONGO_DATABASE", Value: "forge_app", Secret: false, ResourceType: "mongodb", ResourceName: "the-mongo", StackName: "parent"},
			{Name: "", Value: "wire-artefact-drops"},
			nil,
		},
	}

	rctx := &Context{Pulumi: &sdk.Context{}}
	var got pApi.ComputeContextCollector
	_, _, err := RunComputeProcess(
		context.Background(), rctx,
		api.Stack{Name: "consumer"},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "sc-backup", Name: "my-backup"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			got = collector
			return &api.ResourceOutput{Ref: "ok"}, nil
		},
		WithIncomingEnvVars(req),
	)
	if err != nil {
		t.Fatalf("RunComputeProcess: %v", err)
	}
	plain := got.EnvVariables()
	if len(plain) != 1 || plain[0].Name != "MONGO_DATABASE" {
		t.Fatalf("EnvVariables = %+v, want [MONGO_DATABASE]", plain)
	}
	secrets := got.SecretEnvVariables()
	if len(secrets) != 1 || secrets[0].Name != "MONGO_URI" || !secrets[0].Secret {
		t.Fatalf("SecretEnvVariables = %+v, want [MONGO_URI secret=true]", secrets)
	}
}

// TestRunComputeProcess_WithIncomingEnvVars_NilRequest — defensive:
// passing a nil request through the option keeps the empty-pool
// default rather than panicking.
func TestRunComputeProcess_WithIncomingEnvVars_NilRequest(t *testing.T) {
	t.Parallel()
	rctx := &Context{Pulumi: &sdk.Context{}}
	var got pApi.ComputeContextCollector
	_, _, err := RunComputeProcess(
		context.Background(), rctx,
		api.Stack{Name: "consumer"},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "sc-backup", Name: "my-backup"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			got = collector
			return &api.ResourceOutput{Ref: "ok"}, nil
		},
		WithIncomingEnvVars(nil),
	)
	if err != nil {
		t.Fatalf("RunComputeProcess with nil req: %v", err)
	}
	if n := len(got.EnvVariables()) + len(got.SecretEnvVariables()); n != 0 {
		t.Fatalf("nil-req option should leave pools empty, got %d entries", n)
	}
}

// TestRunComputeProcess_DefaultEmptyIncomingPool pins the pre-1d.7
// default: a plugin that does NOT pass WithIncomingEnvVars gets the
// empty incoming pool. Keeps today's plugins (aws, mongodb, yandex,
// gcp, cloudru) on their verified behaviour.
func TestRunComputeProcess_DefaultEmptyIncomingPool(t *testing.T) {
	t.Parallel()
	rctx := &Context{Pulumi: &sdk.Context{}}
	var got pApi.ComputeContextCollector
	_, _, err := RunComputeProcess(
		context.Background(), rctx,
		api.Stack{Name: "consumer"},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "aws-s3", Name: "forge-storage"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			got = collector
			return &api.ResourceOutput{Ref: "ok"}, nil
		},
		// No WithIncomingEnvVars — the common case.
	)
	if err != nil {
		t.Fatalf("RunComputeProcess: %v", err)
	}
	if n := len(got.EnvVariables()) + len(got.SecretEnvVariables()); n != 0 {
		t.Fatalf("default should leave incoming pools empty, got %d entries", n)
	}
}

// TestRunComputeProcess_SyncMutations covers the happy path: the handler
// adds env/tpl mutations synchronously (no ApplyT) and they flow through
// to the response unchanged. Also asserts the response has NO incoming
// env (ComputeProcess doesn't echo it back).
func TestRunComputeProcess_SyncMutations(t *testing.T) {
	t.Parallel()
	rctx := &Context{Pulumi: &sdk.Context{}}

	resp, out, err := RunComputeProcess(
		context.Background(), rctx,
		api.Stack{Name: "consumer-stack"},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "yc-bucket", Name: "forge-storage"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			collector.AddEnvVariableIfNotExist("S3_BUCKET", "my-bucket", "yc-bucket", "forge-storage", "parent")
			collector.AddEnvVariableIfNotExist("S3_REGION", "ru-central1", "yc-bucket", "forge-storage", "parent")
			collector.AddSecretEnvVariableIfNotExist("S3_SECRET_KEY", "redacted", "yc-bucket", "forge-storage", "parent")
			collector.AddResourceTplExtension("forge-storage", map[string]string{"bucket": "my-bucket"})
			collector.AddDependencyTplExtension("consumer", "forge-storage", map[string]string{"domain": "example.com"})
			return &api.ResourceOutput{Ref: "sentinel"}, nil
		},
	)
	if err != nil {
		t.Fatalf("RunComputeProcess: %v", err)
	}
	if out == nil || out.Ref != "sentinel" {
		t.Fatalf("ResourceOutput.Ref = %v, want sentinel", out)
	}
	if len(resp.EnvVars) != 3 {
		t.Fatalf("EnvVars = %d, want 3", len(resp.EnvVars))
	}
	if len(resp.TplExtensions) != 2 {
		t.Fatalf("TplExtensions = %d, want 2", len(resp.TplExtensions))
	}
	// Secret flag survives marshal.
	for _, v := range resp.EnvVars {
		if v.Name == "S3_SECRET_KEY" && !v.Secret {
			t.Fatal("S3_SECRET_KEY lost its Secret flag")
		}
	}
}

// TestRunComputeProcess_DrainsCapturedOutputs is the trap-16 regression:
// a handler that emits every env mutation from INSIDE an ApplyT must
// still see those mutations in the response. The old aws collector had
// AddOutput as a no-op, so the ApplyT body was discarded and ToResponse
// returned an empty EnvVars.
//
// # Determinism
//
// `sdk.All(...).ApplyT(fn)` does NOT run `fn` synchronously, even when
// every input is already resolved — the SDK dispatches the callback on
// a goroutine (pulumi/sdk/v3@v3.245.0/go/internal/types.go awaitInputs
// → go func). So an ApplyT that completes in <1µs can race `toResponse`
// and, with `AddOutput` reverted to the old no-op, let the test pass
// nondeterministically against the exact bug it pins. Under
// `-count=100` it would flake rather than fail.
//
// The 50ms delay inside the callback forces the race the right way: if
// RunComputeProcess ever reverts to calling toResponse without the
// drain, toResponse sees 0 env vars (deterministically) and the test
// fails. With the drain in place, UnsafeAwaitOutput blocks through the
// sleep and all 5 vars land.
//
// RED-GREEN VERIFIED: commenting out the `awaitOutputs` call in
// RunComputeProcess makes this test fail with
// `expected 5 env vars after drain, got 0`.
func TestRunComputeProcess_DrainsCapturedOutputs(t *testing.T) {
	t.Parallel()

	// A pulumi.Context under mocks is enough to make ApplyT run; the
	// handler never registers resources, only chains an ApplyT on a
	// pre-resolved Output.
	err := sdk.RunErr(func(pctx *sdk.Context) error {
		rctx := &Context{Pulumi: pctx}
		resp, _, err := RunComputeProcess(
			context.Background(), rctx,
			api.Stack{Name: "consumer"},
			api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "yc-bucket", Name: "forge-storage"}},
			pApi.ProvisionParams{},
			func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
				// This is the shape every ObjectStorageBucketComputeProcessor /
				// RdsPostgresComputeProcessor uses in production: an ApplyT
				// callback emits S3_* env vars after the parent StackReference
				// resolves. The old aws collector's AddOutput was a no-op, so
				// everything added here was dropped. The sleep forces the race
				// — without the drain, toResponse wins and sees 0.
				out := sdk.All(sdk.String("my-bucket").ToStringOutput()).ApplyT(func(args []any) any {
					time.Sleep(50 * time.Millisecond)
					bucket := args[0].(string)
					collector.AddEnvVariableIfNotExist("S3_BUCKET", bucket, "yc-bucket", "forge-storage", "parent")
					collector.AddEnvVariableIfNotExist("S3_REGION", "ru-central1", "yc-bucket", "forge-storage", "parent")
					// yandex's ObjectStorageBucketComputeProcessor emits a 5th —
					// S3_ENDPOINT — on top of aws's four; the outgoing pool
					// is a plain slice so a plugin-specific extra just appends.
					collector.AddEnvVariableIfNotExist("S3_ENDPOINT", "https://storage.yandexcloud.net", "yc-bucket", "forge-storage", "parent")
					collector.AddEnvVariableIfNotExist("S3_ACCESS_KEY", "AKI...", "yc-bucket", "forge-storage", "parent")
					collector.AddSecretEnvVariableIfNotExist("S3_SECRET_KEY", "redacted", "yc-bucket", "forge-storage", "parent")
					return nil
				})
				collector.AddOutput(pctx, out)
				return &api.ResourceOutput{Ref: "sentinel"}, nil
			},
		)
		if err != nil {
			return err
		}
		if n := len(resp.EnvVars); n != 5 {
			return errors.Errorf("expected 5 env vars after drain, got %d — AddOutput+drain pair regressed", n)
		}
		names := map[string]bool{}
		for _, v := range resp.EnvVars {
			names[v.Name] = true
		}
		for _, want := range []string{"S3_BUCKET", "S3_REGION", "S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY"} {
			if !names[want] {
				return errors.Errorf("missing env var %q from response", want)
			}
		}
		return nil
	}, sdk.WithMocks("test-project", "test-stack", &collectorTestMocks{}))
	if err != nil {
		t.Fatalf("RunComputeProcess with ApplyT body: %v", err)
	}
}

// TestRunComputeProcess_PropagatesApplyTError — if an ApplyT callback
// resolves to an error (e.g. a StackReference read that fails), the
// drain surfaces it instead of swallowing. Converting silent
// `emitted 0` into a visible failure is the whole point of the file;
// a future "simplification" to log-and-continue would break this.
func TestRunComputeProcess_PropagatesApplyTError(t *testing.T) {
	t.Parallel()
	err := sdk.RunErr(func(pctx *sdk.Context) error {
		rctx := &Context{Pulumi: pctx}
		_, _, err := RunComputeProcess(
			context.Background(), rctx,
			api.Stack{Name: "consumer"},
			api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "yc-bucket", Name: "forge-storage"}},
			pApi.ProvisionParams{},
			func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
				// `(any, error)` ApplyT signature — error from the callback
				// surfaces through UnsafeAwaitOutput into awaitOutputs.
				out := sdk.All(sdk.String("x").ToStringOutput()).ApplyT(func(_ []any) (any, error) {
					return nil, errors.New("stack reference read failed")
				})
				collector.AddOutput(pctx, out)
				return &api.ResourceOutput{Ref: "sentinel"}, nil
			},
		)
		if err == nil {
			return errors.New("expected error from ApplyT callback, got nil")
		}
		if !strings.Contains(err.Error(), "stack reference read failed") {
			return errors.Errorf("expected wrapped callback error, got %v", err)
		}
		if !strings.Contains(err.Error(), "await compute-context output") {
			return errors.Errorf("expected await wrap, got %v", err)
		}
		return nil
	}, sdk.WithMocks("test-project", "test-stack", &collectorTestMocks{}))
	if err != nil {
		t.Fatalf("ApplyT error case: %v", err)
	}
}

// TestRunComputeProcess_DrainsNestedAddOutput — an ApplyT callback that
// itself calls AddOutput appends after the first drain pass. The
// outer-loop drain-until-stable in awaitOutputs catches it; a single
// snapshot would reintroduce trap 16 one level down. Not a shape any
// current processor uses, but a plausible one for two sequential
// StackReference reads where the second needs a value from the first.
func TestRunComputeProcess_DrainsNestedAddOutput(t *testing.T) {
	t.Parallel()
	err := sdk.RunErr(func(pctx *sdk.Context) error {
		rctx := &Context{Pulumi: pctx}
		resp, _, err := RunComputeProcess(
			context.Background(), rctx,
			api.Stack{Name: "consumer"},
			api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "yc-bucket", Name: "forge-storage"}},
			pApi.ProvisionParams{},
			func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
				outer := sdk.All(sdk.String("outer").ToStringOutput()).ApplyT(func(_ []any) any {
					time.Sleep(20 * time.Millisecond)
					// Inside the outer ApplyT, chain a second one and
					// register it with the SAME collector. The second
					// callback writes the actual env var — a single-pass
					// drain would never await it.
					inner := sdk.All(sdk.String("inner").ToStringOutput()).ApplyT(func(_ []any) any {
						time.Sleep(20 * time.Millisecond)
						collector.AddEnvVariableIfNotExist("NESTED_VAR", "from-inner", "yc-bucket", "forge-storage", "parent")
						return nil
					})
					collector.AddOutput(pctx, inner)
					return nil
				})
				collector.AddOutput(pctx, outer)
				return &api.ResourceOutput{Ref: "sentinel"}, nil
			},
		)
		if err != nil {
			return err
		}
		if len(resp.EnvVars) != 1 || resp.EnvVars[0].Name != "NESTED_VAR" {
			return errors.Errorf("nested AddOutput env lost — got %d entries, drain-until-stable regressed", len(resp.EnvVars))
		}
		return nil
	}, sdk.WithMocks("test-project", "test-stack", &collectorTestMocks{}))
	if err != nil {
		t.Fatalf("nested AddOutput case: %v", err)
	}
}

// TestRunComputeProcess_PropagatesHandlerError — a handler that returns
// an error must short-circuit before the drain, so a half-populated
// collector doesn't produce a misleading partial response.
func TestRunComputeProcess_PropagatesHandlerError(t *testing.T) {
	t.Parallel()
	rctx := &Context{Pulumi: &sdk.Context{}}
	resp, out, err := RunComputeProcess(
		context.Background(), rctx,
		api.Stack{Name: "consumer"},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "yc-bucket", Name: "forge-storage"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, collector pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			collector.AddEnvVariableIfNotExist("A", "1", "", "", "")
			return nil, errors.New("boom")
		},
	)
	// strings.Contains, not ==: a future `errors.Wrap(err, "compute handler")`
	// shouldn't fail this test for the wrong reason.
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected handler error containing \"boom\", got err=%v", err)
	}
	if resp != nil || out != nil {
		t.Fatal("response/output must be nil when handler errors")
	}
}

// TestRunComputeProcess_NilContextRejected — defensive guard against
// callers that construct a RunComputeProcess call without first opening
// a runtime.NewContext.
func TestRunComputeProcess_NilContextRejected(t *testing.T) {
	t.Parallel()
	_, _, err := RunComputeProcess(
		context.Background(), nil,
		api.Stack{}, api.ResourceInput{}, pApi.ProvisionParams{},
		func(*sdk.Context, api.Stack, api.ResourceInput, pApi.ComputeContextCollector, pApi.ProvisionParams) (*api.ResourceOutput, error) {
			t.Fatal("handler should not have been called")
			return nil, nil
		},
	)
	if err == nil {
		t.Fatal("expected nil-Context error, got nil")
	}

	rctx := &Context{Pulumi: &sdk.Context{}}
	_, _, err = RunComputeProcess(context.Background(), rctx, api.Stack{}, api.ResourceInput{}, pApi.ProvisionParams{}, nil)
	if err == nil {
		t.Fatal("expected nil-handler error, got nil")
	}
}

// collectorTestMocks is the smallest pulumi.MockResourceMonitor that lets
// sdk.RunErr open a context — no resources are registered in these tests,
// but RunErr still needs the hook.
type collectorTestMocks struct{}

func (collectorTestMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "-id", args.Inputs, nil
}

func (collectorTestMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}
