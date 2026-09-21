// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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
			expectResult: "eyJhdXRocyI6eyJkb2NrZXIuc2ltcGxlLWNvbnRhaW5lci5jb20iOnsiYXV0aCI6ImRYTmxjanB3WVhOemQyOXlaQT09IiwidXNlcm5hbWUiOiJ1c2VyIiwicGFzc3dvcmQiOiJwYXNzd29yZCJ9fX0=", // trufflehog:ignore (nested base64 of fake user:password test fixture)
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

// RegistryRequiresAuth gates whether an imagePullSecret is generated at all, so
// a partially-configured registry (username without password) must read as
// "no auth" rather than produce a half-formed secret.
func TestRegistryCredentials_RegistryRequiresAuth(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name  string
		creds RegistryCredentials
		want  bool
	}{
		{name: "both set", want: true, creds: RegistryCredentials{
			DockerRegistryUsername: lo.ToPtr("user"),
			DockerRegistryPassword: lo.ToPtr("password"),
		}},
		{name: "username only", want: false, creds: RegistryCredentials{
			DockerRegistryUsername: lo.ToPtr("user"),
		}},
		{name: "password only", want: false, creds: RegistryCredentials{
			DockerRegistryPassword: lo.ToPtr("password"),
		}},
		{name: "neither", want: false, creds: RegistryCredentials{}},
		// An empty-but-present pointer is a declared-yet-blank secret. It still
		// counts as "auth requested" so the deploy fails on a bad credential
		// rather than silently pulling anonymously.
		{name: "both present but empty", want: true, creds: RegistryCredentials{
			DockerRegistryUsername: lo.ToPtr(""),
			DockerRegistryPassword: lo.ToPtr(""),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(tc.creds.RegistryRequiresAuth()).To(Equal(tc.want))
		})
	}
}
