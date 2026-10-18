// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package actions

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// credentialFileVars name files that an earlier workflow step writes and a cloud
// SDK reads; these are what google-github-actions/auth exports after exchanging
// the job's OIDC token through Workload Identity Federation.
var credentialFileVars = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
	"GOOGLE_GHA_CREDS_PATH",
}

// PreserveWorkspaceCredentialFiles points credential-file variables that name a
// file in the job's workspace at a private copy outside it. Those tools write the
// file at the root of the workspace, and the variable reaches the container either
// already translated to the mounted workspace or still naming the runner path,
// which does not exist in here; in the second case the file of the same name at
// the root of GITHUB_WORKSPACE is used.
//
// The file is copied out of the workspace rather than referenced in it: when the
// job has no checkout, the action clones the repository and replaces the
// workspace's contents, which would delete the credentials of the identity the
// rest of the run is meant to use. A variable naming a file outside the workspace
// is left alone, so a second call is a no-op.
//
// It returns the variables it changed and, per variable, why a file it should
// have preserved was not; the caller reports those, since the clone that follows
// would otherwise remove the identity without a word.
func PreserveWorkspaceCredentialFiles() (map[string]string, map[string]error) {
	workspace := os.Getenv("GITHUB_WORKSPACE")
	changed := map[string]string{}
	failed := map[string]error{}
	if workspace == "" {
		return changed, failed
	}
	var dir string
	var err error
	for _, name := range credentialFileVars {
		p := os.Getenv(name)
		if p == "" {
			continue
		}
		src := p
		if abs, err := filepath.Abs(p); err == nil {
			src = abs
		}
		if _, err := os.Stat(src); err == nil {
			if !insideDir(src, workspace) {
				continue
			}
		} else {
			src = filepath.Join(workspace, filepath.Base(p))
		}
		if st, err := os.Stat(src); err != nil || !st.Mode().IsRegular() {
			continue
		}
		if dir == "" {
			if dir, err = os.MkdirTemp("", "sc-credentials-"); err != nil {
				failed[name] = errors.Wrap(err, "failed to create a private directory for credentials")
				dir = ""
				continue
			}
		}
		dst, err := preserveCredentialFile(dir, name, src)
		if err != nil {
			failed[name] = err
			continue
		}
		if err := os.Setenv(name, dst); err != nil {
			failed[name] = err
			continue
		}
		changed[name] = dst
	}
	return changed, failed
}

// preserveCredentialFile copies src into dir, a directory this call created
// (0700, unpredictable name), as a file only this process can have created
// (os.CreateTemp: O_EXCL, 0600, random suffix). The copy is named after the
// variable, so two variables naming different files never share a copy.
func preserveCredentialFile(dir, name, src string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", src)
	}
	f, err := os.CreateTemp(dir, name+"-*-"+filepath.Base(src))
	if err != nil {
		return "", errors.Wrapf(err, "failed to create a copy of %s", src)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", errors.Wrapf(err, "failed to write %s", f.Name())
	}
	if err := f.Close(); err != nil {
		return "", errors.Wrapf(err, "failed to close %s", f.Name())
	}
	return f.Name(), nil
}

// insideDir reports whether path lies below dir. Both are compared as given and,
// when they resolve, with symlinks evaluated, so a symlinked alias of the
// workspace still counts as the workspace.
func insideDir(path, dir string) bool {
	if below(path, dir) {
		return true
	}
	rp, err1 := filepath.EvalSymlinks(path)
	rd, err2 := filepath.EvalSymlinks(dir)
	return err1 == nil && err2 == nil && below(rp, rd)
}

func below(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
