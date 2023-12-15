package git

import (
	"os"
	"path"

	"api/pkg/api/git/path_util"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"

	"github.com/pkg/errors"
)

var ErrRepositoryAlreadyExists = errors.New("repository already exists")

type Repo interface {
	Init(wd string, opts ...Option) error
	Open(wd string, opts ...Option) error
	InitOrOpen(wd string, opts ...Option) error

	OpenFile(filePath string, flag int, perm os.FileMode) (billy.File, error)
	CreateFile(filePath string) (billy.File, error)
	Exists(filePath string) bool
	CreateDir(filePath string) (billy.Dir, error)

	RemoveFileFromIgnore(filePath string) error
	AddFileToIgnore(filePath string) error

	AddFileToGit(filePath string) error
	Commit(msg string, opts CommitOpts) error
	Log() []Commit
	Workdir() string
}

type Commit struct {
	Author  string
	Hash    string
	Message string
}

type repo struct {
	workDir string
	gitDir  string

	wdFs    billy.Filesystem
	gitFs   billy.Filesystem
	gitRepo *git.Repository

	opts []Option
}

func (r *repo) OpenFile(filePath string, flag int, perm os.FileMode) (billy.File, error) {
	rPath, err := path_util.ReplaceTildeWithHome(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to replace path %q with home", filePath)
	}
	if path.IsAbs(rPath) {
		return osfs.New("").OpenFile(rPath, flag, perm)
	}
	return r.wdFs.OpenFile(rPath, flag, perm)
}

func New(opts ...Option) (Repo, error) {
	return newWithOpts(opts...)
}

func newWithOpts(opts ...Option) (*repo, error) {
	res := &repo{
		opts: opts,
	}
	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (r *repo) InitOrOpen(wd string, opts ...Option) error {
	if err := r.Init(wd, opts...); err == ErrRepositoryAlreadyExists {
		return r.Open(wd, opts...)
	} else if err != nil {
		return err
	}
	return nil
}

func (r *repo) Workdir() string {
	return r.workDir
}

func (r *repo) Init(wd string, opts ...Option) error {
	c, wt, st, err := initRepo(wd, append(r.opts, opts...))
	if err != nil {
		return err
	}
	if r.gitRepo, err = git.Init(st, wt); err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			return ErrRepositoryAlreadyExists
		}
		return err
	}
	r.overwriteFrom(c)
	return err
}

func (r *repo) Open(wd string, opts ...Option) error {
	c, wt, st, err := initRepo(wd, append(r.opts, opts...))
	if err != nil {
		return err
	}
	if r.gitRepo, err = git.Open(st, wt); err != nil {
		return err
	}
	r.overwriteFrom(c)
	return err
}

func (r *repo) overwriteFrom(c *repo) {
	r.gitFs = c.gitFs
	r.gitDir = c.gitDir
	r.workDir = c.workDir
	r.wdFs = c.wdFs
}

func initRepo(wd string, opts []Option) (*repo, billy.Filesystem, *filesystem.Storage, error) {
	gitRepo, err := newWithOpts(opts...)
	if err != nil {
		return nil, nil, nil, err
	}

	gitRepo.workDir = wd

	var fs *dotgit.RepositoryFilesystem
	wt := osfs.New(wd)
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

func (r *repo) detectRootDir() error {
	gitRepo, err := git.PlainOpenWithOptions("", &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: false,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to detect git dir")
	}
	r.gitRepo = gitRepo
	wt, err := gitRepo.Worktree()
	if err != nil {
		return errors.Wrapf(err, "failed to open worktree")
	}
	r.gitFs = wt.Filesystem
	r.workDir = wt.Filesystem.Root()
	r.wdFs = osfs.New(r.workDir)
	return nil
}

func (r *repo) CreateFile(filePath string) (billy.File, error) {
	return r.wdFs.Create(filePath)
}

func (r *repo) CreateDir(filePath string) (billy.Dir, error) {
	if err := r.wdFs.MkdirAll(filePath, os.ModePerm); err != nil {
		return nil, errors.Wrapf(err, "failed to create dir %q", filePath)
	}
	return r.wdFs.Chroot(filePath)
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

func (r *repo) AddFileToGit(filePath string) error {
	if wt, err := r.gitRepo.Worktree(); err != nil {
		return errors.Wrapf(err, "failed to get worktree")
	} else if _, err = wt.Add(filePath); err != nil {
		return errors.Wrapf(err, "failed to add file to git")
	}
	return nil
}

func (r *repo) Commit(msg string, opts CommitOpts) error {
	if wt, err := r.gitRepo.Worktree(); err != nil {
		return errors.Wrapf(err, "failed to get worktree")
	} else if _, err = wt.Commit(msg, &git.CommitOptions{
		All: opts.All,
		// TODO: pass other opts
	}); err != nil {
		return errors.Wrapf(err, "failed to make commit")
	}
	return nil
}

func (r *repo) Log() []Commit {
	var res []Commit
	if ci, err := r.gitRepo.Log(&git.LogOptions{
		All: true,
	}); err != nil {
		// TODO: log error
		return res
	} else {
		for {
			c, err := ci.Next()
			if err != nil {
				// TODO: log error
				break
			}
			res = append(res, Commit{
				Author:  c.Author.String(),
				Hash:    c.Hash.String(),
				Message: c.Message,
			})
		}
	}
	return res
}
