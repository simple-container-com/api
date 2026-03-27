package docker

import (
	"fmt"
	"path/filepath"
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
		securityOpts, err := executeSecurityOperations(ctx, stack, res, image)
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
func executeSecurityOperations(ctx *sdk.Context, stack api.Stack, dockerImage *docker.Image, image Image) ([]sdk.ResourceOption, error) {
	security := stack.Client.Security
	var lastResource sdk.Resource = dockerImage
	var opts []sdk.ResourceOption
	imageName := image.Name
	imageUrl := image.RepositoryUrl.ApplyT(func(repoURI string) string {
		if image.RepositoryUrlWithImage {
			return fmt.Sprintf("%s:%s", repoURI, image.Version)
		}
		return fmt.Sprintf("%s/%s:%s", repoURI, image.Name, image.Version)
	}).(sdk.StringOutput)
	securityImageRef := resolveSecurityImageRef(ctx, dockerImage.RepoDigest, imageUrl)

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
		enabledTools := enabledScanDescriptors(security.Scan.Tools)
		if len(enabledTools) == 0 {
			if security.Scan.Required {
				return nil, errors.Errorf("security.scan.enabled requires at least one enabled scan tool")
			}
			fmt.Printf("Warning: skipping image scanning for %s because no enabled scan tools were configured\n", imageName)
		} else {
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

				scanCmdArgs := []string{
					"sc", "image", "scan",
					"--image", "%s",
					"--tool", scanToolName,
					fmt.Sprintf("--required=%t", required),
				}
				appendScanCacheArg(&scanCmdArgs, security.Scan)
				if failOn != "" {
					scanCmdArgs = append(scanCmdArgs, "--fail-on", failOn)
				}
				if warnOn != "" {
					scanCmdArgs = append(scanCmdArgs, "--warn-on", warnOn)
				}
				if len(enabledTools) == 1 {
					appendScanOutputArg(&scanCmdArgs, resolveToolScanOutputPath(security, imageName, scanToolName, false))
				} else {
					appendScanOutputArg(&scanCmdArgs, resolveScanOutputPath(security, imageName))
				}
				if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
					scanCmdArgs = appendDefectDojoFlags(scanCmdArgs, security.Reporting.DefectDojo)
				}
				if commentOutput := resolveCommentOutputPath(security, imageName); commentOutput != "" {
					scanCmdArgs = append(scanCmdArgs, "--comment-output", commentOutput)
				}

				scanName := fmt.Sprintf("scan-%s-%s", imageName, scanToolName)
				if len(enabledTools) > 1 {
					scanName = fmt.Sprintf("scan-%s-merged", imageName)
				}
				scanCmd, err := local.NewCommand(ctx, scanName, &local.CommandArgs{
					Create: securityImageRef.ApplyT(func(img string) string {
						args := make([]string, 0, len(scanCmdArgs))
						for _, arg := range scanCmdArgs {
							if arg == "%s" {
								args = append(args, shellQuote(img))
								continue
							}
							args = append(args, shellQuote(arg))
						}
						return strings.Join(args, " ")
					}).(sdk.StringOutput),
					Environment: scanCommandEnvironment(security.Reporting),
				}, sdk.DependsOn([]sdk.Resource{lastResource}))
				if err != nil {
					return nil, errors.Wrapf(err, "failed to create %s scan command", scanToolName)
				}
				lastResource = scanCmd
			} else {
				toolResultPaths := make([]string, 0, len(enabledTools))
				needsArtifacts := needsMergedScanArtifacts(security, imageName)
				for _, tool := range enabledTools {
					scanCmdArgs := []string{
						"sc", "image", "scan",
						"--image", "%s",
						"--tool", tool.Name,
						fmt.Sprintf("--required=%t", security.Scan.Required || tool.Required),
					}
					appendScanCacheArg(&scanCmdArgs, security.Scan)
					if failOn := effectiveScanThreshold(security.Scan.FailOn, tool.FailOn); failOn != "" {
						scanCmdArgs = append(scanCmdArgs, "--fail-on", failOn)
					}
					if warnOn := effectiveScanThreshold(security.Scan.WarnOn, tool.WarnOn); warnOn != "" {
						scanCmdArgs = append(scanCmdArgs, "--warn-on", warnOn)
					}
					outputPath := resolveToolScanOutputPath(security, imageName, tool.Name, true)
					if outputPath == "" && needsArtifacts {
						outputPath = tempArtifactPath(fmt.Sprintf("scan-results-%s", tool.Name), imageName, "json")
					}
					appendScanOutputArg(&scanCmdArgs, outputPath)
					if outputPath != "" {
						toolResultPaths = append(toolResultPaths, outputPath)
					}

					scanCmd, err := local.NewCommand(ctx, fmt.Sprintf("scan-%s-%s", imageName, tool.Name), &local.CommandArgs{
						Create: securityImageRef.ApplyT(func(img string) string {
							args := make([]string, 0, len(scanCmdArgs))
							for _, arg := range scanCmdArgs {
								if arg == "%s" {
									args = append(args, shellQuote(img))
									continue
								}
								args = append(args, shellQuote(arg))
							}
							return strings.Join(args, " ")
						}).(sdk.StringOutput),
						Environment: scanCommandEnvironment(security.Reporting),
					}, sdk.DependsOn([]sdk.Resource{lastResource}))
					if err != nil {
						return nil, errors.Wrapf(err, "failed to create %s scan command", tool.Name)
					}
					lastResource = scanCmd
				}

				if needsArtifacts {
					scanCmdArgs := []string{
						"sc", "image", "scan",
						"--image", "%s",
						fmt.Sprintf("--required=%t", security.Scan.Required),
					}
					for _, inputPath := range toolResultPaths {
						scanCmdArgs = append(scanCmdArgs, "--input", inputPath)
					}
					if security.Scan.FailOn != "" {
						scanCmdArgs = append(scanCmdArgs, "--fail-on", security.Scan.FailOn)
					}
					if security.Scan.WarnOn != "" {
						scanCmdArgs = append(scanCmdArgs, "--warn-on", security.Scan.WarnOn)
					}
					appendScanOutputArg(&scanCmdArgs, resolveScanOutputPath(security, imageName))
					if security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled {
						scanCmdArgs = appendDefectDojoFlags(scanCmdArgs, security.Reporting.DefectDojo)
					}
					if commentOutput := resolveCommentOutputPath(security, imageName); commentOutput != "" {
						scanCmdArgs = append(scanCmdArgs, "--comment-output", commentOutput)
					}

					mergedScanCmd, err := local.NewCommand(ctx, fmt.Sprintf("scan-%s-merged", imageName), &local.CommandArgs{
						Create: securityImageRef.ApplyT(func(img string) string {
							args := make([]string, 0, len(scanCmdArgs))
							for _, arg := range scanCmdArgs {
								if arg == "%s" {
									args = append(args, shellQuote(img))
									continue
								}
								args = append(args, shellQuote(arg))
							}
							return strings.Join(args, " ")
						}).(sdk.StringOutput),
						Environment: scanCommandEnvironment(security.Reporting),
					}, sdk.DependsOn([]sdk.Resource{lastResource}))
					if err != nil {
						return nil, errors.Wrapf(err, "failed to create merged scan command")
					}
					lastResource = mergedScanCmd
				}
			}
		}
	}

	// Step 2: Image Signing (if enabled)
	if security.Signing != nil && security.Signing.Enabled {
		signCmd, err := local.NewCommand(ctx, fmt.Sprintf("sign-%s", imageName), &local.CommandArgs{
			Create: securityImageRef.ApplyT(func(img string) string {
				args := []string{"sc", "image", "sign", "--image", img}
				args = append(args, signingCLIArgs(security.Signing)...)
				for i, arg := range args {
					args[i] = shellQuote(arg)
				}
				return envPrefix + strings.Join(args, " ")
			}).(sdk.StringOutput),
			Environment: signingCommandEnvironment(security.Signing),
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sign command")
		}
		lastResource = signCmd
	}

	// Step 3a: SBOM Generation and Attachment (if enabled) - runs in parallel with provenance
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

		// Generate SBOM
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
		}, sdk.DependsOn([]sdk.Resource{lastResource}))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sbom generate command")
		}

		// Attach SBOM as attestation (if enabled)
		if attachSBOM {
			if !signingEnabled(security) {
				if security.SBOM.Required {
					return nil, errors.Errorf("sbom attachment requires security.signing.enabled=true")
				}
				fmt.Printf("Warning: skipping SBOM attachment for %s because signing is disabled\n", imageName)
				sbomResource = sbomGenCmd
			} else {
				sbomAttCmd, err := local.NewCommand(ctx, fmt.Sprintf("sbom-att-%s", imageName), &local.CommandArgs{
					Create: securityImageRef.ApplyT(func(img string) string {
						args := []string{"sc", "sbom", "attach", "--image", img, "--sbom", sbomOutputPath}
						args = append(args, signingCLIArgs(security.Signing)...)
						for i, arg := range args {
							args[i] = shellQuote(arg)
						}
						return envPrefix + strings.Join(args, " ")
					}).(sdk.StringOutput),
					Environment: signingCommandEnvironment(security.Signing),
				}, sdk.DependsOn([]sdk.Resource{sbomGenCmd}))
				if err != nil {
					return nil, errors.Wrapf(err, "failed to create sbom attach command")
				}
				sbomResource = sbomAttCmd
			}
		} else {
			sbomResource = sbomGenCmd
		}
	}

	// Step 3b: Provenance Attestation (if enabled) - runs in parallel with SBOM
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

			provAttCmd, err := local.NewCommand(ctx, fmt.Sprintf("prov-att-%s", imageName), &local.CommandArgs{
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
					return envPrefix + strings.Join(args, " ")
				}).(sdk.StringOutput),
				Environment: signingCommandEnvironment(security.Signing),
			}, sdk.DependsOn([]sdk.Resource{lastResource}))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create provenance attach command")
			}
			provenanceResource = provAttCmd
		}
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

func needsMergedScanArtifacts(security *api.SecurityDescriptor, imageName string) bool {
	if resolveScanOutputPath(security, imageName) != "" {
		return true
	}
	if resolveCommentOutputPath(security, imageName) != "" {
		return true
	}
	return security.Reporting != nil && security.Reporting.DefectDojo != nil && security.Reporting.DefectDojo.Enabled
}

func resolveSecurityImageRef(ctx *sdk.Context, repoDigest, imageURL sdk.StringOutput) sdk.StringOutput {
	return sdk.All(repoDigest, imageURL).ApplyT(func(values []interface{}) (string, error) {
		repoDigestValue, _ := values[0].(string)
		imageURLValue, _ := values[1].(string)
		if repoDigestValue != "" {
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

func shellQuote(value string) string {
	return fmt.Sprintf("%q", value)
}
