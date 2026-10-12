// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// A stack whose provider credentials changed (a rotated key, a move to ambient
// credentials) still holds the old ones in state. A plain refresh uses those and
// fails; refreshOptions makes the refresh run the program, so it uses the
// configuration the stack has now. Reproduced with the Docker provider: its
// host is a socket path that stops existing after the first deploy.
func TestRefreshUsesTheProviderConfigOfTheProgram(t *testing.T) {
	RegisterTestingT(t)
	const daemon = "/var/run/docker.sock"
	if _, err := os.Stat(daemon); err != nil {
		t.Skipf("needs a Docker daemon at %s: %v", daemon, err)
	}
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skipf("needs the pulumi CLI: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dir := t.TempDir()
	socket := func(name string) string {
		p := filepath.Join(dir, name)
		Expect(os.Symlink(daemon, p)).To(Succeed())
		return "unix://" + p
	}
	host := socket("old.sock")
	network := fmt.Sprintf("sc-refresh-test-%d", time.Now().UnixNano())
	program := func(pctx *sdk.Context) error {
		prov, err := docker.NewProvider(pctx, "docker", &docker.ProviderArgs{Host: sdk.String(host)})
		if err != nil {
			return err
		}
		_, err = docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)}, sdk.Provider(prov))
		return err
	}
	s, err := auto.UpsertStackInlineSource(ctx, "test", "sc-refresh-test", program,
		auto.Project(workspace.Project{
			Name:    "sc-refresh-test",
			Runtime: workspace.NewProjectRuntimeInfo("go", nil),
			Backend: &workspace.ProjectBackend{URL: "file://" + dir},
		}),
		auto.EnvVars(map[string]string{"PULUMI_CONFIG_PASSPHRASE": "test"}),
	)
	Expect(err).NotTo(HaveOccurred())
	_, err = s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		_, _ = s.Destroy(context.Background(), optdestroy.RunProgram(true))
	}()

	// The stored host goes away; the program now points at a live one.
	Expect(os.Remove(filepath.Join(dir, "old.sock"))).To(Succeed())
	host = socket("new.sock")

	_, err = s.Refresh(ctx)
	Expect(err).To(HaveOccurred(), "a refresh with the stored provider config should fail")

	_, err = s.Refresh(ctx, refreshOptions()...)
	Expect(err).NotTo(HaveOccurred())
}
