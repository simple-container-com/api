// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/types"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

func TestComposeLoad(t *testing.T) {
	RegisterTestingT(t)

	cfg, err := ReadDockerCompose(context.Background(), "", "testdata/stacks/refapp/docker-compose.yaml")
	Expect(err).To(BeNil())
	Expect(cfg.Project).NotTo(BeNil())

	Expect(cfg.Project.Services).To(HaveLen(3))
	api, apiFound := lo.Find(cfg.Project.Services, func(svc types.ServiceConfig) bool {
		return svc.Name == "api"
	})
	Expect(apiFound).To(BeTrue())
	Expect(api.ContainerName).To(Equal("refapp-api"))
	ui, uiFound := lo.Find(cfg.Project.Services, func(svc types.ServiceConfig) bool {
		return svc.Name == "ui"
	})
	Expect(uiFound).To(BeTrue())
	Expect(ui.ContainerName).To(Equal("refapp-ui"))
}

// An absolute composeFilePath must re-root workingDir onto the file's own
// directory: relative build contexts inside the compose file are resolved
// against workingDir, so honouring a caller-supplied one would point them at
// the wrong tree.
func TestComposeLoad_AbsolutePathRerootsWorkingDir(t *testing.T) {
	RegisterTestingT(t)

	abs, err := filepath.Abs("testdata/stacks/refapp/docker-compose.yaml")
	Expect(err).ToNot(HaveOccurred())

	cfg, err := ReadDockerCompose(context.Background(), "/nonexistent/working/dir", abs)
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg.Project).ToNot(BeNil())
	Expect(cfg.Project.WorkingDir).To(Equal(filepath.Dir(abs)),
		"workingDir must follow the compose file, not the caller's argument")
	Expect(cfg.Project.Services).To(HaveLen(3))
}

// Interpolation reads the process environment through syscall.Getenv. Loading
// with SkipResolveEnvironment still interpolates ${VAR} in the compose file
// itself, so a broken lookup silently ships the literal placeholder.
func TestComposeLoad_InterpolatesFromProcessEnv(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("SC_COMPOSE_TEST_IMAGE", "mongo:7")

	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yaml")
	Expect(os.WriteFile(file, []byte(`services:
  db:
    image: ${SC_COMPOSE_TEST_IMAGE}
    container_name: interp-db
`), 0o600)).To(Succeed())

	cfg, err := ReadDockerCompose(context.Background(), dir, "docker-compose.yaml")
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg.Project.Services).To(HaveLen(1))
	Expect(cfg.Project.Services[0].Image).To(Equal("mongo:7"))
}

func TestComposeLoad_ReturnsErrorForMissingFile(t *testing.T) {
	RegisterTestingT(t)

	cfg, err := ReadDockerCompose(context.Background(), t.TempDir(), "docker-compose.yaml")
	Expect(err).To(HaveOccurred())
	Expect(cfg.Project).To(BeNil(), "a load failure must not return a half-built project")
}
