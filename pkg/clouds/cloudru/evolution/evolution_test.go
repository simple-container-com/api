// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

// fakeAPI serves IAM, Artifact Registry and Container Apps from one httptest
// server, which is what WithAllEndpoints exists for.
//
// It is deliberately picky about the two things that are easy to get wrong against
// the real API and impossible to notice against a permissive fake: the bearer token
// on every call but the token mint, and the project scope — which rides the QUERY
// on GET/DELETE and the BODY on POST/PATCH. A call that forgets either answers 403
// `non_authorized` on the real thing, indistinguishable from a missing IAM role.
type fakeAPI struct {
	mu sync.Mutex

	projectID string

	registries []Registry
	app        *App
	revisions  []Revision
	logs       []LogLine

	tokensMinted int
	creates      int
	patches      int
	deletes      int

	// onRevisions is called before each revision list, so a test can flip a status
	// mid-poll the way the platform does.
	onRevisions func(f *fakeAPI)
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]string
		Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
		// The wire names are `keyId`/`secret`, not `keySecret` — that spelling is the
		// stdin document's, not IAM's.
		Expect(body["keyId"]).NotTo(BeEmpty())
		Expect(body["secret"]).NotTo(BeEmpty())
		f.tokensMinted++
		writeJSON(w, tokenResponse{AccessToken: "token", ExpiresIn: 3600, TokenType: "Bearer"})
	})

	mux.HandleFunc("/v1/registries", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
			writeJSON(w, listRegistriesResponse{Registries: f.registries})
		case http.MethodPost:
			var req createRegistryRequest
			Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			// Project scope in the BODY here, not the query.
			Expect(req.ProjectID).To(Equal(f.projectID))
			Expect(req.RegistryType).To(Equal(RegistryTypeDocker))
			f.registries = append(f.registries, Registry{
				ID: "reg-1", Name: req.Name, Status: RegistryStatusActive, RegistryType: RegistryTypeDocker,
			})
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s /v1/registries", r.Method)
		}
	})

	mux.HandleFunc("/v2/containers", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		f.mu.Lock()
		defer f.mu.Unlock()
		Expect(r.Method).To(Equal(http.MethodPost))
		var spec AppSpec
		Expect(json.NewDecoder(r.Body).Decode(&spec)).To(Succeed())
		Expect(spec.ProjectID).To(Equal(f.projectID))
		f.creates++
		f.app = &App{ID: "app-1", Name: spec.Name, Status: "publishing", Configuration: spec.Configuration, Template: spec.Template}
		if f.app.Configuration != nil && f.app.Configuration.Ingress != nil {
			f.app.Configuration.Ingress.PublicURI = spec.Name + "." + PublicDomainSuffix
		}
		writeJSON(w, Operation{ID: "op-1", ResourceID: "app-1", Done: true})
	})

	mux.HandleFunc("/v2/containers/", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		f.mu.Lock()
		defer f.mu.Unlock()

		switch {
		case hasSuffix(r.URL.Path, "/revisions"):
			Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
			if f.onRevisions != nil {
				f.onRevisions(f)
			}
			writeJSON(w, revisionListResponse{Data: f.revisions})
		case hasSuffix(r.URL.Path, "/logs"):
			Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
			writeJSON(w, logListResponse{Data: f.logs})
		default:
			switch r.Method {
			case http.MethodGet:
				Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
				if f.app == nil {
					http.Error(w, `{"error_msg":"not found"}`, http.StatusNotFound)
					return
				}
				writeJSON(w, f.app)
			case http.MethodPatch:
				var spec AppSpec
				Expect(json.NewDecoder(r.Body).Decode(&spec)).To(Succeed())
				Expect(spec.ProjectID).To(Equal(f.projectID))
				// The patch schema has no `privileged` field at all; the client strips
				// it, and this asserts the strip rather than trusting it.
				if spec.Configuration != nil {
					Expect(spec.Configuration.Privileged).To(BeNil())
				}
				f.patches++
				writeJSON(w, Operation{ID: "op-2", ResourceID: "app-1", Done: true})
			case http.MethodDelete:
				Expect(r.URL.Query().Get("projectId")).To(Equal(f.projectID))
				f.deletes++
				if f.app == nil {
					// How the real API answers a name that no longer exists: 499 with
					// "not found" in the body, not a 404.
					http.Error(w, `{"error_msg":"Serverless with name is not found"}`, 499)
					return
				}
				f.app = nil
				w.WriteHeader(http.StatusOK)
			default:
				t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
			}
		}
	})

	return mux
}

func (f *fakeAPI) requireBearer(r *http.Request) {
	Expect(r.Header.Get("Authorization")).To(Equal("Bearer token"))
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newTestClient(t *testing.T, f *fakeAPI) *Client {
	f.projectID = "project-1"
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return New(Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: f.projectID}, WithAllEndpoints(srv.URL))
}

func TestCredentialsValidate(t *testing.T) {
	RegisterTestingT(t)

	Expect(Credentials{Secret: "s", ProjectID: "p"}.Validate()).NotTo(Succeed())
	Expect(Credentials{KeyID: "k", ProjectID: "p"}.Validate()).NotTo(Succeed())
	// The one worth a named assertion: an omitted project is a 403 that looks
	// exactly like a missing role.
	err := Credentials{KeyID: "k", Secret: "s"}.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("projectId"))
	Expect(Credentials{KeyID: "k", Secret: "s", ProjectID: "p"}.Validate()).To(Succeed())
}

// One token for many calls. The real cost of getting this wrong is an IAM mint per
// poll during a readiness wait.
func TestTokenIsReused(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{}
	c := newTestClient(t, f)
	for range 3 {
		_, err := c.ListRegistries(context.Background())
		Expect(err).To(BeNil())
	}
	Expect(f.tokensMinted).To(Equal(1))
}

func TestEnsureRegistryCreatesThenReuses(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{}
	c := newTestClient(t, f)

	reg, err := c.EnsureRegistry(context.Background(), "sc-smoke", time.Minute)
	Expect(err).To(BeNil())
	Expect(reg.Name).To(Equal("sc-smoke"))
	Expect(reg.Host()).To(Equal("sc-smoke.cr.cloud.ru"))
	Expect(f.registries).To(HaveLen(1))

	// Idempotent: the API has no create-if-absent, so the second deploy must find
	// the registry by name rather than create a duplicate (or fail).
	_, err = c.EnsureRegistry(context.Background(), "sc-smoke", time.Minute)
	Expect(err).To(BeNil())
	Expect(f.registries).To(HaveLen(1))
}

func TestEnsureRegistryRefusesErrorStatus(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{registries: []Registry{{Name: "broken", Status: RegistryStatusError}}}
	c := newTestClient(t, f)
	_, err := c.EnsureRegistry(context.Background(), "broken", time.Minute)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(RegistryStatusError))
}

func TestRevisionStatusClassification(t *testing.T) {
	RegisterTestingT(t)

	Expect(RevisionHealthy("ACTIVE")).To(BeTrue())
	Expect(RevisionHealthy("  running ")).To(BeTrue())
	Expect(RevisionHealthy("publishing")).To(BeFalse())
	// Substring matching, not an enumeration: the API documents no status
	// vocabulary, and the failure mode being avoided is burning a ten-minute
	// timeout on a revision that already said it failed.
	Expect(RevisionFailed("CRASH_LOOP_BACKOFF")).To(BeTrue())
	Expect(RevisionFailed("ImagePullError")).To(BeTrue())
	Expect(RevisionFailed("CANCELLED")).To(BeTrue())
	Expect(RevisionFailed("ACTIVE")).To(BeFalse())
	Expect(RevisionFailed("publishing")).To(BeFalse())
}

// Newest first, by reverse lexicographic order of the zero-padded ordinal — not by
// a timestamp whose format the API does not declare.
func TestRevisionsNewestFirst(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{revisions: []Revision{
		{Name: "app-00002", Status: "ACTIVE"},
		{Name: "app-00010", Status: "publishing"},
		{Name: "app-00001", Status: "ACTIVE"},
	}}
	c := newTestClient(t, f)
	revisions, err := c.Revisions(context.Background(), "app")
	Expect(err).To(BeNil())
	Expect(revisions[0].Name).To(Equal("app-00010"))
	Expect(revisions[2].Name).To(Equal("app-00001"))
}

func TestWaitReadyBecomesHealthy(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{
		app:       &App{ID: "app-1", Name: "atrium", Status: "publishing"},
		revisions: []Revision{{Name: "atrium-00001", Status: "publishing"}},
	}
	polls := 0
	f.onRevisions = func(f *fakeAPI) {
		polls++
		if polls >= 2 {
			f.revisions = []Revision{{Name: "atrium-00001", Status: "ACTIVE"}}
		}
	}
	c := newTestClient(t, f)

	app, err := c.WaitReady(context.Background(), "atrium", time.Minute, time.Millisecond)
	Expect(err).To(BeNil())
	Expect(app.Name).To(Equal("atrium"))
}

// The error has to carry the container's own output, because that is the only place
// the cause appears — the operation says `done: true`, the app's status says
// `publishing`, and the URL 404s.
func TestWaitReadyFoldsLogsIntoFailure(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{
		app:       &App{ID: "app-1", Name: "atrium"},
		revisions: []Revision{{Name: "atrium-00001", Status: "CRASH_LOOP"}},
		logs: []LogLine{
			{Message: `mkdir("/var/cache/nginx/client_temp") failed (13: Permission denied)`},
		},
	}
	c := newTestClient(t, f)

	_, err := c.WaitReady(context.Background(), "atrium", time.Minute, time.Millisecond)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("CRASH_LOOP"))
	Expect(err.Error()).To(ContainSubstring("Permission denied"))
}

func TestWaitReadyTimesOutWithLogs(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{
		revisions: []Revision{{Name: "atrium-00001", Status: "publishing"}},
		logs:      []LogLine{{Message: "still starting"}},
	}
	c := newTestClient(t, f)

	_, err := c.WaitReady(context.Background(), "atrium", 5*time.Millisecond, time.Millisecond)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("did not become ready"))
	// Not just the timeout: the last status and the output, so the CI log holds the
	// diagnosis instead of a second API call to make.
	Expect(err.Error()).To(ContainSubstring("publishing"))
	Expect(err.Error()).To(ContainSubstring("still starting"))
}

// A timeout against an app with no revisions at all is the YC shape: green deploy,
// zero revisions, nothing serving. The message has to say so.
func TestWaitReadyNoRevisionsSaysImagePull(t *testing.T) {
	RegisterTestingT(t)

	c := newTestClient(t, &fakeAPI{})
	_, err := c.WaitReady(context.Background(), "atrium", 5*time.Millisecond, time.Millisecond)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no container logs yet"))
}

func TestApplyAppCreatesThenPatches(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{revisions: []Revision{{Name: "atrium-00001", Status: "ACTIVE"}}}
	c := newTestClient(t, f)
	req := Request{
		Credentials:  Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: "project-1"},
		Name:         "atrium",
		PollInterval: "1ms",
		Spec: &AppSpec{
			Name:          "atrium",
			Configuration: &AppConfig{Ingress: &AppIngress{PubliclyAccessible: lo.ToPtr(true)}},
			Template: &AppTemplate{
				Containers: []AppContainer{{Name: "atrium", Image: "r.cr.cloud.ru/atrium:1", ContainerPort: 8080}},
			},
		},
	}

	res, err := c.ApplyApp(context.Background(), req)
	Expect(err).To(BeNil())
	Expect(f.creates).To(Equal(1))
	Expect(f.patches).To(Equal(0))
	Expect(res.URL).To(Equal("https://atrium." + PublicDomainSuffix))
	Expect(res.Revision).To(Equal("atrium-00001"))

	// Second apply patches rather than recreating — which is what keeps 00001
	// serving until 00002 is active, and what makes a redeploy not take the URL down.
	res, err = c.ApplyApp(context.Background(), req)
	Expect(err).To(BeNil())
	Expect(f.creates).To(Equal(1))
	Expect(f.patches).To(Equal(1))
	Expect(res.ID).To(Equal("app-1"))
}

func TestApplyAppRejectsNameDisagreement(t *testing.T) {
	RegisterTestingT(t)

	c := newTestClient(t, &fakeAPI{})
	_, err := c.ApplyApp(context.Background(), Request{
		Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"},
		Name:        "atrium",
		Spec:        &AppSpec{Name: "something-else"},
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("disagree"))
}

func TestApplyAppRequiresSpec(t *testing.T) {
	RegisterTestingT(t)

	c := newTestClient(t, &fakeAPI{})
	_, err := c.ApplyApp(context.Background(), Request{
		Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"},
		Name:        "atrium",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`spec` is required"))
}

// The API would accept this patch, publish a revision, and leave the setting
// exactly as it was — a green deploy over an unchanged runtime.
func TestPrivilegedDriftIsRefused(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{
		app: &App{
			ID: "app-1", Name: "atrium",
			Configuration: &AppConfig{Privileged: lo.ToPtr(false), Ingress: &AppIngress{}},
		},
		revisions: []Revision{{Name: "atrium-00001", Status: "ACTIVE"}},
	}
	c := newTestClient(t, f)

	_, err := c.ApplyApp(context.Background(), Request{
		Credentials:  Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"},
		Name:         "atrium",
		PollInterval: "1ms",
		Spec: &AppSpec{
			Name:          "atrium",
			Configuration: &AppConfig{Privileged: lo.ToPtr(true)},
		},
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("privileged"))
	Expect(err.Error()).To(ContainSubstring("sc destroy"))
	Expect(f.patches).To(Equal(0))
}

// Equal values must NOT trip the check, including the nil-vs-false spelling the API
// returns for an app that was created without the field.
func TestPrivilegedDriftAllowsUnchanged(t *testing.T) {
	RegisterTestingT(t)

	Expect(checkPrivilegedDrift(&App{Name: "a"}, &AppSpec{})).To(Succeed())
	Expect(checkPrivilegedDrift(&App{Name: "a", Configuration: &AppConfig{}}, &AppSpec{Configuration: &AppConfig{Privileged: lo.ToPtr(false)}})).To(Succeed())
	Expect(checkPrivilegedDrift(
		&App{Name: "a", Configuration: &AppConfig{Privileged: lo.ToPtr(true)}},
		&AppSpec{Configuration: &AppConfig{Privileged: lo.ToPtr(true)}},
	)).To(Succeed())
}

// A destroy that runs twice, or after a create that failed halfway, must converge
// — and `pulumi destroy` does exactly that after a failed run.
func TestDestroyAppIsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	f := &fakeAPI{app: &App{ID: "app-1", Name: "atrium"}}
	c := newTestClient(t, f)
	req := Request{Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"}, Name: "atrium"}

	_, err := c.DestroyApp(context.Background(), req)
	Expect(err).To(BeNil())
	_, err = c.DestroyApp(context.Background(), req)
	Expect(err).To(BeNil())
	Expect(f.deletes).To(Equal(2))
}

// Both spellings, because the Container Apps API uses both: 404 for an absent app,
// and 499 with "not found" in the body for a name handed to an id-shaped path.
func TestNotFoundAcceptsBothSpellings(t *testing.T) {
	RegisterTestingT(t)

	Expect(NotFound(&APIError{Status: http.StatusNotFound})).To(BeTrue())
	Expect(NotFound(&APIError{Status: 499, Body: `{"error_msg":"Serverless with name is not found"}`})).To(BeTrue())
	Expect(NotFound(&APIError{Status: http.StatusForbidden, Body: `non_authorized`})).To(BeFalse())
	Expect(NotFound(fmt.Errorf("plain error"))).To(BeFalse())
}

func TestRequestDurationDefaults(t *testing.T) {
	RegisterTestingT(t)

	timeout, err := Request{}.waitTimeout()
	Expect(err).To(BeNil())
	Expect(timeout).To(Equal(DefaultWaitTimeout))

	interval, err := Request{PollInterval: "2s"}.pollInterval()
	Expect(err).To(BeNil())
	Expect(interval).To(Equal(2 * time.Second))

	_, err = Request{WaitTimeout: "soon"}.waitTimeout()
	Expect(err).To(HaveOccurred())
	_, err = Request{WaitTimeout: "-1s"}.waitTimeout()
	Expect(err).To(HaveOccurred())
}

// The stdin document's wire names are a contract between the provisioning tier and
// the CLI, and the two halves ship in the same binary — so a rename would pass
// every compile and fail only on a runner whose `sc` is a different build.
func TestRequestWireNames(t *testing.T) {
	RegisterTestingT(t)

	encoded, err := json.Marshal(Request{
		Credentials:  Credentials{KeyID: "k", Secret: "s", ProjectID: "p"},
		Name:         "atrium",
		RegistryName: "sc-smoke",
	})
	Expect(err).To(BeNil())
	Expect(string(encoded)).To(ContainSubstring(`"keyId":"k"`))
	Expect(string(encoded)).To(ContainSubstring(`"keySecret":"s"`))
	Expect(string(encoded)).To(ContainSubstring(`"projectId":"p"`))
	Expect(string(encoded)).To(ContainSubstring(`"registryName":"sc-smoke"`))
}
