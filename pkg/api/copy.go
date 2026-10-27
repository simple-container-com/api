// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/samber/lo"
)

type WithCopy[T any] interface {
	Copy() T
}

// mapValues is lo.MapValues with the nil-vs-empty distinction kept.
//
// lo.MapValues always allocates, so an absent map comes back empty-but-present and a
// copy stops comparing equal to its original. That is not cosmetic: Copy is the
// parent-descriptor inheritance path (ChildStack), so a config field whose nil means
// "unset, apply the default" would arrive at every child stack already set to nothing
// — the same failure the placeholder walker used to cause for every parsed config
// (provisioner/placeholders/placeholders.go).
//
// Not every map here can preserve nil: Templates, Secrets.Values and Client.Defaults
// are all written to AFTER a copy (api/models.go:126, api/read.go:316,
// provisioner/provision.go:259, api/client.go:81), and a nil map is not writable.
// Those deliberately keep allocating; the ones below have no such writer.
func mapValues[K comparable, V, R any](in map[K]V, fn func(V, K) R) map[K]R {
	if in == nil {
		return nil
	}
	return lo.MapValues(in, fn)
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
		Variables: mapValues(sd.Variables, func(value VariableDescriptor, key string) VariableDescriptor {
			return value.Copy()
		}),
		// Plugins MUST be copied. A child stack reaches the parent's `plugins:`
		// block only through this constructor (ReconcileForDeploy does
		// `stack.Server = parentStack.Server.Copy()`), so omitting it means every
		// `sc deploy` sees zero plugins, loadPreEnginePlugins no-ops, and the
		// type the plugin owns fails as if it were never installed — while
		// `sc provision` of the same stack works, because that path never copies.
		// Live-caught 2026-10-27, api release run 40554605682: `sc deploy -s dist
		// -e prod` died with `unsupported registrar type "cloudflare"` and not one
		// line of plugin logging, because this field was absent here.
		Plugins: mapValues(sd.Plugins, func(value PluginSpec, key string) PluginSpec {
			return value.Copy()
		}),
	}
}

// Copy deep-copies the spec, including the Source pointer — a shared pointer
// would let one stack's placeholder resolution mutate another's pin.
func (s *PluginSpec) Copy() PluginSpec {
	out := PluginSpec{
		Path:   s.Path,
		SHA256: s.SHA256,
	}
	if s.Source != nil {
		src := *s.Source
		out.Source = &src
	}
	return out
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
		Registrars: mapValues(s.Registrars, func(value RegistrarDescriptor, key string) RegistrarDescriptor {
			return value.Copy()
		}),
		Resources: mapValues(s.Resources, func(value PerEnvResourcesDescriptor, key string) PerEnvResourcesDescriptor {
			return value.Copy()
		}),
	}
}

func (s *PerEnvResourcesDescriptor) Copy() PerEnvResourcesDescriptor {
	return PerEnvResourcesDescriptor{
		Template: s.Template,
		Resources: mapValues(s.Resources, func(value ResourceDescriptor, key string) ResourceDescriptor {
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
	// Every scalar field must be listed here. A child stack reaches its registrars
	// through Copy (parent-descriptor inheritance), so a field omitted below is not
	// merely lost on copy — it is absent from every deploy that is not the parent's
	// own provision. Omitting Default silently unmarked the fleet's catch-all
	// registrar and broke every `.com` deploy with "no registrar handles domain
	// storage.simple-forge.com"; TestCopyPreservesEveryScalarField now fails when a
	// new field is added and not copied.
	return RegistrarDescriptor{
		Type:    s.Type,
		Default: s.Default,
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
		Defaults:      lo.Assign(map[string]interface{}{}, s.Defaults),
		Stacks: lo.MapValues(s.Stacks, func(v StackClientDescriptor, k string) StackClientDescriptor {
			return v.Copy()
		}),
		Security:   s.Security,
		ImageBuild: s.ImageBuild,
	}
}

func (s *StackClientDescriptor) Copy() StackClientDescriptor {
	return StackClientDescriptor{
		Type:        s.Type,
		ParentStack: s.ParentStack,
		ParentEnv:   s.ParentEnv,
		Template:    s.Template,
		Config:      s.Config.Copy(),
	}
}

func (s *StackConfigCompose) Copy() any {
	return &StackConfigCompose{
		DockerComposeFile: s.DockerComposeFile,
		Domain:            s.Domain,
		Prefix:            s.Prefix,
		ProxyKeepPrefix:   s.ProxyKeepPrefix,
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
		CloudExtras:       MustClone(s.CloudExtras),
		StaticEgressIP:    s.StaticEgressIP,
		ImagePullPolicy:   s.ImagePullPolicy,
		ClusterIPAddress:  s.ClusterIPAddress,
		Security:          s.Security,
	}
}

// MustClone creates a deep copy of any value using YAML marshaling/unmarshaling
// This generic function works with any type T and handles complex nested structures automatically
// It is nil-safe and will return the zero value for nil inputs
func MustClone[T any](original T) T {
	var dst T

	// Handle nil pointers gracefully
	if reflect.ValueOf(original).Kind() == reflect.Ptr && reflect.ValueOf(original).IsNil() {
		return dst // Return zero value for nil pointers
	}

	dstValue := reflect.ValueOf(&dst).Elem()
	if dstValue.Kind() == reflect.Ptr {
		structType := dstValue.Type().Elem()
		newValue := reflect.New(structType) // *MyStruct
		dstValue.Set(newValue)
	}
	bytes, _ := yaml.Marshal(original)
	_ = yaml.Unmarshal(bytes, &dst)
	return dst
}
