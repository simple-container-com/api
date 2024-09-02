package provisioner

import (
	"context"
	"os"
	"path"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/git"
	git_mocks "github.com/simple-container-com/api/pkg/api/git/mocks"
	"github.com/simple-container-com/api/pkg/api/tests"
	"github.com/simple-container-com/api/pkg/api/tests/testutil"
	pulumi_mocks "github.com/simple-container-com/api/pkg/clouds/pulumi/mocks"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)
	format.MaxLength = 10000
	testCases := []struct {
		name         string
		params       api.ProvisionParams
		init         func(t *testing.T, ctx context.Context, gitRepo git.Repo) (Provisioner, error)
		opts         []Option
		expectStacks api.StacksMap
		wantErr      string
	}{
		{
			name: "happy path gcp",
			params: api.ProvisionParams{
				StacksDir: "stacks",
				Stacks: []string{
					"common",
					"refapp",
				},
			},
			expectStacks: map[string]api.Stack{
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
					Client:  *tests.ResolvedRefappClientDescriptor("testdata"),
				},
			},
		},
		{
			name: "happy path aws",
			params: api.ProvisionParams{
				StacksDir: "stacks",
				Stacks: []string{
					"common",
					"refapp-aws",
				},
			},
			expectStacks: map[string]api.Stack{
				"common": {
					Name:    "common",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedCommonServerDescriptor,
					Client:  api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{}},
				},
				"refapp-aws": {
					Name:    "refapp-aws",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedRefappAwsServerDescriptor,
					Client:  *tests.ResolvedRefappCloudClientDescriptor("testdata", tests.RefappAwsClientDescriptor),
				},
			},
		},
		{
			name: "happy path gcp static",
			params: api.ProvisionParams{
				StacksDir: "stacks",
				Stacks: []string{
					"common",
					"refapp-static-gcp",
				},
			},
			expectStacks: map[string]api.Stack{
				"common": {
					Name:    "common",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedCommonServerDescriptor,
					Client:  api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{}},
				},
				"refapp-static-gcp": {
					Name:    "refapp-static-gcp",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedRefappStaticGCPServerDescriptor,
					Client:  *tests.RefappStaticGCPClientDescriptor,
				},
			},
		},
		{
			name: "happy path gcp gke-autopilot",
			params: api.ProvisionParams{
				StacksDir: "stacks",
				Stacks: []string{
					"common",
					"refapp-gke-autopilot",
				},
			},
			expectStacks: map[string]api.Stack{
				"common": {
					Name:    "common",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedCommonServerDescriptor,
					Client:  api.ClientDescriptor{Stacks: map[string]api.StackClientDescriptor{}},
				},
				"refapp-gke-autopilot": {
					Name:    "refapp-gke-autopilot",
					Secrets: *tests.CommonSecretsDescriptor,
					Server:  *tests.ResolvedRefappGkeAutopilotServerDescriptor,
					Client:  *tests.ResolvedRefappCloudClientDescriptor("testdata", tests.RefappGkeAutopilotClientDescriptor),
				},
			},
		},
		{
			name: "pulumi error",
			params: api.ProvisionParams{
				StacksDir: "stacks",
				Stacks: []string{
					"common",
					"refapp",
				},
			},
			init: func(t *testing.T, ctx context.Context, gitRepo git.Repo) (Provisioner, error) {
				pulumiMock := pulumi_mocks.NewPulumiMock(t)
				pulumiMock.On("ProvisionStack", ctx, mock.Anything, mock.Anything, mock.Anything).
					Return(errors.New("failed to create stacks"))
				pulumiMock.On("SetPublicKey", mock.Anything).Return()
				return New(
					WithPlaceholders(placeholders.New()),
					WithOverrideProvisioner(pulumiMock),
					WithGitRepo(gitRepo),
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
			gitRepoMock := git_mocks.NewGitRepoMock(t)
			gitRepoMock.On("Workdir").Return("testdata")

			if tt.init != nil {
				p, err = tt.init(t, ctx, gitRepoMock)
			} else {
				if len(tt.opts) == 0 {
					pulumiMock := pulumi_mocks.NewPulumiMock(t)
					pulumiMock.On("ProvisionStack", ctx, mock.Anything, mock.Anything, mock.Anything).
						Return(nil)
					pulumiMock.On("SetPublicKey", mock.Anything).Return()
					tt.opts = []Option{
						WithGitRepo(gitRepoMock),
						WithPlaceholders(placeholders.New(placeholders.WithGitRepo(gitRepoMock))),
						WithOverrideProvisioner(pulumiMock),
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
						actual := p.Stacks()[stackName]
						expected := tt.expectStacks[stackName]
						assert.EqualValuesf(t, expected, actual.ValuesOnly(), "%v/%v failed", tt.name, stackName)
					}
				}
			}
		})
	}
}

func Test_Deploy(t *testing.T) {
	RegisterTestingT(t)
	format.MaxLength = 10000
	testCases := []struct {
		name            string
		params          api.DeployParams
		verify          func(t *testing.T, ttName string, pulumiMock *pulumi_mocks.PulumiMock, err error)
		setExpectations bool
		wantErr         string
	}{
		{
			name: "happy path staging gcp",
			params: api.DeployParams{
				StackParams: api.StackParams{
					StacksDir:   "stacks",
					StackName:   "refapp",
					Environment: "staging",
				},
			},
			setExpectations: true,
			verify: func(t *testing.T, ttName string, pulumiMock *pulumi_mocks.PulumiMock, err error) {
				pulumiMock.AssertCalled(t, "DeployStack", mock.Anything, mock.Anything, mock.MatchedBy(func(actual any) bool {
					expected := api.Stack{
						Name:    "refapp",
						Secrets: *tests.CommonSecretsDescriptor,
						Server:  *tests.ResolvedRefappServerDescriptor,
						Client:  *tests.ResolvedRefappClientDescriptor("testdata"),
					}
					return assert.EqualValuesf(t, expected, actual, "%v failed", ttName)
				}), mock.MatchedBy(func(actual any) bool {
					return assert.EqualValuesf(t, api.DeployParams{
						StackParams: api.StackParams{
							StacksDir:   "stacks",
							StackName:   "refapp",
							Environment: "staging",
						},
					}, actual, "%v failed", ttName)
				}))
			},
		},
		{
			name: "error stack not found",
			params: api.DeployParams{
				StackParams: api.StackParams{
					StacksDir:   "stacks",
					StackName:   "refapp-notexisting",
					Environment: "staging",
				},
			},
			wantErr: `stack "refapp-notexisting" is not configured`,
			verify: func(t *testing.T, ttName string, pulumiMock *pulumi_mocks.PulumiMock, err error) {
				pulumiMock.AssertNotCalled(t, "DeployStack", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				pulumiMock.AssertNotCalled(t, "SetPublicKey", mock.Anything)
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			var p Provisioner
			var err error

			gitRepoMock := git_mocks.NewGitRepoMock(t)
			gitRepoMock.On("Workdir").Return("testdata")
			pulumiMock := pulumi_mocks.NewPulumiMock(t)
			if tt.setExpectations {
				pulumiMock.On("DeployStack", ctx, mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
				pulumiMock.On("SetPublicKey", mock.Anything).Return()
			}
			p, err = New(
				WithPlaceholders(placeholders.New(placeholders.WithGitRepo(gitRepoMock))),
				WithOverrideProvisioner(pulumiMock),
				WithGitRepo(gitRepoMock),
			)

			if err != nil && tt.wantErr != "" {
				Expect(err).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
			}

			err = p.Deploy(ctx, tt.params)

			if tt.verify != nil {
				tt.verify(t, tt.name, pulumiMock, err)
			}
			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).To(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func Test_Init(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name        string
		params      api.InitParams
		opts        []Option
		init        func(wd string) Provisioner
		check       func(t *testing.T, wd string, p Provisioner)
		wantInitErr string
		wantAnyErr  string
	}{
		{
			name: "happy path",
			params: api.InitParams{
				ProjectName: "test-project",
				RootDir:     "testdata/refapp",
			},
			opts:  []Option{WithPlaceholders(placeholders.New())},
			check: checkInitSuccess,
		},
		{
			name: "existing repo no error",
			params: api.InitParams{
				ProjectName: "test-project",
				RootDir:     "testdata/refapp-existing-gitdir",
			},
			init: func(wd string) Provisioner {
				gitRepo, err := git.New(git.WithGitDir("gitdir"), git.WithRootDir(wd))
				Expect(err).To(BeNil())
				p, err := New(WithGitRepo(gitRepo), WithPlaceholders(placeholders.New()))
				Expect(err).To(BeNil())
				return p
			},
			check: checkInitSuccess,
		},
		{
			name: "project name is not set",
			params: api.InitParams{
				RootDir: "testdata/refapp",
			},
			wantInitErr: "project name is not configured",
		},
		{
			name: "skip profile creation",
			params: api.InitParams{
				ProjectName:         "test-project",
				RootDir:             "testdata/refapp",
				SkipProfileCreation: true,
			},
			opts: []Option{WithPlaceholders(placeholders.New())},
			check: func(t *testing.T, wd string, p Provisioner) {
				profileFile := path.Join(wd, ".sc/cfg.default.yaml")
				Expect(profileFile).NotTo(BeAnExistingFile())
			},
		},
		{
			name: "skip initial commit",
			params: api.InitParams{
				ProjectName:       "test-project",
				RootDir:           "testdata/refapp",
				SkipInitialCommit: true,
			},
			opts: []Option{WithPlaceholders(placeholders.New())},
			check: func(t *testing.T, wd string, p Provisioner) {
				checkProfileIsCreated(t, wd, p)
				commits := p.GitRepo().Log()
				_, exists := lo.Find(commits, func(c git.Commit) bool {
					return c.Message == "simple-container.com initial commit"
				})
				Expect(exists).To(BeFalse())
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			workDir, cleanup, err := testutil.CopyTempProject(tt.params.RootDir)
			defer cleanup()

			testutil.CheckError(err, tt.wantAnyErr)

			var p Provisioner
			if tt.init != nil {
				p = tt.init(workDir)
			} else {
				p, err = New(tt.opts...)
			}

			testutil.CheckError(err, tt.wantAnyErr)

			// overwrite root dir to temp
			tt.params.RootDir = workDir

			err = p.Init(ctx, tt.params)
			testutil.CheckError(err, tt.wantInitErr)

			if tt.check != nil {
				tt.check(t, workDir, p)
			}
		})
	}
}

func checkInitSuccess(t *testing.T, wd string, p Provisioner) {
	t.Run("profile template is created", func(t *testing.T) {
		templateFile := path.Join(wd, ".sc/cfg.yaml.template")
		Expect(templateFile).To(BeAnExistingFile())
	})
	checkProfileIsCreated(t, wd, p)
	checkInitialCommit(t, wd, p)
}

func checkInitialCommit(t *testing.T, wd string, p Provisioner) {
	if os.Getenv("GITHUB_REPOSITORY") != "" {
		t.Skip("Skipping checking for the initial commit on github")
		return
	}
	t.Run("initial commit is present", func(t *testing.T) {
		commits := p.GitRepo().Log()
		commit, exists := lo.Find(commits, func(c git.Commit) bool {
			return c.Message == "simple-container.com initial commit"
		})
		Expect(exists).To(BeTrue())
		Expect(commit.Message).To(Equal("simple-container.com initial commit"))
	})
}

func checkProfileIsCreated(t *testing.T, wd string, p Provisioner) {
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
