// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The ingress scripts, asserted the same way the stack's are: pure functions, run
// through `sh -n`, read back through decodedBlobs. No Pulumi, no SSH, no docker.

func ingressSpec(t *testing.T) ingressConfigSpec {
	t.Helper()
	assets, err := vm.CaddyAssets()
	Expect(err).ToNot(HaveOccurred())
	return ingressConfigSpec{
		Caddyfile:   "# caddyfile\n{\n\temail ops@example.com\n}\n",
		DefaultSite: "http:// {\n}\n",
		Compose:     "name: sc-caddy\nservices:\n  caddy: {}\n",
		Assets:      assets,
	}
}

func TestIngressConfigScript_CreatesEverythingDeclaredExternal(t *testing.T) {
	RegisterTestingT(t)

	script := ingressConfigScript(ingressSpec(t))
	shellCheck(t, script)

	// The compose project declares all three as external, so none of them is created
	// by `up`: a missing one fails the project with "network not found", which reads
	// like a docker problem rather than a missing provision step.
	Expect(script).To(ContainSubstring("docker network inspect " + shellQuote(vm.IngressNetwork)))
	for _, v := range []string{vm.CaddyDataVolume, vm.CaddyConfigVolume} {
		Expect(script).To(ContainSubstring("docker volume create " + shellQuote(v)))
	}
	for _, d := range []string{vm.CaddyDir(), vm.CaddyConfDir(), vm.CaddyPagesDir()} {
		Expect(script).To(ContainSubstring("install -d -m 0750 " + shellQuote(d)))
	}

	// Every file is written through a temp name and moved into place, so an
	// interrupted provision cannot leave Caddy a truncated Caddyfile to load.
	for _, p := range []string{vm.CaddyfilePath(), vm.CaddyDefaultSitePath(), vm.CaddyComposePath()} {
		Expect(script).To(ContainSubstring("mv -f " + shellQuote(p+".new") + " " + shellQuote(p)))
	}

	// The catch-all is delivered with the config, not written on first deploy: without
	// a site in conf.d, `import conf.d/*.caddy` matches nothing and Caddy refuses a
	// config with no site at all — so a freshly provisioned machine would not start.
	blobs := decodedBlobs(script)
	Expect(blobs).To(ContainElement(ContainSubstring("http:// {")))
	Expect(blobs).To(ContainElement(ContainSubstring("email ops@example.com")))
	// And the error pages, because `handle_server_error` serves them off disk.
	Expect(script).To(ContainSubstring(shellQuote(vm.CaddyPagesDir() + "/503.html")))

	// The env file is NOT here: it carries the ACME EAB MAC key and travels over
	// stdin on its own command.
	Expect(script).ToNot(ContainSubstring(vm.CaddyEnvPath() + ".new"))
}

func TestIngressUpScript_GatesOnTheContainerThenReloads(t *testing.T) {
	RegisterTestingT(t)

	script := ingressUpScript("simplecontainer/caddy:latest")
	shellCheck(t, script)

	Expect(script).To(ContainSubstring("docker compose -p " + shellQuote(vm.CaddyProject)))
	Expect(script).To(ContainSubstring("sc_wait " + shellQuote(vm.CaddyProject)))
	// Both, and in this order. `up -d` is a no-op when only the bind-mounted Caddyfile
	// changed, so without the reload a provision that rewrote the config would report
	// green while the running Caddy still served the previous one.
	Expect(strings.Index(script, "up -d")).
		To(BeNumerically("<", strings.Index(script, "caddy reload")))
	Expect(script).To(ContainSubstring("caddy reload --config " + shellQuote(vm.CaddyContainerConfigDir+"/Caddyfile")))

	// Best-effort pull, then an explicit assertion that the image is there. A machine
	// in a closed network may have the image loaded by hand and no route to Docker
	// Hub — which is the market this path exists for — so a hard `docker pull` would
	// make the ingress unprovisionable there. The inspect is what keeps "could not
	// pull" from silently becoming "no ingress".
	Expect(script).To(ContainSubstring("docker pull " + shellQuote("simplecontainer/caddy:latest")))
	Expect(script).To(ContainSubstring("docker image inspect " + shellQuote("simplecontainer/caddy:latest")))
}

// The volumes stay. CaddyDataVolume holds the ACME account key and every issued
// certificate, and Let's Encrypt allows 50 per domain per week — a teardown that
// deleted them would turn re-provisioning the same machine into a week-long outage.
func TestIngressTeardownScript_KeepsTheCertificates(t *testing.T) {
	RegisterTestingT(t)

	script := ingressTeardownScript()
	shellCheck(t, script)

	Expect(script).To(ContainSubstring("sc_rm " + shellQuote(vm.CaddyProject)))
	Expect(script).To(ContainSubstring("rm -rf " + shellQuote(vm.CaddyDir())))
	Expect(script).ToNot(ContainSubstring("docker volume rm"))
	Expect(script).ToNot(ContainSubstring("down -v"))
	Expect(script).ToNot(ContainSubstring("docker network rm"))
}

func routeSpecFor(content string) routeSpec {
	return routeSpec{
		Stack:   testStack,
		Env:     testEnv,
		Slot:    vm.SlotB,
		Domain:  "atrium.example.ru",
		Content: content,
	}
}

// The flip itself, and the property that matters most about it: a site file this
// stack renders badly must not break the ingress every OTHER stack on the machine is
// behind. conf.d is loaded as one config, so a bad file fails the reload for
// everyone — hence validate-then-reload with the previous file restored on either
// failure.
func TestRouteScript_RestoresThePreviousRouteOnFailure(t *testing.T) {
	RegisterTestingT(t)

	script := routeScript(routeSpecFor("atrium.example.ru {\n  reverse_proxy http://atrium-staging-app-b:8080\n}\n"))
	shellCheck(t, script)

	site := vm.CaddySitePath(testStack, testEnv)
	Expect(script).To(ContainSubstring("site=" + shellQuote(site)))
	Expect(script).To(ContainSubstring("backup=" + shellQuote(site+".prev")))
	// cp, not mv: the previous route has to stay live while the new one is validated,
	// because a validate failure means traffic keeps going where it was.
	Expect(script).To(ContainSubstring(`cp -p "$site" "$backup"`))

	// Validated against the GLOBAL Caddyfile, which is the only thing that reads
	// conf.d the way Caddy will — validating the site file alone would reject every
	// `import` of a snippet the global file defines.
	Expect(script).To(ContainSubstring("caddy validate --config " + shellQuote(vm.CaddyContainerConfigDir+"/Caddyfile")))
	validateAt := strings.Index(script, "caddy validate")
	reloadAt := strings.Index(script, "caddy reload")
	Expect(validateAt).To(BeNumerically("<", reloadAt), "validate must precede reload")

	// The restore appears twice — once for a failed validate, once for a reload that
	// never succeeded. Both are reachable: validate passes on a config Caddy can
	// parse, and reload still fails if the admin API is unreachable.
	restores := strings.Count(script, `if [ -f "$backup" ]; then mv -f "$backup" "$site"`)
	Expect(restores).To(Equal(2), "both the validate and the reload failure paths must restore the previous route")
	// And the backup is dropped only after the reload succeeded.
	Expect(strings.LastIndex(script, `rm -f "$backup"`)).To(BeNumerically(">", reloadAt))

	// The rendered site reaches the machine base64-encoded, like every other
	// generated file: it contains braces, quotes and the operator's own Caddy
	// directives, none of which survive a heredoc reliably.
	Expect(decodedBlobs(script)).To(ContainElement(ContainSubstring("reverse_proxy http://atrium-staging-app-b:8080")))
}

// A missing Caddy is the one failure this script can diagnose better than docker can:
// it only runs when the parent stack said `ingress.enable: true`, so the container not
// being there means the parent was never provisioned with it.
func TestRouteScript_NamesTheParentStackWhenThereIsNoIngress(t *testing.T) {
	RegisterTestingT(t)

	script := routeScript(routeSpecFor("x {\n}\n"))
	Expect(script).To(ContainSubstring("docker inspect " + shellQuote(vm.CaddyContainer)))
	Expect(script).To(ContainSubstring("sc provision"))
	Expect(script).To(ContainSubstring(vm.ResourceTypeVm))
}

// A site file left behind in conf.d outlives the containers it points at: the
// machine's Caddy keeps answering for that domain with a 502 forever, and keeps
// renewing a certificate for it. The removal is unconditional — `rm -f` is a no-op
// for a stack that was never served by Caddy — and the reload is skipped when there
// is no Caddy to reload.
func TestTeardownScript_RemovesTheIngressRoute(t *testing.T) {
	RegisterTestingT(t)

	script := teardownScript(testStack, testEnv)
	shellCheck(t, script)

	Expect(script).To(ContainSubstring("rm -f " + shellQuote(vm.CaddySitePath(testStack, testEnv))))
	Expect(script).To(ContainSubstring("docker inspect " + shellQuote(vm.CaddyContainer)))
	Expect(script).To(ContainSubstring("caddy reload"))
	// The ingress itself is not torn down here: it belongs to the vm resource, and
	// other stacks on the machine are behind it.
	Expect(script).ToNot(ContainSubstring("sc_rm " + shellQuote(vm.CaddyProject)))
	Expect(script).ToNot(ContainSubstring("rm -rf " + shellQuote(vm.CaddyDir())))
}

// The dev-build branch exists because the generic advice ("point it at a registry
// this machine can reach") is wrong for a tag that was never published, and that is
// exactly the tag every locally-rebuilt `sc` asks for.
func TestIngressImageMissingMessage(t *testing.T) {
	RegisterTestingT(t)

	orig := build.Version
	defer func() { build.Version = orig }()

	t.Run("a local build is told its tag does not exist", func(t *testing.T) {
		RegisterTestingT(t)
		build.Version = "0.0.0-pre2026.10.18.11.12.36"

		msg := ingressImageMissingMessage("simplecontainer/caddy:" + build.Version)
		Expect(msg).To(ContainSubstring("local build"))
		Expect(msg).To(ContainSubstring("no such image was published"))
		// The generic advice would send the reader looking for a network fault.
		Expect(msg).ToNot(ContainSubstring("a registry this machine can reach"))
	})

	t.Run("a pinned tag gets the generic advice even on a local build", func(t *testing.T) {
		RegisterTestingT(t)
		build.Version = "0.0.0-pre2026.10.18.11.12.36"

		// Not the default tag, so the version string is irrelevant to why it is absent.
		msg := ingressImageMissingMessage("registry.example.ru/mirror/caddy:2.11")
		Expect(msg).To(ContainSubstring("a registry this machine can reach"))
		Expect(msg).ToNot(ContainSubstring("local build"))
	})

	t.Run("a released build gets the generic advice", func(t *testing.T) {
		RegisterTestingT(t)
		// Release stamps the CalVer that `simplecontainer/caddy` is tagged with, so an
		// absent image here really is a registry or network problem.
		build.Version = "2026.10.24"

		msg := ingressImageMissingMessage("simplecontainer/caddy:" + build.Version)
		Expect(msg).To(ContainSubstring("a registry this machine can reach"))
		Expect(msg).ToNot(ContainSubstring("local build"))
	})

	t.Run("an unstamped go build resolves `latest` and gets the generic advice", func(t *testing.T) {
		RegisterTestingT(t)
		// internal/build defaults Version to "latest", which IS a published tag — so
		// this is not the broken case and must not claim to be.
		build.Version = "latest"

		msg := ingressImageMissingMessage("simplecontainer/caddy:latest")
		Expect(msg).To(ContainSubstring("a registry this machine can reach"))
		Expect(msg).ToNot(ContainSubstring("local build"))
	})
}
