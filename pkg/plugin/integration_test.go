// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration

package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestIntegration_PluginRegistersAgainstRealEngine is the Stage A smoke: it
// stands up a real Pulumi engine via Automation API against a filesystem
// backend, launches the provision-plugin fixture, hands it the monitor address
// (through the MonitorProxy because pulumi/sdk/v3 v3.245.0 does not expose
// Context.MonitorAddr publicly), and verifies the plugin-registered resource
// lands in state with the expected URN.
//
// What this proves vs. the fake-monitor smoke in provision_smoke_test.go:
//
//	fake-monitor smoke  — plugin dials any gRPC endpoint, calls RegisterResource,
//	                      URN flows back. Does not touch Pulumi engine state.
//
//	this smoke         — plugin dials a MonitorProxy that forwards to a REAL
//	                      Pulumi monitor. Pulumi engine commits the resource;
//	                      exported state has the plugin-registered URN; the
//	                      URN survives a destroy cycle too.
//
// This is the integration-level evidence that the "single Pulumi session"
// mechanism from 03-architecture.md §3 option (b) works end-to-end at the
// Pulumi runtime level. Full SC-core-driven e2e (via `sc deploy`) comes with
// Step 4's dispatch wiring; this test is the gating precursor.
//
// Build-tagged `integration` because it requires the `pulumi` CLI and a real
// engine subprocess; `go test` runs it when the tag is on, and plain
// `go test ./...` skips it (keeping `make check` fast). CI wires the tag on
// via a dedicated job that already has pulumi installed.
func TestIntegration_PluginRegistersAgainstRealEngine(t *testing.T) {
	RegisterTestingT(t)

	// Confirm pulumi CLI is on PATH before we build anything.
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("pulumi CLI not on PATH; set up with `pulumi login file:///...` and rerun")
	}

	// Build the provision-plugin fixture (same binary provision_smoke_test uses).
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "provision-plugin")
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	src := filepath.Join(wd, "testdata", "provision-plugin")
	buildOut, err := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, src).CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "build: %s", string(buildOut))

	// Local-fs Pulumi backend under tmpDir. Isolated per test.
	stateDir := filepath.Join(tmpDir, "state")
	Expect(os.MkdirAll(stateDir, 0o755)).To(Succeed())
	backendURL := "file://" + stateDir

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logger := hclog.New(&hclog.LoggerOptions{Name: "integration", Level: hclog.Warn})

	// The RunFunc: SC-core-equivalent code. Captures the plugin URN so the
	// test assertions after Up() can check it.
	var pluginURN string
	runFunc := func(pulumiCtx *pulumi.Context) error {
		// Stand up the monitor proxy. Extracts the live Pulumi monitor client
		// from pulumiCtx via reflect, serves it over gRPC on a loopback port.
		proxy, err := plugin.NewMonitorProxy(pulumiCtx, logger)
		if err != nil {
			return err
		}
		proxyAddr, stop, err := proxy.Serve()
		if err != nil {
			return err
		}
		defer stop()

		// Spawn the plugin via plugin.NewClient.
		cli, err := plugin.NewClient(plugin.ClientConfig{
			Path:    binPath,
			Managed: true,
			Logger:  logger,
		})
		if err != nil {
			return err
		}
		defer cli.Close()

		// Issue Provision with the proxy address as monitor_address. The plugin
		// dials the proxy, calls RegisterResource; proxy forwards to the real
		// monitor; engine commits.
		resp, err := cli.Provision(pulumiCtx.Context(), &scpluginv1.ProvisionRequest{
			Header: &scpluginv1.RequestHeader{
				MonitorAddress: proxyAddr,
				StackName:      "integration-stack",
				ProjectName:    "integration-project",
			},
			// Pulumi's random:index/randomPet:RandomPet is tiny and needs no
			// credentials — appropriate for a plumbing-level smoke.
			ResourceType: "random:index/randomPet:RandomPet",
			ResourceName: "integration-pet",
		})
		if err != nil {
			return err
		}
		pluginURN = resp.GetUrn()
		return nil
	}

	// Automation API — real engine, inline source, fs backend.
	proj := workspace.Project{
		Name:    "integration-project",
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		Backend: &workspace.ProjectBackend{URL: backendURL},
	}
	stack, err := auto.UpsertStackInlineSource(
		ctx, "integration-stack", "integration-project", runFunc,
		auto.Project(proj),
		auto.WorkDir(tmpDir),
		auto.EnvVars(map[string]string{
			"PULUMI_CONFIG_PASSPHRASE": "integration-smoke",
		}),
	)
	Expect(err).ToNot(HaveOccurred())

	// Pull up any Pulumi plugins the test needs (pulumi-random) BEFORE Up.
	// Automation API also auto-installs on first use, but explicitly here keeps
	// the Up phase bounded.
	Expect(stack.Workspace().InstallPlugin(ctx, "random", "v4.18.1")).To(Succeed())

	// `pulumi up` — this is where the real engine runs, the proxy serves, and
	// the plugin registers via gRPC through it.
	upRes, err := stack.Up(ctx, optup.ProgressStreams(os.Stderr))
	Expect(err).ToNot(HaveOccurred(), "pulumi up failed")
	t.Logf("up summary: %+v  plugin-URN=%s", upRes.Summary.ResourceChanges, pluginURN)
	Expect(pluginURN).ToNot(BeEmpty(), "plugin did not return a URN")

	// Export state and verify the plugin-registered RandomPet is in there.
	exportResult, err := stack.Export(ctx)
	Expect(err).ToNot(HaveOccurred())
	type stateCheckpoint struct {
		Resources []struct {
			URN      string `json:"urn"`
			Type     string `json:"type"`
			Provider string `json:"provider"`
		} `json:"resources"`
	}
	var cp stateCheckpoint
	Expect(json.Unmarshal(exportResult.Deployment, &cp)).To(Succeed())

	// The plugin's RandomPet must appear with the URN plugin.Provision returned.
	var found bool
	for _, r := range cp.Resources {
		if r.URN == pluginURN {
			found = true
			Expect(r.Type).To(Equal("random:index/randomPet:RandomPet"))
			Expect(r.Provider).ToNot(BeEmpty(),
				"plugin-registered resource must have a provider ref in state")
			break
		}
	}
	Expect(found).To(BeTrue(),
		"plugin's URN %q not found in state resources: %+v", pluginURN, cp.Resources)

	// Clean up: destroy + remove.
	_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stderr))
	Expect(err).ToNot(HaveOccurred(), "destroy")
}
