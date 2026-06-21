// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/logger"
)

const (
	testProviderType = "test-prov-api"
	testComposeTpl   = "test-compose-tpl"
	testImageTpl     = "test-image-tpl"
	testStaticTpl    = "test-static-tpl"
)

// registerTestProviders wires up minimal provider/provisioner/converter
// registrations so the Detect*/Prepare* code paths can be exercised without a
// real cloud backend. Uses unique type names to avoid clobbering real ones.
func registerTestProviders() {
	passthrough := func(c *Config) (Config, error) { return *c, nil }
	RegisterProviderConfig(ConfigRegisterMap{
		testProviderType: passthrough,
		testComposeTpl:   passthrough,
		testImageTpl:     passthrough,
		testStaticTpl:    passthrough,
	})
	RegisterProvisioner(ProvisionerRegisterMap{
		testProviderType: func(c Config, opts ...ProvisionerOption) (Provisioner, error) {
			p := &noopProvisioner{}
			for _, o := range opts {
				_ = o(p)
			}
			return p, nil
		},
	})
	RegisterProvisionerFieldConfig(ProvisionerFieldConfigRegister{
		testProviderType: passthrough,
	})
	RegisterCloudSingleImageConverter(CloudSingleImageConfigRegister{
		testImageTpl: func(tpl any, stackCfg *StackConfigSingleImage) (any, error) {
			return map[string]any{"image": stackCfg.Domain}, nil
		},
	})
	RegisterCloudStaticSiteConverter(CloudStaticSiteConfigRegister{
		testStaticTpl: func(tpl any, rootDir, stackName string, stackCfg *StackConfigStatic) (any, error) {
			return map[string]any{"static": stackCfg.BundleDir}, nil
		},
	})
}

func TestConvertDescriptor(t *testing.T) {
	RegisterTestingT(t)

	type target struct {
		Name string `yaml:"name"`
		N    int    `yaml:"n"`
	}
	out, err := ConvertDescriptor(map[string]any{"name": "x", "n": 3}, &target{})
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Name).To(Equal("x"))
	Expect(out.N).To(Equal(3))
}

func TestConvertConfig(t *testing.T) {
	RegisterTestingT(t)

	type target struct {
		Name string `yaml:"name"`
	}
	cfg := &Config{Config: map[string]any{"name": "converted"}}
	res, err := ConvertConfig(cfg, &target{})
	Expect(err).ToNot(HaveOccurred())
	// ConvertConfig rewrites the source Config.Config to the converted value.
	Expect(res.Config).To(BeAssignableToTypeOf(&target{}))
	Expect(cfg.Config.(*target).Name).To(Equal("converted"))
}

func TestConvertAuth(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		fa := &fakeAuth{cred: `{"credentials":"secret-value"}`}
		var creds Credentials
		Expect(ConvertAuth(fa, &creds)).To(Succeed())
		Expect(creds.Credentials).To(Equal("secret-value"))
	})

	t.Run("invalid json", func(t *testing.T) {
		RegisterTestingT(t)
		fa := &fakeAuth{cred: "not-json"}
		Expect(ConvertAuth(fa, &Credentials{})).ToNot(Succeed())
	})
}

func TestAuthToString(t *testing.T) {
	RegisterTestingT(t)
	s := AuthToString(&Credentials{Credentials: "c"})
	Expect(s).To(Equal(`{"credentials":"c"}`))
}

func TestRegisterAndGetProviderConfigs(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	Expect(GetRegisteredProviderConfigs()).To(HaveKey(testProviderType))
	Expect(GetRegisteredProvisionerFieldConfigs()).To(HaveKey(testProviderType))
}

func TestRegisterCloudHelperAndGet(t *testing.T) {
	RegisterTestingT(t)

	const ht = CloudHelperType("test-helper")
	RegisterCloudHelper(CloudHelpersRegisterMap{
		ht: func(opts ...CloudHelperOption) (CloudHelper, error) {
			h := &fakeCloudHelper{}
			for _, o := range opts {
				_ = o(h)
			}
			return h, nil
		},
	})
	Expect(GetRegisteredCloudHelpers()).To(HaveKey(ht))
}

func TestNewCloudHelper(t *testing.T) {
	RegisterTestingT(t)

	const ht = "test-helper-new"
	RegisterCloudHelper(CloudHelpersRegisterMap{
		CloudHelperType(ht): func(opts ...CloudHelperOption) (CloudHelper, error) {
			return &fakeCloudHelper{}, nil
		},
	})

	t.Run("supported", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := NewCloudHelper(ht)
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeNil())
	})

	t.Run("unsupported", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := NewCloudHelper("nope-helper")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not supported"))
	})
}

func TestWithLogger_Option(t *testing.T) {
	RegisterTestingT(t)
	h := &fakeCloudHelper{}
	Expect(WithLogger(logger.New())(h)).To(Succeed())
	Expect(h.logger).ToNot(BeNil())
}

func TestWithFieldConfigReader_Option(t *testing.T) {
	RegisterTestingT(t)
	p := &noopProvisioner{}
	reader := func(cType string, c *Config) (Config, error) { return *c, nil }
	Expect(WithFieldConfigReader(reader)(p)).To(Succeed())
	Expect(p.configReader).ToNot(BeNil())
}

func TestReadServerConfigs_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	desc := &ServerDescriptor{
		SchemaVersion: ServerSchemaVersion,
		Provisioner:   ProvisionerDescriptor{Type: testProviderType},
		Secrets:       SecretsConfigDescriptor{Type: testProviderType},
		CiCd:          CiCdDescriptor{Type: testProviderType},
		Templates: map[string]StackDescriptor{
			"tpl": {Type: testProviderType},
		},
		Resources: PerStackResourcesDescriptor{
			Registrar: RegistrarDescriptor{Type: testProviderType},
			Resources: map[string]PerEnvResourcesDescriptor{
				"prod": {Resources: map[string]ResourceDescriptor{
					"db": {Type: testProviderType, Name: "db"},
				}},
			},
		},
	}

	out, err := ReadServerConfigs(desc)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Provisioner.GetProvisioner()).ToNot(BeNil())
}

func TestReadServerConfigs_NilDescriptor(t *testing.T) {
	RegisterTestingT(t)
	_, err := ReadServerConfigs(nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("reference is nil"))
}

func TestDetect_UnknownTypes(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{"provisioner", func() error {
			_, err := DetectProvisionerType(&ServerDescriptor{Provisioner: ProvisionerDescriptor{Type: "ghost"}})
			return err
		}, "unknown provisioner type"},
		{"secrets", func() error {
			_, err := DetectSecretsType(&ServerDescriptor{Secrets: SecretsConfigDescriptor{Type: "ghost"}})
			return err
		}, "unknown secrets type"},
		{"cicd", func() error {
			_, err := DetectCiCdType(&ServerDescriptor{CiCd: CiCdDescriptor{Type: "ghost"}})
			return err
		}, "unknown cicd type"},
		{"registrar", func() error {
			_, err := DetectRegistrarType(&PerStackResourcesDescriptor{Registrar: RegistrarDescriptor{Type: "ghost"}})
			return err
		}, "unknown registrar type"},
		{"template", func() error {
			_, err := DetectTemplateType(StackDescriptor{Type: "ghost"})
			return err
		}, "unknown template type"},
		{"auth", func() error {
			_, err := DetectAuthProvider(&AuthDescriptor{Type: "ghost"})
			return err
		}, "unknown auth type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.run()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.want))
		})
	}
}

func TestDetect_InheritedSkips(t *testing.T) {
	RegisterTestingT(t)

	// Inherited descriptors short-circuit detection (no type lookup needed).
	d := &ServerDescriptor{Provisioner: ProvisionerDescriptor{Inherit: Inherit{Inherit: "base"}}}
	out, err := DetectProvisionerType(d)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Provisioner.GetProvisioner()).To(BeNil())

	sec := &ServerDescriptor{Secrets: SecretsConfigDescriptor{Inherit: Inherit{Inherit: "base"}}}
	_, err = DetectSecretsType(sec)
	Expect(err).ToNot(HaveOccurred())

	cic := &ServerDescriptor{CiCd: CiCdDescriptor{Inherit: Inherit{Inherit: "base"}}}
	_, err = DetectCiCdType(cic)
	Expect(err).ToNot(HaveOccurred())
}

func TestDetectAuthType_InheritedWithType_Errors(t *testing.T) {
	RegisterTestingT(t)
	d := &SecretsDescriptor{Auth: map[string]AuthDescriptor{
		"a": {Type: "x", Inherit: Inherit{Inherit: "base"}},
	}}
	_, err := DetectAuthType(d)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("is inherited, but type"))
}

func TestReadProvisionerFieldConfig(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	out, err := ReadProvisionerFieldConfig(testProviderType, &Config{Config: "x"})
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Config).To(Equal("x"))

	_, err = ReadProvisionerFieldConfig("ghost-field", &Config{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unknown provisioner field config type"))
}

func TestMarshalDescriptor(t *testing.T) {
	RegisterTestingT(t)
	b, err := MarshalDescriptor(&ConfigFile{ProjectName: "p"})
	Expect(err).ToNot(HaveOccurred())
	Expect(string(b)).To(ContainSubstring("projectName: p"))
}

func TestReadServerDescriptor_File(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	dir := t.TempDir()
	p := filepath.Join(dir, ServerDescriptorFileName)
	yaml := "schemaVersion: \"1.0\"\nprovisioner:\n  type: " + testProviderType + "\n"
	Expect(os.WriteFile(p, []byte(yaml), 0o644)).To(Succeed())

	out, err := ReadServerDescriptor(p)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Provisioner.Type).To(Equal(testProviderType))
	Expect(out.Provisioner.GetProvisioner()).ToNot(BeNil())

	_, err = ReadServerDescriptor(filepath.Join(dir, "missing.yaml"))
	Expect(err).To(HaveOccurred())
}

func TestReadSecretsDescriptor_File(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()

	dir := t.TempDir()
	p := filepath.Join(dir, SecretsDescriptorFileName)
	yaml := "schemaVersion: \"1.0\"\nauth:\n  default:\n    type: " + testProviderType + "\n"
	Expect(os.WriteFile(p, []byte(yaml), 0o644)).To(Succeed())

	out, err := ReadSecretsDescriptor(p)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Auth).To(HaveKey("default"))

	_, err = ReadSecretsDescriptor(filepath.Join(dir, "missing.yaml"))
	Expect(err).To(HaveOccurred())
}

func TestReadClientDescriptor_File(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	p := filepath.Join(dir, ClientDescriptorFileName)
	yaml := "schemaVersion: \"1.0\"\nstacks:\n  prod:\n    type: " + ClientTypeStatic + "\n    bundleDir: ./dist\n"
	Expect(os.WriteFile(p, []byte(yaml), 0o644)).To(Succeed())

	out, err := ReadClientDescriptor(p)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Stacks).To(HaveKey("prod"))
	Expect(out.Stacks["prod"].Config.Config).To(BeAssignableToTypeOf(&StackConfigStatic{}))

	_, err = ReadClientDescriptor(filepath.Join(dir, "missing.yaml"))
	Expect(err).To(HaveOccurred())
}

func TestConvertClientConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("static", func(t *testing.T) {
		RegisterTestingT(t)
		desc := StackClientDescriptor{Type: ClientTypeStatic, Config: Config{Config: map[string]any{"bundleDir": "d"}}}
		out, err := ConvertClientConfig(desc)
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Config.Config).To(BeAssignableToTypeOf(&StackConfigStatic{}))
	})

	t.Run("unsupported type", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ConvertClientConfig(StackClientDescriptor{Type: "ghost"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported client config type"))
	})
}

func TestPrepareForDeploy(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()
	ctx := context.Background()

	t.Run("single image happy path", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := StackDescriptor{Type: testImageTpl}
		out, err := PrepareCloudSingleImageForDeploy(ctx, "/dir", "stk", tpl, &StackConfigSingleImage{Domain: "x.io"}, "parent")
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Type).To(Equal(testImageTpl))
		Expect(out.ParentStack).To(Equal("parent"))
	})

	t.Run("single image incompatible template", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PrepareCloudSingleImageForDeploy(ctx, "/dir", "stk", StackDescriptor{Type: testProviderType}, &StackConfigSingleImage{}, "p")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incompatible server template type"))
	})

	t.Run("static happy path", func(t *testing.T) {
		RegisterTestingT(t)
		out, err := PrepareStaticForDeploy(ctx, "/dir", "stk", StackDescriptor{Type: testStaticTpl}, &StackConfigStatic{BundleDir: "d"}, "parent")
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Type).To(Equal(testStaticTpl))
	})

	t.Run("static incompatible", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PrepareStaticForDeploy(ctx, "/dir", "stk", StackDescriptor{Type: testProviderType}, &StackConfigStatic{}, "p")
		Expect(err).To(HaveOccurred())
	})

	t.Run("compose missing file errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PrepareCloudComposeForDeploy(ctx, t.TempDir(), "stk", StackDescriptor{Type: testComposeTpl}, &StackConfigCompose{DockerComposeFile: "nope.yml"}, "p")
		Expect(err).To(HaveOccurred())
	})

	t.Run("unknown template type", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PrepareCloudSingleImageForDeploy(ctx, "/dir", "stk", StackDescriptor{Type: "ghost"}, &StackConfigSingleImage{}, "p")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown template type"))
	})
}

func TestPrepareClientConfigForDeploy(t *testing.T) {
	RegisterTestingT(t)
	registerTestProviders()
	ctx := context.Background()

	t.Run("static via prepare map", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := StackDescriptor{Type: testStaticTpl}
		clientDesc := StackClientDescriptor{Type: ClientTypeStatic, Config: Config{Config: &StackConfigStatic{BundleDir: "d"}}}
		out, err := PrepareClientConfigForDeploy(ctx, "/dir", "stk", tpl, clientDesc)
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Type).To(Equal(testStaticTpl))
	})

	t.Run("unsupported client type", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PrepareClientConfigForDeploy(ctx, "/dir", "stk", StackDescriptor{}, StackClientDescriptor{Type: "ghost"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported client type"))
	})

	t.Run("wrong config type for client type", func(t *testing.T) {
		RegisterTestingT(t)
		// ClientTypeStatic but Config holds a non-static struct -> type assertion fails.
		_, err := PrepareClientConfigForDeploy(ctx, "/dir", "stk", StackDescriptor{Type: testStaticTpl},
			StackClientDescriptor{Type: ClientTypeStatic, Config: Config{Config: &StackConfigCompose{}}})
		Expect(err).To(HaveOccurred())
	})
}
