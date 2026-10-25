// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	. "github.com/onsi/gomega"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestRoundtrip_Schema builds a tiny fixture plugin in testdata/echo-plugin,
// spawns it as a subprocess via the host-side plugin.Client, calls Schema,
// and asserts the response round-trips cleanly.
//
// This is the end-to-end smoke that proves:
//   - the generated proto compiles (host- and plugin-side use the same types),
//   - the handshake (magic cookie + protocol version 1 + AutoMTLS) succeeds,
//   - gRPC dispatch over the hashicorp/go-plugin transport works,
//   - plugin.Close() tears the subprocess down cleanly.
//
// It does NOT exercise Provider / Provision / InitStateStore — those need
// the Pulumi Resource Monitor on the other end, which lands with Phase 0
// Step 4's dispatch wiring. The Schema RPC is deliberately the one that
// needs no Pulumi context.
func TestRoundtrip_Schema(t *testing.T) {
	RegisterTestingT(t)

	// Build the fixture plugin binary. `go test` already set up a per-test
	// working dir; put the binary in Go's cache dir so it's isolated.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "echo-plugin")

	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	src := filepath.Join(wd, "testdata", "echo-plugin")

	buildCmd := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, src)
	buildCmd.Env = append(os.Environ(), "GOFLAGS=") // avoid inheriting -race, -cover, …
	buildOutput, err := buildCmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "build failed: %s", string(buildOutput))

	// Spawn via plugin.NewClient.
	logger := hclog.New(&hclog.LoggerOptions{Name: "roundtrip-test", Level: hclog.Error})
	cli, err := plugin.NewClient(plugin.ClientConfig{
		Path:    binPath,
		Managed: true,
		Logger:  logger,
	})
	Expect(err).ToNot(HaveOccurred())
	defer cli.Close()

	// Call Schema with a 10 s timeout — a slow spawn on a loaded CI guest can
	// trip this, but it bounds the test if the plugin wedges.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := cli.Schema(ctx, &scpluginv1.RequestHeader{
		StackName:   "test-stack",
		ProjectName: "test-project",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.GetPluginVersion()).To(Equal("test-echo-0.0.1"))
	Expect(resp.GetProviderTypes()).To(Equal([]string{"echo"}))
	Expect(resp.GetResourceTypes()).To(Equal([]string{"echo:say:Say"}))
}
