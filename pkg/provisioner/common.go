package provisioner

import (
	"context"
	"io"
	"path"
	"sync"

	git2 "api/pkg/api/git"
	"api/pkg/api/logger"
	secrets2 "api/pkg/api/secrets"

	"api/pkg/provisioner/placeholders"

	"github.com/pkg/errors"

	"api/pkg/api"
	"api/pkg/provisioner/misc"
)

type Provisioner interface {
	ReadStacks(ctx context.Context, params ProvisionParams) error

	Init(ctx context.Context, params InitParams) error

	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error

	Stacks() api.StacksMap

	GitRepo() git2.Repo
}

const DefaultProfile = "default"

type provisioner struct {
	profile string
	stacks  api.StacksMap

	_lock               sync.RWMutex // для защиты secrets & registry
	context             context.Context
	gitRepo             git2.Repo
	cryptor             secrets2.Cryptor
	phResolver          placeholders.Placeholders
	log                 logger.Logger
	overrideProvisioner api.Provisioner
}

type ProvisionParams struct {
	RootDir string   `json:"rootDir" yaml:"rootDir"`
	Profile string   `json:"profile" yaml:"profile"`
	Stacks  []string `json:"stacks" yaml:"stacks"`
}

type InitParams struct {
	ProjectName string `json:"projectName" yaml:"projectName"`
	RootDir     string `json:"rootDir,omitempty" yaml:"rootDir"`
}

type DeployParams struct {
	Stack       string             `json:"stack" yaml:"stack"`
	Environment string             `json:"environment" yaml:"environment"`
	Vars        api.VariableValues `json:"vars" yaml:"vars"`
}

func New(opts ...Option) (Provisioner, error) {
	res := &provisioner{
		stacks: make(api.StacksMap),
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

func (p *provisioner) Stacks() api.StacksMap {
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

	if params.ProjectName == "" {
		return errors.New("project name is not configured")
	}

	if p.phResolver == nil {
		p.phResolver = placeholders.New(p.log)
	}

	if p.gitRepo == nil {
		if repo, err := git2.New(); err != nil {
			return errors.Wrapf(err, "failed to init git")
		} else {
			p.gitRepo = repo
		}
	}

	if err := p.gitRepo.InitOrOpen(params.RootDir); err != nil {
		return errors.Wrapf(err, "failed to init git repo")
	}
	if p.cryptor == nil {
		if cryptor, err := secrets2.NewCryptor(params.RootDir); err != nil {
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
	if err := p.cryptor.GenerateKeyPairWithProfile(params.ProjectName, DefaultProfile); err != nil {
		return errors.Wrapf(err, "failed to generate key pair")
	}
	if err := p.gitRepo.AddFileToIgnore(api.ConfigFilePath("", DefaultProfile)); err != nil {
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
	if err := p.gitRepo.Commit("simple-container.com initial commit", git2.CommitOpts{
		All: true,
	}); err != nil {
		return errors.Wrapf(err, "failed to make initial commit")
	}

	return nil
}

func (p *provisioner) GitRepo() git2.Repo {
	return p.gitRepo
}
