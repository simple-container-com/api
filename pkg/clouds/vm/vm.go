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

// IngressEnabled reports whether Caddy should be installed in front of the stacks
// on this machine.
func (r *VmConfig) IngressEnabled() bool {
	return r.Ingress != nil && r.Ingress.Enable
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
			r.Driver, drv.RoadmapItem, DriverByo)
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
			strings.Join(quoteAll(ignored), ", "), DriverByo, lo.If(len(ignored) == 1, "it").Else("them"))
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
				"k3s cluster resource (roadmap item sc-k3s-cluster-resource)", DriverByo, len(r.Hosts))
	}
	for i, h := range r.Hosts {
		if h.Address == "" {
			return errors.Errorf("hosts[%d] requires `address`", i)
		}
		if h.PrivateKey == "" {
			return errors.Errorf(
				"hosts[%d] (%s) requires `privateKey` — write it as `${secret:<name>}`; password and "+
					"ssh-agent authentication are not supported", i, h.Address)
		}
		if h.Port < 0 || h.Port > 65535 {
			return errors.Errorf("hosts[%d] (%s) has an invalid `port`: %d", i, h.Address, h.Port)
		}
	}
	if r.IngressEnabled() && r.Ingress.Email == "" {
		return errors.Errorf("`ingress.enable: true` requires `ingress.email` (the ACME account address)")
	}
	return nil
}

// validateCloud holds the invariants of every driver that creates a machine. No such
// driver is implemented yet — Validate refuses them after this runs — but the rules
// are the contract the first one is written against, so they live here rather than
// being rediscovered per cloud.
func (r *VmConfig) validateCloud() error {
	if len(r.Hosts) > 0 {
		return errors.Errorf(
			"`hosts` is only valid with `provider: %s`; with `provider: %s` SC creates the machine itself",
			DriverByo, r.Driver)
	}
	if r.Region == "" {
		return errors.Errorf("`provider: %s` requires `region`", r.Driver)
	}
	if r.IngressEnabled() && r.Ingress.Email == "" {
		return errors.Errorf("`ingress.enable: true` requires `ingress.email` (the ACME account address)")
	}
	return nil
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
