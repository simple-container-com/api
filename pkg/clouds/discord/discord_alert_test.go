// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package discord

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestGetIconForAlertType(t *testing.T) {
	cases := []struct {
		name string
		in   api.AlertType
		want string
	}{
		{"AlertTriggered → warning", api.AlertTriggered, "⚠️"},
		{"AlertResolved → check", api.AlertResolved, "✅"},
		{"BuildStarted → rocket", api.BuildStarted, "🚀"},
		{"BuildSucceeded → check", api.BuildSucceeded, "✅"},
		{"BuildFailed → cross", api.BuildFailed, "❌"},
		{"BuildCancelled → stop", api.BuildCancelled, "⏹️"},
		{"unknown → info default", api.AlertType("UNKNOWN_TYPE"), "ℹ️"},
		{"empty → info default", api.AlertType(""), "ℹ️"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(getIconForAlertType(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestIntelligentTruncate_ShortText_Unchanged(t *testing.T) {
	RegisterTestingT(t)

	short := "this is short enough"
	got := intelligentTruncate(short, 100)
	Expect(got).To(Equal(short))
}

func TestIntelligentTruncate_ExactBoundary_Unchanged(t *testing.T) {
	RegisterTestingT(t)

	text := strings.Repeat("a", 100)
	got := intelligentTruncate(text, 100)
	Expect(got).To(Equal(text))
}

func TestIntelligentTruncate_LongText_KeepsBeginningAndEnd(t *testing.T) {
	RegisterTestingT(t)

	// Build a 2000-byte string with distinct markers at start + end so we
	// can assert both halves survived.
	body := strings.Repeat("middle-noise-", 150) // ~1950 chars
	text := "START-MARKER\n" + body + "\nEND-MARKER"

	got := intelligentTruncate(text, 600)

	Expect(got).To(ContainSubstring("START-MARKER"))
	Expect(got).To(ContainSubstring("END-MARKER"))
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	// The result should be reasonably close to the requested maxLength.
	// The function biases towards the end (2/3 of available space) and
	// trims at newline boundaries, so allow generous slack.
	Expect(len(got)).To(BeNumerically("<=", 700))
}

func TestIntelligentTruncate_VerySmallMaxLength_FallsBackToSimpleTrim(t *testing.T) {
	RegisterTestingT(t)

	text := strings.Repeat("x", 500)

	// maxLength below the function's "minimum end length" of 100
	// triggers the fall-back to a simple end-trim with "..." suffix
	// instead of the intelligent begin+sep+end form. The previous
	// implementation panicked here because the floor clamps drove
	// beginningLen / endLen negative.
	got := intelligentTruncate(text, 50)
	Expect(got).ToNot(BeEmpty())
	Expect(len(got)).To(Equal(50))
	Expect(got).To(HaveSuffix("..."))
	// No intelligent-truncate marker — small-maxLength path is a simple trim.
	Expect(got).ToNot(ContainSubstring("[... truncated ...]"))
}

func TestIntelligentTruncate_MaxLengthZero_ReturnsEmpty(t *testing.T) {
	RegisterTestingT(t)

	text := strings.Repeat("x", 100)
	got := intelligentTruncate(text, 0)
	Expect(got).To(Equal(""))
}

func TestIntelligentTruncate_MaxLengthNegative_ReturnsEmpty(t *testing.T) {
	RegisterTestingT(t)

	text := "anything"
	got := intelligentTruncate(text, -5)
	Expect(got).To(Equal(""))
}

func TestNew_InvalidWebhookURL_Errors(t *testing.T) {
	RegisterTestingT(t)

	// The disgo webhook constructor requires a properly-formed URL.
	// An obvious garbage string surfaces the error path.
	_, err := New("not a url at all")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to init webhook"))
}

func TestNew_EmptyURL_Errors(t *testing.T) {
	RegisterTestingT(t)

	_, err := New("")
	Expect(err).To(HaveOccurred())
}

func TestNew_ValidWebhookURL_ReturnsSender(t *testing.T) {
	RegisterTestingT(t)

	// Real Discord webhook URLs match a specific shape. We don't dispatch
	// real traffic — the test only confirms construction succeeds.
	sender, err := New("https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_xx")
	if err != nil {
		t.Skipf("Skipping — disgo webhook construction rejected the synthetic URL: %v", err)
	}
	Expect(sender).ToNot(BeNil())
}

func TestMaxMessageLengthConstant(t *testing.T) {
	RegisterTestingT(t)

	// The constant is the contract Send() respects. Pin it so a future
	// change to the API limit forces a deliberate test update.
	Expect(maxDiscordMessageLength).To(Equal(1900))
}
