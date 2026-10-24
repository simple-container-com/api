// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	hclog "github.com/hashicorp/go-hclog"
	. "github.com/onsi/gomega"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	plugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// fakeMonitor records every RegisterResource call so the smoke test can
// assert the plugin registered the right thing. It is NOT a full Pulumi
// monitor — it only implements RegisterResource; everything else inherits
// pulumirpc.UnimplementedResourceMonitorServer and returns UNIMPLEMENTED.
// That is enough: this test exercises the one gRPC call the plugin needs
// to work.
type fakeMonitor struct {
	pulumirpc.UnimplementedResourceMonitorServer

	mu       sync.Mutex
	received []*pulumirpc.RegisterResourceRequest
}

func (m *fakeMonitor) RegisterResource(_ context.Context, req *pulumirpc.RegisterResourceRequest) (*pulumirpc.RegisterResourceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received = append(m.received, req)
	// Shape the URN the way the real engine would, so the plugin's response
	// looks real to the SC-core caller.
	urn := "urn:pulumi:test-stack::test-project::" + req.GetType() + "::" + req.GetName()
	return &pulumirpc.RegisterResourceResponse{Urn: urn, Id: req.GetName() + "-id"}, nil
}

// TestSmoke_ProvisionDialsMonitorAndRegisters is the end-to-end proof of the
// single-session mechanism from 03-architecture.md §3 option (b):
//
//  1. SC core (this test) stands up a Pulumi Resource Monitor at a known address.
//  2. SC core spawns the plugin via plugin.NewClient and calls Provision,
//     passing the monitor's address in RequestHeader.MonitorAddress.
//  3. The plugin dials THAT monitor (not its own) and calls RegisterResource.
//  4. The monitor records the call; the plugin returns the resulting URN.
//  5. Test verifies: the monitor saw the right RegisterResource (type, name,
//     explicit provider URN), the plugin's response carried the right URN.
//
// This proves the plumbing SC core's inline RunFunc will use when it hands a
// real Pulumi engine's monitor address to a plugin. It does NOT exercise the
// Pulumi engine's state machinery — that comes with Step 4's dispatch wiring,
// which uses a real pulumi.Context's monitor. The plumbing is identical.
func TestSmoke_ProvisionDialsMonitorAndRegisters(t *testing.T) {
	RegisterTestingT(t)

	// Build the fixture plugin.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "provision-plugin")

	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	src := filepath.Join(wd, "testdata", "provision-plugin")

	buildOut, err := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, src).CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "build failed: %s", string(buildOut))

	// Stand up the fake monitor.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())
	monitor := &fakeMonitor{}
	srv := grpc.NewServer()
	pulumirpc.RegisterResourceMonitorServer(srv, monitor)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	// Spawn the plugin via the host-side client.
	logger := hclog.New(&hclog.LoggerOptions{Name: "provision-smoke", Level: hclog.Error})
	cli, err := plugin.NewClient(plugin.ClientConfig{
		Path:    binPath,
		Managed: true,
		Logger:  logger,
	})
	Expect(err).ToNot(HaveOccurred())
	defer cli.Close()

	// Pre-computed explicit provider URN SC core would have handed down. The
	// name byte-matches the template provision.go:320 generates today.
	explicitProvider := "urn:pulumi:test-stack::test-project::pulumi:providers:random::test-stack--random-pet--my-pet--prod--provider::deadbeef-1234"

	// Issue the Provision call.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := cli.Provision(ctx, &scpluginv1.ProvisionRequest{
		Header: &scpluginv1.RequestHeader{
			MonitorAddress: lis.Addr().String(),
			StackName:      "test-stack",
			ProjectName:    "test-project",
			ProviderUrns: map[string]string{
				"random:index/randomPet:RandomPet": explicitProvider,
			},
		},
		ResourceType: "random:index/randomPet:RandomPet",
		ResourceName: "my-pet",
	})
	Expect(err).ToNot(HaveOccurred())

	// The plugin's response carries the URN the fake monitor minted.
	Expect(resp.GetUrn()).To(Equal(
		"urn:pulumi:test-stack::test-project::random:index/randomPet:RandomPet::my-pet",
	))

	// The fake monitor saw ONE RegisterResource call, with the exact shape
	// the design pins expect: type token unchanged, SC-core-computed name, and
	// the explicit provider URN SC core handed down.
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	Expect(monitor.received).To(HaveLen(1))
	got := monitor.received[0]
	Expect(got.GetType()).To(Equal("random:index/randomPet:RandomPet"))
	Expect(got.GetName()).To(Equal("my-pet"))
	Expect(got.GetCustom()).To(BeTrue())
	Expect(got.GetProvider()).To(Equal(explicitProvider),
		"plugin MUST pass the explicit provider URN from RequestHeader.ProviderUrns — "+
			"this is the backward-compat lever 04-no-destroy-invariant.md §3 relies on")
}
