package git

import (
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"os"
	"path"
)

type Repo interface {
	Init(wd string, opts ...Option) error

	OpenFile(filePath string, flag int, perm os.FileMode) (billy.File, error)
	CreateFile(filePath string) (billy.File, error)
	Exists(filePath string) bool

	RemoveFileFromIgnore(filePath string) error
	AddFileToIgnore(filePath string) error
}

type repo struct {
	workDir string
	gitDir  string

	wdFs    billy.Filesystem
	gitFs   billy.Filesystem
	gitRepo *git.Repository
}

type Option func(r *repo) error

func WithGitDir(dir string) Option {
	return func(r *repo) error {
		r.gitDir = dir
		return nil
	}
}

func (r *repo) OpenFile(filePath string, flag int, perm os.FileMode) (billy.File, error) {
	return r.wdFs.OpenFile(filePath, flag, perm)
}

func New(opts ...Option) (Repo, error) {
	return newWithOpts(opts...)
}

func newWithOpts(opts ...Option) (*repo, error) {
	res := &repo{}
	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (r *repo) Init(wd string, opts ...Option) error {
	c, wt, st, err := initRepo(wd, opts)
	if err != nil {
		return err
	}
	c.gitRepo, err = git.Init(st, wt)
	return err
}

func initRepo(wd string, opts []Option) (*repo, billy.Filesystem, *filesystem.Storage, error) {
	gitRepo, err := newWithOpts(opts...)
	if err != nil {
		return nil, nil, nil, err
	}

	gitRepo.workDir = wd

	var fs *dotgit.RepositoryFilesystem
	var wt = osfs.New(wd)
	if gitRepo.gitDir != "" {
		gitRepo.gitFs = osfs.New(path.Join(gitRepo.workDir, gitRepo.gitDir))
	} else {
		gitRepo.gitFs = osfs.New(path.Join(gitRepo.workDir, git.GitDirName))
	}
	if gitRepo.workDir != "" {
		gitRepo.wdFs = osfs.New(gitRepo.workDir)
	} else {
		gitRepo.wdFs = osfs.New("")
	}
	fs = dotgit.NewRepositoryFilesystem(gitRepo.gitFs, nil)
	storage := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	return gitRepo, wt, storage, nil
}

func Open(wd string, opts ...Option) (Repo, error) {
	c, wt, st, err := initRepo(wd, opts)
	if err != nil {
		return nil, err
	}
	c.gitRepo, err = git.Open(st, wt)
	return c, err
}

func (r *repo) CreateFile(filePath string) (billy.File, error) {
	return r.gitFs.Create(filePath)
}

func (r *repo) Exists(filePath string) bool {
	if _, err := r.wdFs.Stat(filePath); os.IsNotExist(err) {
		return false
	} else if err != nil {
		// TODO: log err
		return false
	}
	return true
}
