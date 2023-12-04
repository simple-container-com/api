package provisioner

import (
	"api/pkg/provisioner/git"
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"api/pkg/api/tests"
	testutils "api/pkg/provisioner/tests"
)

func Test_Provision(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name         string
		params       ProvisionParams
		opts         []Option
		expectStacks StacksMap
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
			expectStacks: map[string]Stack{
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
		check   func(wd string, p Provisioner)
		wantErr string
	}{
		{
			name: "happy path",
			params: InitParams{
				RootDir: "testdata/refapp",
			},
			check: checkInitialCommit,
		},
		{
			name: "existing repo no error",
			params: InitParams{
				RootDir: "testdata/refapp",
			},
			init: func(wd string) Provisioner {
				gitRepo, err := git.New(git.WithGitDir("gitdir"), git.WithRootDir(wd))
				Expect(err).To(BeNil())
				p, err := New(WithGitRepo(gitRepo))
				Expect(err).To(BeNil())
				return p
			},
			check: checkInitialCommit,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			workDir, err := testutils.CopyTempProject(tt.params.RootDir)
			defer func() { _ = os.RemoveAll(workDir) }()

			checkError(err, tt.wantErr)

			var p Provisioner
			if tt.init != nil {
				p = tt.init(workDir)
			} else {
				p, err = New(tt.opts...)
			}

			checkError(err, tt.wantErr)

			// overwrite root dir to temp
			tt.params.RootDir = workDir

			Expect(err).To(BeNil())
			err = p.Init(ctx, tt.params)
			checkError(err, tt.wantErr)

			if tt.check != nil {
				tt.check(workDir, p)
			}
		})
	}
}

func checkError(err error, checkErr string) {
	if checkErr != "" {
		Expect(err).To(MatchRegexp(checkErr))
	}
}

func checkInitialCommit(wd string, p Provisioner) {
	commits := p.GitRepo().Log()
	Expect(commits).To(HaveLen(1))
	Expect(commits[0].Message).To(Equal("simple-container.com initial commit"))
}
