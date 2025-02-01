package docker

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

func Test_GenerateImagePullSecret(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name         string
		creds        RegistryCredentials
		expectResult string
		expectError  string
	}{
		{
			name:         "happy-path",
			expectResult: "eyJhdXRocyI6eyJkb2NrZXIuc2ltcGxlLWNvbnRhaW5lci5jb20iOnsiYXV0aCI6ImRYTmxjanB3WVhOemQyOXlaQT09IiwidXNlcm5hbWUiOiJ1c2VyIiwicGFzc3dvcmQiOiJwYXNzd29yZCJ9fX0=",
			creds: RegistryCredentials{
				DockerRegistryURL:      lo.ToPtr("docker.simple-container.com"),
				DockerRegistryUsername: lo.ToPtr("user"),
				DockerRegistryPassword: lo.ToPtr("password"),
			},
		},
		{
			name:        "error on empty",
			creds:       RegistryCredentials{},
			expectError: "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.creds.ToImagePullSecret()
			if tt.expectError != "" {
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring(tt.expectError))
			}
			Expect(res).To(Equal(tt.expectResult))
		})
	}
}
