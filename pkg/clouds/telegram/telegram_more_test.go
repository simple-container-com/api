// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package telegram

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestFormatAlertMessage_EmojiAndTitleByType(t *testing.T) {
	cases := []struct {
		name      string
		alertType api.AlertType
		emoji     string
		title     string
	}{
		{"triggered", api.AlertTriggered, "🚨", "Simple Container Alert"},
		{"resolved", api.AlertResolved, "✅", "Simple Container Alert"},
		{"build started", api.BuildStarted, "🔨", "Simple Container Build"},
		{"build succeeded", api.BuildSucceeded, "🎉", "Simple Container Build"},
		{"build failed", api.BuildFailed, "❌", "Simple Container Build"},
		{"build cancelled", api.BuildCancelled, "⏸️", "Simple Container Build"},
		{"unknown -> notification", api.AlertType("WHATEVER"), "📢", "Simple Container Notification"},
		{"empty -> notification", api.AlertType(""), "📢", "Simple Container Notification"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := &alertSender{}
			got := s.formatAlertMessage(api.Alert{AlertType: tc.alertType})
			Expect(got).To(HavePrefix(tc.emoji + " <b>" + tc.title + "</b>"))
		})
	}
}

func TestFormatAlertMessage_AllFieldsRendered(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	alert := api.Alert{
		Name:          "svc-staging",
		Title:         "Deploy Failed",
		Description:   "boom happened",
		Reason:        "exit code 1",
		AlertType:     api.BuildFailed,
		StackName:     "svc",
		StackEnv:      "staging",
		CommitAuthor:  "alice",
		CommitMessage: "fix the build",
		DetailsUrl:    "https://ci/run/9",
	}
	got := s.formatAlertMessage(alert)

	Expect(got).To(ContainSubstring("<b>Name:</b> svc-staging"))
	Expect(got).To(ContainSubstring("<b>Title:</b> Deploy Failed"))
	Expect(got).To(ContainSubstring("<b>Description:</b> boom happened"))
	Expect(got).To(ContainSubstring("<b>Reason:</b> exit code 1"))
	Expect(got).To(ContainSubstring("<b>Type:</b> <code>BUILD_FAILED</code>"))
	Expect(got).To(ContainSubstring("<b>Stack:</b> <code>svc</code>"))
	Expect(got).To(ContainSubstring("<b>Environment:</b> <code>staging</code>"))
	Expect(got).To(ContainSubstring("<b>Author:</b> alice"))
	Expect(got).To(ContainSubstring("<b>Commit:</b> fix the build"))
	Expect(got).To(ContainSubstring("<b>Details:</b> https://ci/run/9"))
	// Footer timestamp line.
	Expect(got).To(ContainSubstring("⏰ <i>"))
}

func TestFormatAlertMessage_LongCommitMessageTruncated(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	got := s.formatAlertMessage(api.Alert{
		AlertType:     api.BuildSucceeded,
		CommitMessage: strings.Repeat("k", 200),
	})
	// Commit > 100 bytes is cut to first 97 chars + "...".
	Expect(got).To(ContainSubstring("<b>Commit:</b> " + strings.Repeat("k", 97) + "..."))
	Expect(got).ToNot(ContainSubstring(strings.Repeat("k", 101)))
}

func TestFormatAlertMessage_EmptyAlert_OnlyHeaderAndFooter(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	got := s.formatAlertMessage(api.Alert{})
	Expect(got).To(HavePrefix("📢 <b>Simple Container Notification</b>"))
	Expect(got).To(ContainSubstring("⏰ <i>"))
	Expect(got).ToNot(ContainSubstring("<b>Name:</b>"))
	Expect(got).ToNot(ContainSubstring("<b>Type:</b>"))
}

func TestTruncateMessage_ShortReturnedUnchanged(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	msg := "short and sweet"
	Expect(s.truncateMessage(msg)).To(Equal(msg))
}

func TestTruncateMessage_FewLines_SimpleTrim(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	// A single (no-newline) over-limit message has < 3 lines, hitting the
	// simple-truncation branch.
	msg := strings.Repeat("a", maxTelegramMessageLength+500)
	got := s.truncateMessage(msg)

	Expect(len(got)).To(BeNumerically("<", len(msg)))
	Expect(got).To(ContainSubstring("[Message truncated due to length]"))
}

// TestTruncateMessage_StructuredMarkdown_VerySmallSpace exercises the
// "essentials only" branch. NOTE/QUIRK: truncateMessage classifies fields by
// MARKDOWN prefixes (**Description:**, **Reason:**, **Type:** ...), but the
// production formatAlertMessage emits HTML prefixes (<b>Description:</b>). Real
// messages therefore never match these classifiers; we feed synthetic
// Markdown-shaped input to reach the branch logic the parser was written for.
func TestTruncateMessage_StructuredMarkdown_EssentialsOnly(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}

	var b strings.Builder
	b.WriteString("🚨 Header\n")
	b.WriteString("**Name:** my-service\n")
	b.WriteString("**Title:** A Title\n")
	// Huge header lines so the computed availableSpace drops below 200, taking
	// the essentials-only path.
	b.WriteString(strings.Repeat("header padding line\n", 250))
	b.WriteString("**Description:** " + strings.Repeat("d", 3000) + "\n")
	b.WriteString("**Reason:** " + strings.Repeat("r", 1000) + "\n")
	b.WriteString("**Type:** BUILD_FAILED\n")
	b.WriteString("**Stack:** svc\n")
	b.WriteString("**Environment:** prod\n")
	b.WriteString("**Details:** https://ci/run\n")
	b.WriteString("⏰ time")

	got := s.truncateMessage(b.String())
	Expect(len(got)).To(BeNumerically("<=", maxTelegramMessageLength))
	Expect(got).To(ContainSubstring("Error details truncated"))
}

func TestTruncateMessage_StructuredMarkdown_IntelligentDescAndReason(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}

	var b strings.Builder
	b.WriteString("🚨 Header\n")
	b.WriteString("**Name:** my-service\n")
	b.WriteString("**Title:** A Title\n")
	// Description + Reason large enough to exceed availableSpace/2 each so the
	// intelligentTruncate path runs for BOTH, but small header/footer keep
	// availableSpace >= 200.
	b.WriteString("**Description:** START-D\n" + strings.Repeat("d", 3500) + "\nEND-D\n")
	b.WriteString("**Reason:** START-R\n" + strings.Repeat("r", 3500) + "\nEND-R\n")
	b.WriteString("**Type:** BUILD_FAILED\n")
	b.WriteString("⏰ time")

	got := s.truncateMessage(b.String())
	Expect(len(got)).To(BeNumerically("<=", maxTelegramMessageLength))
	Expect(got).To(ContainSubstring("Error details truncated"))
	// Intelligent truncation keeps both ends of the description/reason.
	Expect(got).To(ContainSubstring("[... truncated ...]"))
}

func TestTruncateMessage_EssentialsOnly_FinalTrim(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}

	// Force the essentials-only branch (availableSpace < 200) AND make the
	// collected essential lines themselves exceed the limit so the final
	// `len(result) > max` trim (telegram_alert.go:305-307) runs. The Title is
	// an essential line and is kept verbatim.
	var b strings.Builder
	b.WriteString("🚨 Header\n")
	b.WriteString("**Title:** " + strings.Repeat("T", maxTelegramMessageLength+500) + "\n")
	b.WriteString(strings.Repeat("header padding line\n", 250)) // shrinks availableSpace
	b.WriteString("**Description:** " + strings.Repeat("d", 2000) + "\n")
	b.WriteString("⏰ time")

	got := s.truncateMessage(b.String())
	Expect(len(got)).To(Equal(maxTelegramMessageLength))
	Expect(got).To(HaveSuffix("..."))
}

func TestTruncateMessage_MainReconstructPath_StaysUnderLimit(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}

	// Exercise the main reconstruct path (availableSpace >= 200): Description
	// and Reason are intelligently truncated and the message is rebuilt from
	// header + truncated details + indicator + footer.
	//
	// NOTE/DEAD-CODE: the final safety trim at telegram_alert.go:338-340 is
	// unreachable from this path. availableSpace = max - header - footer - 190,
	// and the truncated details are bounded by availableSpace, so the rebuilt
	// result is always <= max - 100 (here ~3900). We therefore assert the
	// result stays strictly under the limit (no "..." final trim) rather than
	// trying to force an impossible >max reconstruction.
	var b strings.Builder
	b.WriteString("🚨 Header\n")
	b.WriteString("**Name:** svc\n")
	b.WriteString("**Description:** START-D\n" + strings.Repeat("d", 3500) + "\nEND-D\n")
	b.WriteString("**Reason:** START-R\n" + strings.Repeat("r", 3500) + "\nEND-R\n")
	b.WriteString("**Type:** BUILD_FAILED\n")
	b.WriteString("⏰ time")

	got := s.truncateMessage(b.String())
	Expect(len(got)).To(BeNumerically("<", maxTelegramMessageLength))
	Expect(got).To(ContainSubstring("Error details truncated"))
	Expect(got).To(ContainSubstring("[... truncated ...]"))
}

func TestIntelligentTruncate_SmallMaxLength_ClampsToMinimums(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	// maxLength=160 -> availableSpace=137 so beginningLen (45) is below the
	// 50-byte floor and the recomputed endLen (87) is below the 100-byte floor,
	// exercising BOTH clamp branches (telegram_alert.go:365-372).
	//
	// QUIRK (telegram_alert.go:379-380, same as slack): with the clamped
	// beginningLen < 100 and a newline-free beginning window, LastIndex returns
	// -1 which still passes `-1 > beginningLen-100`, so `beginning[:-1]` would
	// PANIC. An early newline keeps the trim index positive.
	text := "abc\n" + strings.Repeat("p", 5000)
	got := s.intelligentTruncate(text, 160)

	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(got).To(HavePrefix("abc\n\n[... truncated ...]"))
}

func TestIntelligentTruncate_ShortText_Unchanged(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	short := "tiny"
	Expect(s.intelligentTruncate(short, 100)).To(Equal(short))
}

func TestIntelligentTruncate_LongText_KeepsBeginningAndEnd(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	text := "START-MARKER\n" + strings.Repeat("middle-", 800) + "\nEND-MARKER"
	got := s.intelligentTruncate(text, 1000)

	Expect(got).To(ContainSubstring("START-MARKER"))
	Expect(got).To(ContainSubstring("END-MARKER"))
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(len(got)).To(BeNumerically("<", len(text)))
}

func TestIntelligentTruncate_NewlineBoundaryTrims(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{}
	// maxLength=900 -> availableSpace=877, beginningLen=292, endLen=585.
	// Newline at 250 lands within the last 100 bytes of the beginning window;
	// newline at 5465 lands ~50 bytes into the end window (total length 6000).
	buf := []byte(strings.Repeat("x", 6000))
	buf[250] = '\n'
	buf[5465] = '\n'
	text := string(buf)

	got := s.intelligentTruncate(text, 900)
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(got).To(HavePrefix(strings.Repeat("x", 250) + "\n\n[... truncated ...]"))
}

func TestGetBotID(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"empty token", "", "empty"},
		{"normal token with colon", "123456789:AAEabcdef", "123456789"},
		{"no colon, short (<=10)", "shorttok", "shorttok"},
		{"no colon, long (>10)", "abcdefghijklmnop", "abcdefghij..."},
		{"colon at index 0 -> falls through to length check", ":xyz", ":xyz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := &alertSender{token: tc.token}
			Expect(s.getBotID()).To(Equal(tc.want))
		})
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"present", "123:abc", ":", true},
		{"absent", "123abc", ":", false},
		{"prefix", "abcdef", "abc", true},
		{"suffix", "abcdef", "def", true},
		{"empty substr always matches", "abc", "", true},
		{"substr longer than string", "ab", "abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(contains(tc.s, tc.substr)).To(Equal(tc.want))
		})
	}
}

func TestSend_MissingToken_Errors(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{chatId: "c", token: ""}
	err := s.Send(api.Alert{AlertType: api.BuildStarted})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("token is required"))
}

func TestSend_MissingChatId_Errors(t *testing.T) {
	RegisterTestingT(t)

	s := &alertSender{chatId: "", token: "123:abc"}
	err := s.Send(api.Alert{AlertType: api.BuildStarted})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("chat ID is required"))
}
