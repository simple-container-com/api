// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type capturingMocks struct {
	mu     sync.Mutex
	inputs map[string]resource.PropertyMap
}

func (m *capturingMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs[args.Name] = args.Inputs
	return args.Name, args.Inputs, nil
}

func (m *capturingMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// Pulumi prints a command's create text in every preview diff. The registry
// token must not be in it, and the environment value that carries it must be
// secret so the diff shows [secret].
func TestRegistryLoginKeepsTheTokenOutOfTheCommand(t *testing.T) {
	RegisterTestingT(t)
	const token = "ya29.not-a-real-access-token"
	auth := base64.StdEncoding.EncodeToString([]byte("oauth2accesstoken:" + token))
	mocks := &capturingMocks{inputs: map[string]resource.PropertyMap{}}

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := createRegistryLogin(ctx, Image{Registry: docker.RegistryArgs{
			Server:   sdk.String("europe-docker.pkg.dev/project/repo"),
			Username: sdk.String("oauth2accesstoken"),
			Password: sdk.String(token),
		}}, "web", nil)
		return err
	}, sdk.WithMocks("project", "stack", mocks))
	Expect(err).NotTo(HaveOccurred())

	in, ok := mocks.inputs["registry-login-web"]
	Expect(ok).To(BeTrue(), "no registry-login command was created")
	create := in["create"].StringValue()
	Expect(create).To(Equal(registryLoginScript))
	Expect(create).NotTo(ContainSubstring(token))
	Expect(create).NotTo(ContainSubstring(auth))

	env := in["environment"].ObjectValue()
	Expect(env[registryServerEnv].StringValue()).To(Equal("europe-docker.pkg.dev/project/repo"))
	authValue := env[registryAuthEnv]
	Expect(authValue.IsSecret()).To(BeTrue(), "the registry auth is not secret: %v", authValue)
	Expect(authValue.SecretValue().Element.StringValue()).To(Equal(auth))
}

func TestRegistryLoginWarnsWithoutAPassword(t *testing.T) {
	RegisterTestingT(t)
	Expect(registryLoginCommand("")).To(Equal(emptyRegistryPasswordWarning))
	Expect(registryAuth("", "p")).To(Equal(base64.StdEncoding.EncodeToString([]byte("_token:p"))))
}

// The script reads the credentials from the environment and writes a docker
// config that registry clients accept, both into a new file and merged into an
// existing one.
func TestRegistryLoginScriptWritesDockerConfig(t *testing.T) {
	RegisterTestingT(t)
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatalf("jq is required to test the merge path: %v", err)
	}
	const server = "europe-docker.pkg.dev/project/repo"
	auth := registryAuth("oauth2accesstoken", `tok"en'with$chars`)

	run := func(home string) map[string]map[string]string {
		cmd := exec.Command("sh", "-c", registryLoginScript)
		cmd.Env = append(os.Environ(), "HOME="+home, registryServerEnv+"="+server, registryAuthEnv+"="+auth)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
		data, err := os.ReadFile(filepath.Join(home, ".docker", "config.json"))
		Expect(err).NotTo(HaveOccurred())
		var cfg struct {
			Auths map[string]map[string]string `json:"auths"`
		}
		Expect(json.Unmarshal(data, &cfg)).To(Succeed(), string(data))
		return cfg.Auths
	}

	fresh := t.TempDir()
	auths := run(fresh)
	Expect(auths).To(HaveKeyWithValue(server, HaveKeyWithValue("auth", auth)))

	existing := t.TempDir()
	Expect(os.MkdirAll(filepath.Join(existing, ".docker"), 0o700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(existing, ".docker", "config.json"),
		[]byte(`{"auths":{"ghcr.io":{"auth":"b3RoZXI="}}}`), 0o600)).To(Succeed())
	auths = run(existing)
	Expect(auths).To(HaveKeyWithValue(server, HaveKeyWithValue("auth", auth)))
	Expect(auths).To(HaveKeyWithValue("ghcr.io", HaveKeyWithValue("auth", "b3RoZXI=")))
	Expect(strings.Contains(registryLoginScript, auth)).To(BeFalse())
}
