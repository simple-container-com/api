// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// localImageRepoPrefix is the repository every ssh-tar image is tagged under.
//
// A deliberate non-registry: no dot and no colon in the first path component, so
// docker resolves it against Docker Hub rather than treating it as a host — which
// means an accidental `docker push` fails on authentication instead of publishing a
// customer's image to a registry nobody meant to use.
const localImageRepoPrefix = "sc-local"

// dockerTagRe is docker's own constraint on a tag. The tag here is the deploy
// version, which in CI is CalVer (`2026.10.16-a1b2c3d`) and is fine, but on a
// workstation is whatever `--version` was given.
var dockerTagRe = regexp.MustCompile(`^[\w][\w.-]{0,127}$`)

// imagePlan is the result of building the stack's images: what each service should
// run, and what the rest of the deploy has to wait for.
type imagePlan struct {
	// Refs maps a service name to the image reference to put in the compose file. An
	// Output because a registry-delivered image is referenced by the digest the push
	// produced, which is not known until the push has happened.
	Refs map[string]sdk.StringOutput
	// Opts are the resource options every command that runs a container must carry.
	// For registry delivery these are the security gates BuildAndPushImage returns
	// (sign + verify); for ssh-tar they include the ship itself, which is what makes
	// `docker compose up` wait for the image to be on the machine.
	Opts []sdk.ResourceOption
	// ContentIDs identify WHAT this deploy built, as opposed to what it called it.
	// One entry per service whose image SC builds; empty for a stack that only runs
	// third-party images.
	//
	// They exist because `Refs` is not a content identity under `ssh-tar`: the
	// reference there is the local tag (`sc-local/<stack>-<svc>:<version>`), and a
	// tag is exactly what does NOT change when the code does. Every command
	// downstream of the build has to be able to tell a rebuild from a no-op, so the
	// identity is carried separately and put in those commands' Triggers. Under
	// `registry` delivery `Refs` already IS the digest, so this adds nothing there —
	// it is listed anyway, because a trigger set that is correct only in one delivery
	// mode is how this class of bug comes back.
	ContentIDs []sdk.StringOutput
	// PruneRepos are the image repositories this stack owns, and only those. A
	// third-party repository must never be pruned: the mongo image this stack pulled
	// may be the one another stack on the machine is running.
	PruneRepos []string
}

// triggerInputs is ContentIDs as Pulumi inputs, for appending to a Triggers array.
func (p *imagePlan) triggerInputs() []sdk.Input {
	return lo.Map(p.ContentIDs, func(id sdk.StringOutput, _ int) sdk.Input { return id })
}

// buildImages builds every service's image and makes it reachable from the machine.
//
// Two delivery modes, and the difference is one decision made once here rather than
// threaded through the rest of the deploy: with `registry` the image is pushed and
// the machine pulls it; with `ssh-tar` (the default) the image never leaves the
// runner except over the SSH connection the deploy already has. ssh-tar is the
// default because it is the mode that works with no registry at all — which is the
// situation of every operator who has a VM and a docker install and nothing else,
// and of anyone inside Russia for whom reaching a registry is itself the open
// question (compose-on-vm.md §5, D14).
func buildImages(
	ctx *sdk.Context,
	stack api.Stack,
	input api.ResourceInput,
	params pApi.ProvisionParams,
	in *vm.VmComposeInput,
	host HostFacts,
) (*imagePlan, error) {
	version := lo.If(input.StackParams.Version != "", input.StackParams.Version).Else("latest")
	if !dockerTagRe.MatchString(version) {
		return nil, errors.Errorf(
			"deploy version %q cannot be used as a docker tag (must match %s)", version, dockerTagRe.String())
	}
	registry := in.EffectiveImageDelivery() == vm.ImageDeliveryRegistry
	registryURL := lo.FromPtr(in.DockerRegistryURL)

	plan := &imagePlan{Refs: map[string]sdk.StringOutput{}}
	repos := map[string]bool{}

	for _, c := range in.Deployment.Containers {
		container := c
		if container.Name == "" {
			return nil, errors.Errorf("stack %q has a container with no name", stack.Name)
		}

		if container.Image.Dockerfile == "" && container.Image.Context == "" {
			ref := container.Image.Name
			if ref == "" {
				// Mirrors pkg/clouds/pulumi/kubernetes/images_build.go:36-43, which
				// does the same substitution — and is wrong in the same way:
				// k8s.ConvertComposeToContainers drops `svc.Image`, so a compose
				// service whose image is `mongo:7` arrives here with an empty
				// Image.Name, and the service name is only the right answer when it
				// happens to equal the image name. Repeated rather than fixed here
				// because fixing it means changing the conversion the Kubernetes path
				// also uses, which is its own slice; warned about so that the
				// substitution is visible in the deploy log rather than discovered as
				// "why is it pulling the wrong image".
				ref = container.Name
				in.Warnings = append(in.Warnings, fmt.Sprintf(
					"service %q has no build context and no resolvable `image:` — using the service name %q as the "+
						"image reference. If that is not the image you meant, set `image:` on the service explicitly",
					container.Name, ref))
			}
			params.Log.Info(ctx.Context(), "service %q of stack %q runs third-party image %q: SC does not build or ship it",
				container.Name, stack.Name, ref)
			if !registry {
				// Worth saying out loud under ssh-tar, because the whole point of
				// that mode is "the machine needs no registry" and this one service
				// breaks that: it still has to pull. Inside Russia that is the
				// difference between a stack that comes up and one that does not.
				params.Log.Warn(ctx.Context(),
					"service %q runs third-party image %q, so the machine must be able to pull it even though "+
						"`imageDelivery: %s` ships this stack's own images over SSH",
					container.Name, ref, vm.ImageDeliverySshTar)
			}
			plan.Refs[container.Name] = sdk.String(ref).ToStringOutput()
			continue
		}

		dockerfile := resolveDockerfile(container)
		if registry {
			if registryURL == "" {
				// Unreachable through Validate(), which refuses this combination.
				// Kept because this function is the one that would silently push to
				// `/<name>:<tag>` if it ever became reachable.
				return nil, errors.Errorf("`dockerRegistryURL` is required for `imageDelivery: %s`", vm.ImageDeliveryRegistry)
			}
			img, err := pDocker.BuildAndPushImage(ctx, stack, params, *input.StackParams, pDocker.Image{
				Name:                   fmt.Sprintf("%s--%s", stack.Name, container.Name),
				Dockerfile:             dockerfile,
				Context:                container.Image.Context,
				Args:                   lo.FromPtr(container.Image.Build).Args,
				RepositoryUrlWithImage: false,
				Version:                version,
				RepositoryUrl:          sdk.String(registryURL).ToStringOutput(),
				Platform:               lo.If(container.Image.Platform != "", lo.ToPtr(string(container.Image.Platform))).Else(nil),
				Registry: docker.RegistryArgs{
					Server:   sdk.String(registryURL),
					Username: lo.If(in.DockerRegistryUsername != nil, sdk.StringPtr(lo.FromPtr(in.DockerRegistryUsername))).Else(nil),
					Password: lo.If(in.DockerRegistryPassword != nil, sdk.StringPtr(lo.FromPtr(in.DockerRegistryPassword))).Else(nil),
				},
			})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to build and push image for service %q of stack %q", container.Name, stack.Name)
			}
			// DeployImageRef, not the tag: it is the digest whenever one exists, so
			// what the machine runs is what was scanned and signed rather than
			// whatever the tag points at by the time compose pulls.
			plan.Refs[container.Name] = img.DeployImageRef
			plan.ContentIDs = append(plan.ContentIDs, img.DeployImageRef)
			plan.Opts = append(plan.Opts, img.AddOpts...)
			repos[fmt.Sprintf("%s/%s--%s", strings.TrimRight(registryURL, "/"), stack.Name, container.Name)] = true
			continue
		}

		// ssh-tar. pDocker.BuildAndPushImage is not usable here: it requires a
		// registry URL and resolves the deploy reference from the push's digest, and
		// there is no push. The cost of going to docker.NewImage directly is that the
		// security pipeline (scan/sign/verify/SBOM) does not run on an ssh-tar image —
		// a real gap, and the honest one to have: signing an image that is only ever
		// addressed by a local tag on one machine verifies nothing, because there is
		// no registry for anyone to re-resolve the digest against.
		repo := fmt.Sprintf("%s/%s-%s", localImageRepoPrefix, vm.NamePrefix(input.StackParams.StackName, input.StackParams.Environment), container.Name)
		localRef := fmt.Sprintf("%s:%s", repo, version)
		params.Log.Info(ctx.Context(), "building image %q for service %q of stack %q (delivery: %s)",
			localRef, container.Name, stack.Name, vm.ImageDeliverySshTar)

		built, err := docker.NewImage(ctx, input.ToResName(fmt.Sprintf("%s-%s-img", stack.Name, container.Name)), &docker.ImageArgs{
			Build: &docker.DockerBuildArgs{
				Context:    sdk.String(container.Image.Context),
				Dockerfile: sdk.String(dockerfile),
				Args:       buildArgs(container, version),
				Platform: sdk.String(lo.If(container.Image.Platform != "",
					string(container.Image.Platform)).Else(string(api.ImagePlatformLinuxAmd64))),
			},
			ImageName: sdk.String(localRef),
			// Nothing to push to. Also what keeps the build from needing registry
			// credentials at all in this mode.
			SkipPush: sdk.Bool(true),
		}, sdk.DependsOn(params.ComputeContext.Dependencies()))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to build image for service %q of stack %q", container.Name, stack.Name)
		}

		ship, err := shipImage(ctx, input, params, stack, host, container.Name, localRef, version, built)
		if err != nil {
			return nil, err
		}
		// The ship, not the build, is what the compose up has to wait for: the image
		// existing on the runner tells the machine nothing.
		plan.Opts = append(plan.Opts, sdk.DependsOn([]sdk.Resource{ship}))
		plan.Refs[container.Name] = sdk.String(localRef).ToStringOutput()
		// RepoDigest and not ImageName: under SkipPush there is no registry and no
		// manifest digest, and pulumi-docker documents the field as carrying the
		// built image's LOCAL id in that case (`sha256:…`) — which is the one value
		// in this resource that moves when the Dockerfile's output does.
		plan.ContentIDs = append(plan.ContentIDs, built.RepoDigest)
		repos[repo] = true
	}

	plan.PruneRepos = lo.Keys(repos)
	sort.Strings(plan.PruneRepos)
	return plan, nil
}

// shipImage streams one built image to the machine via `sc image ship`.
//
// Run as a local.Command rather than by calling pkg/clouds/vm/imageship in process,
// which would be shorter and is wrong twice over: an in-process transfer runs during
// `pulumi preview` (where nothing is supposed to change) and it sits outside Pulumi's
// resource model, so nothing downstream can depend on it and a repeat deploy has no
// way to know it already happened. As a command it has Triggers, a dependency edge
// to the compose up, and the same idempotency `sc image ship` gives a human: a
// second run with the same image id is a no-op on the machine.
func shipImage(
	ctx *sdk.Context,
	input api.ResourceInput,
	params pApi.ProvisionParams,
	stack api.Stack,
	host HostFacts,
	service, localRef, version string,
	built *docker.Image,
) (*local.Command, error) {
	// built.ImageName is threaded through the apply, not RepoDigest: with SkipPush
	// there is no push and therefore no repo digest, so depending on one would make
	// this command's Create unresolvable in exactly the mode that needs it.
	script := sdk.All(host.Address, built.ImageName).ApplyT(func(vals []any) string {
		addr, _ := vals[0].(string)
		return shipScript(shipSpec{
			ImageRef: localRef,
			Host:     addr,
			User:     host.User,
			Port:     host.Port,
			HostKey:  host.HostKey,
		})
	}).(sdk.StringOutput)

	cmd, err := local.NewCommand(ctx, input.ToResName(fmt.Sprintf("%s-%s-ship", stack.Name, service)), &local.CommandArgs{
		Create: script,
		// Both set to the same script: without Update, pulumi-command treats an
		// input change as a replacement and runs Create after Delete, and there is
		// no Delete here that could undo a ship anyway. Without Triggers, a repeat
		// deploy of a new version would not re-run at all and would report green
		// while the machine kept running the old image (compose-on-vm.md §0.7).
		Update: script,
		// RepoDigest is the one trigger that earns its place here, and the reason the
		// others do not: `localRef`, `version`, `host.Address` and `ImageName` are all
		// the same string on every deploy of a given stack at a given version — and
		// the default version, when `sc deploy` is given no `--version`, is the
		// constant `latest`. So on the default path this command's whole trigger set
		// was invariant: an operator who edits their code and redeploys got a freshly
		// built image on the RUNNER, nothing shipped, nothing restarted, and
		// `Resources: 1 updated, 10 unchanged` with the stack reporting success while
		// the machine served the previous build.
		//
		// Live-caught 2026-10-18 on the byo-host smoke, with the one symptom that
		// makes it expensive: the deploy is green, so the next place anybody looks is
		// their own Dockerfile.
		Triggers: sdk.Array{
			sdk.String(localRef),
			sdk.String(version),
			host.Address,
			built.RepoDigest,
		},
		// The deploy key, never an argument and never an environment variable.
		Stdin: host.PrivateKey.ToStringPtrOutput(),
		// The passphrase has no stdin left to arrive on. An environment variable on
		// a LOCAL command is acceptable where it would not be on a remote one: there
		// is no sshd AcceptEnv to work around, pulumi stores it as a secret because
		// the Output is secret, and it does not land in the command string.
		Environment: sdk.StringMap{"SC_SSH_KEY_PASSWORD": host.PrivateKeyPassword},
		// Nothing about this command's output is useful and all of it is a transfer
		// of a customer image over a connection authenticated by the key on stdin.
		Logging:                local.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn([]sdk.Resource{built}))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to ship image %q for service %q of stack %q", localRef, service, stack.Name)
	}
	params.Log.Info(ctx.Context(), "image %q of stack %q will be shipped to host %q over SSH", localRef, stack.Name, host.Name)
	return cmd, nil
}

// resolveDockerfile reproduces the path resolution of
// pkg/clouds/pulumi/kubernetes/images_build.go:43-47 exactly.
//
// Reproduced rather than shared because it is the behaviour customers' client.yaml
// files are already written against — a compose file's `dockerfile:` is relative to
// the compose file's directory, a `build.context` makes it relative to the context —
// and a VM deploy of the same stack must resolve it the same way or the same
// client.yaml builds on GKE and fails here.
func resolveDockerfile(c k8s.CloudRunContainer) string {
	dockerfile := c.Image.Dockerfile
	if filepath.IsAbs(dockerfile) {
		return dockerfile
	}
	if c.Image.Context == "" || c.Image.Context == "." {
		return filepath.Join(c.ComposeDir, dockerfile)
	}
	return filepath.Join(c.Image.Context, dockerfile)
}

// buildArgs is the build-arg set for an ssh-tar image.
//
// VERSION is injected explicitly here. pDocker.BuildAndPushImage does it for every
// other path in SC and a Dockerfile with `ARG VERSION` + `-ldflags -X main.version`
// relies on it, so an ssh-tar image that reported 0.0.0 while the same Dockerfile
// reported CalVer everywhere else would be a trap rather than a difference.
func buildArgs(c k8s.CloudRunContainer, version string) sdk.StringMap {
	args := sdk.StringMap{"VERSION": sdk.String(version)}
	for k, v := range lo.FromPtr(c.Image.Build).Args {
		args[k] = sdk.String(v)
	}
	return args
}
