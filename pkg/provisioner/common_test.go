package provisioner

import (
	"context"
	"os"
	"path"
	"testing"

	pulumi_mocks "api/pkg/provisioner/pulumi/mocks"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"

	"api/pkg/api"
	"api/pkg/provisioner/logger"
	"api/pkg/provisioner/placeholders"

	"github.com/onsi/gomega/format"

	"api/pkg/provisioner/models"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"api/pkg/api/tests"
	"api/pkg/provisioner/git"
	testutils "api/pkg/provisioner/tests"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)
	format.MaxLength = 10000
	testCases := []struct {
		name         string
		params       ProvisionParams
		init         func(t *testing.T, ctx context.Context) (Provisioner, error)
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
					Server:  *tests.ResolvedCommonServerDescriptor,
					Client:  api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{}},
				},
				"refapp": {
					Name:    "refapp",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedRefappServerDescriptor,
					Client:  *tests.RefappClientDescriptor,
				},
			},
		},
		{
			name: "pulumi error",
			params: ProvisionParams{
				RootDir: "testdata/stacks",
				Stacks: []string{
					"common",
					"refapp",
				},
			},
			init: func(t *testing.T, ctx context.Context) (Provisioner, error) {
				pulumiMock := pulumi_mocks.NewPulumiMock(t)
				pulumiMock.On("CreateStacks", ctx, mock.Anything, mock.Anything).
					Return(errors.New("failed to create stacks"))
				return New(
					WithPlaceholders(placeholders.New(logger.New())),
					WithPulumi(pulumiMock),
				)
			},
			wantErr: "failed to create stacks",
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			var p Provisioner
			var err error
			if tt.init != nil {
				p, err = tt.init(t, ctx)
			} else {
				if len(tt.opts) == 0 {
					pulumiMock := pulumi_mocks.NewPulumiMock(t)
					pulumiMock.On("CreateStacks", ctx, mock.Anything, mock.Anything).Return(nil)
					tt.opts = []Option{
						WithPlaceholders(placeholders.New(logger.New())),
						WithPulumi(pulumiMock),
					}
				}
				p, err = New(tt.opts...)
			}

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
			}

			err = p.Provision(ctx, tt.params)

			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				if tt.expectStacks != nil {
					for stackName := range tt.expectStacks {
						Expect(p.Stacks()[stackName]).To(Equal(tt.expectStacks[stackName]))
					}
				}
			}
		})
	}
}

func Test_Init(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name        string
		params      InitParams
		opts        []Option
		init        func(wd string) Provisioner
		check       func(t *testing.T, wd string, p Provisioner)
		wantInitErr string
		wantAnyErr  string
	}{
		{
			name: "happy path",
			params: InitParams{
				ProjectName: "test-project",
				RootDir:     "testdata/refapp",
			},
			opts:  []Option{WithPlaceholders(placeholders.New(logger.New()))},
			check: checkInitSuccess,
		},
		{
			name: "existing repo no error",
			params: InitParams{
				ProjectName: "test-project",
				RootDir:     "testdata/refapp-existing-gitdir",
			},
			init: func(wd string) Provisioner {
				gitRepo, err := git.New(git.WithGitDir("gitdir"), git.WithRootDir(wd))
				Expect(err).To(BeNil())
				p, err := New(WithGitRepo(gitRepo), WithPlaceholders(placeholders.New(logger.New())))
				Expect(err).To(BeNil())
				return p
			},
			check: checkInitSuccess,
		},
		{
			name: "project name is not set",
			params: InitParams{
				RootDir: "testdata/refapp",
			},
			wantInitErr: "project name is not configured",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			workDir, cleanup, err := testutils.CopyTempProject(tt.params.RootDir)
			defer cleanup()

			testutils.CheckError(err, tt.wantAnyErr)

			var p Provisioner
			if tt.init != nil {
				p = tt.init(workDir)
			} else {
				p, err = New(tt.opts...)
			}

			testutils.CheckError(err, tt.wantAnyErr)

			// overwrite root dir to temp
			tt.params.RootDir = workDir

			err = p.Init(ctx, tt.params)
			testutils.CheckError(err, tt.wantInitErr)

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
