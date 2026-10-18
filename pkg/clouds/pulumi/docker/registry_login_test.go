// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package docker

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

// plain returns the value under any secret wrapper.
func plain(v resource.PropertyValue) resource.PropertyValue {
	for v.IsSecret() {
		v = v.SecretValue().Element
	}
	return v
}

const registryServer = "europe-docker.pkg.dev/project/repo"

// loginInputs runs createRegistryLogin with the given password and returns the
// inputs the registry-login command was registered with.
func loginInputs(t *testing.T, password sdk.StringPtrInput) resource.PropertyMap {
	t.Helper()
	mocks := &capturingMocks{inputs: map[string]resource.PropertyMap{}}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := createRegistryLogin(ctx, Image{Registry: docker.RegistryArgs{
			Server:   sdk.String(registryServer),
			Username: sdk.String("oauth2accesstoken"),
			Password: password,
		}}, "web", nil)
		return err
	}, sdk.WithMocks("project", "stack", mocks))
	Expect(err).NotTo(HaveOccurred())
	in, ok := mocks.inputs["registry-login-web"]
	Expect(ok).To(BeTrue(), "no registry-login command was created")
	return in
}

// Pulumi prints a command's create text in every preview diff. The registry
// token must not be in it, and the environment value that carries it must be
// secret so the diff shows [secret]. This holds whether the password arrives
// plain, as a pointer, or already secret.
func TestRegistryLoginKeepsTheTokenOutOfTheCommand(t *testing.T) {
	RegisterTestingT(t)
	const token = "ya29.not-a-real-access-token"
	auth := base64.StdEncoding.EncodeToString([]byte("oauth2accesstoken:" + token))
	for name, password := range map[string]sdk.StringPtrInput{
		"plain":   sdk.String(token),
		"pointer": sdk.StringPtr(token),
		"secret":  sdk.ToSecret(sdk.String(token)).(sdk.StringOutput).ToStringPtrOutput(),
	} {
		in := loginInputs(t, password)
		Expect(in["create"].IsSecret()).To(BeFalse(), "%s: the command text is hidden in diffs", name)
		create := plain(in["create"]).StringValue()
		Expect(create).To(Equal(registryLoginScript), name)
		Expect(create).NotTo(ContainSubstring(token), name)
		Expect(create).NotTo(ContainSubstring(auth), name)

		env := plain(in["environment"]).ObjectValue()
		Expect(env[registryServerEnv].IsSecret()).To(BeFalse(), "%s: the server is hidden in diffs", name)
		Expect(plain(env[registryServerEnv]).StringValue()).To(Equal(registryServer), name)
		Expect(env[registryAuthEnv].IsSecret()).To(BeTrue(), "%s: the registry auth is not secret", name)
		Expect(plain(env[registryAuthEnv]).StringValue()).To(Equal(auth), name)
	}
}

// A new token changes only the secret environment value, never the command
// text, so the diff of a routine deploy shows no credential.
func TestRegistryLoginCommandIsTheSameForEveryToken(t *testing.T) {
	RegisterTestingT(t)
	first := loginInputs(t, sdk.String("token-one"))
	second := loginInputs(t, sdk.String("token-two"))
	Expect(plain(first["create"]).StringValue()).To(Equal(plain(second["create"]).StringValue()))
	authOf := func(in resource.PropertyMap) string {
		return plain(plain(in["environment"]).ObjectValue()[registryAuthEnv]).StringValue()
	}
	Expect(authOf(first)).NotTo(Equal(authOf(second)))
}

func TestRegistryLoginWarnsWithoutAPassword(t *testing.T) {
	RegisterTestingT(t)
	in := loginInputs(t, nil)
	Expect(plain(in["create"]).StringValue()).To(Equal(emptyRegistryPasswordWarning))
	Expect(registryAuth("", "p")).To(Equal(base64.StdEncoding.EncodeToString([]byte("_token:p"))))
}

// The docker provider stores registry.password as given; SecretRegistry makes
// it a secret so the token is encrypted in state.
func TestSecretRegistryEncryptsThePasswordInState(t *testing.T) {
	RegisterTestingT(t)
	mocks := &capturingMocks{inputs: map[string]resource.PropertyMap{}}
	err := sdk.RunErr(func(ctx *sdk.Context) error {
		_, err := docker.NewImage(ctx, "img", &docker.ImageArgs{
			ImageName: sdk.String(registryServer + "/img:v1"),
			Registry: SecretRegistry(docker.RegistryArgs{
				Server:   sdk.String(registryServer),
				Username: sdk.String("oauth2accesstoken"),
				Password: sdk.String("ya29.token"),
			}),
		})
		return err
	}, sdk.WithMocks("project", "stack", mocks))
	Expect(err).NotTo(HaveOccurred())
	reg := plain(mocks.inputs["img"]["registry"]).ObjectValue()
	Expect(reg["password"].IsSecret()).To(BeTrue(), "registry.password is not secret: %v", reg["password"])
	Expect(plain(reg["password"]).StringValue()).To(Equal("ya29.token"))
	Expect(reg["server"].IsSecret()).To(BeFalse())

	Expect(SecretRegistry(docker.RegistryArgs{Server: sdk.String(registryServer)}).Password).To(BeNil())
}

type dockerAuths map[string]map[string]string

func readAuths(path string) dockerAuths {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	var cfg struct {
		Auths dockerAuths `json:"auths"`
	}
	Expect(json.Unmarshal(data, &cfg)).To(Succeed(), string(data))
	return cfg.Auths
}

// runLoginScript runs the real script with a temporary HOME and fallback home.
func runLoginScript(t *testing.T, server, auth, path string) (home, fallback, out string) {
	home, fallback = t.TempDir(), t.TempDir()
	return home, fallback, runLoginScriptIn(home, fallback, server, auth, path)
}

func runLoginScriptIn(home, fallback, server, auth, path string) string {
	cmd := exec.Command("sh", "-c", registryLoginScript)
	cmd.Env = []string{"PATH=" + path, "HOME=" + home, registryServerEnv + "=" + server,
		registryAuthEnv + "=" + auth, registryFallbackHomeEnv + "=" + fallback}
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))
	return string(out)
}

// The script writes a docker config that registry clients accept: into a new
// file and the fallback home, merged into an existing one, and without jq.
func TestRegistryLoginScriptWritesDockerConfig(t *testing.T) {
	auth := registryAuth("oauth2accesstoken", `tok"en'with$chars`)
	path := os.Getenv("PATH")
	needJq := func(t *testing.T) {
		if _, err := exec.LookPath("jq"); err != nil {
			t.Skipf("jq is not installed, so its path cannot run here: %v", err)
		}
	}

	t.Run("new file and fallback home", func(t *testing.T) {
		RegisterTestingT(t)
		needJq(t)
		home, fallback, _ := runLoginScript(t, registryServer, auth, path)
		Expect(readAuths(filepath.Join(home, ".docker", "config.json"))).To(HaveKeyWithValue(registryServer, HaveKeyWithValue("auth", auth)))
		Expect(readAuths(filepath.Join(fallback, ".docker", "config.json"))).To(HaveKeyWithValue(registryServer, HaveKeyWithValue("auth", auth)))
	})

	t.Run("merged into an existing config", func(t *testing.T) {
		RegisterTestingT(t)
		needJq(t)
		home, fallback := t.TempDir(), t.TempDir()
		Expect(os.MkdirAll(filepath.Join(home, ".docker"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(home, ".docker", "config.json"),
			[]byte(`{"auths":{"ghcr.io":{"auth":"b3RoZXI="}}}`), 0o600)).To(Succeed())
		runLoginScriptIn(home, fallback, registryServer, auth, path)
		auths := readAuths(filepath.Join(home, ".docker", "config.json"))
		Expect(auths).To(HaveKeyWithValue(registryServer, HaveKeyWithValue("auth", auth)))
		Expect(auths).To(HaveKeyWithValue("ghcr.io", HaveKeyWithValue("auth", "b3RoZXI=")))
	})

	t.Run("without jq", func(t *testing.T) {
		RegisterTestingT(t)
		noJq := t.TempDir()
		for _, tool := range []string{"sh", "mkdir", "mv"} {
			p, err := exec.LookPath(tool)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Symlink(p, filepath.Join(noJq, tool))).To(Succeed())
		}
		home, fallback, _ := runLoginScript(t, registryServer, auth, noJq)
		Expect(readAuths(filepath.Join(home, ".docker", "config.json"))).To(HaveKeyWithValue(registryServer, HaveKeyWithValue("auth", auth)))
		Expect(readAuths(filepath.Join(fallback, ".docker", "config.json"))).To(HaveKeyWithValue(registryServer, HaveKeyWithValue("auth", auth)))
	})

	t.Run("empty server writes nothing", func(t *testing.T) {
		RegisterTestingT(t)
		home, fallback, out := runLoginScript(t, "", auth, path)
		Expect(out).To(ContainSubstring("registry server is empty"))
		Expect(filepath.Join(home, ".docker", "config.json")).NotTo(BeAnExistingFile())
		Expect(filepath.Join(fallback, ".docker", "config.json")).NotTo(BeAnExistingFile())
	})
}
