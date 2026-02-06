package tools

import (
	"context"
	"os/exec"
	"time"
)

// ExecCommand executes a command with the given arguments and environment variables.
// It returns stdout, stderr, and any error that occurred.
func ExecCommand(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (stdout, stderr string, err error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, name, args...)
	if env != nil {
		cmd.Env = append(cmd.Environ(), env...)
	}

	stdoutBytes, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(stdoutBytes), string(exitErr.Stderr), err
		}
		return string(stdoutBytes), "", err
	}

	return string(stdoutBytes), "", nil
}
