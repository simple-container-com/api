// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// devVersionPrefix is what welder's `rebuild` task stamps into build.Version
// (`0.0.0-pre<timestamp>`), and also what an unset `${env:VERSION}` falls back to.
const devVersionPrefix = "0.0.0"

const (
	// reloadWaitAttempts × reloadWaitInterval is how long `caddy reload` gets to find
	// the admin API after the container was (re)created. Short: the API is up within
	// a second of the process starting, and the only reason for a loop at all is the
	// gap between `docker compose up -d` returning and Caddy having parsed its
	// config.
	reloadWaitAttempts = 10
	reloadWaitInterval = 2
)

// ensureIngress installs the machine's Caddy — the thing that terminates TLS for every
// stack deployed onto it.
//
// It belongs to the `vm` RESOURCE (the parent stack), not to a stack's template, and
// that is the decision the whole slice rests on: one machine has one port 443, so if
// each stack brought its own Caddy the second stack to deploy would fail to bind and
// the first would keep serving (compose-on-vm.md §3, D15). A stack's deploy owns
// exactly one file in conf.d and a reload.
//
// Three commands rather than one, for the reason the vm-compose deploy is split: each
// boundary is a place the provision must be able to stop without having done the next
// thing. Writing the config and bringing Caddy up in one script means a half-written
// tree is what the ingress loads; and the env file holds an ACME credential, so it
// must travel over stdin, which is a property of a whole command rather than of a line
// in it.
func ensureIngress(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	h HostFacts,
	params pApi.ProvisionParams,
	ready *remote.Command,
) error {
	caddyfile, err := vm.RenderCaddyfile(cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to render the Caddyfile of vm %q", input.Descriptor.Name)
	}
	compose, err := vm.RenderCaddyCompose(cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to render the ingress compose file of vm %q", input.Descriptor.Name)
	}
	assets, err := vm.CaddyAssets()
	if err != nil {
		return err
	}

	configScript := ingressConfigScript(ingressConfigSpec{
		Caddyfile:   caddyfile,
		DefaultSite: vm.RenderCaddyDefaultSite(),
		Compose:     compose,
		Assets:      assets,
	})
	upScript := ingressUpScript(cfg.IngressImage())
	envContent := vm.RenderCaddyEnvFile(cfg)

	base := func(a *remote.CommandArgs) *remote.CommandArgs {
		a.Connection = ConnectionArgs(h)
		a.AddPreviousOutputInEnv = sdk.Bool(false)
		return a
	}

	config, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-ingress-config"), base(&remote.CommandArgs{
		Create:   sdk.String(configScript),
		Update:   sdk.String(configScript),
		Triggers: sdk.Array{h.Address, sdk.String(hashOf(configScript))},
	}), sdk.DependsOn([]sdk.Resource{ready}))
	if err != nil {
		return errors.Wrapf(err, "failed to write the ingress configuration of vm %q", input.Descriptor.Name)
	}

	// The env file carries the ACME External Account Binding MAC key when there is
	// one: stdin, Logging off, and a hash rather than the content in Triggers.
	env, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-ingress-env"), base(&remote.CommandArgs{
		Create:   sdk.String(writeFileScript(vm.CaddyEnvPath(), vm.EnvFileMode)),
		Update:   sdk.String(writeFileScript(vm.CaddyEnvPath(), vm.EnvFileMode)),
		Stdin:    secretString(envContent).ToStringPtrOutput(),
		Triggers: sdk.Array{h.Address, sdk.String(hashOf(envContent))},
		Logging:  remote.LoggingNone.ToLoggingPtrOutput(),
	}), sdk.DependsOn([]sdk.Resource{config}))
	if err != nil {
		return errors.Wrapf(err, "failed to write the ingress env file of vm %q", input.Descriptor.Name)
	}

	if _, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-ingress-up"), base(&remote.CommandArgs{
		Create: sdk.String(upScript),
		Update: sdk.String(upScript),
		// The config script's hash is a trigger of the `up` as well, because a
		// Caddyfile change does NOT change the compose file: the config tree is a
		// bind mount, so `up -d` sees nothing to do and the running Caddy keeps the
		// old config. The reload inside this script is what applies it, and without
		// this trigger the script would not run to do the reload.
		Triggers: sdk.Array{h.Address, sdk.String(hashOf(upScript, configScript))},
		Delete:   sdk.String(ingressTeardownScript()),
	}), sdk.DependsOn([]sdk.Resource{env})); err != nil {
		return errors.Wrapf(err, "failed to start the ingress of vm %q", input.Descriptor.Name)
	}

	params.Log.Info(ctx.Context(),
		"vm %q runs Caddy on host %q (image %q, ACME account %q): stacks with a `domain` are served over HTTPS "+
			"on this machine and publish no host port",
		input.Descriptor.Name, h.Name, cfg.IngressImage(), cfg.Ingress.Email)
	if cfg.Ingress.AcmeCa != "" {
		// Said out loud because it is the one ingress setting whose failure mode is
		// silent: Caddy starts, serves, and simply has no certificate. The RU case
		// this field exists for (compose-on-vm.md §8.3) is also unverified.
		params.Log.Warn(ctx.Context(),
			"vm %q points ACME at %q instead of Let's Encrypt. If that CA does not speak ACME, or needs "+
				"External Account Binding it was not given, Caddy will serve the site WITHOUT a certificate and "+
				"say so only in `docker logs %s`",
			input.Descriptor.Name, cfg.Ingress.AcmeCa, vm.CaddyContainer)
	}
	return nil
}

// ingressConfigSpec is the non-secret half of the ingress: everything that can go into
// the command string because it is public, generated, or both.
type ingressConfigSpec struct {
	Caddyfile   string
	DefaultSite string
	Compose     string
	Assets      []vm.CaddyAsset
}

// ingressConfigScript lays down the config tree and everything the compose project
// declares as external.
func ingressConfigScript(s ingressConfigSpec) string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	// The ingress network has to exist before Caddy can join it, and the ingress may
	// well be provisioned before any stack is deployed — so this is the second
	// creator of sc-ingress, alongside a stack's own prepare step. Both are
	// create-if-absent-then-assert for the same reason (see ensureNetwork).
	b.WriteString(ensureNetwork(vm.IngressNetwork) + "\n")
	for _, v := range []string{vm.CaddyDataVolume, vm.CaddyConfigVolume} {
		fmt.Fprintf(b, "docker volume inspect %s >/dev/null 2>&1 || docker volume create %s >/dev/null\n",
			shellQuote(v), shellQuote(v))
	}
	for _, d := range []string{vm.CaddyDir(), vm.CaddyConfDir(), vm.CaddyPagesDir()} {
		fmt.Fprintf(b, "install -d -m 0750 %s\n", shellQuote(d))
	}

	write := func(filePath, content string) {
		fmt.Fprintf(b, "install -d -m 0750 %s\n", shellQuote(path.Dir(filePath)))
		fmt.Fprintf(b, "printf %%s %s | base64 -d > %s\n", shellQuote(encodeFile(content)), shellQuote(filePath+".new"))
		fmt.Fprintf(b, "chmod 0640 %s\n", shellQuote(filePath+".new"))
		fmt.Fprintf(b, "mv -f %s %s\n", shellQuote(filePath+".new"), shellQuote(filePath))
	}
	for _, a := range s.Assets {
		write(a.Path, a.Content)
	}
	write(vm.CaddyfilePath(), s.Caddyfile)
	// The catch-all is written on every provision rather than only when absent. An
	// operator who edited it to debug something gets it back, and the generated
	// header says that will happen.
	write(vm.CaddyDefaultSitePath(), s.DefaultSite)
	write(vm.CaddyComposePath(), s.Compose)
	fmt.Fprintf(b, "echo %s\n", shellQuote("ingress configuration written to "+vm.CaddyDir()))
	return b.String()
}

// ingressImageMissingMessage explains an ingress image the machine does not have.
//
// The generic advice is the right advice when the tag was pinned or the machine has
// no route to Docker Hub. It is actively misleading for the one case that produces a
// tag which was never published and never could be: `IngressImage()` defaults to
// `simplecontainer/caddy:<build.Version>`, and the Caddy image is tagged with
// exactly `${project:version}` at release (welder.yaml), so the default resolves for
// a released `sc` and for an unstamped `go build` (`build.Version` is `latest`, which
// is published) — but welder's `rebuild` task stamps `0.0.0-pre<timestamp>`, so every
// locally-rebuilt binary asks for an image that does not exist. The Kubernetes edge
// defaults the same way and fails the same way; the difference is that this path is
// the one in the byo-host example, which is run from a local build by definition.
//
// Live-caught 2026-10-18, after the readiness gate and the compose render had already
// passed — i.e. at the point where the remaining explanations all sound plausible.
func ingressImageMissingMessage(image string) string {
	msg := fmt.Sprintf("the ingress image %q is not on the machine and could not be pulled; ", image)
	if strings.HasPrefix(build.Version, devVersionPrefix) && strings.HasSuffix(image, ":"+build.Version) {
		return msg + fmt.Sprintf("this `sc` is a local build, so the default tag is its own version "+
			"string (%q) and no such image was published — pin `ingress.image` to a released tag "+
			"(see the tags of `simplecontainer/caddy`) or load an image by hand (`docker load`)", build.Version)
	}
	return msg + "load it by hand (`docker load`) or point `ingress.image` at a registry this machine can reach"
}

// ingressUpScript brings Caddy up and makes the current configuration live.
//
// `up -d` then `reload`, and both are needed. `up -d` creates or recreates the
// container when the compose file or the image changed; it does nothing at all when
// only the Caddyfile changed, because the config tree is a bind mount — so without the
// reload a provision that rewrote the Caddyfile would report green while the running
// Caddy still served the previous one.
func ingressUpScript(image string) string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	// Pulled explicitly so that a tag that moved (`:latest` on a dev machine) is
	// actually fetched; best-effort, because a machine in a closed network may have
	// the image loaded by hand and no route to Docker Hub — which is precisely the RU
	// case this delivery path exists for, and a hard pull would make the ingress
	// unprovisionable there (compose-on-vm.md §8.3).
	fmt.Fprintf(b, "{ docker pull %s || true; } >&2 2>&1\n", shellQuote(image))
	fmt.Fprintf(b, "docker image inspect %s >/dev/null 2>&1 || { echo %s >&2; exit 1; }\n",
		shellQuote(image),
		shellQuote(ingressImageMissingMessage(image)))
	fmt.Fprintf(b, "docker compose -p %s -f %s up -d --remove-orphans\n",
		shellQuote(vm.CaddyProject), shellQuote(vm.CaddyComposePath()))
	// The same gate every stack service gets: running, and healthy if it declares a
	// healthcheck (this one does not — see RenderCaddyCompose). A Caddyfile Caddy
	// refuses makes the container exit, so this is also where a bad global config is
	// reported, with the container's own log.
	fmt.Fprintf(b, "sc_wait %s %s %s\n",
		shellQuote(vm.CaddyProject), shellQuote(vm.CaddyComposePath()), shellQuote("caddy"))
	b.WriteString(reloadLines(""))
	fmt.Fprintf(b, "echo %s\n", shellQuote("ingress is up"))
	return b.String()
}

// reloadLines is `caddy reload`, retried while the admin API comes up.
//
// `onFail` is shell run before the failure is reported — the per-stack route script
// uses it to put the previous site file back. Empty for the ingress itself, where
// there is nothing to roll back to.
//
// Reload goes through Caddy's admin API, which is why the generated Caddyfile does NOT
// contain `admin off` despite compose-on-vm.md §8.1 asking for it. The API listens on
// localhost:2019 inside the container and the compose project publishes no port for
// it, so `docker exec` is the only way in. See RenderCaddyfile.
func reloadLines(onFail string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "i=0; while [ \"$i\" -lt %d ]; do\n", reloadWaitAttempts)
	fmt.Fprintf(b, "  if docker exec %s caddy reload --config %s --adapter caddyfile >&2 2>&1; then break; fi\n",
		shellQuote(vm.CaddyContainer), shellQuote(vm.CaddyContainerConfigDir+"/Caddyfile"))
	fmt.Fprintf(b, "  i=$((i+1)); sleep %d\n", reloadWaitInterval)
	b.WriteString("done\n")
	fmt.Fprintf(b, "if [ \"$i\" -ge %d ]; then\n", reloadWaitAttempts)
	fmt.Fprintf(b, "  echo %s >&2\n", shellQuote(fmt.Sprintf(
		"`caddy reload` did not succeed within %ds; the previous configuration is still live",
		reloadWaitAttempts*reloadWaitInterval)))
	if onFail != "" {
		b.WriteString(indentLines(onFail, "  "))
	}
	b.WriteString("  exit 1\nfi\n")
	return b.String()
}

// ingressTeardownScript removes the ingress when the vm resource is destroyed.
//
// It keeps the two named volumes, deliberately and for a reason specific to this one:
// CaddyDataVolume holds the ACME account key and the issued certificates, and Let's
// Encrypt allows 50 certificates per domain per week. A teardown that deleted them
// would make re-provisioning the same machine a week-long outage rather than a
// five-minute one.
func ingressTeardownScript() string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	fmt.Fprintf(b, "sc_rm %s\n", shellQuote(vm.CaddyProject))
	fmt.Fprintf(b, "rm -rf %s\n", shellQuote(vm.CaddyDir()))
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf(
		"removed the ingress and its configuration; the volumes %s (certificates) and %s were kept",
		vm.CaddyDataVolume, vm.CaddyConfigVolume)))
	return b.String()
}

// indentLines prefixes every non-empty line, for embedding one script fragment inside
// another's `if` block.
func indentLines(s, indent string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
