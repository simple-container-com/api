package api

import (
	"github.com/samber/lo"
)

type WithCopy[T any] interface {
	Copy() T
}

func (s *SecretsDescriptor) Copy() SecretsDescriptor {
	return SecretsDescriptor{
		SchemaVersion: s.SchemaVersion,
		Auth: lo.MapValues(s.Auth, func(value AuthDescriptor, key string) AuthDescriptor {
			return value.Copy()
		}),
		Values: lo.Assign(map[string]string{}, s.Values),
	}
}

func (s *AuthDescriptor) Copy() AuthDescriptor {
	return AuthDescriptor{
		Type:    s.Type,
		Config:  s.Config.Copy(),
		Inherit: s.Inherit,
	}
}

func (sd *ServerDescriptor) Copy() ServerDescriptor {
	return ServerDescriptor{
		SchemaVersion: sd.SchemaVersion,
		Provisioner:   sd.Provisioner.Copy(),
		Secrets:       sd.Secrets.Copy(),
		CiCd:          sd.CiCd.Copy(),
		Templates: lo.MapValues(sd.Templates, func(value StackDescriptor, key string) StackDescriptor {
			return value.Copy()
		}),
		Resources: sd.Resources.Copy(),
		Variables: lo.MapValues(sd.Variables, func(value VariableDescriptor, key string) VariableDescriptor {
			return value.Copy()
		}),
	}
}

func (s *VariableDescriptor) Copy() VariableDescriptor {
	return VariableDescriptor{
		Type:  s.Type,
		Value: s.Value,
	}
}

func (s *StackDescriptor) Copy() StackDescriptor {
	return StackDescriptor{
		Type:        s.Type,
		Config:      s.Config.Copy(),
		ParentStack: s.ParentStack,
		Inherit:     s.Inherit,
	}
}

func (s *PerStackResourcesDescriptor) Copy() PerStackResourcesDescriptor {
	return PerStackResourcesDescriptor{
		Registrar: s.Registrar.Copy(),
		Resources: lo.MapValues(s.Resources, func(value PerEnvResourcesDescriptor, key string) PerEnvResourcesDescriptor {
			return value.Copy()
		}),
	}
}

func (s *PerEnvResourcesDescriptor) Copy() PerEnvResourcesDescriptor {
	return PerEnvResourcesDescriptor{
		Template: s.Template,
		Resources: lo.MapValues(s.Resources, func(value ResourceDescriptor, key string) ResourceDescriptor {
			return value.Copy()
		}),
		Inherit: s.Inherit,
	}
}

func (s *ResourceDescriptor) Copy() ResourceDescriptor {
	return ResourceDescriptor{
		Type:    s.Type,
		Name:    s.Name,
		Config:  s.Config.Copy(),
		Inherit: s.Inherit,
	}
}

func (s *RegistrarDescriptor) Copy() RegistrarDescriptor {
	return RegistrarDescriptor{
		Type:    s.Type,
		Config:  s.Config.Copy(),
		Inherit: s.Inherit,
	}
}

func (s *SecretsConfigDescriptor) Copy() SecretsConfigDescriptor {
	return SecretsConfigDescriptor{
		Type:    s.Type,
		Config:  s.Config.Copy(),
		Inherit: s.Inherit,
	}
}

func (s *CiCdDescriptor) Copy() CiCdDescriptor {
	return CiCdDescriptor{
		Type:    s.Type,
		Config:  s.Config.Copy(),
		Inherit: s.Inherit,
	}
}

func (s *Config) Copy() Config {
	if withCopy, ok := s.Config.(WithCopy[any]); ok {
		return Config{
			Config: withCopy.Copy(),
		}
	}
	return Config{
		Config: s.Config,
	}
}

func (s *ProvisionerDescriptor) Copy() ProvisionerDescriptor {
	return ProvisionerDescriptor{
		Type:        s.Type,
		Config:      s.Config.Copy(),
		Inherit:     s.Inherit,
		provisioner: s.provisioner,
	}
}

func (s *Stack) ChildStack(name string) Stack {
	return Stack{
		Name:    name,
		Secrets: s.Secrets.Copy(),
		Server:  s.Server.Copy(),
		Client:  s.Client.Copy(),
	}
}

func (s *ClientDescriptor) Copy() ClientDescriptor {
	return ClientDescriptor{
		SchemaVersion: s.SchemaVersion,
		Stacks: lo.MapValues(s.Stacks, func(v StackClientDescriptor, k string) StackClientDescriptor {
			return v.Copy()
		}),
	}
}

func (s *StackClientDescriptor) Copy() StackClientDescriptor {
	return StackClientDescriptor{
		Type:        s.Type,
		ParentStack: s.ParentStack,
		Template:    s.Template,
		Config:      s.Config.Copy(),
	}
}

func (s *StackConfigCompose) Copy() any {
	return &StackConfigCompose{
		DockerComposeFile: s.DockerComposeFile,
		Domain:            s.Domain,
		DomainProxied:     s.DomainProxied,
		BaseDnsZone:       s.BaseDnsZone,
		Uses:              s.Uses,
		Runs:              s.Runs,
		Env:               lo.Assign(s.Env),
		Secrets:           lo.Assign(s.Secrets),
		Version:           s.Version,
		Size:              s.Size,
		Scale:             s.Scale,
		Dependencies:      lo.If(s.Dependencies == nil, []StackConfigDependencyResource{}).Else(s.Dependencies),
		Alerts:            s.Alerts,
		TextVolumes:       s.TextVolumes,
		Headers:           s.Headers,
		LBConfig:          s.LBConfig,
		CloudExtras:       s.CloudExtras,
		StaticEgressIP:    s.StaticEgressIP,
	}
}
