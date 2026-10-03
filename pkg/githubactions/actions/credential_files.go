// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"strings"
)

// credentialFileVars name files that an earlier workflow step writes and a cloud
// SDK reads; these are what google-github-actions/auth exports after exchanging
// the job's OIDC token through Workload Identity Federation.
var credentialFileVars = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
	"GOOGLE_GHA_CREDS_PATH",
}

// remapWorkspaceCredentialFiles points credential-file variables that name a file
// in the job's workspace at a private copy inside this container. Those tools
// write the file at the root of the workspace, and the variable reaches the
// container either already translated to the mounted workspace or still naming
// the runner path, which does not exist in here; in the second case the file of
// the same name at the root of GITHUB_WORKSPACE is used.
//
// The file is copied out of the workspace rather than referenced in it: when the
// job has no checkout, the action clones the repository and replaces the
// workspace's contents, which would delete the credentials of the identity the
// rest of the run is meant to use. A variable naming a file outside the
// workspace is left alone.
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
		src := p
		if _, err := os.Stat(p); err == nil {
			if !insideDir(p, workspace) {
				continue
			}
		} else {
			src = filepath.Join(workspace, filepath.Base(p))
		}
		if st, err := os.Stat(src); err != nil || !st.Mode().IsRegular() {
			continue
		}
		dst, err := preserveCredentialFile(src)
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

func insideDir(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}
