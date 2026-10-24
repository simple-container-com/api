// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package evolution is a hand-written client for the Cloud.ru **Evolution**
// public REST API — the subset SC needs to run a container: IAM tokens, Artifact
// Registry, and Container Apps.
//
// Why hand-written rather than bridged. Cloud.ru's Terraform provider
// (cloud-ru/evo-terraform) can be turned into a Pulumi SDK — it ships as a binary
// and Pulumi's *parameterized* bridge consumes binaries — but Container Apps
// appear nowhere in it: no resource, no data source, not even an entry in its
// endpoint map. The 61 resources it does expose are VM, mk8s, Artifact Registry,
// managed databases, object storage, DNS, load balancing and IAM. So REST is not
// a stopgap pending a bridge; it is the only route to a serverless container on
// this cloud, and it has the side benefit of depending on neither get.pulumi.com
// nor a hosted provider binary.
//
// Everything in here was measured against the operator's own project on
// 2026-10-22, not read off documentation; see
// forge/docs/design/2026-10-13/sc-russia-market-readiness/ (amendment
// «2026-10-22 (вторая)») for the session that produced it.
package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	// DefaultIAMEndpoint issues the bearer token every other endpoint wants.
	DefaultIAMEndpoint = "https://iam.api.cloud.ru"
	// DefaultContainersEndpoint serves the Container Apps API (OpenAPI 3.0, v2).
	DefaultContainersEndpoint = "https://containers.api.cloud.ru"
	// DefaultRegistryEndpoint serves the Artifact Registry API (Swagger 2.0, v1).
	DefaultRegistryEndpoint = "https://ar.api.cloud.ru"
	// DefaultComputeEndpoint serves the Evolution Compute API (VMs, disks,
	// floating IPs, subnets, security groups). Note the project scope on this one
	// is a **snake_case** `project_id` query parameter, unlike the camelCase
	// `projectId` the Containers and Registry APIs want — see computeQuery.
	DefaultComputeEndpoint = "https://compute.api.cloud.ru"

	// RegistryHostSuffix completes a registry's docker host: a registry named
	// `foo` is pushed to and pulled from `foo.cr.cloud.ru`.
	RegistryHostSuffix = "cr.cloud.ru"

	// PublicDomainSuffix is the platform domain a publicly-accessible app is
	// published under. The whole zone is covered by a wildcard certificate
	// (GlobalSign, `O = CLOUD TECHNOLOGIES LLC`), which is why nothing on this
	// path needs ACME — and therefore why "does Let's Encrypt reach from inside
	// RU" is not a question this driver has to answer.
	PublicDomainSuffix = "containerapps.ru"

	// tokenLifetime is how long an IAM token lasts. Tokens are refreshed at
	// tokenRefreshSlack before expiry so a long poll cannot straddle the boundary.
	tokenLifetime     = time.Hour
	tokenRefreshSlack = 5 * time.Minute
)

// Credentials are an Evolution IAM service-account key pair plus the project the
// calls are scoped to.
//
// ProjectID is not optional and not inferable: every endpoint here is
// project-scoped, and a call without it answers 403 `non_authorized` —
// indistinguishable from a missing role, which is how an hour gets spent
// granting permissions that were never the problem.
// The field names are the wire names of the `sc cloudru` stdin document (see
// Request), which is why a struct that is otherwise internal carries JSON tags.
type Credentials struct {
	KeyID     string `json:"keyId"`
	Secret    string `json:"keySecret"`
	ProjectID string `json:"projectId"`
}

// Client talks to one Cloud.ru project. Safe for concurrent use; the token is
// shared and refreshed under a mutex.
type Client struct {
	creds Credentials

	iamURL        string
	containersURL string
	registryURL   string
	computeURL    string
	dnsURL        string

	httpClient *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// Option overrides a Client default. Only the endpoints and the HTTP client are
// overridable, and only so the tests can point at an httptest server.
type Option func(*Client)

func WithIAMEndpoint(u string) Option {
	return func(c *Client) { c.iamURL = strings.TrimSuffix(u, "/") }
}
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

func WithContainersEndpoint(u string) Option {
	return func(c *Client) { c.containersURL = strings.TrimSuffix(u, "/") }
}

func WithRegistryEndpoint(u string) Option {
	return func(c *Client) { c.registryURL = strings.TrimSuffix(u, "/") }
}

func WithComputeEndpoint(u string) Option {
	return func(c *Client) { c.computeURL = strings.TrimSuffix(u, "/") }
}

func WithDnsEndpoint(u string) Option {
	return func(c *Client) { c.dnsURL = strings.TrimSuffix(u, "/") }
}

// WithAllEndpoints points IAM, Containers, Artifact Registry, Compute and Cloud
// DNS at one base URL. Only useful for tests, where a single httptest server
// serves all.
func WithAllEndpoints(u string) Option {
	return func(c *Client) {
		u = strings.TrimSuffix(u, "/")
		c.iamURL, c.containersURL, c.registryURL, c.computeURL, c.dnsURL = u, u, u, u, u
	}
}

func New(creds Credentials, opts ...Option) *Client {
	c := &Client{
		creds:         creds,
		iamURL:        DefaultIAMEndpoint,
		containersURL: DefaultContainersEndpoint,
		registryURL:   DefaultRegistryEndpoint,
		computeURL:    DefaultComputeEndpoint,
		dnsURL:        DefaultDnsEndpoint,
		// Generous but bounded: an image-less POST answers in well under a second,
		// but the API has been seen to sit on a request for tens of seconds.
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Validate refuses a credential set that cannot authenticate, naming the field.
func (c Credentials) Validate() error {
	if strings.TrimSpace(c.KeyID) == "" {
		return errors.Errorf("`keyId` is required")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return errors.Errorf("`keySecret` is required")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return errors.Errorf("`projectId` is required: every Cloud.ru Evolution endpoint is project-scoped and answers 403 `non_authorized` without it")
	}
	return nil
}

// APIError is a non-2xx response. The body is kept because Cloud.ru puts the only
// useful part of a failure there (`error_msg`, or a google.rpc.Status), while the
// status code alone is routinely misleading: a wrong-shaped identifier in a path
// comes back 400 or 499, not 404.
type APIError struct {
	Method string
	URL    string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.URL, e.Status, strings.TrimSpace(e.Body))
}

// NotFound reports whether err is the API saying the object does not exist.
//
// Both spellings are accepted because the Container Apps API uses both: a GET of
// an absent app is 404, while a *name* handed to an id-shaped path segment comes
// back 499 `Serverless with name is not found`.
func NotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Status == http.StatusNotFound {
		return true
	}
	return strings.Contains(strings.ToLower(apiErr.Body), "not found")
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// bearer returns a valid access token, minting one if needed.
func (c *Client) bearer(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-tokenRefreshSlack)) {
		return c.token, nil
	}

	body, err := json.Marshal(map[string]string{"keyId": c.creds.KeyID, "secret": c.creds.Secret})
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal cloud.ru IAM token request")
	}
	endpoint := c.iamURL + "/api/v1/auth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.Wrapf(err, "failed to build cloud.ru IAM token request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to call cloud.ru IAM at %s", endpoint)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read cloud.ru IAM response")
	}
	if resp.StatusCode/100 != 2 {
		// The request body carried the key secret, so the error deliberately quotes
		// neither it nor the request — only the response.
		return "", errors.Wrapf(&APIError{Method: http.MethodPost, URL: endpoint, Status: resp.StatusCode, Body: string(raw)},
			"cloud.ru IAM rejected the key pair")
	}
	var out tokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.Wrapf(err, "failed to decode cloud.ru IAM response")
	}
	if out.AccessToken == "" {
		return "", errors.Errorf("cloud.ru IAM returned no access_token")
	}

	lifetime := tokenLifetime
	if out.ExpiresIn > 0 {
		lifetime = time.Duration(out.ExpiresIn) * time.Second
	}
	c.token = out.AccessToken
	c.tokenExp = time.Now().Add(lifetime)
	return c.token, nil
}

// request is one authenticated call. `query` is applied as-is; `out` may be nil.
func (c *Client) request(ctx context.Context, method, base, path string, query url.Values, body, out any) error {
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}

	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal %s %s request", method, path)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return errors.Wrapf(err, "failed to build %s %s request", method, path)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to call %s %s", method, endpoint)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s %s response", method, endpoint)
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Method: method, URL: endpoint, Status: resp.StatusCode, Body: string(raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.Wrapf(err, "failed to decode %s %s response", method, endpoint)
	}
	return nil
}

// projectQuery is the project scope as a QUERY parameter, which is what GET and
// DELETE want. POST and PATCH take `projectId` in the BODY instead — the one
// asymmetry in this API that cannot be factored away, so it is spelled out at
// each call site rather than hidden here.
func (c *Client) projectQuery() url.Values {
	return url.Values{"projectId": []string{c.creds.ProjectID}}
}

// computeQuery is the project scope the **Compute** API wants: `project_id`,
// snake_case, as a query parameter on every verb including POST (which ALSO
// wants `project_id` inside the body). This is not a style difference that can
// be normalised away — the Compute API is a FastAPI service and rejects an
// unexpected query key outright (422 `extra_forbidden`), so handing it
// `projectId` is a hard failure, not a tolerated alias.
func (c *Client) computeQuery() url.Values {
	return url.Values{"project_id": []string{c.creds.ProjectID}}
}
