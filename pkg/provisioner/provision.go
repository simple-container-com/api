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

		if secretsDesc, err := p.readSecretsDescriptor(stacksDir, stackName); err != nil && (!readOpts.IgnoreSecretsMissing || lo.Contains(readOpts.RequireSecretConfigs, stackName)) {
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

func (p *provisioner) readSecretsDescriptor(rootDir string, stackName string) (*api.SecretsDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.SecretsDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrapf(err, "file not found: %q", descFilePath)
	}
	return p.readSecretsDescriptorFromFile(descFilePath)
}

func (p *provisioner) readSecretsDescriptorFromFile(descFilePath string) (*api.SecretsDescriptor, error) {
	if desc, err := api.ReadSecretsDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read secrets descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
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
