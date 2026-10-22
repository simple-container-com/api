// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Revision statuses. Only `active` has been observed on a healthy revision; the
// API's own schema documents `running` as the example, so both are accepted
// rather than pinning the one value this cloud happened to return.
const (
	RevisionStatusActive  = "active"
	RevisionStatusRunning = "running"
	RevisionStatusReady   = "ready"
)

// ProtocolHTTP1 is the only protocol an SC container needs; `http_2` and `grpc`
// exist but neither is reachable from SC's config today.
const ProtocolHTTP1 = "http_1"

// ScalingRuleConcurrency scales on in-flight requests per instance, which is the
// only rule type the API documents.
const ScalingRuleConcurrency = "concurrency"

// MinContainerPort is the lowest port an **unprivileged** container may listen
// on. Below it the platform refuses the revision outright.
//
// This is one half of a trap with two halves and one cause: the runtime drops
// privileges unless `configuration.privileged` is set, so a container both
// cannot bind a low port and cannot write to root-owned paths in its own image.
// Plain `nginx` fails the second half with `mkdir("/var/cache/nginx/client_temp")
// failed (13: Permission denied)` — in the runtime, as a CrashLoop, long after a
// green deploy. See the driver's validation, which refuses this combination up
// front instead.
const MinContainerPort = 1024

// AppSpec is the desired state of one Container App. It is the request body of
// both create and patch, which differ only in whether `name` is in the path.
type AppSpec struct {
	// ProjectID is sent in the BODY for create and patch (and as a query
	// parameter everywhere else — see Client.projectQuery).
	ProjectID     string       `json:"projectId,omitempty"`
	Name          string       `json:"name,omitempty"`
	Description   string       `json:"description,omitempty"`
	Configuration *AppConfig   `json:"configuration,omitempty"`
	Template      *AppTemplate `json:"template,omitempty"`
}

// AppConfig is `configuration`: everything about the app that is not the
// workload itself.
type AppConfig struct {
	Ingress *AppIngress `json:"ingress,omitempty"`
	// Privileged runs the container as root. **Create-only**: the patch request
	// schema has no `privileged` field at all, so flipping it on an existing app
	// is not an update but a destroy-and-recreate. The driver checks for that
	// rather than silently patching around it.
	Privileged *bool `json:"privileged,omitempty"`
}

// AppIngress is `configuration.ingress`. PublicURI and InternalURI are read-only
// and only ever appear in a response.
type AppIngress struct {
	PubliclyAccessible *bool                 `json:"publiclyAccessible,omitempty"`
	CustomDomains      []CustomDomainBinding `json:"customDomains,omitempty"`

	PublicURI   string `json:"publicUri,omitempty"`
	InternalURI string `json:"internalUri,omitempty"`
}

// CustomDomainBinding attaches an operator-owned domain. CertificateID refers to
// a certificate already uploaded to Cloud.ru — this path has no ACME, because the
// platform domain is covered by a wildcard certificate the platform owns.
type CustomDomainBinding struct {
	Domain        string `json:"domain"`
	CertificateID string `json:"certificateId,omitempty"`
}

// AppTemplate is `template`: the workload and how it scales.
type AppTemplate struct {
	// Timeout and IdleTimeout are Go-style duration strings with a unit the API
	// parses itself ("600s", "60s").
	Timeout     string         `json:"timeout,omitempty"`
	IdleTimeout string         `json:"idleTimeout,omitempty"`
	Protocol    string         `json:"protocol,omitempty"`
	Scaling     *AppScaling    `json:"scaling,omitempty"`
	Containers  []AppContainer `json:"containers,omitempty"`
}

type AppScaling struct {
	MinInstanceCount int             `json:"minInstanceCount"`
	MaxInstanceCount int             `json:"maxInstanceCount"`
	Rule             *AppScalingRule `json:"rule,omitempty"`
}

// AppScalingRule mirrors ContainerScalingRule, whose `value` the schema declares
// as a string carrying integer properties — in practice a JSON object
// (`{"soft":100,"hard":0}`), which is how the API returns it.
type AppScalingRule struct {
	Type  string         `json:"type,omitempty"`
	Value map[string]int `json:"value,omitempty"`
}

type AppContainer struct {
	// Name is checked against the same regex as the app itself — a container
	// called `app` is a 400, because the minimum length is 5.
	Name string `json:"name"`
	// Image must resolve inside this project's Artifact Registry. A digest
	// reference is preferred to a tag for the same reason ECS prefers one: a tag
	// can be moved after the image was built.
	Image         string        `json:"image"`
	Resources     *AppResources `json:"resources,omitempty"`
	ContainerPort int           `json:"containerPort,omitempty"`
	Env           []AppEnv      `json:"env,omitempty"`
	Command       []string      `json:"command,omitempty"`
	Args          []string      `json:"args,omitempty"`
}

// AppResources are Kubernetes-shaped request strings ("0.1", "256Mi").
type AppResources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// AppEnv is one environment variable. Type `secret` makes Value the *name* of a
// secret in Cloud.ru Secret Management rather than the value itself; SC does not
// use that yet, so Type stays empty and values are inlined.
type AppEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

// App is the API's view of a Container App.
type App struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Status is NOT a readiness signal. It read `publishing` for four minutes
	// while the pod CrashLooped, with `updatedAt` frozen at creation time, the
	// create operation already `done: true`, and the public URL serving 404 —
	// none of the three reporting an error. Readiness comes from Revisions.
	Status        string       `json:"status"`
	Configuration *AppConfig   `json:"configuration"`
	Template      *AppTemplate `json:"template"`
	CreatedAt     string       `json:"createdAt"`
	UpdatedAt     string       `json:"updatedAt"`
}

// PublicURL is the HTTPS URL the platform publishes the app on, or "" when the
// app is not publicly accessible.
func (a *App) PublicURL() string {
	if a == nil || a.Configuration == nil || a.Configuration.Ingress == nil {
		return ""
	}
	host := a.Configuration.Ingress.PublicURI
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		return host
	}
	return "https://" + host
}

// Operation is an AsyncOperation: what create, patch and delete return.
type Operation struct {
	ID         string `json:"id"`
	ResourceID string `json:"resourceId"`
	Done       bool   `json:"done"`
	Error      *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Revision is one published version of an app, from RevisionResponse.
type Revision struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	// StatusReason is only populated by the single-revision GET, not by the list.
	StatusReason string `json:"statusReason,omitempty"`
}

type revisionListResponse struct {
	Data []Revision `json:"data"`
}

// LogLine is one line of container output.
type LogLine struct {
	Timestamp     string `json:"timestamp"`
	Message       string `json:"message"`
	Level         string `json:"level"`
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
}

type logListResponse struct {
	Data []LogLine `json:"data"`
}

// GetApp reads one app by name.
func (c *Client) GetApp(ctx context.Context, name string) (*App, error) {
	var out App
	if err := c.request(ctx, http.MethodGet, c.containersURL,
		"/v2/containers/"+url.PathEscape(name), c.projectQuery(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindApp reads one app by name, reporting absence rather than erroring.
func (c *Client) FindApp(ctx context.Context, name string) (*App, bool, error) {
	app, err := c.GetApp(ctx, name)
	if err != nil {
		if NotFound(err) {
			return nil, false, nil
		}
		return nil, false, errors.Wrapf(err, "failed to read cloud.ru container app %q", name)
	}
	return app, true, nil
}

// CreateApp creates a new app. The name travels in the body.
func (c *Client) CreateApp(ctx context.Context, spec AppSpec) (*Operation, error) {
	spec.ProjectID = c.creds.ProjectID
	var op Operation
	if err := c.request(ctx, http.MethodPost, c.containersURL, "/v2/containers", nil, spec, &op); err != nil {
		return nil, errors.Wrapf(err, "failed to create cloud.ru container app %q", spec.Name)
	}
	return &op, nil
}

// PatchApp updates an existing app, which publishes a NEW revision rather than
// replacing the running one: 00001 stays up until 00002 is active, and the switch
// is unnoticed. That is blue-green for free, and the reason this driver patches
// instead of recreating.
//
// `privileged` cannot travel in a patch (PatchContainerConfiguration has no such
// field), so it is stripped here and the caller is responsible for noticing a
// change — see Configuration in AppConfig.
func (c *Client) PatchApp(ctx context.Context, spec AppSpec) (*Operation, error) {
	name := spec.Name
	if name == "" {
		return nil, errors.Errorf("cannot patch a cloud.ru container app without a name")
	}
	spec.ProjectID = c.creds.ProjectID
	if spec.Configuration != nil && spec.Configuration.Privileged != nil {
		cfg := *spec.Configuration
		cfg.Privileged = nil
		spec.Configuration = &cfg
	}
	var op Operation
	if err := c.request(ctx, http.MethodPatch, c.containersURL,
		"/v2/containers/"+url.PathEscape(name), nil, spec, &op); err != nil {
		return nil, errors.Wrapf(err, "failed to patch cloud.ru container app %q", name)
	}
	return &op, nil
}

// DeleteApp removes an app. An absent app is not an error: a destroy that runs
// twice, or after a partial create, must still converge.
func (c *Client) DeleteApp(ctx context.Context, name string) error {
	err := c.request(ctx, http.MethodDelete, c.containersURL,
		"/v2/containers/"+url.PathEscape(name), c.projectQuery(), nil, nil)
	if err != nil && !NotFound(err) {
		return errors.Wrapf(err, "failed to delete cloud.ru container app %q", name)
	}
	return nil
}

// Revisions lists an app's revisions, newest first.
//
// Takes the app's NAME. The neighbouring `/{containerId}/operations` takes the
// app's UUID, and each rejects the other's identifier with an error that reads
// like "no such resource": a name in the id path is `400 badly formed hexadecimal
// UUID string`, a UUID in this one is `499 Serverless with name is not found`.
func (c *Client) Revisions(ctx context.Context, name string) ([]Revision, error) {
	var out revisionListResponse
	if err := c.request(ctx, http.MethodGet, c.containersURL,
		"/v2/containers/"+url.PathEscape(name)+"/revisions", c.projectQuery(), nil, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to list revisions of cloud.ru container app %q", name)
	}
	revisions := out.Data
	// Revision names carry a zero-padded ordinal (`…-00002`), so a reverse
	// lexicographic sort puts the newest first without parsing a timestamp whose
	// format the API does not declare.
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].Name > revisions[j].Name })
	return revisions, nil
}

// Logs returns the app's most recent output. Used on a readiness timeout: it is
// the only endpoint that showed the CrashLoop at all. `/instances`, `/events` and
// `/status` are 503 from envoy — they do not exist.
func (c *Client) Logs(ctx context.Context, name string, limit int) ([]LogLine, error) {
	query := c.projectQuery()
	if limit > 0 {
		query.Set("pageSize", strconv.Itoa(limit))
	}
	var out logListResponse
	if err := c.request(ctx, http.MethodGet, c.containersURL,
		"/v2/containers/"+url.PathEscape(name)+"/logs", query, nil, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to read logs of cloud.ru container app %q", name)
	}
	return out.Data, nil
}

// RevisionHealthy reports whether a revision status means "serving".
func RevisionHealthy(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case RevisionStatusActive, RevisionStatusRunning, RevisionStatusReady:
		return true
	}
	return false
}

// RevisionFailed reports whether a revision status means "will never serve".
//
// Matched on substrings rather than an enumeration: the API documents no status
// vocabulary, and treating an unknown terminal status as "still waiting" is the
// failure mode this whole function exists to avoid — a deploy that burns its
// timeout and then reports a timeout instead of the error it already had.
func RevisionFailed(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	for _, bad := range []string{"fail", "error", "crash", "unhealthy", "cancel"} {
		if strings.Contains(s, bad) {
			return true
		}
	}
	return false
}

// WaitReady blocks until the app's newest revision is serving.
//
// This is the whole reason the driver cannot trust the obvious signals. The
// create *operation* reports `done: true` in ~21 s whether or not the container
// ever started; the app's own `status` field reported `publishing` for four
// minutes against a CrashLooping pod; and the public URL answers 404 in the
// meantime. Only the revision list tells the truth. Same shape as a green Yandex
// Cloud deploy with zero revisions — and the same consequence if ignored: a green
// `sc deploy` and a dead site.
//
// On failure or timeout the app's recent log lines are folded into the error,
// because that is where the actual cause is, and an operator reading a CI log
// will not go hunting for a second API call.
func (c *Client) WaitReady(ctx context.Context, name string, timeout, interval time.Duration) (*App, error) {
	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		revisions, err := c.Revisions(ctx, name)
		if err != nil && !NotFound(err) {
			return nil, err
		}
		if len(revisions) > 0 {
			newest := revisions[0]
			lastStatus = newest.Status
			switch {
			case RevisionHealthy(newest.Status):
				return c.GetApp(ctx, name)
			case RevisionFailed(newest.Status):
				return nil, errors.Errorf("cloud.ru container app %q revision %q is %s%s",
					name, newest.Name, newest.Status, c.logTail(ctx, name))
			}
		}
		if time.Now().After(deadline) {
			return nil, errors.Errorf("cloud.ru container app %q did not become ready within %s (newest revision status: %q)%s",
				name, timeout, lastStatus, c.logTail(ctx, name))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// logTail renders recent output for an error message, or a note saying why it
// could not. It never returns an error of its own: it is called from the failure
// path, where losing the original diagnosis to a secondary failure is worse than
// a missing log.
func (c *Client) logTail(ctx context.Context, name string) string {
	lines, err := c.Logs(ctx, name, 50)
	if err != nil {
		return fmt.Sprintf("\n(container logs unavailable: %v)", err)
	}
	if len(lines) == 0 {
		return "\n(no container logs yet — the image may have failed to be pulled at all)"
	}
	var b strings.Builder
	b.WriteString("\ncontainer logs (most recent last):")
	// The API returns newest first; reverse so the error reads like a log.
	for i := len(lines) - 1; i >= 0; i-- {
		b.WriteString("\n  ")
		b.WriteString(strings.TrimRight(lines[i].Message, "\n"))
	}
	return b.String()
}
