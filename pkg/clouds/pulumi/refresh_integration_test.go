// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration

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
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const dockerDaemon = "/var/run/docker.sock"

// requireEngine needs a Docker daemon and the pulumi CLI; in CI their absence
// fails the test rather than skipping it.
func requireEngine(t *testing.T) {
	t.Helper()
	missing := ""
	if _, err := os.Stat(dockerDaemon); err != nil {
		missing = "a Docker daemon at " + dockerDaemon
	} else if _, err := exec.LookPath("pulumi"); err != nil {
		missing = "the pulumi CLI"
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
// directory. Everything it creates is destroyed when the test ends.
func engineStack(t *testing.T, ctx context.Context, program sdk.RunFunc) auto.Stack {
	t.Helper()
	dir := t.TempDir()
	s, err := auto.UpsertStackInlineSource(ctx, "test", "sc-refresh-test", program,
		auto.WorkDir(t.TempDir()),
		auto.Project(workspace.Project{
			Name:    "sc-refresh-test",
			Runtime: workspace.NewProjectRuntimeInfo("go", nil),
			Backend: &workspace.ProjectBackend{URL: "file://" + dir},
		}),
		auto.EnvVars(map[string]string{"PULUMI_CONFIG_PASSPHRASE": "test"}),
	)
	Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		if _, err := s.Destroy(context.Background(), optdestroy.RunProgram(true)); err != nil {
			t.Logf("destroy: %v", err)
		}
	})
	return s
}

func uniqueName(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

// After a credential change the provider configuration in state no longer
// works. A plain refresh uses it and fails; the refresh inside a preview and an
// update runs the program and uses the configuration the stack has now, and
// the update writes it to state. Reproduced with the Docker provider: its host
// is a socket path that stops existing after the first deploy.
func TestRefreshUsesTheProviderConfigOfTheProgram(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dir := t.TempDir()
	socket := func(name string) string {
		p := filepath.Join(dir, name)
		Expect(os.Symlink(dockerDaemon, p)).To(Succeed())
		return "unix://" + p
	}
	host := socket("old.sock")
	network := uniqueName("sc-refresh-test")
	s := engineStack(t, ctx, func(pctx *sdk.Context) error {
		prov, err := docker.NewProvider(pctx, "docker", &docker.ProviderArgs{Host: sdk.String(host)})
		if err != nil {
			return err
		}
		_, err = docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)}, sdk.Provider(prov))
		return err
	})
	_, err := s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Remove(filepath.Join(dir, "old.sock"))).To(Succeed())
	host = socket("new.sock")

	_, err = s.Refresh(ctx)
	Expect(err).To(HaveOccurred(), "a refresh with the provider configuration in state should fail")

	_, err = s.Preview(ctx, previewRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())
	_, err = s.Up(ctx, upRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())

	_, err = s.Refresh(ctx)
	Expect(err).NotTo(HaveOccurred(), "the update should have written the current provider configuration to state")
}

// A resource that does not exist yet has unknown outputs in a preview. The
// program's check that a real run gets a real value must hold on a first
// deploy. A standalone refresh that runs the program is not a dry run and
// would hand it an empty value; the refresh inside the operations keeps the
// operation's semantics.
func TestRefreshKeepsPreviewSemanticsOnAFirstDeploy(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	network := uniqueName("sc-refresh-first")
	s := engineStack(t, ctx, func(pctx *sdk.Context) error {
		net, err := docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)})
		if err != nil {
			return err
		}
		pctx.Export("id", net.ID().ApplyT(func(id sdk.ID) (string, error) {
			if !pctx.DryRun() && id == "" {
				return "", fmt.Errorf("a real run got an empty network id")
			}
			return string(id), nil
		}))
		return nil
	})

	_, err := s.Refresh(ctx, optrefresh.RunProgram(true))
	Expect(err).To(HaveOccurred(), "a standalone refresh with the program is expected to see the empty id")

	_, err = s.Preview(ctx, previewRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())
	_, err = s.Up(ctx, upRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())
}

// A preview never writes state: the refresh inside it must not publish the
// outputs of a program that has not been applied.
func TestPreviewRefreshDoesNotPublishOutputs(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	version := "v1"
	network := uniqueName("sc-refresh-outputs")
	s := engineStack(t, ctx, func(pctx *sdk.Context) error {
		if _, err := docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)}); err != nil {
			return err
		}
		pctx.Export("template", sdk.String(version))
		return nil
	})
	_, err := s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())

	version = "v2"
	_, err = s.Preview(ctx, previewRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())
	out, err := s.Outputs(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(out["template"].Value).To(Equal("v1"))
}

// The update still refreshes: a resource deleted outside Pulumi is found
// missing and created again.
func TestUpdateRefreshFindsDrift(t *testing.T) {
	RegisterTestingT(t)
	requireEngine(t)
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("needs the docker CLI: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	network := uniqueName("sc-refresh-drift")
	s := engineStack(t, ctx, func(pctx *sdk.Context) error {
		_, err := docker.NewNetwork(pctx, "net", &docker.NetworkArgs{Name: sdk.String(network)})
		return err
	})
	_, err := s.Up(ctx)
	Expect(err).NotTo(HaveOccurred())

	out, err := exec.Command("docker", "network", "rm", network).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))

	_, err = s.Up(ctx, upRefresh(false)...)
	Expect(err).NotTo(HaveOccurred())
	out, err = exec.Command("docker", "network", "inspect", network).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "the update did not recreate the network deleted outside Pulumi: %s", out)
}
