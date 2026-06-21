// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// fullStack builds a deeply-populated Stack covering every descriptor that has
// a Copy() method, so the deep-copy assertions exercise the whole tree.
func fullStack() Stack {
	compose := &StackConfigCompose{
		DockerComposeFile: "docker-compose.yml",
		Env:               map[string]string{"K": "V"},
	}
	return Stack{
		Name: "web",
		Secrets: SecretsDescriptor{
			SchemaVersion: SecretsSchemaVersion,
			Auth: map[string]AuthDescriptor{
				"gcp": {Type: "gcp-sa", Config: Config{Config: "creds"}, Inherit: Inherit{Inherit: ""}},
			},
			Values: map[string]string{"TOKEN": "abc"},
		},
		Server: ServerDescriptor{
			SchemaVersion: ServerSchemaVersion,
			Provisioner:   ProvisionerDescriptor{Type: "pulumi", Config: Config{Config: "pcfg"}},
			Secrets:       SecretsConfigDescriptor{Type: "gcp-bucket", Config: Config{Config: "scfg"}},
			CiCd:          CiCdDescriptor{Type: "github-actions", Config: Config{Config: "ccfg"}},
			Templates: map[string]StackDescriptor{
				"tpl": {Type: "aws-ecs", ParentStack: "parent", Config: Config{Config: "tcfg"}},
			},
			Resources: PerStackResourcesDescriptor{
				Registrar: RegistrarDescriptor{Type: "cloudflare", Config: Config{Config: "rcfg"}},
				Resources: map[string]PerEnvResourcesDescriptor{
					"prod": {
						Template: "tpl",
						Resources: map[string]ResourceDescriptor{
							"db": {Type: "postgres", Name: "db", Config: Config{Config: "dbcfg"}},
						},
					},
				},
			},
			Variables: map[string]VariableDescriptor{
				"region": {Type: "string", Value: "eu"},
			},
		},
		Client: ClientDescriptor{
			SchemaVersion: ClientSchemaVersion,
			Defaults:      map[string]interface{}{"d": 1},
			Stacks: map[string]StackClientDescriptor{
				"prod": {Type: ClientTypeCloudCompose, ParentStack: "p", ParentEnv: "prod", Template: "tpl", Config: Config{Config: compose}},
			},
		},
	}
}

func TestDescriptorCopy_DeepTree(t *testing.T) {
	RegisterTestingT(t)

	orig := fullStack()

	secrets := orig.Secrets.Copy()
	Expect(secrets.SchemaVersion).To(Equal(SecretsSchemaVersion))
	Expect(secrets.Auth).To(HaveKey("gcp"))
	Expect(secrets.Auth["gcp"].Type).To(Equal("gcp-sa"))
	Expect(secrets.Values).To(HaveKeyWithValue("TOKEN", "abc"))

	// Mutating the original maps must not affect the copy.
	orig.Secrets.Values["TOKEN"] = "MUT"
	orig.Secrets.Auth["new"] = AuthDescriptor{Type: "x"}
	Expect(secrets.Values["TOKEN"]).To(Equal("abc"))
	Expect(secrets.Auth).ToNot(HaveKey("new"))

	server := orig.Server.Copy()
	Expect(server.Provisioner.Type).To(Equal("pulumi"))
	Expect(server.Secrets.Type).To(Equal("gcp-bucket"))
	Expect(server.CiCd.Type).To(Equal("github-actions"))
	Expect(server.Templates).To(HaveKey("tpl"))
	Expect(server.Templates["tpl"].ParentStack).To(Equal("parent"))
	Expect(server.Resources.Registrar.Type).To(Equal("cloudflare"))
	Expect(server.Resources.Resources["prod"].Resources["db"].Name).To(Equal("db"))
	Expect(server.Variables["region"].Value).To(Equal("eu"))

	client := orig.Client.Copy()
	Expect(client.SchemaVersion).To(Equal(ClientSchemaVersion))
	Expect(client.Defaults).To(HaveKey("d"))
	Expect(client.Stacks).To(HaveKey("prod"))
	Expect(client.Stacks["prod"].ParentEnv).To(Equal("prod"))

	// Config with a WithCopy implementation is deep-copied.
	copiedCompose := client.Stacks["prod"].Config.Config.(*StackConfigCompose)
	origCompose := orig.Client.Stacks["prod"].Config.Config.(*StackConfigCompose)
	Expect(copiedCompose).ToNot(BeIdenticalTo(origCompose))
	Expect(copiedCompose.DockerComposeFile).To(Equal("docker-compose.yml"))
}

func TestConfig_Copy(t *testing.T) {
	RegisterTestingT(t)

	t.Run("with WithCopy config", func(t *testing.T) {
		RegisterTestingT(t)
		c := Config{Config: &StackConfigCompose{Domain: "x.io", Env: map[string]string{"A": "B"}}}
		cp := c.Copy()
		Expect(cp.Config).ToNot(BeIdenticalTo(c.Config))
		Expect(cp.Config.(*StackConfigCompose).Domain).To(Equal("x.io"))
	})

	t.Run("without WithCopy config keeps reference", func(t *testing.T) {
		RegisterTestingT(t)
		c := Config{Config: "plain-string"}
		cp := c.Copy()
		Expect(cp.Config).To(Equal("plain-string"))
	})

	t.Run("nil config", func(t *testing.T) {
		RegisterTestingT(t)
		c := Config{}
		Expect(c.Copy().Config).To(BeNil())
	})
}

func TestProvisionerDescriptor_Copy_PreservesProvisioner(t *testing.T) {
	RegisterTestingT(t)

	p := &noopProvisioner{}
	pd := ProvisionerDescriptor{Type: "pulumi", Config: Config{Config: "c"}}
	pd.SetProvisioner(p)

	cp := pd.Copy()
	Expect(cp.Type).To(Equal("pulumi"))
	Expect(cp.GetProvisioner()).To(BeIdenticalTo(p))
}

func TestStack_ChildStack(t *testing.T) {
	RegisterTestingT(t)

	parent := fullStack()
	child := parent.ChildStack("child")
	Expect(child.Name).To(Equal("child"))
	Expect(child.Secrets.Values).To(HaveKeyWithValue("TOKEN", "abc"))
	Expect(child.Server.Provisioner.Type).To(Equal("pulumi"))
	Expect(child.Client.Stacks).To(HaveKey("prod"))
}

func TestStack_ValuesOnly(t *testing.T) {
	RegisterTestingT(t)

	s := fullStack()
	s.Server.Provisioner.SetProvisioner(&noopProvisioner{})

	vo := s.ValuesOnly()
	Expect(vo.Name).To(Equal("web"))
	// ValuesOnly strips the provisioner reference.
	Expect(vo.Server.Provisioner.GetProvisioner()).To(BeNil())
	Expect(vo.Server.Provisioner.Type).To(Equal("pulumi"))
}

func TestServerDescriptor_ValuesOnly(t *testing.T) {
	RegisterTestingT(t)

	sd := fullStack().Server
	sd.Provisioner.SetProvisioner(&noopProvisioner{})
	vo := sd.ValuesOnly()
	Expect(vo.SchemaVersion).To(Equal(ServerSchemaVersion))
	Expect(vo.Provisioner.GetProvisioner()).To(BeNil())
	Expect(vo.CiCd.Type).To(Equal("github-actions"))
}

func TestProvisionerDescriptor_GetSetProvisioner(t *testing.T) {
	RegisterTestingT(t)

	pd := &ProvisionerDescriptor{}
	Expect(pd.GetProvisioner()).To(BeNil())
	p := &noopProvisioner{}
	pd.SetProvisioner(p)
	Expect(pd.GetProvisioner()).To(BeIdenticalTo(p))
}

func TestInherit_IsInherited(t *testing.T) {
	RegisterTestingT(t)
	Expect(Inherit{Inherit: "x"}.IsInherited()).To(BeTrue())
	Expect(Inherit{}.IsInherited()).To(BeFalse())
	Expect((&CiCdDescriptor{Inherit: Inherit{Inherit: "y"}}).IsInherited()).To(BeTrue())
}

func TestResourceInput_ToResName(t *testing.T) {
	cases := []struct {
		name      string
		env       string
		parentEnv string
		resName   string
		want      string
	}{
		{"no env", "", "", "db", "db"},
		{"with env", "prod", "", "db", "db--prod"},
		{"parent env overrides", "prod", "staging", "db", "db--staging"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			ri := &ResourceInput{StackParams: &StackParams{Environment: tc.env, ParentEnv: tc.parentEnv}}
			Expect(ri.ToResName(tc.resName)).To(Equal(tc.want))
		})
	}
}

func TestAuthDescriptor_AuthConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("config implements AuthConfig", func(t *testing.T) {
		RegisterTestingT(t)
		a := &AuthDescriptor{Type: "fake", Config: Config{Config: &fakeAuth{cred: "x"}}}
		ac, err := a.AuthConfig()
		Expect(err).ToNot(HaveOccurred())
		Expect(ac.CredentialsValue()).To(Equal("x"))
	})

	t.Run("config does not implement AuthConfig", func(t *testing.T) {
		RegisterTestingT(t)
		a := &AuthDescriptor{Type: "bad", Config: Config{Config: "not-auth"}}
		_, err := a.AuthConfig()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not implement AuthConfig"))
	})
}

func TestStacksMap_ResolveInheritance(t *testing.T) {
	RegisterTestingT(t)

	base := Stack{
		Name: "base",
		Server: ServerDescriptor{
			Provisioner: ProvisionerDescriptor{Type: "pulumi"},
			Resources:   PerStackResourcesDescriptor{Registrar: RegistrarDescriptor{Type: "cloudflare"}},
			CiCd:        CiCdDescriptor{Type: "github-actions"},
			Secrets:     SecretsConfigDescriptor{Type: "gcp-bucket"},
		},
		Secrets: SecretsDescriptor{Values: map[string]string{"S": "1"}},
	}
	child := Stack{
		Name: "child",
		Server: ServerDescriptor{
			Provisioner: ProvisionerDescriptor{Inherit: Inherit{Inherit: "base"}},
			Resources:   PerStackResourcesDescriptor{Registrar: RegistrarDescriptor{Inherit: Inherit{Inherit: "base"}}},
			CiCd:        CiCdDescriptor{Inherit: Inherit{Inherit: "base"}},
			Secrets:     SecretsConfigDescriptor{Inherit: Inherit{Inherit: "base"}},
		},
	}
	m := StacksMap{"base": base, "child": child}

	resolved := *m.ResolveInheritance()
	Expect(resolved["child"].Server.Provisioner.Type).To(Equal("pulumi"))
	Expect(resolved["child"].Server.Resources.Registrar.Type).To(Equal("cloudflare"))
	Expect(resolved["child"].Server.CiCd.Type).To(Equal("github-actions"))
	Expect(resolved["child"].Server.Secrets.Type).To(Equal("gcp-bucket"))
	Expect(resolved["child"].Secrets.Values).To(HaveKeyWithValue("S", "1"))
}

func TestStacksMap_ReconcileForDeploy(t *testing.T) {
	RegisterTestingT(t)

	parent := Stack{
		Name: "parent",
		Server: ServerDescriptor{
			Provisioner: ProvisionerDescriptor{Type: "pulumi"},
		},
		Secrets: SecretsDescriptor{Values: map[string]string{"P": "1"}},
	}
	child := Stack{
		Name: "child",
		Client: ClientDescriptor{
			Stacks: map[string]StackClientDescriptor{
				"prod": {Type: ClientTypeCloudCompose, ParentStack: "org/repo/parent"},
			},
		},
	}
	m := StacksMap{"parent": parent, "child": child}

	t.Run("happy path inherits parent server+secrets", func(t *testing.T) {
		RegisterTestingT(t)
		out, err := m.ReconcileForDeploy(StackParams{StackName: "child", Environment: "prod"})
		Expect(err).ToNot(HaveOccurred())
		Expect((*out)["child"].Server.Provisioner.Type).To(Equal("pulumi"))
		Expect((*out)["child"].Secrets.Values).To(HaveKeyWithValue("P", "1"))
	})

	t.Run("missing parent stack errors", func(t *testing.T) {
		RegisterTestingT(t)
		bad := StacksMap{"child": {
			Name: "child",
			Client: ClientDescriptor{Stacks: map[string]StackClientDescriptor{
				"prod": {ParentStack: "ghost"},
			}},
		}}
		_, err := bad.ReconcileForDeploy(StackParams{StackName: "child", Environment: "prod"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parent stack"))
	})

	t.Run("env not configured for target errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := m.ReconcileForDeploy(StackParams{StackName: "child", Environment: "missing-env"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not configured"))
	})
}
