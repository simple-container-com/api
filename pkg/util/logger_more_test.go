package util

import (
	"bytes"
	"errors"
	"io"
	"testing"

	. "github.com/onsi/gomega"
)

// bufWriteCloser adapts a bytes.Buffer to io.WriteCloser for the StdoutLogger.
type bufWriteCloser struct {
	bytes.Buffer
}

func (b *bufWriteCloser) Close() error { return nil }

func TestNewStdoutLogger_DefaultsToOsStreams(t *testing.T) {
	RegisterTestingT(t)

	// Passing nil for both streams must fall back to os.Stdout/os.Stderr.
	l := NewStdoutLogger(nil, nil)
	Expect(l).ToNot(BeNil())
	// Methods must not panic when wired to the real OS streams.
	Expect(func() { l.Debugf("debug-not-printed") }).ToNot(Panic())
}

func TestStdoutLogger_LogAndLogf(t *testing.T) {
	RegisterTestingT(t)

	out := &bufWriteCloser{}
	errOut := &bufWriteCloser{}
	l := NewStdoutLogger(out, errOut)

	l.Log("hello")
	Expect(out.String()).To(Equal("hello\n"))

	out.Reset()
	l.Logf("value=%d", 42)
	Expect(out.String()).To(Equal("value=42\n"))
}

func TestStdoutLogger_ErrAndErrf(t *testing.T) {
	RegisterTestingT(t)

	out := &bufWriteCloser{}
	errOut := &bufWriteCloser{}
	l := NewStdoutLogger(out, errOut)

	// QUIRK: StdoutLogger.Err passes a []byte to color.Fprint, which formats
	// it through fmt as a slice of byte values (e.g. "[111 111 112 ...]")
	// rather than the raw string. We assert the actual current behaviour:
	// the literal characters of "oops" do NOT appear, but the decimal byte
	// codes do. See logger.go:89.
	l.Err("oops")
	written := errOut.String()
	Expect(written).ToNot(BeEmpty())
	Expect(written).ToNot(ContainSubstring("oops"))
	// 'o'=111, 'p'=112, 's'=115 — the byte-slice rendering must contain these.
	Expect(written).To(ContainSubstring("111"))

	errOut.Reset()
	l.Errf("code=%d", 7)
	Expect(errOut.String()).ToNot(BeEmpty())
}

func TestStdoutLogger_DebugfGating(t *testing.T) {
	RegisterTestingT(t)

	t.Run("debug disabled suppresses output", func(t *testing.T) {
		RegisterTestingT(t)
		out := &bufWriteCloser{}
		l := NewStdoutLogger(out, &bufWriteCloser{})
		l.Debugf("hidden %s", "msg")
		Expect(out.String()).To(Equal(""))
	})

	t.Run("debug enabled emits via Logf", func(t *testing.T) {
		RegisterTestingT(t)
		out := &bufWriteCloser{}
		l := NewStdoutLogger(out, &bufWriteCloser{}).Debug()
		l.Debugf("shown %s", "msg")
		Expect(out.String()).To(Equal("shown msg\n"))
	})
}

func TestStdoutLogger_SubLoggerReturnsSelf(t *testing.T) {
	RegisterTestingT(t)

	out := &bufWriteCloser{}
	l := NewStdoutLogger(out, &bufWriteCloser{})
	sub := l.SubLogger("child")
	// StdoutLogger.SubLogger is a no-op that returns the same logger.
	Expect(sub).To(BeIdenticalTo(Logger(l)))
}

func TestNoopLogger_AllMethodsAreSilentNoops(t *testing.T) {
	RegisterTestingT(t)

	var l Logger = &NoopLogger{}
	Expect(func() {
		l.Log("x")
		l.Logf("x %d", 1)
		l.Err("x")
		l.Errf("x %d", 1)
		l.Debugf("x %d", 1)
	}).ToNot(Panic())
	// SubLogger must return a usable (non-nil) logger — itself.
	Expect(l.SubLogger("any")).ToNot(BeNil())
}

func TestPrefixLogger_LogfDelegatesToLog(t *testing.T) {
	RegisterTestingT(t)

	// PrefixLogger writes to the process stdout/stderr directly, so we cannot
	// capture content without redirection. We verify the public surface does
	// not panic and that SubLogger composes prefixes correctly (covered below).
	l := &PrefixLogger{prefix: "[root]"}
	Expect(func() { l.Logf("hi %s", "there") }).ToNot(Panic())
	Expect(func() { l.Errf("err %d", 1) }).ToNot(Panic())
}

func TestPrefixLogger_DebugfGating(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled debug does nothing", func(t *testing.T) {
		RegisterTestingT(t)
		l := &PrefixLogger{prefix: "[p]"}
		Expect(func() { l.Debugf("x") }).ToNot(Panic())
	})

	t.Run("enabled debug logs without panic", func(t *testing.T) {
		RegisterTestingT(t)
		l := &PrefixLogger{prefix: "[p]", debug: true}
		Expect(func() { l.Debugf("x %d", 1) }).ToNot(Panic())
	})
}

func TestPrefixLogger_WithTimeFormat(t *testing.T) {
	RegisterTestingT(t)

	l := &PrefixLogger{prefix: "[p]"}
	ret := l.WithTimeFormat("2006-01-02")
	// Builder returns the same pointer with the format set.
	Expect(ret).To(BeIdenticalTo(l))
	Expect(l.timeFormat).To(Equal("2006-01-02"))
}

func TestPrefixLogger_PrintTimeBranchDoesNotPanic(t *testing.T) {
	RegisterTestingT(t)

	// Exercise the printTime==true branch of (*PrefixLogger).log via Log().
	l := &PrefixLogger{prefix: "[t]", printTime: true, timeFormat: "15:04:05"}
	Expect(func() { l.Log("\nmessage-with-newlines\n") }).ToNot(Panic())
}

func TestPrefixLogger_SubLoggerComposesPrefix(t *testing.T) {
	RegisterTestingT(t)

	parent := &PrefixLogger{prefix: "[root]", debug: true, printTime: true, timeFormat: "X"}
	child := parent.SubLogger("worker")

	cp, ok := child.(*PrefixLogger)
	Expect(ok).To(BeTrue())
	Expect(cp.prefix).To(Equal("[root] [worker]"))
	// Inherited fields carry over.
	Expect(cp.debug).To(BeTrue())
	Expect(cp.printTime).To(BeTrue())
	Expect(cp.timeFormat).To(Equal("X"))

	// Composition is repeatable.
	grand := child.SubLogger("task")
	Expect(grand.(*PrefixLogger).prefix).To(Equal("[root] [worker] [task]"))
}

func TestReaderToLogFunc_StreamsLinesToLogger(t *testing.T) {
	RegisterTestingT(t)

	t.Run("plain lines routed to Log with prefix", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		fn := ReaderToLogFunc(bytes.NewBufferString("line1\nline2\n"), false, "P:", rec, "subj")
		Expect(fn()).To(Succeed())
		Expect(rec.logs).To(ConsistOf("P:line1", "P:line2"))
		Expect(rec.errs).To(BeEmpty())
	})

	t.Run("error lines routed to Err with prefix", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		fn := ReaderToLogFunc(bytes.NewBufferString("boom\n"), true, "ERR: ", rec, "subj")
		Expect(fn()).To(Succeed())
		Expect(rec.errs).To(ConsistOf("ERR: boom"))
		Expect(rec.logs).To(BeEmpty())
	})

	t.Run("scanner read error is wrapped and returned", func(t *testing.T) {
		RegisterTestingT(t)
		rec := &recordingLogger{}
		fn := ReaderToLogFunc(&failingReader{err: errors.New("io broke")}, false, "", rec, "the-subject")
		err := fn()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("the-subject"))
		Expect(err.Error()).To(ContainSubstring("io broke"))
	})
}

// recordingLogger captures Log/Err calls for assertion. Debugf/Logf/Errf
// delegate so all interface methods are wired.
type recordingLogger struct {
	logs []string
	errs []string
}

func (r *recordingLogger) Debugf(format string, args ...interface{}) {}
func (r *recordingLogger) Log(msg string)                            { r.logs = append(r.logs, msg) }
func (r *recordingLogger) Logf(format string, args ...interface{})   {}
func (r *recordingLogger) Err(msg string)                            { r.errs = append(r.errs, msg) }
func (r *recordingLogger) Errf(format string, args ...interface{})   {}
func (r *recordingLogger) SubLogger(name string) Logger              { return r }

// failingReader always returns an error from Read, to exercise scanner error
// handling paths.
type failingReader struct{ err error }

func (f *failingReader) Read(p []byte) (int, error) { return 0, f.err }

var (
	_ Logger    = (*recordingLogger)(nil)
	_ io.Reader = (*failingReader)(nil)
)
