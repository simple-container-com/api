// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// containerArgs stands in for kubernetes.SimpleContainerArgs: the type a
// template runs its pre-processors on.
type containerArgs struct{ sidecars []string }

type otherArgs struct{}

// fakeCtx is a Context whose teardown is counted instead of talking to an engine.
func fakeCtx(key string, drained *int, err error) *Context {
	return &Context{Pulumi: &sdk.Context{}, key: key, done: func() error { *drained++; return err }}
}

func provisionReq(stack string) *scpluginv1.ProvisionRequest {
	return &scpluginv1.ProvisionRequest{Header: &scpluginv1.RequestHeader{StackName: stack}}
}

func settle(rctx *Context, handlerErr error) error {
	var resp *struct{}
	err := handlerErr
	Settle(rctx, &resp, &err)
	return err
}

// A preview of a deployed fleet planned the whole Cloud SQL proxy apparatus
// (service accounts, keys, the credentials Secrets, the db-user-init Job) for
// deletion on every stack that uses Postgres. The proxy is attached by a
// closure the dependency's compute processor registers and the template runs on
// its own container args; with the dependency and the template in different
// RPCs the closure was dropped and the app deployed with no database proxy.
func TestPreProcessors_ParkedByComputeProcessAndRunByTheTemplate(t *testing.T) {
	const stack = "pp-support-bot--production"
	computeDrained, provisionDrained := 0, 0

	compute := fakeCtx(stack, &computeDrained, nil)
	_, _, err := RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Type: "gcp-cloudsql-postgres", Name: "postgres"}},
		pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			c.AddPreProcessor(&containerArgs{}, func(arg any) error {
				a := arg.(*containerArgs)
				a.sidecars = append(a.sidecars, "cloudsql-proxy")
				return nil
			})
			return &api.ResourceOutput{}, nil
		})
	if err != nil {
		t.Fatalf("RunComputeProcess: %v", err)
	}

	// The handler returns; its context must stay open for the closure.
	if err := settle(compute, nil); err != nil {
		t.Fatalf("settle compute: %v", err)
	}
	if computeDrained != 0 {
		t.Fatalf("the ComputeProcess context was drained and closed (%d) before its closure could run", computeDrained)
	}

	// The template's Provision builds its collector, runs the closure on its
	// own args, and settles.
	consumer := NewProvisionCollector(provisionReq(stack))
	args := &containerArgs{}
	if err := consumer.RunPreProcessors(args, args); err != nil {
		t.Fatalf("RunPreProcessors: %v", err)
	}
	if !reflect.DeepEqual(args.sidecars, []string{"cloudsql-proxy"}) {
		t.Fatalf("sidecars = %v, want the proxy the dependency parked", args.sidecars)
	}
	provision := fakeCtx(stack, &provisionDrained, nil)
	if err := settle(provision, nil); err != nil {
		t.Fatalf("settle provision: %v", err)
	}
	if provisionDrained != 1 || computeDrained != 1 {
		t.Fatalf("drained provision=%d compute=%d, want both exactly once", provisionDrained, computeDrained)
	}

	// Taken, not copied: a second template run does not attach the proxy twice.
	again := &containerArgs{}
	if err := NewProvisionCollector(provisionReq(stack)).RunPreProcessors(again, again); err != nil {
		t.Fatal(err)
	}
	if len(again.sidecars) != 0 {
		t.Fatalf("a parked closure ran twice: %v", again.sidecars)
	}
}

func TestPreProcessors_RunInTheOrderTheyWereParkedByType(t *testing.T) {
	const stack = "pp-order--staging"
	var drained int
	var order []string
	for _, name := range []string{"postgres", "pubsub"} {
		name := name
		compute := fakeCtx(stack, &drained, nil)
		_, _, err := RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
			api.ResourceInput{Descriptor: &api.ResourceDescriptor{Name: name}}, pApi.ProvisionParams{},
			func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
				c.AddPreProcessor(&containerArgs{}, func(any) error { order = append(order, name); return nil })
				c.AddPreProcessor(&otherArgs{}, func(any) error { order = append(order, name+"-other"); return nil })
				return nil, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		_ = settle(compute, nil)
	}
	consumer := NewProvisionCollector(provisionReq(stack))
	if err := consumer.RunPreProcessors(&containerArgs{}, &containerArgs{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"postgres", "pubsub"}) {
		t.Fatalf("ran %v, want the closures for this type only, in parking order", order)
	}
	// The other type's closures were not taken.
	if err := consumer.RunPreProcessors(&otherArgs{}, &otherArgs{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"postgres", "pubsub", "postgres-other", "pubsub-other"}) {
		t.Fatalf("ran %v", order)
	}
	_ = settle(fakeCtx(stack, &drained, nil), nil)
}

// A failed ComputeProcess must not leave a closure behind for the template to
// run on a context that has been closed, and its context is settled normally.
func TestPreProcessors_FailedComputeProcessIsDiscardedAndSettled(t *testing.T) {
	const stack = "pp-failed--staging"
	var drained int
	compute := fakeCtx(stack, &drained, nil)
	boom := errors.New("boom")
	_, _, err := RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Name: "postgres"}}, pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			c.AddPreProcessor(&containerArgs{}, func(any) error { t.Fatal("a discarded closure ran"); return nil })
			return nil, boom
		})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got := settle(compute, err); !errors.Is(got, boom) {
		t.Fatalf("settle err = %v", got)
	}
	if drained != 1 {
		t.Fatalf("a failed ComputeProcess context was drained %d times, want 1", drained)
	}
	if err := NewProvisionCollector(provisionReq(stack)).RunPreProcessors(&containerArgs{}, &containerArgs{}); err != nil {
		t.Fatal(err)
	}
}

// Closures the template never ran (it has no container args, say) must not
// strand the ComputeProcess context: the registrations that call made still have
// to reach the engine, and a context nobody drains is dropped when the plugin
// exits.
func TestPreProcessors_UnusedParkedClosureStillDrainsItsContext(t *testing.T) {
	const stack = "pp-unused--staging"
	var computeDrained, provisionDrained int
	compute := fakeCtx(stack, &computeDrained, nil)
	_, _, err := RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Name: "postgres"}}, pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			c.AddPreProcessor(&containerArgs{}, func(any) error { t.Fatal("ran without anyone asking"); return nil })
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	_ = settle(compute, nil)

	NewProvisionCollector(provisionReq(stack)) // a template that never calls RunPreProcessors
	_ = settle(fakeCtx(stack, &provisionDrained, nil), nil)
	if computeDrained != 1 {
		t.Fatalf("the ComputeProcess context was drained %d times, want 1", computeDrained)
	}
}

// Other Settles must not drain a context that a template is still waiting for:
// a second dependency's ComputeProcess, or a Provision that builds no collector,
// finishes while the first dependency's closure is parked.
func TestPreProcessors_OtherSettlesLeaveAParkedContextOpen(t *testing.T) {
	const stack = "pp-bystander--staging"
	var drained int
	compute := fakeCtx(stack, &drained, nil)
	_, _, _ = RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Name: "postgres"}}, pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			c.AddPreProcessor(&containerArgs{}, func(any) error { return nil })
			return nil, nil
		})
	_ = settle(compute, nil)

	var otherDrained int
	_ = settle(fakeCtx(stack, &otherDrained, nil), nil) // e.g. a bucket's ComputeProcess
	if otherDrained != 1 || drained != 0 {
		t.Fatalf("other=%d parked=%d: an unrelated Settle drained the parked context", otherDrained, drained)
	}

	// And another stack's template does not touch it.
	var foreign int
	NewProvisionCollector(provisionReq("pp-someone-else--staging"))
	_ = settle(fakeCtx("pp-someone-else--staging", &foreign, nil), nil)
	if drained != 0 {
		t.Fatal("a template of a different stack drained the parked context")
	}

	NewProvisionCollector(provisionReq(stack))
	_ = settle(fakeCtx(stack, new(int), nil), nil)
	if drained != 1 {
		t.Fatalf("parked context drained %d times at the end, want 1", drained)
	}
}

// What the parked context's drain reports (a registration that failed) fails the
// template's RPC instead of being lost.
func TestPreProcessors_ParkedContextErrorFailsTheTemplate(t *testing.T) {
	const stack = "pp-drainerr--staging"
	var n int
	compute := fakeCtx(stack, &n, errors.New("registration failed"))
	_, _, _ = RunComputeProcess(context.Background(), compute, api.Stack{Name: stack},
		api.ResourceInput{Descriptor: &api.ResourceDescriptor{Name: "postgres"}}, pApi.ProvisionParams{},
		func(_ *sdk.Context, _ api.Stack, _ api.ResourceInput, c pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
			c.AddPreProcessor(&containerArgs{}, func(any) error { return nil })
			return nil, nil
		})
	_ = settle(compute, nil)
	NewProvisionCollector(provisionReq(stack))
	err := settle(fakeCtx(stack, new(int), nil), nil)
	if err == nil || !strings.Contains(err.Error(), "registration failed") {
		t.Fatalf("template Settle err = %v, want the parked context's registration error", err)
	}
}

func TestPreProcessors_ClosureErrorIsWrapped(t *testing.T) {
	c := &Collector{}
	c.AddPreProcessor(&containerArgs{}, func(any) error { return errors.New("no credentials secret") })
	err := c.RunPreProcessors(&containerArgs{}, &containerArgs{})
	if err == nil || !strings.Contains(err.Error(), "failed to apply pre processor: no credentials secret") {
		t.Fatalf("err = %v", err)
	}
}

// A closure added and run inside one RPC (a template registering its own) works
// without the bridge.
func TestPreProcessors_LocalClosuresRunInTheSameRPC(t *testing.T) {
	c := NewProvisionCollector(nil)
	args := &containerArgs{}
	c.AddPreProcessor(&containerArgs{}, func(arg any) error {
		a := arg.(*containerArgs)
		a.sidecars = append(a.sidecars, "local")
		return nil
	})
	got, ok := c.GetPreProcessors(&containerArgs{})
	if !ok || len(got) != 1 {
		t.Fatalf("GetPreProcessors = %d, %v", len(got), ok)
	}
	if err := c.RunPreProcessors(args, args); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args.sidecars, []string{"local"}) {
		t.Fatalf("sidecars = %v", args.sidecars)
	}
}
