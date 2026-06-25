// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package slack

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestGetIconForAlertType(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name string
		in   api.AlertType
		want string
	}{
		{"AlertTriggered -> warning", api.AlertTriggered, "⚠️"},
		{"AlertResolved -> check", api.AlertResolved, "✅"},
		{"BuildStarted -> rocket", api.BuildStarted, "🚀"},
		{"BuildSucceeded -> check", api.BuildSucceeded, "✅"},
		{"BuildFailed -> cross", api.BuildFailed, "❌"},
		{"BuildCancelled -> stop", api.BuildCancelled, "⏹️"},
		{"unknown -> info default", api.AlertType("NOPE"), "ℹ️"},
		{"empty -> info default", api.AlertType(""), "ℹ️"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(getIconForAlertType(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestNew_ReturnsSender(t *testing.T) {
	RegisterTestingT(t)

	sender, err := New("https://hooks.slack.com/services/T/B/X")
	Expect(err).ToNot(HaveOccurred())
	Expect(sender).ToNot(BeNil())

	as, ok := sender.(*alertSender)
	Expect(ok).To(BeTrue())
	Expect(as.webhookUrl).To(Equal("https://hooks.slack.com/services/T/B/X"))
}

func TestNew_EmptyURL_StillConstructs(t *testing.T) {
	RegisterTestingT(t)

	// The constructor performs no validation; an empty URL is accepted and
	// the (eventual) failure surfaces at Send time.
	sender, err := New("")
	Expect(err).ToNot(HaveOccurred())
	Expect(sender).ToNot(BeNil())
}

func TestIntelligentTruncate_ShortText_Unchanged(t *testing.T) {
	RegisterTestingT(t)

	short := "this is short enough"
	Expect(intelligentTruncate(short, 100)).To(Equal(short))
}

func TestIntelligentTruncate_ExactBoundary_Unchanged(t *testing.T) {
	RegisterTestingT(t)

	text := strings.Repeat("a", 100)
	Expect(intelligentTruncate(text, 100)).To(Equal(text))
}

func TestIntelligentTruncate_LongText_KeepsBeginningAndEnd(t *testing.T) {
	RegisterTestingT(t)

	body := strings.Repeat("middle-noise-", 200)
	text := "START-MARKER\n" + body + "\nEND-MARKER"

	got := intelligentTruncate(text, 600)

	Expect(got).To(ContainSubstring("START-MARKER"))
	Expect(got).To(ContainSubstring("END-MARKER"))
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(len(got)).To(BeNumerically("<=", 700))
}

func TestIntelligentTruncate_SmallMaxLength_ClampsToMinimums(t *testing.T) {
	RegisterTestingT(t)

	// maxLength=160 drives availableSpace=137 so the raw beginningLen (45) is
	// below the 50-byte floor and the recomputed endLen (87) is below the
	// 100-byte floor, exercising BOTH minimum-length clamp branches (slack's
	// helper, unlike discord's, has no negative-length guard).
	//
	// QUIRK (slack_alert.go:113-114): when beginningLen ends up < 100 AND the
	// beginning window contains no newline, strings.LastIndex returns -1 which
	// still satisfies `-1 > beginningLen-100`, so `beginning[:-1]` PANICS with
	// slice-out-of-range. We therefore place a newline early in the beginning
	// window so the trim slices to a valid (positive) index. A purely
	// newline-free input at this maxLength would crash the helper.
	text := "abc\n" + strings.Repeat("p", 5000)
	got := intelligentTruncate(text, 160)

	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(len(got)).To(BeNumerically("<", len(text)))
	// The beginning is trimmed at the newline (index 3) -> "abc".
	Expect(got).To(HavePrefix("abc\n\n[... truncated ...]"))
}

func TestIntelligentTruncate_NewlineBoundaryBreaks(t *testing.T) {
	RegisterTestingT(t)

	// Place a newline near the very end of the beginning window and one near
	// the very start of the end window so both line-boundary trim branches
	// (LastIndex on beginning, Index on end) are exercised.
	begin := strings.Repeat("b", 200) + "\nBEGIN-TAIL"
	mid := strings.Repeat("m", 4000)
	end := "END-HEAD\n" + strings.Repeat("e", 200)
	text := begin + mid + end

	got := intelligentTruncate(text, 900)
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(len(got)).To(BeNumerically("<", len(text)))
}

// sendServer spins up an httptest server returning the given status code and
// captures the last request body so the test can assert the posted payload.
func sendServer(t *testing.T, status int, captured *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			*captured = string(buf)
		}
		w.WriteHeader(status)
	}))
}

func TestSend_Success_PostsExpectedPayload(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	alert := api.Alert{
		AlertType:  api.BuildFailed,
		Title:      "Deploy Failed",
		StackName:  "payments",
		StackEnv:   "production",
		DetailsUrl: "https://ci/run/1",
	}

	Expect(sender.Send(alert)).To(Succeed())

	// The marshalled Slack message carries the icon + formatted text and the
	// markdown flag (json tag mrkdwn).
	Expect(body).To(ContainSubstring("BUILD_FAILED"))
	Expect(body).To(ContainSubstring("Deploy Failed"))
	Expect(body).To(ContainSubstring("payments"))
	Expect(body).To(ContainSubstring("production"))
	Expect(body).To(ContainSubstring(`"mrkdwn":true`))
	// The cross icon for BuildFailed is prepended.
	Expect(body).To(ContainSubstring("❌"))
}

func TestSend_WithCommitAuthorAndMessage(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	alert := api.Alert{
		AlertType:     api.BuildStarted,
		Title:         "t",
		CommitAuthor:  "alice",
		CommitMessage: "fix the thing",
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(body).To(ContainSubstring("👤 Author: alice"))
	Expect(body).To(ContainSubstring("💬 fix the thing"))
	// Author + message present -> the bullet separator appears.
	Expect(body).To(ContainSubstring(" • "))
}

func TestSend_CommitMessageOnly_NoBulletSeparator(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	alert := api.Alert{
		AlertType:     api.BuildSucceeded,
		Title:         "t",
		CommitMessage: "solo commit",
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(body).To(ContainSubstring("💬 solo commit"))
	Expect(body).ToNot(ContainSubstring("👤 Author"))
	Expect(body).ToNot(ContainSubstring(" • "))
}

func TestSend_LongCommitMessage_Truncated(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	longCommit := strings.Repeat("z", 200)
	alert := api.Alert{
		AlertType:     api.AlertTriggered,
		Title:         "t",
		CommitMessage: longCommit,
	}

	Expect(sender.Send(alert)).To(Succeed())
	// Commit messages over 100 bytes are cut to 97 + "..." (JSON-escaped dots).
	Expect(body).To(ContainSubstring(strings.Repeat("z", 97) + "..."))
	Expect(body).ToNot(ContainSubstring(strings.Repeat("z", 101)))
}

func TestSend_ShortDescription_Appended(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	alert := api.Alert{
		AlertType:   api.AlertResolved,
		Title:       "t",
		Description: "all clear now",
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(body).To(ContainSubstring("all clear now"))
}

func TestSend_VeryLongDescription_IntelligentlyTruncated(t *testing.T) {
	RegisterTestingT(t)

	var body string
	srv := sendServer(t, http.StatusOK, &body)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	// Over 2000 bytes triggers the intelligentTruncate branch in Send.
	desc := "DESC-START\n" + strings.Repeat("q", 5000) + "\nDESC-END"
	alert := api.Alert{
		AlertType:   api.BuildFailed,
		Title:       "t",
		Description: desc,
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(body).To(ContainSubstring("DESC-START"))
	Expect(body).To(ContainSubstring("DESC-END"))
	Expect(body).To(ContainSubstring("[... truncated ...]"))
}

func TestSend_Non2xx_ReturnsError(t *testing.T) {
	RegisterTestingT(t)

	srv := sendServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()

	sender := &alertSender{webhookUrl: srv.URL}
	err := sender.Send(api.Alert{AlertType: api.BuildFailed, Title: "t"})
	Expect(err).To(HaveOccurred())
	// The slack-webhook library reports the HTTP status on >= 400.
	Expect(err.Error()).To(ContainSubstring("500"))
}

func TestSend_UnreachableURL_ReturnsError(t *testing.T) {
	RegisterTestingT(t)

	// A server that is created then immediately closed yields a connection
	// refused, exercising the transport-error branch of slack.Send.
	srv := sendServer(t, http.StatusOK, nil)
	url := srv.URL
	srv.Close()

	sender := &alertSender{webhookUrl: url}
	err := sender.Send(api.Alert{AlertType: api.BuildStarted, Title: "t"})
	Expect(err).To(HaveOccurred())
}
