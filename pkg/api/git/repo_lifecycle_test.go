// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package git

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	. "github.com/onsi/gomega"
)

// newRepoIn initialises a brand-new git repo inside dir using the package's own
// Init, and configures a deterministic local author so commit metadata is
// predictable regardless of the host's global git config.
func newRepoIn(dir string) *repo {
	r, err := newWithOpts()
	Expect(err).ToNot(HaveOccurred())
	Expect(r.Init(dir)).To(Succeed())
	setLocalAuthor(r, "Test Author", "author@test.local")
	return r
}

func setLocalAuthor(r *repo, name, email string) {
	cfg, err := r.gitRepo.Config()
	Expect(err).ToNot(HaveOccurred())
	cfg.User.Name = name
	cfg.User.Email = email
	Expect(r.gitRepo.SetConfig(cfg)).To(Succeed())
}

// writeWorktreeFile creates/overwrites a file in the repo worktree via the
// repo's own CreateFile and returns nothing; it asserts success.
func writeWorktreeFile(r Repo, name, content string) {
	f, err := r.CreateFile(name)
	Expect(err).ToNot(HaveOccurred())
	_, err = f.Write([]byte(content))
	Expect(err).ToNot(HaveOccurred())
	Expect(f.Close()).To(Succeed())
}

func TestNewAndOptions(t *testing.T) {
	RegisterTestingT(t)

	t.Run("New returns a non-nil Repo with no opts", func(t *testing.T) {
		RegisterTestingT(t)
		r, err := New()
		Expect(err).ToNot(HaveOccurred())
		Expect(r).ToNot(BeNil())
	})

	t.Run("WithRootDir + WithGitDir are applied to the repo", func(t *testing.T) {
		RegisterTestingT(t)
		r, err := newWithOpts(WithRootDir("/some/root"), WithGitDir("customgit"))
		Expect(err).ToNot(HaveOccurred())
		Expect(r.workDir).To(Equal("/some/root"))
		Expect(r.gitDir).To(Equal("customgit"))
	})

	t.Run("an option returning an error aborts construction", func(t *testing.T) {
		RegisterTestingT(t)
		boom := func(_ *repo) error { return io.ErrUnexpectedEOF }
		r, err := New(boom)
		Expect(err).To(MatchError(io.ErrUnexpectedEOF))
		Expect(r).To(BeNil())
	})

	t.Run("Init surfaces an erroring option through initRepo", func(t *testing.T) {
		RegisterTestingT(t)
		boom := func(_ *repo) error { return io.ErrUnexpectedEOF }
		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		Expect(r.Init(t.TempDir(), boom)).To(MatchError(io.ErrUnexpectedEOF))
	})

	t.Run("Open surfaces an erroring option through initRepo", func(t *testing.T) {
		RegisterTestingT(t)
		boom := func(_ *repo) error { return io.ErrUnexpectedEOF }
		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		Expect(r.Open(t.TempDir(), boom)).To(MatchError(io.ErrUnexpectedEOF))
	})

	t.Run("package-level Open surfaces an erroring option", func(t *testing.T) {
		RegisterTestingT(t)
		boom := func(_ *repo) error { return io.ErrUnexpectedEOF }
		got, err := Open(t.TempDir(), boom)
		Expect(err).To(MatchError(io.ErrUnexpectedEOF))
		Expect(got).To(BeNil())
	})
}

func TestInitOpenLifecycle(t *testing.T) {
	RegisterTestingT(t)

	t.Run("Init creates a real .git dir and sets work/git dirs", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.Workdir()).To(Equal(wd))
		Expect(r.Gitdir()).To(Equal(""))

		// default git dir name is ".git"
		info, err := os.Stat(path.Join(wd, gogit.GitDirName))
		Expect(err).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
	})

	t.Run("Init honours WithGitDir for the dot-git location", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r, err := newWithOpts(WithGitDir("dotgit"))
		Expect(err).ToNot(HaveOccurred())
		Expect(r.Init(wd)).To(Succeed())
		Expect(r.Gitdir()).To(Equal("dotgit"))

		_, err = os.Stat(path.Join(wd, "dotgit"))
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Init twice in the same dir reports ErrRepositoryAlreadyExists", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		_ = newRepoIn(wd)

		// A fresh repo handle pointing at the same worktree.
		r2, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		Expect(r2.Init(wd)).To(Equal(ErrRepositoryAlreadyExists))
	})

	t.Run("Init returns a non-already-exists error for an invalid worktree", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		// Point the worktree at a regular file, not a directory.
		asFile := filepath.Join(wd, "afile")
		Expect(os.WriteFile(asFile, []byte("x"), 0o644)).To(Succeed())

		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		err = r.Init(asFile)
		Expect(err).To(HaveOccurred())
		Expect(err).ToNot(Equal(ErrRepositoryAlreadyExists))
		Expect(err.Error()).To(ContainSubstring("not a directory"))
	})

	t.Run("Open on a non-repo directory fails", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		err = r.Open(wd)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("repository does not exist"))
	})

	t.Run("Open reopens an initialised repo", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		_ = newRepoIn(wd)

		r2, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		Expect(r2.Open(wd)).To(Succeed())
		Expect(r2.Workdir()).To(Equal(wd))
	})

	t.Run("package-level Open helper returns a usable Repo", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		_ = newRepoIn(wd)

		got, err := Open(wd)
		Expect(err).ToNot(HaveOccurred())
		Expect(got).ToNot(BeNil())
		Expect(got.Workdir()).To(Equal(wd))
	})

	t.Run("package-level Open helper errors on a non-repo", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		got, err := Open(wd)
		Expect(err).To(HaveOccurred())
		Expect(got).ToNot(BeNil()) // helper returns the partially-built repo alongside the error
	})

	t.Run("Open with an empty workdir takes the empty-root branch in initRepo", func(t *testing.T) {
		RegisterTestingT(t)
		// wd == "" exercises the else-branch that points wdFs at osfs.New("").
		// There is no repo at the empty path, so the open itself still fails.
		got, err := Open("")
		Expect(err).To(HaveOccurred())
		Expect(got).ToNot(BeNil())
		Expect(got.Workdir()).To(Equal(""))
	})
}

func TestInitOrOpen(t *testing.T) {
	RegisterTestingT(t)

	t.Run("InitOrOpen initialises when no repo exists", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		Expect(r.InitOrOpen(wd)).To(Succeed())
		Expect(r.Workdir()).To(Equal(wd))

		_, statErr := os.Stat(path.Join(wd, gogit.GitDirName))
		Expect(statErr).ToNot(HaveOccurred())
	})

	t.Run("InitOrOpen opens when a repo already exists", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		_ = newRepoIn(wd)

		r2, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		// First Init returns ErrRepositoryAlreadyExists internally, so InitOrOpen
		// must fall through to Open and still succeed.
		Expect(r2.InitOrOpen(wd)).To(Succeed())
		Expect(r2.Workdir()).To(Equal(wd))
	})

	t.Run("InitOrOpen propagates a non-already-exists Init error", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		asFile := filepath.Join(wd, "afile")
		Expect(os.WriteFile(asFile, []byte("x"), 0o644)).To(Succeed())

		r, err := newWithOpts()
		Expect(err).ToNot(HaveOccurred())
		// Init fails with something other than ErrRepositoryAlreadyExists, so
		// InitOrOpen must surface that error rather than attempting Open.
		err = r.InitOrOpen(asFile)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a directory"))
	})
}

func TestFileOperations(t *testing.T) {
	RegisterTestingT(t)

	t.Run("CreateFile + Exists round-trip in the worktree", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.Exists("nope.txt")).To(BeFalse())
		writeWorktreeFile(r, "present.txt", "data")
		Expect(r.Exists("present.txt")).To(BeTrue())

		// content actually landed on disk
		b, err := os.ReadFile(path.Join(wd, "present.txt"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(b)).To(Equal("data"))
	})

	t.Run("Exists returns false when Stat fails with a non-NotExist error", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		// "f" is a regular file, so Stat("f/child") yields ENOTDIR (not ENOENT),
		// exercising the non-IsNotExist error branch which also returns false.
		writeWorktreeFile(r, "f", "x")
		Expect(r.Exists("f/child")).To(BeFalse())
	})

	t.Run("CreateDir makes a directory and returns a chrooted billy.Dir", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		dir, err := r.CreateDir("nested/deep")
		Expect(err).ToNot(HaveOccurred())
		Expect(dir).ToNot(BeNil())

		info, statErr := os.Stat(path.Join(wd, "nested", "deep"))
		Expect(statErr).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
	})

	t.Run("CopyFile duplicates content into a new path", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "src.txt", "copy-me")
		Expect(r.CopyFile("src.txt", "dst.txt")).To(Succeed())

		b, err := os.ReadFile(path.Join(wd, "dst.txt"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(b)).To(Equal("copy-me"))
	})

	t.Run("CopyFile fails when the source is missing", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		err := r.CopyFile("ghost.txt", "dst.txt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to open file"))
	})

	t.Run("CopyFile fails when the destination cannot be opened", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "src.txt", "payload")
		// Make the destination path an existing directory so opening it as a file fails.
		_, err := r.CreateDir("dst-is-a-dir")
		Expect(err).ToNot(HaveOccurred())

		err = r.CopyFile("src.txt", "dst-is-a-dir")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to open file"))
	})

	t.Run("CreateDir fails when a path component is an existing file", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		// "blocker" is a regular file; trying to MkdirAll under it must fail.
		writeWorktreeFile(r, "blocker", "i am a file")
		_, err := r.CreateDir("blocker/child")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create dir"))
	})

	t.Run("OpenFile resolves a relative path against the worktree", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)
		writeWorktreeFile(r, "rel.txt", "relative")

		f, err := r.OpenFile("rel.txt", os.O_RDONLY, 0)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(f)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(b)).To(Equal("relative"))
	})

	// QUIRK (repo.go:74-75): for an absolute path OpenFile delegates to
	// osfs.New("").OpenFile(...). Under go-billy v5.9.0 the default ChrootOS has
	// root "" (a *relative* path), and its symlink-follow boundary check runs
	// filepath.Rel(".", <absolute path>) which always errors, so it returns
	// "chroot boundary crossed". The absolute-path branch therefore cannot
	// actually open any absolute file with this billy version; we assert that
	// observed behaviour (the branch is still exercised for coverage).
	t.Run("OpenFile wraps a tilde-expansion error for an unknown user", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		// "~unknown-user/..." routes through path_util.ReplaceTildeWithHome which
		// fails user.Lookup, so OpenFile must wrap that error.
		_, err := r.OpenFile("~no-such-user-deadbeef-9c4f/file", os.O_RDONLY, 0)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to replace path"))
	})

	t.Run("OpenFile with an absolute path hits the chroot boundary error", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		absPath := filepath.Join(t.TempDir(), "abs.txt")
		Expect(os.WriteFile(absPath, []byte("absolute"), 0o644)).To(Succeed())

		_, err := r.OpenFile(absPath, os.O_RDONLY, 0)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("boundary"))
	})
}

func TestGitOperations(t *testing.T) {
	RegisterTestingT(t)

	t.Run("AddFileToGit + Commit + Log + Hash + Branch full cycle", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "one.txt", "first content")
		Expect(r.AddFileToGit("one.txt")).To(Succeed())
		Expect(r.Commit("initial commit", CommitOpts{})).To(Succeed())

		log := r.Log()
		Expect(log).To(HaveLen(1))
		Expect(log[0].Message).To(Equal("initial commit"))
		// Author is taken from the commit's Author signature, which go-git fills
		// from the repo's local user config (NOT the committer the source hard-codes).
		Expect(log[0].Author).To(ContainSubstring("Test Author"))
		Expect(log[0].Author).To(ContainSubstring("author@test.local"))
		Expect(log[0].Hash).To(HaveLen(40))

		hash, err := r.Hash()
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(Equal(log[0].Hash))

		branch, err := r.Branch()
		Expect(err).ToNot(HaveOccurred())
		// go-git's default initial branch is "master".
		Expect(branch).To(Equal("master"))
	})

	t.Run("Commit with All:true stages tracked modifications", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "tracked.txt", "v1")
		Expect(r.AddFileToGit("tracked.txt")).To(Succeed())
		Expect(r.Commit("add tracked", CommitOpts{})).To(Succeed())

		// Modify the already-tracked file, then commit with All:true (no explicit Add).
		writeWorktreeFile(r, "tracked.txt", "v2")
		Expect(r.Commit("update tracked", CommitOpts{All: true})).To(Succeed())

		log := r.Log()
		Expect(log).To(HaveLen(2))
		messages := []string{log[0].Message, log[1].Message}
		Expect(messages).To(ConsistOf("add tracked", "update tracked"))
	})

	t.Run("Log returns multiple commits newest-first", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "a.txt", "a")
		Expect(r.AddFileToGit("a.txt")).To(Succeed())
		Expect(r.Commit("commit a", CommitOpts{})).To(Succeed())

		writeWorktreeFile(r, "b.txt", "b")
		Expect(r.AddFileToGit("b.txt")).To(Succeed())
		Expect(r.Commit("commit b", CommitOpts{})).To(Succeed())

		log := r.Log()
		Expect(log).To(HaveLen(2))
		Expect(log[0].Message).To(Equal("commit b"))
		Expect(log[1].Message).To(Equal("commit a"))
	})

	t.Run("Commit with nothing staged fails", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		// No staged changes: go-git refuses an empty commit.
		err := r.Commit("empty", CommitOpts{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to make commit"))
	})

	t.Run("AddFileToGit fails for a path that does not exist", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		err := r.AddFileToGit("does-not-exist.txt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to add file to git"))
	})

	t.Run("Hash and Branch error before any commit", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		_, err := r.Hash()
		Expect(err).To(HaveOccurred())

		_, err = r.Branch()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get HEAD reference"))
	})

	t.Run("Log on an empty repo returns no commits", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.Log()).To(BeEmpty())
	})

	t.Run("Branch resolves from a detached HEAD via reference scan", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "x.txt", "x")
		Expect(r.AddFileToGit("x.txt")).To(Succeed())
		Expect(r.Commit("only commit", CommitOpts{})).To(Succeed())

		hash, err := r.Hash()
		Expect(err).ToNot(HaveOccurred())

		// Detach HEAD onto the commit hash. The branch ref still points there, so
		// the reference-scan fallback in Branch() must recover "master".
		wt, err := r.gitRepo.Worktree()
		Expect(err).ToNot(HaveOccurred())
		Expect(wt.Checkout(&gogit.CheckoutOptions{Hash: plumbing.NewHash(hash)})).To(Succeed())

		branch, err := r.Branch()
		Expect(err).ToNot(HaveOccurred())
		Expect(branch).To(Equal("master"))
	})

	t.Run("Branch reports 'unable to determine' for a detached HEAD with no matching branch", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		writeWorktreeFile(r, "c1.txt", "1")
		Expect(r.AddFileToGit("c1.txt")).To(Succeed())
		Expect(r.Commit("c1", CommitOpts{})).To(Succeed())
		first, err := r.Hash()
		Expect(err).ToNot(HaveOccurred())

		writeWorktreeFile(r, "c2.txt", "2")
		Expect(r.AddFileToGit("c2.txt")).To(Succeed())
		Expect(r.Commit("c2", CommitOpts{})).To(Succeed())

		// Detach onto the FIRST commit; the only branch (master) points at the
		// second commit, so the reference scan finds no branch for HEAD's hash.
		wt, err := r.gitRepo.Worktree()
		Expect(err).ToNot(HaveOccurred())
		Expect(wt.Checkout(&gogit.CheckoutOptions{Hash: plumbing.NewHash(first)})).To(Succeed())

		_, err = r.Branch()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to determine current branch"))
	})
}

func TestIgnoreOperations(t *testing.T) {
	RegisterTestingT(t)

	t.Run("AddFileToIgnore creates .gitignore and appends entries", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.AddFileToIgnore("secrets.yaml")).To(Succeed())
		Expect(r.AddFileToIgnore("dist/")).To(Succeed())

		content, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
		lines := nonEmptyLines(string(content))
		Expect(lines).To(ConsistOf("secrets.yaml", "dist/"))
	})

	t.Run("AddFileToIgnore is idempotent for an already-ignored path", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.AddFileToIgnore("dup.txt")).To(Succeed())
		Expect(r.AddFileToIgnore("dup.txt")).To(Succeed())

		content, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nonEmptyLines(string(content))).To(ConsistOf("dup.txt"))
	})

	t.Run("RemoveFileFromIgnore drops only the matching line", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.AddFileToIgnore("keep.txt")).To(Succeed())
		Expect(r.AddFileToIgnore("remove.txt")).To(Succeed())

		Expect(r.RemoveFileFromIgnore("remove.txt")).To(Succeed())

		content, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
		lines := nonEmptyLines(string(content))
		Expect(lines).To(ConsistOf("keep.txt"))
		Expect(string(content)).ToNot(ContainSubstring("remove.txt"))
	})

	t.Run("RemoveFileFromIgnore on a fresh repo creates an empty .gitignore", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		// .gitignore does not exist yet; readIgnore must create it, and removing a
		// not-present entry simply yields an (essentially empty) file.
		Expect(r.RemoveFileFromIgnore("anything.txt")).To(Succeed())

		_, err := os.Stat(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("add then remove then add again round-trips", func(t *testing.T) {
		RegisterTestingT(t)
		wd := t.TempDir()
		r := newRepoIn(wd)

		Expect(r.AddFileToIgnore("cycle.txt")).To(Succeed())
		Expect(r.RemoveFileFromIgnore("cycle.txt")).To(Succeed())

		content, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nonEmptyLines(string(content))).ToNot(ContainElement("cycle.txt"))

		Expect(r.AddFileToIgnore("cycle.txt")).To(Succeed())
		content, err = os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nonEmptyLines(string(content))).To(ContainElement("cycle.txt"))
	})
}

func TestDetectRootDir(t *testing.T) {
	RegisterTestingT(t)

	// This test changes the process working directory, so it must NOT run in
	// parallel with anything and must always restore the original cwd.
	t.Run("WithDetectRootDir finds the repo root by walking up from cwd", func(t *testing.T) {
		RegisterTestingT(t)

		wd, err := filepath.EvalSymlinks(t.TempDir())
		Expect(err).ToNot(HaveOccurred())
		_ = newRepoIn(wd) // creates a real .git at wd

		orig, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = os.Chdir(orig) }()

		sub := filepath.Join(wd, "deeply", "nested")
		Expect(os.MkdirAll(sub, 0o755)).To(Succeed())
		Expect(os.Chdir(sub)).To(Succeed())

		r, err := newWithOpts(WithDetectRootDir())
		Expect(err).ToNot(HaveOccurred())
		Expect(r.workDir).To(Equal(wd))
		Expect(r.gitRepo).ToNot(BeNil())
	})

	t.Run("WithDetectRootDir errors when no git repo is found above cwd", func(t *testing.T) {
		RegisterTestingT(t)

		// A tempdir with no .git anywhere up the tree (its ancestors are /tmp/...).
		dir, err := filepath.EvalSymlinks(t.TempDir())
		Expect(err).ToNot(HaveOccurred())

		orig, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = os.Chdir(orig) }()
		Expect(os.Chdir(dir)).To(Succeed())

		_, err = newWithOpts(WithDetectRootDir())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect git dir"))
	})
}

// --- small helpers -----------------------------------------------------------

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
