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
// Lifetime: created by an inline RunFunc via startPluginSession — both
// provisionProgram and deployStackProgram — and torn down when that RunFunc
// returns. The in-engine sidecars (resource / provider / registrar /
// compute-process) read `activeSession` from the package-level var at call
// time; a nil session means the sidecar was called outside the inline RunFunc,
// which is an error.
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
)

func attachPluginSession(_ *sdk.Context, s *pluginSession) {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	activeSession = s
}

func detachPluginSession() {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	activeSession = nil
}

// startPluginSession stands up the in-engine MonitorProxy and publishes it as the
// active session, returning a cleanup the caller MUST defer. When no plugins were
// loaded pre-engine it is a no-op returning a no-op cleanup, so callers need no
// conditional.
//
// EVERY inline RunFunc that can reach a plugin sidecar needs this, not only
// provisionProgram. The in-engine sidecars (resource / provider / registrar /
// compute-process) read the session from the package-level activeSession at call
// time; a program that skips this leaves them with no monitor address, and the
// failure reads exactly like the plugin was never installed. That is what
// deployStackProgram did until 2026-10-27 — `sc provision` of a plugin-owned
// registrar worked while `sc deploy` of the same stack could not, which sends
// you looking at the plugin instead of at the program.
func (p *pulumi) startPluginSession(ctx *sdk.Context) (func(), error) {
	if p.preEnginePlugins == nil {
		return func() {}, nil
	}
	sess := newPluginSession()
	if err := sess.StartProxy(ctx); err != nil {
		sess.Close()
		return nil, errors.Wrap(err, "start monitor proxy")
	}
	attachPluginSession(ctx, sess)
	// Detach before Close, matching the original defer order in provisionProgram:
	// no sidecar should find a session whose proxy is already shut down.
	return func() {
		detachPluginSession()
		sess.Close()
	}, nil
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
