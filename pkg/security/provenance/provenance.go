package provenance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/security/attestation"
	"github.com/simple-container-com/api/pkg/security/signing"
)

const (
	// Cosign attestation types for supported provenance predicate schemas.
	CosignAttestationTypeV10 = "slsaprovenance1"
	CosignAttestationTypeV02 = "slsaprovenance02"
)

// Format identifies the provenance predicate schema.
type Format string

const (
	FormatSLSAV10 Format = "slsa-v1.0"
	// FormatSLSAV02 is retained only so legacy attestations can still be detected.
	FormatSLSAV02 Format = "slsa-v0.2"
)

// Statement holds a generated provenance predicate.
type Statement struct {
	Format      Format    `json:"format"`
	Content     []byte    `json:"content"`
	Digest      string    `json:"digest"`
	ImageRef    string    `json:"imageRef"`
	GeneratedAt time.Time `json:"generatedAt"`
	Metadata    *Metadata `json:"metadata,omitempty"`
}

// Metadata tracks key provenance metadata used during generation.
type Metadata struct {
	BuilderID  string `json:"builderId,omitempty"`
	SourceURI  string `json:"sourceUri,omitempty"`
	GitCommit  string `json:"gitCommit,omitempty"`
	GitBranch  string `json:"gitBranch,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
}

// GenerateOptions controls provenance generation.
type GenerateOptions struct {
	BuilderID         string
	SourceRoot        string
	ContextPath       string
	DockerfilePath    string
	IncludeGit        bool
	IncludeDockerfile bool
	IncludeEnv        bool
	IncludeMaterials  bool
}

// ValidateOptions controls post-verification provenance policy checks.
type ValidateOptions struct {
	ExpectedFormat    Format
	ExpectedDigest    string
	ExpectedBuilderID string
	ExpectedSourceURI string
	ExpectedCommit    string
}

// Attacher attaches and verifies provenance attestations via cosign.
type Attacher struct {
	SigningConfig *signing.Config
	Timeout       time.Duration
}

// ParseFormat parses the configured provenance format.
func ParseFormat(value string) (Format, error) {
	switch Format(value) {
	case "", FormatSLSAV10:
		return FormatSLSAV10, nil
	default:
		return "", fmt.Errorf("unsupported provenance format %q: only slsa-v1.0 is accepted", value)
	}
}

// NewStatement creates a normalized provenance statement.
func NewStatement(format Format, content []byte, imageRef string, metadata *Metadata) *Statement {
	sum := sha256.Sum256(content)
	return &Statement{
		Format:      format,
		Content:     append([]byte(nil), content...),
		Digest:      "sha256:" + hex.EncodeToString(sum[:]),
		ImageRef:    imageRef,
		GeneratedAt: time.Now(),
		Metadata:    metadata,
	}
}

// Save persists the generated statement to a local file.
func (s *Statement) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating provenance output directory: %w", err)
	}
	if err := os.WriteFile(path, s.Content, 0o644); err != nil {
		return fmt.Errorf("writing provenance file: %w", err)
	}
	return nil
}

// Predicate returns the provenance predicate content suitable for cosign attest.
// When Content is already a bare predicate, it is returned unchanged.
func (s *Statement) Predicate() ([]byte, error) {
	var envelope struct {
		Predicate json.RawMessage `json:"predicate"`
	}
	if err := json.Unmarshal(s.Content, &envelope); err != nil {
		return nil, fmt.Errorf("parsing provenance statement: %w", err)
	}
	if len(envelope.Predicate) == 0 {
		return append([]byte(nil), s.Content...), nil
	}
	return append([]byte(nil), envelope.Predicate...), nil
}

// Generate creates a provenance predicate for the supplied image.
func Generate(ctx context.Context, imageRef string, format Format, opts GenerateOptions) (*Statement, error) {
	if format == "" {
		format = FormatSLSAV10
	}

	metadata := &Metadata{
		BuilderID:  detectBuilderID(opts.BuilderID),
		Dockerfile: opts.DockerfilePath,
	}

	if opts.SourceRoot == "" {
		opts.SourceRoot = "."
	}

	if opts.IncludeGit {
		sourceURI, commit, branch := detectGitMetadata(ctx, opts.SourceRoot)
		metadata.SourceURI = sourceURI
		metadata.GitCommit = commit
		metadata.GitBranch = branch
	}

	content, err := buildPredicate(format, imageRef, metadata, opts)
	if err != nil {
		return nil, err
	}

	return NewStatement(format, content, imageRef, metadata), nil
}

// NewAttacher creates a provenance attacher.
func NewAttacher(signingConfig *signing.Config) *Attacher {
	return &Attacher{
		SigningConfig: signingConfig,
		Timeout:       2 * time.Minute,
	}
}

// Attach attaches the provenance statement to the image using cosign attest.
func (a *Attacher) Attach(ctx context.Context, statement *Statement, imageRef string) error {
	predicate, err := statement.Predicate()
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp("", "provenance-*.json")
	if err != nil {
		return fmt.Errorf("creating temp provenance file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(predicate); err != nil {
		return fmt.Errorf("writing temp provenance file: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	args := []string{
		"attest",
		"--predicate", tmpFile.Name(),
		"--type", attestationType(statement.Format),
	}
	args = append(args, a.buildSigningArgs()...)
	args = append(args, imageRef)

	cmd := exec.CommandContext(timeoutCtx, "cosign", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), a.buildSigningEnv()...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign attest failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// Verify verifies the provenance attestation and returns the decoded predicate.
func (a *Attacher) Verify(ctx context.Context, imageRef string, format Format) (*Statement, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	args := []string{
		"verify-attestation",
		"--type", attestationType(format),
	}
	args = append(args, a.buildVerificationArgs()...)
	args = append(args, imageRef)

	cmd := exec.CommandContext(timeoutCtx, "cosign", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), a.buildSigningEnv()...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cosign verify-attestation failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	payload, err := attestation.DecodeFirstPayload(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parsing verify-attestation output: %w", err)
	}

	detectedFormat, detectErr := DetectFormat(payload)
	if detectErr == nil {
		format = detectedFormat
	}

	return NewStatement(format, payload, imageRef, nil), nil
}

// Validate verifies that the statement content matches expected provenance policy values.
func (s *Statement) Validate(opts ValidateOptions) error {
	return ValidateStatementContent(s.Content, opts)
}

func (a *Attacher) buildSigningArgs() []string {
	if a.SigningConfig == nil {
		return nil
	}

	if a.SigningConfig.Keyless {
		return []string{"--yes"}
	}
	if a.SigningConfig.PrivateKey != "" {
		return []string{"--key", a.SigningConfig.PrivateKey}
	}

	return nil
}

// ExtractDigestFromImageRef returns the sha256 digest component from an image ref.
func ExtractDigestFromImageRef(imageRef string) string {
	if !strings.Contains(imageRef, "@sha256:") {
		return ""
	}
	return strings.TrimPrefix(strings.SplitN(imageRef, "@", 2)[1], "sha256:")
}

// DetectFormat inspects a provenance payload and returns the matching supported format.
func DetectFormat(content []byte) (Format, error) {
	var envelope struct {
		Type          string `json:"_type"`
		PredicateType string `json:"predicateType"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return "", fmt.Errorf("parsing provenance envelope: %w", err)
	}

	switch {
	case envelope.PredicateType == "https://slsa.dev/provenance/v1":
		return FormatSLSAV10, nil
	case envelope.PredicateType == "https://slsa.dev/provenance/v0.2":
		return FormatSLSAV02, nil
	default:
		return "", fmt.Errorf("unsupported provenance envelope _type=%q predicateType=%q", envelope.Type, envelope.PredicateType)
	}
}

// ValidateStatementContent applies policy checks to a verified provenance payload.
func ValidateStatementContent(content []byte, opts ValidateOptions) error {
	var statement struct {
		Type    string `json:"_type"`
		Subject []struct {
			Name   string            `json:"name"`
			Digest map[string]string `json:"digest"`
		} `json:"subject"`
		PredicateType string          `json:"predicateType"`
		Predicate     json.RawMessage `json:"predicate"`
	}
	if err := json.Unmarshal(content, &statement); err != nil {
		return fmt.Errorf("parsing provenance statement: %w", err)
	}

	if opts.ExpectedFormat != "" {
		if !matchesExpectedEnvelope(opts.ExpectedFormat, statement.Type, statement.PredicateType) {
			expectedType, expectedPredicateType := expectedEnvelope(opts.ExpectedFormat)
			return fmt.Errorf(
				"provenance format mismatch: expected %s (%s, %s), got (%s, %s)",
				opts.ExpectedFormat,
				expectedType,
				expectedPredicateType,
				statement.Type,
				statement.PredicateType,
			)
		}
	}

	expectedDigest := normalizeDigest(opts.ExpectedDigest)
	if expectedDigest != "" {
		actualDigest := subjectDigest(statement.Subject)
		if actualDigest == "" {
			return fmt.Errorf("provenance subject digest is missing")
		}
		if actualDigest != expectedDigest {
			return fmt.Errorf("provenance subject digest mismatch: expected %s, got %s", expectedDigest, actualDigest)
		}
	}

	var predicate map[string]interface{}
	if len(statement.Predicate) > 0 {
		if err := json.Unmarshal(statement.Predicate, &predicate); err != nil {
			return fmt.Errorf("parsing provenance predicate: %w", err)
		}
	}

	if opts.ExpectedBuilderID != "" {
		builderID := predicateBuilderID(predicate)
		if builderID == "" {
			return fmt.Errorf("provenance builder ID is missing")
		}
		if builderID != opts.ExpectedBuilderID {
			return fmt.Errorf("provenance builder ID mismatch: expected %s, got %s", opts.ExpectedBuilderID, builderID)
		}
	}

	if opts.ExpectedSourceURI != "" || opts.ExpectedCommit != "" {
		if !hasExpectedDependency(predicateDependencies(predicate), opts.ExpectedSourceURI, opts.ExpectedCommit) {
			return fmt.Errorf("provenance materials do not contain expected source dependency")
		}
	}

	return nil
}

func expectedEnvelope(format Format) (string, string) {
	switch format {
	case FormatSLSAV10:
		return "https://in-toto.io/Statement/v1", "https://slsa.dev/provenance/v1"
	case FormatSLSAV02:
		return "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v0.2"
	default:
		return "", ""
	}
}

func matchesExpectedEnvelope(format Format, actualType, actualPredicateType string) bool {
	switch format {
	case FormatSLSAV10:
		return actualPredicateType == "https://slsa.dev/provenance/v1" &&
			(actualType == "https://in-toto.io/Statement/v1" || actualType == "https://in-toto.io/Statement/v0.1")
	case FormatSLSAV02:
		return actualPredicateType == "https://slsa.dev/provenance/v0.2" &&
			actualType == "https://in-toto.io/Statement/v0.1"
	default:
		return false
	}
}

func attestationType(format Format) string {
	switch format {
	case FormatSLSAV02:
		return CosignAttestationTypeV02
	case FormatSLSAV10:
		fallthrough
	default:
		return CosignAttestationTypeV10
	}
}

func (a *Attacher) buildVerificationArgs() []string {
	if a.SigningConfig == nil {
		return nil
	}

	if a.SigningConfig.Keyless {
		var args []string
		if a.SigningConfig.IdentityRegexp != "" {
			args = append(args, "--certificate-identity-regexp", a.SigningConfig.IdentityRegexp)
		}
		if a.SigningConfig.OIDCIssuer != "" {
			args = append(args, "--certificate-oidc-issuer", a.SigningConfig.OIDCIssuer)
		}
		return args
	}
	if a.SigningConfig.PublicKey != "" {
		return []string{"--key", a.SigningConfig.PublicKey}
	}

	return nil
}

func (a *Attacher) buildSigningEnv() []string {
	if a.SigningConfig == nil {
		return nil
	}

	if !a.SigningConfig.Keyless && a.SigningConfig.PrivateKey != "" {
		return []string{fmt.Sprintf("COSIGN_PASSWORD=%s", a.SigningConfig.Password)}
	}

	return nil
}

func buildPredicate(format Format, imageRef string, metadata *Metadata, opts GenerateOptions) ([]byte, error) {
	switch format {
	case FormatSLSAV10:
		return json.MarshalIndent(map[string]interface{}{
			"_type":         "https://in-toto.io/Statement/v1",
			"subject":       []map[string]interface{}{subjectDescriptor(imageRef)},
			"predicateType": "https://slsa.dev/provenance/v1",
			"predicate": map[string]interface{}{
				"buildDefinition": map[string]interface{}{
					"buildType":            "https://simple-container.com/container-image@v1",
					"externalParameters":   externalParameters(imageRef, opts),
					"internalParameters":   internalParameters(opts),
					"resolvedDependencies": resolvedDependencies(metadata, opts),
				},
				"runDetails": map[string]interface{}{
					"builder": map[string]interface{}{
						"id": metadata.BuilderID,
					},
					"metadata": map[string]interface{}{
						"invocationId": detectInvocationID(),
						"startedOn":    time.Now().UTC().Format(time.RFC3339),
						"finishedOn":   time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		}, "", "  ")
	case FormatSLSAV02:
		return json.MarshalIndent(map[string]interface{}{
			"_type":         "https://in-toto.io/Statement/v0.1",
			"subject":       []map[string]interface{}{subjectDescriptor(imageRef)},
			"predicateType": "https://slsa.dev/provenance/v0.2",
			"predicate": map[string]interface{}{
				"builder": map[string]interface{}{
					"id": metadata.BuilderID,
				},
				"buildType":  "https://simple-container.com/container-image@v1",
				"invocation": map[string]interface{}{"parameters": externalParameters(imageRef, opts)},
				"metadata": map[string]interface{}{
					"buildInvocationID": detectInvocationID(),
					"buildStartedOn":    time.Now().UTC().Format(time.RFC3339),
					"buildFinishedOn":   time.Now().UTC().Format(time.RFC3339),
				},
				"materials": resolvedDependencies(metadata, opts),
			},
		}, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported provenance format %q", format)
	}
}

func subjectDescriptor(imageRef string) map[string]interface{} {
	subject := map[string]interface{}{
		"name": imageRef,
	}
	if digest := ExtractDigestFromImageRef(imageRef); digest != "" {
		subject["digest"] = map[string]string{
			"sha256": digest,
		}
	}
	return subject
}

func externalParameters(imageRef string, opts GenerateOptions) map[string]interface{} {
	params := map[string]interface{}{
		"image": imageRef,
	}
	if opts.ContextPath != "" {
		params["contextPath"] = opts.ContextPath
	}
	if opts.DockerfilePath != "" {
		params["dockerfilePath"] = opts.DockerfilePath
	}
	return params
}

func internalParameters(opts GenerateOptions) map[string]interface{} {
	if !opts.IncludeEnv {
		return map[string]interface{}{}
	}

	env := map[string]string{}
	for _, key := range []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITHUB_REPOSITORY",
		"GITHUB_RUN_ID",
		"GITHUB_RUN_ATTEMPT",
		"GITHUB_SHA",
		"GITHUB_REF_NAME",
	} {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}

	return map[string]interface{}{
		"environment": env,
	}
}

func resolvedDependencies(metadata *Metadata, opts GenerateOptions) []map[string]interface{} {
	if metadata == nil || !opts.IncludeMaterials {
		return nil
	}

	var dependencies []map[string]interface{}
	if metadata.SourceURI != "" && metadata.GitCommit != "" {
		dependencies = append(dependencies, map[string]interface{}{
			"uri": metadata.SourceURI,
			"digest": map[string]string{
				"sha1": metadata.GitCommit,
			},
		})
	}
	if opts.IncludeDockerfile && opts.DockerfilePath != "" {
		if sum, err := fileSHA256(opts.DockerfilePath); err == nil {
			dependencies = append(dependencies, map[string]interface{}{
				"uri": filepath.Clean(opts.DockerfilePath),
				"digest": map[string]string{
					"sha256": sum,
				},
			})
		}
	}

	return dependencies
}

func fileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func detectBuilderID(configured string) string {
	if configured != "" {
		return configured
	}
	if server := os.Getenv("GITHUB_SERVER_URL"); server != "" {
		repo := os.Getenv("GITHUB_REPOSITORY")
		runID := os.Getenv("GITHUB_RUN_ID")
		if repo != "" && runID != "" {
			return fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID)
		}
	}
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return "local://" + hostname
	}
	return "local://simple-container"
}

func detectInvocationID() string {
	for _, key := range []string{"GITHUB_RUN_ID", "CI_PIPELINE_ID", "BUILD_BUILDID"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fmt.Sprintf("local-%d", time.Now().Unix())
}

func detectGitMetadata(ctx context.Context, root string) (string, string, string) {
	remote := gitOutput(ctx, root, "config", "--get", "remote.origin.url")
	commit := gitOutput(ctx, root, "rev-parse", "HEAD")
	branch := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	return remote, commit, branch
}

func gitOutput(ctx context.Context, root string, args ...string) string {
	allArgs := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", allArgs...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func normalizeDigest(value string) string {
	return strings.TrimPrefix(value, "sha256:")
}

func subjectDigest(subjects []struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}) string {
	for _, subject := range subjects {
		if digest := normalizeDigest(subject.Digest["sha256"]); digest != "" {
			return digest
		}
	}
	return ""
}

func predicateBuilderID(predicate map[string]interface{}) string {
	if value := nestedString(predicate, "runDetails", "builder", "id"); value != "" {
		return value
	}
	return nestedString(predicate, "builder", "id")
}

func predicateDependencies(predicate map[string]interface{}) []map[string]interface{} {
	values := nestedSlice(predicate, "buildDefinition", "resolvedDependencies")
	if len(values) == 0 {
		values = nestedSlice(predicate, "materials")
	}
	dependencies := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		dependency, ok := value.(map[string]interface{})
		if ok {
			dependencies = append(dependencies, dependency)
		}
	}
	return dependencies
}

func hasExpectedDependency(dependencies []map[string]interface{}, expectedSourceURI, expectedCommit string) bool {
	for _, dependency := range dependencies {
		uri, _ := dependency["uri"].(string)
		if expectedSourceURI != "" && uri != expectedSourceURI {
			continue
		}
		if expectedCommit == "" {
			return true
		}
		digestMap, _ := dependency["digest"].(map[string]interface{})
		for _, key := range []string{"sha1", "sha256"} {
			if normalizeDigest(asString(digestMap[key])) == normalizeDigest(expectedCommit) {
				return true
			}
		}
	}
	return false
}

func nestedString(root map[string]interface{}, path ...string) string {
	current := interface{}(root)
	for _, segment := range path {
		node, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = node[segment]
	}
	return asString(current)
}

func nestedSlice(root map[string]interface{}, path ...string) []interface{} {
	current := interface{}(root)
	for _, segment := range path {
		node, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = node[segment]
	}
	values, _ := current.([]interface{})
	return values
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
