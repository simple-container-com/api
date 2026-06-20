// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package util

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

// fakeReader is a programmable ConsoleReader for driving the Console without
// touching real stdin.
type fakeReader struct {
	line    string
	lineErr error
	pass    string
	passErr error
}

func (f *fakeReader) ReadLine() (string, error)     { return f.line, f.lineErr }
func (f *fakeReader) ReadPassword() (string, error) { return f.pass, f.passErr }

// captureWriter records everything written through the ConsoleWriter so tests
// can assert the rendered prompt text.
type captureWriter struct {
	buf bytes.Buffer
}

func (w *captureWriter) Print(args ...interface{}) {
	// Plain concatenation is sufficient and deterministic for assertions.
	for _, a := range args {
		_, _ = w.buf.WriteString(toStr(a))
	}
}

func (w *captureWriter) Println(args ...interface{}) {
	for _, a := range args {
		_, _ = w.buf.WriteString(toStr(a))
	}
	_, _ = w.buf.WriteString("\n")
}

func toStr(a interface{}) string {
	if s, ok := a.(string); ok {
		return s
	}
	return ""
}

func newTestConsole(r ConsoleReader, w ConsoleWriter) *ConsoleImpl {
	c := NewDefaultConsole()
	c.SetReader(r)
	c.SetWriter(w)
	return c
}

func TestNewDefaultConsole_UsesPackageDefaults(t *testing.T) {
	RegisterTestingT(t)

	c := NewDefaultConsole()
	Expect(c.Reader()).To(Equal(DefaultConsoleReader))
	Expect(c.Writer()).To(Equal(DefaultConsoleWriter))
}

func TestConsoleImpl_SettersAndGetters(t *testing.T) {
	RegisterTestingT(t)

	r := &fakeReader{}
	w := &captureWriter{}
	c := NewDefaultConsole()

	c.SetReader(r)
	c.SetWriter(w)
	Expect(c.Reader()).To(BeIdenticalTo(r))
	Expect(c.Writer()).To(BeIdenticalTo(w))
}

func TestConsoleImpl_AskQuestionWithDefault(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		inputErr     error
		defaultResp  string
		alwaysDef    bool
		wantResp     string
		wantErr      bool
		wantInPrompt string
	}{
		{
			name:         "non-empty answer is trimmed and returned",
			input:        "  hello  ",
			defaultResp:  "fallback",
			wantResp:     "hello",
			wantInPrompt: "[fallback]",
		},
		{
			name:        "empty answer falls back to default",
			input:       "   ",
			defaultResp: "the-default",
			wantResp:    "the-default",
		},
		{
			name:        "alwaysDefault short-circuits reader",
			input:       "ignored",
			defaultResp: "auto",
			alwaysDef:   true,
			wantResp:    "auto",
		},
		{
			name:        "reader error is propagated",
			input:       "x",
			inputErr:    errors.New("read failed"),
			defaultResp: "d",
			wantResp:    "x",
			wantErr:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			r := &fakeReader{line: tc.input, lineErr: tc.inputErr}
			w := &captureWriter{}
			c := newTestConsole(r, w)
			if tc.alwaysDef {
				c.AlwaysRespondDefault()
			}

			resp, err := c.AskQuestionWithDefault("Proceed?", tc.defaultResp)
			Expect(resp).To(Equal(tc.wantResp))
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(w.buf.String()).To(ContainSubstring("Proceed?"))
			Expect(w.buf.String()).To(ContainSubstring(tc.defaultResp))
		})
	}
}

func TestConsoleImpl_AskYesNoQuestionWithDefault(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
		wantPrompt string
	}{
		{"explicit y answer", "y", false, true, "[N]"},
		{"explicit uppercase Y answer", "Y", false, true, "[N]"},
		{"explicit n answer", "n", true, false, "[Y]"},
		{"empty uses yes default", "", true, true, "[Y]"},
		{"empty uses no default", "", false, false, "[N]"},
		{"non-y answer is false", "maybe", true, false, "[Y]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			r := &fakeReader{line: tc.input}
			w := &captureWriter{}
			c := newTestConsole(r, w)

			got, err := c.AskYesNoQuestionWithDefault("Continue?", tc.defaultYes)
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
			Expect(w.buf.String()).To(ContainSubstring(tc.wantPrompt))
		})
	}
}

func TestConsoleImpl_AskQuestion(t *testing.T) {
	RegisterTestingT(t)

	t.Run("trims and returns the answer", func(t *testing.T) {
		RegisterTestingT(t)
		r := &fakeReader{line: "\tanswer\t"}
		w := &captureWriter{}
		c := newTestConsole(r, w)

		resp, err := c.AskQuestion("Name")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp).To(Equal("answer"))
		Expect(w.buf.String()).To(ContainSubstring("Name"))
	})

	t.Run("alwaysDefault errors because no default is available", func(t *testing.T) {
		RegisterTestingT(t)
		r := &fakeReader{line: "ignored"}
		w := &captureWriter{}
		c := newTestConsole(r, w)
		c.AlwaysRespondDefault()

		resp, err := c.AskQuestion("Name")
		Expect(err).To(HaveOccurred())
		Expect(resp).To(Equal(""))
		Expect(err.Error()).To(ContainSubstring("cannot respond with default"))
	})

	t.Run("propagates reader error", func(t *testing.T) {
		RegisterTestingT(t)
		r := &fakeReader{line: "partial", lineErr: errors.New("eof")}
		w := &captureWriter{}
		c := newTestConsole(r, w)

		resp, err := c.AskQuestion("Name")
		Expect(err).To(HaveOccurred())
		Expect(resp).To(Equal("partial"))
	})
}

func TestStdinConsoleReader_ReadLine(t *testing.T) {
	t.Run("reads a line and strips the trailing newline", func(t *testing.T) {
		RegisterTestingT(t)

		// Redirect os.Stdin to a pipe we control, then restore it.
		r, w, err := os.Pipe()
		Expect(err).ToNot(HaveOccurred())
		orig := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = orig }()

		go func() {
			_, _ = io.WriteString(w, "typed-input\n")
			_ = w.Close()
		}()

		line, err := StdinConsoleReader{}.ReadLine()
		Expect(err).ToNot(HaveOccurred())
		Expect(line).To(Equal("typed-input"))
	})

	t.Run("returns an error on EOF with no newline", func(t *testing.T) {
		RegisterTestingT(t)

		r, w, err := os.Pipe()
		Expect(err).ToNot(HaveOccurred())
		orig := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = orig }()

		// Close immediately with no data → ReadString hits io.EOF.
		_ = w.Close()

		_, err = StdinConsoleReader{}.ReadLine()
		Expect(err).To(HaveOccurred())
	})
}

func TestStdoutConsoleWriter_PrintAndPrintln(t *testing.T) {
	RegisterTestingT(t)

	// StdoutConsoleWriter writes to the real stdout; we just exercise the
	// methods to ensure they don't panic and satisfy the interface.
	var w ConsoleWriter = StdoutConsoleWriter{}
	Expect(func() { w.Print("") }).ToNot(Panic())
	Expect(func() { w.Println() }).ToNot(Panic())
}

func TestMultiWriteCloser_FanOutWrite(t *testing.T) {
	RegisterTestingT(t)

	a := &nopWriteCloser{buf: &bytes.Buffer{}}
	b := &nopWriteCloser{buf: &bytes.Buffer{}}

	mw := MultiWriteCloser(a, b)
	n, err := mw.Write([]byte("payload"))
	Expect(err).ToNot(HaveOccurred())
	Expect(n).To(Equal(len("payload")))
	Expect(a.buf.String()).To(Equal("payload"))
	Expect(b.buf.String()).To(Equal("payload"))

	Expect(mw.Close()).To(Succeed())
	Expect(a.closed).To(BeTrue())
	Expect(b.closed).To(BeTrue())
}

func TestMultiWriteCloser_WriteErrorShortCircuits(t *testing.T) {
	RegisterTestingT(t)

	good := &nopWriteCloser{buf: &bytes.Buffer{}}
	bad := &errWriteCloser{writeErr: errors.New("disk full")}
	never := &nopWriteCloser{buf: &bytes.Buffer{}}

	mw := MultiWriteCloser(good, bad, never)
	_, err := mw.Write([]byte("data"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("disk full"))
	// The first writer got the data; the writer after the failing one never did.
	Expect(good.buf.String()).To(Equal("data"))
	Expect(never.buf.String()).To(Equal(""))
}

func TestMultiWriteCloser_CloseErrorPropagates(t *testing.T) {
	RegisterTestingT(t)

	bad := &errWriteCloser{closeErr: errors.New("close boom")}
	mw := MultiWriteCloser(bad)
	err := mw.Close()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("close boom"))
}

// nopWriteCloser is a buffer-backed io.WriteCloser that records closure.
type nopWriteCloser struct {
	buf    *bytes.Buffer
	closed bool
}

func (n *nopWriteCloser) Write(p []byte) (int, error) { return n.buf.Write(p) }
func (n *nopWriteCloser) Close() error                { n.closed = true; return nil }

// errWriteCloser returns configurable errors on Write/Close.
type errWriteCloser struct {
	writeErr error
	closeErr error
}

func (e *errWriteCloser) Write(p []byte) (int, error) {
	if e.writeErr != nil {
		return 0, e.writeErr
	}
	return len(p), nil
}
func (e *errWriteCloser) Close() error { return e.closeErr }

// compile-time guard: our fakes satisfy the io interfaces used by the package.
var (
	_ io.WriteCloser = (*nopWriteCloser)(nil)
	_ io.WriteCloser = (*errWriteCloser)(nil)
)
