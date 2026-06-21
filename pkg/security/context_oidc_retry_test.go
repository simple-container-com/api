// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

type oidcResp struct {
	status int
	body   string
	delay  time.Duration
}

func noSleep(context.Context, time.Duration) error { return nil }

func fastPolicy(attempts int) oidcRetryPolicy {
	return oidcRetryPolicy{
		Attempts:          attempts,
		PerAttemptTimeout: 2 * time.Second,
		BaseBackoff:       time.Millisecond,
		MaxBackoff:        time.Millisecond,
	}
}

func oidcServer(t *testing.T, responses []oidcResp) (*httptest.Server, *int32, chan string) {
	t.Helper()
	var hits int32
	urls := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls <- r.URL.String()
		i := int(atomic.AddInt32(&hits, 1)) - 1
		resp := responses[len(responses)-1]
		if i < len(responses) {
			resp = responses[i]
		}
		if resp.delay > 0 {
			time.Sleep(resp.delay)
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, urls
}

func TestRequestOIDCTokenSuccessFirstTry(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, urls := oidcServer(t, []oidcResp{{status: 200, body: `{"value":"tok"}`}})

	token, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).ToNot(HaveOccurred())
	Expect(token).To(Equal("tok"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
	Expect(<-urls).To(ContainSubstring("audience=sigstore"))
}

func TestRequestOIDCTokenRetriesOn5xxThenSucceeds(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 500}, {status: 502}, {status: 200, body: `{"value":"tok"}`}})

	token, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).ToNot(HaveOccurred())
	Expect(token).To(Equal("tok"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(3)))
}

func TestRequestOIDCTokenRetriesOn429(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 429}, {status: 200, body: `{"value":"tok"}`}})

	token, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).ToNot(HaveOccurred())
	Expect(token).To(Equal("tok"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(2)))
}

func TestRequestOIDCTokenFailsFastOn401(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 401, body: "bad credentials"}, {status: 200, body: `{"value":"tok"}`}})

	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("401"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
}

func TestRequestOIDCTokenExhaustsOnPersistent503(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 503}})

	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("after 4 attempts"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(4)))
}

func TestRequestOIDCTokenNoRetryOnMalformed200(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 200, body: "not-json"}})

	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("parsing OIDC token response"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
}

func TestRequestOIDCTokenNoRetryOnEmptyValue(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 200, body: `{"value":""}`}})

	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("empty value"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
}

func TestRequestOIDCTokenAbortsOnParentCancelDuringBackoff(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 500}, {status: 200, body: `{"value":"tok"}`}})

	cancelSleep := func(context.Context, time.Duration) error { return context.Canceled }
	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", fastPolicy(4), cancelSleep)
	Expect(err).To(MatchError(context.Canceled))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
}

func TestRequestOIDCTokenAbortsWhenParentContextDone(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 200, body: `{"value":"tok"}`}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := requestOIDCTokenWithRetry(ctx, srv.URL, "req-token", fastPolicy(4), noSleep)
	Expect(err).To(MatchError(context.Canceled))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(0)))
}

func TestRequestOIDCTokenRetriesOnPerAttemptTimeout(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 200, body: `{"value":"tok"}`, delay: 200 * time.Millisecond}})
	policy := oidcRetryPolicy{Attempts: 2, PerAttemptTimeout: 20 * time.Millisecond, BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond}

	_, err := requestOIDCTokenWithRetry(context.Background(), srv.URL, "req-token", policy, noSleep)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("after 2 attempts"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(2)))
}

func TestRetryableOIDCStatus(t *testing.T) {
	RegisterTestingT(t)
	for status, want := range map[int]bool{
		408: true, 429: true, 500: true, 503: true, 599: true,
		200: false, 400: false, 401: false, 403: false, 404: false,
	} {
		Expect(retryableOIDCStatus(status)).To(Equal(want), "status %d", status)
	}
}

func TestOIDCTokenURLAddsAudienceAndPreservesParams(t *testing.T) {
	RegisterTestingT(t)
	got, err := oidcTokenURL("https://token.actions.example.com/req?api-version=2.0")
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(ContainSubstring("audience=sigstore"))
	Expect(got).To(ContainSubstring("api-version=2.0"))

	_, err = oidcTokenURL("http://example.com/%zz")
	Expect(err).To(HaveOccurred())
}

func TestGetOIDCTokenGitHubActionsHappyPath(t *testing.T) {
	RegisterTestingT(t)
	srv, hits, _ := oidcServer(t, []oidcResp{{status: 200, body: `{"value":"ci-tok"}`}})
	clearCIEnv(t)
	t.Setenv("SIGSTORE_ID_TOKEN", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "req-token")

	e := &ExecutionContext{}
	e.DetectCI()
	Expect(e.GetOIDCToken(context.Background())).ToNot(HaveOccurred())
	Expect(e.OIDCToken).To(Equal("ci-tok"))
	Expect(atomic.LoadInt32(hits)).To(Equal(int32(1)))
}
