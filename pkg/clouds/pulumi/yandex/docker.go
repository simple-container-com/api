// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pDocker "github.com/simple-container-com/api/pkg/clouds/pulumi/docker"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
)

const (
	// ContainerRegistryHost is the single host every Yandex Container Registry lives
	// under. Unlike ECR (one host per account/region) the registry id is the first
	// path segment, so an image is cr.yandex/<registry-id>/<name>:<tag>.
	ContainerRegistryHost = "cr.yandex"

	// ContainerRegistryUsername is the literal username `docker login cr.yandex`
	// expects when the password is a service account's authorized-key document.
	// It is not a placeholder — YC matches on this exact string.
	ContainerRegistryUsername = "json_key"
)

type dockerImage struct {
	name       string
	dockerfile string
	args       map[string]string
	context    string
	version    string
	platform   api.ImagePlatform
}

type dockerImageOut struct {
	image   *docker.Image
	addOpts []sdk.ResourceOption
	// deployImageRef is the immutable digest reference to point the container at,
	// for the same reason ECS does: a tag can be moved after the image was built.
	deployImageRef sdk.StringOutput
	// registry is non-nil only when SC created it. When the operator supplies
	// registryId the registry is out of SC's ownership and must not be diffed.
	registry *sdkYandex.ContainerRegistry
}

// buildAndPushDockerImage resolves (or creates) a Container Registry in the
// configured folder and pushes the stack's image into it through the shared
// build path, which also injects VERSION as a build arg.
func buildAndPushDockerImage(
	ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams,
	cfg *yandex.ServerlessContainerInput, image dockerImage, opts ...sdk.ResourceOption,
) (*dockerImageOut, error) {
	password, err := containerRegistryPassword(&cfg.AccountConfig)
	if err != nil {
		return nil, err
	}

	var registry *sdkYandex.ContainerRegistry
	var registryID sdk.StringOutput
	// Only the registry dependency is forwarded to the image: the rest of opts
	// carries sdk.Provider(<the yandex provider>), which a docker.Image resource
	// has no use for.
	var imageOpts []sdk.ResourceOption
	if cfg.RegistryID != "" {
		params.Log.Info(ctx.Context(), "using pre-existing yandex container registry %q for stack %q", cfg.RegistryID, stack.Name)
		registryID = sdk.String(cfg.RegistryID).ToStringOutput()
	} else {
		registryName := fmt.Sprintf("%s-registry", image.name)
		params.Log.Info(ctx.Context(), "configure yandex container registry %q for stack %q...", registryName, stack.Name)
		registry, err = sdkYandex.NewContainerRegistry(ctx, registryName, &sdkYandex.ContainerRegistryArgs{
			Name:     sdk.String(registryName),
			FolderId: sdk.String(cfg.FolderID),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create container registry %q for stack %q", registryName, stack.Name)
		}
		// ID() rather than RegistryId: the latter is bridged from a field the TF
		// schema also uses as a *lookup* input, so it is not guaranteed populated on
		// a plain create. The resource id of yandex_container_registry IS the
		// registry id.
		registryID = registry.ID().ToStringOutput()
		imageOpts = append(imageOpts, sdk.DependsOn([]sdk.Resource{registry}))
	}

	repositoryURL := registryID.ApplyT(func(id string) string {
		return fmt.Sprintf("%s/%s", ContainerRegistryHost, id)
	}).(sdk.StringOutput)

	repositoryName := toRepositoryName(image.name)
	if repositoryName == "" {
		return nil, errors.Errorf("cannot derive a container registry repository name from %q for stack %q", image.name, stack.Name)
	}
	if repositoryName != image.name {
		params.Log.Info(ctx.Context(), "image name %q is not a valid %s repository name, pushing as %q instead",
			image.name, ContainerRegistryHost, repositoryName)
	}

	out, err := pDocker.BuildAndPushImage(ctx, stack, params, deployParams, pDocker.Image{
		Name:       repositoryName,
		Dockerfile: image.dockerfile,
		Args:       image.args,
		Context:    image.context,
		Version:    image.version,
		// false, unlike AWS: one registry holds every image of the folder and the
		// image name is a path segment under it, so the name must be appended.
		RepositoryUrlWithImage: false,
		RepositoryUrl:          repositoryURL,
		ProviderOptions:        imageOpts,
		Platform:               lo.If(image.platform != "", lo.ToPtr(string(image.platform))).Else(nil),
		Registry: docker.RegistryArgs{
			Server:   sdk.String(ContainerRegistryHost),
			Username: sdk.String(ContainerRegistryUsername),
			Password: sdk.ToSecret(sdk.String(password)).(sdk.StringOutput).ToStringPtrOutput(),
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push docker image %q (from %q) for stack %q env %q",
			image.name, image.context, stack.Name, deployParams.Environment)
	}

	return &dockerImageOut{
		image:          out.Image,
		addOpts:        out.AddOpts,
		deployImageRef: out.DeployImageRef,
		registry:       registry,
	}, nil
}

// separatorRunRegexp matches a run of two or more consecutive separators in a
// repository name.
var separatorRunRegexp = regexp.MustCompile(`[._-]{2,}`)

// toRepositoryName makes an image name pushable to cr.yandex.
//
// Yandex Container Registry enforces the *legacy* Docker repository grammar,
// `[a-z0-9]+(?:[._-][a-z0-9]+)*` — at most ONE separator between components.
// Modern Docker clients accept `__` and `-+`, so the client happily forms the
// request and the registry answers a bare `400 Bad Request` to the blob HEAD,
// with no hint that the repository name is what it objects to. Live-caught on
// the first YC container deploy, 2026-09-26: `--` and `__` both fail, `-`, `_`
// and `.` all succeed.
//
// This matters for every stack, not just oddly-named ones: SC's image name is
// the stack name, and a client stack is always `<stack>--<env>`. So without this
// NO Forge service could push an image to YC at all.
//
// Runs collapse to their first character (`a--b` -> `a-b`, `a__b` -> `a_b`)
// rather than being hashed or stripped, to keep the registry readable. Two
// stacks whose names differ only in the position of a doubled separator would
// collide, which is accepted: it takes deliberately adversarial naming to hit.
func toRepositoryName(name string) string {
	collapsed := separatorRunRegexp.ReplaceAllStringFunc(strings.ToLower(name), func(run string) string {
		return run[:1]
	})
	// A leading or trailing separator is rejected by the same grammar.
	return strings.Trim(collapsed, "._-")
}

// containerRegistryPassword returns the credential that authenticates a docker
// push to cr.yandex: the service account's authorized-key document itself.
//
// The static access key pair (accessKey/secretAccessKey) does NOT work here — it
// signs SigV4 requests to the S3-compatible services only. And because
// AccountConfig.ServiceAccountKey may hold either the JSON document or a path to
// it (ProviderArgs.ServiceAccountKeyFile accepts both), a path has to be read
// here; handing the filename to docker would fail as an opaque 401.
func containerRegistryPassword(cfg *yandex.AccountConfig) (string, error) {
	key := strings.TrimSpace(cfg.ServiceAccountKey)
	if key == "" {
		return "", errors.Errorf("serviceAccountKey must be set to push images to %s: the static access key pair authenticates the S3-compatible services, not the container registry", ContainerRegistryHost)
	}
	if json.Valid([]byte(key)) && strings.HasPrefix(key, "{") {
		return key, nil
	}
	content, err := os.ReadFile(key)
	if err != nil {
		// Deliberately does not echo the value: it is either a path (harmless but
		// noisy) or a malformed key document (a credential).
		return "", errors.Wrapf(err, "serviceAccountKey is neither a JSON authorized-key document nor a readable file path")
	}
	return string(content), nil
}
