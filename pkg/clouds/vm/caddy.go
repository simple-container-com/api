// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
)

// The ingress render: the single Caddy that fronts every stack on the machine, and the
// per-stack site file a deploy drops into its conf.d.
//
// Pure functions, like render.go and for the same reason — the generated Caddyfile is
// what an operator reads when a domain answers the wrong thing, and being able to
// produce it in a unit test from a client.yaml is what keeps it reviewable.
//
// Two shapes, two owners, and the split is the whole design (compose-on-vm.md §8):
//
//	Caddyfile          one per MACHINE, owned by the `vm` resource (parent stack).
//	                   ACME configuration + the shared snippet library + an import.
//	conf.d/<s>-<e>.caddy  one per STACK-ENV, owned by that stack's deploy (child stack).
//
// A deploy therefore never edits a file another stack owns: it writes its own and runs
// `caddy reload`, which is also what makes the blue-green flip atomic — Caddy validates
// the new config and swaps it in without dropping a connection, so the new slot starts
// taking traffic before the old one is stopped. That is the step P5.2 could not have
// (see StopOtherFirst in pkg/clouds/pulumi/vm/compose_scripts.go) and the reason this
// slice removes a documented downtime window rather than adding a feature.

// RenderCaddyfile produces the machine's global Caddyfile.
//
// The snippet library is INLINED from the embedded asset tree rather than imported from
// a second file. Importing would be one line shorter and gives the operator two files
// to read instead of one; inlining also sidesteps the question of whether a snippet
// defined in an imported file is registered at the top level, which is a detail of
// Caddy's parser that this code has no business depending on.
//
// There is deliberately no `admin off`, which compose-on-vm.md §8.1 asks for. It
// cannot coexist with §8.2's `docker exec sc-caddy caddy reload`: reload is a client
// that talks to the admin API, so turning the API off turns every subsequent deploy
// into a full container restart — which drops in-flight connections and, worse, makes
// a config error visible only after the old config is already gone. Caddy's default
// admin endpoint is localhost:2019 INSIDE the container and the compose project
// publishes no port for it, so it is reachable exactly through `docker exec` and from
// nowhere else. Amendment dated 2026-10-17 in the design doc.
func RenderCaddyfile(cfg *VmConfig) (string, error) {
	if cfg == nil || !cfg.IngressEnabled() {
		return "", errors.Errorf("ingress is not enabled on this vm resource")
	}
	if err := cfg.validateIngress(); err != nil {
		return "", err
	}
	snippets, err := caddyassets.FS.ReadFile(caddyassets.CaddyfilePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read the embedded caddy snippet library")
	}

	b := &strings.Builder{}
	b.WriteString(caddyGeneratedHeader)
	b.WriteString("\n{\n")
	fmt.Fprintf(b, "\temail %s\n", caddyToken(cfg.Ingress.Email))
	if cfg.Ingress.AcmeCa != "" {
		// An `acme_ca` that does not speak ACME, or is unreachable, presents as
		// "the site has no certificate" with the reason only in the container's
		// log — hence the pointer in the generated file itself.
		fmt.Fprintf(b, "\t# Certificate authority overridden on the vm resource (`ingress.acmeCa`).\n")
		fmt.Fprintf(b, "\t# If a site has no certificate, read `docker logs %s` first.\n", CaddyContainer)
		fmt.Fprintf(b, "\tacme_ca %s\n", caddyToken(cfg.Ingress.AcmeCa))
	}
	if cfg.Ingress.AcmeEabKeyID != "" {
		// Placeholders, not values: both come from the 0600 env file. `{$VAR}` is
		// substituted by Caddy's own parser at load time.
		fmt.Fprintf(b, "\tacme_eab {\n\t\tkey_id {$%s}\n\t\tmac_key {$%s}\n\t}\n", EnvAcmeEabKeyID, EnvAcmeEabMacKey)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(b, "# Shared snippet library, inlined from %s in the SC binary.\n",
		"pkg/clouds/caddy/"+caddyassets.CaddyfilePath)
	fmt.Fprintf(b, "# Identical to the one the Kubernetes edge serves, so `lbConfig.extraHelpers`\n")
	fmt.Fprintf(b, "# and `lbConfig.siteExtraHelpers` mean the same thing on both paths.\n")
	b.WriteString(strings.TrimRight(string(snippets), "\n"))
	b.WriteString("\n\n")

	fmt.Fprintf(b, "# One file per stack-env, each written by that stack's own deploy.\n")
	fmt.Fprintf(b, "import %s/*.caddy\n", CaddyContainerConfigDir+"/conf.d")
	return b.String(), nil
}

// RenderCaddyDefaultSite is the catch-all site, written once with the ingress.
//
// Same answer as the Kubernetes edge's default entry and copied from it deliberately
// (kubernetes/caddy.go, `defaultCaddyFileEntry`): a 503 with Retry-After, served from
// the same 503.html. A 404 or an empty 200 for an unknown Host is invisible to
// monitoring — which is the failure this shape exists to avoid — and `import
// conf.d/*.caddy` matching nothing on a machine with no stacks yet would leave Caddy
// with a config containing no site at all, which it refuses to load.
//
// `http://` only, and no `import hsts`: this block answers any Host, and Caddy has no
// certificate for a name it was never told about, so an HTTPS catch-all would fail the
// TLS handshake instead of answering — and an HSTS header from a catch-all is
// meaningless.
func RenderCaddyDefaultSite() string {
	return caddyGeneratedHeader + `
http:// {
  import gzip
  handle {
    root * ` + CaddyContainerConfigDir + `/pages
    rewrite * /503.html
    header Cache-Control "no-store"
    header Retry-After "60"
    file_server {
      status 503
    }
  }
}
`
}

// CaddyAsset is one static file the ingress needs on disk, with the absolute path it
// takes on the machine.
type CaddyAsset struct {
	Path    string `json:"path" yaml:"path"`
	Content string `json:"content" yaml:"content"`
}

// CaddyAssets returns every embedded asset except the snippet library, which
// RenderCaddyfile inlines instead.
//
// Today that is the four error pages. It walks the tree rather than naming them,
// because the consumer that must not drift is the `handle_server_error` snippet — it
// serves `/etc/caddy/pages/{rp.status_code}.html`, so a page added to the shared asset
// tree for the Kubernetes edge has to appear here too, and a list of filenames in this
// file is a list somebody will forget.
func CaddyAssets() ([]CaddyAsset, error) {
	var out []CaddyAsset
	err := fs.WalkDir(caddyassets.FS, caddyassets.AssetsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || p == caddyassets.CaddyfilePath {
			return nil
		}
		content, err := caddyassets.FS.ReadFile(p)
		if err != nil {
			return errors.Wrapf(err, "failed to read embedded caddy asset %q", p)
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, caddyassets.AssetsDir), "/")
		out = append(out, CaddyAsset{Path: CaddyDir() + "/" + rel, Content: string(content)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// RenderCaddyEnvFile is the ingress env file. Mode EnvFileMode, content delivered over
// stdin — EnvAcmeEabMacKey is a credential.
//
// Written even when there is nothing to put in it, because compose refuses a project
// whose `env_file` does not exist, and an ingress that silently stops starting after an
// operator removes `acmeEabKeyId` would be a genuinely hard failure to read.
func RenderCaddyEnvFile(cfg *VmConfig) string {
	b := &strings.Builder{}
	b.WriteString("# GENERATED BY SIMPLE CONTAINER — DO NOT EDIT.\n")
	b.WriteString("# Rewritten on every `sc provision` of the parent stack.\n")
	if cfg != nil && cfg.IngressEnabled() && cfg.Ingress.AcmeEabKeyID != "" {
		fmt.Fprintf(b, "%s=%s\n", EnvAcmeEabKeyID, cfg.Ingress.AcmeEabKeyID)
		fmt.Fprintf(b, "%s=%s\n", EnvAcmeEabMacKey, cfg.Ingress.AcmeEabMacKey)
	}
	return b.String()
}

// RenderCaddyCompose is the ingress's own compose project.
func RenderCaddyCompose(cfg *VmConfig) (string, error) {
	if cfg == nil || !cfg.IngressEnabled() {
		return "", errors.Errorf("ingress is not enabled on this vm resource")
	}
	f := newComposeFile(CaddyProject)
	f.Services["caddy"] = composeService{
		Image:         cfg.IngressImage(),
		ContainerName: CaddyContainer,
		// Explicit, not inherited from the image: the config path is SC's choice and
		// the adapter has to be named because the file is a Caddyfile rather than
		// JSON. Same argv as the Kubernetes edge's container.
		Command: []string{
			"caddy", "run",
			"--config", CaddyContainerConfigDir + "/Caddyfile",
			"--adapter", "caddyfile",
		},
		Restart: "always",
		// 443/udp is HTTP/3. Published because Caddy serves it by default, and a
		// published TCP 443 without the UDP one gives clients that try h3 a
		// connection timeout before they fall back.
		Ports:   []string{"80:80", "443:443", "443:443/udp"},
		EnvFile: []string{CaddyEnvPath()},
		Volumes: []string{
			// Read-only: the only writer of this tree is a deploy, over SSH. A Caddy
			// that could write here would also be a Caddy that could be made to
			// write here by a request it mishandled.
			mountString(CaddyDir(), CaddyContainerConfigDir, true),
			// NOT under the config tree. See CaddyDataVolume — this is the Let's
			// Encrypt rate-limit footgun, and the one that takes a week to recover
			// from.
			mountString(CaddyDataVolume, "/data", false),
			mountString(CaddyConfigVolume, "/config", false),
		},
		Logging: &composeLogging{
			Driver:  "json-file",
			Options: map[string]string{"max-size": LogMaxSize, "max-file": LogMaxFile},
		},
		Networks: map[string]composeServiceNetwork{
			networkKeyIngress: {Aliases: []string{CaddyContainer}},
		},
		// No healthcheck, deliberately. The image's tooling is not something SC
		// pins, so a `wget`/`nc` probe would be a guess that fails as "the ingress
		// is unhealthy" on an image that is in fact serving. What the deploy gates
		// on instead is stronger: `caddy reload` validates the config and returns
		// non-zero if it is bad, so a broken site file fails the deploy with the
		// parser's own message and the previous config stays loaded.
	}
	f.Networks = map[string]composeNetwork{
		networkKeyIngress: {External: true, Name: IngressNetwork},
	}
	f.Volumes = map[string]composeVolume{
		// External, like every stack volume and for the same reason: `docker compose
		// -p sc-caddy down -v` must not be able to take the certificates with it.
		CaddyDataVolume:   {External: true, Name: CaddyDataVolume},
		CaddyConfigVolume: {External: true, Name: CaddyConfigVolume},
	}
	return marshalCompose(caddyGeneratedHeader+"\n", f)
}

// ServesDomain reports whether this stack-env gets a site file in conf.d, and is the
// one predicate the deploy branches on: a stack that serves a domain through Caddy
// publishes NO host port, which is what lets both slots run at once.
//
// `prefix` without `domain` is not servable here. On the Kubernetes edge a prefix-only
// stack is grafted into an aggregator site that owns the machine's domain; on a VM
// there is no such site and nothing to graft onto, so SC publishes the host port and
// says so rather than inventing a hostname.
func ServesDomain(in *VmComposeInput, ingressEnabled bool) bool {
	return ingressEnabled &&
		in != nil &&
		in.Deployment.StackConfig != nil &&
		in.Deployment.StackConfig.Domain != "" &&
		in.Deployment.IngressContainer != nil
}

// RenderCaddySite produces one stack-env's file in conf.d.
//
// The directive order is the Kubernetes edge's, kept the same on purpose
// (kubernetes/simple_container.go, `caddyfileEntryTemplate`). It is load-bearing in
// two places that have both already been bugs: `${imports}` and `siteExtraHelpers` sit
// at SITE level, below the reverse_proxy block, so that a `rate_limit` or a catch-all
// `respond` wired through siteExtraHelpers cannot shadow the proxy; and
// `request_buffers` sits INSIDE the proxy block, where a chunked upload is buffered
// into a Content-Length the application can read (Django #28668).
//
// The one difference from Kubernetes is the upstream address, and it carries the slot:
// `http://<stack>-<env>-<service>-<slot>:<port>` instead of a cluster-DNS name. Writing
// this file with the other slot's alias and reloading IS the blue-green flip.
func RenderCaddySite(in *VmComposeInput, opts RenderOptions) (string, error) {
	if in == nil || in.Deployment.StackConfig == nil {
		return "", errors.Errorf("stack has no stack config")
	}
	if err := ValidateNames(opts.Stack, opts.Env); err != nil {
		return "", err
	}
	if !lo.Contains(Slots, opts.Slot) {
		return "", errors.Errorf("unknown slot %q; one of: %s", opts.Slot, strings.Join(Slots, ", "))
	}
	sc := in.Deployment.StackConfig
	if sc.Domain == "" {
		return "", errors.Errorf("stack %q sets no `domain`, so it has no site to serve", opts.Stack)
	}
	ingress := in.Deployment.IngressContainer
	if ingress == nil {
		return "", errors.Errorf("stack %q declares no ingress service, so there is nothing to proxy to", opts.Stack)
	}
	port := lo.FromPtr(ingress.MainPort)
	if port <= 0 {
		return "", errors.Errorf("ingress service %q of stack %q has no port to proxy to", ingress.Name, opts.Stack)
	}

	lbc := lo.FromPtr(sc.LBConfig)
	requestBufferBytes, err := caddyassets.RequestBufferBytes(lbc.RequestBufferSize)
	if err != nil {
		return "", err
	}

	upstream := fmt.Sprintf("http://%s:%d", SlotAlias(opts.Stack, opts.Env, ingress.Name, opts.Slot), port)

	// The proxy block, rendered at the indent of the deepest level it can occupy and
	// re-indented below. Keeps one copy of the body for both the plain-site and the
	// handle_path shapes — the Kubernetes path has two near-identical templates and
	// they have drifted.
	proxy := &strings.Builder{}
	fmt.Fprintf(proxy, "reverse_proxy %s {\n", upstream)
	if requestBufferBytes > 0 {
		// A parsed 0 omits the directive entirely — the escape hatch for a pinned
		// Caddy older than 2.6.0, which predates `request_buffers`.
		fmt.Fprintf(proxy, "  request_buffers %d\n", requestBufferBytes)
	}
	// `Server: nginx` on the way out, exactly as in Kubernetes. Not security, and not
	// claimed to be: it is one less free fact about the stack in every response.
	proxy.WriteString("  header_down Server nginx\n")
	if hdrs := lo.FromPtr(in.Deployment.Headers); len(hdrs) > 0 {
		keys := lo.Keys(hdrs)
		sort.Strings(keys)
		for _, k := range keys {
			// %q, like the Kubernetes path: a multi-token value (CSP,
			// Permissions-Policy) does not survive Caddy's tokenizer unquoted.
			fmt.Fprintf(proxy, "  header_down %s %q\n", k, hdrs[k])
		}
	}
	proxy.WriteString("  import handle_server_error\n")
	for _, h := range lbc.ExtraHelpers {
		fmt.Fprintf(proxy, "  %s\n", h)
	}
	proxy.WriteString("}")

	imports := []string{"import gzip", "import handle_static"}
	// Unconditional, unlike Kubernetes' `useSSL`. There is nothing in front of this
	// Caddy, so it is the thing that terminates TLS and the site is always HTTPS —
	// `lbConfig.https` describes what an external load balancer terminates and has
	// nothing to answer here.
	imports = append(imports, "import hsts")

	b := &strings.Builder{}
	b.WriteString(fmt.Sprintf(caddySiteHeader, opts.Stack, opts.Env, opts.Slot))
	// The bare domain as the site address: Caddy's automatic HTTPS then provisions a
	// certificate for it and redirects HTTP to HTTPS by itself.
	fmt.Fprintf(b, "%s {\n", sc.Domain)
	if sc.Prefix == "" {
		writeIndented(b, proxy.String(), "  ")
		for _, imp := range imports {
			fmt.Fprintf(b, "  %s\n", imp)
		}
		for _, h := range lbc.SiteExtraHelpers {
			fmt.Fprintf(b, "  %s\n", h)
		}
	} else {
		fmt.Fprintf(b, "  handle_path /%s* {\n", sc.Prefix)
		if sc.ProxyKeepPrefix {
			fmt.Fprintf(b, "    rewrite * /%s{uri}\n", sc.Prefix)
		}
		writeIndented(b, proxy.String(), "    ")
		for _, h := range lbc.SiteExtraHelpers {
			fmt.Fprintf(b, "    %s\n", h)
		}
		b.WriteString("  }\n")
		for _, imp := range imports {
			fmt.Fprintf(b, "  %s\n", imp)
		}
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// writeIndented writes a multi-line block with every line prefixed. Blank lines stay
// blank rather than becoming trailing whitespace.
func writeIndented(b *strings.Builder, block, indent string) {
	for _, line := range strings.Split(block, "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + line + "\n")
	}
}

// caddyToken quotes a value that goes into the Caddyfile as a single token.
//
// The two callers pass an email address and a URL, neither of which can contain a
// space in any form Caddy would accept — but they are operator input reaching a config
// file, and the cost of being wrong is an ingress that will not start on the next
// provision.
func caddyToken(v string) string {
	if strings.ContainsAny(v, " \t\"{}") {
		return fmt.Sprintf("%q", v)
	}
	return v
}

const caddyGeneratedHeader = `# GENERATED BY SIMPLE CONTAINER — DO NOT EDIT.
# Rewritten on every deploy; local edits are lost.
# Source of truth: the ` + "`" + ResourceTypeVm + "`" + ` resource in the parent stack's server.yaml.`

// caddySiteHeader takes stack, env and slot. The slot is in there because the question
// an operator asks of this file is "which copy is serving traffic right now", and the
// answer is the alias in the reverse_proxy line.
const caddySiteHeader = `# GENERATED BY SIMPLE CONTAINER — DO NOT EDIT.
# Rewritten on every deploy; local edits are lost.
# Stack %q, environment %q, serving slot %q.
# Regenerate: sc deploy -s %[1]s -e %[2]s
`
