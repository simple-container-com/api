package docker

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

// --- Shell utilities ---

// shellQuote wraps value in POSIX single quotes, escaping embedded quotes.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// shellEscape escapes a string for use in a double-quoted shell string.
func shellEscape(s string) string {
	return strings.NewReplacer("`", "\\`", "$", "\\$", "\"", "\\\"", "\\", "\\\\").Replace(s)
}

// securityPATHPrefix ensures $HOME/.local/bin and /usr/local/bin are on PATH
// before running security tools. Go-side os.Setenv does not propagate to
// Pulumi local.Command subshells.
const securityPATHPrefix = `export PATH="$HOME/.local/bin:/usr/local/bin:$PATH" && `

// buildShellCommand constructs a shell command string from args with PATH prefix
// and shell quoting. Used by all security local.Command resources.
func buildShellCommand(args []string, suffix string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	cmd := securityPATHPrefix + strings.Join(quoted, " ")
	if suffix != "" {
		cmd += " " + suffix
	}
	return cmd
}

// newSecurityCommand creates a Pulumi local.Command that runs a shell command
// derived from the image digest reference. Reduces boilerplate across sign,
// verify, sbom, provenance commands that all follow the same pattern.
func newSecurityCommand(
	ctx *sdk.Context,
	name string,
	imageRef sdk.StringOutput,
	buildArgs func(img string) []string,
	suffix string,
	env sdk.StringMap,
	deps []sdk.Resource,
) (*local.Command, error) {
	return local.NewCommand(ctx, name, &local.CommandArgs{
		Create: imageRef.ApplyT(func(img string) string {
			return buildShellCommand(buildArgs(img), suffix)
		}).(sdk.StringOutput),
		Environment: env,
	}, sdk.DependsOn(deps))
}

// resolveStringArg extracts a string from an interface{} value that may be
// either a string or a *string. Needed because sdk.All may pass through
// *string values (from sdk.StringPtr in RegistryArgs) without dereferencing.
func resolveStringArg(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if p, ok := v.(*string); ok && p != nil {
		return *p
	}
	return ""
}

// --- Registry auth ---

// writeDockerConfigScript returns a shell script that writes registry
// credentials to ~/.docker/config.json (and /root/.docker as fallback).
func writeDockerConfigScript(server, auth string) string {
	return fmt.Sprintf(
		`CREDS='{"auths":{"%[1]s":{"auth":"%[2]s"}}}' && `+
			`mkdir -p "$HOME/.docker" && `+
			`if [ -f "$HOME/.docker/config.json" ] && command -v jq >/dev/null 2>&1; then `+
			`jq '.auths["%[1]s"]={"auth":"%[2]s"}' "$HOME/.docker/config.json" > "$HOME/.docker/config.json.tmp" && `+
			`mv "$HOME/.docker/config.json.tmp" "$HOME/.docker/config.json"; `+
			`else `+
			`printf '%%s' "$CREDS" > "$HOME/.docker/config.json"; `+
			`fi && `+
			`if [ "$HOME" != "/root" ]; then `+
			`mkdir -p /root/.docker 2>/dev/null && printf '%%s' "$CREDS" > /root/.docker/config.json 2>/dev/null || true; `+
			`fi`,
		server, auth)
}

// writeRegistryLogin builds the registry-login shell command from resolved credentials.
func writeRegistryLogin(server, username, password string) string {
	if password == "" {
		return "echo 'WARNING: Registry password is empty — security tools may fail to authenticate. Check RegistryArgs credential flow.'"
	}
	if username == "" {
		username = "_token"
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return writeDockerConfigScript(server, auth)
}

// --- Signing/verify CLI helpers ---

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

// verifyIdentityArgs returns cosign verify/verify-attestation args for identity
// checking. For keyless: --certificate-oidc-issuer + --certificate-identity-regexp.
// For key-based: --key.
func verifyIdentityArgs(signing *api.SigningDescriptor) []string {
	if signing.Keyless && signing.Verify != nil {
		return []string{
			"--certificate-oidc-issuer", signing.Verify.OIDCIssuer,
			"--certificate-identity-regexp", signing.Verify.IdentityRegexp,
		}
	}
	if signing.PublicKey != "" {
		return []string{"--key", signing.PublicKey}
	}
	return nil
}

// --- Environment builders ---

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
// Unlike signingCommandEnvironment, it never includes COSIGN_PASSWORD.
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

// --- Path resolvers ---

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

// --- Scan helpers ---

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

func scanDescriptorEnabled(tool api.ScanToolDescriptor) bool {
	if tool.Enabled != nil {
		return *tool.Enabled
	}
	return tool.Required || tool.FailOn != "" || tool.WarnOn != "" || tool.Name != ""
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

// --- Config validators ---

func signingEnabled(security *api.SecurityDescriptor) bool {
	return security != nil && security.Signing != nil && security.Signing.Enabled
}

func provenanceRegistryOutputEnabled(config *api.ProvenanceDescriptor) bool {
	return config != nil && config.Output != nil && config.Output.Registry
}

func shouldAttachProvenance(config *api.ProvenanceDescriptor) bool {
	return provenanceRegistryOutputEnabled(config)
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
		return fmt.Errorf("sbom.attach.sign must be true when sbom.attach.enabled=true")
	}
	if config.Output != nil && config.Output.Registry && config.Attach != nil {
		if !config.Attach.Enabled {
			return fmt.Errorf("sbom.attach.enabled=false is not compatible with sbom.output.registry=true")
		}
		if !config.Attach.Sign {
			return fmt.Errorf("sbom.attach.sign=false is not compatible with sbom.output.registry=true")
		}
	}
	return nil
}
