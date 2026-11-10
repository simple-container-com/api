// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

func ingressConfig() *VmConfig {
	return &VmConfig{
		Driver:  DriverByo,
		Hosts:   []VmHost{{Address: "203.0.113.10", User: "ubuntu", PrivateKey: "k"}},
		Ingress: &VmIngress{Enable: true, Email: "ops@example.com"},
	}
}

// The three things the global Caddyfile has to get right, and all three have a
// failure mode that looks like something else.
func TestRenderCaddyfile(t *testing.T) {
	RegisterTestingT(t)

	got, err := RenderCaddyfile(ingressConfig())
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(ContainSubstring("email ops@example.com"))
	// The snippet library is inlined, not imported. If this breaks, every site file's
	// `import gzip` becomes "unrecognized directive" — which reads as a bug in the
	// site renderer rather than in this one.
	for _, snippet := range []string{"(gzip)", "(hsts)", "(handle_static)", "(handle_server_error)"} {
		Expect(got).To(ContainSubstring(snippet), "snippet %s must be inlined into the global Caddyfile", snippet)
	}
	// The one line that makes a stack's deploy effective at all.
	Expect(got).To(ContainSubstring("import " + CaddyContainerConfigDir + "/conf.d/*.caddy"))
	// NOT `admin off`, despite the design doc asking for it: `caddy reload` is an
	// admin-API client, so turning the API off would turn every deploy into a full
	// restart. See RenderCaddyfile's doc comment.
	Expect(got).ToNot(ContainSubstring("admin off"))
	// No ACME override unless asked for — otherwise an empty `acme_ca` would send
	// every machine to a CA that does not exist.
	Expect(got).ToNot(ContainSubstring("acme_ca"))
	Expect(got).ToNot(ContainSubstring("acme_eab"))
}

func TestRenderCaddyfileRefusesWithoutIngress(t *testing.T) {
	RegisterTestingT(t)

	cfg := ingressConfig()
	cfg.Ingress.Enable = false
	_, err := RenderCaddyfile(cfg)
	Expect(err).To(HaveOccurred())

	// Enabled but with no ACME account address: refused here rather than producing a
	// Caddyfile whose `email` line is empty, which Caddy accepts and which then fails
	// at certificate time with the CA's own error.
	cfg = ingressConfig()
	cfg.Ingress.Email = ""
	_, err = RenderCaddyfile(cfg)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ingress.email"))
}

// The EAB credential must reach Caddy as a placeholder, never as a literal. The
// Caddyfile is world-readable on the machine (0640, and in the Pulumi command string
// before that); the env file is 0600 and travels over stdin.
func TestRenderCaddyfileKeepsTheEabMacKeyOutOfTheConfig(t *testing.T) {
	RegisterTestingT(t)

	cfg := ingressConfig()
	cfg.Ingress.AcmeCa = "https://acme.example.ru/directory"
	cfg.Ingress.AcmeEabKeyID = "eab-key-id-value"
	cfg.Ingress.AcmeEabMacKey = "s3cr3t-mac-key"

	got, err := RenderCaddyfile(cfg)
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(ContainSubstring("acme_ca https://acme.example.ru/directory"))
	Expect(got).To(ContainSubstring("key_id {$" + EnvAcmeEabKeyID + "}"))
	Expect(got).To(ContainSubstring("mac_key {$" + EnvAcmeEabMacKey + "}"))
	Expect(got).ToNot(ContainSubstring("s3cr3t-mac-key"))

	env, err := RenderCaddyEnvFile(cfg, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(env).To(ContainSubstring(EnvAcmeEabKeyID + "=eab-key-id-value"))
	Expect(env).To(ContainSubstring(EnvAcmeEabMacKey + "=s3cr3t-mac-key"))
}

// The shared certificate store, rendered per backend. On a fleet this block is what
// makes origin HTTPS possible at all — see writeCaddyAcmeStorage.
func TestRenderCaddyfileSharedAcmeStorage(t *testing.T) {
	RegisterTestingT(t)

	// Yandex Object Storage: a custom endpoint, so path-style addressing, and HMAC
	// credentials that must not appear in the config.
	cfg := ingressConfig()
	cfg.Ingress.AcmeStorage = &VmAcmeStorage{
		Bucket:    "sc-acme",
		Endpoint:  "https://storage.yandexcloud.net",
		AccessKey: "YCAJEaccesskey",
		SecretKey: "YCNsecretkeyvalue",
	}

	got, err := RenderCaddyfile(cfg)
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(ContainSubstring("storage s3 {"))
	Expect(got).To(ContainSubstring("bucket sc-acme"))
	Expect(got).To(ContainSubstring("endpoint https://storage.yandexcloud.net"))
	Expect(got).To(ContainSubstring("region " + DefaultAcmeStorageRegion))
	Expect(got).To(ContainSubstring("prefix " + DefaultAcmeStoragePrefix))
	Expect(got).To(ContainSubstring("access_key {$" + EnvAcmeS3AccessKey + "}"))
	Expect(got).To(ContainSubstring("secret_key {$" + EnvAcmeS3SecretKey + "}"))
	// Same discipline as the EAB mac key: the Caddyfile is 0640 on the machine and is
	// echoed verbatim by `caddy adapt`.
	Expect(got).ToNot(ContainSubstring("YCNsecretkeyvalue"))
	Expect(got).ToNot(ContainSubstring("YCAJEaccesskey"))
	// The block belongs to the GLOBAL options, not to a site. Caddy rejects `storage`
	// at site level, so a misplaced block is a config-load failure on every machine.
	Expect(strings.Index(got, "storage s3 {")).To(BeNumerically("<", strings.Index(got, "(gzip)")))

	env, err := RenderCaddyEnvFile(cfg, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(env).To(ContainSubstring(EnvAcmeS3AccessKey + "=YCAJEaccesskey"))
	Expect(env).To(ContainSubstring(EnvAcmeS3SecretKey + "=YCNsecretkeyvalue"))
	Expect(env).ToNot(ContainSubstring(EnvAcmeS3EncryptionKey))

	// Real AWS S3: no endpoint, so virtual-host addressing and the operator's region.
	aws := ingressConfig()
	aws.Ingress.AcmeStorage = &VmAcmeStorage{Bucket: "sc-acme", Region: "eu-central-1", Prefix: "certs"}
	got, err = RenderCaddyfile(aws)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(ContainSubstring("endpoint "))
	Expect(got).To(ContainSubstring("region eu-central-1"))
	Expect(got).To(ContainSubstring("prefix certs"))
	Expect(got).To(ContainSubstring("use_path_style false"))
	// No credentials given: the AWS default chain is the module's own fallback, and on
	// EC2 it is the right answer. The pair must then be absent from BOTH files rather
	// than rendered as placeholders that expand to nothing.
	Expect(got).ToNot(ContainSubstring("access_key"))
	Expect(got).ToNot(ContainSubstring("secret_key"))
	awsEnv, err := RenderCaddyEnvFile(aws, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(awsEnv).ToNot(ContainSubstring(EnvAcmeS3AccessKey))

	// minio on host 0, self-signed: the only shape in which `insecure` is rendered.
	minio := ingressConfig()
	minio.Ingress.AcmeStorage = &VmAcmeStorage{
		Bucket:        "sc-acme",
		Endpoint:      "https://10.0.0.5:9000",
		AccessKey:     "minioadmin",
		SecretKey:     "minioadmin",
		Insecure:      true,
		EncryptionKey: strings.Repeat("e", AcmeStorageEncryptionKeyLen),
	}
	got, err = RenderCaddyfile(minio)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(ContainSubstring("use_path_style true"))
	Expect(got).To(ContainSubstring("insecure true"))
	Expect(got).To(ContainSubstring("encryption_key {$" + EnvAcmeS3EncryptionKey + "}"))
	Expect(got).ToNot(ContainSubstring(strings.Repeat("e", AcmeStorageEncryptionKeyLen)))
	minioEnv, err := RenderCaddyEnvFile(minio, nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(minioEnv).To(ContainSubstring(
		EnvAcmeS3EncryptionKey + "=" + strings.Repeat("e", AcmeStorageEncryptionKeyLen),
	))

	// And absent entirely on a single machine, which is the common case.
	bare, err := RenderCaddyfile(ingressConfig())
	Expect(err).ToNot(HaveOccurred())
	Expect(bare).ToNot(ContainSubstring("storage"))
}

// certmagic-s3's UnmarshalCaddyfile reads every subdirective with
// `if !d.Args(&value) { continue }`. A bare `use_path_style` is therefore NOT an error
// and NOT true — it is silently skipped, and the only symptom is a fleet on minio whose
// every storage request fails because it addressed the bucket virtual-host style.
//
// So: no boolean is ever rendered without a value. This test asserts the property
// directly rather than trusting the three cases above to keep covering it.
func TestRenderCaddyfileNeverRendersABareBoolean(t *testing.T) {
	RegisterTestingT(t)

	cfg := ingressConfig()
	cfg.Ingress.AcmeStorage = &VmAcmeStorage{
		Bucket:    "sc-acme",
		Endpoint:  "https://10.0.0.5:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Insecure:  true,
	}
	got, err := RenderCaddyfile(cfg)
	Expect(err).ToNot(HaveOccurred())

	for _, line := range strings.Split(got, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if !lo.Contains([]string{"use_path_style", "insecure"}, fields[0]) {
			continue
		}
		Expect(fields).To(HaveLen(2), "%q must carry an explicit value — certmagic-s3 silently skips a valueless subdirective", line)
		Expect(fields[1]).To(BeElementOf("true", "false"))
	}
}

// Written even when empty: compose refuses a project whose `env_file` is missing, so
// an ingress with no EAB would otherwise stop starting the moment the file was not
// created.
func TestRenderCaddyEnvFileIsNeverEmpty(t *testing.T) {
	RegisterTestingT(t)

	got, err := RenderCaddyEnvFile(ingressConfig(), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).ToNot(BeEmpty())
	Expect(got).ToNot(ContainSubstring(EnvAcmeEabKeyID))
}

// The catch-all. `import conf.d/*.caddy` matching nothing leaves Caddy with a config
// that has no site in it, which it refuses to load — so a machine provisioned before
// its first stack is deployed needs this file to come up at all.
func TestRenderCaddyDefaultSite(t *testing.T) {
	RegisterTestingT(t)

	got := RenderCaddyDefaultSite()
	Expect(got).To(ContainSubstring("http:// {"))
	Expect(got).To(ContainSubstring("status 503"))
	Expect(got).To(ContainSubstring("Retry-After"))
	Expect(got).To(ContainSubstring("root * " + CaddyContainerConfigDir + "/pages"))
	// No HSTS from a block that answers any Host, and no https:// — Caddy has no
	// certificate for a name it was never told about, so an HTTPS catch-all would fail
	// the handshake instead of answering 503.
	Expect(got).ToNot(ContainSubstring("import hsts"))
}

// The assets are walked, not listed, because `handle_server_error` serves
// pages/{rp.status_code}.html — a page added to the shared tree for the Kubernetes
// edge has to land here too.
func TestCaddyAssetsCoverTheErrorPages(t *testing.T) {
	RegisterTestingT(t)

	assets, err := CaddyAssets()
	Expect(err).ToNot(HaveOccurred())
	Expect(assets).ToNot(BeEmpty())

	paths := lo.Map(assets, func(a CaddyAsset, _ int) string { return a.Path })
	for _, code := range []string{"404", "500", "502", "503"} {
		Expect(paths).To(ContainElement(CaddyPagesDir()+"/"+code+".html"),
			"the %s error page must be delivered to the machine", code)
	}
	// The snippet library is inlined into the Caddyfile, so delivering it as a file
	// too would give Caddy two definitions of every snippet.
	Expect(paths).ToNot(ContainElement(CaddyfilePath()))
	for _, a := range assets {
		Expect(a.Content).ToNot(BeEmpty(), "asset %q is empty — check the //go:embed glob", a.Path)
		Expect(a.Path).To(HavePrefix(CaddyDir()+"/"), "asset %q escapes the config tree", a.Path)
	}
}

// The ingress compose project. Asserted on the parsed tree rather than on substrings,
// like render_test.go, so the test cannot pass on a file that is textually right and
// syntactically wrong.
func TestRenderCaddyCompose(t *testing.T) {
	RegisterTestingT(t)

	content, err := RenderCaddyCompose(ingressConfig())
	Expect(err).ToNot(HaveOccurred())

	var f map[string]any
	Expect(yaml.Unmarshal([]byte(content), &f)).To(Succeed())
	Expect(f["name"]).To(Equal(CaddyProject))

	svc := service(t, content, "caddy")
	// container_name is fixed, because the flip runs `docker exec sc-caddy caddy
	// reload` and a compose-generated name would make that command depend on the
	// project's numbering.
	Expect(svc["container_name"]).To(Equal(CaddyContainer))
	Expect(svc["restart"]).To(Equal("always"))
	Expect(svc["ports"]).To(ConsistOf("80:80", "443:443", "443:443/udp"))
	Expect(svc["env_file"]).To(ConsistOf(CaddyEnvPath()))
	Expect(svc["volumes"]).To(ContainElement(CaddyDir() + ":" + CaddyContainerConfigDir + ":ro"))
	// The one that takes a week to recover from: /data is a named volume, so a
	// recreate does not throw away the ACME account key and every issued certificate
	// (Let's Encrypt allows 50 per domain per week).
	Expect(svc["volumes"]).To(ContainElement(CaddyDataVolume + ":/data"))

	volumes, ok := f["volumes"].(map[string]any)
	Expect(ok).To(BeTrue())
	for _, name := range []string{CaddyDataVolume, CaddyConfigVolume} {
		vol, ok := volumes[name].(map[string]any)
		Expect(ok).To(BeTrue(), "volume %q is not declared", name)
		// External, so that `docker compose -p sc-caddy down -v` cannot take the
		// certificates with it.
		Expect(vol["external"]).To(BeTrue(), "volume %q must be external", name)
	}

	networks, ok := f["networks"].(map[string]any)
	Expect(ok).To(BeTrue())
	shared, ok := networks[networkKeyIngress].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(shared["external"]).To(BeTrue())
	Expect(shared["name"]).To(Equal(IngressNetwork))
}

// ServesDomain is the single predicate three downstream decisions read: whether a host
// port is published, whether both slots may run at once, and what the URL is. Each row
// here is a case that has to resolve to a host port rather than to a broken site.
func TestServesDomain(t *testing.T) {
	RegisterTestingT(t)

	withDomain := renderTestInput(t)
	Expect(withDomain.Deployment.StackConfig.Domain).ToNot(BeEmpty())
	Expect(withDomain.Deployment.IngressContainer).ToNot(BeNil())

	Expect(ServesDomain(withDomain, true)).To(BeTrue())
	// The machine does not run Caddy: nothing listens on 443, so the domain is not
	// served however the stack is configured.
	Expect(ServesDomain(withDomain, false)).To(BeFalse())
	Expect(ServesDomain(nil, true)).To(BeFalse())

	// A stack with no domain. `prefix` alone is not servable on a VM: on Kubernetes it
	// is grafted onto an aggregator site that owns the machine's domain, and there is
	// no such site here.
	noDomain := renderTestInput(t)
	noDomain.Deployment.StackConfig.Domain = ""
	Expect(ServesDomain(noDomain, true)).To(BeFalse())

	// A stack whose services are all internal: there is nothing for Caddy to proxy to.
	noIngress := renderTestInput(t)
	noIngress.Deployment.IngressContainer = nil
	Expect(ServesDomain(noIngress, true)).To(BeFalse())
}

func caddySiteOpts() RenderOptions {
	return RenderOptions{Stack: "refapp", Env: "staging", Slot: SlotB, Version: "2026.10.17-abc1234"}
}

// The site file, and the two ordering constraints inside it that have each already
// been a bug on the Kubernetes path.
func TestRenderCaddySite(t *testing.T) {
	RegisterTestingT(t)

	in := renderTestInput(t)
	got, err := RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(ContainSubstring("refapp.example.com {"))
	// The upstream carries the SLOT. Writing this file with the other slot's alias and
	// reloading IS the blue-green flip — nothing else in the deploy moves traffic.
	ingress := in.Deployment.IngressContainer
	alias := SlotAlias("refapp", "staging", ingress.Name, SlotB)
	Expect(got).To(ContainSubstring("reverse_proxy http://" + alias + ":"))
	Expect(got).To(ContainSubstring(alias))
	Expect(got).ToNot(ContainSubstring(SlotAlias("refapp", "staging", ingress.Name, SlotA)))
	// No cluster DNS: that is the one line this renderer changes relative to the
	// Kubernetes template, and pasting the k8s version is the obvious mistake.
	Expect(got).ToNot(ContainSubstring("svc.cluster.local"))

	Expect(got).To(ContainSubstring("header_down Server nginx"))
	Expect(got).To(ContainSubstring("import handle_server_error"))
	// Unconditional, unlike Kubernetes' `useSSL`: nothing sits in front of this Caddy,
	// so it is the thing that terminates TLS and the site is always HTTPS.
	Expect(got).To(ContainSubstring("import hsts"))
	Expect(got).To(ContainSubstring("import gzip"))
	Expect(got).To(ContainSubstring("import handle_static"))

	// The site-level imports sit BELOW the reverse_proxy block, so that a catch-all
	// wired in through siteExtraHelpers cannot shadow the proxy.
	Expect(strings.Index(got, "reverse_proxy")).To(BeNumerically("<", strings.Index(got, "import hsts")))
	// And the slot is in the header, because "which copy is serving right now" is the
	// question this file gets asked.
	Expect(got).To(ContainSubstring(`serving slot "b"`))
}

// request_buffers goes INSIDE the proxy block: that is where a chunked upload is
// buffered into a Content-Length the application can read (Django #28668). At site
// level the directive is accepted and does nothing of the sort.
func TestRenderCaddySitePutsRequestBuffersInsideTheProxy(t *testing.T) {
	RegisterTestingT(t)

	in := renderTestInput(t)
	in.Deployment.StackConfig.LBConfig = &api.SimpleContainerLBConfig{RequestBufferSize: "1MB"}

	got, err := RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(ContainSubstring("request_buffers 1000000"))

	proxyAt := strings.Index(got, "reverse_proxy")
	closeAt := strings.Index(got[proxyAt:], "\n  }\n") + proxyAt
	Expect(strings.Index(got, "request_buffers")).To(BeNumerically(">", proxyAt))
	Expect(strings.Index(got, "request_buffers")).To(BeNumerically("<", closeAt),
		"request_buffers must be inside the reverse_proxy block, not at site level:\n%s", got)
}

// extraHelpers belong to the proxy, siteExtraHelpers to the site — the same meaning
// they have on the Kubernetes edge, which is the point of sharing the snippet library.
func TestRenderCaddySitePlacesHelpersAtTheirOwnLevel(t *testing.T) {
	RegisterTestingT(t)

	in := renderTestInput(t)
	in.Deployment.StackConfig.LBConfig = &api.SimpleContainerLBConfig{
		ExtraHelpers:     []string{"import lb_retries_2"},
		SiteExtraHelpers: []string{"respond /healthz 200"},
	}
	in.Deployment.Headers = lo.ToPtr(map[string]string{
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'self'",
	})

	got, err := RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())

	proxyAt := strings.Index(got, "reverse_proxy")
	closeAt := strings.Index(got[proxyAt:], "\n  }\n") + proxyAt
	Expect(strings.Index(got, "import lb_retries_2")).To(BeNumerically("<", closeAt))
	Expect(strings.Index(got, "respond /healthz 200")).To(BeNumerically(">", closeAt))

	// Multi-token header values are quoted, like the Kubernetes path: Caddy's
	// tokenizer splits `default-src 'self'` into two arguments otherwise.
	Expect(got).To(ContainSubstring(`header_down Content-Security-Policy "default-src 'self'"`))
	// Sorted, so that an unrelated map iteration does not rewrite the file and make
	// every deploy look like a change.
	Expect(strings.Index(got, "Content-Security-Policy")).
		To(BeNumerically("<", strings.Index(got, "X-Frame-Options")))
}

func TestRenderCaddySiteWithPrefix(t *testing.T) {
	RegisterTestingT(t)

	in := renderTestInput(t)
	in.Deployment.StackConfig.Prefix = "api"
	in.Deployment.StackConfig.ProxyKeepPrefix = true
	in.Deployment.StackConfig.LBConfig = &api.SimpleContainerLBConfig{SiteExtraHelpers: []string{"respond /healthz 200"}}

	got, err := RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())

	Expect(got).To(ContainSubstring("handle_path /api* {"))
	// handle_path strips the prefix; proxyKeepPrefix puts it back, which is what a
	// `single-image` stack with a `basePath` needs (it serves under that path itself).
	Expect(got).To(ContainSubstring("rewrite * /api{uri}"))
	// siteExtraHelpers move INSIDE the handle_path with the proxy they qualify; the
	// three imports stay at site level.
	handleAt := strings.Index(got, "handle_path")
	Expect(strings.Index(got, "respond /healthz 200")).To(BeNumerically(">", handleAt))
	Expect(strings.Index(got, "import hsts")).To(BeNumerically(">", strings.Index(got, "respond /healthz 200")))

	// Without proxyKeepPrefix the rewrite is absent: the application serves at the
	// root and handle_path's strip is the whole point.
	in.Deployment.StackConfig.ProxyKeepPrefix = false
	got, err = RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(ContainSubstring("handle_path /api* {"))
	Expect(got).ToNot(ContainSubstring("rewrite *"))
}

// Every way the site renderer can be asked for something it cannot produce. Each one
// is refused with the reason, rather than rendered into a file Caddy accepts and that
// then serves the wrong thing.
func TestRenderCaddySiteRefusals(t *testing.T) {
	RegisterTestingT(t)

	for name, mutate := range map[string]func(in *VmComposeInput, opts *RenderOptions){
		"no domain": func(in *VmComposeInput, _ *RenderOptions) {
			in.Deployment.StackConfig.Domain = ""
		},
		"no ingress service": func(in *VmComposeInput, _ *RenderOptions) {
			in.Deployment.IngressContainer = nil
		},
		"ingress service with no port": func(in *VmComposeInput, _ *RenderOptions) {
			in.Deployment.IngressContainer.MainPort = nil
		},
		"unknown slot": func(_ *VmComposeInput, opts *RenderOptions) {
			opts.Slot = "c"
		},
		"stack name compose would rewrite": func(_ *VmComposeInput, opts *RenderOptions) {
			opts.Stack = "Ref App"
		},
	} {
		in := renderTestInput(t)
		opts := caddySiteOpts()
		mutate(in, &opts)
		_, err := RenderCaddySite(in, opts)
		Expect(err).To(HaveOccurred(), "%s must be refused", name)
	}
}

// `ingress.originTls: false`: the machine serves plain HTTP for an edge that
// terminates, and the global block has to say so in a way that cannot be contradicted
// by a hand-written site block in conf.d.
func TestRenderCaddyfileNoOriginTls(t *testing.T) {
	RegisterTestingT(t)

	cfg := ingressConfig()
	cfg.Ingress.Email = ""
	cfg.Ingress.OriginTls = lo.ToPtr(false)

	got, err := RenderCaddyfile(cfg)
	Expect(err).ToNot(HaveOccurred())

	// The whole point: no automatic HTTPS anywhere on this machine. Without it one
	// bare domain in one conf.d file sends the machine off to a CA for a certificate
	// whose challenge it cannot answer, and the fleet quietly earns a rate-limit.
	Expect(got).To(ContainSubstring("auto_https off"))
	// Nothing ACME-shaped is rendered, because nothing requests a certificate.
	Expect(got).ToNot(ContainSubstring("email "))
	Expect(got).ToNot(ContainSubstring("acme_ca"))
	Expect(got).ToNot(ContainSubstring("acme_eab"))
	Expect(got).ToNot(ContainSubstring("storage s3"))
	// The hop the operator is now responsible for, named in the file they will read.
	Expect(got).To(ContainSubstring("CLEARTEXT"))
	// The snippet library and the conf.d import are NOT a casualty of the early
	// return — a `siteExtraHelpers` that resolves on an HTTPS machine and not on this
	// one would be a 500 on a fraction of a fleet's requests.
	for _, snippet := range []string{"(gzip)", "(hsts)", "(handle_static)", "(handle_server_error)"} {
		Expect(got).To(ContainSubstring(snippet), "snippet %s must be inlined here too", snippet)
	}
	Expect(got).To(ContainSubstring("import " + CaddyContainerConfigDir + "/conf.d/*.caddy"))
}

// The site file's scheme. Both halves are asserted against the SAME stack input, so a
// regression that renders one scheme unconditionally fails one of them.
func TestRenderCaddySiteSchemeFollowsOriginTls(t *testing.T) {
	RegisterTestingT(t)

	in := renderTestInput(t)
	domain := in.Deployment.StackConfig.Domain
	Expect(domain).ToNot(BeEmpty())

	https, err := RenderCaddySite(in, caddySiteOpts())
	Expect(err).ToNot(HaveOccurred())
	Expect(https).To(ContainSubstring(domain + " {"))
	Expect(https).ToNot(ContainSubstring("http://" + domain))

	opts := caddySiteOpts()
	opts.NoOriginTls = true
	plain, err := RenderCaddySite(in, opts)
	Expect(err).ToNot(HaveOccurred())
	// The scheme is not decoration: it is what suppresses automatic HTTPS for this
	// site and puts the listener on :80.
	Expect(plain).To(ContainSubstring("http://" + domain + " {"))
	// HSTS stays. It reaches the browser over the EDGE's TLS, where it is honoured;
	// with no edge at all a browser ignores HSTS received over cleartext, so it is
	// inert rather than harmful. There is no case where dropping it is better.
	Expect(plain).To(ContainSubstring("import hsts"))
	// And the upstream is untouched — the proxy hop was always plain HTTP over the
	// docker network.
	Expect(plain).To(ContainSubstring("reverse_proxy http://"))
}
