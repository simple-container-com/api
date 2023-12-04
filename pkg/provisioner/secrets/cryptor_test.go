package secrets

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewCryptor(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		workDir string
		opts    []Option
		wantErr string
	}{
		{
			name:    "happy path",
			workDir: "testdata/repo",
			opts:    []Option{WithGitDir("gitdir")},
		},
		{
			name:    "bad workdir",
			workDir: "testdata/non-existent-repo",
			wantErr: "failed to open git repository.*",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCryptor(tt.workDir, tt.opts...)
			if err != nil && tt.wantErr != "" {
				Expect(err.Error()).Should(MatchRegexp(tt.wantErr))
			} else {
				Expect(err).To(BeNil())
				Expect(got).NotTo(BeNil())
			}
		})
	}
}
