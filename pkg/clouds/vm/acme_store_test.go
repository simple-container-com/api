// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
)

// storeConfig is the smallest legal shape of the managed store: two machines, the
// overlay on (which is where host 0's address comes from), the ingress on.
func storeConfig() *VmConfig {
	cfg := &VmConfig{
		Driver: DriverByo,
		Hosts: []VmHost{
			{Address: "203.0.113.10", User: "ubuntu", PrivateKey: "k"},
			{Address: "203.0.113.11", User: "ubuntu", PrivateKey: "k"},
		},
		Mesh:    &VmMesh{Enable: true},
		Ingress: &VmIngress{Enable: true, Email: "ops@example.com"},
	}
	cfg.Ingress.AcmeStore = &VmAcmeStore{Enable: true}
	return cfg
}

func TestAcmeStoreAccessors(t *testing.T) {
	RegisterTestingT(t)

	cfg := storeConfig()
	Expect(cfg.AcmeStoreEnabled()).To(BeTrue())
	Expect(cfg.AcmeStoreImage()).To(Equal(DefaultAcmeStoreImage))
	Expect(cfg.AcmeStorePort()).To(Equal(DefaultAcmeStorePort))
	Expect(cfg.AcmeStoreBucket()).To(Equal(DefaultAcmeStoreBucket))
	// Pinned, not floating: this is a server every node's TLS depends on.
	Expect(DefaultAcmeStoreImage).ToNot(HaveSuffix(":latest"))

	cfg.Ingress.AcmeStore = &VmAcmeStore{
		Enable: true, Image: "mirror.example.com/seaweedfs:4.50", Port: 9333, Bucket: "other-acme",
	}
	Expect(cfg.AcmeStoreImage()).To(Equal("mirror.example.com/seaweedfs:4.50"))
	Expect(cfg.AcmeStorePort()).To(Equal(9333))
	Expect(cfg.AcmeStoreBucket()).To(Equal("other-acme"))

	// The nil-safety that matters, and not hypothetically: Validate is driven with a
	// half-filled config by cmd/schema-gen, and every one of these accessors reaches
	// through `ingress`, which is itself a pointer and absent on most vm resources.
	// A nil dereference here panicked two unrelated plugin-driver tests before the
	// acmeStore() helper existed.
	for _, bare := range []*VmConfig{nil, {}, {Ingress: &VmIngress{}}, {Ingress: &VmIngress{Enable: true}}} {
		Expect(bare.AcmeStoreEnabled()).To(BeFalse())
		Expect(bare.AcmeStoreImage()).To(Equal(DefaultAcmeStoreImage))
		Expect(bare.AcmeStorePort()).To(Equal(DefaultAcmeStorePort))
		Expect(bare.AcmeStoreBucket()).To(Equal(DefaultAcmeStoreBucket))
		Expect(bare.validateAcmeStore()).ToNot(HaveOccurred())
	}

	// Enabled is AND-ed with the ingress: a store with no Caddy on the machine is off,
	// whatever the block says. (Validate refuses that combination outright; this is the
	// accessor not lying in the meantime.)
	off := storeConfig()
	off.Ingress.Enable = false
	Expect(off.AcmeStoreEnabled()).To(BeFalse())
}

// Where the other machines dial the store. Three outcomes, and the third is a refusal
// rather than a fallback — every fallback available here is the wrong one.
func TestAcmeStoreHost(t *testing.T) {
	RegisterTestingT(t)

	// 1. A mesh: host 0's overlay address, which is a pure function of (cidr, 0).
	mesh := storeConfig()
	host, err := mesh.AcmeStoreHost()
	Expect(err).ToNot(HaveOccurred())
	Expect(host).To(Equal("10.88.0.1"))
	endpoint, err := mesh.AcmeStoreEndpoint()
	Expect(err).ToNot(HaveOccurred())
	// Plain http, deliberately — the hop is inside WireGuard. See AcmeStoreEndpoint.
	Expect(endpoint).To(Equal(fmt.Sprintf("http://10.88.0.1:%d", DefaultAcmeStorePort)))

	// A non-default range moves it, still deterministically.
	mesh.Mesh.Cidr = "10.99.7.0/24"
	host, err = mesh.AcmeStoreHost()
	Expect(err).ToNot(HaveOccurred())
	Expect(host).To(Equal("10.99.7.1"))

	// 2. No mesh but a shared private network: host 0's privateAddress.
	priv := storeConfig()
	priv.Mesh = nil
	priv.Hosts[0].PrivateAddress = "10.15.1.216"
	host, err = priv.AcmeStoreHost()
	Expect(err).ToNot(HaveOccurred())
	Expect(host).To(Equal("10.15.1.216"))

	// 3. Neither: refused, naming both ways out.
	none := storeConfig()
	none.Mesh = nil
	_, err = none.AcmeStoreHost()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`mesh.enable: true`"))
	Expect(err.Error()).To(ContainSubstring("`hosts[0].privateAddress`"))
	// And it says WHY a public address is not the fallback, because that is the
	// obvious third option and it is the dangerous one.
	Expect(err.Error()).To(ContainSubstring("logs its credentials in the clear"))
	// The public addresses never appear in the endpoint, on any path.
	for _, cfg := range []*VmConfig{mesh, priv} {
		ep, epErr := cfg.AcmeStoreEndpoint()
		Expect(epErr).ToNot(HaveOccurred())
		Expect(ep).ToNot(ContainSubstring("203.0.113."))
	}
}

// The synthesized acmeStorage. Its job is to give the Caddyfile renderer one input
// shape whether the bucket is Yandex's, GCP's, an operator's minio or SC's container.
func TestManagedAcmeStorage(t *testing.T) {
	RegisterTestingT(t)

	cfg := storeConfig()
	storage, err := cfg.ManagedAcmeStorage()
	Expect(err).ToNot(HaveOccurred())
	Expect(storage.Enabled()).To(BeTrue())
	Expect(storage.Bucket).To(Equal(DefaultAcmeStoreBucket))
	Expect(storage.Endpoint).To(Equal("http://10.88.0.1:8333"))
	// Path style, not a default to reason about: SeaweedFS serves no
	// `<bucket>.<host>` label, and a bare IPv4 host cannot have one.
	Expect(storage.PathStyle).ToNot(BeNil())
	Expect(*storage.PathStyle).To(BeTrue())
	// Sentinels, never values. The real credentials are generated at provision time.
	Expect(storage.AccessKey).To(Equal(AcmeStoreManagedCredential))
	Expect(storage.SecretKey).To(Equal(AcmeStoreManagedCredential))
	Expect(storage.EncryptionKey).To(Equal(AcmeStoreManagedCredential))

	// EffectiveAcmeStorage is the single reader every renderer goes through.
	eff, err := cfg.EffectiveAcmeStorage()
	Expect(err).ToNot(HaveOccurred())
	Expect(eff).ToNot(BeNil())
	Expect(eff.Endpoint).To(Equal(storage.Endpoint))

	// An operator's own store comes back unchanged.
	own := storeConfig()
	own.Ingress.AcmeStore = nil
	own.Ingress.AcmeStorage = &VmAcmeStorage{Bucket: "b", Region: "eu-central-1"}
	eff, err = own.EffectiveAcmeStorage()
	Expect(err).ToNot(HaveOccurred())
	Expect(eff.Bucket).To(Equal("b"))

	// And a single machine has none, which is correct — its own /data is the store.
	solo := storeConfig()
	solo.Ingress.AcmeStore = nil
	eff, err = solo.EffectiveAcmeStorage()
	Expect(err).ToNot(HaveOccurred())
	Expect(eff).To(BeNil())
}

// The config-tier refusals. Every one is a thing an operator can write and every one
// would otherwise be accepted and quietly do nothing or the wrong thing.
func TestValidateAcmeStore(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name     string
		mutate   func(*VmConfig)
		errMatch string
	}{
		{
			name:   "the smallest legal shape",
			mutate: func(*VmConfig) {},
		},
		{
			name:   "an absent block",
			mutate: func(c *VmConfig) { c.Ingress.AcmeStore = nil },
		},
		{
			name:     "fields with enable off are refused, not ignored",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore = &VmAcmeStore{Bucket: "sc-acme"} },
			errMatch: "configured but `enable` is not true",
		},
		{
			name:   "enable off and nothing else is fine",
			mutate: func(c *VmConfig) { c.Ingress.AcmeStore = &VmAcmeStore{} },
		},
		{
			// The reason validateAcmeStore is called from validateByo/validateCloud
			// rather than from inside validateIngress, whose `if !IngressEnabled()
			// { return nil }` guard would skip exactly this case.
			name:     "a store with no Caddy to serve",
			mutate:   func(c *VmConfig) { c.Ingress.Enable = false },
			errMatch: "whose ingress is off",
		},
		{
			name: "two stores declared and one used",
			mutate: func(c *VmConfig) {
				c.Ingress.AcmeStorage = &VmAcmeStorage{Bucket: "b", Region: "eu-central-1"}
			},
			errMatch: "are both set",
		},
		{
			name:     "a shared store for one machine",
			mutate:   func(c *VmConfig) { c.Hosts = c.Hosts[:1]; c.Mesh = nil },
			errMatch: "on a single-machine resource",
		},
		{
			name:     "no address to reach host 0 on",
			mutate:   func(c *VmConfig) { c.Mesh = nil },
			errMatch: "needs an address the other machines can reach the first one on",
		},
		{
			name:     "a port that is not one",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore.Port = 70000 },
			errMatch: "is not a port",
		},
		{
			name:     "a bucket name S3 will not take",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore.Bucket = "SC_Acme" },
			errMatch: "is not a valid bucket name",
		},
		{
			name:     "a bucket name ending in a hyphen",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore.Bucket = "sc-acme-" },
			errMatch: "is not a valid bucket name",
		},
		{
			// The image is interpolated into a generated compose file on the machine,
			// so the shape is checked here and not only at provision time.
			name:     "an image reference carrying a shell metacharacter",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore.Image = "seaweedfs:4.50 && curl evil.sh" },
			errMatch: "is not a plain image reference",
		},
		{
			name:     "an image reference carrying an expansion",
			mutate:   func(c *VmConfig) { c.Ingress.AcmeStore.Image = "seaweedfs:${TAG}" },
			errMatch: "is not a plain image reference",
		},
	}
	for _, c := range cases {
		cfg := storeConfig()
		c.mutate(cfg)
		err := cfg.validateAcmeStore()
		if c.errMatch == "" {
			Expect(err).ToNot(HaveOccurred(), "case %q", c.name)
			continue
		}
		Expect(err).To(HaveOccurred(), "case %q must be refused", c.name)
		Expect(err.Error()).To(ContainSubstring(c.errMatch), "case %q", c.name)
	}
}

// Both refusals have to be reachable through the public Validate, not only through the
// unexported checker — the first version of this wiring put validateAcmeStore where the
// ingress-off case could never run.
func TestAcmeStoreRefusalsAreReachableFromValidate(t *testing.T) {
	RegisterTestingT(t)

	ingressOff := storeConfig()
	ingressOff.Ingress.Enable = false
	err := ingressOff.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("whose ingress is off"))

	// `originTls: false` means nothing on this machine ever asks for a certificate, so
	// a store would be stood up for nobody to write to.
	noTls := storeConfig()
	noTls.Ingress.OriginTls = boolPtr(false)
	// `email` has its own `originTls: false` refusal and it fires first. Cleared here
	// so this case exercises the store's refusal rather than that one — an operator who
	// wrote both gets them one at a time, which is fine, but a test that asserts on the
	// first one is not testing what it says.
	noTls.Ingress.Email = ""
	err = noTls.Validate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`ingress.originTls: false`"))
	Expect(err.Error()).To(ContainSubstring("nothing would ever write to it"))

	// And the legal shape passes the whole validator, not just its store half.
	Expect(storeConfig().Validate()).ToNot(HaveOccurred())
}

// The store's compose project. The port bindings here are the security-load-bearing
// part of the whole slice.
func TestRenderAcmeStoreCompose(t *testing.T) {
	RegisterTestingT(t)

	cfg := storeConfig()
	out, err := RenderAcmeStoreCompose(cfg)
	Expect(err).ToNot(HaveOccurred())

	// TWO explicit host IPs and NEVER a bare `8333:8333`: a bare mapping binds
	// 0.0.0.0, and docker's own iptables rules bypass the host firewall — which would
	// put the fleet's private keys on the public internet behind a credential
	// certmagic-s3 prints into `docker logs` on every node.
	Expect(out).To(ContainSubstring("10.88.0.1:8333:8333"))
	Expect(out).To(ContainSubstring("127.0.0.1:8333:8333"))
	Expect(out).ToNot(MatchRegexp(`(?m)^\s*-\s*"?8333:8333`))
	Expect(out).ToNot(ContainSubstring("0.0.0.0"))
	// No public address, on any line.
	Expect(out).ToNot(ContainSubstring("203.0.113."))

	Expect(out).To(ContainSubstring(DefaultAcmeStoreImage))
	Expect(out).To(ContainSubstring("container_name: " + AcmeStoreContainer))
	// The data volume is EXTERNAL: `docker compose -p sc-acme-store down -v` must not
	// be able to delete the store every node reads from. Losing it is a Let's Encrypt
	// rate-limit event, not an inconvenience.
	Expect(out).To(ContainSubstring("external: true"))
	Expect(out).To(ContainSubstring(AcmeStoreDataVolume))
	// The identity file is mounted read-only — the container has no business
	// rewriting the credential SC generated for it.
	Expect(out).To(ContainSubstring(AcmeStoreS3ConfigPath() + ":/etc/sc/s3.json:ro"))
	Expect(out).To(ContainSubstring("restart: always"))
	// The image's entrypoint is bypassed, and this is a credential-permissions
	// assertion rather than a cosmetic one. `/entrypoint.sh` ends with
	// `exec su-exec seaweed "$0" "$@"` whenever it starts as root, so without the
	// override `weed` runs as uid 1000 and cannot read the 0600 root-owned identity
	// file — the S3 gateway then dies on a loop inside a container docker reports as
	// `running` (live-caught on the 3-host stand, 2026-11-10). The flags carried over
	// are the ones that entrypoint's own `server` case passes.
	Expect(out).To(ContainSubstring("- /usr/bin/weed"))
	Expect(out).To(ContainSubstring("-logtostderr=true"))
	Expect(out).To(ContainSubstring("-volume.max=0"))
	Expect(out).To(ContainSubstring("GODEBUG: fips140=on"))
	// And no `user:` key, which would NOT have fixed it: the drop happens whenever
	// the entrypoint sees uid 0, and starting as 1000 lands on the same unreadable
	// file from the other side.
	Expect(out).ToNot(MatchRegexp(`(?m)^\s*user:`))
	// Log rotation, because this container's logs are on a machine SC does not own.
	Expect(out).To(ContainSubstring("max-size"))
	// No healthcheck, deliberately: the image's tooling is not pinned by SC, so a
	// probe here is a guess, and nothing reads the result. The provisioning tier
	// asserts the store answers instead, and fails the provision when it does not.
	Expect(out).ToNot(ContainSubstring("healthcheck"))
	// Not on the ingress network: Caddy reaches the store over the overlay by IP on
	// every machine, host 0 included, so there is one code path rather than two.
	Expect(out).ToNot(ContainSubstring(IngressNetwork))
	Expect(out).To(HavePrefix(caddyGeneratedHeader))

	// An overridden port moves both bindings together.
	cfg.Ingress.AcmeStore.Port = 9500
	out, err = RenderAcmeStoreCompose(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).To(ContainSubstring("10.88.0.1:9500:8333"))
	Expect(out).To(ContainSubstring("127.0.0.1:9500:8333"))
	// The container-side port is the image's own and does not move with it.
	Expect(out).ToNot(ContainSubstring(":9500:9500"))

	// A disabled store renders nothing rather than a file nobody asked for.
	off := storeConfig()
	off.Ingress.AcmeStore = nil
	_, err = RenderAcmeStoreCompose(off)
	Expect(err).To(HaveOccurred())
	_, err = RenderAcmeStoreCompose(nil)
	Expect(err).To(HaveOccurred())
}

// SeaweedFS's identity file. Parsed strictly, so it is asserted as JSON rather than as
// text.
func TestRenderAcmeStoreS3Config(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderAcmeStoreS3Config("AKIA-generated", "secret-generated")
	Expect(err).ToNot(HaveOccurred())

	var parsed acmeStoreS3Config
	Expect(json.Unmarshal([]byte(out), &parsed)).ToNot(HaveOccurred())
	Expect(parsed.Identities).To(HaveLen(1))
	id := parsed.Identities[0]
	Expect(id.Name).To(Equal(AcmeStoreIdentity))
	Expect(id.Credentials).To(HaveLen(1))
	Expect(id.Credentials[0].AccessKey).To(Equal("AKIA-generated"))
	Expect(id.Credentials[0].SecretKey).To(Equal("secret-generated"))
	// `Admin` is required and the rest are not enough on their own: SeaweedFS gates
	// bucket creation on it, and SC creates the bucket with this same identity rather
	// than inventing a second one.
	Expect(id.Actions).To(ContainElements("Admin", "Read", "Write", "List", "Tagging"))

	// No generated-by header: SeaweedFS parses this file as JSON and a comment line
	// would make it refuse to start.
	Expect(out).ToNot(ContainSubstring("GENERATED BY"))
	Expect(out).To(HavePrefix("{"))
	Expect(out).To(HaveSuffix("\n"))
}

// The env file is the only place the store's real credentials reach a node. The
// sentinel reaching it instead is the worst available outcome — a store that exists, a
// Caddy that starts, and certificates that never issue — so it is a hard refusal.
func TestRenderCaddyEnvFileManagedStore(t *testing.T) {
	RegisterTestingT(t)

	cfg := storeConfig()

	_, err := RenderCaddyEnvFile(cfg, nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("was not resolved before rendering the ingress env file"))
	Expect(err.Error()).To(ContainSubstring("this is an SC bug"))

	env, err := RenderCaddyEnvFile(cfg, map[string]string{
		caddyassets.EnvAcmeS3AccessKey:     "real-access",
		caddyassets.EnvAcmeS3SecretKey:     "real-secret",
		caddyassets.EnvAcmeS3EncryptionKey: strings.Repeat("e", 32),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(env).To(ContainSubstring(caddyassets.EnvAcmeS3AccessKey + "=real-access"))
	Expect(env).To(ContainSubstring(caddyassets.EnvAcmeS3SecretKey + "=real-secret"))
	Expect(env).ToNot(ContainSubstring(AcmeStoreManagedCredential))

	// A store SC does not manage needs no `managed` map and must not grow a
	// dependency on one.
	own := storeConfig()
	own.Ingress.AcmeStore = nil
	own.Ingress.AcmeStorage = &VmAcmeStorage{
		Bucket: "b", Region: "eu-central-1", AccessKey: "a", SecretKey: "s",
	}
	env, err = RenderCaddyEnvFile(own, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(env).To(ContainSubstring(caddyassets.EnvAcmeS3AccessKey + "=a"))
}

// The Caddyfile every node runs. One assertion here has a live-caught CrashLoop behind
// it; see persist_config.
func TestIngressConfigWithManagedStore(t *testing.T) {
	RegisterTestingT(t)

	cfg := storeConfig()
	out, err := RenderCaddyfile(cfg)
	Expect(err).ToNot(HaveOccurred())

	// The endpoint is a literal, because host 0's overlay address is a pure function
	// of (cidr, 0) — which is exactly why the mesh addresses machines the way it does
	// instead of reading an address back off a provisioned host.
	Expect(out).To(ContainSubstring("endpoint http://10.88.0.1:8333"))
	Expect(out).To(ContainSubstring("bucket " + DefaultAcmeStoreBucket))
	// Required for SeaweedFS, and silently skipped by the module's parser unless it is
	// rendered WITH a value.
	Expect(out).To(ContainSubstring("use_path_style true"))

	// Credentials are `{$VAR}` placeholders and nothing else. No sentinel, no value.
	Expect(out).To(ContainSubstring("access_key {$" + caddyassets.EnvAcmeS3AccessKey + "}"))
	Expect(out).To(ContainSubstring("secret_key {$" + caddyassets.EnvAcmeS3SecretKey + "}"))
	Expect(out).ToNot(ContainSubstring(AcmeStoreManagedCredential))

	// `{$VAR}` is substituted at ADAPT time, so without this line Caddy writes the
	// resolved credentials into /config/caddy/autosave.json in cleartext on every
	// node. The obvious spelling — `admin { config { persist false } }` — is rejected
	// at adapt time and CrashLoops every replica; live-caught on the 3-VPS stand,
	// 2026-11-09.
	Expect(out).To(ContainSubstring("persist_config off"))
	Expect(out).ToNot(ContainSubstring("persist false"))

	// A single machine gets neither a storage block nor persist_config: its own /data
	// is the store, and turning the autosave off would be a cost with no benefit.
	solo := storeConfig()
	solo.Hosts = solo.Hosts[:1]
	solo.Mesh = nil
	solo.Ingress.AcmeStore = nil
	out, err = RenderCaddyfile(solo)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(ContainSubstring("storage s3"))
	Expect(out).ToNot(ContainSubstring("persist_config off"))
}

func boolPtr(b bool) *bool { return &b }
