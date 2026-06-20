package util

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	. "github.com/onsi/gomega"
)

func TestScanNewLineOrReturn(t *testing.T) {
	cases := []struct {
		name        string
		data        string
		atEOF       bool
		wantAdvance int
		wantToken   string
		wantErr     bool
	}{
		{
			name:        "splits on newline",
			data:        "abc\ndef",
			atEOF:       false,
			wantAdvance: 4, // "abc" + the '\n'
			wantToken:   "abc",
		},
		{
			name:        "splits on carriage return",
			data:        "abc\rdef",
			atEOF:       false,
			wantAdvance: 4,
			wantToken:   "abc",
		},
		{
			name:        "skips leading newlines and returns next token",
			data:        "\n\nabc\n",
			atEOF:       false,
			wantAdvance: 6, // two leading newlines skipped, "abc", trailing '\n'
			wantToken:   "abc",
		},
		{
			name:        "non-terminated word at EOF is returned",
			data:        "trailing",
			atEOF:       true,
			wantAdvance: 8,
			wantToken:   "trailing",
		},
		{
			name:        "no terminator and not EOF requests more data",
			data:        "partial",
			atEOF:       false,
			wantAdvance: 0,
			wantToken:   "",
		},
		{
			name:        "only newlines at EOF yields empty token",
			data:        "\n\n",
			atEOF:       true,
			wantAdvance: 2, // start advances past both newlines, len(data)==start so no final word
			wantToken:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			advance, token, err := ScanNewLineOrReturn([]byte(tc.data), tc.atEOF)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(advance).To(Equal(tc.wantAdvance))
			Expect(string(token)).To(Equal(tc.wantToken))
		})
	}
}

func TestNewLineOrReturnScanner_SplitsOnBothTerminators(t *testing.T) {
	RegisterTestingT(t)

	// Mixed \n and \r separators should both split.
	scanner := NewLineOrReturnScanner(bytes.NewBufferString("one\ntwo\rthree\n"))
	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	Expect(scanner.Err()).ToNot(HaveOccurred())
	Expect(tokens).To(Equal([]string{"one", "two", "three"}))
}

func TestNewLineOrReturnScanner_HandlesLongLineWithinBuffer(t *testing.T) {
	RegisterTestingT(t)

	// The scanner pre-sizes a 1MiB buffer; a line well under that must not
	// trigger bufio.ErrTooLong.
	long := bytes.Repeat([]byte("x"), 500_000)
	scanner := NewLineOrReturnScanner(bytes.NewReader(append(long, '\n')))
	Expect(scanner.Scan()).To(BeTrue())
	Expect(len(scanner.Bytes())).To(Equal(500_000))
	Expect(scanner.Err()).ToNot(HaveOccurred())
}

func TestReaderToBufFunc(t *testing.T) {
	t.Run("concatenates scanned lines into buffer without separators", func(t *testing.T) {
		RegisterTestingT(t)
		var buf bytes.Buffer
		fn := ReaderToBufFunc(bytes.NewBufferString("alpha\nbeta\rgamma\n"), &buf)
		Expect(fn()).To(Succeed())
		// ReaderToBufFunc writes raw bytes of each token with no delimiter.
		Expect(buf.String()).To(Equal("alphabetagamma"))
	})

	t.Run("empty reader yields empty buffer", func(t *testing.T) {
		RegisterTestingT(t)
		var buf bytes.Buffer
		fn := ReaderToBufFunc(bytes.NewBufferString(""), &buf)
		Expect(fn()).To(Succeed())
		Expect(buf.Len()).To(Equal(0))
	})

	t.Run("reader error is wrapped", func(t *testing.T) {
		RegisterTestingT(t)
		var buf bytes.Buffer
		fn := ReaderToBufFunc(&failingReader{err: errors.New("read kaput")}, &buf)
		err := fn()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read next line"))
		Expect(err.Error()).To(ContainSubstring("read kaput"))
	})
}

func TestReaderToCallbackFunc(t *testing.T) {
	t.Run("invokes callback per scanned line", func(t *testing.T) {
		RegisterTestingT(t)
		var got []string
		fn := ReaderToCallbackFunc(context.Background(), bytes.NewBufferString("l1\nl2\nl3\n"),
			func(line string) { got = append(got, line) })
		Expect(fn()).To(Succeed())
		Expect(got).To(Equal([]string{"l1", "l2", "l3"}))
	})

	t.Run("returns ctx error when context already cancelled", func(t *testing.T) {
		RegisterTestingT(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before running
		called := false
		fn := ReaderToCallbackFunc(ctx, bytes.NewBufferString("never\n"),
			func(string) { called = true })
		err := fn()
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(called).To(BeFalse())
	})

	t.Run("reader error is wrapped", func(t *testing.T) {
		RegisterTestingT(t)
		fn := ReaderToCallbackFunc(context.Background(), &failingReader{err: errors.New("boom")},
			func(string) {})
		err := fn()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read next log stream"))
	})

	t.Run("empty reader completes without invoking callback", func(t *testing.T) {
		RegisterTestingT(t)
		called := false
		fn := ReaderToCallbackFunc(context.Background(), bytes.NewBufferString(""),
			func(string) { called = true })
		Expect(fn()).To(Succeed())
		Expect(called).To(BeFalse())
	})
}

// ensure the io.Reader fakes used here are valid (failingReader defined in
// logger_more_test.go is reused).
var _ io.Reader = bytes.NewBufferString("")
