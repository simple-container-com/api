package docker

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type Image struct {
	Name                   string
	Dockerfile             string
	Args                   map[string]string
	Context                string
	Version                string
	RepositoryUrlWithImage bool
	ProviderOptions        []sdk.ResourceOption
	RepositoryUrl          sdk.StringOutput
	Registry               docker.RegistryArgs
	Platform               *string
}

type ImageOut struct {
	Image   *docker.Image
	AddOpts []sdk.ResourceOption
}

func BuildAndPushImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image Image) (*ImageOut, error) {
	imageFullUrl := image.RepositoryUrl.ApplyT(func(repoUri string) string {
		if image.RepositoryUrlWithImage {
			return fmt.Sprintf("%s:%s", repoUri, image.Version)
		}
		return fmt.Sprintf("%s/%s:%s", repoUri, image.Name, image.Version)
	}).(sdk.StringOutput)
	params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for stack %q env %q",
		image.Name, image.Context, stack.Name, deployParams.Environment)
	args := sdk.StringMap{
		"VERSION": sdk.String(image.Version),
	}
	for k, v := range image.Args {
		args[k] = sdk.String(v)
	}
	res, err := docker.NewImage(ctx, image.Name, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:    sdk.String(image.Context),
			Dockerfile: sdk.String(image.Dockerfile),
			Args:       args,
			Platform:   sdk.String(lo.If(image.Platform != nil, lo.FromPtr(image.Platform)).Else(string(api.ImagePlatformLinuxAmd64))),
		},
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: imageFullUrl,
		Registry:  image.Registry,
	}, append(image.ProviderOptions, sdk.DependsOn(params.ComputeContext.Dependencies()))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q for stack %q env %q", image.Name, stack.Name, deployParams.Environment)
	}

	var addOpts []sdk.ResourceOption

	// Execute security operations if configured
	if stack.Client.Security != nil {
		securityOpts, err := executeSecurityOperations(ctx, stack, res, image.Name, imageFullUrl)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to execute security operations for image %q", image.Name)
		}
		addOpts = append(addOpts, securityOpts...)
	}

	if len(addOpts) == 0 {
		addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{res}))
	}

	return &ImageOut{
		Image:   res,
		AddOpts: addOpts,
	}, nil
}

// executeSecurityOperations creates Pulumi local.Command resources for security operations
// Dependency chain: dockerImage → scanCmd → signCmd → [sbomGenCmd, provenanceCmd] (parallel)
func executeSecurityOperations(ctx *sdk.Context, stack api.Stack, dockerImage *docker.Image, imageName string, imageUrl sdk.StringOutput) ([]sdk.ResourceOption, error) {
	security := stack.Client.Security
	var lastResource sdk.Resource = dockerImage
	var opts []sdk.ResourceOption

	// Prepare environment variables for cosign
	env := map[string]string{}
	// OIDC token should be set in environment by CI/CD
	// We just enable experimental mode for keyless signing
	if security.Signing != nil && security.Signing.Keyless {
		env["COSIGN_EXPERIMENTAL"] = "1"
	}

	envArgs := []string{}
	for k, v := range env {
		envArgs = append(envArgs, fmt.Sprintf("%s=%s", k, v))
	}
	envPrefix := ""
	if len(envArgs) > 0 {
		envPrefix = strings.Join(envArgs, " ") + " "
	}

	// Step 1: Vulnerability Scanning (if enabled) - fail-fast
	if security.Scan != nil && security.Scan.Enabled {
		scanToolName := "grype"
		if len(security.Scan.Tools) > 0 && security.Scan.Tools[0].Name != "" {
			scanToolName = security.Scan.Tools[0].Name
		}

		failOnFlag := ""
		if security.Scan.FailOn != "" {
			failOnFlag = fmt.Sprintf("--fail-on %s", security.Scan.FailOn)
		}

		scanCmd, err := local.NewCommand(ctx, fmt.Sprintf("scan-%s", imageName), &local.CommandArgs{
			Create: imageUrl.ApplyT(func(img string) string {
				return fmt.Sprintf("sc image scan --image %s --tool %s %s", img, scanToolName, failOnFlag)
			}).(sdk.StringOutput),
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create scan command")
		}
		lastResource = scanCmd
	}

	// Step 2: Image Signing (if enabled)
	if security.Signing != nil && security.Signing.Enabled {
		keylessFlag := ""
		if security.Signing.Keyless {
			keylessFlag = "--keyless"
		} else if security.Signing.PrivateKey != "" {
			keylessFlag = fmt.Sprintf("--key %s", security.Signing.PrivateKey)
		}

		signCmd, err := local.NewCommand(ctx, fmt.Sprintf("sign-%s", imageName), &local.CommandArgs{
			Create: imageUrl.ApplyT(func(img string) string {
				return fmt.Sprintf("%ssc image sign --image %s %s", envPrefix, img, keylessFlag)
			}).(sdk.StringOutput),
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sign command")
		}
		lastResource = signCmd
	}

	// Step 3a: SBOM Generation and Attachment (if enabled) - runs in parallel with provenance
	var sbomResource sdk.Resource
	if security.SBOM != nil && security.SBOM.Enabled {
		format := "cyclonedx-json"
		if security.SBOM.Format != "" {
			format = security.SBOM.Format
		}

		// Generate SBOM
		sbomGenCmd, err := local.NewCommand(ctx, fmt.Sprintf("sbom-gen-%s", imageName), &local.CommandArgs{
			Create: imageUrl.ApplyT(func(img string) string {
				return fmt.Sprintf("sc sbom generate --image %s --format %s --output /tmp/sbom-%s.json", img, format, imageName)
			}).(sdk.StringOutput),
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sbom generate command")
		}

		// Attach SBOM as attestation (if enabled)
		if security.SBOM.Output != nil && security.SBOM.Output.Registry {
			attachFlag := "--keyless"
			if security.Signing != nil && security.Signing.PrivateKey != "" {
				attachFlag = fmt.Sprintf("--key %s", security.Signing.PrivateKey)
			}

			sbomAttCmd, err := local.NewCommand(ctx, fmt.Sprintf("sbom-att-%s", imageName), &local.CommandArgs{
				Create: imageUrl.ApplyT(func(img string) string {
					return fmt.Sprintf("%ssc sbom attach --image %s --sbom /tmp/sbom-%s.json %s", envPrefix, img, imageName, attachFlag)
				}).(sdk.StringOutput),
			}, sdk.DependsOn([]sdk.Resource{sbomGenCmd}))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create sbom attach command")
			}
			sbomResource = sbomAttCmd
		} else {
			sbomResource = sbomGenCmd
		}
	}

	// Step 3b: Provenance Attestation (if enabled) - runs in parallel with SBOM
	var provenanceResource sdk.Resource
	if security.Provenance != nil && security.Provenance.Enabled {
		attachFlag := "--keyless"
		if security.Signing != nil && security.Signing.PrivateKey != "" {
			attachFlag = fmt.Sprintf("--key %s", security.Signing.PrivateKey)
		}

		provAttCmd, err := local.NewCommand(ctx, fmt.Sprintf("prov-att-%s", imageName), &local.CommandArgs{
			Create: imageUrl.ApplyT(func(img string) string {
				return fmt.Sprintf("%ssc provenance attach --image %s %s", envPrefix, img, attachFlag)
			}).(sdk.StringOutput),
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create provenance attach command")
		}
		provenanceResource = provAttCmd
	}

	// Collect all final resources for dependency
	finalResources := []sdk.Resource{}
	if sbomResource != nil {
		finalResources = append(finalResources, sbomResource)
	}
	if provenanceResource != nil {
		finalResources = append(finalResources, provenanceResource)
	}
	if len(finalResources) == 0 {
		finalResources = append(finalResources, lastResource)
	}

	opts = append(opts, sdk.DependsOn(finalResources))
	return opts, nil
}
