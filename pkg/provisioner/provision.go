package provisioner

import (
	"context"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

func (p *provisioner) Provision(ctx context.Context, params api.ProvisionParams) error {
	cfg, err := p.readConfigForProvision(ctx, params)
	if err != nil {
		return err
	}

	for _, stack := range p.stacks {
		pv, err := p.getProvisionerForStack(ctx, stack)
		if err != nil {
			return errors.Wrapf(err, "failed to get provisioner for stack %q", stack.Name)
		}
		if err := pv.ProvisionStack(ctx, cfg, stack); err != nil {
			return errors.Wrapf(err, "failed to create stack %q", stack.Name)
		}
	}
	return nil
}

func (p *provisioner) readConfigForProvision(ctx context.Context, params api.ProvisionParams) (*api.ConfigFile, error) {
	cfg, err := api.ReadConfigFile(p.rootDir, p.profile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file for profile %q", p.profile)
	}

	if err := p.ReadStacks(ctx, cfg, params, false); err != nil {
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

func (p *provisioner) ReadStacks(ctx context.Context, cfg *api.ConfigFile, params api.ProvisionParams, ignoreErrors bool) error {
	stacksDir := p.getStacksDir(cfg, params.StacksDir)

	stacks := params.Stacks
	if len(stacks) == 0 {
		p.log.Info(ctx, "stacks list is not provided, reading from %q", stacksDir)
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
		p.log.Info(ctx, "read stacks from %q: %q", stacksDir, strings.Join(stacks, ", "))
	}

	for _, stackName := range stacks {
		stack := api.Stack{
			Name: stackName,
		}

		if serverDesc, err := p.readServerDescriptor(stacksDir, stackName); err != nil && !ignoreErrors {
			return err
		} else if serverDesc != nil {
			p.log.Debug(ctx, "Successfully read server descriptor: %q", serverDesc)
			stack.Server = *serverDesc
		}

		if clientDesc, err := p.optionallyReadClientDescriptor(stacksDir, stackName); err != nil {
			return err
		} else if clientDesc != nil {
			p.log.Debug(ctx, "Successfully read client descriptor: %q", clientDesc)
			stack.Client = *clientDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		if secretsDesc, err := p.optionallyReadSecretsDescriptor(stacksDir, stackName); err != nil {
			return err
		} else if secretsDesc != nil {
			p.log.Debug(ctx, "Successfully read secrets descriptor: %q", secretsDesc)
			stack.Secrets = *secretsDesc
		} else {
			p.log.Debug(ctx, "Secrets descriptor not found for %s", stackName)
		}

		p.stacks[stackName] = stack
	}

	provisioners := map[string]api.Provisioner{}
	for stackName, stack := range p.stacks {
		provisioners[stackName] = stack.Server.Provisioner.GetProvisioner()
	}

	err := p.phResolver.Resolve(p.stacks)
	if err != nil {
		return err
	}

	p.stacks = lo.MapValues(p.stacks, func(stack api.Stack, name string) api.Stack {
		stack.Server.Provisioner.SetProvisioner(provisioners[name])
		return stack
	})

	return err
}

func (p *provisioner) readServerDescriptor(rootDir string, stackName string) (*api.ServerDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ServerDescriptorFileName)
	if desc, err := api.ReadServerDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read server descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) optionallyReadSecretsDescriptor(rootDir string, stackName string) (*api.SecretsDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.SecretsDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return p.readSecretsDescriptor(descFilePath)
}

func (p *provisioner) readSecretsDescriptor(descFilePath string) (*api.SecretsDescriptor, error) {
	if desc, err := api.ReadSecretsDescriptor(descFilePath); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", descFilePath)
	} else {
		return desc, nil
	}
}

func (p *provisioner) optionallyReadClientDescriptor(rootDir string, stackName string) (*api.ClientDescriptor, error) {
	descFilePath := path.Join(rootDir, stackName, api.ClientDescriptorFileName)
	if _, err := os.Stat(descFilePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return p.readClientDescriptor(descFilePath)
}

func (p *provisioner) readClientDescriptor(path string) (*api.ClientDescriptor, error) {
	if desc, err := api.ReadClientDescriptor(path); err != nil {
		return nil, errors.Wrapf(err, "failed to read client descriptor from %q", path)
	} else {
		return desc, nil
	}
}
