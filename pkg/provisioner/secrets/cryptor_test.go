package secrets

import (
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/otiai10/copy"
)

const testPubKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCeVkFyudvqIp1rYrgPDpoYXJ0CtwYpGWrbUESK+ZDN22XflKmaSAMqHiuZ60NomuNv3uxjRU1acOYX0+BtwYrmTlH3COYmDR0z29d4ZjmTWa3H1z4Al/z1zOgrFxdDZ82MXTRXn478Mw/MCCQ1D4oGDNjwVKSan06FrSffE6aKKEZGPUC5BKRwMzkKeEZdFJCZifykd/7WXAIpXa9BLxL/FdjAFjPy8mRe1I2qRoPR2LRWReAukbpk1hOjS0OFiYLVhbE/jUAunlAUug/5D7OI7Q9P7/xL/kIlfuG+/tVQ3EFXkR9EX2RkRjD2C1vQrIEqu8Kdt/PnqzFfvs3KapGdUNxqlAo9tvBC4Q+8OJ4Y0vfHNIihhwecLBu3DQJJXXJZFIlactDmTYvhnNTt0T6DDPAv+aaw7SLTvBBZtwgi9eFbwYtlFVp2EMzBBlmpPsLtHlPAnnq7tOQihAGrBzO3iViV0Az9Q6as5P9Lor6Xeu71ke4xlmkRTSN0fi1sqLUy4s2srLIOLIykbIzeKujElJaSHumy+AD+x5SsZ6/WvgyLQyWR5cwPoM+yHslOZYTPdcfRmyEZTa5C4drytjFyvlR0yCwucPrJADtx6qJkfomDG+9xfd3zS/9GFr0LD+JMRFaBQhjQ7VHc/zIb3bNEEY9TCsmH5ie6hjKj5N6f0Q== test@localhost"

func TestNewCryptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
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
				WithGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir, err := copyTempProject(tt.testExampleDir)
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
	oldSecretFileContent, err := os.ReadFile("testdata/repo/stacks/common/secrets.yaml")
	Expect(err).To(BeNil())

	t.Run("add file", func(t *testing.T) {
		Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())
		secrets := c.GetSecretFiles().Secrets
		Expect(secrets).NotTo(BeNil())
		Expect(secrets).To(HaveKey(testPubKey))
		files := secrets[testPubKey].Files
		Expect(files).To(HaveLen(1))
		Expect(files[0].Path).To(Equal("stacks/common/secrets.yaml"))
		Expect(files[0].EncryptedData).NotTo(BeEmpty())
	})
	gitIgnoreFile := path.Join(wd, ".gitignore")
	t.Run("secrets.yaml added to gitignore", func(t *testing.T) {
		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(gitIgnoreFile)
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).To(ContainSubstring("stacks/common/secrets.yaml"))
	})
	t.Run("decrypt file", func(t *testing.T) {
		Expect(c.DecryptAll()).To(BeNil())
		newSecretFileContent, err := os.ReadFile(path.Join(wd, "stacks/common/secrets.yaml"))
		Expect(err).To(BeNil())
		Expect(newSecretFileContent).To(Equal(oldSecretFileContent))
	})
	t.Run("remove file", func(t *testing.T) {
		Expect(c.RemoveFile("stacks/common/secrets.yaml")).To(BeNil())
		secrets := c.GetSecretFiles().Secrets
		Expect(secrets).NotTo(BeNil())
		Expect(secrets).To(HaveKey(testPubKey))
		files := secrets[testPubKey].Files
		Expect(files).To(HaveLen(0))

		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(path.Join(wd, ".gitignore"))
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).To(ContainSubstring("stacks/common/secrets.yaml"))
	})
	t.Run("secrets.yaml removed from gitignore", func(t *testing.T) {
		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile("testdata/repo/stacks/common/secrets.yaml")
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).NotTo(ContainSubstring("stacks/common/secrets.yaml"))
	})
}

func copyTempProject(pathToExample string) (string, error) {
	if depDir, err := os.MkdirTemp(os.TempDir(), "project"); err != nil {
		return pathToExample, err
	} else if err = copy.Copy(pathToExample, depDir); err != nil {
		return pathToExample, err
	} else {
		return depDir, nil
	}
}
