// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// Defaults for the converge operations below.
const (
	// DefaultWaitTimeout is how long a deploy waits for a revision to serve. The
	// measured create takes ~21 s to report `done` and the revision follows within
	// seconds, so ten minutes is not a budget — it is the point at which "still
	// publishing" has stopped being a plausible explanation.
	DefaultWaitTimeout = 10 * time.Minute
	// DefaultPollInterval is how often the revision list is re-read.
	DefaultPollInterval = 5 * time.Second
	// DefaultRegistryTimeout is how long registry creation is given to reach ACTIVE.
	DefaultRegistryTimeout = 5 * time.Minute
)

// Request is the document the `sc cloudru …` subcommands read from **stdin**, and
// the document the provisioning tier writes.
//
// It lives here, next to the client, so the two halves cannot drift: the
// provisioning tier marshals this type into a pulumi-command `Stdin` and the CLI
// unmarshals the same type out of os.Stdin. Stdin rather than flags because it
// carries the IAM key secret and the stack's own secrets, and an argument is
// visible in `ps` to every other process on the runner.
type Request struct {
	Credentials

	// Name is the Container App. Authoritative for both apply and destroy — on
	// apply it must agree with Spec.Name.
	Name string `json:"name,omitempty"`

	// RegistryName is the Artifact Registry to ensure, for the registry
	// subcommand. Ignored by the app subcommands.
	RegistryName string `json:"registryName,omitempty"`

	// Spec is the desired state of the app. Required by apply, unused by destroy.
	Spec *AppSpec `json:"spec,omitempty"`

	// VmSpec is the desired state of a compute VM, for the compute-vm
	// subcommands. Required by their apply, optional for destroy (which needs
	// only Name).
	VmSpec *VmSpec `json:"vmSpec,omitempty"`

	// DnsSpec is the desired state of one Cloud DNS record set, for the
	// dns-record subcommands. Required by both of theirs — a destroy needs the
	// zone reference as well as the name, since the zone is resolved by lookup.
	DnsSpec *DnsRecordSpec `json:"dnsSpec,omitempty"`

	// WaitTimeout and PollInterval are Go duration strings; empty means the
	// defaults above.
	WaitTimeout  string `json:"waitTimeout,omitempty"`
	PollInterval string `json:"pollInterval,omitempty"`
}

// Result is what the subcommands print to **stdout** as JSON, so the
// provisioning tier can read the facts it could not know before the call (the
// app's id and the platform URL) off the command's Stdout output.
//
// Progress goes to stderr. Keeping stdout to exactly one JSON document is what
// makes it parseable at all.
type Result struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	URL          string `json:"url,omitempty"`
	Status       string `json:"status,omitempty"`
	Revision     string `json:"revision,omitempty"`
	RegistryHost string `json:"registryHost,omitempty"`

	// VmResult carries the compute-vm facts. Embedded rather than nested so the
	// stdout document stays one flat object, which is what the provisioning tier
	// parses with a single pass.
	VmResult

	// DnsResult carries the dns-record facts, embedded for the same reason.
	DnsResult
}

func (r Request) waitTimeout() (time.Duration, error) {
	return parseDuration(r.WaitTimeout, DefaultWaitTimeout, "waitTimeout")
}

func (r Request) pollInterval() (time.Duration, error) {
	return parseDuration(r.PollInterval, DefaultPollInterval, "pollInterval")
}

func parseDuration(value string, fallback time.Duration, field string) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid %s %q", field, value)
	}
	if d <= 0 {
		return 0, errors.Errorf("%s must be positive, got %q", field, value)
	}
	return d, nil
}

// ApplyRegistry makes the project's Artifact Registry exist and reports its
// docker host.
//
// Separate from ApplyApp, and run before it, because the image push happens
// between them: `docker push <name>.cr.cloud.ru/…` needs the registry, and the
// app needs the image.
func (c *Client) ApplyRegistry(ctx context.Context, req Request) (*Result, error) {
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if req.RegistryName == "" {
		return nil, errors.Errorf("`registryName` is required")
	}
	timeout, err := parseDuration(req.WaitTimeout, DefaultRegistryTimeout, "waitTimeout")
	if err != nil {
		return nil, err
	}
	reg, err := c.EnsureRegistry(ctx, req.RegistryName, timeout)
	if err != nil {
		return nil, err
	}
	return &Result{
		ID:           reg.ID,
		Name:         reg.Name,
		Status:       reg.Status,
		RegistryHost: reg.Host(),
	}, nil
}

// ApplyApp creates or updates one Container App and waits until it serves.
//
// Create-or-patch rather than always-recreate, because a patch publishes a new
// revision while the old one keeps serving — blue-green at no cost. The one thing
// a patch cannot carry is `privileged`, so a change there is detected and refused
// instead of being silently dropped by the API.
func (c *Client) ApplyApp(ctx context.Context, req Request) (*Result, error) {
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if req.Spec == nil {
		return nil, errors.Errorf("`spec` is required")
	}
	name := req.Name
	if name == "" {
		name = req.Spec.Name
	}
	if name == "" {
		return nil, errors.Errorf("`name` is required")
	}
	if req.Spec.Name != "" && req.Spec.Name != name {
		return nil, errors.Errorf("`name` (%q) and `spec.name` (%q) disagree", name, req.Spec.Name)
	}
	spec := *req.Spec
	spec.Name = name

	timeout, err := req.waitTimeout()
	if err != nil {
		return nil, err
	}
	interval, err := req.pollInterval()
	if err != nil {
		return nil, err
	}

	existing, found, err := c.FindApp(ctx, name)
	if err != nil {
		return nil, err
	}
	if found {
		if err := checkPrivilegedDrift(existing, &spec); err != nil {
			return nil, err
		}
		if _, err := c.PatchApp(ctx, spec); err != nil {
			return nil, err
		}
	} else if _, err := c.CreateApp(ctx, spec); err != nil {
		return nil, err
	}

	app, err := c.WaitReady(ctx, name, timeout, interval)
	if err != nil {
		return nil, err
	}

	res := &Result{Name: name}
	if app != nil {
		res.ID = app.ID
		res.URL = app.PublicURL()
		res.Status = app.Status
	}
	if revisions, err := c.Revisions(ctx, name); err == nil && len(revisions) > 0 {
		res.Revision = revisions[0].Name
	}
	return res, nil
}

// checkPrivilegedDrift refuses a change the API would accept and then ignore.
//
// PatchContainerConfiguration has no `privileged` field, so a patch that flips it
// succeeds, publishes a revision, and leaves the setting exactly as it was. The
// operator would see a green deploy and an unchanged runtime — the failure mode
// this whole driver is written to avoid. Recreating the app on SC's own initiative
// is not the answer either: it takes the public URL down and, for a stack with a
// custom domain, changes what the record has to point at.
func checkPrivilegedDrift(existing *App, desired *AppSpec) error {
	current := false
	if existing != nil && existing.Configuration != nil && existing.Configuration.Privileged != nil {
		current = *existing.Configuration.Privileged
	}
	want := false
	if desired.Configuration != nil && desired.Configuration.Privileged != nil {
		want = *desired.Configuration.Privileged
	}
	if current == want {
		return nil
	}
	return errors.Errorf("cloud.ru container app %q is deployed with privileged=%t and the config asks for privileged=%t, "+
		"which cannot be changed in place: Cloud.ru's patch request has no `privileged` field, so a deploy would report "+
		"success and leave the container exactly as it is. Destroy the stack (`sc destroy`) and deploy it again to change this",
		existing.Name, current, want)
}

// DestroyApp removes one Container App. Absence is success — a destroy that runs
// twice, or after a create that failed halfway, must still converge.
func (c *Client) DestroyApp(ctx context.Context, req Request) (*Result, error) {
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	name := req.Name
	if name == "" && req.Spec != nil {
		name = req.Spec.Name
	}
	if name == "" {
		return nil, errors.Errorf("`name` is required")
	}
	if err := c.DeleteApp(ctx, name); err != nil {
		return nil, err
	}
	return &Result{Name: name}, nil
}
