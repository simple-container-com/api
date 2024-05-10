package git

import (
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/tests/testutil"
)

func TestOpenRepo(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name           string
		testExampleDir string
		opts           []Option
		wantErr        string
		actions        func(t *testing.T, g Repo, wd string)
	}{
		{
			name:           "happy path",
			testExampleDir: "testdata/repo",
			opts: []Option{
				WithGitDir("gitdir"),
			},
			actions: func(t *testing.T, g Repo, wd string) {
				Expect(g.Exists("stacks/refapp/secrets.yaml")).To(BeTrue())
				Expect(g.AddFileToIgnore("stacks/refapp/secrets.yaml")).To(BeNil())

				gitIgnoreFile := path.Join(wd, ".gitignore")
				content, err := os.ReadFile(gitIgnoreFile)
				Expect(err).To(BeNil())
				Expect(content).To(ContainSubstring("stacks/refapp/secrets.yaml"))
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
			wantErr:        "repository does not exist",
		},
		{
			name:           "file not found",
			testExampleDir: "testdata/repo",
			opts: []Option{
				WithGitDir("gitdir"),
			},
			actions: func(t *testing.T, g Repo, wd string) {
				Expect(g.Exists("stacks/refapp/not-existing.yaml")).To(BeFalse())
			},
		},
	}
	t.Parallel()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			workDir, cleanup, err := testutil.CopyTempProject(tt.testExampleDir)
			defer cleanup()

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
				return
			}

			got, err := Open(workDir, tt.opts...)

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
