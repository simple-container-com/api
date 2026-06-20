// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package logger

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

// captureStdout redirects os.Stdout for the lifetime of fn and returns
// what was written. Used here because the logger writes directly to
// stdout via fmt.Println instead of taking a writer parameter.
//
// Order is critical: close the writer + wait for the copier goroutine
// BEFORE reading the buffer. A `defer` runs after the return value is
// evaluated, so we cannot use defer here without losing the captured
// output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
	}()

	fn()
	_ = w.Close()
	wg.Wait()
	os.Stdout = orig

	return buf.String()
}

func TestNewReturnsLogger(t *testing.T) {
	RegisterTestingT(t)

	l := New()
	Expect(l).ToNot(BeNil())
}

func TestLogLevelConstants(t *testing.T) {
	RegisterTestingT(t)

	// The numeric ordering of the levels is the contract that
	// getLogLevel and the Debug/Info/Warn/Error gates rely on.
	Expect(LogLevelDebug).To(BeNumerically("<", LogLevelInfo))
	Expect(LogLevelInfo).To(BeNumerically("<", LogLevelWarn))
	Expect(LogLevelWarn).To(BeNumerically("<", LogLevelError))
	Expect(LogLevelError).To(BeNumerically("<", LogLevelSilent))
}

func TestInfo_DefaultLevel_Emits(t *testing.T) {
	RegisterTestingT(t)

	l := New()
	out := captureStdout(t, func() {
		l.Info(context.Background(), "hello %s", "world")
	})

	Expect(out).To(ContainSubstring("INFO"))
	Expect(out).To(ContainSubstring("hello world"))
}

func TestDebug_DefaultLevel_Suppressed(t *testing.T) {
	RegisterTestingT(t)

	// Default is Info; Debug should not emit.
	l := New()
	out := captureStdout(t, func() {
		l.Debug(context.Background(), "noisy debug")
	})
	Expect(out).To(BeEmpty())
}

func TestSetLogLevel_Debug_EmitsDebug(t *testing.T) {
	RegisterTestingT(t)

	l := New()
	ctx := l.SetLogLevel(context.Background(), LogLevelDebug)

	out := captureStdout(t, func() {
		l.Debug(ctx, "debug line")
	})
	Expect(out).To(ContainSubstring("DEBUG"))
	Expect(out).To(ContainSubstring("debug line"))
}

func TestSetLogLevel_Silent_SuppressesAll(t *testing.T) {
	RegisterTestingT(t)

	l := New()
	ctx := l.Silent(context.Background())

	out := captureStdout(t, func() {
		l.Error(ctx, "err")
		l.Warn(ctx, "warn")
		l.Info(ctx, "info")
		l.Debug(ctx, "debug")
	})
	Expect(out).To(BeEmpty())
}

func TestLevelGating(t *testing.T) {
	cases := []struct {
		name        string
		level       int
		expectError bool
		expectWarn  bool
		expectInfo  bool
		expectDebug bool
	}{
		{"debug shows all", LogLevelDebug, true, true, true, true},
		{"info hides debug", LogLevelInfo, true, true, true, false},
		{"warn hides info+debug", LogLevelWarn, true, true, false, false},
		{"error hides warn+info+debug", LogLevelError, true, false, false, false},
		{"silent hides everything", LogLevelSilent, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			l := New()
			ctx := l.SetLogLevel(context.Background(), tc.level)

			out := captureStdout(t, func() {
				l.Error(ctx, "X-ERROR-X")
				l.Warn(ctx, "X-WARN-X")
				l.Info(ctx, "X-INFO-X")
				l.Debug(ctx, "X-DEBUG-X")
			})

			Expect(strings.Contains(out, "X-ERROR-X")).To(Equal(tc.expectError))
			Expect(strings.Contains(out, "X-WARN-X")).To(Equal(tc.expectWarn))
			Expect(strings.Contains(out, "X-INFO-X")).To(Equal(tc.expectInfo))
			Expect(strings.Contains(out, "X-DEBUG-X")).To(Equal(tc.expectDebug))
		})
	}
}
