package provisioner

import (
	"context"
	"io"
	"os"
	"path"
	"sync"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/provisioner/misc"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

type Provisioner interface {
	ReadStacks(ctx context.Context, params ProvisionParams) error

	Init(ctx context.Context, params InitParams) error
	InitProfile(generateKeyPair bool) error
	MakeInitialCommit() error

	Provision(ctx context.Context, params ProvisionParams) error

	Deploy(ctx context.Context, params DeployParams) error

	Stacks() api.StacksMap

	GitRepo() git.Repo

	Cryptor() secrets.Cryptor
}

const DefaultProfile = "default"

type provisioner struct {
	projectName string
	rootDir     string
	profile     string
	stacks      api.StacksMap

	_lock               sync.RWMutex // для защиты secrets & registry
	context             context.Context
	gitRepo             git.Repo
	cryptor             secrets.Cryptor
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
	ProjectName         string `json:"projectName" yaml:"projectName"`
	RootDir             string `json:"rootDir,omitempty" yaml:"rootDir"`
	Profile             string `json:"profile,omitempty" yaml:"profile"`
	SkipInitialCommit   bool   `json:"skipInitialCommit" yaml:"skipInitialCommit"`
	SkipProfileCreation bool   `json:"skipProfileCreation" yaml:"skipProfileCreation"`
	GenerateKeyPair     bool   `json:"generateKeyPair" yaml:"generateKeyPair"`
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
		res.log.Debug(res.context, "context is not configured, using background context")
	}
	if res.profile == "" {
		res.log.Debug(res.context, "profile is not set, using default profile")
		res.profile = DefaultProfile
	}
	if res.rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		res.rootDir = path.Base(wd)
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
	p.context = ctx
	if params.ProjectName == "" {
		return errors.New("project name is not configured")
	}
	p.projectName = params.ProjectName

	if params.RootDir == "" {
		return errors.New("root dir is not configured")
	}
	p.rootDir = params.RootDir

	if p.phResolver == nil {
		p.phResolver = placeholders.New(p.log)
	}

	if p.gitRepo == nil {
		if repo, err := git.New(); err != nil {
			return errors.Wrapf(err, "failed to init git")
		} else {
			p.gitRepo = repo
		}
	}

	if err := p.gitRepo.InitOrOpen(p.rootDir); err != nil {
		return errors.Wrapf(err, "failed to init git repo")
	}
	if p.cryptor == nil {
		if cryptor, err := secrets.NewCryptor(p.rootDir, secrets.WithProfile(p.profile), secrets.WithGitRepo(p.gitRepo)); err != nil {
			return errors.Wrapf(err, "failed to init cryptor")
		} else {
			p.cryptor = cryptor
		}
	}

	// create .sc dir
	if _, err := p.gitRepo.CreateDir(api.ScConfigDirectory); err != nil {
		return errors.Wrapf(err, "failed to init config dir")
	}

	if !params.SkipProfileCreation {
		if err := p.InitProfile(params.GenerateKeyPair); err != nil {
			return err
		}
	}

	if !params.SkipInitialCommit {
		if err := p.MakeInitialCommit(); err != nil {
			return err
		}
	}

	return nil
}

func (p *provisioner) MakeInitialCommit() error {
	// initial commit
	if err := p.gitRepo.Commit("simple-container.com initial commit", git.CommitOpts{
		All: true,
	}); err != nil {
		return errors.Wrapf(err, "failed to make initial commit")
	}
	return nil
}

func (p *provisioner) InitProfile(generateKeyPair bool) error {
	if p.profile == "" {
		return errors.Errorf("profile is not configured")
	}

	// create .sc/cfg.yaml.template
	tplFilePath := path.Join(api.ScConfigDirectory, "cfg.yaml.template")
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

	profileCfgFile := api.ConfigFilePath("", p.profile)
	if generateKeyPair {
		// generate profile
		if err := p.cryptor.GenerateKeyPairWithProfile(p.projectName, p.profile); err != nil {
			return errors.Wrapf(err, "failed to generate key pair")
		}
	} else if err := p.gitRepo.CopyFile(tplFilePath, profileCfgFile); err != nil {
		return errors.Wrapf(err, "failed to copy template file to profile")
	}
	if err := p.gitRepo.AddFileToIgnore(profileCfgFile); err != nil {
		return errors.Wrapf(err, "failed to add config file to ignore")
	}

	return p.cryptor.ReadProfileConfig()
}

func (p *provisioner) GitRepo() git.Repo {
	return p.gitRepo
}

func (p *provisioner) Cryptor() secrets.Cryptor {
	return p.cryptor
}
