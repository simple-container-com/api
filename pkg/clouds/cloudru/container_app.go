// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// TemplateTypeCloudruContainerApp is the Cloud.ru Evolution analogue of
// `yc-serverless-container` and of an AWS lambda: one `type: single-image` client
// stack becomes one serverless container on a public HTTPS domain, scaling to
// zero.
const TemplateTypeCloudruContainerApp = "cloudru-container-app"

// Defaults, all of them the values a live walking skeleton was measured working
// with on 2026-10-22 rather than the API's documented defaults.
const (
	// DefaultContainerPort is above evolution.MinContainerPort on purpose: the
	// platform refuses a lower port unless the container runs privileged.
	DefaultContainerPort = 8080
	// DefaultCPU is a CPU *request*, as a core fraction.
	DefaultCPU = "0.1"
	// DefaultMemoryMB is used when the client stack sets no maxMemory.
	DefaultMemoryMB = 256
	// DefaultMaxInstances keeps an unconfigured stack from scaling without bound.
	DefaultMaxInstances = 1
	// DefaultIdleTimeout is how long an idle container lives before scaling to
	// zero. The platform's own default; cold start after it is ~3.1 s against
	// ~0.68 s warm, so anything interactive wants `min: 1` instead of a longer
	// idle window.
	DefaultIdleTimeout = "60s"
	// DefaultTimeoutSeconds matches the AWS lambda and YC container defaults so
	// one client.yaml sizes the same on all three.
	DefaultTimeoutSeconds = 10
)

// ReservedEnvNames are the environment-variable names the Cloud.ru Evolution
// runtime defines itself and refuses to let a revision set.
//
// The refusal is not a warning and not a per-variable skip — the create/update
// call fails and no revision is published:
//
//	code 3, Invalid argument; fieldViolations[0]: field "name",
//	"The container env variable name 'PORT' is reserved. Give another name for
//	 the variable; input=PORT"
//
// Measured 2026-10-23. The same name is reserved on YC Serverless Containers,
// where SC hit it first; see the Yandex section of forge/CLAUDE.md. The list is
// deliberately a list rather than one constant because the platform is young and
// the error above names the variable, so the next one is cheap to add.
var ReservedEnvNames = []string{"PORT"}

// appNameRegexp is Cloud.ru's own constraint on a Container App name, and on the
// name of every container inside its template. Checked here because the API
// answers a violation with a bare 400 that names no field — and because the
// minimum length of 5 catches the obvious container name `app`.
var appNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{3,48}[a-z0-9]$`)

// MaxAppNameLen is the longest name the regex above admits.
const MaxAppNameLen = 50

// MinAppNameLen is the shortest.
const MinAppNameLen = 5

// ContainerAppTemplateConfig is the parent-stack half of a `cloudru-container-app`
// template: the credentials plus the knobs an operator sets once for every service
// deployed under the template.
type ContainerAppTemplateConfig struct {
	EvolutionAccountConfig `json:",inline" yaml:",inline"`

	// RegistryName is the Artifact Registry images are pushed to and pulled from,
	// within this project. Optional: when empty the driver creates (and reuses) one
	// named after the client stack with the environment stripped, so all
	// environments of a stack share a registry and the repository inside it carries
	// the `--<env>` suffix. (Not the parent stack, which would read better here —
	// api.StackParams carries no parent stack name.) Set this to share one registry
	// across several stacks.
	//
	// A registry is not optional in the way it is on other clouds — Container Apps
	// resolve images from the calling project's own Artifact Registry and from
	// nowhere else, so there is always one, SC's or the operator's.
	RegistryName string `json:"registryName,omitempty" yaml:"registryName,omitempty"`
}

func ReadContainerAppTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ContainerAppTemplateConfig{})
}

// ContainerAppInput is what the provisioning tier receives for a
// `cloudru-container-app` stack: resolved account credentials plus the client
// stack config, kept whole so the provisioner can still decode cloudExtras.
type ContainerAppInput struct {
	EvolutionAccountConfig `json:",inline" yaml:",inline"`

	// RegistryName is carried over from the template, since ConvertAuth rehydrates
	// only credential fields.
	RegistryName string                     `json:"registryName,omitempty" yaml:"registryName,omitempty"`
	StackConfig  api.StackConfigSingleImage `json:"stackConfig" yaml:"stackConfig"`
}

func (i *ContainerAppInput) Uses() []string {
	return i.StackConfig.Uses
}

func (i *ContainerAppInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.StackConfig.Dependencies
}

func (i *ContainerAppInput) OverriddenBaseZone() string {
	return i.StackConfig.BaseDnsZone
}

// CloudExtras decodes and validates the client stack's cloudExtras block.
func (i *ContainerAppInput) CloudExtras() (*ContainerAppExtras, error) {
	return ReadContainerAppExtras(i.StackConfig.CloudExtras)
}

// ContainerAppExtras is the Cloud.ru decoding of a client stack's `cloudExtras`
// block. Everything here is a property of one service, not of the shared
// infrastructure its parent stack provisions.
type ContainerAppExtras struct {
	// ContainerPort is the port the image listens on. Defaults to
	// DefaultContainerPort.
	//
	// Cloud.ru needs this explicitly, unlike YC (which injects PORT) and AWS
	// (where a lambda has no port at all) — so it is the one field a stack moving
	// to this cloud has to add.
	ContainerPort *int `json:"containerPort,omitempty" yaml:"containerPort,omitempty"`

	// Privileged runs the container as root.
	//
	// Off by default, and that is a decision rather than an omission. The runtime
	// drops privileges unless this is set, which means an unprivileged container
	// can neither bind a port below evolution.MinContainerPort nor write to a
	// root-owned path inside its own image — so plain `nginx` CrashLoops on
	// `mkdir("/var/cache/nginx/client_temp") failed (13: Permission denied)`.
	// Defaulting this to true would make such images work, and would also hand
	// root to every client image SC ever deploys on this cloud, silently and
	// permanently. Instead the driver refuses the combinations it can prove will
	// fail and names this field in the error, so the privilege is granted by
	// someone who meant to grant it.
	//
	// **Create-only.** Cloud.ru's patch request has no `privileged` field, so
	// changing it on a live app is a destroy-and-recreate, not an update; the
	// driver detects the drift and says so rather than patching around it.
	Privileged *bool `json:"privileged,omitempty" yaml:"privileged,omitempty"`

	// CPU is the CPU request as a core fraction ("0.1", "1"). Defaults to
	// DefaultCPU.
	CPU string `json:"cpu,omitempty" yaml:"cpu,omitempty"`

	// Concurrency is how many simultaneous requests one instance handles before
	// the platform scales out. Maps to the `concurrency` scaling rule's soft
	// limit.
	Concurrency *int `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`

	// IdleTimeout is how long an idle instance survives before scale-to-zero, as
	// a duration string with a unit ("60s", "5m"). Defaults to
	// DefaultIdleTimeout.
	IdleTimeout string `json:"idleTimeout,omitempty" yaml:"idleTimeout,omitempty"`

	// CertificateID is the Cloud.ru certificate bound to the client stack's
	// `domain`. Required to use a custom domain at all: the platform's wildcard
	// certificate covers only its own `*.containerapps.ru` zone, and there is no
	// ACME on this path.
	CertificateID string `json:"certificateId,omitempty" yaml:"certificateId,omitempty"`

	// ContainerName overrides the name of the container inside the app's template.
	// Rarely needed; it exists because the name is regex-checked independently of
	// the app's own, with the same 5-character minimum.
	ContainerName string `json:"containerName,omitempty" yaml:"containerName,omitempty"`
}

// EffectiveContainerPort resolves the configured port.
func (c *ContainerAppExtras) EffectiveContainerPort() int {
	if c.ContainerPort != nil && *c.ContainerPort > 0 {
		return *c.ContainerPort
	}
	return DefaultContainerPort
}

// EffectivePrivileged reports whether the container runs as root.
func (c *ContainerAppExtras) EffectivePrivileged() bool {
	return c.Privileged != nil && *c.Privileged
}

// EffectiveCPU resolves the CPU request.
func (c *ContainerAppExtras) EffectiveCPU() string {
	if strings.TrimSpace(c.CPU) == "" {
		return DefaultCPU
	}
	return strings.TrimSpace(c.CPU)
}

// EffectiveIdleTimeout resolves the scale-to-zero window.
func (c *ContainerAppExtras) EffectiveIdleTimeout() string {
	if strings.TrimSpace(c.IdleTimeout) == "" {
		return DefaultIdleTimeout
	}
	return strings.TrimSpace(c.IdleTimeout)
}

// Validate rejects what the API would reject, at config-parse time.
func (c *ContainerAppExtras) Validate() error {
	port := c.EffectiveContainerPort()
	if port <= 0 || port > 65535 {
		return errors.Errorf("containerPort %d is not a valid port", port)
	}
	// The one combination that fails in the runtime rather than at the API, which
	// is why it is refused here: an unprivileged container cannot bind a low port,
	// and the symptom is a CrashLoop behind a deploy that already reported success.
	if port <= evolution.MinContainerPort && !c.EffectivePrivileged() {
		return errors.Errorf("containerPort %d requires `privileged: true`: a Cloud.ru container runs "+
			"unprivileged by default and cannot bind a port at or below %d. Either listen on a higher port "+
			"(the default is %d) or set `cloudExtras.privileged: true` — which also runs the container as root",
			port, evolution.MinContainerPort, DefaultContainerPort)
	}
	if c.Concurrency != nil && *c.Concurrency < 1 {
		return errors.Errorf("concurrency must be at least 1, got %d", *c.Concurrency)
	}
	if name := strings.TrimSpace(c.ContainerName); name != "" && !appNameRegexp.MatchString(name) {
		return errors.Errorf("containerName %q is not a valid Cloud.ru name: %s", name, appNameRule())
	}
	return nil
}

// ReadContainerAppExtras decodes and validates a client stack's cloudExtras block,
// returning an empty (valid) value when the block is absent.
func ReadContainerAppExtras(cloudExtras *any) (*ContainerAppExtras, error) {
	res := &ContainerAppExtras{}
	if cloudExtras == nil {
		return res, res.Validate()
	}
	res, err := api.ConvertDescriptor(cloudExtras, res)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloudExtras to cloudru.ContainerAppExtras")
	}
	if err := res.Validate(); err != nil {
		return nil, err
	}
	return res, nil
}

// ToContainerAppConfig converts a `cloudru-container-app` template plus a
// single-image client stack into the provisioning input.
func ToContainerAppConfig(tpl any, stackCfg *api.StackConfigSingleImage) (any, error) {
	templateCfg, ok := tpl.(*ContainerAppTemplateConfig)
	if !ok {
		return nil, errors.Errorf("template config is not of type *cloudru.ContainerAppTemplateConfig")
	}
	if templateCfg == nil {
		return nil, errors.Errorf("template config is nil")
	}
	if stackCfg == nil {
		return nil, errors.Errorf("stack config cannot be nil")
	}

	accountConfig := &EvolutionAccountConfig{}
	if err := api.ConvertAuth(&templateCfg.EvolutionAccountConfig, accountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert cloud.ru evolution account config")
	}

	res := &ContainerAppInput{
		EvolutionAccountConfig: *accountConfig,
		RegistryName:           templateCfg.RegistryName,
		StackConfig:            *stackCfg,
	}

	// Fail here rather than mid-provision: these are config errors and the
	// operator is looking at the config right now.
	if _, err := res.CloudExtras(); err != nil {
		return nil, err
	}

	return res, nil
}

func appNameRule() string {
	return fmt.Sprintf("%d-%d characters, lowercase letters, digits and single hyphens, starting with a letter and "+
		"not ending with a hyphen", MinAppNameLen, MaxAppNameLen)
}

// ValidateAppName checks a name against Cloud.ru's rule, explaining it.
//
// The hyphen-run check is separate from appNameRegexp rather than folded into it
// so that this function refuses exactly what the API refuses: a run is legal in
// SC's own naming and illegal in Cloud.ru's, and before 2026-10-23 this validator
// passed `<stack>--<env>` straight through to a 400 from Artifact Registry.
func ValidateAppName(name string) error {
	if appNameRegexp.MatchString(name) && !hyphenRunRegexp.MatchString(name) {
		return nil
	}
	return errors.Errorf("%q is not a valid Cloud.ru Container App name: %s", name, appNameRule())
}

// nonNameCharsRegexp matches every run of characters a Cloud.ru name may not
// contain.
var nonNameCharsRegexp = regexp.MustCompile(`[^a-z0-9-]+`)

// hyphenRunRegexp matches a run of two or more hyphens, which is exactly what
// SC's `<stack>--<env>` naming produces and what Cloud.ru refuses — see
// ToAppName.
var hyphenRunRegexp = regexp.MustCompile(`-{2,}`)

// ToAppName makes an SC resource name usable as a Container App name.
//
// It normalises rather than rejecting, because the input is not the operator's to
// choose: SC's name for a client stack is always `<stack>--<env>`, and the app's
// own name is derived from it. The rule has four edges that a stack name hits in
// practice — it must start with a letter, it must not end with a hyphen, it must
// not contain a RUN of hyphens, and it must be at least 5 and at most 50
// characters — so a bare rejection would make perfectly ordinary stacks
// undeployable on this cloud alone.
//
// The hyphen run is the one that is not obvious, and it is measured rather than
// read off a doc page (live smoke 2026-10-23, project a8236278):
//
//   - Artifact Registry validates a registry name against
//     `^[a-z][a-z0-9]+(?:[-][a-z0-9]+)*$` and answers a doubled hyphen with
//     `HTTP 400 invalid CreateRegistryRequest.Name`.
//   - The registry's docker endpoint rejects it again, separately, on push:
//     `invalid repository name`.
//
// So `<stack>--<env>` — the name EVERY client stack has — is refused twice on the
// way to a first deploy. Collapsing runs here is what makes the default path work
// at all, and it matches what the Yandex driver already does for the same reason
// (toRepositoryName, pkg/clouds/pulumi/yandex/docker.go). The cost is that
// `foo--staging` and `foo-staging` map to one name; that is accepted because the
// alternative is a cloud on which no stack deploys without a manual override.
//
// Truncation keeps the PREFIX, which is the stack name; the `--<env>` suffix is
// what gets cut on a long one. That is deliberate but worth knowing: two
// environments of a stack whose name is already near 50 characters would collide,
// and the driver would then patch one app from two stacks. ValidateAppName is
// still called on the result, so an input this function cannot rescue fails
// loudly.
func ToAppName(name string) string {
	out := nonNameCharsRegexp.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	// Must not contain a run of hyphens — collapsed BEFORE the length checks, so
	// truncation measures the name the API will actually see.
	out = hyphenRunRegexp.ReplaceAllString(out, "-")
	// Must start with a letter.
	out = strings.TrimLeft(out, "-0123456789")
	if len(out) > MaxAppNameLen {
		out = out[:MaxAppNameLen]
	}
	// Must not end with a hyphen — including after truncation.
	out = strings.TrimRight(out, "-")
	// Pad a too-short name rather than leaving it invalid: `sc` is a legal
	// 2-character stack name and a 4-character one is not rare.
	for len(out) < MinAppNameLen && out != "" {
		out += "0"
	}
	return out
}
