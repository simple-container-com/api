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

	// DriverCloudruEvolution creates machines in Cloud.ru **Evolution**, which is a
	// different cloud from DriverCloudru's Cloud.ru **Advanced** (ex-SberCloud)
	// despite the shared brand and console: different API, different credentials
	// (a service-account key pair, not AK/SK), different regions, and no Terraform
	// provider with open sources. Two drivers rather than one `cloudru` with a mode
	// flag, because an account generally has one estate and not the other — the
	// operator's own project has only Evolution, which is why this driver exists.
	DriverCloudruEvolution = "cloudru-evolution-vm"
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
	// AcmeStorage points every machine of this resource at ONE shared certificate
	// store. Leave it unset for a single machine; set it for a fleet.
	//
	// On a fleet it is not a tuning knob, it is the thing that makes HTTPS possible.
	// SC's Caddy image carries no `dns.*` provider, so HTTP-01 is the only ACME
	// challenge available, and a name with N A records sends the CA's validator to an
	// arbitrary machine — which 404s a token a sibling minted. certmagic's distributed
	// solver shares that challenge state through the storage module, so one store turns
	// N machines into one ACME client holding one certificate instead of N. Without it
	// a fleet either serves plain HTTP on the origin (see `domainProxied`) or walks
	// into a rate-limit lockout.
	AcmeStorage *VmAcmeStorage `json:"acmeStorage,omitempty" yaml:"acmeStorage,omitempty"`
}

// VmAcmeStorage is an S3-compatible bucket holding the fleet's certificates and ACME
// challenge state, rendered as Caddy's `storage s3` global option.
//
// S3 and not one module per cloud. The image also carries `caddy.storage.gcs`, but GCS
// serves an S3-compatible interop endpoint and `gcp-bucket` already exports HMAC
// credentials for it, so one code path covers Yandex, AWS, GCP, minio and anything
// else that speaks S3. The GCS module stays what the Kubernetes edge uses. There is
// deliberately no `type:` field: one legal value is not a choice, and adding the field
// later with a default of `s3` stays backwards compatible.
//
// Every field is a property of the RESOURCE, not of a machine — the whole point is
// that all machines write to the same place.
type VmAcmeStorage struct {
	// Bucket is required. It must already exist: SC does not create it here, because
	// the bucket for a fleet generally lives in the same parent stack (a `yc-bucket` /
	// `gcp-bucket` resource) or on an operator-run minio, and inventing a second
	// owner for it would make a `sc destroy` able to take the certificates with it.
	Bucket string `json:"bucket" yaml:"bucket"`
	// Endpoint is the store's base URL, scheme included
	// (`https://storage.yandexcloud.net`, `http://10.0.0.4:9000`). Empty means real
	// AWS S3.
	//
	// Setting it also switches the SDK to computing request checksums only when
	// required, which is what makes most non-AWS implementations work at all.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// Region is required when Endpoint is empty, since that is real AWS and the wrong
	// region is a failed request rather than a redirect. With an Endpoint it defaults
	// to `us-east-1`, which S3-compatible stores conventionally ignore.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
	// AccessKey / SecretKey are the static credential, both or neither.
	//
	// Neither means the AWS SDK's default chain — an EC2 instance role, in practice.
	// That works on AWS and nowhere else: a VPS has no instance metadata, so on
	// Cloud.ru, MWS or byo, omitting them is a silent fall-through to "no credentials"
	// whose only symptom is certificates that never issue. SecretKey is a credential:
	// write it as `${secret:...}`. Neither value reaches the rendered Caddyfile — both
	// are delivered in the ingress env file (0600, over stdin) and referenced as
	// `{$...}`, the same discipline as the EAB key above.
	AccessKey string `json:"accessKey,omitempty" yaml:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty" yaml:"secretKey,omitempty"`
	// Prefix is the key prefix inside the bucket. Defaults to `acme`.
	//
	// Worth setting when one bucket serves several fleets: two fleets sharing a prefix
	// share locks and certificates, which is correct only if they also share every
	// domain name.
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	// PathStyle selects path-style addressing (`<endpoint>/<bucket>/<key>`) over
	// virtual-host style (`<bucket>.<endpoint>/<key>`). Defaults to TRUE whenever
	// Endpoint is set and false otherwise.
	//
	// That default is the useful one: virtual-host addressing needs the bucket name to
	// resolve as a DNS label under the endpoint, which minio does not serve at all and
	// several S3-compatible clouds serve only for some bucket names. Real AWS prefers
	// virtual-host. Set it explicitly to override.
	PathStyle *bool `json:"pathStyle,omitempty" yaml:"pathStyle,omitempty"`
	// Insecure disables TLS verification against the store. For a minio with a
	// self-signed certificate, and nothing else — the credential and every private key
	// the fleet owns travel over this connection. Prefer plain `http://` on a private
	// network, which is at least honest about what it is.
	Insecure bool `json:"insecure,omitempty" yaml:"insecure,omitempty"`
	// EncryptionKey encrypts the stored objects with secretbox. Exactly 32 bytes when
	// set; a credential, so write it as `${secret:...}`.
	//
	// Unset means the module logs `Clear text certificate storage active` and stores
	// the fleet's private keys as they are. Acceptable when the bucket is private AND
	// the cloud encrypts at rest; on a minio running on one of the fleet's own
	// machines it is neither, so set it there.
	EncryptionKey string `json:"encryptionKey,omitempty" yaml:"encryptionKey,omitempty"`
}

// AcmeStorageEnabled reports whether this resource has a shared certificate store.
func (r *VmConfig) AcmeStorageEnabled() bool {
	return r.IngressEnabled() && r.Ingress.AcmeStorage != nil && r.Ingress.AcmeStorage.Bucket != ""
}

// UsePathStyle resolves PathStyle's default: on whenever there is an endpoint.
func (s *VmAcmeStorage) UsePathStyle() bool {
	if s.PathStyle != nil {
		return *s.PathStyle
	}
	return s.Endpoint != ""
}

// StoragePrefix resolves Prefix's default. Rendered explicitly rather than left to the
// module, so the generated Caddyfile says where the fleet's certificates actually are.
func (s *VmAcmeStorage) StoragePrefix() string {
	if s.Prefix != "" {
		return s.Prefix
	}
	return DefaultAcmeStoragePrefix
}

// StorageRegion resolves Region's default, which only applies when there is an endpoint
// — validateIngress refuses an empty region without one.
func (s *VmAcmeStorage) StorageRegion() string {
	if s.Region != "" {
		return s.Region
	}
	return DefaultAcmeStorageRegion
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

	// IngressPorts are additional ports SC opens in the machine's firewall, on top
	// of SSH (and 80/443 when `ingress.enable` is on) and on top of what SC infers
	// from this vm's consumers in the same parent stack. See VmIngressPort and
	// FirewallPorts (ports.go).
	IngressPorts []VmIngressPort `json:"ingressPorts,omitempty" yaml:"ingressPorts,omitempty"`

	// AllowUnverifiedProvider opts into a driver whose code exists but has never
	// provisioned a machine on the cloud it addresses (see DriverInfo.Unverified).
	// Refused when the driver IS verified, so the flag cannot linger as dead config
	// reading like a standing exception.
	AllowUnverifiedProvider bool `json:"allowUnverifiedProvider,omitempty" yaml:"allowUnverifiedProvider,omitempty"`

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

	// HostKey pins the SSH host key of a machine SC created but could not give an
	// identity to, as one authorized_keys line. Valid only with a driver whose cloud
	// has no cloud-init (today: DriverCloudruEvolution) — see validateCloud.
	//
	// It exists because of a cloud that accepts no user-data at all: SC cannot
	// pre-seed a host key there, so the FIRST connection has nothing to verify and
	// trust-on-first-use is the only thing left. That is a real exposure (the deploy
	// key and every secret of every stack on the machine ride that connection), and
	// an operator who reads the key off the cloud's console can close it by pasting
	// it here on the next provision. Same semantics and same format as
	// `hosts[].hostKey` — `ssh-keyscan -t ed25519 <address>` output pastes verbatim.
	HostKey string `json:"hostKey,omitempty" yaml:"hostKey,omitempty"`

	// Hosts is valid only with DriverByo.
	Hosts []VmHost `json:"hosts,omitempty" yaml:"hosts,omitempty"`

	api.Credentials `json:",inline" yaml:",inline"`
}

func (r *VmConfig) ProviderType() string {
	return ProviderType
}

// DriverType implements api.VmConfigAware so that the parse-time plugin
// requirement walker can reach a vm's `provider:` string. The driver — not
// just the resource's `type: vm` — is what selects which plugin is needed
// (cloudru-evolution-vm, future mws-vm, …).
func (r *VmConfig) DriverType() string {
	return r.Driver
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
	return r.validateAcmeStorage()
}

// validateAcmeStorage checks the shared certificate store.
//
// Every refusal here replaces a failure that is otherwise invisible until a certificate
// does not renew, which is weeks after the deploy that caused it and on a machine nobody
// is watching. The store is only consulted on issuance and renewal, so a misconfigured
// one serves perfectly from cache until it matters.
func (r *VmConfig) validateAcmeStorage() error {
	s := r.Ingress.AcmeStorage
	if s == nil {
		return nil
	}
	if s.Bucket == "" {
		return errors.Errorf(
			"`ingress.acmeStorage` is set without `bucket`: the shared certificate store needs a bucket " +
				"that already exists (SC does not create it — see the field docs)",
		)
	}
	if (s.AccessKey == "") != (s.SecretKey == "") {
		return errors.Errorf(
			"`ingress.acmeStorage.accessKey` and `.secretKey` must be set together: the module uses the " +
				"static credential only when it has both, and otherwise falls through to the AWS SDK's " +
				"default chain — which on a VPS finds nothing and presents as certificates that never issue",
		)
	}
	if s.Endpoint == "" && s.Region == "" {
		return errors.Errorf(
			"`ingress.acmeStorage` has neither `endpoint` nor `region`: no endpoint means real AWS S3, "+
				"where the wrong region is a failed request rather than a redirect. Set `region`, or set "+
				"`endpoint` (then `region` defaults to %q, which S3-compatible stores ignore)",
			DefaultAcmeStorageRegion,
		)
	}
	if s.Endpoint != "" && !strings.HasPrefix(s.Endpoint, "http://") && !strings.HasPrefix(s.Endpoint, "https://") {
		return errors.Errorf(
			"`ingress.acmeStorage.endpoint` %q has no scheme: it is used as the SDK's base endpoint and "+
				"must be a URL (`https://storage.yandexcloud.net`, `http://10.0.0.4:9000`)",
			s.Endpoint,
		)
	}
	if k := s.EncryptionKey; k != "" && len(k) != AcmeStorageEncryptionKeyLen {
		// Checked here even though the module checks it too, because the module's
		// check lands in two different places depending on how the value arrives, and
		// SC always makes it arrive the harder way: a literal fails Caddyfile parsing
		// (visible, the deploy fails), but SC delivers this through the env file as a
		// `{$...}` placeholder, and a substituted wrong-length key reaches `Provision`
		// — which returns an error that is only in `docker logs sc-caddy`.
		return errors.Errorf(
			"`ingress.acmeStorage.encryptionKey` is %d bytes; secretbox needs exactly %d. Leave it unset "+
				"to store the fleet's private keys unencrypted (acceptable only when the bucket is private "+
				"and the cloud encrypts at rest)",
			len(k), AcmeStorageEncryptionKeyLen,
		)
	}
	if s.Insecure && strings.HasPrefix(s.Endpoint, "http://") {
		return errors.Errorf(
			"`ingress.acmeStorage.insecure: true` with an `http://` endpoint: there is no TLS to skip " +
				"verifying. Drop `insecure`",
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
	// Built-in table first, then the drivers the loaded plugins advertised — see
	// plugin_drivers.go for why those live in a second map and why they are already
	// registered by the time any Validate runs.
	drv, known := LookupDriver(r.Driver)
	if !known {
		return errors.Errorf("unknown vm provider %q; one of: %s", r.Driver, strings.Join(KnownDrivers(), ", "))
	}

	var err error
	if r.Driver == DriverByo {
		err = r.validateByo()
	} else {
		err = r.validateCloud(drv)
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
	// An unverified driver is refused by default, and the opt-in is refused on a
	// driver that does not need it. Both halves matter: "implemented" and "known to
	// work" are different claims, and until this gate existed `main` offered a
	// Cloud.ru Advanced driver indistinguishable in the DSL from the Evolution one
	// that had actually been provisioned end to end.
	switch {
	case drv.Unverified != "" && !r.AllowUnverifiedProvider:
		return errors.Errorf(
			"vm provider %q has never provisioned a machine on the cloud it addresses: %s. Set "+
				"`allowUnverifiedProvider: true` to try it anyway and expect to debug the driver; progress "+
				"is tracked on roadmap item %s",
			r.Driver, drv.Unverified, drv.RoadmapItem,
		)
	case drv.Unverified == "" && r.AllowUnverifiedProvider:
		return errors.Errorf(
			"`allowUnverifiedProvider` is not valid with `provider: %s`: that driver is live-verified, so the "+
				"flag would read as a standing exception to a gate it is not subject to. Remove it", r.Driver,
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
	// SC owns no firewall on a machine it did not create: there is no cloud API to
	// call and no security group to find. Accepting the list would be the exact
	// failure this field was added to fix, one layer up — an operator certain the
	// NodePort range is open, and a deploy that reports success and times out.
	if len(r.IngressPorts) > 0 {
		ignored = append(ignored, "ingressPorts")
	}
	if len(ignored) > 0 {
		return errors.Errorf(
			"%s is not valid with `provider: %s`: SC does not create the machine, so it cannot apply %s; "+
				"configure it in the cloud console instead",
			strings.Join(quoteAll(ignored), ", "), DriverByo, lo.If(len(ignored) == 1, "it").Else("them"),
		)
	}
	// Refused rather than merged with hosts[].hostKey: with byo the host key already
	// has a per-host spelling, and accepting a second one would make whichever lost
	// look applied. A pool of byo hosts does not share a host key.
	if r.HostKey != "" {
		return errors.Errorf(
			"`hostKey` is not valid with `provider: %s`: pin it per machine, as `hosts[].hostKey`", DriverByo,
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
func (r *VmConfig) validateCloud(drv DriverInfo) error {
	// `hostKey` is accepted by exactly the clouds that cannot be given one, and the
	// condition is the cloud-init capability rather than a driver name: seeding a
	// machine's host key means writing it into user-data (see
	// pkg/clouds/pulumi/vm/cloudru.go), so a cloud that runs cloud-init gets an
	// SC-generated identity and a pasted key would be dead config — while a cloud
	// that accepts no user-data at all has no other way to ever be verified.
	if r.HostKey != "" && drv.Capabilities.CloudInit {
		return errors.Errorf(
			"`hostKey` is not valid with `provider: %s`: this cloud runs cloud-init, so SC generates the "+
				"machine's host key itself and pins it — a pasted key would never be used", r.Driver,
		)
	}
	if len(r.Hosts) > 0 {
		return errors.Errorf(
			"`hosts` is only valid with `provider: %s`; with `provider: %s` SC creates the machine itself",
			DriverByo, r.Driver,
		)
	}
	// The mirror of the `hostKey` rule above, and keyed off the same capability for
	// the same reason. Extra keys are authorised by writing them into user-data
	// (cloudinit.go: authorized = deploy key + ExtraSshKeys), so on a cloud that
	// accepts none there is nowhere to put them — the create call takes exactly one
	// public key, the deploy key SC generates. Accepted and dropped, this would be
	// the worst kind of dead config: an operator certain they can still reach the
	// machine during an incident, finding out in the incident that they cannot.
	if len(r.ExtraSshKeys) > 0 && !drv.Capabilities.CloudInit {
		return errors.Errorf(
			"`extraSshKeys` is not valid with `provider: %s`: this cloud runs no cloud-init and its VM create "+
				"takes exactly one public key (the deploy key SC generates), so there is nowhere to authorise "+
				"another one. Add the key to the machine yourself, or keep console access for incidents",
			r.Driver,
		)
	}
	// Required on a region-scoped cloud, and REFUSED on one that has no regions —
	// not merely optional there. A `region:` that SC silently drops would read as
	// placement the operator controlled, and this package treats an ignored field
	// as a future incident. Keyed off the capability rather than the driver name so
	// the next regionless cloud needs no change here.
	switch {
	case drv.Capabilities.Regionless && r.Region != "":
		return errors.Errorf(
			"`region` is not valid with `provider: %s`: this cloud's API has no region — one global endpoint, "+
				"and placement is the availability zone. Remove `region`, and pin placement with `zones:` "+
				"(at most one entry)", r.Driver,
		)
	case !drv.Capabilities.Regionless && r.Region == "":
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
				"bastion or VPN path to a machine without a public address", r.Driver,
		)
	}
	// One zone, because one machine. Several zones only mean something once a vm
	// resource can describe a pool, which arrives with the k3s cluster resource.
	if len(r.Zones) > 1 {
		return errors.Errorf(
			"`provider: %s` currently supports at most one entry under `zones`, got %d; multi-zone pools "+
				"arrive with the k3s cluster resource (roadmap item sc-k3s-cluster-resource)",
			r.Driver, len(r.Zones),
		)
	}
	// `network.id` without the matching subnet is the one shape of a BYO network that
	// cannot be completed by guessing: a driver that picked "some subnet in the VPC"
	// would place the machine in a zone, or behind a route table, the operator did not
	// choose. Caught here because the driver only learns the zone later.
	if r.Network != nil && r.Network.ID != "" && len(r.Network.Subnets) == 0 {
		return errors.Errorf(
			"`network.id` is set but `network.subnets` is empty: SC needs the subnet to place the machine in, " +
				"as `subnets: {<zone>: <subnet-id>}`",
		)
	}
	if r.Network != nil && r.Network.ID == "" && len(r.Network.Subnets) > 0 {
		return errors.Errorf("`network.subnets` is set but `network.id` is empty: give both, or neither")
	}
	// Keyed off the capability, like every other refusal here: a cloud whose
	// filtering SC cannot write to has nowhere to put these, and a list of ports
	// that is accepted and never opened is worse than no field at all.
	if len(r.IngressPorts) > 0 && !drv.Capabilities.SecurityGroups {
		return errors.Errorf(
			"`ingressPorts` is not valid with `provider: %s`: SC does not manage this cloud's packet filtering, "+
				"so it cannot open them. Open the ports in the cloud's console", r.Driver,
		)
	}
	// Shape of each entry, checked through the same function the drivers resolve the
	// firewall with, so a config that validates is a config that expands.
	if _, err := r.FirewallPorts(); err != nil {
		return err
	}
	return r.validateIngress()
}

// KnownDrivers lists every `provider:` value a vm resource accepts, implemented or
// not, so an error message can name them all.
//
// The union of the built-in table and the drivers the loaded plugins advertised. A
// plugin driver that is installed but whose name the operator mistyped has to
// appear here, or the message tells them to use a list that does not contain the
// driver they just installed.
func KnownDrivers() []string {
	names := append(lo.Keys(Drivers), pluginDriverNames()...)
	sort.Strings(names)
	return names
}

func quoteAll(in []string) []string {
	return lo.Map(in, func(s string, _ int) string { return fmt.Sprintf("%q", s) })
}

func ReadVmConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &VmConfig{})
}
