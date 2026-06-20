// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package discord

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/disgo/webhook"
	"github.com/disgoorg/snowflake/v2"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// roundTripFunc adapts a function to http.RoundTripper so tests can intercept
// the disgo rest client's HTTP traffic without touching discord.com.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newSenderWithTransport builds an *alertSender whose webhook client routes all
// REST traffic through rt. The production New() always targets discord.com via
// the default rest client, so to exercise Send() deterministically (without a
// real network call) we construct the client directly with an injected
// transport — identical to what production builds, minus the hard-coded host.
func newSenderWithTransport(t *testing.T, rt roundTripFunc) *alertSender {
	t.Helper()
	client := webhook.New(
		snowflake.ID(123456789012345678),
		"test-token",
		webhook.WithRestClientConfigOpts(rest.WithHTTPClient(&http.Client{Transport: rt})),
	)
	return &alertSender{client: client, webhookUrl: "https://discord.com/api/webhooks/123/abc"}
}

// okResponse returns a 200 with an empty JSON object so the rest client's
// success branch (unmarshal into *discord.Message) completes cleanly.
func okResponse(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

func TestSend_Success_BuildsAndPosts(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			captured = string(b)
		}
		return okResponse(r)
	})

	alert := api.Alert{
		AlertType:  api.BuildFailed,
		Title:      "Deploy Failed",
		DetailsUrl: "https://ci/run/1",
		StackName:  "payments",
		StackEnv:   "production",
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(captured).To(ContainSubstring("BUILD_FAILED"))
	Expect(captured).To(ContainSubstring("Deploy Failed"))
	Expect(captured).To(ContainSubstring("payments"))
	Expect(captured).To(ContainSubstring("production"))
	Expect(captured).To(ContainSubstring("❌")) // BuildFailed icon
}

func TestSend_WithCommitAuthorAndMessage(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	alert := api.Alert{
		AlertType:     api.BuildStarted,
		Title:         "t",
		CommitAuthor:  "alice",
		CommitMessage: "fix the thing",
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(captured).To(ContainSubstring("Author: alice"))
	Expect(captured).To(ContainSubstring("fix the thing"))
	Expect(captured).To(ContainSubstring(`•`)) // bullet separator present
}

func TestSend_LongCommitMessage_Truncated(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	alert := api.Alert{
		AlertType:     api.AlertResolved,
		Title:         "t",
		CommitMessage: strings.Repeat("z", 200),
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(captured).To(ContainSubstring(strings.Repeat("z", 97) + "..."))
	Expect(captured).ToNot(ContainSubstring(strings.Repeat("z", 101)))
}

func TestSend_ShortDescription_Included(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	Expect(sender.Send(api.Alert{
		AlertType:   api.AlertTriggered,
		Title:       "t",
		Description: "all clear now",
	})).To(Succeed())
	Expect(captured).To(ContainSubstring("all clear now"))
}

func TestSend_OverLimit_TruncatesWithIndicator(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	// A description far exceeding the 1900-char Discord limit drives the
	// truncation branch: intelligent begin+end trim + the truncation banner.
	desc := "DESC-START\n" + strings.Repeat("q", 6000) + "\nDESC-END"
	alert := api.Alert{
		AlertType:   api.BuildFailed,
		Title:       "Big Failure",
		DetailsUrl:  "https://ci/run/2",
		StackName:   "svc",
		StackEnv:    "prod",
		Description: desc,
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(captured).To(ContainSubstring("Error details truncated"))
	// The posted content stays within Discord's limit (JSON-escaped, so assert
	// the original characters are well under the byte budget by checking the
	// truncation indicator is present and the raw 6000-q run is gone).
	Expect(captured).ToNot(ContainSubstring(strings.Repeat("q", 6000)))
}

func TestSend_OverLimit_NoDescription_EssentialsOnly(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	// A huge title (no Description) makes the BASE message alone exceed the
	// 1900 limit. availableSpace goes deeply negative so the
	// `availableSpace > 50 && Description != ""` guard is false -> else branch
	// (baseMessage + indicator), then the FINAL safety check trims the whole
	// thing to maxDiscordMessageLength-3 + "...". This covers both the else
	// branch and the final-trim branch.
	alert := api.Alert{
		AlertType:  api.BuildFailed,
		Title:      strings.Repeat("T", 2100),
		DetailsUrl: "https://ci/run/3",
		StackName:  "svc",
		StackEnv:   "prod",
	}

	Expect(sender.Send(alert)).To(Succeed())
	// The base message overflows the limit on its own, so the truncation
	// indicator is itself sliced away by the final safety trim; the content
	// value ends in the "..." ellipsis.
	Expect(captured).To(ContainSubstring(`...`))
	Expect(captured).ToNot(ContainSubstring(strings.Repeat("T", 2000)))
}

func TestSend_OverLimit_WithCommitInfo_RebuildsBaseMessage(t *testing.T) {
	RegisterTestingT(t)

	var captured string
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		return okResponse(r)
	})

	// Over-limit message WITH commit author + (long) commit message so the
	// truncation branch re-builds the base message including the commit lines
	// (discord_alert.go:65-78) and the bullet separator.
	desc := "DESC-START\n" + strings.Repeat("w", 6000) + "\nDESC-END"
	alert := api.Alert{
		AlertType:     api.BuildFailed,
		Title:         "Failure",
		DetailsUrl:    "https://ci/run/4",
		StackName:     "svc",
		StackEnv:      "prod",
		CommitAuthor:  "bob",
		CommitMessage: strings.Repeat("c", 200),
		Description:   desc,
	}

	Expect(sender.Send(alert)).To(Succeed())
	Expect(captured).To(ContainSubstring("Author: bob"))
	// Long commit truncated to 97 chars + "..." inside the rebuilt base.
	Expect(captured).To(ContainSubstring(strings.Repeat("c", 97) + "..."))
	Expect(captured).To(ContainSubstring("Error details truncated"))
}

func TestIntelligentTruncate_NewlineBoundaryTrims(t *testing.T) {
	RegisterTestingT(t)

	// Drive BOTH newline-trim branches (discord_alert.go:179-184). For
	// maxLength=900: separator=23 -> availableSpace=877, beginningLen=292,
	// endLen=585 (no clamps). The begin-trim fires only when the last newline
	// sits beyond beginningLen-100 (i.e. index > 192); the end-trim fires only
	// when the first newline in the end window is within bytes 1..99.
	//
	// total length 6000 -> end window starts at 6000-585 = 5415.
	buf := []byte(strings.Repeat("x", 6000))
	buf[250] = '\n'  // inside last 100 bytes of the 292-byte beginning window
	buf[5465] = '\n' // ~50 bytes into the 585-byte end window
	text := string(buf)

	got := intelligentTruncate(text, 900)
	Expect(got).To(ContainSubstring("[... truncated ...]"))
	Expect(len(got)).To(BeNumerically("<", len(text)))
	// Begin trimmed at index 250 -> result starts with 250 x's then separator.
	Expect(got).To(HavePrefix(strings.Repeat("x", 250) + "\n\n[... truncated ...]"))
}

func TestSend_RestError_Returned(t *testing.T) {
	RegisterTestingT(t)

	// A non-2xx response makes the disgo rest client surface an error, which
	// Send() propagates.
	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader(`{"message":"boom","code":0}`)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})

	err := sender.Send(api.Alert{AlertType: api.BuildFailed, Title: "t"})
	Expect(err).To(HaveOccurred())
}

func TestSend_TransportError_Returned(t *testing.T) {
	RegisterTestingT(t)

	sender := newSenderWithTransport(t, func(r *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})

	err := sender.Send(api.Alert{AlertType: api.BuildStarted, Title: "t"})
	Expect(err).To(HaveOccurred())
}

// TestNew_ProductionClientTargetsDiscord documents that the production New()
// constructs a client whose REST base URL is disgo's hard-coded
// "https://discord.com/api/" — it cannot be redirected at a test server. Send()
// against a real client is therefore network-bound; the formatting/truncation
// branches are covered above via an injected transport.
func TestNew_ProductionClientTargetsDiscord(t *testing.T) {
	RegisterTestingT(t)

	sender, err := New("https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz_-0123456789")
	Expect(err).ToNot(HaveOccurred())
	as, ok := sender.(*alertSender)
	Expect(ok).To(BeTrue())
	Expect(as.client).ToNot(BeNil())
	Expect(as.client.URL()).To(ContainSubstring("discord.com"))
}
