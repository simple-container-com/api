package secrets

import (
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/util/test"
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

type mocks struct {
	consoleReaderMock      *test.ConsoleReaderMock
	confirmationReaderMock *test.ConsoleReaderMock
}

func TestNewCryptor(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name           string
		testExampleDir string
		opts           []Option
		prepareMocks   func(m *mocks)
		actions        func(t *testing.T, c Cryptor, m *mocks, wd string)
		wantErr        string
	}{
		{
			name:           "happy path",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
			},
			prepareMocks: acceptAllChanges,
			actions:      happyPathScenario,
		},
		{
			name:           "happy path with inline keys",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-inline"),
			},
			prepareMocks: acceptAllChanges,
			actions:      happyPathScenario,
		},
		{
			name:           "happy path with passphrase",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files-passphrase"),
			},
			prepareMocks: acceptAllChanges,
			actions:      happyPathScenario,
		},
		{
			name:           "happy path with profile",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithProfile("local-key-files"),
				WithKeysFromCurrentProfile(),
			},
			prepareMocks: acceptAllChanges,
			actions:      happyPathScenario,
		},
		{
			name:           "happy path with invalid passphrase from console",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files-no-passphrase"),
			},
			prepareMocks: acceptAllChanges,
			actions: func(t *testing.T, c Cryptor, m *mocks, wd string) {
				m.consoleReaderMock.On("ReadLine").Return("invalid-passphrase", nil)
				Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())
				Expect(c.EncryptChanged(true, false)).To(BeNil())
				err := c.DecryptAll(false)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(MatchRegexp(".*failed to parse private key with passphrase.*"))
			},
		},
		{
			name:           "happy path with valid passphrase from console",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files-no-passphrase"),
			},
			prepareMocks: acceptAllChanges,
			actions: func(t *testing.T, c Cryptor, m *mocks, wd string) {
				m.consoleReaderMock.On("ReadLine").Return("test", nil)
				Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())
				Expect(c.EncryptChanged(true, false)).To(BeNil())
				err := c.DecryptAll(false)
				Expect(err).To(BeNil())
			},
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
				WithGeneratedKeys("test-project", "test-profile"),
			},
			prepareMocks: acceptAllChanges,
			actions: func(t *testing.T, c Cryptor, m *mocks, wd string) {
				happyPathScenario(t, c, m, wd)
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
			actions: func(t *testing.T, c Cryptor, m *mocks, wd string) {
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
		{
			name:           "happy path with ed25519 keys",
			testExampleDir: "testdata/repo",
			opts: []Option{
				withGitDir("gitdir"),
				WithProfile("test-profile"),
				WithGeneratedEd25519Keys("test-project", "test-profile"),
			},
			prepareMocks: acceptAllChanges,
			actions:      happyPathEd25519Scenario,
		},
	}
	t.Parallel()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks{
				consoleReaderMock:      &test.ConsoleReaderMock{},
				confirmationReaderMock: &test.ConsoleReaderMock{},
			}

			workDir, cleanup, err := testutil.CopyTempProject(tt.testExampleDir)
			defer cleanup()

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
				return
			}

			tt.opts = append(tt.opts, WithConsoleReader(m.consoleReaderMock), WithConfirmationReader(m.confirmationReaderMock))

			got, err := NewCryptor(workDir, tt.opts...)

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				Expect(got).NotTo(BeNil())
			}

			if tt.prepareMocks != nil {
				tt.prepareMocks(m)
			}

			if tt.actions != nil {
				tt.actions(t, got, m, workDir)
			}
		})
	}
}

func acceptAllChanges(m *mocks) {
	m.confirmationReaderMock.On("ReadLine").Return("Y", nil)
}

func happyPathScenario(t *testing.T, c Cryptor, m *mocks, wd string) {
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
		Expect(c.DecryptAll(false)).To(BeNil())
		newSecretFileContent, err := os.ReadFile(commonSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile1Content))

		newSecretFileContent, err = os.ReadFile(refappSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile2Content))
	})

	anotherPrivKeyBytes, anotherPubKeyBytes, err := ciphers.GenerateKeyPair(2048)
	Expect(err).To(BeNil())
	anotherPubKey, err := ciphers.MarshalPublicKey(anotherPubKeyBytes)
	Expect(err).To(BeNil())
	anotherPrivKey := ciphers.MarshalRSAPrivateKey(anotherPrivKeyBytes)

	anotherPubKeyString := strings.TrimSpace(string(anotherPubKey))

	t.Run("allow another key", func(t *testing.T) {
		Expect(c.AddPublicKey(anotherPubKeyString)).To(BeNil())
		Expect(c.ReadSecretFiles()).To(BeNil())
		knownKeys := c.GetKnownPublicKeys()
		Expect(knownKeys).To(ContainElement(c.PublicKey()))
		Expect(knownKeys).To(ContainElement(anotherPubKeyString))
	})

	// clone to another dir
	anotherC, cleanup, err := cloneWorkdir(c, wd, anotherPubKeyString, anotherPrivKey, m)
	Expect(err).To(BeNil())
	defer cleanup()

	t.Run("decrypt secrets in another dir", func(t *testing.T) {
		Expect(anotherC.PrivateKey()).To(Equal(anotherPrivKey))
		Expect(anotherC.ReadSecretFiles()).To(BeNil())
		knownKeys := anotherC.GetKnownPublicKeys()
		Expect(knownKeys).To(ContainElement(c.PublicKey()))
		Expect(knownKeys).To(ContainElement(anotherPubKeyString))
		Expect(anotherC.DecryptAll(false)).To(BeNil())

		newSecretFileContent, err := os.ReadFile(path.Join(anotherC.Workdir(), "stacks/common/secrets.yaml"))
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile1Content))

		newSecretFileContent, err = os.ReadFile(path.Join(anotherC.Workdir(), "stacks/refapp/secrets.yaml"))
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile2Content))
	})

	t.Run("do not re-encrypt if no changes", func(t *testing.T) {
		prevEncrypted := c.GetSecretFiles().Secrets[c.PublicKey()].Files
		Expect(c.EncryptChanged(false, false)).To(BeNil())
		newEncrypted := c.GetSecretFiles().Secrets[c.PublicKey()].Files
		Expect(prevEncrypted).To(Equal(newEncrypted))
	})

	t.Run("disallow another key", func(t *testing.T) {
		Expect(c.GetKnownPublicKeys()).To(ContainElement(anotherPubKeyString))
		Expect(c.RemovePublicKey(anotherPubKeyString)).To(BeNil())
		Expect(c.GetKnownPublicKeys()).NotTo(ContainElement(anotherPubKeyString))
	})

	// clone to another dir
	anotherC, cleanup, err = cloneWorkdir(c, wd, anotherPubKeyString, anotherPrivKey, m)
	Expect(err).To(BeNil())
	defer cleanup()

	t.Run("fail to decrypt secrets in another dir", func(t *testing.T) {
		Expect(anotherC.PrivateKey()).To(Equal(anotherPrivKey))
		Expect(anotherC.ReadSecretFiles()).To(BeNil())
		knownKeys := anotherC.GetKnownPublicKeys()
		Expect(knownKeys).To(ContainElement(c.PublicKey()))
		Expect(knownKeys).NotTo(ContainElement(anotherPubKeyString))
		decryptErr := anotherC.DecryptAll(false)
		Expect(decryptErr).NotTo(BeNil())
		Expect(decryptErr.Error()).To(MatchRegexp("current public key .* is not found in secrets"))
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

func cloneWorkdir(c Cryptor, wd, pubKey, privKey string, m *mocks) (Cryptor, func(), error) {
	anotherDir, cleanup, err := testutil.CopyTempProject(wd)
	Expect(err).To(BeNil())
	anotherGitRepo, err := git.New(git.WithRootDir(anotherDir))
	Expect(err).To(BeNil())
	Expect(anotherGitRepo.Open(anotherDir, git.WithGitDir(c.GitRepo().Gitdir()))).To(BeNil())
	anotherC, err := NewCryptor(anotherDir,
		WithPublicKey(pubKey),
		WithPrivateKey(privKey),
		WithGitRepo(anotherGitRepo),
		WithConfirmationReader(m.confirmationReaderMock),
		WithConsoleReader(m.consoleReaderMock),
	)
	return anotherC, cleanup, err
}

func happyPathEd25519Scenario(t *testing.T, c Cryptor, m *mocks, wd string) {
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
		Expect(c.DecryptAll(false)).To(BeNil())
		newSecretFileContent, err := os.ReadFile(commonSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile1Content))

		newSecretFileContent, err = os.ReadFile(refappSecretsFilePath)
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile2Content))
	})

	// Test ed25519 key compatibility by generating another ed25519 key pair
	anotherEd25519PrivKey, anotherEd25519PubKey, err := ciphers.GenerateEd25519KeyPair()
	Expect(err).To(BeNil())
	anotherEd25519PubKeySSH, err := ciphers.MarshalEd25519PublicKey(anotherEd25519PubKey)
	Expect(err).To(BeNil())
	anotherEd25519PrivKeyPEM, err := ciphers.MarshalEd25519PrivateKey(anotherEd25519PrivKey)
	Expect(err).To(BeNil())

	anotherEd25519PubKeyString := strings.TrimSpace(string(anotherEd25519PubKeySSH))

	t.Run("allow another ed25519 key", func(t *testing.T) {
		Expect(c.AddPublicKey(anotherEd25519PubKeyString)).To(BeNil())
		Expect(c.ReadSecretFiles()).To(BeNil())
		knownKeys := c.GetKnownPublicKeys()
		Expect(knownKeys).To(ContainElement(c.PublicKey()))
		Expect(knownKeys).To(ContainElement(anotherEd25519PubKeyString))
	})

	// clone to another dir with ed25519 key
	anotherC, cleanup, err := cloneWorkdir(c, wd, anotherEd25519PubKeyString, anotherEd25519PrivKeyPEM, m)
	Expect(err).To(BeNil())
	defer cleanup()

	t.Run("decrypt secrets in another dir with ed25519", func(t *testing.T) {
		Expect(anotherC.PrivateKey()).To(Equal(anotherEd25519PrivKeyPEM))
		Expect(anotherC.ReadSecretFiles()).To(BeNil())
		knownKeys := anotherC.GetKnownPublicKeys()
		Expect(knownKeys).To(ContainElement(c.PublicKey()))
		Expect(knownKeys).To(ContainElement(anotherEd25519PubKeyString))
		Expect(anotherC.DecryptAll(false)).To(BeNil())

		newSecretFileContent, err := os.ReadFile(path.Join(anotherC.Workdir(), "stacks/common/secrets.yaml"))
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile1Content))

		newSecretFileContent, err = os.ReadFile(path.Join(anotherC.Workdir(), "stacks/refapp/secrets.yaml"))
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFile2Content))
	})

	t.Run("verify ed25519 key format in config", func(t *testing.T) {
		// Verify the generated ed25519 keys are properly formatted
		Expect(c.PublicKey()).To(HavePrefix("ssh-ed25519 "))
		Expect(c.PrivateKey()).To(ContainSubstring("-----BEGIN PRIVATE KEY-----"))
		Expect(c.PrivateKey()).To(ContainSubstring("-----END PRIVATE KEY-----"))
	})

	t.Run("do not re-encrypt if no changes", func(t *testing.T) {
		prevEncrypted := c.GetSecretFiles().Secrets[c.PublicKey()].Files
		Expect(c.EncryptChanged(false, false)).To(BeNil())
		newEncrypted := c.GetSecretFiles().Secrets[c.PublicKey()].Files
		Expect(prevEncrypted).To(Equal(newEncrypted))
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
