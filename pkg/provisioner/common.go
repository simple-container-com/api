package provisioner

import (
	"context"
	"io"
	"path"
	"sync"

	"github.com/pkg/errors"

	"api/pkg/api"
	"api/pkg/provisioner/git"
	"api/pkg/provisioner/logger"
	"api/pkg/provisioner/misc"
	"api/pkg/provisioner/secrets"
)

type Provisioner interface {
	Init(ctx context.Context, params InitParams) error

	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error

	Stacks() StacksMap

	GitRepo() git.Repo
}

const DefaultProfile = "default"

type StacksMap map[string]Stack
type provisioner struct {
	profile string
	stacks  StacksMap

	_lock   sync.RWMutex // для защиты secrets & registry
	context context.Context
	gitRepo git.Repo
	cryptor secrets.Cryptor
	log     logger.Logger
}

type ProvisionParams struct {
	RootDir string   `json:"rootDir" yaml:"rootDir"`
	Stacks  []string `json:"stacks" yaml:"stacks"`
}

type InitParams struct {
	RootDir string `json:"rootDir,omitempty" yaml:"rootDir"`
}

type DeployParams struct {
	Stack       string         `json:"stack" yaml:"stack"`
	Environment string         `json:"environment" yaml:"environment"`
	Vars        VariableValues `json:"vars" yaml:"vars"`
}

type VariableValues map[string]any

type Stack struct {
	Name    string                `json:"name" yaml:"name"`
	Secrets api.SecretsDescriptor `json:"secrets" yaml:"secrets"`
	Server  api.ServerDescriptor  `json:"server" yaml:"server"`
	Client  api.ClientDescriptor  `json:"client" yaml:"client"`
}

func New(opts ...Option) (Provisioner, error) {
	res := &provisioner{
		stacks: make(StacksMap),
		log:    logger.New(),
	}

	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	if res.context == nil {
		res.context = context.Background()
		res.log.Warn(res.context, "context is not configured, using background context")
	}
	if res.profile == "" {
		res.log.Warn(res.context, "profile is not set, using default profile")
		res.profile = DefaultProfile
	}
	return res, nil
}

func (p *provisioner) Stacks() StacksMap {
	return p.stacks
}

func (p *provisioner) withReadLock() func() {
	p._lock.RLock()
	return func() {
		p._lock.RUnlock()
	}
}

func (p *provisioner) withWriteLock() func() {
	p._lock.Lock()
	return func() {
		p._lock.Unlock()
	}
}

func (p *provisioner) Init(ctx context.Context, params InitParams) error {
	defer p.withWriteLock()()

	if p.gitRepo == nil {
		if repo, err := git.New(); err != nil {
			return errors.Wrapf(err, "failed to init git")
		} else {
			p.gitRepo = repo
		}
	}
	if err := p.gitRepo.Init(params.RootDir); err != nil {
		return errors.Wrapf(err, "failed to init git repo")
	}
	if p.cryptor == nil {
		if cryptor, err := secrets.NewCryptor(params.RootDir); err != nil {
			return errors.Wrapf(err, "failed to init cryptor")
		} else {
			p.cryptor = cryptor
		}
	}

	// create .sc dir
	if _, err := p.gitRepo.CreateDir(api.ScConfigDirectory); err != nil {
		return errors.Wrapf(err, "failed to init config dir")
	}

	// generate default profile
	if err := p.cryptor.GenerateKeyPairWithProfile(DefaultProfile); err != nil {
		return errors.Wrapf(err, "failed to generate key pair")
	}
	if err := p.gitRepo.AddFileToIgnore(api.ConfigFilePath(params.RootDir, DefaultProfile)); err != nil {
		return errors.Wrapf(err, "failed to add config file to ignore")
	}

	// create .sc/cfg.yaml.template
	tplFilePath := path.Join(params.RootDir, api.ScConfigDirectory, "cfg.yaml.template")
	if tplFile, err := p.gitRepo.CreateFile(tplFilePath); err != nil {
		return errors.Wrapf(err, "failed to init config template file")
	} else if cfgTpl, err := misc.Templates.ReadFile("embed/templates/cfg.yaml.template"); err != nil {
		return errors.Wrapf(err, "failed to read config template file")
	} else {
		defer func() { _ = tplFile.Close() }()
		if _, err := io.WriteString(tplFile, string(cfgTpl)); err != nil {
			return errors.Wrapf(err, "failed to write config template file %q", tplFile.Name())
		}
		err := p.gitRepo.AddFileToGit(tplFilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to add template file to git")
		}
	}

	// initial commit
	if err := p.gitRepo.Commit("simple-container.com initial commit", git.CommitOpts{
		All: true,
	}); err != nil {
		return errors.Wrapf(err, "failed to make initial commit")
	}

	return nil
}

func (p *provisioner) GitRepo() git.Repo {
	return p.gitRepo
}
