// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
)

// The SC-managed certificate store: the thing `ingress.acmeStore.enable: true` stands
// up on the FIRST machine of the fleet so that `ingress.acmeStorage` has a bucket to
// point at on a hoster that sells none.
//
// Why it exists at all. A fleet needs one shared certificate store or it has no origin
// HTTPS (see VmIngress.AcmeStorage for the mechanism). On Yandex and GCP the bucket is
// a resource in the same parent stack and `acmeStorage` is the whole answer. On
// Cloud.ru, MWS and byo there is no bucket to buy, and the operator's only remaining
// option was to run an S3 server themselves — which is the one piece of a fleet SC was
// asking a human to assemble by hand, including its credentials.
const (
	// AcmeStoreProject / AcmeStoreContainer: one per fleet, on host 0, in its own
	// compose project for the same reason the ingress has one — it belongs to the
	// RESOURCE and must survive a `docker compose down` of any stack.
	AcmeStoreProject   = "sc-acme-store"
	AcmeStoreContainer = "sc-acme-store"
	// AcmeStoreDataVolume holds the fleet's certificates and its ACME account key.
	//
	// External, like CaddyDataVolume and for the same reason, except that here the
	// stake is the whole fleet rather than one machine: `docker compose -p
	// sc-acme-store down -v` must not be able to delete the store every node reads
	// from. Losing it is a Let's Encrypt rate-limit event (50 certificates per
	// registered domain per week) and the fleet serves unencrypted until it clears.
	AcmeStoreDataVolume = "sc-acme-store-data"
	// AcmeStoreIdentity is the single S3 identity in the generated config. One
	// identity, used by every node: this store exists to be shared, and a per-node
	// credential would buy nothing (they already share the bucket, the prefix, the
	// locks and the certificates) at the cost of N secrets to rotate.
	AcmeStoreIdentity = "acme"

	// DefaultAcmeStoreImage is SeaweedFS, pinned.
	//
	// SeaweedFS and not minio, and the reason is licensing rather than taste: minio's
	// server is AGPL-3.0, and SC would be shipping it as the default component of a
	// commercial platform's fleet. SeaweedFS is Apache-2.0. It is also one static
	// binary with an S3 gateway built in (`weed server -s3`), which is the whole of
	// what is needed here — this store holds a few kilobytes of certificates and a
	// lock file, not objects.
	//
	// Pinned to an exact tag, not `latest`, because this is a server every node's TLS
	// depends on and a silent upgrade underneath a running fleet is not something an
	// operator asked for. 4.50 is the version the 3-VPS fleet stand ran
	// (digest sha256:b2e5156274…, amd64); bump it deliberately, with a smoke.
	DefaultAcmeStoreImage = "chrislusf/seaweedfs:4.50"
	// DefaultAcmeStorePort is SeaweedFS's conventional S3 port.
	DefaultAcmeStorePort = 8333
	// DefaultAcmeStoreBucket is the bucket SC creates in it.
	DefaultAcmeStoreBucket = "sc-acme"
	// acmeStoreVolumeSizeLimitMB caps a SeaweedFS volume file. The default is 30 GB,
	// pre-allocated-ish, for a store whose entire contents are a handful of PEMs.
	acmeStoreVolumeSizeLimitMB = 128

	// AcmeStoreManagedCredential is what the config tier puts in the synthesized
	// AcmeStorage's credential fields.
	//
	// It is a SENTINEL, never a value, and nothing ever sends it anywhere: the
	// Caddyfile renderer only tests these fields for emptiness and then writes
	// `{$SC_ACME_S3_ACCESS_KEY}` placeholders, so what matters is that they are
	// non-empty and that a reader of a dumped config can tell this store's credential
	// is generated rather than configured. The real values are created at provision
	// time (pkg/clouds/pulumi/vm/acme_store.go) and delivered only in the two 0600
	// files that need them: the ingress env file on every node, and the store's own
	// s3 config on host 0.
	AcmeStoreManagedCredential = "<managed-by-sc>"
)

// VmAcmeStore asks SC to run the fleet's certificate store itself, on the first
// machine, and to point every node's Caddy at it.
//
// Mutually exclusive with `ingress.acmeStorage`: that field names a store someone else
// owns, this one makes SC the owner, and a resource that said both would be declaring
// two stores and using one. The refusal says which to delete.
//
// What it is NOT: highly available. One container on one machine. Losing host 0 does
// not take the fleet down — every node keeps serving from its own cached certificates —
// but it stops RENEWALS, and that is a failure that surfaces up to 60 days later on a
// machine nobody is watching. It is the right trade for a hoster with no bucket, and
// the wrong one where a managed bucket exists; `acmeStorage` is cheaper and safer
// wherever it is available.
type VmAcmeStore struct {
	// Enable turns the store on. Nothing else here is required.
	Enable bool `json:"enable" yaml:"enable"`
	// Image overrides DefaultAcmeStoreImage — to pin a different version, or to point
	// at a mirror on a machine with no route to Docker Hub.
	Image string `json:"image,omitempty" yaml:"image,omitempty"`
	// Port is the S3 port the store listens on, on the mesh address only. Overriding
	// it is for a machine where something else already holds 8333.
	Port int `json:"port,omitempty" yaml:"port,omitempty"`
	// Bucket is the bucket SC creates inside the store. Defaults to `sc-acme`.
	//
	// Worth setting only when one store serves several fleets — and note that two
	// fleets sharing a bucket AND a prefix share locks and certificates, which is
	// correct only if they also share every domain name.
	Bucket string `json:"bucket,omitempty" yaml:"bucket,omitempty"`
}

// acmeStore is the declared block or nil. A helper rather than `r.Ingress.AcmeStore` at
// each use, because `ingress` itself is a pointer and is absent on most vm resources —
// and Validate runs against a half-filled config by design (cmd/schema-gen drives these
// readers with an empty one).
func (r *VmConfig) acmeStore() *VmAcmeStore {
	if r == nil || r.Ingress == nil {
		return nil
	}
	return r.Ingress.AcmeStore
}

// AcmeStoreEnabled reports whether SC stands up this resource's certificate store.
//
// The helper is consulted FIRST and the order is not cosmetic: `s != nil` already
// implies a non-nil receiver with a non-nil `ingress`, whereas IngressEnabled nil-guards
// neither and panics on a nil config. Written the other way round, this accessor's own
// nil-safety claim was half-true.
func (r *VmConfig) AcmeStoreEnabled() bool {
	s := r.acmeStore()
	return s != nil && s.Enable && r.IngressEnabled()
}

// AcmeStoreImage resolves the store's image.
func (r *VmConfig) AcmeStoreImage() string {
	if s := r.acmeStore(); s != nil && s.Image != "" {
		return s.Image
	}
	return DefaultAcmeStoreImage
}

// AcmeStorePort resolves the store's S3 port.
func (r *VmConfig) AcmeStorePort() int {
	if s := r.acmeStore(); s != nil && s.Port != 0 {
		return s.Port
	}
	return DefaultAcmeStorePort
}

// AcmeStoreBucket resolves the bucket SC creates in the store.
func (r *VmConfig) AcmeStoreBucket() string {
	if s := r.acmeStore(); s != nil && s.Bucket != "" {
		return s.Bucket
	}
	return DefaultAcmeStoreBucket
}

// AcmeStoreHost is the address the OTHER machines dial the store on — host 0's address
// on whatever node-to-node network this fleet has.
//
// The mesh when there is one, else host 0's `privateAddress`. Nothing else, and in
// particular never host 0's public address: the credential this store accepts is
// logged in cleartext by certmagic-s3 on every node that uses it
// (`sc-certmagic-s3-creds-in-container-logs`), so a store reachable from the internet
// is a store whose credential is one `docker logs` away from a bucket holding every
// private key the fleet owns. The firewall hole SC opens for the mesh is a WireGuard
// port, which answers an unauthenticated packet with silence; there is deliberately no
// hole for this port at all.
//
// Returns an error rather than falling back, because every fallback available here is
// the wrong one.
func (r *VmConfig) AcmeStoreHost() (string, error) {
	if r.MeshEnabled() {
		return r.MeshAddress(0)
	}
	if len(r.Hosts) > 0 && r.Hosts[0].PrivateAddress != "" {
		return r.Hosts[0].PrivateAddress, nil
	}
	return "", errors.Errorf(
		"`ingress.acmeStore.enable: true` needs an address the other machines can reach the first one " +
			"on. Either set `mesh.enable: true` and SC builds one (a WireGuard overlay across this " +
			"resource's machines), or give `hosts[0].privateAddress` the address of host 0 on a network " +
			"the fleet already shares. SC will not publish this store on a public address: the module " +
			"that reads it logs its credentials in the clear on every node",
	)
}

// AcmeStoreEndpoint is the S3 base URL every node's Caddy points at.
//
// Plain `http://`, not https. The store runs on a private overlay, and the honest
// options on it are an unencrypted hop inside a WireGuard tunnel or a self-signed
// certificate plus `insecure: true` — which is TLS whose verification is switched off,
// i.e. the same security with more moving parts and a worse name. On a mesh the hop is
// encrypted by WireGuard itself. On a `privateAddress` network it is as private as the
// operator's own network, which is the same assumption everything else in the fleet
// already makes about that address.
func (r *VmConfig) AcmeStoreEndpoint() (string, error) {
	host, err := r.AcmeStoreHost()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:%d", host, r.AcmeStorePort()), nil
}

// ManagedAcmeStorage synthesizes the `acmeStorage` struct that the managed store
// implies, so that the Caddyfile renderer has exactly one input shape to deal with
// whether the bucket is Yandex's, GCP's, an operator's minio or SC's own container.
//
// The credential fields carry AcmeStoreManagedCredential rather than real values. See
// that constant: the renderer only tests them for emptiness before writing `{$VAR}`
// placeholders, and the real values exist only at provision time.
func (r *VmConfig) ManagedAcmeStorage() (*VmAcmeStorage, error) {
	endpoint, err := r.AcmeStoreEndpoint()
	if err != nil {
		return nil, err
	}
	pathStyle := true
	return &VmAcmeStorage{
		Bucket:   r.AcmeStoreBucket(),
		Endpoint: endpoint,
		// Not read by anything — an S3-compatible store ignores it — but rendered
		// explicitly, like every other field, so the generated Caddyfile is readable
		// without knowing the module's defaults.
		Region: caddyassets.DefaultAcmeStorageRegion,
		// Path style, and not a default anyone has to reason about: SeaweedFS serves
		// no `<bucket>.<host>` DNS label, and the host here is a bare IPv4 address,
		// which cannot have one.
		PathStyle:     &pathStyle,
		AccessKey:     AcmeStoreManagedCredential,
		SecretKey:     AcmeStoreManagedCredential,
		EncryptionKey: AcmeStoreManagedCredential,
	}, nil
}

// EffectiveAcmeStorage is the one shared certificate store of this resource, whichever
// way it was declared: the operator's own `acmeStorage`, or the one SC stands up. nil
// means this resource has none, which is correct for a single machine.
//
// Validate refuses both at once, so the order here is not a precedence.
func (r *VmConfig) EffectiveAcmeStorage() (*VmAcmeStorage, error) {
	if !r.IngressEnabled() {
		return nil, nil
	}
	if r.Ingress.AcmeStorage.Enabled() {
		return r.Ingress.AcmeStorage, nil
	}
	if r.AcmeStoreEnabled() {
		return r.ManagedAcmeStorage()
	}
	return nil, nil
}

// validateAcmeStore checks the managed store. Called from both validateByo and
// validateCloud.
func (r *VmConfig) validateAcmeStore() error {
	s := r.acmeStore()
	if s == nil {
		return nil
	}
	if !s.Enable {
		// A block that only turns itself off is a block somebody wrote and then
		// disabled; refusing the fields rather than ignoring them is the same rule
		// `mesh` follows, for the same reason — a setting SC silently does not read is
		// worse than one it refuses.
		if s.Image != "" || s.Port != 0 || s.Bucket != "" {
			return errors.Errorf(
				"`ingress.acmeStore` is configured but `enable` is not true, so none of it is read. " +
					"Set `enable: true` or delete the block",
			)
		}
		return nil
	}
	if !r.IngressEnabled() {
		return errors.Errorf(
			"`ingress.acmeStore.enable: true` on a resource whose ingress is off: the store exists so " +
				"that several Caddies can share one set of certificates, and there is no Caddy here. " +
				"Either enable the ingress or delete the block",
		)
	}
	if r.Ingress.AcmeStorage.Enabled() {
		return errors.Errorf(
			"`ingress.acmeStore.enable: true` and `ingress.acmeStorage` are both set: the first makes SC " +
				"run the fleet's certificate store, the second points at one somebody else runs. Keep " +
				"`acmeStorage` if you have a bucket (it is cheaper and does not put the whole fleet's " +
				"renewals on one machine), and `acmeStore` if you do not",
		)
	}
	if r.NodeCount() < 2 {
		return errors.Errorf(
			"`ingress.acmeStore.enable: true` on a single-machine resource: one Caddy needs no shared " +
				"store (its own /data is the store), and running one buys nothing but a container. " +
				"Delete the block, or declare the fleet it is for",
		)
	}
	if s.Port < 0 || s.Port > 65535 {
		return errors.Errorf("`ingress.acmeStore.port` %d is not a port", s.Port)
	}
	if _, err := r.AcmeStoreHost(); err != nil {
		return err
	}
	if s.Bucket != "" && !validAcmeStoreBucket(s.Bucket) {
		return errors.Errorf(
			"`ingress.acmeStore.bucket` %q is not a valid bucket name: 3-63 characters, lowercase "+
				"letters, digits and hyphens, starting and ending with a letter or digit",
			s.Bucket,
		)
	}
	if s.Image != "" {
		// Checked here and not only at provision time: the value is interpolated into
		// a generated compose file, and the one on the machine must never be able to
		// name something an operator did not.
		if err := validateImageRefShape(s.Image); err != nil {
			return errors.Wrapf(err, "`ingress.acmeStore.image` %q", s.Image)
		}
	}
	return nil
}

// validAcmeStoreBucket is the intersection of the S3 bucket-naming rules and what
// SeaweedFS accepts, which is the subset SC can create without the create silently
// producing something the module cannot then address.
func validAcmeStoreBucket(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && i != 0 && i != len(name)-1:
		default:
			return false
		}
	}
	return true
}

// validateImageRefShape rejects the characters that would let an image reference break
// out of the compose field it is written into, or name a second argument.
func validateImageRefShape(ref string) error {
	if strings.ContainsAny(ref, " \t\n\r\"'`$\\") {
		return errors.Errorf("is not a plain image reference")
	}
	return nil
}

// RenderAcmeStoreCompose is the store's own compose project, on host 0.
func RenderAcmeStoreCompose(cfg *VmConfig) (string, error) {
	if cfg == nil || !cfg.AcmeStoreEnabled() {
		return "", errors.Errorf("the managed ACME store is not enabled on this vm resource")
	}
	host, err := cfg.AcmeStoreHost()
	if err != nil {
		return "", err
	}
	port := cfg.AcmeStorePort()

	f := newComposeFile(AcmeStoreProject)
	f.Services["store"] = composeService{
		Image:         cfg.AcmeStoreImage(),
		ContainerName: AcmeStoreContainer,
		// The image's own `/entrypoint.sh` is BYPASSED, and that is what lets the
		// credential file stay 0600 root-only. Live-caught on the 3-host stand
		// 2026-11-10: the entrypoint ends with `exec su-exec seaweed "$0" "$@"`
		// whenever it starts as root — unconditionally, with no env knob — so `weed`
		// runs as uid 1000 and the S3 gateway dies on every restart with
		//
		//	F auth_credentials.go:389 fail to load config file /etc/sc/s3.json:
		//	  open /etc/sc/s3.json: permission denied
		//
		// while master, volume server and filer come up fine. The container is
		// `running` and answers nothing, which is why the provisioning tier's health
		// gate is the thing that catches it.
		//
		// The alternative was to chown the credential to the image's uid 1000. That
		// hardcodes an image-internal detail into SC, and on any host where uid 1000
		// is a real login account it hands that account ownership of the fleet's
		// certificate-store credential — protected then only by the 0750 on the
		// parent directory. Running this one single-purpose container as root instead
		// is the same trade SC already makes for Caddy, whose env file holds the same
		// credential at the same 0600.
		//
		// Both values below are what the entrypoint's own `server` case passes, minus
		// the drop: `-logtostderr=true`, and the GODEBUG it exports before dispatch.
		// Keeping them means this runs the configuration the image is tested in.
		Entrypoint:  []string{"/usr/bin/weed", "-logtostderr=true"},
		Environment: map[string]string{"GODEBUG": "fips140=on"},
		// `server -s3` is the whole of SeaweedFS in one process: master, volume
		// server, filer and S3 gateway. A fleet's certificates are kilobytes; the
		// distributed modes this project exists for are not what is being used.
		Command: []string{
			"server",
			"-dir=" + acmeStoreContainerDataDir,
			// Also from the entrypoint's `server` case: volumes are created on
			// demand rather than capped at the binary's default of 8.
			"-volume.max=0",
			"-s3",
			"-s3.config=" + acmeStoreContainerConfigPath,
			fmt.Sprintf("-master.volumeSizeLimitMB=%d", acmeStoreVolumeSizeLimitMB),
		},
		Restart: "always",
		// THE SECURITY-LOAD-BEARING LINE OF THIS FILE. Both entries name an explicit
		// host IP, so docker publishes on those two addresses and nowhere else — a
		// bare `8333:8333` binds 0.0.0.0 and docker's own iptables rules bypass the
		// host firewall, which would put this store, and therefore every private key
		// the fleet owns, on the public internet behind a credential that
		// certmagic-s3 prints into `docker logs` on every node.
		//
		// 127.0.0.1 is there for the provisioning tier's own health check and bucket
		// create, which run over SSH on this machine rather than across the overlay.
		Ports: []string{
			fmt.Sprintf("%s:%d:%d", host, port, acmeStoreContainerS3Port),
			fmt.Sprintf("127.0.0.1:%d:%d", port, acmeStoreContainerS3Port),
		},
		Volumes: []string{
			mountString(AcmeStoreDataVolume, acmeStoreContainerDataDir, false),
			// Read-only, and the only file on the machine with the store's
			// credential in it besides the ingress env file.
			mountString(AcmeStoreS3ConfigPath(), acmeStoreContainerConfigPath, true),
		},
		Logging: &composeLogging{
			Driver:  "json-file",
			Options: map[string]string{"max-size": LogMaxSize, "max-file": LogMaxFile},
		},
		// No healthcheck in the compose file, deliberately, and for a sharper reason
		// than the ingress's: the image's tooling is not pinned by SC, so a `wget`
		// probe is a guess — and a wrong guess here marks the store unhealthy, which
		// `depends_on` nothing reads, so it would be a red container beside a working
		// store. The provisioning tier asserts the store answers instead, from the
		// host, and FAILS the provision when it does not.
	}
	f.Volumes = map[string]composeVolume{
		AcmeStoreDataVolume: {External: true, Name: AcmeStoreDataVolume},
	}
	// Deliberately not on IngressNetwork. Caddy reaches this store over the overlay by
	// IP, like every other node does, rather than by container name on host 0 — one
	// code path, and the one that is exercised on N-1 of the machines.
	return marshalCompose(caddyGeneratedHeader+"\n", f)
}

// The paths inside the store's container. Not derived from the host paths: they are
// the container's filesystem and change independently.
const (
	acmeStoreContainerDataDir    = "/data"
	acmeStoreContainerConfigPath = "/etc/sc/s3.json"
	acmeStoreContainerS3Port     = 8333
)

// AcmeStoreDir is the store's configuration directory on host 0.
func AcmeStoreDir() string {
	return ConfigDir + "/acme-store"
}

// AcmeStoreComposePath is the store's compose file on host 0.
func AcmeStoreComposePath() string {
	return AcmeStoreDir() + "/docker-compose.yaml"
}

// AcmeStoreS3ConfigPath is SeaweedFS's S3 identity file on host 0: mode 0600, written
// over stdin, holding the credential every node of the fleet authenticates with.
func AcmeStoreS3ConfigPath() string {
	return AcmeStoreDir() + "/s3.json"
}

// RenderAcmeStoreS3Config is SeaweedFS's `-s3.config` file: one identity, the
// credential SC generated, and the five actions a certmagic store needs.
//
// Takes the credential as arguments rather than reading it off the config, because it
// does not exist in the config — it is generated at provision time and this file is
// rendered inside the apply that resolves it.
//
// `Admin` is in the action list and the others would not be enough on their own:
// SeaweedFS gates bucket creation on it, and SC creates the bucket with this same
// identity rather than inventing a second one. The actions are unscoped (no
// `:<bucket>` suffix), which is the whole store — correct, because this store has one
// bucket and one user by construction.
func RenderAcmeStoreS3Config(accessKey, secretKey string) (string, error) {
	cfg := acmeStoreS3Config{
		Identities: []acmeStoreS3Identity{{
			Name: AcmeStoreIdentity,
			Credentials: []acmeStoreS3Credential{{
				AccessKey: accessKey,
				SecretKey: secretKey,
			}},
			Actions: []string{"Admin", "Read", "Write", "List", "Tagging"},
		}},
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "failed to render the ACME store's S3 configuration")
	}
	// No generated-by header: the file is JSON and SeaweedFS parses it strictly.
	return string(out) + "\n", nil
}

type (
	acmeStoreS3Config struct {
		Identities []acmeStoreS3Identity `json:"identities"`
	}
	acmeStoreS3Identity struct {
		Name        string                  `json:"name"`
		Credentials []acmeStoreS3Credential `json:"credentials"`
		Actions     []string                `json:"actions"`
	}
	acmeStoreS3Credential struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	}
)
