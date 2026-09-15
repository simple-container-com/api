// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package provisioner

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

func (p *provisioner) Provision(ctx context.Context, params api.ProvisionParams) error {
	p.logWelcome(ctx, nil)

	cfg, err := p.prepareForParentStack(ctx, params)
	if err != nil {
		return err
	}

	for _, stack := range p.stacks {
		pv, err := p.getProvisionerForStack(ctx, stack)
		if err != nil {
			return errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
		}
		if err := pv.ProvisionStack(ctx, cfg, stack, params); err != nil {
			return errors.Wrapf(err, "failed to create stack %q", stack.Name)
		}
	}
	return nil
}

func (p *provisioner) prepareForParentStack(ctx context.Context, params api.ProvisionParams) (*api.ConfigFile, error) {
	cfg, err := api.ReadConfigFile(p.rootDir, p.profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if err := p.ReadStacks(ctx, cfg, params, api.ReadIgnoreNoSecretsAndClientCfg); err != nil {
		return nil, errors.Wrapf(err, "failed to read stacks")
	}

	if p.profile == "" && params.Profile == "" {
		return nil, errors.Errorf("profile is not set")
	} else if params.Profile != "" {
		p.profile = params.Profile
	}

	return cfg, nil
}

func (p *provisioner) getProvisionerForStack(ctx context.Context, stack api.Stack) (api.Provisioner, error) {
	pv := stack.Server.Provisioner.GetProvisioner()
	if p.overrideProvisioner != nil {
		pv = p.overrideProvisioner
	}
	if pv == nil {
		return nil, errors.Errorf("provisioner is not set for stack %q", stack.Name)
	}
	var pubKey string
	if p.cryptor != nil {
		pubKey = p.cryptor.PublicKey()
	} else {
		p.log.Warn(ctx, "Cryptor is not set, secrets will not be encrypted")
	}
	pv.SetPublicKey(pubKey)
	return pv, nil
}

func (p *provisioner) ReadStacks(ctx context.Context, cfg *api.ConfigFile, params api.ProvisionParams, readOpts api.ReadOpts) error {
	stacksDir := p.getStacksDir(cfg, params.StacksDir)

	stacks := params.Stacks
	if len(stacks) == 0 {
		p.log.Debug(ctx, "stacks list is not provided, reading from %q", stacksDir)
		dirs, err := os.ReadDir(stacksDir)
		if err != nil {
			return errors.Wrapf(err, "failed to read stacks dir")
		}
		stacks = lo.Map(lo.Filter(dirs, func(d os.DirEntry, _ int) bool {
			dInfo, err := d.Info()
			if err != nil {
				return false
			}
			// could be a symlink to dir
			return dInfo.Mode()&os.ModeSymlink == os.ModeSymlink || d.IsDir()
		}), func(d os.DirEntry, _ int) string {
			return d.Name()
		})
		p.log.Info(ctx, "reading stacks from %q: [\"%s\"]", stacksDir, strings.Join(stacks, "\",\""))
	}

	for _, stackName := range stacks {
		stack := api.Stack{
			Name: stackName,
		}

		if serverDesc, err := p.readServerDescriptor(stacksDir, stackName); err != nil && (!readOpts.IgnoreServerMissing || lo.Contains(readOpts.RequireServerConfigs, stackName)) {
			return err
		} else if serverDesc != nil {
			// SECURITY: Never log actual server descriptor content - may contain resolved secrets
			p.log.Debug(ctx, "Successfully read server descriptor for stack: %s", stackName)
			stack.Server = *serverDesc
		} else {
			p.log.Debug(ctx, "Server descriptor not found for %s", stackName)
		}

		if clientDesc, err := p.readClientDescriptor(stacksDir, stackName); err != nil && (!readOpts.IgnoreClientMissing || lo.Contains(readOpts.RequireClientConfigs, stackName)) {
			return err
		} else if clientDesc != nil {
			// SECURITY: Never log actual descriptor content that might contain credentials
			p.log.Debug(ctx, "Successfully read client descriptor for stack: %s", stackName)
			stack.Client = *clientDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		if secretsDesc, err := p.readSecretsDescriptor(ctx, stacksDir, stackName); err != nil && (errors.Is(err, scoped.ErrScopedIntegrity) || errors.Is(err, scoped.ErrScopedUnavailable) || !readOpts.IgnoreSecretsMissing || lo.Contains(readOpts.RequireSecretConfigs, stackName)) {
			// A scoped integrity failure (tamper / corrupt / ambiguous) OR a transient
			// backend outage (KMS throttle) is fatal even under IgnoreSecretsMissing — a
			// secret we could not resolve is never treated as "simply absent".
			return err
		} else if secretsDesc != nil {
			// SECURITY: Never log actual secrets descriptor content - contains credential values
			p.log.Debug(ctx, "Successfully read secrets descriptor for stack: %s", stackName)
			stack.Secrets = *secretsDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		p.stacks[stackName] = stack
	}

	err := p.resolvePlaceholders()
	if err != nil {
		return err
	}

	return err
}

func (p *provisioner) resolvePlaceholders() error {
	ctx := context.Background() // Create context for debug logging
	p.log.Debug(ctx, "🔍 Starting placeholder resolution for %d stacks", len(p.stacks))

	provisioners := map[string]api.Provisioner{}
	for stackName, stack := range p.stacks {
		provisioners[stackName] = stack.Server.Provisioner.GetProvisioner()

		// Debug provisioner config before resolution
		p.log.Debug(ctx, "🔍 Stack %s provisioner type: %s", stackName, stack.Server.Provisioner.Type)
		p.log.Debug(ctx, "🔍 Stack %s provisioner config before resolution: %+v", stackName, stack.Server.Provisioner.Config)
	}

	p.log.Debug(ctx, "🔧 Calling phResolver.Resolve()...")
	err := p.phResolver.Resolve(p.stacks)
	if err != nil {
		p.log.Debug(ctx, "❌ Placeholder resolution failed: %v", err)
		return err
	}
	p.log.Debug(ctx, "✅ Placeholder resolution completed successfully")

	// Debug provisioner config after resolution (without sensitive values)
	for stackName, stack := range p.stacks {
		// Only show structure, not actual credential values to avoid security leaks
		p.log.Debug(ctx, "🔍 Stack %s provisioner config after resolution - checking if placeholders were resolved", stackName)
		configStr := fmt.Sprintf("%+v", stack.Server.Provisioner.Config)
		if strings.Contains(configStr, "${") {
			p.log.Debug(ctx, "❌ Stack %s still has unresolved placeholders after resolution", stackName)
		} else {
			p.log.Debug(ctx, "✅ Stack %s placeholders appear to be resolved (no ${} found)", stackName)
		}
	}

	p.stacks = lo.MapValues(p.stacks, func(stack api.Stack, name string) api.Stack {
		stack.Server.Provisioner.SetProvisioner(provisioners[name])
		return stack
	})
	return nil
}

func (p *provisioner) readServerDescriptor(rootDir string, stackName string) (*api.ServerDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ServerDescriptorFileName)
	if desc, err := api.ReadServerDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read server descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) readSecretsDescriptor(ctx context.Context, rootDir string, stackName string) (*api.SecretsDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.SecretsDescriptorFileName)
	legacyExists := true
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		legacyExists = false
	}
	return p.readSecretsDescriptorFromFile(ctx, descFilePath, legacyExists)
}

func (p *provisioner) readSecretsDescriptorFromFile(ctx context.Context, descFilePath string, legacyExists bool) (*api.SecretsDescriptor, error) {
	desc := &api.SecretsDescriptor{}
	if legacyExists {
		d, err := api.ReadSecretsDescriptor(descFilePath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read secrets descriptor from %q", descFilePath)
		}
		desc = d
	}
	// Additively merge any per-scope secrets (secrets.<scope>.yaml) that a candidate
	// key — the ambient config key OR a CI scope key (SC_KEY_<SCOPE> / SC_SCOPE_KEY) —
	// is a recipient of. Repos without scope files are unaffected. Whole-file values
	// win on conflict, so a scoped value never changes an existing ${secret:} result;
	// only new keys are added. The pull_request clamp is cryptographic — a scope key
	// that is not a recipient of secrets.prod.yaml cannot open it. Integrity failures
	// (tamper / corrupt / ambiguous) are tagged scoped.ErrScopedIntegrity and MUST NOT
	// be swallowed by IgnoreSecretsMissing (see ReadStacks).
	scopedVals, sErr := scoped.ResolveScopedValues(path.Dir(descFilePath), p.scopeCandidateKeys())
	if sErr != nil {
		return nil, sErr
	}
	for k, v := range scopedVals {
		if _, exists := desc.Values[k]; exists {
			// The legacy whole-file store wins on conflict (an actor who can only write a
			// scope file cannot override a legacy secret). `sc secrets scope lint` flags
			// this collision, but lint is not always a required check — so warn at deploy
			// too, or an operator who set a scoped value silently gets the legacy one.
			if p.log != nil {
				p.log.Warn(ctx, "scoped secret %q is shadowed by the legacy secrets.yaml for this stack; the legacy value is used. Remove one (see `sc secrets scope lint`).", k)
			}
			continue
		}
		if desc.Values == nil {
			desc.Values = map[string]string{}
		}
		desc.Values[k] = v
	}
	// A stack with neither a legacy secrets.yaml nor any openable scoped value has no
	// secrets for this key — preserve the previous "not found" behavior (ignorable
	// under IgnoreSecretsMissing) rather than returning an empty descriptor.
	if !legacyExists && len(desc.Values) == 0 && len(desc.Auth) == 0 {
		return nil, errors.Wrapf(os.ErrNotExist, "file not found: %q (and no openable scoped secrets)", descFilePath)
	}
	return desc, nil
}

// scopeCandidateKeys gathers every private key that might open a scope file: the
// ambient cryptor key (from SIMPLE_CONTAINER_CONFIG / profile) plus CI scope keys
// supplied without a full config — a generic SC_SCOPE_KEY and any per-scope
// SC_KEY_<SCOPE> (e.g. SC_KEY_PR). This lets a pull_request job resolve scoped
// secrets holding ONLY its scope key.
func (p *provisioner) scopeCandidateKeys() []string {
	var keys []string
	if p.cryptor != nil {
		if pk := p.cryptor.PrivateKey(); strings.TrimSpace(pk) != "" {
			keys = append(keys, pk)
		}
	}
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" {
		keys = append(keys, v)
	}
	// Accept only SC_KEY_<SCOPE> where <SCOPE> maps back to a valid scope name
	// (uppercase, '-'→'_'), so an unrelated SC_KEY_* env var is not blindly tried as
	// a decryption key. A job with several scope keys resolves the union of its scopes.
	for _, e := range os.Environ() {
		name, val, ok := strings.Cut(e, "=")
		if !ok || strings.TrimSpace(val) == "" || !strings.HasPrefix(name, "SC_KEY_") {
			continue
		}
		scopeName := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, "SC_KEY_"), "_", "-"))
		if scoped.ValidateScopeName(scopeName) != nil {
			continue
		}
		keys = append(keys, val)
	}
	return keys
}

func (p *provisioner) readClientDescriptor(rootDir string, stackName string) (*api.ClientDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ClientDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrapf(err, "file not found: %q", descFilePath)
	}
	return p.readClientDescriptorFromFile(descFilePath)
}

func (p *provisioner) readClientDescriptorFromFile(path string) (*api.ClientDescriptor, error) {
	if desc, err := api.ReadClientDescriptor(path); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", path)
	} else {
		return desc, nil
	}
}
