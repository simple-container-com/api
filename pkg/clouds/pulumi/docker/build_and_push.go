package docker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// repoDigestRe matches a Docker image digest reference: name@sha256:<64 hex>.
// Used by resolveSecurityImageRef to reject truncated or malformed digests.
var repoDigestRe = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)

// Image describes a container image to build and push via Pulumi.
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
	// CacheFrom is an optional list of registry image references to use as
	// BuildKit layer cache sources (--cache-from). When set, BUILDKIT_INLINE_CACHE=1
	// is injected into build args so the pushed image embeds cache metadata.
	//
	// SLSA L3 hermeticity note: CacheFrom pulls layers from the registry, which
	// technically breaks strict build hermeticity. This is an accepted deviation —
	// all post-push security operations use the immutable content digest, so the
	// cache source cannot affect signing or attestation integrity.
	CacheFrom sdk.StringArrayInput
}

// ImageOut is the result of BuildAndPushImage.
type ImageOut struct {
	Image   *docker.Image
	AddOpts []sdk.ResourceOption
}

// BuildAndPushImage builds a Docker image, pushes it, and runs security
// operations (scan, sign, verify, SBOM, provenance) in parallel.
//
// The service update (ECS task definition / K8s deployment) depends on
// ImageOut.AddOpts, which gates on sign+verify — not on scan. Scan runs
// parallel and reports findings without blocking the deploy.
func BuildAndPushImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image Image) (*ImageOut, error) {
	imageFullUrl := image.RepositoryUrl.ApplyT(func(repoUri string) string {
		if image.RepositoryUrlWithImage {
			return fmt.Sprintf("%s:%s", repoUri, image.Version)
		}
		return fmt.Sprintf("%s/%s:%s", repoUri, image.Name, image.Version)
	}).(sdk.StringOutput)
	params.Log.Info(ctx.Context(), "building and pushing docker image %q (from %q) for stack %q env %q",
		image.Name, image.Context, stack.Name, deployParams.Environment)

	platform := lo.If(image.Platform != nil, lo.FromPtr(image.Platform)).Else(string(api.ImagePlatformLinuxAmd64))
	args := sdk.StringMap{"VERSION": sdk.String(image.Version)}
	for k, v := range image.Args {
		args[k] = sdk.String(v)
	}

	var cacheFromArg docker.CacheFromPtrInput
	var builderVersion docker.BuilderVersionPtrInput
	if image.CacheFrom != nil {
		cacheFromArg = &docker.CacheFromArgs{Images: image.CacheFrom}
		builderVersion = docker.BuilderVersionBuilderBuildKit.ToBuilderVersionPtrOutput()
		args["BUILDKIT_INLINE_CACHE"] = sdk.String("1")
	}

	res, err := docker.NewImage(ctx, image.Name, &docker.ImageArgs{
		Build: &docker.DockerBuildArgs{
			Context:        sdk.String(image.Context),
			Dockerfile:     sdk.String(image.Dockerfile),
			Args:           args,
			Platform:       sdk.String(platform),
			CacheFrom:      cacheFromArg,
			BuilderVersion: builderVersion,
		},
		SkipPush:  sdk.Bool(ctx.DryRun()),
		ImageName: imageFullUrl,
		Registry:  image.Registry,
	}, append(image.ProviderOptions, sdk.DependsOn(params.ComputeContext.Dependencies()))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build and push image %q for stack %q env %q", image.Name, stack.Name, deployParams.Environment)
	}

	var addOpts []sdk.ResourceOption
	if stack.Client.Security != nil && stack.Client.Security.Enabled {
		securityOpts, err := executeSecurityOperations(ctx, stack, res, image)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to execute security operations for image %q", image.Name)
		}
		addOpts = append(addOpts, securityOpts...)
	}
	if len(addOpts) == 0 {
		addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{res}))
	}

	return &ImageOut{Image: res, AddOpts: addOpts}, nil
}

// executeSecurityOperations creates Pulumi resources for post-push security ops.
//
// Dependency graph (softFail=true, default):
//
//	push ──→ sign ──→ verify          ← deploy waits here (10s overhead)
//	push ──→ scan ──→ report          ← parallel, does not block deploy
//	push ──→ sbom-gen ──→ sbom-att    ← sbom-att waits for sign + sbom-gen
//	push ──→ prov-gen ──→ prov-att    ← prov-att waits for sign + prov-gen
//
// Dependency graph (softFail=false, future enforcement):
//
//	push ──→ scan ──→ sign ──→ verify ← scan gates sign; deploy waits here
//	push ──→ sbom-gen ──→ sbom-att    ← sbom-att waits for sign + sbom-gen
//	push ──→ prov-gen ──→ prov-att    ← prov-att waits for sign + prov-gen
//
// All operations use the immutable content digest (name@sha256:...) returned
// by the push step. No mutable tags are used after push.
func executeSecurityOperations(ctx *sdk.Context, stack api.Stack, dockerImage *docker.Image, image Image) ([]sdk.ResourceOption, error) {
	security := stack.Client.Security
	var opts []sdk.ResourceOption
	imageName := image.Name

	securityImageRef := resolveSecurityImageRef(ctx, dockerImage.RepoDigest, dockerImage.ImageName)
	baseDeps := []sdk.Resource{dockerImage}

	// Ensure Docker is authenticated to the image registry so cosign and other
	// tools can access image manifests for signing, verification, and attestation.
	// The Pulumi Docker provider uses its own auth mechanism that doesn't populate
	// the Docker credential store — we need an explicit docker login.
	if image.Registry.Server != nil && image.Registry.Password != nil {
		loginCmd, err := local.NewCommand(ctx, fmt.Sprintf("registry-login-%s", imageName), &local.CommandArgs{
			Create: sdk.All(image.Registry.Server, image.Registry.Username, image.Registry.Password).ApplyT(func(args []interface{}) string {
				server, _ := args[0].(string)
				username, _ := args[1].(string)
				password, _ := args[2].(string)
				if username == "" {
					username = "AWS" // ECR default
				}
				return fmt.Sprintf("echo %s | docker login --username %s --password-stdin %s",
					shellQuote(password), shellQuote(username), shellQuote(server))
			}).(sdk.StringOutput),
		}, sdk.DependsOn(baseDeps))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create registry login for image %q", imageName)
		}
		baseDeps = []sdk.Resource{loginCmd}
	}

	softFail := security.Scan != nil && security.Scan.SoftFail

	// --- Scan (parallel from push, reports findings, gates sign only when softFail=false) ---
	var scanGate sdk.Resource
	if security.Scan != nil && security.Scan.Enabled {
		scanCmd, err := createScanCommands(ctx, stack, securityImageRef, image, baseDeps)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create scan commands for image %q", imageName)
		}
		scanGate = scanCmd
	}

	// signDeps controls whether sign waits for scan.
	// softFail=true  → sign starts immediately after push (velocity)
	// softFail=false → sign waits for scan to pass (enforcement)
	signDeps := baseDeps
	if !softFail && scanGate != nil {
		signDeps = []sdk.Resource{scanGate}
	}

	// --- Sign + Verify ---
	var signCmd *local.Command
	if security.Signing != nil && security.Signing.Enabled {
		var err error
		signCmd, err = local.NewCommand(ctx, fmt.Sprintf("sign-%s", imageName), &local.CommandArgs{
			Create: securityImageRef.ApplyT(func(img string) string {
				args := []string{"sc", "image", "sign", "--image", img}
				args = append(args, signingCLIArgs(security.Signing)...)
				for i, arg := range args {
					args[i] = shellQuote(arg)
				}
				return strings.Join(args, " ")
			}).(sdk.StringOutput),
			Environment: signingCommandEnvironment(security.Signing),
		}, sdk.DependsOn(signDeps))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sign command for image %q", imageName)
		}
	}

	var verifyCmd *local.Command
	if signCmd != nil && security.Signing.Verify != nil && security.Signing.Verify.Enabled {
		if security.Signing.Keyless {
			if security.Signing.Verify.OIDCIssuer == "" {
				return nil, errors.Errorf("signing.verify.oidcIssuer is required for keyless verification of image %q", imageName)
			}
			if security.Signing.Verify.IdentityRegexp == "" {
				return nil, errors.Errorf("signing.verify.identityRegexp is required for keyless verification of image %q", imageName)
			}
		} else if security.Signing.PublicKey == "" {
			return nil, errors.Errorf("signing.publicKey is required for key-based verification of image %q", imageName)
		}

		var err error
		verifyCmd, err = local.NewCommand(ctx, fmt.Sprintf("verify-%s", imageName), &local.CommandArgs{
			Create: securityImageRef.ApplyT(func(img string) string {
				args := []string{"sc", "image", "verify", "--image", img}
				if security.Signing.Keyless {
					args = append(args,
						"--oidc-issuer", security.Signing.Verify.OIDCIssuer,
						"--identity-regexp", security.Signing.Verify.IdentityRegexp,
					)
				} else {
					args = append(args, "--public-key", security.Signing.PublicKey)
				}
				for i, arg := range args {
					args[i] = shellQuote(arg)
				}
				return strings.Join(args, " ")
			}).(sdk.StringOutput),
			Environment: verifyCommandEnvironment(security.Signing),
		}, sdk.DependsOn([]sdk.Resource{signCmd}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create verify command for image %q", imageName)
		}
	}

	// --- SBOM Generation + Attestation (parallel from push, att waits for sign) ---
	var sbomResource sdk.Resource
	if security.SBOM != nil && security.SBOM.Enabled {
		attachSBOM := shouldAttachSBOM(security.SBOM)
		if err := validateSBOMDescriptor(security.SBOM); err != nil {
			if security.SBOM.Required {
				return nil, err
			}
			fmt.Printf("Warning: skipping SBOM attachment for %s: %v\n", imageName, err)
			attachSBOM = false
		}
		format := "cyclonedx-json"
		if security.SBOM.Format != "" {
			format = security.SBOM.Format
		}
		sbomOutputPath := resolveSBOMOutputPath(security, imageName)

		sbomGenCmd, err := local.NewCommand(ctx, fmt.Sprintf("sbom-gen-%s", imageName), &local.CommandArgs{
			Create: securityImageRef.ApplyT(func(img string) string {
				args := []string{
					"sc", "sbom", "generate",
					"--image", img,
					"--format", format,
					"--output", sbomOutputPath,
				}
				if security.SBOM.Cache != nil && security.SBOM.Cache.Enabled && security.SBOM.Cache.Dir != "" {
					args = append(args, "--cache-dir", security.SBOM.Cache.Dir)
				}
				for i, arg := range args {
					args[i] = shellQuote(arg)
				}
				return strings.Join(args, " ")
			}).(sdk.StringOutput),
		}, sdk.DependsOn(baseDeps))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sbom generate command for image %q", imageName)
		}

		if attachSBOM {
			if !signingEnabled(security) {
				if security.SBOM.Required {
					return nil, errors.Errorf("sbom attachment requires security.signing.enabled=true")
				}
				fmt.Printf("Warning: skipping SBOM attachment for %s because signing is disabled\n", imageName)
				sbomResource = sbomGenCmd
			} else {
				sbomAttDeps := []sdk.Resource{sbomGenCmd}
				if signCmd != nil {
					sbomAttDeps = append(sbomAttDeps, signCmd)
				}
				sbomAttCmd, err := local.NewCommand(ctx, fmt.Sprintf("sbom-att-%s", imageName), &local.CommandArgs{
					Create: securityImageRef.ApplyT(func(img string) string {
						args := []string{"sc", "sbom", "attach", "--image", img, "--sbom", sbomOutputPath}
						args = append(args, signingCLIArgs(security.Signing)...)
						for i, arg := range args {
							args[i] = shellQuote(arg)
						}
						return strings.Join(args, " ")
					}).(sdk.StringOutput),
					Environment: signingCommandEnvironment(security.Signing),
				}, sdk.DependsOn(sbomAttDeps))
				if err != nil {
					return nil, errors.Wrapf(err, "failed to create sbom attach command for image %q", imageName)
				}
				sbomResource = sbomAttCmd
			}
		} else {
			sbomResource = sbomGenCmd
		}
	}

	// --- Provenance Generation + Attestation (parallel from push, att waits for sign) ---
	var provenanceResource sdk.Resource
	if security.Provenance != nil && security.Provenance.Enabled {
		provenanceCommand := "generate"
		skipProvenance := false
		if shouldAttachProvenance(security.Provenance) {
			if !signingEnabled(security) {
				if security.Provenance.Required {
					return nil, errors.Errorf("provenance.output.registry requires security.signing.enabled=true")
				}
				if resolveProvenanceOutputPath(security, imageName) == "" {
					fmt.Printf("Warning: skipping provenance for %s because registry attachment requires signing and no local output was configured\n", imageName)
					skipProvenance = true
				} else {
					fmt.Printf("Warning: generating provenance locally for %s because registry attachment requires signing\n", imageName)
				}
			} else {
				provenanceCommand = "attach"
			}
		}
		if !skipProvenance {
			format := "slsa-v1.0"
			if security.Provenance.Format != "" {
				format = security.Provenance.Format
			}
			provenanceOutputPath := resolveProvenanceOutputPath(security, imageName)

			provGenDeps := baseDeps
			var provEnv sdk.StringMap
			if provenanceCommand == "attach" {
				provEnv = signingCommandEnvironment(security.Signing)
				// When attaching, the provenance attestation needs signing creds.
				// Generation starts from push; attachment waits for sign.
				if signCmd != nil {
					provGenDeps = []sdk.Resource{signCmd}
				}
			}

			provCmd, err := local.NewCommand(ctx, fmt.Sprintf("prov-att-%s", imageName), &local.CommandArgs{
				Create: securityImageRef.ApplyT(func(img string) string {
					args := []string{
						"sc", "provenance", provenanceCommand,
						"--image", img,
						"--format", format,
						fmt.Sprintf("--include-git=%t", security.Provenance.IncludeGit),
						fmt.Sprintf("--include-dockerfile=%t", security.Provenance.IncludeDocker),
						"--source-root", ".",
					}
					if image.Context != "" {
						args = append(args, "--context", image.Context)
					}
					if image.Dockerfile != "" {
						args = append(args, "--dockerfile", image.Dockerfile)
					}
					if provenanceOutputPath != "" {
						args = append(args, "--output", provenanceOutputPath)
					}
					if security.Provenance.Builder != nil && security.Provenance.Builder.ID != "" {
						args = append(args, "--builder-id", security.Provenance.Builder.ID)
					}
					if security.Provenance.Metadata != nil {
						args = append(args,
							fmt.Sprintf("--include-env=%t", security.Provenance.Metadata.IncludeEnv),
							fmt.Sprintf("--include-materials=%t", security.Provenance.Metadata.IncludeMaterials),
						)
					}
					if provenanceCommand == "attach" {
						args = append(args, signingCLIArgs(security.Signing)...)
					}
					for i, arg := range args {
						args[i] = shellQuote(arg)
					}
					return strings.Join(args, " ")
				}).(sdk.StringOutput),
				Environment: provEnv,
			}, sdk.DependsOn(provGenDeps))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create provenance command for image %q", imageName)
			}
			provenanceResource = provCmd
		}
	}

	// Fan-in: the caller (ECS/K8s service update) depends on these resources.
	// The sign branch terminal is verify (if enabled) or sign.
	// When softFail=false and signing is disabled, scan must still gate deploy.
	finalResources := make([]sdk.Resource, 0, 4)
	switch {
	case verifyCmd != nil:
		finalResources = append(finalResources, verifyCmd)
	case signCmd != nil:
		finalResources = append(finalResources, signCmd)
	}
	if sbomResource != nil {
		finalResources = append(finalResources, sbomResource)
	}
	if provenanceResource != nil {
		finalResources = append(finalResources, provenanceResource)
	}
	// When softFail=false and scan ran but signing is disabled, scan must
	// still gate the service update — otherwise scan failures are ignored.
	if !softFail && scanGate != nil && signCmd == nil {
		finalResources = append(finalResources, scanGate)
	}
	if len(finalResources) == 0 {
		finalResources = append(finalResources, dockerImage)
	}

	opts = append(opts, sdk.DependsOn(finalResources))
	return opts, nil
}

// createScanCommands creates Pulumi local.Command resource(s) for vulnerability scanning.
// Returns the last resource in the scan chain (merge barrier for multi-tool scans).
func createScanCommands(ctx *sdk.Context, stack api.Stack, imageRef sdk.StringOutput, image Image, deps []sdk.Resource) (sdk.Resource, error) {
	security := stack.Client.Security
	imageName := image.Name

	enabledTools := enabledScanDescriptors(security.Scan.Tools)
	if len(enabledTools) == 0 {
		if security.Scan.Required {
			return nil, errors.Errorf("security.scan.enabled requires at least one enabled scan tool")
		}
		fmt.Printf("Warning: skipping image scanning for %s because no enabled scan tools were configured\n", imageName)
		return nil, nil //nolint:nilnil
	}

	// Duplicate tool names produce Pulumi resources with the same logical name.
	seenTools := make(map[string]bool, len(enabledTools))
	for _, t := range enabledTools {
		if seenTools[t.Name] {
			return nil, errors.Errorf("security.scan.tools: duplicate tool name %q", t.Name)
		}
		seenTools[t.Name] = true
	}

	if len(enabledTools) == 1 || canUseMergedScanCommand(enabledTools) {
		scanToolName := enabledTools[0].Name
		required := security.Scan.Required || enabledTools[0].Required
		failOn := effectiveScanThreshold(security.Scan.FailOn, enabledTools[0].FailOn)
		warnOn := effectiveScanThreshold(security.Scan.WarnOn, enabledTools[0].WarnOn)
		if len(enabledTools) > 1 {
			scanToolName = "all"
			required = security.Scan.Required
			failOn = security.Scan.FailOn
			warnOn = security.Scan.WarnOn
		}

		scanCmdArgs := buildScanCmdArgs(security, imageName, scanToolName, required, failOn, warnOn,
			len(enabledTools) == 1, security.Scan.SoftFail)

		scanName := fmt.Sprintf("scan-%s-%s", imageName, scanToolName)
		if len(enabledTools) > 1 {
			scanName = fmt.Sprintf("scan-%s-merged", imageName)
		}
		return createScanLocalCommand(ctx, scanName, imageRef, scanCmdArgs,
			scanCommandEnvironment(security.Reporting), deps)
	}

	// Multi-tool with per-tool overrides: parallel scans + merge barrier.
	toolResultPaths := make([]string, 0, len(enabledTools))
	toolCmds := make([]sdk.Resource, 0, len(enabledTools))
	needsArtifacts := needsMergedScanArtifacts(security, imageName)

	for _, tool := range enabledTools {
		failOn := effectiveScanThreshold(security.Scan.FailOn, tool.FailOn)
		warnOn := effectiveScanThreshold(security.Scan.WarnOn, tool.WarnOn)
		scanCmdArgs := buildScanCoreArgs(security.Scan, tool.Name,
			security.Scan.Required || tool.Required, failOn, warnOn, security.Scan.SoftFail)

		outputPath := resolveToolScanOutputPath(security, imageName, tool.Name, true)
		if outputPath == "" && needsArtifacts {
			outputPath = tempArtifactPath(fmt.Sprintf("scan-results-%s", tool.Name), imageName, "json")
		}
		appendScanOutputArg(&scanCmdArgs, outputPath)
		if outputPath != "" {
			toolResultPaths = append(toolResultPaths, outputPath)
		}

		cmd, err := createScanLocalCommand(ctx, fmt.Sprintf("scan-%s-%s", imageName, tool.Name),
			imageRef, scanCmdArgs, scanCommandEnvironment(security.Reporting), deps)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create %s scan command for image %q", tool.Name, imageName)
		}
		toolCmds = append(toolCmds, cmd)
	}

	if !needsArtifacts {
		// No merge command needed, but we still need a single barrier resource
		// that depends on ALL parallel tool scans — not just the last one.
		// Create a lightweight barrier command that waits for all scans.
		barrier, err := local.NewCommand(ctx, fmt.Sprintf("scan-%s-barrier", imageName), &local.CommandArgs{
			Create: sdk.String("echo 'all scans completed'"),
		}, sdk.DependsOn(toolCmds))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create scan barrier for image %q", imageName)
		}
		return barrier, nil
	}

	mergedCmdArgs := []string{
		"sc", "image", "scan",
		"--image", "%s",
		fmt.Sprintf("--required=%t", security.Scan.Required),
	}
	for _, inputPath := range toolResultPaths {
		mergedCmdArgs = append(mergedCmdArgs, "--input", inputPath)
	}
	if security.Scan.FailOn != "" {
		mergedCmdArgs = append(mergedCmdArgs, "--fail-on", security.Scan.FailOn)
	}
	if security.Scan.WarnOn != "" {
		mergedCmdArgs = append(mergedCmdArgs, "--warn-on", security.Scan.WarnOn)
	}
	if security.Scan.SoftFail {
		mergedCmdArgs = append(mergedCmdArgs, "--soft-fail")
	}
	appendScanOutputArg(&mergedCmdArgs, resolveScanOutputPath(security, imageName))
	if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
		mergedCmdArgs = appendDefectDojoFlags(mergedCmdArgs, security.Reporting.DefectDojo)
	}
	if commentOutput := resolveCommentOutputPath(security, imageName); commentOutput != "" {
		mergedCmdArgs = append(mergedCmdArgs, "--comment-output", commentOutput)
	}
	return createScanLocalCommand(ctx, fmt.Sprintf("scan-%s-merged", imageName),
		imageRef, mergedCmdArgs, scanCommandEnvironment(security.Reporting), toolCmds)
}

// buildScanCoreArgs constructs the base sc image scan argument list without
// output or reporting flags. Used by both single-command and per-tool paths.
func buildScanCoreArgs(scan *api.ScanDescriptor, toolName string, required bool, failOn, warnOn string, softFail bool) []string {
	args := []string{
		"sc", "image", "scan",
		"--image", "%s",
		"--tool", toolName,
		fmt.Sprintf("--required=%t", required),
	}
	appendScanCacheArg(&args, scan)
	if failOn != "" {
		args = append(args, "--fail-on", failOn)
	}
	if warnOn != "" {
		args = append(args, "--warn-on", warnOn)
	}
	if softFail {
		args = append(args, "--soft-fail")
	}
	return args
}

// buildScanCmdArgs constructs the full sc image scan argument list including
// output and reporting flags for single-tool or merged (--tool all) commands.
func buildScanCmdArgs(security *api.SecurityDescriptor, imageName, toolName string, required bool, failOn, warnOn string, singleTool, softFail bool) []string {
	args := buildScanCoreArgs(security.Scan, toolName, required, failOn, warnOn, softFail)
	if singleTool {
		appendScanOutputArg(&args, resolveToolScanOutputPath(security, imageName, toolName, false))
	} else {
		appendScanOutputArg(&args, resolveScanOutputPath(security, imageName))
	}
	if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
		args = appendDefectDojoFlags(args, security.Reporting.DefectDojo)
	}
	if commentOutput := resolveCommentOutputPath(security, imageName); commentOutput != "" {
		args = append(args, "--comment-output", commentOutput)
	}
	return args
}

// createScanLocalCommand creates a local.Command for sc image scan.
func createScanLocalCommand(ctx *sdk.Context, name string, imageRef sdk.StringOutput, scanCmdArgs []string, env sdk.StringMap, deps []sdk.Resource) (*local.Command, error) {
	return local.NewCommand(ctx, name, &local.CommandArgs{
		Create: imageRef.ApplyT(func(img string) string {
			parts := make([]string, 0, len(scanCmdArgs))
			for _, arg := range scanCmdArgs {
				if arg == "%s" {
					parts = append(parts, shellQuote(img))
					continue
				}
				parts = append(parts, shellQuote(arg))
			}
			return strings.Join(parts, " ")
		}).(sdk.StringOutput),
		Environment: env,
	}, sdk.DependsOn(deps))
}

// --- Helper functions ---

func enabledScanDescriptors(tools []api.ScanToolDescriptor) []api.ScanToolDescriptor {
	enabled := make([]api.ScanToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		if scanDescriptorEnabled(tool) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

func resolveScanOutputPath(security *api.SecurityDescriptor, imageName string) string {
	if security.Scan != nil && security.Scan.Output != nil && security.Scan.Output.Local != "" {
		return security.Scan.Output.Local
	}
	return ""
}

func resolveToolScanOutputPath(security *api.SecurityDescriptor, imageName, toolName string, split bool) string {
	basePath := resolveScanOutputPath(security, imageName)
	if basePath == "" {
		return ""
	}
	if !split {
		return basePath
	}
	return appendPathSuffix(basePath, toolName)
}

func resolveSBOMOutputPath(security *api.SecurityDescriptor, imageName string) string {
	if security.SBOM != nil && security.SBOM.Output != nil && security.SBOM.Output.Local != "" {
		return security.SBOM.Output.Local
	}
	return tempArtifactPath("sbom", imageName, "json")
}

func resolveProvenanceOutputPath(security *api.SecurityDescriptor, imageName string) string {
	if security.Provenance != nil && security.Provenance.Output != nil && security.Provenance.Output.Local != "" {
		return security.Provenance.Output.Local
	}
	return ""
}

// needsMergedScanArtifacts reports whether any configured output, PR comment, or
// DefectDojo upload requires a merged scan result file.
func needsMergedScanArtifacts(security *api.SecurityDescriptor, imageName string) bool {
	if resolveScanOutputPath(security, imageName) != "" {
		return true
	}
	if resolveCommentOutputPath(security, imageName) != "" {
		return true
	}
	return security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled
}

func resolveCommentOutputPath(security *api.SecurityDescriptor, imageName string) string {
	if security.Reporting == nil || security.Reporting.PRComment == nil || !security.Reporting.PRComment.Enabled {
		return ""
	}
	if security.Reporting.PRComment.Output != "" {
		return security.Reporting.PRComment.Output
	}
	return tempArtifactPath("scan-comment", imageName, "md")
}

func tempArtifactPath(prefix, imageName, extension string) string {
	safeName := strings.NewReplacer("/", "-", ":", "-", "@", "-", "\\", "-").Replace(imageName)
	return filepath.Join("/tmp", fmt.Sprintf("%s-%s.%s", prefix, safeName, extension))
}

func appendPathSuffix(path, suffix string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + "-" + suffix
	}
	return strings.TrimSuffix(path, ext) + "-" + suffix + ext
}

func signingCommandEnvironment(cfg *api.SigningDescriptor) sdk.StringMap {
	if cfg == nil {
		return nil
	}
	env := sdk.StringMap{}
	if cfg.Keyless {
		env["COSIGN_EXPERIMENTAL"] = sdk.String("1")
	}
	if !cfg.Keyless && cfg.PrivateKey != "" {
		env["COSIGN_PASSWORD"] = sdk.ToSecret(cfg.Password).(sdk.StringOutput)
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// verifyCommandEnvironment returns the minimal environment for cosign verify.
// Unlike signingCommandEnvironment, it never includes COSIGN_PASSWORD — verify
// uses a public key or Fulcio certificate chain, never the private key passphrase.
func verifyCommandEnvironment(cfg *api.SigningDescriptor) sdk.StringMap {
	if cfg == nil {
		return nil
	}
	if cfg.Keyless {
		return sdk.StringMap{
			"COSIGN_EXPERIMENTAL": sdk.String("1"),
		}
	}
	return nil
}

func scanCommandEnvironment(cfg *api.ReportingDescriptor) sdk.StringMap {
	if cfg == nil || cfg.DefectDojo == nil || !cfg.DefectDojo.Enabled || cfg.DefectDojo.APIKey == "" {
		return nil
	}
	return sdk.StringMap{
		"DEFECTDOJO_API_KEY": sdk.ToSecret(cfg.DefectDojo.APIKey).(sdk.StringOutput),
	}
}

func effectiveScanThreshold(global, tool string) string {
	if tool != "" {
		return tool
	}
	return global
}

func appendScanOutputArg(args *[]string, outputPath string) {
	if outputPath == "" {
		return
	}
	*args = append(*args, "--output", outputPath)
}

func appendScanCacheArg(args *[]string, config *api.ScanDescriptor) {
	if config == nil || config.Cache == nil || !config.Cache.Enabled || config.Cache.Dir == "" {
		return
	}
	*args = append(*args, "--cache-dir", config.Cache.Dir)
}

func canUseMergedScanCommand(tools []api.ScanToolDescriptor) bool {
	if len(tools) <= 1 {
		return false
	}
	for _, tool := range tools {
		if tool.Required || tool.FailOn != "" || tool.WarnOn != "" {
			return false
		}
	}
	return true
}

func resolveSecurityImageRef(ctx *sdk.Context, repoDigest, imageURL sdk.StringOutput) sdk.StringOutput {
	return sdk.All(repoDigest, imageURL).ApplyT(func(values []interface{}) (string, error) {
		repoDigestValue, _ := values[0].(string)
		imageURLValue, _ := values[1].(string)
		if repoDigestValue != "" {
			if !repoDigestRe.MatchString(repoDigestValue) {
				return "", errors.Errorf("repo digest %q is not a valid immutable digest (expected name@sha256:<64 hex chars>)", repoDigestValue)
			}
			return repoDigestValue, nil
		}
		if ctx.DryRun() {
			return imageURLValue, nil
		}
		return "", errors.Errorf("docker image repo digest is unavailable for %s; security operations require an immutable digest", imageURLValue)
	}).(sdk.StringOutput)
}

func scanDescriptorEnabled(tool api.ScanToolDescriptor) bool {
	if tool.Enabled != nil {
		return *tool.Enabled
	}
	return tool.Required || tool.FailOn != "" || tool.WarnOn != "" || tool.Name != ""
}

func shouldAttachSBOM(config *api.SBOMDescriptor) bool {
	if config == nil {
		return false
	}
	if config.Output != nil && config.Output.Registry {
		return true
	}
	return config.Attach != nil && config.Attach.Enabled
}

func validateSBOMDescriptor(config *api.SBOMDescriptor) error {
	if config == nil || !config.Enabled {
		return nil
	}
	if config.Attach != nil && config.Attach.Enabled && !config.Attach.Sign {
		return errors.Errorf("sbom.attach.sign must be true when sbom.attach.enabled=true")
	}
	if config.Output != nil && config.Output.Registry && config.Attach != nil {
		if !config.Attach.Enabled {
			return errors.Errorf("sbom.attach.enabled=false is not compatible with sbom.output.registry=true")
		}
		if !config.Attach.Sign {
			return errors.Errorf("sbom.attach.sign=false is not compatible with sbom.output.registry=true")
		}
	}
	return nil
}

func signingEnabled(security *api.SecurityDescriptor) bool {
	return security != nil && security.Signing != nil && security.Signing.Enabled
}

func provenanceRegistryOutputEnabled(config *api.ProvenanceDescriptor) bool {
	return config != nil && config.Output != nil && config.Output.Registry
}

func shouldAttachProvenance(config *api.ProvenanceDescriptor) bool {
	return provenanceRegistryOutputEnabled(config)
}

func signingCLIArgs(cfg *api.SigningDescriptor) []string {
	if cfg == nil {
		return nil
	}
	if cfg.Keyless {
		return []string{"--keyless"}
	}
	if cfg.PrivateKey != "" {
		return []string{"--key", cfg.PrivateKey}
	}
	return nil
}

func appendDefectDojoFlags(args []string, config *api.DefectDojoDescriptor) []string {
	args = append(args,
		"--upload-defectdojo",
		"--defectdojo-url", config.URL,
		"--defectdojo-auto-create="+fmt.Sprintf("%t", config.AutoCreate),
	)
	if config.EngagementID > 0 {
		args = append(args, "--defectdojo-engagement-id", fmt.Sprintf("%d", config.EngagementID))
	}
	if config.EngagementName != "" {
		args = append(args, "--defectdojo-engagement-name", config.EngagementName)
	}
	if config.ProductID > 0 {
		args = append(args, "--defectdojo-product-id", fmt.Sprintf("%d", config.ProductID))
	}
	if config.ProductName != "" {
		args = append(args, "--defectdojo-product-name", config.ProductName)
	}
	if config.TestType != "" {
		args = append(args, "--defectdojo-test-type", config.TestType)
	}
	if config.Environment != "" {
		args = append(args, "--defectdojo-environment", config.Environment)
	}
	for _, tag := range config.Tags {
		args = append(args, "--defectdojo-tag", tag)
	}
	return args
}

// shellQuote wraps value in POSIX single quotes, escaping embedded quotes.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
