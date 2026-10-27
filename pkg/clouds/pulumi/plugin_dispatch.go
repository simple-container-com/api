// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"sync"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
)

// pluginSession is the per-ProvisionStack/DeployStack/DestroyStack call-level
// state that in-engine plugin sidecars read from. It owns the MonitorProxy
// (the loopback gRPC endpoint plugins dial to call RegisterResource against
// SC core's live Pulumi engine) and the provider URNs plugins have registered
// in the active deploy — those get propagated to downstream RegisterResource
// calls via RequestHeader.ProviderUrns.
//
// Lifetime: attached by an inline RunFunc via startPluginSession — both
// provisionProgram and deployStackProgram — and torn down by
// closePluginSessions once the ENGINE OPERATION that invoked that RunFunc has
// returned. NOT when the RunFunc returns: see startPluginSession. The
// in-engine sidecars (resource / provider / registrar / compute-process) read
// `activeSession` from the package-level var at call time; a nil session means
// the sidecar was called outside a live engine operation, which is an error.
type pluginSession struct {
	mu           sync.Mutex
	proxyAddr    string
	proxyStopper func()
	providerURNs map[string]string
}

func newPluginSession() *pluginSession {
	return &pluginSession{providerURNs: map[string]string{}}
}

// StartProxy stands up a MonitorProxy inside the given pulumi.Context.
// Returns a shutdown func the caller is expected to defer.
func (s *pluginSession) StartProxy(pulumiCtx *sdk.Context) error {
	hcLog := hclog.New(&hclog.LoggerOptions{Name: "sc-monitor-proxy", Level: hclog.Error})
	proxy, err := scPlugin.NewMonitorProxy(pulumiCtx, hcLog)
	if err != nil {
		return err
	}
	addr, stop, err := proxy.Serve()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.proxyAddr = addr
	s.proxyStopper = stop
	s.mu.Unlock()
	return nil
}

// Close shuts the MonitorProxy down. Safe to call multiple times.
func (s *pluginSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proxyStopper != nil {
		s.proxyStopper()
		s.proxyStopper = nil
	}
}

// Active pluginSession lookup — package-level because threading one through
// sdk.Context would require writing to pulumi.Context's unexported `ctx`
// field. SC today runs one provision call per process at a time, which
// matches this contract. If that invariant ever breaks (parallel deploys
// inside one process), switch to the sdk.Context ↔ session map pattern.
var (
	activeSessionMu sync.Mutex
	activeSession   *pluginSession

	// pendingSessionStops holds the teardown of every session attached since
	// the last closePluginSessions. It exists because teardown happens one
	// level further out than attach does — see startPluginSession.
	pendingStopsMu      sync.Mutex
	pendingSessionStops []func()
)

func attachPluginSession(_ *sdk.Context, s *pluginSession) {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if activeSession != nil && activeSession != s {
		// A session left over from the previous engine run of the same
		// operation — typically the preview that precedes an update. That run
		// is definitively finished (the engine does not start a second one
		// until the first completes), so its proxy can go now rather than
		// sitting on a loopback port until closePluginSessions.
		activeSession.Close()
	}
	activeSession = s
}

// detachPluginSession clears the active session, but only if it is still the
// one the caller attached. A stale teardown must never unpublish a newer
// session.
func detachPluginSession(s *pluginSession) {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if s == nil || activeSession == s {
		activeSession = nil
	}
}

func registerSessionStop(stop func()) {
	pendingStopsMu.Lock()
	defer pendingStopsMu.Unlock()
	pendingSessionStops = append(pendingSessionStops, stop)
}

// closePluginSessions tears down every plugin session attached since the last
// call. EVERY function that runs a plugin-capable inline program through the
// engine must defer it — provisionStack, deployStack, previewStack,
// previewChildStack and destroyStack — because that is the scope a session's
// lifetime is tied to. Safe to call when nothing was attached.
func closePluginSessions() {
	pendingStopsMu.Lock()
	stops := pendingSessionStops
	pendingSessionStops = nil
	pendingStopsMu.Unlock()
	// LIFO: the last one attached is the one activeSession points at.
	for i := len(stops) - 1; i >= 0; i-- {
		stops[i]()
	}
}

// startPluginSession stands up the in-engine MonitorProxy and publishes it as the
// active session. When no plugins were loaded pre-engine it is a no-op, so callers
// need no conditional.
//
// It deliberately hands the caller NO cleanup func. The teardown is registered
// internally and runs in closePluginSessions, which the OUTER operation defers
// around the engine call. Deferring it inside the RunFunc — the obvious thing,
// and what both programs did until 2026-10-28 — is a bug: the real work of
// deployStackProgram and provisionProgram happens inside an output
// continuation (`collector.Outputs().ApplyTWithContext(...)` in deploy.go),
// which the engine invokes AFTER the RunFunc has returned, while draining
// outstanding RPCs. A `defer stop()` therefore nils the session before the
// dispatch that needs it, and the operator sees:
//
//	error: an unhandled error occurred: waiting for RPCs: ... :
//	plugin dispatch for "cloudflare" requires an active plugin session (no inline RunFunc running)
//
// The `waiting for RPCs` prefix IS the tell — it means the engine had already
// left the RunFunc. Live-caught 2026-10-27 on this repo's own release pipeline
// (api run 40587424994, `dist--prod`), immediately after deployStackProgram was
// given a session at all: initRegistrar, which runs synchronously inside the
// RunFunc, succeeded one second before the ApplyT'd NewRecord on the very same
// registrar failed. Two dispatches to one plugin, one working and one not, is
// the signature of this defect rather than of a broken plugin.
//
// EVERY inline RunFunc that can reach a plugin sidecar needs this, not only
// provisionProgram. The in-engine sidecars (resource / provider / registrar /
// compute-process) read the session from the package-level activeSession at call
// time; a program that skips this leaves them with no monitor address, and the
// failure reads exactly like the plugin was never installed. That is what
// deployStackProgram did until 2026-10-27 — `sc provision` of a plugin-owned
// registrar worked while `sc deploy` of the same stack could not, which sends
// you looking at the plugin instead of at the program.
func (p *pulumi) startPluginSession(ctx *sdk.Context) error {
	if p.preEnginePlugins == nil {
		return nil
	}
	sess := newPluginSession()
	if err := sess.StartProxy(ctx); err != nil {
		sess.Close()
		return errors.Wrap(err, "start monitor proxy")
	}
	attachPluginSession(ctx, sess)
	// Detach before Close: no sidecar should find a session whose proxy is
	// already shut down.
	registerSessionStop(func() {
		detachPluginSession(sess)
		sess.Close()
	})
	return nil
}

// verifySHA256 validates spec.SHA256 against the binary at spec.Path when
// a hash is configured. An empty hash is a warn-and-proceed per D10 — the
// smoke accepts this; production should treat it as an error.
func verifySHA256(spec api.PluginSpec) error {
	if spec.SHA256 == "" {
		return nil
	}
	f, err := os.Open(spec.Path)
	if err != nil {
		return errors.Wrap(err, "open plugin for hash check")
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, "hash plugin binary")
	}
	got := hex.EncodeToString(h.Sum(nil))
	want := strings.TrimSpace(spec.SHA256)
	if !strings.EqualFold(got, want) {
		return errors.Errorf("plugin SHA256 mismatch: want %s, got %s", want, got)
	}
	return nil
}

// Unused imports after the refactor: context, api are kept because sidecars
// in plugin_sidecars.go reach into activeSession via these types.
var (
	_ context.Context = nil
	_ api.PluginSpec  // keep import for verifySHA256
)
