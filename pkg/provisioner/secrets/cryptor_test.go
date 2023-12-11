package secrets

import (
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api"
	"api/pkg/provisioner/git"
	"api/pkg/provisioner/tests"
)

func withGitDir(gitDir string) Option {
	return Option{
		beforeInit: true,
		f: func(c *cryptor) error {
			repo, err := git.Open(c.workDir, git.WithGitDir(gitDir))
			if err != nil {
				return err
			}
			c.gitRepo = repo
			return nil
		},
	}
}

func TestNewCryptor(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name           string
		testExampleDir string
		opts           []Option
		actions        func(t *testing.T, c Cryptor, wd string)
		wantErr        string
	}{
		{
			name:           "happy path",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
			},
			actions: happyPathScenario,
		},
		{
			name:           "happy path with inline keys",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-inline"),
			},
			actions: happyPathScenario,
		},
		{
			name:           "happy path with profile",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithProfile("local-key-files"),
				WithKeysFromCurrentProfile(),
			},
			actions: happyPathScenario,
		},
		{
			name:           "bad workdir",
			testExampleDir: "testdata/non-existent-repo",
			wantErr:        "no such file or directory",
		},
		{
			name:           "bad git dir",
			testExampleDir: "testdata/repo",
			wantErr:        "failed to open git repository.*",
		},
		{
			name:           "with generated keys",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithProfile("test-profile"),
				WithGeneratedKeys("test-profile"),
			},
			actions: func(t *testing.T, c Cryptor, wd string) {
				happyPathScenario(t, c, wd)
				cfg, err := api.ReadConfigFile(wd, "test-profile")
				Expect(err).To(BeNil())
				Expect(cfg.PrivateKey).To(ContainSubstring(c.PrivateKey()))
				Expect(cfg.PublicKey).To(ContainSubstring(c.PublicKey()))
				Expect(cfg.PrivateKeyPath).To(Equal(""))
				Expect(cfg.PublicKeyPath).To(Equal(""))
			},
		},
		{
			name:           "with not existing profile",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("not-existing-profile"),
			},
			wantErr: "profile does not exist: \"not-existing-profile\"",
		},
		{
			name:           "public key not configured",
			testExampleDir: "testdata/repo",
			actions: func(t *testing.T, c Cryptor, wd string) {
				Expect(c.AddFile("stacks/common/secrets.yaml")).
					To(MatchError("public key is not configured"))
			},
		},
		{
			name:           "git repo not configured",
			testExampleDir: "testdata/repo",
			opts: []Option{
				WithKeysFromScConfig("local-key-files"),
			},
			wantErr: "git repo is not configured",
		},
	}
	t.Parallel()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			workDir, cleanup, err := tests.CopyTempProject(tt.testExampleDir)
			defer cleanup()

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
				return
			}

			got, err := NewCryptor(workDir, tt.opts...)

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				Expect(got).NotTo(BeNil())
			}

			if tt.actions != nil {
				tt.actions(t, got, workDir)
			}
		})
	}
}

func happyPathScenario(t *testing.T, c Cryptor, wd string) {
	oldSecretFile1Content, err := os.ReadFile("testdata/repo/stacks/common/secrets.yaml")
	Expect(err).To(BeNil())
	oldSecretFile2Content, err := os.ReadFile("testdata/repo/stacks/refapp/secrets.yaml")
	Expect(err).To(BeNil())
	commonSecretsFilePath := path.Join(wd, "stacks/common/secrets.yaml")
	refappSecretsFilePath := path.Join(wd, "stacks/refapp/secrets.yaml")

	t.Run("add file", func(t *testing.T) {
		Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())
		secrets := c.GetSecretFiles().Secrets
		Expect(secrets).NotTo(BeNil())
		Expect(secrets).To(HaveKey(c.PublicKey()))
		files := secrets[c.PublicKey()].Files
		Expect(files).To(HaveLen(1))
		Expect(files[0].Path).To(Equal("stacks/common/secrets.yaml"))
		Expect(files[0].EncryptedData).NotTo(BeEmpty())
		Expect(c.AddFile("stacks/refapp/secrets.yaml")).To(BeNil())
	})
	gitIgnoreFile := path.Join(wd, ".gitignore")
	t.Run("secrets added to gitignore", func(t *testing.T) {
		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(gitIgnoreFile)
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).To(ContainSubstring("stacks/common/secrets.yaml"))
		Expect(string(gitignoreContent)).To(ContainSubstring("stacks/refapp/secrets.yaml"))
	})
	t.Run("decrypt file", func(t *testing.T) {
		Expect(os.RemoveAll(commonSecretsFilePath)).To(BeNil())
		Expect(c.DecryptAll()).To(BeNil())
		newSecretFileContent, err := os.ReadFile(commonSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile1Content))

		newSecretFileContent, err = os.ReadFile(refappSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile2Content))
	})
	t.Run("remove file", func(t *testing.T) {
		Expect(c.RemoveFile("stacks/common/secrets.yaml")).To(BeNil())
		secrets := c.GetSecretFiles().Secrets
		Expect(secrets).NotTo(BeNil())
		Expect(secrets).To(HaveKey(c.PublicKey()))
		files := secrets[c.PublicKey()].Files
		Expect(files).To(HaveLen(1))

		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).NotTo(ContainSubstring("stacks/common/secrets.yaml"))
	})
	t.Run("secrets removed from gitignore", func(t *testing.T) {
		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).NotTo(ContainSubstring("stacks/common/secrets.yaml"))
		Expect(string(gitignoreContent)).To(ContainSubstring("stacks/refapp/secrets.yaml"))
	})
}
