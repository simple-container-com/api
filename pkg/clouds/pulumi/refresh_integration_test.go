// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration

package pulumi

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// dockerSocket is the daemon socket from DOCKER_HOST, or the default one.
func dockerSocket() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	return "/var/run/docker.sock"
}

// requireEngine needs a reachable Docker daemon and the pulumi and docker
// CLIs; in CI their absence fails the test rather than skipping it.
func requireEngine(t *testing.T) {
	t.Helper()
	missing := ""
	if c, err := net.Dial("unix", dockerSocket()); err != nil {
		missing = "a Docker daemon at " + dockerSocket()
	} else {
		_ = c.Close()
		for _, cli := range []string{"pulumi", "docker"} {
			if _, err := exec.LookPath(cli); err != nil {
				missing = "the " + cli + " CLI"
			}
		}
	}
	if missing == "" {
		return
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("needs %s", missing)
	}
	t.Skipf("needs %s", missing)
}

// engineStack is an inline-program stack in a file backend under a temporary
// directory. The stack and the named networks are removed when the test ends.
func engineStack(ctx context.Context, t *testing.T, program sdk.RunFunc, networks ...string) auto.Stack {
	t.Helper()
	s, err := auto.UpsertStackInlineSource(
		ctx, "test", "sc-refresh-test", program,
		auto.WorkDir(t.TempDir()),
		auto.Project(workspace.Project{
			Name:    "sc-refresh-test",
			Runtime: workspace.NewProjectRuntimeInfo("go", nil),
			Backend: &workspace.ProjectBackend{URL: "file://" + t.TempDir()},
		}),
		auto.EnvVars(map[string]string{"PULUMI_CONFIG_PASSPHRASE": "test"}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		dctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if _, err := s.Destroy(dctx, optdestroy.RunProgram(true)); err != nil {
			t.Errorf("destroy: %v", err)
		}
		for _, n := range networks {
			_ = exec.Command("docker", "network", "rm", n).Run()
		}
	})
	return s
}

func uniqueName(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func testProvisioner() *pulumi { return &pulumi{logger: logger.New()} }

func networkExists(name string) bool {
	return exec.Command("docker", "network", "inspect", name).Run() == nil
}

// After a credential change the provider configuration in state no longer
// works: a plain refresh uses it and fails. The update's refresh runs the
// program, so it reaches the provider with the configuration the stack has
// now, finds the drift, and writes the current configuration to state.
// Reproduced with the Docker provider: its host is a socket path that stops
// existing after the first deploy, and its network is deleted outside Pulumi.
func TestEngineUpdateRefreshesWithTheProviderConfigOfTheProgram(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	p := testProvisioner()

	dir, err := os.MkdirTemp("", "scr")
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := func(name string) string {
		path := filepath.Join(dir, name)
		Expect(os.Symlink(dockerSocket(), path)).To(Succeed())
		return "unix://" + path
	}
	host := socket("old.sock")
	network := uniqueName("sc-refresh-config")
	s := engineStack(ctx, t, func(pctx *sdk.Context) error {
		prov, err := docker.NewProvider(pctx, "docker", &docker.ProviderArgs{Host: sdk.String(host)})
		if err != nil {
			return err
		}
		_, err = docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)}, sdk.Provider(prov))
		return err
	}, network)
	_, err = s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Remove(filepath.Join(dir, "old.sock"))).To(Succeed())
	host = socket("new.sock")
	out, err := exec.Command("docker", "network", "rm", network).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))

	_, err = s.Refresh(ctx)
	Expect(err).To(MatchError(ContainSubstring("failed to connect to any docker daemon")))

	_, err = s.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, false)...)
	Expect(err).NotTo(HaveOccurred())
	_, err = s.Up(ctx, p.upOptions(ctx, ActionContextDeploy, false, false)...)
	Expect(err).NotTo(HaveOccurred())
	Expect(networkExists(network)).To(BeTrue(), "the update's refresh did not find the network deleted outside Pulumi")

	_, err = s.Refresh(ctx)
	Expect(err).NotTo(HaveOccurred(), "the update should have written the current provider configuration to state")
}

// A provider configured from the output of a resource being replaced has an
// unknown configuration in a preview. A refresh inside the preview would read
// the resources of that provider as deleted and report them as created; the
// preview must report what the update will do, and the update must refresh
// them as they are.
func TestEnginePreviewOfAProviderWithUnknownConfig(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	p := testProvisioner()

	first, second, dependent := uniqueName("sc-refresh-a"), uniqueName("sc-refresh-a2"), uniqueName("sc-refresh-b")
	name := first
	s := engineStack(ctx, t, func(pctx *sdk.Context) error {
		a, err := docker.NewNetwork(pctx, "a", &docker.NetworkArgs{Name: sdk.String(name)})
		if err != nil {
			return err
		}
		prov, err := docker.NewProvider(pctx, "b", &docker.ProviderArgs{
			Host: a.ID().ApplyT(func(sdk.ID) string { return "unix://" + dockerSocket() }).(sdk.StringOutput),
		})
		if err != nil {
			return err
		}
		_, err = docker.NewNetwork(pctx, "b", &docker.NetworkArgs{Name: sdk.String(dependent)}, sdk.Provider(prov))
		return err
	}, first, second, dependent)
	_, err := s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())

	name = second
	preview, err := s.Preview(ctx, p.previewOptions(ctx, ActionContextPreview, false)...)
	Expect(err).NotTo(HaveOccurred())
	Expect(preview.ChangeSummary).To(HaveKeyWithValue(apitype.OpReplace, 1))
	Expect(preview.ChangeSummary).NotTo(HaveKey(apitype.OpCreate), "the preview reports a resource of the unknown provider as created")

	up, err := s.Up(ctx, p.upOptions(ctx, ActionContextDeploy, false, false)...)
	Expect(err).NotTo(HaveOccurred())
	Expect(*up.Summary.ResourceChanges).NotTo(HaveKey(string(apitype.OpCreate)))
	Expect(networkExists(dependent)).To(BeTrue())
}
