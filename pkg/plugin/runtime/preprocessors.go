// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package runtime

import (
	"reflect"
	"sync"

	"github.com/pkg/errors"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// Pre-processors across the plugin bus.
//
// In-tree, a `uses:` dependency's compute processor registers a closure with the
// consuming stack's ComputeContext (collector.AddPreProcessor), and the stack's
// template runs it on its own container args just before building them
// (collector.RunPreProcessors, in kubernetes.NewSimpleContainer). That is how
// the Cloud SQL proxy becomes a sidecar of the app that uses the database, and
// how its service accounts, credential Secrets and db-user-init Job get created.
//
// With the dependency and the template both owned by a plugin they are two RPCs,
// ComputeProcess and Provision, and the collector of each is a fresh object. A
// closure cannot cross the gRPC boundary, and this collector used to drop every
// one silently: the app deployed with no database proxy and the deploy stayed
// green. A preview of a deployed fleet showed it as the whole proxy apparatus
// planned for deletion, on every stack that uses Cloud SQL.
//
// Both RPCs run in the same plugin process, during the same core program, so the
// closure never has to cross anything: ComputeProcess parks it here, keyed by the
// stack name both request headers carry, and the Provision of the consuming
// template takes it back out.
//
// The closure is only usable with the Pulumi context it captured, and a context
// refuses further registrations once it has been drained and closed, which is
// what Settle does at the end of the ComputeProcess RPC. So a ComputeProcess that
// parked a closure is not settled there: its context stays open, and the consuming
// Provision drains and closes it after running the closure. Ownership moves with
// the closure.
//
// What this does not do: carry a post-processor, a dependency handle or a
// resource handle. Those stay inside one RPC.

type preEntry struct {
	typ   reflect.Type
	fn    pApi.PreProcessor
	owner *Context // nil for a closure added inside the same RPC that runs it
}

var bridge = struct {
	mu        sync.Mutex
	pending   map[string][]preEntry // closures parked by ComputeProcess, by stack
	owners    map[string][]*Context // contexts to drain once the consumer is done
	consumers map[string]bool       // stacks with a Provision collector in flight
}{
	pending:   map[string][]preEntry{},
	owners:    map[string][]*Context{},
	consumers: map[string]bool{},
}

func bridgePark(key string, e preEntry) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.pending[key] = append(bridge.pending[key], e)
	for _, o := range bridge.owners[key] {
		if o == e.owner {
			e.owner.retained = true
			return
		}
	}
	bridge.owners[key] = append(bridge.owners[key], e.owner)
	e.owner.retained = true
}

// bridgeTake removes and returns the parked closures of one stack that apply to
// typ, in the order they were parked.
func bridgeTake(key string, typ reflect.Type) []preEntry {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	var taken, kept []preEntry
	for _, e := range bridge.pending[key] {
		if e.typ == typ {
			taken = append(taken, e)
		} else {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		delete(bridge.pending, key)
	} else {
		bridge.pending[key] = kept
	}
	return taken
}

func bridgeMarkConsumer(key string) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.consumers[key] = true
}

// bridgeDiscard forgets everything one ComputeProcess parked, for a handler that
// failed: nothing will consume it, so it must be settled like any other context.
func bridgeDiscard(owner *Context) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	for key, entries := range bridge.pending {
		var kept []preEntry
		for _, e := range entries {
			if e.owner != owner {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(bridge.pending, key)
		} else {
			bridge.pending[key] = kept
		}
	}
	for key, owners := range bridge.owners {
		var kept []*Context
		for _, o := range owners {
			if o != owner {
				kept = append(kept, o)
			}
		}
		if len(kept) == 0 {
			delete(bridge.owners, key)
		} else {
			bridge.owners[key] = kept
		}
	}
	owner.retained = false
}

func (c *Context) isRetained() bool {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	return c.retained
}

// bridgeFinish drains and closes the contexts of the ComputeProcess calls whose
// closures were parked for key, when a consumer (a Provision that built a
// collector for the stack) has finished. Closures it never ran are dropped: the
// resources their ComputeProcess registered still have to reach the engine, and
// a context left open is one the process would drop on exit.
func bridgeFinish(key string) error {
	bridge.mu.Lock()
	if !bridge.consumers[key] {
		bridge.mu.Unlock()
		return nil
	}
	delete(bridge.consumers, key)
	owners := bridge.owners[key]
	delete(bridge.owners, key)
	delete(bridge.pending, key)
	bridge.mu.Unlock()

	var first error
	for _, o := range owners {
		if err := o.done(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// AddPreProcessor registers a closure to run on a template's container args.
// Added during ComputeProcess it is parked for the consuming Provision; added
// inside the Provision itself it runs there.
func (c *Collector) AddPreProcessor(forType any, processor pApi.PreProcessor) {
	e := preEntry{typ: reflect.TypeOf(forType), fn: processor}
	if c.owner != nil && c.owner.key != "" {
		e.owner = c.owner
		bridgePark(c.owner.key, e)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localPre = append(c.localPre, e)
}

// GetPreProcessors returns the closures registered for forType: those added in
// this RPC, then those parked for this stack by its ComputeProcess calls.
func (c *Collector) GetPreProcessors(forType any) ([]pApi.PreProcessor, bool) {
	typ := reflect.TypeOf(forType)
	var out []pApi.PreProcessor
	if c.key != "" {
		bridge.mu.Lock()
		for _, e := range bridge.pending[c.key] {
			if e.typ == typ {
				out = append(out, e.fn)
			}
		}
		bridge.mu.Unlock()
	}
	c.mu.Lock()
	for _, e := range c.localPre {
		if e.typ == typ {
			out = append(out, e.fn)
		}
	}
	c.mu.Unlock()
	return out, len(out) > 0
}

// RunPreProcessors runs, on onObject, every closure registered for forType's
// type: first the ones the stack's ComputeProcess calls parked, in the order they
// were parked, then the ones added in this RPC. Parked closures are taken, so a
// second call does not run them again.
func (c *Collector) RunPreProcessors(forType any, onObject any) error {
	typ := reflect.TypeOf(forType)
	var run []pApi.PreProcessor
	if c.key != "" {
		for _, e := range bridgeTake(c.key, typ) {
			run = append(run, e.fn)
		}
	}
	c.mu.Lock()
	for _, e := range c.localPre {
		if e.typ == typ {
			run = append(run, e.fn)
		}
	}
	c.mu.Unlock()
	for _, fn := range run {
		if err := fn(onObject); err != nil {
			return errors.Wrap(err, "failed to apply pre processor")
		}
	}
	return nil
}
