package git

type Option func(r *repo) error

type CommitOpts struct {
	All bool
}

func WithGitDir(dir string) Option {
	return func(r *repo) error {
		r.gitDir = dir
		return nil
	}
}

func WithRootDir(dir string) Option {
	return func(r *repo) error {
		r.workDir = dir
		return nil
	}
}
