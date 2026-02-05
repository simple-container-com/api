package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommandExecutor executes external commands
type CommandExecutor struct {
	timeout time.Duration
}

// NewCommandExecutor creates a new command executor
func NewCommandExecutor(timeout time.Duration) *CommandExecutor {
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout
	}
	return &CommandExecutor{
		timeout: timeout,
	}
}

// Execute runs a command and returns output
func (e *CommandExecutor) Execute(ctx context.Context, args []string, env map[string]string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(timeoutCtx, args[0], args[1:]...)

	// Set environment variables
	if env != nil {
		cmd.Env = append(cmd.Env, e.envMapToSlice(env)...)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}

	return stdout.Bytes(), nil
}

// ExecuteWithStderr runs a command and returns both stdout and stderr
func (e *CommandExecutor) ExecuteWithStderr(ctx context.Context, args []string, env map[string]string) (stdout []byte, stderr []byte, err error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("no command specified")
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(timeoutCtx, args[0], args[1:]...)

	// Set environment variables
	if env != nil {
		cmd.Env = append(cmd.Env, e.envMapToSlice(env)...)
	}

	// Capture output
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run command
	err = cmd.Run()

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// CheckCommandExists checks if a command is available in PATH
func (e *CommandExecutor) CheckCommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// envMapToSlice converts environment map to slice format
func (e *CommandExecutor) envMapToSlice(env map[string]string) []string {
	var envSlice []string
	for key, value := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, value))
	}
	return envSlice
}

// BuildCommand is a helper to build command arguments
func BuildCommand(command string, args ...string) []string {
	cmd := []string{command}
	cmd = append(cmd, args...)
	return cmd
}

// QuoteArg quotes an argument if it contains spaces
func QuoteArg(arg string) string {
	if strings.Contains(arg, " ") {
		return fmt.Sprintf("\"%s\"", arg)
	}
	return arg
}
