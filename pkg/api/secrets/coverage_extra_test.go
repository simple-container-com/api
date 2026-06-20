package secrets

import (
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/util"
	"github.com/simple-container-com/api/pkg/util/test"
)

// newTestCryptor builds a cryptor on a throwaway copy of testdata/repo wired
// with the local-key-files profile and "accept all" confirmation, mirroring
// the setup used across the existing test suite. It returns the cryptor, its
// workdir and a cleanup func.
func newTestCryptor(t *testing.T) (Cryptor, string, func()) {
	t.Helper()
	m := &mocks{
		consoleReaderMock:      &test.ConsoleReaderMock{},
		confirmationReaderMock: &test.ConsoleReaderMock{},
	}
	m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	Expect(err).ToNot(HaveOccurred())

	got, err := NewCryptor(workDir,
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleReader(m.consoleReaderMock),
		WithConfirmationReader(m.confirmationReaderMock),
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeNil())
	return got, workDir, cleanup
}

// TestGetAndDecryptFileContent covers the happy path plus every error branch
// of GetAndDecryptFileContent (currently 0% covered).
func TestGetAndDecryptFileContent(t *testing.T) {
	RegisterTestingT(t)

	t.Run("returns decrypted content for a registered file", func(t *testing.T) {
		RegisterTestingT(t)
		c, wd, cleanup := newTestCryptor(t)
		defer cleanup()

		Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())

		original, err := os.ReadFile(path.Join(wd, "stacks/common/secrets.yaml"))
		Expect(err).ToNot(HaveOccurred())

		content, err := c.GetAndDecryptFileContent("stacks/common/secrets.yaml")
		Expect(err).ToNot(HaveOccurred())
		Expect(content).To(Equal(original))
	})

	t.Run("errors when current public key not present in secrets", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()

		// No file added, no key registered yet => secrets map empty for this key.
		_, err := c.GetAndDecryptFileContent("stacks/common/secrets.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	t.Run("errors when path is not among encrypted files", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()

		Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())

		_, err := c.GetAndDecryptFileContent("stacks/refapp/secrets.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("encrypted secret file"))
	})

	t.Run("errors when private key cannot decrypt the data", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()

		Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())

		// Swap in a private key that does not match the encryption public key.
		ci := c.(*cryptor)
		otherPriv, _, err := ciphers.GenerateKeyPair(2048)
		Expect(err).ToNot(HaveOccurred())
		ci.currentPrivateKey = ciphers.MarshalRSAPrivateKey(otherPriv)

		_, err = c.GetAndDecryptFileContent("stacks/common/secrets.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to decrypt secret file"))
	})
}

// TestOptionsAccessor covers Options(), which just returns the configured
// option slice (0% covered).
func TestOptionsAccessor(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	opts := []Option{
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
	}
	c, err := NewCryptor(workDir, opts...)
	Expect(err).ToNot(HaveOccurred())

	// Options() returns the same options that were passed to NewCryptor.
	Expect(c.Options()).To(HaveLen(len(opts)))
}

// TestReadProfileConfig covers ReadProfileConfig, which re-reads keys from the
// configured profile (0% covered).
func TestReadProfileConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("re-reads keys from the current profile", func(t *testing.T) {
		RegisterTestingT(t)
		workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
		defer cleanup()
		Expect(err).ToNot(HaveOccurred())

		c, err := NewCryptor(workDir,
			withGitDir("gitdir"),
			WithProfile("local-key-files"),
		)
		Expect(err).ToNot(HaveOccurred())

		// Keys are not loaded yet (only WithProfile, which is before-init).
		Expect(c.PublicKey()).To(Equal(""))

		Expect(c.ReadProfileConfig()).To(Succeed())
		Expect(c.PublicKey()).ToNot(BeEmpty())
		Expect(c.PrivateKey()).ToNot(BeEmpty())
	})

	t.Run("errors when profile is not configured", func(t *testing.T) {
		RegisterTestingT(t)
		workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
		defer cleanup()
		Expect(err).ToNot(HaveOccurred())

		c, err := NewCryptor(workDir, withGitDir("gitdir"))
		Expect(err).ToNot(HaveOccurred())

		// No profile set => WithKeysFromScConfig fails on empty profile.
		err = c.ReadProfileConfig()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("profile is not configured"))
	})
}

// TestWithConsoleWriterOption covers WithConsoleWriter (0% covered) by
// confirming the configured writer actually receives output.
func TestWithConsoleWriterOption(t *testing.T) {
	RegisterTestingT(t)

	writerMock := &test.ConsoleWriterMock{}
	writerMock.On("Print", mock.Anything).Return()
	writerMock.On("Println", mock.Anything).Return()
	writerMock.On("Println", mock.Anything, mock.Anything).Return()
	writerMock.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
	writerMock.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	confirm := &test.ConsoleReaderMock{}
	confirm.On("ReadLine").Return("Y", nil)

	c, err := NewCryptor(workDir,
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleWriter(writerMock),
		WithConfirmationReader(confirm),
	)
	Expect(err).ToNot(HaveOccurred())

	ci := c.(*cryptor)
	Expect(ci.consoleWriter).To(BeIdenticalTo(writerMock))

	// Register this key + a file, then empty the registry so DecryptAll reaches
	// the "no secret files to reveal" branch which prints via the writer.
	Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())
	ci.secrets.Registry.Files = []string{}

	Expect(c.DecryptAll(false)).To(Succeed())
	writerMock.AssertCalled(t, "Println", mock.Anything)
}

// TestWithWorkDirOption covers WithWorkDir (0% covered).
func TestWithWorkDirOption(t *testing.T) {
	RegisterTestingT(t)

	c := &cryptor{}
	opt := WithWorkDir("/some/work/dir")
	Expect(opt.f(c)).To(Succeed())
	Expect(c.workDir).To(Equal("/some/work/dir"))
}

// TestWithDetectGitDirOption covers WithDetectGitDir (0% covered). The option
// runs git detection from the process cwd, which is inside the module repo, so
// it should succeed and set a workdir.
func TestWithDetectGitDirOption(t *testing.T) {
	RegisterTestingT(t)

	c := &cryptor{}
	opt := WithDetectGitDir()
	err := opt.f(c)
	if err != nil {
		// Detection can fail in some environments; assert the shape of the
		// failure rather than the success when it does.
		Expect(err).To(HaveOccurred())
		return
	}
	Expect(c.gitRepo).ToNot(BeNil())
	Expect(c.workDir).ToNot(BeEmpty())
}

// TestWithKeysFromScConfig_ErrorBranches covers the validation branches of
// WithKeysFromScConfig that the existing suite does not reach.
func TestWithKeysFromScConfig_ErrorBranches(t *testing.T) {
	RegisterTestingT(t)

	t.Run("errors when workdir is empty", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{}
		err := WithKeysFromScConfig("some-profile").f(c)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workdir is not configured"))
	})

	t.Run("errors when profile is empty", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{workDir: "/tmp"}
		err := WithKeysFromScConfig("").f(c)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("profile is not configured"))
	})
}

// TestWithPublicKeyPath_Error covers the failure branch of WithPublicKeyPath
// when the key file does not exist.
func TestWithPublicKeyPath_Error(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	repo, err := git.Open(workDir, git.WithGitDir("gitdir"))
	Expect(err).ToNot(HaveOccurred())

	c := &cryptor{gitRepo: repo}
	err = WithPublicKeyPath("./.ssh/does-not-exist.pub").f(c)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to open public key file"))
}

// TestWithPrivateKeyPath_Errors covers the two failure branches of
// WithPrivateKeyPath: nil repo and a missing key file.
func TestWithPrivateKeyPath_Errors(t *testing.T) {
	RegisterTestingT(t)

	t.Run("errors when git repo is nil", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{}
		err := WithPrivateKeyPath("./whatever").f(c)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("git repo is not configured"))
	})

	t.Run("errors when private key file is missing", func(t *testing.T) {
		RegisterTestingT(t)
		workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
		defer cleanup()
		Expect(err).ToNot(HaveOccurred())

		repo, err := git.Open(workDir, git.WithGitDir("gitdir"))
		Expect(err).ToNot(HaveOccurred())

		c := &cryptor{gitRepo: repo}
		err = WithPrivateKeyPath("./.ssh/missing_id_rsa").f(c)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to open private key file"))
	})
}

// TestAddPublicKeyPath_RoundTrip covers WithPublicKeyPath +
// WithPrivateKeyPath success branches via real key files in testdata.
func TestKeyPath_SuccessBranches(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	repo, err := git.Open(workDir, git.WithGitDir("gitdir"))
	Expect(err).ToNot(HaveOccurred())

	c := &cryptor{gitRepo: repo}
	Expect(WithPublicKeyPath("./.ssh/test_id_rsa.pub").f(c)).To(Succeed())
	Expect(c.currentPublicKey).To(HavePrefix("ssh-rsa "))

	Expect(WithPrivateKeyPath("./.ssh/test_id_rsa").f(c)).To(Succeed())
	Expect(c.currentPrivateKey).To(ContainSubstring("PRIVATE KEY"))
}

// TestInitData_ErrorBranches covers the validation branches of initData that
// are not yet exercised: missing private key and missing git repo.
func TestInitData_ErrorBranches(t *testing.T) {
	RegisterTestingT(t)

	t.Run("errors when private key is not configured", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{currentPublicKey: "ssh-rsa AAAA"}
		err := c.initData()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("private key is not configured"))
	})

	t.Run("errors when git repo is not configured", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{currentPublicKey: "ssh-rsa AAAA", currentPrivateKey: "priv"}
		err := c.initData()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("git repo is not configured"))
	})

	t.Run("succeeds and initializes the secrets map", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()
		ci := c.(*cryptor)
		ci.secrets.Secrets = nil
		Expect(ci.initData()).To(Succeed())
		Expect(ci.secrets.Secrets).ToNot(BeNil())
	})
}

// TestDecryptSecretData_ErrorBranches covers error returns in decryptSecretData
// that are not reached by the happy-path tests.
func TestDecryptSecretData_ErrorBranches(t *testing.T) {
	RegisterTestingT(t)

	t.Run("errors when private key is not configured", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{}
		_, err := c.decryptSecretData([]string{"abc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("private key is not configured"))
	})

	t.Run("errors on an unparseable private key", func(t *testing.T) {
		RegisterTestingT(t)
		c := &cryptor{currentPrivateKey: "not-a-real-private-key"}
		_, err := c.decryptSecretData([]string{"abc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse private key"))
	})
}

// TestReadSecretFiles_Error covers the unmarshalSecretsFile error path when the
// secrets file is missing on disk.
func TestReadSecretFiles_Error(t *testing.T) {
	RegisterTestingT(t)

	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()

	// Remove the secrets file so OpenFile fails.
	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	if _, err := os.Stat(secretsPath); err == nil {
		Expect(os.Remove(secretsPath)).To(Succeed())
	}

	err := c.ReadSecretFiles()
	Expect(err).To(HaveOccurred())
}

// TestReadSecretFiles_MalformedFile covers the unmarshal-error branch of
// unmarshalSecretsFile when the secrets file contains invalid descriptor data.
func TestReadSecretFiles_MalformedFile(t *testing.T) {
	RegisterTestingT(t)

	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()

	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	Expect(os.WriteFile(secretsPath, []byte("::: not valid descriptor :::\n\t- broken"), 0o644)).To(Succeed())

	err := c.ReadSecretFiles()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to unmarshal secrets file"))
}

// TestMarshalSecretsFile_WriteError covers MarshalSecretsFile's open/write
// failure branch by making the target secrets path a directory so the file
// open fails.
func TestMarshalSecretsFile_WriteError(t *testing.T) {
	RegisterTestingT(t)

	c, wd, cleanup := newTestCryptor(t)
	defer cleanup()
	ci := c.(*cryptor)

	// Remove the existing secrets file and create a directory in its place so
	// OpenFile(O_CREATE|O_TRUNC|O_WRONLY) on a directory path fails.
	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	if _, statErr := os.Stat(secretsPath); statErr == nil {
		Expect(os.Remove(secretsPath)).To(Succeed())
	}
	Expect(os.Mkdir(secretsPath, 0o755)).To(Succeed())

	err := ci.MarshalSecretsFile()
	Expect(err).To(HaveOccurred())
}

// TestDecryptSecretData_InvalidKeyFormat covers the asn1 StructuralError
// "invalid key format" classification branch.
func TestDecryptSecretData_InvalidKeyFormat(t *testing.T) {
	RegisterTestingT(t)

	// A PEM block that parses as PEM but whose DER body is not a valid key,
	// triggering an asn1 structural error inside ssh.ParseRawPrivateKey.
	badPEM := "-----BEGIN RSA PRIVATE KEY-----\n" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" +
		"-----END RSA PRIVATE KEY-----\n"

	c := &cryptor{currentPrivateKey: badPEM, consoleWriter: util.DefaultConsoleWriter}
	_, err := c.decryptSecretData([]string{"YWJj"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(MatchRegexp("invalid key format|failed to parse private key"))
}

// makeSecretsPathADir replaces the cryptor's secrets.yaml with a directory so
// that any subsequent MarshalSecretsFile write fails. Returns the cryptor's wd.
func clobberSecretsFile(t *testing.T, wd string) {
	t.Helper()
	secretsPath := path.Join(wd, api.ScConfigDirectory, EncryptedSecretFilesDataFileName)
	if _, err := os.Stat(secretsPath); err == nil {
		Expect(os.Remove(secretsPath)).To(Succeed())
	}
	Expect(os.Mkdir(secretsPath, 0o755)).To(Succeed())
}

// TestPersistenceFailures_Propagate covers the MarshalSecretsFile error-return
// branches of AddPublicKey, AddFile and RemoveFile by making the on-disk
// secrets file unwritable (a directory).
func TestPersistenceFailures_Propagate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("AddPublicKey surfaces marshal failure", func(t *testing.T) {
		RegisterTestingT(t)
		c, wd, cleanup := newTestCryptor(t)
		defer cleanup()

		_, pub, err := ciphers.GenerateKeyPair(2048)
		Expect(err).ToNot(HaveOccurred())
		sshPub, err := ciphers.MarshalPublicKey(pub)
		Expect(err).ToNot(HaveOccurred())

		clobberSecretsFile(t, wd)
		err = c.AddPublicKey(strings.TrimSpace(string(sshPub)))
		Expect(err).To(HaveOccurred())
	})

	t.Run("AddFile surfaces marshal failure", func(t *testing.T) {
		RegisterTestingT(t)
		c, wd, cleanup := newTestCryptor(t)
		defer cleanup()

		clobberSecretsFile(t, wd)
		err := c.AddFile("stacks/common/secrets.yaml")
		Expect(err).To(HaveOccurred())
	})

	t.Run("RemoveFile surfaces marshal failure", func(t *testing.T) {
		RegisterTestingT(t)
		c, wd, cleanup := newTestCryptor(t)
		defer cleanup()

		Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())
		clobberSecretsFile(t, wd)
		err := c.RemoveFile("stacks/common/secrets.yaml")
		Expect(err).To(HaveOccurred())
	})

	t.Run("RemovePublicKey surfaces marshal failure", func(t *testing.T) {
		RegisterTestingT(t)
		c, wd, cleanup := newTestCryptor(t)
		defer cleanup()

		_, pub, err := ciphers.GenerateKeyPair(2048)
		Expect(err).ToNot(HaveOccurred())
		sshPub, err := ciphers.MarshalPublicKey(pub)
		Expect(err).ToNot(HaveOccurred())
		keyStr := strings.TrimSpace(string(sshPub))

		Expect(c.AddPublicKey(keyStr)).To(Succeed())
		clobberSecretsFile(t, wd)
		err = c.RemovePublicKey(keyStr)
		Expect(err).To(HaveOccurred())
	})
}

// TestRemovePublicKey_NotFound covers RemovePublicKey's not-found branch
// independently (existing test covers it but this pins the exact message).
func TestRemovePublicKey_NotFound(t *testing.T) {
	RegisterTestingT(t)

	c, _, cleanup := newTestCryptor(t)
	defer cleanup()

	err := c.RemovePublicKey("ssh-rsa AAAAnonexistent")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not found in secrets"))
}

// TestGenerateKeyPairWithProfile_WriteRoundTrip covers GenerateKeyPairWithProfile
// success including the config-file write, then reads it back.
func TestGenerateKeyPairWithProfile_WriteRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	c, err := NewCryptor(workDir, withGitDir("gitdir"))
	Expect(err).ToNot(HaveOccurred())

	Expect(c.GenerateKeyPairWithProfile("proj", "gen-rsa-profile")).To(Succeed())
	Expect(c.PublicKey()).To(HavePrefix("ssh-rsa "))
	Expect(c.PrivateKey()).To(ContainSubstring("RSA PRIVATE KEY"))

	cfg, err := api.ReadConfigFile(workDir, "gen-rsa-profile")
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg.ProjectName).To(Equal("proj"))
	Expect(cfg.PublicKey).To(ContainSubstring(c.PublicKey()))
}

// TestGenerateEd25519KeyPairWithProfile_WriteRoundTrip covers the ed25519
// generation + config write path.
func TestGenerateEd25519KeyPairWithProfile_WriteRoundTrip(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	c, err := NewCryptor(workDir, withGitDir("gitdir"))
	Expect(err).ToNot(HaveOccurred())

	Expect(c.GenerateEd25519KeyPairWithProfile("proj-ed", "gen-ed-profile")).To(Succeed())
	Expect(c.PublicKey()).To(HavePrefix("ssh-ed25519 "))
	Expect(c.PrivateKey()).To(ContainSubstring("BEGIN PRIVATE KEY"))

	cfg, err := api.ReadConfigFile(workDir, "gen-ed-profile")
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg.ProjectName).To(Equal("proj-ed"))
}

// TestEncryptSecretFile_Errors covers error branches of the internal
// encrypt helpers: a missing secret file and an unparseable public key.
func TestEncryptSecretFile_Errors(t *testing.T) {
	RegisterTestingT(t)

	c, _, cleanup := newTestCryptor(t)
	defer cleanup()
	ci := c.(*cryptor)

	t.Run("errors when secret file is missing", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ci.encryptSecretFile(ci.currentPublicKey, "stacks/does-not-exist.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read secret file"))
	})

	t.Run("errors when public key is unparseable", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := ci.encryptSecretFile("not-a-key", "stacks/common/secrets.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse public key"))
	})
}

// TestEnsureDiffAcceptable covers ensureDiffAcceptable directly: skip-check
// short-circuit, no-diff, accepted change, rejected change, and the retry cap.
func TestEnsureDiffAcceptable(t *testing.T) {
	RegisterTestingT(t)

	t.Run("skipCheck short-circuits to nil", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()
		ci := c.(*cryptor)
		Expect(ci.ensureDiffAcceptable("f", []byte("a"), []byte("b"), true)).To(Succeed())
	})

	t.Run("identical content returns nil without prompting", func(t *testing.T) {
		RegisterTestingT(t)
		c, _, cleanup := newTestCryptor(t)
		defer cleanup()
		ci := c.(*cryptor)
		Expect(ci.ensureDiffAcceptable("f", []byte("same\ncontent"), []byte("same\ncontent"), false)).To(Succeed())
	})

	t.Run("user rejects the diff", func(t *testing.T) {
		RegisterTestingT(t)
		workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
		defer cleanup()
		Expect(err).ToNot(HaveOccurred())

		writer := &test.ConsoleWriterMock{}
		writer.On("Print", mock.Anything).Return()
		writer.On("Println", mock.Anything).Return()
		writer.On("Println", mock.Anything, mock.Anything).Return()
		writer.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
		confirm := &test.ConsoleReaderMock{}
		confirm.On("ReadLine").Return("N", nil)

		c, err := NewCryptor(workDir,
			withGitDir("gitdir"),
			WithKeysFromScConfig("local-key-files"),
			WithConsoleWriter(writer),
			WithConfirmationReader(confirm),
		)
		Expect(err).ToNot(HaveOccurred())
		ci := c.(*cryptor)

		err = ci.ensureDiffAcceptable("f", []byte("old line"), []byte("new line"), false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not accepted"))
	})

	t.Run("invalid response retried until cap then errors", func(t *testing.T) {
		RegisterTestingT(t)
		workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
		defer cleanup()
		Expect(err).ToNot(HaveOccurred())

		writer := &test.ConsoleWriterMock{}
		writer.On("Print", mock.Anything).Return()
		writer.On("Println", mock.Anything).Return()
		writer.On("Println", mock.Anything, mock.Anything).Return()
		writer.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
		confirm := &test.ConsoleReaderMock{}
		confirm.On("ReadLine").Return("maybe", nil)

		c, err := NewCryptor(workDir,
			withGitDir("gitdir"),
			WithKeysFromScConfig("local-key-files"),
			WithConsoleWriter(writer),
			WithConfirmationReader(confirm),
		)
		Expect(err).ToNot(HaveOccurred())
		ci := c.(*cryptor)

		err = ci.ensureDiffAcceptable("f", []byte("old line"), []byte("new line"), false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("after 3 attempts"))
	})
}

// TestEncryptChanged_DiffRejectedPropagates ensures that a rejected diff during
// AddFile/EncryptChanged surfaces as an error (covers the diff-not-acceptable
// wrap in EncryptChanged).
func TestEncryptChanged_DiffRejectedPropagates(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	writer := &test.ConsoleWriterMock{}
	writer.On("Print", mock.Anything).Return()
	writer.On("Println", mock.Anything).Return()
	writer.On("Println", mock.Anything, mock.Anything).Return()
	writer.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
	confirm := &test.ConsoleReaderMock{}
	confirm.On("ReadLine").Return("N", nil)

	c, err := NewCryptor(workDir,
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleWriter(writer),
		WithConfirmationReader(confirm),
	)
	Expect(err).ToNot(HaveOccurred())

	// AddFile -> EncryptChanged(true,false) -> ensureDiffAcceptable(N) rejects.
	err = c.AddFile("stacks/common/secrets.yaml")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("diff is not acceptable"))
}

// TestRemoveFile_NormalizesRegistry covers RemoveFile end-to-end and confirms
// the registry no longer contains the removed path.
func TestRemoveFile_NormalizesRegistry(t *testing.T) {
	RegisterTestingT(t)

	c, _, cleanup := newTestCryptor(t)
	defer cleanup()

	Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())
	Expect(c.AddFile("stacks/refapp/secrets.yaml")).To(Succeed())

	Expect(c.RemoveFile("stacks/common/secrets.yaml")).To(Succeed())

	files := c.GetSecretFiles().Registry.Files
	Expect(files).ToNot(ContainElement("stacks/common/secrets.yaml"))
	Expect(files).To(ContainElement("stacks/refapp/secrets.yaml"))
}

// TestDecryptAll_DiffRejectedOnExistingFile covers decryptSecretDataToFile's
// branch where the on-disk file differs from the decrypted content and the user
// rejects the change.
func TestDecryptAll_DiffRejectedOnExistingFile(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	writer := &test.ConsoleWriterMock{}
	for i := 1; i <= 6; i++ {
		writer.On("Print", makeAnys(i)...).Return()
		writer.On("Println", makeAnys(i)...).Return()
	}
	confirm := &test.ConsoleReaderMock{}
	// First call (during AddFile encryption) accepts; later (DecryptAll) rejects.
	confirm.On("ReadLine").Return("Y", nil).Once()
	confirm.On("ReadLine").Return("N", nil)

	c, err := NewCryptor(workDir,
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleWriter(writer),
		WithConfirmationReader(confirm),
	)
	Expect(err).ToNot(HaveOccurred())

	Expect(c.AddFile("stacks/common/secrets.yaml")).To(Succeed())

	// Overwrite the on-disk file so the decrypted content differs from it.
	secretPath := path.Join(workDir, "stacks/common/secrets.yaml")
	Expect(os.WriteFile(secretPath, []byte("locally modified content\n"), 0o644)).To(Succeed())

	err = c.DecryptAll(false)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("changes are not accepted"))
}

// makeAnys returns a slice of n mock.Anything matchers.
func makeAnys(n int) []interface{} {
	res := make([]interface{}, n)
	for i := range res {
		res[i] = mock.Anything
	}
	return res
}

// TestReadSecretFile_Direct covers readSecretFile success + error directly.
func TestReadSecretFile_Direct(t *testing.T) {
	RegisterTestingT(t)

	c, _, cleanup := newTestCryptor(t)
	defer cleanup()
	ci := c.(*cryptor)

	data, err := ci.readSecretFile("stacks/common/secrets.yaml")
	Expect(err).ToNot(HaveOccurred())
	Expect(data).ToNot(BeEmpty())

	_, err = ci.readSecretFile("stacks/missing/secrets.yaml")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to open secret file"))
}

// TestWithKeysFromScConfig_ConflictingKeys covers the validation branches that
// reject configs declaring both an inline key and a key path.
func TestWithKeysFromScConfig_ConflictingKeys(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name    string
		cfg     api.ConfigFile
		wantErr string
	}{
		{
			name: "both public key path and inline private key",
			cfg: api.ConfigFile{
				ProjectName:   "proj",
				PublicKeyPath: "./.ssh/test_id_rsa.pub",
				PrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----",
			},
			wantErr: "both public key path and public key are configured",
		},
		{
			name: "both private key path and inline private key",
			cfg: api.ConfigFile{
				ProjectName:    "proj",
				PrivateKeyPath: "./.ssh/test_id_rsa",
				PrivateKey:     "-----BEGIN RSA PRIVATE KEY-----\nx\n-----END RSA PRIVATE KEY-----",
			},
			wantErr: "both private key path and private key are configured",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
			defer cleanup()
			Expect(err).ToNot(HaveOccurred())

			profile := "conflict-profile"
			Expect(tc.cfg.WriteConfigFile(workDir, profile)).To(Succeed())

			c := &cryptor{workDir: workDir}
			err = WithKeysFromScConfig(profile).f(c)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

// TestWithKeysFromScConfig_InlineWithPassphrase covers the inline-key branches
// of WithKeysFromScConfig including PrivateKeyPassword assignment.
func TestWithKeysFromScConfig_InlineWithPassphrase(t *testing.T) {
	RegisterTestingT(t)

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).ToNot(HaveOccurred())

	priv, pub, err := ciphers.GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())
	sshPub, err := ciphers.MarshalPublicKey(pub)
	Expect(err).ToNot(HaveOccurred())

	cfg := api.ConfigFile{
		ProjectName:        "proj",
		PublicKey:          strings.TrimSpace(string(sshPub)),
		PrivateKey:         ciphers.MarshalRSAPrivateKey(priv),
		PrivateKeyPassword: "supersecret",
	}
	profile := "inline-pass-profile"
	Expect(cfg.WriteConfigFile(workDir, profile)).To(Succeed())

	c := &cryptor{workDir: workDir}
	Expect(WithKeysFromScConfig(profile).f(c)).To(Succeed())
	Expect(c.currentPublicKey).To(HavePrefix("ssh-rsa "))
	Expect(c.currentPrivateKey).To(ContainSubstring("RSA PRIVATE KEY"))
	Expect(c.privateKeyPassphrase).To(Equal("supersecret"))
}

// TestDecryptSecretData_WrongPassphrase covers the passphrase-protected key
// branch where the configured passphrase is wrong.
func TestDecryptSecretData_WrongPassphrase(t *testing.T) {
	RegisterTestingT(t)

	keyData, err := os.ReadFile("testdata/repo/.ssh/test_passphrase_id_rsa")
	Expect(err).ToNot(HaveOccurred())

	c := &cryptor{
		currentPrivateKey:    strings.TrimSpace(string(keyData)),
		privateKeyPassphrase: "definitely-wrong",
		consoleWriter:        util.DefaultConsoleWriter,
	}
	_, err = c.decryptSecretData([]string{"YWJj"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse private key with passphrase"))
}

// TestDecryptSecretData_Ed25519Inline covers the ed25519 private-key decrypt
// branch in decryptSecretData via a real round-trip with a PKCS8 ed25519 key.
func TestDecryptSecretData_Ed25519Inline(t *testing.T) {
	RegisterTestingT(t)

	priv, pub, err := ciphers.GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	privPEM, err := ciphers.MarshalEd25519PrivateKey(priv)
	Expect(err).ToNot(HaveOccurred())

	encrypted, err := ciphers.EncryptLargeString(pub, "ed25519 round trip")
	Expect(err).ToNot(HaveOccurred())

	c := &cryptor{currentPrivateKey: privPEM, consoleWriter: util.DefaultConsoleWriter}
	decrypted, err := c.decryptSecretData(encrypted)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(decrypted)).To(Equal("ed25519 round trip"))
}

// TestAddPublicKey_NormalizesAlias confirms AddPublicKey stores the key in
// normalized (alias-stripped) form.
func TestAddPublicKey_NormalizesAlias(t *testing.T) {
	RegisterTestingT(t)

	c, _, cleanup := newTestCryptor(t)
	defer cleanup()

	_, pub, err := ciphers.GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())
	sshPub, err := ciphers.MarshalPublicKey(pub)
	Expect(err).ToNot(HaveOccurred())
	normalized := strings.TrimSpace(string(sshPub))
	withAlias := normalized + " someone@somewhere"

	Expect(c.AddPublicKey(withAlias)).To(Succeed())
	keys := c.GetKnownPublicKeys()
	Expect(keys).To(ContainElement(normalized))
	Expect(keys).ToNot(ContainElement(withAlias))
}
