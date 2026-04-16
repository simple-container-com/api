package docker

import (
	"fmt"
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
		securityOpts, err := executeSecurityOperations(ctx, stack, res, image, deployParams.Environment)
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
//	push ──→ sign ──→ verify-image                ← deploy waits here
//	push ──→ scan ──→ report                     ← parallel, does not block deploy
//	push ──→ sbom-gen ──→ sbom-att ──→ verify-sbom  ← attestation verified
//	push ──→ prov-gen ──→ prov-att ──→ verify-prov  ← attestation verified
//
// Dependency graph (softFail=false, future enforcement):
//
//	push ──→ scan ──→ sign ──→ verify-image      ← scan gates sign
//	push ──→ sbom-gen ──→ sbom-att ──→ verify-sbom
//	push ──→ prov-gen ──→ prov-att ──→ verify-prov
//
// All operations use the immutable content digest (name@sha256:...) returned
// by the push step. No mutable tags are used after push.
func executeSecurityOperations(ctx *sdk.Context, stack api.Stack, dockerImage *docker.Image, image Image, environment string) ([]sdk.ResourceOption, error) {
	security := stack.Client.Security
	var opts []sdk.ResourceOption
	imageName := image.Name

	// DefectDojo engagement: "PR-{number}" for PR deploys, configured name for main.
	if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
		if strings.HasPrefix(environment, "pr") {
			num := strings.TrimPrefix(environment, "pr")
			if num != "" && num[0] >= '0' && num[0] <= '9' {
				security.Reporting.DefectDojo.EngagementName = "PR-" + num
			}
		}
	}

	securityImageRef := resolveSecurityImageRef(ctx, dockerImage.RepoDigest, dockerImage.ImageName)
	baseDeps := []sdk.Resource{dockerImage}

	baseDeps, err := createRegistryLogin(ctx, image, imageName, baseDeps)
	if err != nil {
		return nil, err
	}

	softFail := security.Scan != nil && security.Scan.SoftFail

	// --- Scan ---
	var scanGate sdk.Resource
	if security.Scan != nil && security.Scan.Enabled {
		scanCmd, err := createScanCommands(ctx, stack, securityImageRef, image, baseDeps)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create scan commands for image %q", imageName)
		}
		scanGate = scanCmd
	}

	signDeps := baseDeps
	if !softFail && scanGate != nil {
		signDeps = []sdk.Resource{scanGate}
	}

	// --- Sign + Verify ---
	signCmd, verifyCmd, err := createSignVerifyCommands(ctx, security, securityImageRef, imageName, signDeps)
	if err != nil {
		return nil, err
	}

	// --- SBOM ---
	sbomResource, err := createSBOMCommands(ctx, security, securityImageRef, imageName, image, baseDeps, signCmd)
	if err != nil {
		return nil, err
	}

	// --- Provenance ---
	provenanceResource, err := createProvenanceCommands(ctx, security, securityImageRef, imageName, image, baseDeps, signCmd)
	if err != nil {
		return nil, err
	}

	// Fan-in: the caller (ECS/K8s service update) depends on these resources.
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
	if !softFail && scanGate != nil && signCmd == nil {
		finalResources = append(finalResources, scanGate)
	}
	if len(finalResources) == 0 {
		finalResources = append(finalResources, dockerImage)
	}

	// --- Security Report ---
	reportDeps := make([]sdk.Resource, len(finalResources))
	copy(reportDeps, finalResources)
	if scanGate != nil {
		reportDeps = append(reportDeps, scanGate)
	}
	reportCmd, err := local.NewCommand(ctx, fmt.Sprintf("security-report-%s", imageName), &local.CommandArgs{
		Create: securityImageRef.ApplyT(func(img string) string {
			commentOutput := resolveCommentOutputPath(security, imageName)
			return buildSecurityReportScript(img, imageName, security, commentOutput)
		}).(sdk.StringOutput),
	}, sdk.DependsOn(reportDeps))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create security report for image %q", imageName)
	}
	finalResources = []sdk.Resource{reportCmd}

	opts = append(opts, sdk.DependsOn(finalResources))
	return opts, nil
}

// createRegistryLogin writes registry credentials to ~/.docker/config.json
// so security tools (cosign, grype, trivy, syft) can authenticate.
func createRegistryLogin(ctx *sdk.Context, image Image, imageName string, baseDeps []sdk.Resource) ([]sdk.Resource, error) {
	if image.Registry.Server == nil {
		return baseDeps, nil
	}
	var loginInputs sdk.ArrayOutput
	if image.Registry.Password != nil && image.Registry.Username != nil {
		loginInputs = sdk.All(image.Registry.Server, image.Registry.Username, image.Registry.Password)
	} else if image.Registry.Password != nil {
		loginInputs = sdk.All(image.Registry.Server, sdk.String(""), image.Registry.Password)
	} else {
		loginInputs = sdk.All(image.Registry.Server, sdk.String(""), sdk.String(""))
	}
	loginCmd, err := local.NewCommand(ctx, fmt.Sprintf("registry-login-%s", imageName), &local.CommandArgs{
		Create: loginInputs.ApplyT(func(args []interface{}) string {
			return writeRegistryLogin(resolveStringArg(args[0]), resolveStringArg(args[1]), resolveStringArg(args[2]))
		}).(sdk.StringOutput),
	}, sdk.DependsOn(baseDeps))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create registry login for image %q", imageName)
	}
	return []sdk.Resource{loginCmd}, nil
}

// createSignVerifyCommands creates the sign and verify Pulumi commands.
func createSignVerifyCommands(ctx *sdk.Context, security *api.SecurityDescriptor, imageRef sdk.StringOutput, imageName string, signDeps []sdk.Resource) (*local.Command, *local.Command, error) {
	var signCmd *local.Command
	if security.Signing == nil || !security.Signing.Enabled {
		return nil, nil, nil
	}

	var err error
	signCmd, err = newSecurityCommand(ctx, fmt.Sprintf("sign-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"sc", "image", "sign", "--image", img}
			return append(args, signingCLIArgs(security.Signing)...)
		}, "", signingCommandEnvironment(security.Signing), signDeps)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create sign command for image %q", imageName)
	}

	if security.Signing.Verify == nil || !security.Signing.Verify.Enabled {
		return signCmd, nil, nil
	}

	// Validate verify config
	if security.Signing.Keyless {
		if security.Signing.Verify.OIDCIssuer == "" {
			return nil, nil, errors.Errorf("signing.verify.oidcIssuer is required for keyless verification of image %q", imageName)
		}
		if security.Signing.Verify.IdentityRegexp == "" {
			return nil, nil, errors.Errorf("signing.verify.identityRegexp is required for keyless verification of image %q", imageName)
		}
	} else if security.Signing.PublicKey == "" {
		return nil, nil, errors.Errorf("signing.publicKey is required for key-based verification of image %q", imageName)
	}

	verifyCmd, err := newSecurityCommand(ctx, fmt.Sprintf("verify-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"sc", "image", "verify", "--image", img}
			if security.Signing.Keyless {
				args = append(args, "--oidc-issuer", security.Signing.Verify.OIDCIssuer, "--identity-regexp", security.Signing.Verify.IdentityRegexp)
			} else {
				args = append(args, "--public-key", security.Signing.PublicKey)
			}
			return args
		}, "", verifyCommandEnvironment(security.Signing), []sdk.Resource{signCmd})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create verify command for image %q", imageName)
	}
	return signCmd, verifyCmd, nil
}

// createSBOMCommands creates sbom-gen, sbom-att, and verify-sbom Pulumi commands.
func createSBOMCommands(ctx *sdk.Context, security *api.SecurityDescriptor, imageRef sdk.StringOutput, imageName string, image Image, baseDeps []sdk.Resource, signCmd *local.Command) (sdk.Resource, error) {
	if security.SBOM == nil || !security.SBOM.Enabled {
		return nil, nil
	}

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

	sbomGenCmd, err := newSecurityCommand(ctx, fmt.Sprintf("sbom-gen-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"sc", "sbom", "generate", "--image", img, "--format", format, "--output", sbomOutputPath}
			if security.SBOM.Cache != nil && security.SBOM.Cache.Enabled && security.SBOM.Cache.Dir != "" {
				args = append(args, "--cache-dir", security.SBOM.Cache.Dir)
			}
			return args
		}, "", nil, baseDeps)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create sbom generate command for image %q", imageName)
	}

	if !attachSBOM {
		return sbomGenCmd, nil
	}
	if !signingEnabled(security) {
		if security.SBOM.Required {
			return nil, errors.Errorf("sbom attachment requires security.signing.enabled=true")
		}
		fmt.Printf("Warning: skipping SBOM attachment for %s because signing is disabled\n", imageName)
		return sbomGenCmd, nil
	}

	sbomAttDeps := []sdk.Resource{sbomGenCmd}
	if signCmd != nil {
		sbomAttDeps = append(sbomAttDeps, signCmd)
	}
	sbomAttCmd, err := newSecurityCommand(ctx, fmt.Sprintf("sbom-att-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"sc", "sbom", "attach", "--image", img, "--sbom", sbomOutputPath}
			return append(args, signingCLIArgs(security.Signing)...)
		}, "", signingCommandEnvironment(security.Signing), sbomAttDeps)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create sbom attach command for image %q", imageName)
	}

	if security.Signing.Verify == nil || !security.Signing.Verify.Enabled {
		return sbomAttCmd, nil
	}
	sbomVerifyCmd, err := newSecurityCommand(ctx, fmt.Sprintf("verify-sbom-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"cosign", "verify-attestation", "--type", "cyclonedx"}
			args = append(args, verifyIdentityArgs(security.Signing)...)
			return append(args, img)
		}, "> /dev/null", verifyCommandEnvironment(security.Signing), []sdk.Resource{sbomAttCmd})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create SBOM attestation verification for image %q", imageName)
	}
	return sbomVerifyCmd, nil
}

// createProvenanceCommands creates prov-att and verify-prov Pulumi commands.
func createProvenanceCommands(ctx *sdk.Context, security *api.SecurityDescriptor, imageRef sdk.StringOutput, imageName string, image Image, baseDeps []sdk.Resource, signCmd *local.Command) (sdk.Resource, error) {
	if security.Provenance == nil || !security.Provenance.Enabled {
		return nil, nil
	}

	provenanceCommand := "generate"
	if shouldAttachProvenance(security.Provenance) {
		if !signingEnabled(security) {
			if security.Provenance.Required {
				return nil, errors.Errorf("provenance.output.registry requires security.signing.enabled=true")
			}
			if resolveProvenanceOutputPath(security, imageName) == "" {
				fmt.Printf("Warning: skipping provenance for %s because registry attachment requires signing and no local output was configured\n", imageName)
				return nil, nil
			}
			fmt.Printf("Warning: generating provenance locally for %s because registry attachment requires signing\n", imageName)
		} else {
			provenanceCommand = "attach"
		}
	}

	format := "slsa-v1.0"
	if security.Provenance.Format != "" {
		format = security.Provenance.Format
	}
	provenanceOutputPath := resolveProvenanceOutputPath(security, imageName)

	provGenDeps := baseDeps
	var provEnv sdk.StringMap
	if provenanceCommand == "attach" {
		provEnv = signingCommandEnvironment(security.Signing)
		if signCmd != nil {
			provGenDeps = []sdk.Resource{signCmd}
		}
	}

	provCmd, err := newSecurityCommand(ctx, fmt.Sprintf("prov-att-%s", imageName), imageRef,
		func(img string) []string {
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
			return args
		}, "", provEnv, provGenDeps)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create provenance command for image %q", imageName)
	}

	if provenanceCommand != "attach" || security.Signing == nil ||
		security.Signing.Verify == nil || !security.Signing.Verify.Enabled {
		return provCmd, nil
	}
	provVerifyCmd, err := newSecurityCommand(ctx, fmt.Sprintf("verify-provenance-%s", imageName), imageRef,
		func(img string) []string {
			args := []string{"cosign", "verify-attestation", "--type", "https://slsa.dev/provenance/v1"}
			args = append(args, verifyIdentityArgs(security.Signing)...)
			return append(args, img)
		}, "> /dev/null", verifyCommandEnvironment(security.Signing), []sdk.Resource{provCmd})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create provenance attestation verification for image %q", imageName)
	}
	return provVerifyCmd, nil
}

// createScanCommands creates Pulumi local.Command resource(s) for vulnerability scanning.
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
		barrier, err := local.NewCommand(ctx, fmt.Sprintf("scan-%s-barrier", imageName), &local.CommandArgs{
			Create: sdk.String("echo 'all scans completed'"),
		}, sdk.DependsOn(toolCmds))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create scan barrier for image %q", imageName)
		}
		return barrier, nil
	}

	mergedCmdArgs := []string{"sc", "image", "scan", "--image", "%s", fmt.Sprintf("--required=%t", security.Scan.Required)}
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

// buildScanCoreArgs constructs the base sc image scan argument list.
func buildScanCoreArgs(scan *api.ScanDescriptor, toolName string, required bool, failOn, warnOn string, softFail bool) []string {
	args := []string{"sc", "image", "scan", "--image", "%s", "--tool", toolName, fmt.Sprintf("--required=%t", required)}
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
// output and reporting flags.
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
// Uses %s placeholder substitution for the image reference.
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
			return securityPATHPrefix + strings.Join(parts, " ")
		}).(sdk.StringOutput),
		Environment: env,
	}, sdk.DependsOn(deps))
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
