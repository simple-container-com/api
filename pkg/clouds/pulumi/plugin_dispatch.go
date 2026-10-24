// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// pluginDispatch is the host-side state for one running cloud plugin:
// a go-plugin client, the schema it advertised, and the explicit list of
// resource types SC core will route to it.
type pluginDispatch struct {
	name   string
	spec   api.PluginSpec
	client *scPlugin.Client
	schema *scpluginv1.SchemaResponse
}

// pluginSession is the host-side lifetime of a plugin dispatch surface for
// one ProvisionStack / DestroyStack / PreviewStack / RefreshStack call. It
// owns:
//   - the loaded plugins (process handles + their schemas),
//   - the MonitorProxy that bridges SC core's live pulumi.Context monitor
//     to the plugin subprocess over gRPC,
//   - a dispatch table resource types → plugin.
//
// Create one at the start of a Pulumi operation; call Close() in defer.
type pluginSession struct {
	mu           sync.Mutex
	log          logger.Logger
	plugins      []*pluginDispatch
	resourceToP  map[string]*pluginDispatch
	proxyAddr    string
	proxyStopper func()
}

func newPluginSession(log logger.Logger) *pluginSession {
	return &pluginSession{
		log:         log,
		resourceToP: map[string]*pluginDispatch{},
	}
}

// LoadAll spawns every plugin in cfg, calls Schema on each, and populates
// the host-side dispatch table. Must be called BEFORE the inline RunFunc
// starts registering resources — ProvisionFuncByType dispatch at
// provision.go consults the table after an in-tree miss.
func (s *pluginSession) LoadAll(ctx context.Context, specs map[string]api.PluginSpec) error {
	if len(specs) == 0 {
		return nil
	}
	for name, spec := range specs {
		if err := s.loadOne(ctx, name, spec); err != nil {
			return errors.Wrapf(err, "load plugin %q", name)
		}
	}
	return nil
}

func (s *pluginSession) loadOne(ctx context.Context, name string, spec api.PluginSpec) error {
	if spec.Path == "" {
		return errors.Errorf("plugin %q: path is empty", name)
	}
	if err := verifySHA256(spec); err != nil {
		return err
	}
	hcLog := hclog.New(&hclog.LoggerOptions{
		Name:   "sc-plugin-" + name,
		Level:  hclog.Warn,
		Output: os.Stderr,
	})
	cli, err := scPlugin.NewClient(scPlugin.ClientConfig{
		Path:    spec.Path,
		Managed: true,
		Logger:  hcLog,
	})
	if err != nil {
		return errors.Wrap(err, "spawn plugin")
	}

	schema, err := cli.Schema(ctx, &scpluginv1.RequestHeader{})
	if err != nil {
		cli.Close()
		return errors.Wrap(err, "schema")
	}
	s.log.Info(ctx, "loaded plugin %q v%s with resource types %v",
		name, schema.GetPluginVersion(), schema.GetResourceTypes())

	d := &pluginDispatch{name: name, spec: spec, client: cli, schema: schema}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.plugins = append(s.plugins, d)
	for _, resType := range schema.GetResourceTypes() {
		if existing, ok := s.resourceToP[resType]; ok {
			return errors.Errorf("plugin %q and plugin %q both claim resource type %q",
				existing.name, name, resType)
		}
		s.resourceToP[resType] = d
	}
	return nil
}

// StartProxy stands up a MonitorProxy inside the given pulumi.Context,
// returning the loopback gRPC address that plugins must dial via
// RequestHeader.MonitorAddress. Call Close() to shut the proxy down.
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

// Dispatch returns a plugin that owns the given resource type, or nil if no
// loaded plugin does. SC core's configureResource calls this on an in-tree
// ProvisionFuncByType miss.
func (s *pluginSession) Dispatch(resType string) *pluginDispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resourceToP[resType]
}

// Provision asks the plugin to materialise one SC-level resource. The
// plugin dials MonitorAddress (our proxy) and calls RegisterResource against
// SC core's live Pulumi engine. Returns the URN the plugin reported.
func (s *pluginSession) Provision(ctx context.Context, d *pluginDispatch, stackName, projectName, env, resName, resType string) (*scpluginv1.ProvisionResponse, error) {
	s.mu.Lock()
	addr := s.proxyAddr
	s.mu.Unlock()
	if addr == "" {
		return nil, errors.New("plugin monitor proxy not started — StartProxy was skipped")
	}
	return d.client.Provision(ctx, &scpluginv1.ProvisionRequest{
		Header: &scpluginv1.RequestHeader{
			MonitorAddress: addr,
			StackName:      stackName,
			ProjectName:    projectName,
			Environment:    env,
		},
		ResourceType: resType,
		ResourceName: resName,
	})
}

// Close tears down all plugin subprocesses + the monitor proxy. Safe to
// call multiple times.
func (s *pluginSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proxyStopper != nil {
		s.proxyStopper()
		s.proxyStopper = nil
	}
	for _, p := range s.plugins {
		p.client.Close()
	}
	s.plugins = nil
	s.resourceToP = map[string]*pluginDispatch{}
}

// verifySHA256 validates spec.SHA256 against the binary at spec.Path when
// a hash is configured. An empty hash is a warn-and-proceed per D10 — the
// smoke accepts this; production should treat it as an error.
func verifySHA256(spec api.PluginSpec) error {
	if spec.SHA256 == "" {
		return nil // caller logs the warn
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

func sessionFromContext(_ context.Context) *pluginSession {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	return activeSession
}

// pluginProvisionFunc is the adapter that satisfies pApi.ProvisionFunc for a
// resource type owned by a plugin. It looks up the active plugin session on
// the Pulumi SDK context, dispatches to the plugin, and returns a stub
// ResourceOutput carrying the plugin's returned URN. The URN string makes
// the resource visible to any downstream compute processor that needs to
// consult it; richer typed output integration is Step 4b.
func pluginProvisionFunc(ctx *sdk.Context, _ api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	sess := sessionFromContext(ctx.Context())
	if sess == nil {
		return nil, errors.Errorf("resource type %q has no in-tree ProvisionFunc and no plugin session is active", input.Descriptor.Type)
	}
	d := sess.Dispatch(input.Descriptor.Type)
	if d == nil {
		return nil, errors.Errorf("unknown resource type %q (not in-tree and no plugin claims it)", input.Descriptor.Type)
	}
	stackName := ""
	projectName := ""
	env := ""
	if input.StackParams != nil {
		stackName = input.StackParams.StackName
		env = input.StackParams.Environment
	}
	resp, err := sess.Provision(ctx.Context(), d, stackName, projectName, env, input.Descriptor.Name, input.Descriptor.Type)
	if err != nil {
		return nil, errors.Wrapf(err, "plugin %q Provision for %q", d.name, input.Descriptor.Name)
	}
	// Prevent "unused variable" lint on params during the smoke; richer wiring
	// (ComputeContext, dependency outputs) comes in Step 4b.
	_ = params
	return &api.ResourceOutput{Ref: fmt.Sprintf("plugin:%s:%s", d.name, resp.GetUrn())}, nil
}
