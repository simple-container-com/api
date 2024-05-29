package util

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
)

func SafeSplit(s string) []string {
	split := strings.Split(s, " ")

	var result []string
	var inquote string
	var block string
	for _, i := range split {
		if inquote == "" {
			if strings.HasPrefix(i, "'") || strings.HasPrefix(i, "\"") {
				inquote = string(i[0])
				block = strings.TrimPrefix(i, inquote) + " "
			} else {
				result = append(result, i)
			}
		} else {
			if !strings.HasSuffix(i, inquote) {
				block += i + " "
			} else {
				block += strings.TrimSuffix(i, inquote)
				inquote = ""
				result = append(result, block)
				block = ""
			}
		}
	}

	return result
}

func ScanNewLineOrReturn(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// Skip leading spaces.
	start := 0
	for width := 0; start < len(data); start += width {
		var r rune
		r, width = utf8.DecodeRune(data[start:])
		if r != '\n' && r != '\r' {
			break
		}
	}
	// Scan until space, marking end of word.
	for width, i := 0, start; i < len(data); i += width {
		var r rune
		r, width = utf8.DecodeRune(data[i:])
		if r == '\n' || r == '\r' {
			return i + width, data[start:i], nil
		}
	}
	// If we're at EOF, we have a final, non-empty, non-terminated word. Return it.
	if atEOF && len(data) > start {
		return len(data), data[start:], nil
	}
	// Request more data.
	return start, nil, nil
}

func NewLineOrReturnScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Split(ScanNewLineOrReturn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	return scanner
}

// ReaderToCallbackFunc returns function that is meant to be called from a separate goroutine
// function starts streaming from reader to logger and appends extra prefix to each line
func ReaderToCallbackFunc(ctx context.Context, reader io.Reader, callback func(string)) func() error {
	scanner := NewLineOrReturnScanner(reader)
	return func() error {
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !scanner.Scan() {
				if scanner.Err() != nil {
					return errors.Wrapf(scanner.Err(), "failed to read next log stream")
				}
				return nil
			}
			switch scanner.Err() {
			case nil:
				text := scanner.Text()
				callback(text)
			case io.EOF:
				return nil
			default:
				return errors.Wrapf(scanner.Err(), "failed to read next log stream")
			}
		}
	}
}

// ReaderToBufFunc returns function that should be called in a goroutine. It reads lines from
// a provided reader and writes each one into the provided buffer
func ReaderToBufFunc(reader io.Reader, buf *bytes.Buffer) func() error {
	scanner := NewLineOrReturnScanner(reader)
	return func() error {
		for {
			if !scanner.Scan() {
				if scanner.Err() != nil {
					return errors.Wrapf(scanner.Err(), "failed to read next line")
				}
				return nil
			}
			switch scanner.Err() {
			case nil:
				buf.Write(scanner.Bytes())
			default:
				return errors.Wrapf(scanner.Err(), "failed to read next line")
			}
		}
	}
}
