// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"strings"
)

// scScript renders the shell a local.Command runs to invoke one `sc cloudru`
// subcommand.
//
// Nothing is interpolated into it: the whole request — credentials, the stack's
// secrets, the spec — arrives on the command's Stdin, which is why there is no
// mktemp/trap dance here as there is in the vm driver's shipScript (that one has to
// materialise an SSH key as a *file*, because `--ssh-key` takes a path). An
// argument would be readable in `ps` by every other process on the runner, and an
// environment variable would be readable by every other step of the job.
//
// The PATH prefix is the same one pkg/clouds/pulumi/docker/security_helpers.go
// uses: the SC runner image installs its own tooling outside the default PATH, so
// a bare `sc` is not findable without it.
func scScript(args ...string) string {
	return strings.Join([]string{
		"set -eu",
		`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`,
		"sc " + strings.Join(args, " "),
	}, "\n") + "\n"
}
