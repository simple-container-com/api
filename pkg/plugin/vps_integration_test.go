// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration

package plugin_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestIntegration_PluginTouchesVPS is the Stage B smoke: plugin registers a
// command:remote:Command via MonitorProxy; the real Pulumi engine commits it,
// which SSHes to a real VPS and writes a marker file. The test verifies the
// file exists after up and is gone after destroy.
//
// Opt-in via env — requires SC_PLUGIN_VPS_{HOST,USER,PASSWORD} set to a VPS
// the operator has authorised for a smoke. Default CI runs skip.
//
// What this proves beyond Stage A:
//
//	Stage A (TestIntegration_PluginRegistersAgainstRealEngine) — plugin
//	        registers a resource that Pulumi tracks in state. No real
//	        infrastructure touched.
//
//	This test (Stage B) — plugin registers a resource that TOUCHES real
//	        infrastructure. The create command runs over SSH on the VPS, the
//	        file lands, the destroy command removes it. End-to-end evidence
//	        that the plugin runtime can drive real cloud/VPS operations
//	        through a shared Pulumi engine.
//
// Does NOT yet go through the sc CLI — Step 4 (dispatch wiring in SC core)
// is the next slice. This test proves the plumbing works before Step 4
// takes a dependency on it.
func TestIntegration_PluginTouchesVPS(t *testing.T) {
	RegisterTestingT(t)

	host := os.Getenv("SC_PLUGIN_VPS_HOST")
	user := os.Getenv("SC_PLUGIN_VPS_USER")
	password := os.Getenv("SC_PLUGIN_VPS_PASSWORD")
	if host == "" || user == "" || password == "" {
		t.Skip("SC_PLUGIN_VPS_{HOST,USER,PASSWORD} not set; export them to a smoke-authorised VPS to run")
	}
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("pulumi CLI not on PATH")
	}
	if _, err := exec.LookPath("sshpass"); err != nil {
		t.Skip("sshpass not installed; apt-get install sshpass")
	}

	// Build the vps-marker-plugin fixture.
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "vps-marker-plugin")
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	src := filepath.Join(wd, "testdata", "vps-marker-plugin")
	buildOut, err := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, src).CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "build: %s", string(buildOut))

	stateDir := filepath.Join(tmpDir, "state")
	Expect(os.MkdirAll(stateDir, 0o755)).To(Succeed())
	backendURL := "file://" + stateDir

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	logger := hclog.New(&hclog.LoggerOptions{Name: "vps-integration", Level: hclog.Warn})

	var pluginURN string
	runFunc := func(pulumiCtx *pulumi.Context) error {
		proxy, err := plugin.NewMonitorProxy(pulumiCtx, logger)
		if err != nil {
			return err
		}
		proxyAddr, stop, err := proxy.Serve()
		if err != nil {
			return err
		}
		defer stop()

		cli, err := plugin.NewClient(plugin.ClientConfig{
			Path:    binPath,
			Managed: true,
			Logger:  logger,
		})
		if err != nil {
			return err
		}
		defer cli.Close()

		resp, err := cli.Provision(pulumiCtx.Context(), &scpluginv1.ProvisionRequest{
			Header: &scpluginv1.RequestHeader{
				MonitorAddress: proxyAddr,
				StackName:      "vps-smoke",
				ProjectName:    "vps-smoke-project",
			},
			ResourceType: "command:remote:Command",
			ResourceName: "vps-marker",
		})
		if err != nil {
			return err
		}
		pluginURN = resp.GetUrn()
		return nil
	}

	proj := workspace.Project{
		Name:    "vps-smoke-project",
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
		Backend: &workspace.ProjectBackend{URL: backendURL},
	}
	stack, err := auto.UpsertStackInlineSource(
		ctx, "vps-smoke", "vps-smoke-project", runFunc,
		auto.Project(proj),
		auto.WorkDir(tmpDir),
		auto.EnvVars(map[string]string{
			"PULUMI_CONFIG_PASSPHRASE": "vps-smoke",
			"SC_PLUGIN_VPS_HOST":       host,
			"SC_PLUGIN_VPS_USER":       user,
			"SC_PLUGIN_VPS_PASSWORD":   password,
		}),
	)
	Expect(err).ToNot(HaveOccurred())

	// Pre-install pulumi-command to avoid timing-related failures during Up.
	Expect(stack.Workspace().InstallPlugin(ctx, "command", "v1.2.1")).To(Succeed())

	upRes, err := stack.Up(ctx, optup.ProgressStreams(os.Stderr))
	Expect(err).ToNot(HaveOccurred(), "pulumi up failed")
	t.Logf("up summary: %+v  plugin-URN=%s", upRes.Summary.ResourceChanges, pluginURN)

	// Verify the plugin's create command ran against the VPS and wrote the file.
	// Use .Output() to avoid picking up ssh's "known hosts" warning from stderr.
	readCmd := exec.CommandContext(ctx, "sshpass", "-p", password,
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "PubkeyAuthentication=no", "-o", "PreferredAuthentications=password",
		"-o", "IdentitiesOnly=yes", "-o", "LogLevel=ERROR",
		user+"@"+host, "cat /tmp/sc-plugin-marker.txt")
	readOut, err := readCmd.Output()
	Expect(err).ToNot(HaveOccurred(), "ssh read failed: out=%q", string(readOut))
	Expect(strings.TrimSpace(string(readOut))).To(Equal("sc-plugin-marker-vps-marker"),
		"marker file on VPS has wrong content: %q", string(readOut))

	// Destroy — command:remote:Command's delete runs on the VPS and should remove the file.
	_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stderr))
	Expect(err).ToNot(HaveOccurred(), "destroy failed")

	// Verify the destroy command ran — the file should now be gone.
	checkCmd := exec.CommandContext(ctx, "sshpass", "-p", password,
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "PubkeyAuthentication=no", "-o", "PreferredAuthentications=password",
		"-o", "IdentitiesOnly=yes", "-o", "LogLevel=ERROR",
		user+"@"+host, "test -f /tmp/sc-plugin-marker.txt && echo PRESENT || echo ABSENT")
	checkOut, err := checkCmd.Output()
	Expect(err).ToNot(HaveOccurred(), "ssh check failed: out=%q", string(checkOut))
	Expect(strings.TrimSpace(string(checkOut))).To(Equal("ABSENT"),
		"destroy did not remove the marker: %q", string(checkOut))
}
