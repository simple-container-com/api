package provisioner

import (
	"context"
	"os"
	"path"
	"testing"

	"api/pkg/provisioner/models"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"api/pkg/api/tests"
	"api/pkg/provisioner/git"
	testutils "api/pkg/provisioner/tests"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name         string
		params       ProvisionParams
		opts         []Option
		expectStacks models.StacksMap
		wantErr      string
	}{
		{
			name: "happy path",
			params: ProvisionParams{
				RootDir: "testdata/stacks",
				Stacks: []string{
					"common",
					"refapp",
				},
			},
			expectStacks: map[string]models.Stack{
				"common": {
					Name:    "common",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.CommonServerDescriptor,
				},
				"refapp": {
					Name:   "refapp",
					Server: *tests.RefappServerDescriptor,
					Client: *tests.RefappClientDescriptor,
				},
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			p, err := New(tt.opts...)

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
			}

			err = p.Provision(ctx, tt.params)

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				if tt.expectStacks != nil {
					Expect(p.Stacks()).To(Equal(tt.expectStacks))
				}
			}
		})
	}
}

func Test_Init(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name    string
		params  InitParams
		opts    []Option
		init    func(wd string) Provisioner
		check   func(t *testing.T, wd string, p Provisioner)
		wantErr string
	}{
		{
			name: "happy path",
			params: InitParams{
				RootDir: "testdata/refapp",
			},
			check: checkInitSuccess,
		},
		{
			name: "existing repo no error",
			params: InitParams{
				RootDir: "testdata/refapp-existing-gitdir",
			},
			init: func(wd string) Provisioner {
				gitRepo, err := git.New(git.WithGitDir("gitdir"), git.WithRootDir(wd))
				Expect(err).To(BeNil())
				p, err := New(WithGitRepo(gitRepo))
				Expect(err).To(BeNil())
				return p
			},
			check: checkInitSuccess,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			workDir, cleanup, err := testutils.CopyTempProject(tt.params.RootDir)
			defer cleanup()

			testutils.CheckError(err, tt.wantErr)

			var p Provisioner
			if tt.init != nil {
				p = tt.init(workDir)
			} else {
				p, err = New(tt.opts...)
			}

			testutils.CheckError(err, tt.wantErr)

			// overwrite root dir to temp
			tt.params.RootDir = workDir

			Expect(err).To(BeNil())
			err = p.Init(ctx, tt.params)
			testutils.CheckError(err, tt.wantErr)

			if tt.check != nil {
				tt.check(t, workDir, p)
			}
		})
	}
}

func checkInitSuccess(t *testing.T, wd string, p Provisioner) {
	t.Run("initial commit is present", func(t *testing.T) {
		commits := p.GitRepo().Log()
		commit, exists := lo.Find(commits, func(c git.Commit) bool {
			return c.Message == "simple-container.com initial commit"
		})
		Expect(exists).To(BeTrue())
		Expect(commit.Message).To(Equal("simple-container.com initial commit"))
	})

	t.Run("profile file created", func(t *testing.T) {
		profileFile := path.Join(wd, ".sc/cfg.default.yaml")
		Expect(profileFile).To(BeAnExistingFile())
	})
	t.Run("profile added to gitignore", func(t *testing.T) {
		gitIgnoreFile := path.Join(wd, ".gitignore")
		Expect(gitIgnoreFile).To(BeAnExistingFile())
		gitignoreContent, err := os.ReadFile(gitIgnoreFile)
		Expect(err).To(BeNil())
		Expect(string(gitignoreContent)).To(ContainSubstring("\n.sc/cfg.default.yaml"))
	})
}
