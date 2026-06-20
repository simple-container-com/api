package util

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// skipOnNonUnix bails out of exec tests on platforms without a POSIX `sh`
// (the implementation hard-codes `sh -c` and a `trap ... EXIT` invocation).
func skipOnNonUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec helpers require a POSIX shell")
	}
}

func TestNewExec_StoresLoggerAndContext(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()
	logger := &NoopLogger{}
	e := NewExec(ctx, logger)
	Expect(e.context).To(Equal(ctx))
	Expect(e.logger).To(BeIdenticalTo(Logger(logger)))
	Expect(e.output).To(BeNil())
}

func TestNewExecWithOutput_AttachesBuffer(t *testing.T) {
	RegisterTestingT(t)

	buf := &bytes.Buffer{}
	e := NewExecWithOutput(context.Background(), &NoopLogger{}, buf)
	Expect(e.output).To(BeIdenticalTo(buf))
}

func TestLookup(t *testing.T) {
	skipOnNonUnix(t)

	t.Run("finds an existing binary", func(t *testing.T) {
		RegisterTestingT(t)
		path, err := Lookup("sh")
		Expect(err).ToNot(HaveOccurred())
		Expect(path).To(ContainSubstring("sh"))
	})

	t.Run("errors for a non-existent binary", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := Lookup("this-binary-does-not-exist-xyz-123")
		Expect(err).To(HaveOccurred())
	})
}

func TestExec_ExecCommand(t *testing.T) {
	skipOnNonUnix(t)

	t.Run("captures combined stdout output", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		out, err := e.ExecCommand("echo hello-world", ExecOpts{})
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("hello-world"))
	})

	t.Run("captures combined stderr output", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		out, err := e.ExecCommand("echo to-stderr 1>&2", ExecOpts{})
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("to-stderr"))
	})

	t.Run("non-zero exit returns an error", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		_, err := e.ExecCommand("exit 3", ExecOpts{})
		Expect(err).To(HaveOccurred())
	})

	t.Run("honours the working directory option", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		e := NewExec(context.Background(), &NoopLogger{})
		out, err := e.ExecCommand("pwd", ExecOpts{Wd: dir})
		Expect(err).ToNot(HaveOccurred())
		// macOS may symlink /tmp -> /private/tmp; assert the tail to stay robust.
		Expect(strings.TrimSpace(out)).To(HaveSuffix(strings.TrimPrefix(dir, "/private")))
	})

	t.Run("injects extra environment variables", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		out, err := e.ExecCommand("echo val=$MY_TEST_VAR", ExecOpts{Env: []string{"MY_TEST_VAR=present"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("val=present"))
	})
}

func TestExec_ExecCommandAndLog(t *testing.T) {
	skipOnNonUnix(t)

	t.Run("streams stdout to logger and captures buffer + result env", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		buf := &bytes.Buffer{}
		e := NewExecWithOutput(context.Background(), rec, buf)

		res, err := e.ExecCommandAndLog("my-subject", "echo captured-line", ExecOpts{})
		Expect(err).ToNot(HaveOccurred())
		// PID of the finished process is recorded.
		Expect(res.Pid).To(BeNumerically(">", 0))
		// The captured buffer holds the stdout content.
		Expect(buf.String()).To(ContainSubstring("captured-line"))
		// The logger received the streamed line.
		Expect(strings.Join(rec.logs, "\n")).To(ContainSubstring("captured-line"))
		// The trap-written env file is parsed into res.Env (one entry per line).
		Expect(res.Env).ToNot(BeEmpty())
	})

	t.Run("routes stderr through the error logger", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		e := NewExec(context.Background(), rec)
		_, err := e.ExecCommandAndLog("subj", "echo problem 1>&2", ExecOpts{})
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.Join(rec.errs, "\n")).To(ContainSubstring("problem"))
	})

	t.Run("env vars set in the command appear in result env", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		e := NewExec(context.Background(), rec)
		res, err := e.ExecCommandAndLog("subj", "export RESULT_ENV_PROBE=yes", ExecOpts{})
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.Join(res.Env, "\n")).To(ContainSubstring("RESULT_ENV_PROBE=yes"))
	})

	t.Run("failing command returns the run error", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		e := NewExec(context.Background(), rec)
		_, err := e.ExecCommandAndLog("subj", "exit 5", ExecOpts{})
		Expect(err).To(HaveOccurred())
	})
}

func TestExec_prepareCommand(t *testing.T) {
	skipOnNonUnix(t)

	t.Run("sets a unique env file, sh -c args, dir and env", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		e := NewExec(context.Background(), &NoopLogger{})
		cmd := e.prepareCommand("echo hi", ExecOpts{Wd: dir, Env: []string{"FOO=bar"}})

		// resEnvFile is populated as a side effect and embedded in the trap.
		Expect(e.resEnvFile).To(HavePrefix("/tmp/"))
		Expect(e.resEnvFile).To(HaveSuffix(".env"))

		Expect(cmd.Args[0]).To(Equal("sh"))
		Expect(cmd.Args[1]).To(Equal("-c"))
		Expect(cmd.Args[2]).To(ContainSubstring("trap"))
		Expect(cmd.Args[2]).To(ContainSubstring(e.resEnvFile))
		Expect(cmd.Args[2]).To(ContainSubstring("echo hi"))

		Expect(cmd.Dir).To(Equal(dir))
		// When Env opts are provided, the process env = os.Environ() + opts.
		Expect(cmd.Env).To(ContainElement("FOO=bar"))
		Expect(len(cmd.Env)).To(BeNumerically(">", 1))
	})

	t.Run("without env opts leaves cmd.Env nil (inherits parent)", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		cmd := e.prepareCommand("true", ExecOpts{})
		Expect(cmd.Env).To(BeNil())
		Expect(cmd.Dir).To(Equal(""))
	})

	t.Run("each call generates a distinct env file path", func(t *testing.T) {
		RegisterTestingT(t)
		e := NewExec(context.Background(), &NoopLogger{})
		e.prepareCommand("true", ExecOpts{})
		first := e.resEnvFile
		e.prepareCommand("true", ExecOpts{})
		second := e.resEnvFile
		Expect(first).ToNot(Equal(second))
	})
}

// guard: confirm os is referenced (used indirectly via t.TempDir + env probes).
var _ = os.Environ
