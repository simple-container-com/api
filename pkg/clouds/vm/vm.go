// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package vm is the cloud-independent description of a virtual machine (and, later,
// of a pool of them). It is the config tier behind three delivery paths that all
// want the same thing from a machine and would otherwise each grow their own copy
// of cloud-init, SSH readiness and key handling:
//
//   - compose on one VM (P5),
//   - k3s that SC stands up itself (P3),
//   - a machine the operator brought (the `byo` driver — the zeroth cloud).
//
// See forge/docs/design/2026-10-13/sc-russia-market-readiness/vm-matrix.md (D9).
package vm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
)

// ResourceTypeVm is the `type:` a parent stack's resource declares.
const ResourceTypeVm = "vm"

// The drivers named by a vm resource's `provider:` field. The string is the public
// contract of a parent stack — renaming one turns an existing stack into "unknown
// vm provider", so treat these as frozen.
const (
	// DriverByo takes machines the operator created by hand (or by any means SC
	// knows nothing about) and only connects to them. It is the one driver that
	// works in every cloud on earth, and the one P5 and K.0 are proven on first.
	DriverByo = "byo"

	DriverYandex    = "yc-vm"
	DriverOpenstack = "openstack-vm"
	DriverVkcs      = "vkcs-vm"
	DriverMws       = "mws-vm"
	DriverCloudru   = "cloudru-vm"
	DriverTimeweb   = "timeweb-vm"
)

const (
	// DefaultSshUser is deliberately not root and not the image's stock account
	// (`ubuntu`/`debian`/`centos`): root login is closed on part of the images and
	// an image-dependent user name means an `if` by image in the deploy path.
	// vm-matrix.md §3.2.
	DefaultSshUser = "sc"
	DefaultSshPort = 22
	// DefaultImage is an SC-level alias; each driver maps it to its own image id.
	DefaultImage = "ubuntu-2404-lts"

	// DefaultCores and DefaultMemory size the machine when `size` is absent or
	// half-filled.
	//
	// There has to be a default, and it has to live here rather than per driver: a
	// cloud driver picks a flavour by FILTERING the cloud's flavour list on cores and
	// memory, so "no size given" is not "the cloud's default machine" — it is a filter
	// that matches hundreds of flavours, and the one that gets used is whichever the
	// API happened to list first. Deploying a different machine shape on the next
	// `sc deploy` is not a defensible default.
	//
	// 2 cores / 4Gi is the smallest shape that runs a compose stack plus the Caddy
	// ingress without the docker build cache pushing it into swap.
	DefaultCores  = 2
	DefaultMemory = "4Gi"

	// ConfigDir holds rendered configuration (compose files, env files, Caddy).
	ConfigDir = "/etc/sc"
	// StateDir holds state SC manages on the machine (shipped images, markers).
	StateDir = "/var/lib/sc"
	// CloudInitDoneMarker is the ONLY valid "this machine is ready" signal.
	//
	// SSH answers well before cloud-init has finished installing docker, so a
	// deploy that waits for SSH alone fails intermittently on
	// `docker: command not found`. Same class of mistake as a green YC deploy with
	// zero revisions: success reported before readiness. vm-matrix.md §3.1.
	CloudInitDoneMarker = StateDir + "/cloud-init.done"
)

// VmSize is the machine shape, in SC's own units rather than a per-cloud flavor
// name. Each driver maps it onto what its cloud actually takes (YC's
// resources{cores,memory}, OpenStack's flavor_name, Timeweb's preset_id, …).
type VmSize struct {
	Cores int `json:"cores,omitempty" yaml:"cores,omitempty"`
	// Memory and Disk are quantities in the style the rest of SC's DSL uses
	// (`8Gi`, `60Gi`), not raw byte counts.
	Memory   string `json:"memory,omitempty" yaml:"memory,omitempty"`
	Disk     string `json:"disk,omitempty" yaml:"disk,omitempty"`
	DiskType string `json:"diskType,omitempty" yaml:"diskType,omitempty"`
}

// VmNetwork pins an existing private network instead of letting SC create one.
// Everything in it is optional; empty means "create".
type VmNetwork struct {
	ID string `json:"id,omitempty" yaml:"id,omitempty"`
	// Subnets maps a zone name to the subnet id to use in that zone.
	Subnets map[string]string `json:"subnets,omitempty" yaml:"subnets,omitempty"`
}

// VmIngress configures the Caddy front door that terminates TLS on the machine.
// It is a property of the machine, not of the delivery template, because several
// stacks share one machine and therefore one port 443 (compose-on-vm.md §3, D15).
type VmIngress struct {
	Enable bool `json:"enable" yaml:"enable"`
	// Email is the ACME account address. Required when Enable is set, because a
	// certificate authority will not issue without one.
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
	// Image is the Caddy image to run. Defaults to the same `simplecontainer/caddy`
	// tag the Kubernetes edge uses, which is what makes `lbConfig.siteExtraHelpers`
	// mean the same thing on both paths — the plugins (`rate_limit`, certmagic
	// storage) are in that image and not in the official one. Override to pin a tag
	// or to run a local mirror.
	Image string `json:"image,omitempty" yaml:"image,omitempty"`
	// ImageDelivery is how that image reaches the machine: ImageDeliveryRegistry
	// (default — the machine pulls it) or ImageDeliverySshTar (the runner pulls it
	// and streams it over the provision's SSH connection).
	//
	// The default is "the machine pulls" because that is what every existing vm
	// resource already does, and because shipping makes `sc provision` need a local
	// docker daemon it has never needed before. `ssh-tar` is for the case this whole
	// delivery path exists for: a machine with no route to Docker Hub
	// (compose-on-vm.md §8.3). Caddy is the one image SC itself insists on, so
	// without this the ingress is the last thing on such a machine that still needs
	// a registry — `externalImages: ship` on the template covers a stack's own
	// images and the third-party ones beside them, and stops at the ingress because
	// the ingress belongs to the resource, not the stack.
	ImageDelivery string `json:"imageDelivery,omitempty" yaml:"imageDelivery,omitempty"`
	// AcmeCa points Caddy at a certificate authority other than Let's Encrypt.
	//
	// This field exists because of the Russian market specifically
	// (compose-on-vm.md §8.3): a `.ru` domain opened from inside Russia in a browser
	// without foreign roots wants a certificate from the Russian Trusted CA, and
	// Let's Encrypt may not even be reachable from a closed network. Whether that CA
	// speaks ACME is UNVERIFIED — the field is here because without it there is no
	// option at all, not because the path is known to work.
	AcmeCa string `json:"acmeCa,omitempty" yaml:"acmeCa,omitempty"`
	// AcmeEabKeyID / AcmeEabMacKey are External Account Binding, which a private or
	// enterprise ACME CA generally requires and Let's Encrypt does not.
	//
	// AcmeEabMacKey is a credential: write it as `${secret:...}`. It never appears in
	// the rendered Caddyfile — both values are delivered in the ingress env file
	// (0600, over stdin) and referenced as `{$...}` placeholders, so they stay out of
	// the Pulumi checkpoint and out of a file an operator might paste into a ticket.
	AcmeEabKeyID  string `json:"acmeEabKeyId,omitempty" yaml:"acmeEabKeyId,omitempty"`
	AcmeEabMacKey string `json:"acmeEabMacKey,omitempty" yaml:"acmeEabMacKey,omitempty"`
}

// VmHost is one machine the operator brought — the whole input of DriverByo.
type VmHost struct {
	Address string `json:"address" yaml:"address"`
	User    string `json:"user,omitempty" yaml:"user,omitempty"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
	// PrivateKey is the SSH private key in PEM form, normally written as
	// `${secret:...}`. It never reaches a Pulumi resource unwrapped — see
	// pkg/clouds/pulumi/vm/byo.go.
	PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
	// PrivateKeyPassword decrypts PrivateKey when it is an encrypted PEM.
	PrivateKeyPassword string `json:"privateKeyPassword,omitempty" yaml:"privateKeyPassword,omitempty"`
	// HostKey pins the machine's SSH host key as one authorized_keys line
	// (`ssh-ed25519 AAAA…`). The output of `ssh-keyscan -t ed25519 <address>` can be
	// pasted verbatim too — it is known_hosts format, one field wider, and the
	// leading address is discarded rather than checked (see
	// imageship.TestHostKeyCallbackAcceptsRawSshKeyscanOutput).
	//
	// Optional, and the one optional field here whose absence has a real cost. A
	// deploy onto a VM carries the stack's entire env file, every `${secret:...}` in
	// it, and the deploy key itself; without a pinned host key the SSH client has
	// nothing to check the machine against, so all of that goes to whoever answers on
	// that address (compose-on-vm.md §5.5). CI cannot fall back to known_hosts the way
	// a workstation does — a runner's is empty — so there is no safe implicit default
	// and the provisioning tier warns loudly when this is unset.
	//
	// Not a secret: it is the machine's PUBLIC key, and keeping it readable is what
	// lets an operator compare it against the console by eye.
	HostKey string `json:"hostKey,omitempty" yaml:"hostKey,omitempty"`
}

// EffectiveUser resolves the login name: the host's own, else the resource-level
// default, else DefaultSshUser.
func (h VmHost) EffectiveUser(fallback string) string {
	if h.User != "" {
		return h.User
	}
	return lo.If(fallback == "", DefaultSshUser).Else(fallback)
}

func (h VmHost) EffectivePort() int {
	return lo.If(h.Port == 0, DefaultSshPort).Else(h.Port)
}

// VmConfig is the resource config of `type: vm`.
//
// It lives only in a parent stack's `resources:` section. The `templates:` section
// never configures a machine — it references one by name, exactly as
// gcp-gke-autopilot references gkeClusterResource (vm-matrix.md §1.2,
// compose-on-vm.md §2.1). That split is what lets two stacks share one machine and
// lets the delivery strategy change without touching the machine description.
type VmConfig struct {
	// Driver selects which cloud (or none) creates the machine. The YAML key is
	// `provider`, matching the design and the rest of SC's DSL.
	Driver string `json:"provider" yaml:"provider"`

	Region string   `json:"region,omitempty" yaml:"region,omitempty"`
	Zones  []string `json:"zones,omitempty" yaml:"zones,omitempty"`
	// Image is an SC alias (see DefaultImage), resolved per cloud by the driver.
	Image    string     `json:"image,omitempty" yaml:"image,omitempty"`
	Size     *VmSize    `json:"size,omitempty" yaml:"size,omitempty"`
	SshUser  string     `json:"sshUser,omitempty" yaml:"sshUser,omitempty"`
	PublicIP bool       `json:"publicIP,omitempty" yaml:"publicIP,omitempty"`
	Network  *VmNetwork `json:"network,omitempty" yaml:"network,omitempty"`
	Ingress  *VmIngress `json:"ingress,omitempty" yaml:"ingress,omitempty"`

	// RegistryMirrors and PackageMirror exist for deployments that cannot reach
	// Docker Hub or the distribution's package mirrors — the normal case inside
	// Russia, and the reason reachability is being measured before anything is
	// promised about it (roadmap item sc-ru-market-readiness, B0).
	RegistryMirrors []string `json:"registryMirrors,omitempty" yaml:"registryMirrors,omitempty"`
	PackageMirror   string   `json:"packageMirror,omitempty" yaml:"packageMirror,omitempty"`

	// ExtraSshKeys are additional public keys authorised on the machine. Prefer the
	// cloud's own console for incident access; every key added here is a second,
	// permanent access path (vm-matrix.md §3.3).
	ExtraSshKeys []string `json:"extraSshKeys,omitempty" yaml:"extraSshKeys,omitempty"`

	// Hosts is valid only with DriverByo.
	Hosts []VmHost `json:"hosts,omitempty" yaml:"hosts,omitempty"`

	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *VmConfig) ProviderType() string {
	return ProviderType
}

// ProjectIdValue has no meaning for a machine: the project/folder id lives inside
// the cloud credentials a cloud driver rehydrates, and a byo host has no cloud at
// all. Returning the k8s-style "n/a" keeps `${resource:...projectId}` from
// interpolating to something that looks real.
func (r *VmConfig) ProjectIdValue() string {
	return "n/a"
}

// CredentialsValue returns the resolved `${auth:...}` blob verbatim, and "{}" when
// none was given.
//
// Deliberately NOT yandex.AccountConfig's behaviour of serialising the whole config
// as a fallback: a byo VmConfig carries host private keys, and serialising them into
// a value whose entire purpose is to be handed to other code is how a credential
// ends up somewhere nobody looked. A cloud driver reads its own AccountConfig out of
// this blob with api.ConvertAuth and fails on its own required fields if it is empty.
func (r *VmConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", "{}").Else(r.Credentials.Credentials)
}

func (r *VmConfig) EffectiveSshUser() string {
	return lo.If(r.SshUser == "", DefaultSshUser).Else(r.SshUser)
}

func (r *VmConfig) EffectiveImage() string {
	return lo.If(r.Image == "", DefaultImage).Else(r.Image)
}

// EffectiveSize resolves the machine size, filling in DefaultCores/DefaultMemory for
// whichever half was left out. Disk is left empty when unset: unlike cores and memory
// it is not a flavour filter, and every cloud has a sane per-image root-disk default
// that is better than one SC invents.
func (r *VmConfig) EffectiveSize() VmSize {
	size := VmSize{Cores: DefaultCores, Memory: DefaultMemory}
	if r.Size == nil {
		return size
	}
	if r.Size.Cores > 0 {
		size.Cores = r.Size.Cores
	}
	if r.Size.Memory != "" {
		size.Memory = r.Size.Memory
	}
	size.Disk = r.Size.Disk
	size.DiskType = r.Size.DiskType
	return size
}

// IngressEnabled reports whether Caddy should be installed in front of the stacks
// on this machine.
func (r *VmConfig) IngressEnabled() bool {
	return r.Ingress != nil && r.Ingress.Enable
}

// IngressImage is the Caddy image to run, defaulted. Same expression the Kubernetes
// edge uses (kubernetes/caddy.go), so the two paths run the same build of Caddy with
// the same plugin set unless an operator deliberately pins one of them elsewhere.
func (r *VmConfig) IngressImage() string {
	if r.Ingress != nil && r.Ingress.Image != "" {
		return r.Ingress.Image
	}
	return fmt.Sprintf("simplecontainer/caddy:%s", build.Version)
}

// EffectiveIngressImageDelivery resolves how the Caddy image reaches the machine.
// Defaults to ImageDeliveryRegistry — see the field comment for why the default is
// not the other way round.
func (r *VmConfig) EffectiveIngressImageDelivery() string {
	if r.Ingress != nil && r.Ingress.ImageDelivery != "" {
		return r.Ingress.ImageDelivery
	}
	return ImageDeliveryRegistry
}

// ShipsIngressImage reports whether the provision streams Caddy to the machine from
// the runner instead of letting the machine pull it.
func (r *VmConfig) ShipsIngressImage() bool {
	return r.IngressEnabled() && r.EffectiveIngressImageDelivery() == ImageDeliverySshTar
}

// validateIngress holds the ingress rules shared by the byo and cloud paths. One
// function rather than the same two checks in both, because the pair of them drifted
// once already: `acmeEabKeyId` without `acmeEabMacKey` is accepted by Caddy's parser
// and then fails at first issuance, which surfaces as "no certificate" days later.
func (r *VmConfig) validateIngress() error {
	if !r.IngressEnabled() {
		return nil
	}
	if r.Ingress.Email == "" {
		return errors.Errorf("`ingress.enable: true` requires `ingress.email` (the ACME account address)")
	}
	if d := r.EffectiveIngressImageDelivery(); d != ImageDeliveryRegistry && d != ImageDeliverySshTar {
		return errors.Errorf("unknown `ingress.imageDelivery` %q; one of: %s, %s",
			d, ImageDeliveryRegistry, ImageDeliverySshTar)
	}
	if (r.Ingress.AcmeEabKeyID == "") != (r.Ingress.AcmeEabMacKey == "") {
		return errors.Errorf(
			"`ingress.acmeEabKeyId` and `ingress.acmeEabMacKey` must be set together: External Account " +
				"Binding needs both, and half of it is accepted by the config parser and then fails at the " +
				"first certificate request",
		)
	}
	if r.Ingress.AcmeEabKeyID != "" && r.Ingress.AcmeCa == "" {
		return errors.Errorf(
			"`ingress.acmeEabKeyId` is set without `ingress.acmeCa`: External Account Binding is for a " +
				"private or enterprise CA, and Let's Encrypt (the default) does not accept it",
		)
	}
	return nil
}

// Validate enforces the rule that makes this DSL safe to extend: **either support a
// field or refuse it**. A field accepted and silently ignored is a future incident
// in which the operator is certain the setting applied (compose-on-vm.md §2.2, D4).
//
// It is called from the provisioning tier rather than from ReadVmConfig, because the
// config readers are also driven with an empty config by cmd/schema-gen — a reader
// that rejects an empty config drops the whole resource out of the generated schema.
func (r *VmConfig) Validate() error {
	if r.Driver == "" {
		return errors.Errorf("vm resource requires `provider`; one of: %s", strings.Join(KnownDrivers(), ", "))
	}
	drv, known := Drivers[r.Driver]
	if !known {
		return errors.Errorf("unknown vm provider %q; one of: %s", r.Driver, strings.Join(KnownDrivers(), ", "))
	}

	var err error
	if r.Driver == DriverByo {
		err = r.validateByo()
	} else {
		err = r.validateCloud()
	}
	if err != nil {
		return err
	}

	// Checked last on purpose: a config that is wrong in shape should hear about the
	// shape first, and a config that is right should hear exactly what is missing.
	if !drv.Implemented {
		return errors.Errorf(
			"vm provider %q is not implemented yet (roadmap item %s); until it lands, create the machine "+
				"by hand and declare it with `provider: %s` and `hosts:`",
			r.Driver, drv.RoadmapItem, DriverByo,
		)
	}
	return nil
}

func (r *VmConfig) validateByo() error {
	// Fields that only mean something when SC creates the machine. Accepting them
	// here would read as "SC will resize/move your machine", which it will not.
	var ignored []string
	if r.Region != "" {
		ignored = append(ignored, "region")
	}
	if len(r.Zones) > 0 {
		ignored = append(ignored, "zones")
	}
	if r.Image != "" {
		ignored = append(ignored, "image")
	}
	if r.Size != nil {
		ignored = append(ignored, "size")
	}
	if r.PublicIP {
		ignored = append(ignored, "publicIP")
	}
	if r.Network != nil {
		ignored = append(ignored, "network")
	}
	if len(r.ExtraSshKeys) > 0 {
		ignored = append(ignored, "extraSshKeys")
	}
	if len(ignored) > 0 {
		return errors.Errorf(
			"%s is not valid with `provider: %s`: SC does not create the machine, so it cannot apply %s; "+
				"configure it in the cloud console instead",
			strings.Join(quoteAll(ignored), ", "), DriverByo, lo.If(len(ignored) == 1, "it").Else("them"),
		)
	}

	if len(r.Hosts) == 0 {
		return errors.Errorf("`provider: %s` requires at least one entry under `hosts`", DriverByo)
	}
	// One machine is all P5 (compose on a single VM) can place a stack on, and a
	// multi-host byo pool only becomes meaningful with k3s. Refusing now is cheaper
	// than deploying to the first host and leaving the rest idle.
	if len(r.Hosts) > 1 {
		return errors.Errorf(
			"`provider: %s` currently supports exactly one host, got %d; multi-host pools arrive with the "+
				"k3s cluster resource (roadmap item sc-k3s-cluster-resource)", DriverByo, len(r.Hosts),
		)
	}
	for i, h := range r.Hosts {
		if h.Address == "" {
			return errors.Errorf("hosts[%d] requires `address`", i)
		}
		if h.PrivateKey == "" {
			return errors.Errorf(
				"hosts[%d] (%s) requires `privateKey` — write it as `${secret:<name>}`; password and "+
					"ssh-agent authentication are not supported", i, h.Address,
			)
		}
		if h.Port < 0 || h.Port > 65535 {
			return errors.Errorf("hosts[%d] (%s) has an invalid `port`: %d", i, h.Address, h.Port)
		}
	}
	return r.validateIngress()
}

// validateCloud holds the invariants of every driver that creates a machine. They
// live here, once, rather than being rediscovered per cloud — the first driver
// (`cloudru-vm`) is written against exactly these.
func (r *VmConfig) validateCloud() error {
	if len(r.Hosts) > 0 {
		return errors.Errorf(
			"`hosts` is only valid with `provider: %s`; with `provider: %s` SC creates the machine itself",
			DriverByo, r.Driver,
		)
	}
	if r.Region == "" {
		return errors.Errorf("`provider: %s` requires `region`", r.Driver)
	}
	// A machine with no public address is a machine SC cannot deploy to. Every
	// delivery step — readiness, image ship, compose up, the Caddy reload — is an
	// outbound SSH connection from wherever `sc deploy` runs, and SC has no bastion,
	// no VPN and no in-VPC runner. Refused here rather than discovered as a dial
	// timeout against a machine that was created and billed successfully.
	if !r.PublicIP {
		return errors.Errorf(
			"`provider: %s` requires `publicIP: true`: SC deploys over SSH from outside the cloud and has no "+
				"bastion or VPN path to a machine without a public address", r.Driver)
	}
	// One zone, because one machine. Several zones only mean something once a vm
	// resource can describe a pool, which arrives with the k3s cluster resource.
	if len(r.Zones) > 1 {
		return errors.Errorf(
			"`provider: %s` currently supports at most one entry under `zones`, got %d; multi-zone pools "+
				"arrive with the k3s cluster resource (roadmap item sc-k3s-cluster-resource)",
			r.Driver, len(r.Zones))
	}
	// `network.id` without the matching subnet is the one shape of a BYO network that
	// cannot be completed by guessing: a driver that picked "some subnet in the VPC"
	// would place the machine in a zone, or behind a route table, the operator did not
	// choose. Caught here because the driver only learns the zone later.
	if r.Network != nil && r.Network.ID != "" && len(r.Network.Subnets) == 0 {
		return errors.Errorf(
			"`network.id` is set but `network.subnets` is empty: SC needs the subnet to place the machine in, " +
				"as `subnets: {<zone>: <subnet-id>}`")
	}
	if r.Network != nil && r.Network.ID == "" && len(r.Network.Subnets) > 0 {
		return errors.Errorf("`network.subnets` is set but `network.id` is empty: give both, or neither")
	}
	return r.validateIngress()
}

// KnownDrivers lists every `provider:` value a vm resource accepts, implemented or
// not, so an error message can name them all.
func KnownDrivers() []string {
	names := lo.Keys(Drivers)
	sort.Strings(names)
	return names
}

func quoteAll(in []string) []string {
	return lo.Map(in, func(s string, _ int) string { return fmt.Sprintf("%q", s) })
}

func ReadVmConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &VmConfig{})
}
