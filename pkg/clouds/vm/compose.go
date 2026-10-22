// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/docker"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// TemplateTypeVmCompose is the `type:` of the template that delivers a client stack to
// a `vm` resource as a generated docker compose project.
const TemplateTypeVmCompose = "vm-compose"

// How the built image gets onto the machine.
const (
	// ImageDeliveryRegistry builds, pushes to a registry and pulls on the machine —
	// the same path every other target uses.
	ImageDeliveryRegistry = "registry"
	// ImageDeliverySshTar streams the image over SSH (`sc image ship`, P5.1) and needs
	// no registry at all. It is the default, because "no registry" is the reason this
	// delivery path exists: see compose-on-vm.md §5 (and §5.4 for what it costs).
	ImageDeliverySshTar = "ssh-tar"
)

// What happens to the images SC does NOT build — a `mongo:7` or a `redis:7` that a
// compose service only declares an `image:` for.
//
// The default under `ssh-tar` is ExternalImagesShip, and that is the point of the
// setting existing at all: `ssh-tar` promises "the machine needs no registry", and
// until these were shipped too the promise held only for a stack with no third-party
// image in it. Everything else about the delivery was registry-free while a `mongo:7`
// quietly required the machine to reach Docker Hub — which, inside Russia, is the
// difference between a stack that comes up and one that does not
// (compose-on-vm.md §5.4, §8.3).
const (
	// ExternalImagesShip pulls the image on the runner and streams it to the machine
	// in the same `sc image ship` as everything else. The machine needs no registry;
	// the RUNNER needs one. Private third-party images therefore need a `docker
	// login` + `docker pull` in CI before `sc deploy` — the ship only fetches what is
	// absent from the runner, so a pre-pulled image is simply shipped.
	ExternalImagesShip = "ship"
	// ExternalImagesPull leaves them to the machine's own `docker compose pull`. The
	// right choice when the machine has a route (or a `registryMirrors` entry) and the
	// runner does not, and the only choice for a private third-party registry the
	// runner cannot log into.
	ExternalImagesPull = "pull"
)

// How a new version replaces the running one.
const (
	// StrategyBlueGreen brings the new slot up next to the old one, gates on its
	// health, flips Caddy and only then stops the old slot. Zero downtime, at the cost
	// of running both slots at once (compose-on-vm.md §9.1).
	StrategyBlueGreen = "blue-green"
	// StrategyRecreate stops the old containers and starts the new ones. A few seconds
	// of 502 in exchange for not doubling memory — the honest choice on a small
	// machine, and the operator's to make.
	StrategyRecreate = "recreate"
)

// DefaultKeepImages is how many previous versions stay on the machine for rollback.
// Rollback on a VM is faster than in Kubernetes precisely because the image is already
// there (compose-on-vm.md §9.3).
const DefaultKeepImages = 3

// DefaultSingleImagePort is the port a `type: single-image` stack is told to listen on.
//
// StackConfigSingleImage has no ports field at all — on Lambda and YC Serverless
// Container the runtime decides. On a VM somebody has to pick one, so SC picks it and
// passes it in `PORT`, which is the variable both of those runtimes already use.
const DefaultSingleImagePort = 8080

// EnvCloudVm is set on every container SC runs on a VM, alongside PORT. It is the
// value `SIMPLE_CONTAINER_CLOUD` takes on this delivery path.
//
// Known gap, load-bearing for the first live stand: go-aws-lambda-sdk serves plain HTTP
// only when SIMPLE_CONTAINER_CLOUD is "yandex" or LOCAL_DEBUG is set
// (pkg/service/service.go Start, pkg/service/yandex.go IsYandexCloudRuntime), so an
// image built on that SDK calls lambda.Start on a VM and never answers Caddy. The fix
// belongs in the SDK — treat any non-AWS cloud as HTTP mode — not here: setting
// SIMPLE_CONTAINER_CLOUD=yandex on a Cloud.ru VM would be SC lying about where the
// container runs, and the next reader of that env would believe it.
const (
	EnvCloud      = "SIMPLE_CONTAINER_CLOUD"
	EnvCloudVm    = "vm"
	EnvPort       = "PORT"
	EnvSlot       = "SIMPLE_CONTAINER_SLOT"
	EnvDeployment = "SIMPLE_CONTAINER_VERSION"
)

// ComposeTemplateConfig is the `vm-compose` template config.
//
// It holds delivery behaviour ONLY. Everything about the machine — provider, region,
// size, image, sshUser, ingress — belongs to the `vm` resource this references by name,
// exactly as gcp-gke-autopilot references gkeClusterResource. That split is what lets
// several stacks share one machine and lets the delivery strategy change without
// touching the machine description (compose-on-vm.md §2.1, the operator's 2026-10-14
// correction).
type ComposeTemplateConfig struct {
	api.Credentials            `json:",inline" yaml:",inline"`
	docker.RegistryCredentials `json:",inline" yaml:",inline"`

	// VmResource is the NAME of a `type: vm` resource in the same parent stack.
	VmResource string `json:"vmResource" yaml:"vmResource"`
	// ImageDelivery is ImageDeliverySshTar (default) or ImageDeliveryRegistry.
	ImageDelivery string `json:"imageDelivery,omitempty" yaml:"imageDelivery,omitempty"`
	// ExternalImages is ExternalImagesShip or ExternalImagesPull. It defaults to
	// `ship` under `ssh-tar` and to `pull` under `registry`, where the machine is
	// already talking to a registry and shipping would be strictly more work.
	ExternalImages string `json:"externalImages,omitempty" yaml:"externalImages,omitempty"`
	// Strategy is StrategyBlueGreen (default) or StrategyRecreate.
	Strategy string `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	// KeepImages is how many previous versions to keep for rollback. Defaults to
	// DefaultKeepImages; 0 is a valid explicit choice and means "keep none".
	KeepImages *int `json:"keepImages,omitempty" yaml:"keepImages,omitempty"`
}

// The three api.AuthConfig methods. A template config needs them for the same
// reason a resource config does and it is not obvious from either side: before
// provisioning anything, provision.go's initProvider casts EVERY descriptor's config
// — resource and template alike — to api.AuthConfig to look a Pulumi provider up by
// ProviderType(). A template that does not implement it fails the whole deploy at
// `failed to cast config to api.AuthConfig`, with the stack already created and
// nothing said about templates. Live-caught 2026-10-18, on the first `sc deploy`
// through this path.
//
// Every other template satisfies this by embedding a cloud's auth config
// (k8s.CloudrunTemplate embeds KubernetesConfig, which carries a kubeconfig), so the
// requirement has never had to be stated. This one has no cloud: the only credential
// delivery uses is the deploy key, and that is read from the vm resource's host facts
// in the parent stack at provision time, not from anything written here.

func (c *ComposeTemplateConfig) ProviderType() string {
	return ProviderType
}

// ProjectIdValue is "n/a" for the same reason it is on VmConfig — there is no cloud
// project behind an SSH connection.
func (c *ComposeTemplateConfig) ProjectIdValue() string {
	return "n/a"
}

// CredentialsValue mirrors VmConfig.CredentialsValue: the resolved `${auth:...}` blob
// verbatim, "{}" when none was given. Empty is the normal case here — `byo` has no
// cloud credentials, and the registry ones (when imageDelivery is `registry`) are
// their own fields, not this.
func (c *ComposeTemplateConfig) CredentialsValue() string {
	return lo.If(c.Credentials.Credentials == "", "{}").Else(c.Credentials.Credentials)
}

func (c *ComposeTemplateConfig) EffectiveImageDelivery() string {
	return lo.If(c.ImageDelivery == "", ImageDeliverySshTar).Else(c.ImageDelivery)
}

// EffectiveExternalImages resolves what to do with third-party images.
//
// The default follows the delivery mode rather than being a constant, because the two
// defaults are each right for their mode and a single one would be wrong for the
// other: under `registry` the machine is pulling from a registry anyway, so shipping a
// `mongo:7` through the runner would move bytes for nothing.
func (c *ComposeTemplateConfig) EffectiveExternalImages() string {
	if c.ExternalImages != "" {
		return c.ExternalImages
	}
	return lo.If(c.EffectiveImageDelivery() == ImageDeliverySshTar, ExternalImagesShip).Else(ExternalImagesPull)
}

// ShipsExternalImages reports whether this deploy streams third-party images to the
// machine itself.
//
// `registry` delivery never does, even when asked: there is no `sc image ship` in that
// mode at all, and adding one so that a third-party image could arrive by a different
// route than the stack's own would be two delivery paths in one deploy. Validate says
// so out loud rather than letting the setting be silently ignored.
func (c *ComposeTemplateConfig) ShipsExternalImages() bool {
	return c.EffectiveImageDelivery() == ImageDeliverySshTar &&
		c.EffectiveExternalImages() == ExternalImagesShip
}

func (c *ComposeTemplateConfig) EffectiveStrategy() string {
	return lo.If(c.Strategy == "", StrategyBlueGreen).Else(c.Strategy)
}

func (c *ComposeTemplateConfig) EffectiveKeepImages() int {
	if c.KeepImages == nil {
		return DefaultKeepImages
	}
	return *c.KeepImages
}

// Validate refuses a template config SC cannot honour. Like VmConfig.Validate it runs
// in the provisioning tier and never in the config reader, because cmd/schema-gen
// drives every reader with an empty config and silently drops the ones that error.
func (c *ComposeTemplateConfig) Validate() error {
	if c.VmResource == "" {
		return errors.Errorf(
			"`%s` template requires `vmResource` — the name of a `type: %s` resource in the same parent stack",
			TemplateTypeVmCompose, ResourceTypeVm,
		)
	}
	if d := c.EffectiveImageDelivery(); d != ImageDeliverySshTar && d != ImageDeliveryRegistry {
		return errors.Errorf("unknown `imageDelivery` %q; one of: %s, %s", d, ImageDeliveryRegistry, ImageDeliverySshTar)
	}
	if e := c.EffectiveExternalImages(); e != ExternalImagesShip && e != ExternalImagesPull {
		return errors.Errorf("unknown `externalImages` %q; one of: %s, %s", e, ExternalImagesShip, ExternalImagesPull)
	}
	// Refused rather than ignored. A config that says `externalImages: ship` and
	// delivers by registry would otherwise report green while doing the opposite of
	// what it was told, and the symptom — the machine pulling an image the operator
	// believed was shipped — only shows up when the registry is unreachable, which is
	// exactly the day nobody wants to be reading delivery code.
	if c.ExternalImages == ExternalImagesShip && c.EffectiveImageDelivery() != ImageDeliverySshTar {
		return errors.Errorf(
			"`externalImages: %s` needs `imageDelivery: %s`: under `%s` the machine pulls from the registry "+
				"and there is no ship to put a third-party image in",
			ExternalImagesShip, ImageDeliverySshTar, ImageDeliveryRegistry,
		)
	}
	if s := c.EffectiveStrategy(); s != StrategyBlueGreen && s != StrategyRecreate {
		return errors.Errorf("unknown `strategy` %q; one of: %s, %s", s, StrategyBlueGreen, StrategyRecreate)
	}
	if c.EffectiveKeepImages() < 0 {
		return errors.Errorf("`keepImages` must not be negative, got %d", c.EffectiveKeepImages())
	}
	if c.EffectiveImageDelivery() == ImageDeliveryRegistry && lo.FromPtr(c.DockerRegistryURL) == "" {
		return errors.Errorf(
			"`imageDelivery: %s` requires `dockerRegistryURL` on the template (with `dockerRegistryUsername` "+
				"and `dockerRegistryPassword` if the registry is private); use `imageDelivery: %s` to ship the "+
				"image over SSH instead and need no registry at all",
			ImageDeliveryRegistry, ImageDeliverySshTar,
		)
	}
	return nil
}

// Mount types SC renders. A `bind` mount is refused at conversion time: its source is a
// path on the developer's machine, which does not exist on the VM.
const (
	MountTypeVolume = "volume"
	MountTypeTmpfs  = "tmpfs"
)

// VmMount is one volume mount of one service, classified while the customer's compose
// file is still in hand.
//
// k8s.DeploymentConfig flattens every mount into a PersistentVolume and keeps neither
// the mount type nor the read-only flag — on a VM both decide real behaviour: which
// volume must be external (and therefore survive a slot teardown) and which service is
// stateful (and therefore must not be duplicated by blue-green).
type VmMount struct {
	Service   string `json:"service" yaml:"service"`
	Type      string `json:"type" yaml:"type"`
	Name      string `json:"name" yaml:"name"`
	MountPath string `json:"mountPath" yaml:"mountPath"`
	ReadOnly  bool   `json:"readOnly" yaml:"readOnly"`
	// Size is a tmpfs size, when the customer set one.
	Size string `json:"size,omitempty" yaml:"size,omitempty"`
}

// IsStateful reports whether this mount makes its service unsafe to run in two slots at
// once. Two writers on one volume corrupt it, so such a service lives outside the slots
// (compose-on-vm.md §9.1).
func (m VmMount) IsStateful() bool {
	return m.Type == MountTypeVolume && !m.ReadOnly
}

// VmDependsOn is one `depends_on` edge, carried over from the customer's compose file as
// written (compose-on-vm.md §6.1). It is enough for "Mongo before the backend" and is
// not a guarantee of anything: a Mongo restart does not restart the backend, so the
// application still has to survive its dependency being away.
type VmDependsOn struct {
	Service   string `json:"service" yaml:"service"`
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// VmHealthCheck is a compose healthcheck, carried over verbatim.
//
// It cannot come from DeploymentConfig either: ConvertComposeToContainers turns a
// compose healthcheck into a Kubernetes-shaped HTTP probe and drops the test command.
// On a VM the test command IS the check, and for the ingress service it is the gate the
// blue-green flip waits on (compose-on-vm.md §9.2).
type VmHealthCheck struct {
	Test        []string `json:"test" yaml:"test"`
	Interval    string   `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StartPeriod string   `json:"startPeriod,omitempty" yaml:"startPeriod,omitempty"`
	Retries     *int     `json:"retries,omitempty" yaml:"retries,omitempty"`
	// Synthesized marks a check SC invented because the customer declared none. It is
	// weak on purpose and the operator is told so.
	Synthesized bool `json:"synthesized,omitempty" yaml:"synthesized,omitempty"`
}

// VmComposeInput is what the provisioning tier receives for a `vm-compose` stack.
//
// Deployment is the same k8s.DeploymentConfig the Kubernetes path builds, reused rather
// than re-derived — that is what keeps `env`, `secrets`, `size`, `runs` and cloudExtras
// behaving the same here as on GKE. The three maps beside it carry what compose
// expresses and Kubernetes does not (see VmMount, VmDependsOn, VmHealthCheck).
type VmComposeInput struct {
	ComposeTemplateConfig `json:"templateConfig" yaml:"templateConfig"`
	Deployment            k8s.DeploymentConfig `json:"deployment" yaml:"deployment"`

	Mounts       []VmMount                `json:"mounts,omitempty" yaml:"mounts,omitempty"`
	DependsOn    map[string][]VmDependsOn `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	HealthChecks map[string]VmHealthCheck `json:"healthChecks,omitempty" yaml:"healthChecks,omitempty"`

	// SingleImage records that this input was projected from a `type: single-image`
	// client stack rather than read from a compose file.
	SingleImage bool `json:"singleImage,omitempty" yaml:"singleImage,omitempty"`

	// Warnings are the things SC accepted but could not honour literally, and anything
	// it had to invent. The provisioning tier logs them; the rule they serve is that a
	// field is either supported or refused, never silently dropped
	// (compose-on-vm.md §2.2).
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func (i *VmComposeInput) Uses() []string {
	return i.Deployment.StackConfig.Uses
}

func (i *VmComposeInput) OverriddenBaseZone() string {
	return i.Deployment.StackConfig.BaseDnsZone
}

func (i *VmComposeInput) DependsOnResources() []api.StackConfigDependencyResource {
	return i.Deployment.StackConfig.Dependencies
}

// StatefulServices lists the services that must run outside the blue-green slots,
// sorted for a stable render.
func (i *VmComposeInput) StatefulServices() []string {
	names := lo.Uniq(lo.FilterMap(i.Mounts, func(m VmMount, _ int) (string, bool) {
		return m.Service, m.IsStateful()
	}))
	sort.Strings(names)
	return names
}

// MountsOf returns the mounts of one service in declaration order.
func (i *VmComposeInput) MountsOf(service string) []VmMount {
	return lo.Filter(i.Mounts, func(m VmMount, _ int) bool {
		return m.Service == service
	})
}

// MountsOfVolume returns every mount of one named volume, across services.
func (i *VmComposeInput) MountsOfVolume(volume string) []VmMount {
	return lo.Filter(i.Mounts, func(m VmMount, _ int) bool {
		return m.Type == MountTypeVolume && m.Name == volume
	})
}

// ExternalVolumes lists the named volumes the deploy must create before `up`, sorted.
// They are external to every compose project on purpose (compose-on-vm.md §6 rule 4).
func (i *VmComposeInput) ExternalVolumes() []string {
	names := lo.Uniq(lo.FilterMap(i.Mounts, func(m VmMount, _ int) (string, bool) {
		return m.Name, m.Type == MountTypeVolume
	}))
	sort.Strings(names)
	return names
}

func ReadComposeTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ComposeTemplateConfig{})
}

// warn appends a warning, keeping the wording of the places that produce them in one
// style: what SC did, and what the operator should do instead.
func (i *VmComposeInput) warn(format string, args ...any) {
	i.Warnings = append(i.Warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
}
