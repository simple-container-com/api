// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
)

// credentialFileVars name files that an earlier workflow step writes and a cloud
// SDK reads; these are what google-github-actions/auth exports after exchanging
// the job's OIDC token through Workload Identity Federation.
var credentialFileVars = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
	"GOOGLE_GHA_CREDS_PATH",
}

// remapWorkspaceCredentialFiles points credential-file variables at a private
// copy inside this container. GitHub runs a Docker action with the job's
// workspace mounted at GITHUB_WORKSPACE, but a file an earlier step wrote there
// is named by its path on the runner, which does not exist in here. A variable is
// changed only when its file is missing and a regular file of the same name sits
// at the root of the mounted workspace, which is where those tools write it.
//
// The file is copied out of the workspace rather than referenced in it: when the
// job has no checkout, the action clones the repository and replaces the
// workspace's contents, which would delete the credentials of the identity the
// rest of the run is meant to use.
func remapWorkspaceCredentialFiles() map[string]string {
	workspace := os.Getenv("GITHUB_WORKSPACE")
	changed := map[string]string{}
	if workspace == "" {
		return changed
	}
	for _, name := range credentialFileVars {
		p := os.Getenv(name)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			continue
		}
		candidate := filepath.Join(workspace, filepath.Base(p))
		if st, err := os.Stat(candidate); err != nil || !st.Mode().IsRegular() {
			continue
		}
		dst, err := preserveCredentialFile(candidate)
		if err != nil {
			continue
		}
		if os.Setenv(name, dst) == nil {
			changed[name] = dst
		}
	}
	return changed
}

// PreserveWorkspaceCredentialFiles does what remapWorkspaceCredentialFiles does,
// for callers outside this package that are about to replace the workspace.
func PreserveWorkspaceCredentialFiles() map[string]string {
	return remapWorkspaceCredentialFiles()
}

func preserveCredentialFile(src string) (string, error) {
	dir := filepath.Join(os.TempDir(), "sc-credentials")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}
