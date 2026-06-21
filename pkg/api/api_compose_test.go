// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/compose"
)

func TestPrepareCloudComposeForDeploy_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	// Register a compose converter for the test template type.
	RegisterCloudComposeConverter(CloudComposeConfigRegister{
		testComposeTpl: func(tpl any, composeCfg compose.Config, stackCfg *StackConfigCompose) (any, error) {
			return map[string]any{
				"services": len(composeCfg.Project.Services),
				"domain":   stackCfg.Domain,
			}, nil
		},
	})

	dir := t.TempDir()
	composeYaml := "services:\n  web:\n    image: nginx\n"
	Expect(os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeYaml), 0o644)).To(Succeed())

	tpl := StackDescriptor{Type: testComposeTpl}
	clientCfg := &StackConfigCompose{DockerComposeFile: "docker-compose.yml", Domain: "x.io"}

	out, err := PrepareCloudComposeForDeploy(context.Background(), dir, "stk", tpl, clientCfg, "parent")
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Type).To(Equal(testComposeTpl))
	Expect(out.ParentStack).To(Equal("parent"))
}

func TestPrepareCloudComposeForDeploy_IncompatibleTemplate(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	dir := t.TempDir()
	Expect(os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644)).To(Succeed())

	// testProviderType has no compose converter registered -> incompatible.
	_, err := PrepareCloudComposeForDeploy(context.Background(), dir, "stk",
		StackDescriptor{Type: testProviderType},
		&StackConfigCompose{DockerComposeFile: "docker-compose.yml"}, "p")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("incompatible server template type"))
}

func TestWriteConfigFile_Error(t *testing.T) {
	RegisterTestingT(t)

	// The .sc directory does not exist under the temp dir, so the write fails.
	cf := &ConfigFile{ProjectName: "p"}
	err := cf.WriteConfigFile(t.TempDir(), "dev")
	Expect(err).To(HaveOccurred())
}
