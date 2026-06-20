// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package helpers

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

func TestHealthBridgeLambda_HandlerAndSetLogger(t *testing.T) {
	RegisterTestingT(t)

	t.Run("handler is a no-op stub that always succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		// The health-bridge handler is currently a stub that logs the sanitized
		// event and returns nil. Assert the documented stub contract so a future
		// real implementation forces an update here.
		l := &lambdaHealthBridgeCloudHelper{log: logger.New()}
		Expect(l.handler(context.Background(), map[string]any{"detail-type": "AWS Health Event"})).To(Succeed())
		// Even a structurally odd event is accepted by the stub.
		Expect(l.handler(context.Background(), "anything")).To(Succeed())
	})

	t.Run("SetLogger stores the logger", func(t *testing.T) {
		RegisterTestingT(t)
		l := &lambdaHealthBridgeCloudHelper{}
		log := logger.New()
		l.SetLogger(log)
		Expect(l.log).To(Equal(log))
	})
}

func TestNewHealthBridgeLambdaHelper(t *testing.T) {
	RegisterTestingT(t)

	t.Run("applies options and returns a CloudHelper", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := NewHealthBridgeLambdaHelper(api.WithLogger(logger.New()))
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeNil())
		hb, ok := h.(*lambdaHealthBridgeCloudHelper)
		Expect(ok).To(BeTrue())
		Expect(hb.log).ToNot(BeNil())
	})

	t.Run("no options succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		h, err := NewHealthBridgeLambdaHelper()
		Expect(err).ToNot(HaveOccurred())
		Expect(h).ToNot(BeNil())
	})

	t.Run("propagates option errors", func(t *testing.T) {
		RegisterTestingT(t)
		boom := errors.New("kaboom")
		h, err := NewHealthBridgeLambdaHelper(func(c api.CloudHelper) error { return boom })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to apply option on lambda helper"))
		Expect(err.Error()).To(ContainSubstring("kaboom"))
		Expect(h).To(BeNil())
	})
}

func TestSanitizeForLog(t *testing.T) {
	RegisterTestingT(t)

	t.Run("serialises a struct to single-line JSON", func(t *testing.T) {
		RegisterTestingT(t)
		out := sanitizeForLog(map[string]any{"a": 1, "b": "x"})
		Expect(out).To(ContainSubstring(`"a":1`))
		Expect(out).To(ContainSubstring(`"b":"x"`))
	})

	t.Run("strips embedded CR/LF so log lines can't be forged", func(t *testing.T) {
		RegisterTestingT(t)
		// json.Marshal escapes \n inside a string to the two-character sequence
		// backslash-n, so the raw newline never reaches the log; the residual
		// Replacer is the CodeQL-recognised sanitizer. Verify no raw CR/LF byte
		// survives in the output.
		out := sanitizeForLog(map[string]any{"msg": "line1\nline2\rline3"})
		Expect(out).ToNot(ContainSubstring("\n"))
		Expect(out).ToNot(ContainSubstring("\r"))
		// The escaped form is what remains inside the JSON string.
		Expect(out).To(ContainSubstring(`line1\nline2\rline3`))
	})

	t.Run("unmarshallable value falls back to a typed placeholder", func(t *testing.T) {
		RegisterTestingT(t)
		// A func value cannot be JSON-marshalled, so the error branch renders a
		// "%T(<unmarshallable>)" placeholder instead of panicking or returning "".
		out := sanitizeForLog(func() {})
		Expect(out).To(ContainSubstring("<unmarshallable>"))
		Expect(out).To(ContainSubstring("func()"))
	})

	t.Run("channel value also hits the unmarshallable fallback", func(t *testing.T) {
		RegisterTestingT(t)
		out := sanitizeForLog(make(chan int))
		Expect(out).To(ContainSubstring("<unmarshallable>"))
	})
}
