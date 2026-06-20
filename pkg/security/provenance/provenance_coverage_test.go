package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestParseFormat(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{name: "empty defaults to slsa-v1.0", input: "", want: FormatSLSAV10},
		{name: "explicit slsa-v1.0", input: "slsa-v1.0", want: FormatSLSAV10},
		{name: "legacy slsa-v0.2 rejected", input: "slsa-v0.2", wantErr: true},
		{name: "garbage rejected", input: "nope", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := ParseFormat(tc.input)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				Expect(got).To(Equal(Format("")))
				Expect(err.Error()).To(ContainSubstring("only slsa-v1.0 is accepted"))
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestNewStatement(t *testing.T) {
	RegisterTestingT(t)

	content := []byte(`{"predicate":{"a":1}}`)
	meta := &Metadata{BuilderID: "b", SourceURI: "u"}
	stmt := NewStatement(FormatSLSAV10, content, "img@sha256:dead", meta)

	Expect(stmt.Format).To(Equal(FormatSLSAV10))
	Expect(stmt.ImageRef).To(Equal("img@sha256:dead"))
	Expect(stmt.Metadata).To(Equal(meta))

	// Digest is the sha256 of the content with a sha256: prefix.
	sum := sha256.Sum256(content)
	Expect(stmt.Digest).To(Equal("sha256:" + hex.EncodeToString(sum[:])))

	// Content is a defensive copy: mutating the source must not change the statement.
	Expect(stmt.Content).To(Equal(content))
	content[0] = 'X'
	Expect(stmt.Content).ToNot(Equal(content))

	// GeneratedAt is populated.
	Expect(stmt.GeneratedAt.IsZero()).To(BeFalse())
}

func TestStatementSave(t *testing.T) {
	RegisterTestingT(t)

	t.Run("writes nested path and content", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "deeper", "provenance.json")
		stmt := &Statement{Content: []byte(`{"k":"v"}`)}

		Expect(stmt.Save(path)).To(Succeed())

		got, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(got)).To(Equal(`{"k":"v"}`))
	})

	t.Run("fails when output dir cannot be created", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		// Create a regular file, then try to write under it as if it were a dir.
		blocker := filepath.Join(dir, "afile")
		Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())
		path := filepath.Join(blocker, "child", "provenance.json")
		stmt := &Statement{Content: []byte(`{}`)}

		err := stmt.Save(path)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("creating provenance output directory"))
	})
}

func TestStatementPredicateInvalidJSON(t *testing.T) {
	RegisterTestingT(t)

	stmt := &Statement{Content: []byte(`{not json`)}
	_, err := stmt.Predicate()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("parsing provenance statement"))
}

func TestNewAttacher(t *testing.T) {
	RegisterTestingT(t)

	cfg := &signing.Config{Keyless: true}
	a := NewAttacher(cfg)
	Expect(a.SigningConfig).To(Equal(cfg))
	Expect(a.Timeout.String()).To(Equal("2m0s"))
}

func TestBuildSigningArgs(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name string
		cfg  *signing.Config
		want []string
	}{
		{name: "nil config -> nil", cfg: nil, want: nil},
		{name: "keyless -> --yes", cfg: &signing.Config{Keyless: true}, want: []string{"--yes"}},
		{name: "key-based -> --key", cfg: &signing.Config{PrivateKey: "/k/cosign.key"}, want: []string{"--key", "/k/cosign.key"}},
		{name: "no keyless no key -> nil", cfg: &signing.Config{}, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			a := &Attacher{SigningConfig: tc.cfg}
			got := a.buildSigningArgs()
			if tc.want == nil {
				Expect(got).To(BeNil())
				return
			}
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestBuildVerificationArgs(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name string
		cfg  *signing.Config
		want []string
	}{
		{name: "nil config -> nil", cfg: nil, want: nil},
		{
			name: "keyless with identity + issuer",
			cfg:  &signing.Config{Keyless: true, IdentityRegexp: "ident.*", OIDCIssuer: "https://issuer"},
			want: []string{"--certificate-identity-regexp", "ident.*", "--certificate-oidc-issuer", "https://issuer"},
		},
		{
			name: "keyless with only identity",
			cfg:  &signing.Config{Keyless: true, IdentityRegexp: "ident.*"},
			want: []string{"--certificate-identity-regexp", "ident.*"},
		},
		{name: "keyless with neither -> nil", cfg: &signing.Config{Keyless: true}, want: nil},
		{name: "key-based -> --key", cfg: &signing.Config{PublicKey: "/k/cosign.pub"}, want: []string{"--key", "/k/cosign.pub"}},
		{name: "no keyless no pubkey -> nil", cfg: &signing.Config{}, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			a := &Attacher{SigningConfig: tc.cfg}
			got := a.buildVerificationArgs()
			if tc.want == nil {
				Expect(got).To(BeNil())
				return
			}
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestBuildSigningEnvKeyless(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil config -> nil", func(t *testing.T) {
		RegisterTestingT(t)
		a := &Attacher{}
		Expect(a.buildSigningEnv()).To(BeNil())
	})

	t.Run("keyless with OIDC token -> SIGSTORE_ID_TOKEN", func(t *testing.T) {
		RegisterTestingT(t)
		a := &Attacher{SigningConfig: &signing.Config{Keyless: true, OIDCToken: "tok123"}}
		got := a.buildSigningEnv()
		Expect(got).To(ConsistOf("SIGSTORE_ID_TOKEN=tok123"))
	})

	t.Run("keyless without OIDC token -> nil", func(t *testing.T) {
		RegisterTestingT(t)
		a := &Attacher{SigningConfig: &signing.Config{Keyless: true}}
		Expect(a.buildSigningEnv()).To(BeNil())
	})

	t.Run("key-based with password -> COSIGN_PASSWORD", func(t *testing.T) {
		RegisterTestingT(t)
		a := &Attacher{SigningConfig: &signing.Config{PrivateKey: "/k", Password: "pw"}}
		got := a.buildSigningEnv()
		Expect(got).To(ConsistOf("COSIGN_PASSWORD=pw"))
	})
}

func TestExtractDigestFromImageRef(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "no digest", input: "docker.io/library/alpine:3.20", want: ""},
		{name: "with digest", input: "docker.io/library/alpine@sha256:abc123", want: "abc123"},
		{name: "tag and digest", input: "repo/img:tag@sha256:deadbeef", want: "deadbeef"},
		{name: "non-sha256 digest ignored", input: "repo/img@sha512:zzz", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(ExtractDigestFromImageRef(tc.input)).To(Equal(tc.want))
		})
	}
}

func TestDetectFormat(t *testing.T) {
	RegisterTestingT(t)

	cases := []struct {
		name    string
		content string
		want    Format
		wantErr bool
	}{
		{
			name:    "slsa v1.0",
			content: `{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://slsa.dev/provenance/v1"}`,
			want:    FormatSLSAV10,
		},
		{
			name:    "slsa v0.2",
			content: `{"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://slsa.dev/provenance/v0.2"}`,
			want:    FormatSLSAV02,
		},
		{
			name:    "unknown predicateType",
			content: `{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://other/x"}`,
			wantErr: true,
		},
		{name: "invalid json", content: `{bad`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := DetectFormat([]byte(tc.content))
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestExpectedEnvelope(t *testing.T) {
	RegisterTestingT(t)

	st, pt := expectedEnvelope(FormatSLSAV10)
	Expect(st).To(Equal("https://in-toto.io/Statement/v1"))
	Expect(pt).To(Equal("https://slsa.dev/provenance/v1"))

	st, pt = expectedEnvelope(FormatSLSAV02)
	Expect(st).To(Equal("https://in-toto.io/Statement/v0.1"))
	Expect(pt).To(Equal("https://slsa.dev/provenance/v0.2"))

	st, pt = expectedEnvelope(Format("bogus"))
	Expect(st).To(Equal(""))
	Expect(pt).To(Equal(""))
}

func TestMatchesExpectedEnvelopeV02AndDefault(t *testing.T) {
	RegisterTestingT(t)

	Expect(matchesExpectedEnvelope(FormatSLSAV02, "https://in-toto.io/Statement/v0.1", "https://slsa.dev/provenance/v0.2")).To(BeTrue())
	Expect(matchesExpectedEnvelope(FormatSLSAV02, "https://in-toto.io/Statement/v1", "https://slsa.dev/provenance/v0.2")).To(BeFalse(),
		"v0.2 requires the v0.1 statement type exactly")
	Expect(matchesExpectedEnvelope(Format("bogus"), "x", "y")).To(BeFalse())
}

func TestValidateStatementContentInvalidJSON(t *testing.T) {
	RegisterTestingT(t)

	err := ValidateStatementContent([]byte(`{not json`), ValidateOptions{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("parsing provenance statement"))
}

func TestValidateStatementContentMissingDigest(t *testing.T) {
	RegisterTestingT(t)

	// Subject present but lacks a sha256 digest -> "subject digest is missing".
	statement := []byte(`{"subject":[{"name":"img","digest":{}}],"predicate":{}}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedDigest: "sha256:aaaa",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("subject digest is missing"))
}

func TestValidateStatementContentInvalidPredicate(t *testing.T) {
	RegisterTestingT(t)

	// predicate is a JSON string, not an object -> unmarshal into map fails.
	statement := []byte(`{"subject":[],"predicate":"not-an-object"}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedBuilderID: "x",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("parsing provenance predicate"))
}

func TestValidateStatementContentMissingBuilderID(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{"subject":[],"predicate":{}}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedBuilderID: "expected-builder",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("builder ID is missing"))
}

func TestValidateStatementContentBuilderIDMismatch(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{"subject":[],"predicate":{"runDetails":{"builder":{"id":"actual"}}}}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedBuilderID: "expected",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("builder ID mismatch"))
}

func TestValidateStatementContentSourceDependencyMissing(t *testing.T) {
	RegisterTestingT(t)

	statement := []byte(`{"subject":[],"predicate":{"buildDefinition":{"resolvedDependencies":[]}}}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedSourceURI: "https://github.com/owner/repo.git",
		ExpectedCommit:    "abc",
	})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("do not contain expected source dependency"))
}

func TestValidateStatementContentV02Materials(t *testing.T) {
	RegisterTestingT(t)

	// v0.2 predicate uses "materials" and "builder.id" (not runDetails).
	statement := []byte(`{
  "_type":"https://in-toto.io/Statement/v0.1",
  "predicateType":"https://slsa.dev/provenance/v0.2",
  "subject":[{"name":"img","digest":{"sha256":"abcd"}}],
  "predicate":{
    "builder":{"id":"builder-x"},
    "materials":[{"uri":"git://repo","digest":{"sha1":"sha-commit"}}]
  }
}`)
	err := ValidateStatementContent(statement, ValidateOptions{
		ExpectedFormat:    FormatSLSAV02,
		ExpectedDigest:    "abcd",
		ExpectedBuilderID: "builder-x",
		ExpectedSourceURI: "git://repo",
		ExpectedCommit:    "sha-commit",
	})
	Expect(err).ToNot(HaveOccurred())
}

func TestStatementValidateDelegates(t *testing.T) {
	RegisterTestingT(t)

	// (*Statement).Validate just forwards to ValidateStatementContent.
	stmt := &Statement{Content: []byte(`{"subject":[],"predicate":{}}`)}
	Expect(stmt.Validate(ValidateOptions{})).To(Succeed())

	bad := &Statement{Content: []byte(`{bad`)}
	Expect(bad.Validate(ValidateOptions{})).To(HaveOccurred())
}

func TestHasExpectedDependency(t *testing.T) {
	RegisterTestingT(t)

	deps := []map[string]interface{}{
		{"uri": "git://a", "digest": map[string]interface{}{"sha1": "commitA"}},
		{"uri": "git://b", "digest": map[string]interface{}{"sha256": "sha256:commitB"}},
	}

	cases := []struct {
		name      string
		uri       string
		commit    string
		want      bool
	}{
		{name: "uri match, no commit required", uri: "git://a", commit: "", want: true},
		{name: "uri + sha1 commit match", uri: "git://a", commit: "commitA", want: true},
		{name: "uri + sha256 commit match with prefix normalized", uri: "git://b", commit: "commitB", want: true},
		{name: "uri match but commit mismatch", uri: "git://a", commit: "wrong", want: false},
		{name: "uri not present", uri: "git://c", commit: "", want: false},
		{name: "empty uri matches first dep with commit", uri: "", commit: "commitA", want: true},
		{name: "empty uri and empty commit matches anything", uri: "", commit: "", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(hasExpectedDependency(deps, tc.uri, tc.commit)).To(Equal(tc.want))
		})
	}

	t.Run("empty dependency list", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(hasExpectedDependency(nil, "git://a", "")).To(BeFalse())
	})
}

func TestNestedHelpers(t *testing.T) {
	RegisterTestingT(t)

	root := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "value",
			},
			"slice": []interface{}{
				map[string]interface{}{"x": 1},
				"raw",
			},
		},
		"num": 42,
	}

	t.Run("nestedString deep hit", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedString(root, "a", "b", "c")).To(Equal("value"))
	})
	t.Run("nestedString path breaks on non-map", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedString(root, "num", "deeper")).To(Equal(""))
	})
	t.Run("nestedString leaf not a string", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedString(root, "num")).To(Equal(""))
	})
	t.Run("nestedSlice hit", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedSlice(root, "a", "slice")).To(HaveLen(2))
	})
	t.Run("nestedSlice path breaks on non-map", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedSlice(root, "num", "x")).To(BeNil())
	})
	t.Run("nestedSlice leaf not a slice", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(nestedSlice(root, "a", "b")).To(BeNil())
	})
}

func TestAsString(t *testing.T) {
	RegisterTestingT(t)

	Expect(asString("hello")).To(Equal("hello"))
	Expect(asString(123)).To(Equal(""))
	Expect(asString(nil)).To(Equal(""))
}

func TestPredicateBuilderID(t *testing.T) {
	RegisterTestingT(t)

	t.Run("from runDetails", func(t *testing.T) {
		RegisterTestingT(t)
		p := map[string]interface{}{
			"runDetails": map[string]interface{}{"builder": map[string]interface{}{"id": "rd-id"}},
		}
		Expect(predicateBuilderID(p)).To(Equal("rd-id"))
	})
	t.Run("falls back to top-level builder", func(t *testing.T) {
		RegisterTestingT(t)
		p := map[string]interface{}{
			"builder": map[string]interface{}{"id": "top-id"},
		}
		Expect(predicateBuilderID(p)).To(Equal("top-id"))
	})
	t.Run("missing -> empty", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(predicateBuilderID(map[string]interface{}{})).To(Equal(""))
	})
}

func TestPredicateDependencies(t *testing.T) {
	RegisterTestingT(t)

	t.Run("from resolvedDependencies", func(t *testing.T) {
		RegisterTestingT(t)
		p := map[string]interface{}{
			"buildDefinition": map[string]interface{}{
				"resolvedDependencies": []interface{}{
					map[string]interface{}{"uri": "x"},
					"not-a-map", // skipped, not a map
				},
			},
		}
		deps := predicateDependencies(p)
		Expect(deps).To(HaveLen(1))
		Expect(deps[0]).To(HaveKey("uri"))
	})
	t.Run("falls back to materials", func(t *testing.T) {
		RegisterTestingT(t)
		p := map[string]interface{}{
			"materials": []interface{}{map[string]interface{}{"uri": "m"}},
		}
		deps := predicateDependencies(p)
		Expect(deps).To(HaveLen(1))
	})
	t.Run("none -> empty slice", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(predicateDependencies(map[string]interface{}{})).To(HaveLen(0))
	})
}

func TestNormalizeDigest(t *testing.T) {
	RegisterTestingT(t)

	Expect(normalizeDigest("sha256:abc")).To(Equal("abc"))
	Expect(normalizeDigest("abc")).To(Equal("abc"))
	Expect(normalizeDigest("")).To(Equal(""))
}

func TestSubjectDigest(t *testing.T) {
	RegisterTestingT(t)

	type subj = struct {
		Name   string            `json:"name"`
		Digest map[string]string `json:"digest"`
	}

	t.Run("returns first sha256 normalized", func(t *testing.T) {
		RegisterTestingT(t)
		subjects := []subj{
			{Name: "no-digest", Digest: map[string]string{}},
			{Name: "has", Digest: map[string]string{"sha256": "sha256:deadbeef"}},
		}
		Expect(subjectDigest(subjects)).To(Equal("deadbeef"))
	})
	t.Run("empty when no sha256", func(t *testing.T) {
		RegisterTestingT(t)
		subjects := []subj{{Name: "x", Digest: map[string]string{"sha512": "z"}}}
		Expect(subjectDigest(subjects)).To(Equal(""))
	})
}

func TestSubjectDescriptor(t *testing.T) {
	RegisterTestingT(t)

	t.Run("with digest", func(t *testing.T) {
		RegisterTestingT(t)
		d := subjectDescriptor("repo/img@sha256:abc")
		Expect(d["name"]).To(Equal("repo/img@sha256:abc"))
		Expect(d["digest"]).To(Equal(map[string]string{"sha256": "abc"}))
	})
	t.Run("without digest", func(t *testing.T) {
		RegisterTestingT(t)
		d := subjectDescriptor("repo/img:tag")
		Expect(d["name"]).To(Equal("repo/img:tag"))
		Expect(d).ToNot(HaveKey("digest"))
	})
}

func TestExternalParameters(t *testing.T) {
	RegisterTestingT(t)

	t.Run("image only", func(t *testing.T) {
		RegisterTestingT(t)
		p := externalParameters("img", GenerateOptions{})
		Expect(p).To(HaveKeyWithValue("image", "img"))
		Expect(p).ToNot(HaveKey("contextPath"))
		Expect(p).ToNot(HaveKey("dockerfilePath"))
	})
	t.Run("with context and dockerfile", func(t *testing.T) {
		RegisterTestingT(t)
		p := externalParameters("img", GenerateOptions{ContextPath: "ctx", DockerfilePath: "Dockerfile"})
		Expect(p).To(HaveKeyWithValue("contextPath", "ctx"))
		Expect(p).To(HaveKeyWithValue("dockerfilePath", "Dockerfile"))
	})
}

func TestInternalParameters(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled -> empty map", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(internalParameters(GenerateOptions{IncludeEnv: false})).To(BeEmpty())
	})

	t.Run("enabled collects only set env vars", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("CI", "true")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		// Ensure an unset variable is excluded.
		Expect(os.Unsetenv("GITHUB_RUN_ATTEMPT")).To(Succeed())

		p := internalParameters(GenerateOptions{IncludeEnv: true})
		env, ok := p["environment"].(map[string]string)
		Expect(ok).To(BeTrue())
		Expect(env).To(HaveKeyWithValue("CI", "true"))
		Expect(env).To(HaveKeyWithValue("GITHUB_REPOSITORY", "owner/repo"))
		Expect(env).ToNot(HaveKey("GITHUB_RUN_ATTEMPT"))
	})
}

func TestResolvedDependencies(t *testing.T) {
	RegisterTestingT(t)

	t.Run("nil metadata -> nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(resolvedDependencies(nil, GenerateOptions{IncludeMaterials: true})).To(BeNil())
	})
	t.Run("materials disabled -> nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(resolvedDependencies(&Metadata{SourceURI: "u", GitCommit: "c"}, GenerateOptions{IncludeMaterials: false})).To(BeNil())
	})
	t.Run("source dependency added", func(t *testing.T) {
		RegisterTestingT(t)
		deps := resolvedDependencies(
			&Metadata{SourceURI: "git://repo", GitCommit: "abc123"},
			GenerateOptions{IncludeMaterials: true},
		)
		Expect(deps).To(HaveLen(1))
		Expect(deps[0]).To(HaveKeyWithValue("uri", "git://repo"))
		Expect(deps[0]["digest"]).To(Equal(map[string]string{"sha1": "abc123"}))
	})
	t.Run("dockerfile dependency added with sha256", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		df := filepath.Join(dir, "Dockerfile")
		Expect(os.WriteFile(df, []byte("FROM scratch\n"), 0o600)).To(Succeed())

		deps := resolvedDependencies(
			&Metadata{},
			GenerateOptions{IncludeMaterials: true, IncludeDockerfile: true, DockerfilePath: df},
		)
		Expect(deps).To(HaveLen(1))
		Expect(deps[0]).To(HaveKeyWithValue("uri", filepath.Clean(df)))
		digest, ok := deps[0]["digest"].(map[string]string)
		Expect(ok).To(BeTrue())
		sum := sha256.Sum256([]byte("FROM scratch\n"))
		Expect(digest["sha256"]).To(Equal(hex.EncodeToString(sum[:])))
	})
	t.Run("dockerfile missing -> skipped silently", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		df := filepath.Join(dir, "does-not-exist")
		deps := resolvedDependencies(
			&Metadata{},
			GenerateOptions{IncludeMaterials: true, IncludeDockerfile: true, DockerfilePath: df},
		)
		// fileSHA256 errors -> dependency not appended.
		Expect(deps).To(BeNil())
	})
}

func TestFileSHA256(t *testing.T) {
	RegisterTestingT(t)

	t.Run("hashes existing file", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		f := filepath.Join(dir, "data.txt")
		Expect(os.WriteFile(f, []byte("hello"), 0o600)).To(Succeed())
		sum, err := fileSHA256(f)
		Expect(err).ToNot(HaveOccurred())
		want := sha256.Sum256([]byte("hello"))
		Expect(sum).To(Equal(hex.EncodeToString(want[:])))
	})
	t.Run("missing file errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := fileSHA256(filepath.Join(t.TempDir(), "nope"))
		Expect(err).To(HaveOccurred())
	})
}

func TestDetectBuilderID(t *testing.T) {
	RegisterTestingT(t)

	// Clear all CI-related env so each subtest controls its own scenario.
	clearBuilderEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{
			"GITHUB_SERVER_URL", "GITHUB_REPOSITORY", "GITHUB_RUN_ID",
			"CI_PROJECT_URL", "CI_PIPELINE_ID",
		} {
			t.Setenv(k, "")
			Expect(os.Unsetenv(k)).To(Succeed())
		}
	}

	t.Run("configured value wins", func(t *testing.T) {
		RegisterTestingT(t)
		clearBuilderEnv(t)
		Expect(detectBuilderID("explicit-id")).To(Equal("explicit-id"))
	})

	t.Run("github actions", func(t *testing.T) {
		RegisterTestingT(t)
		clearBuilderEnv(t)
		t.Setenv("GITHUB_SERVER_URL", "https://github.com")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_RUN_ID", "999")
		Expect(detectBuilderID("")).To(Equal("https://github.com/owner/repo/actions/runs/999"))
	})

	t.Run("gitlab ci", func(t *testing.T) {
		RegisterTestingT(t)
		clearBuilderEnv(t)
		t.Setenv("CI_PROJECT_URL", "https://gitlab.com/g/p")
		t.Setenv("CI_PIPELINE_ID", "555")
		Expect(detectBuilderID("")).To(Equal("https://gitlab.com/g/p/-/pipelines/555"))
	})

	t.Run("falls back to local hostname", func(t *testing.T) {
		RegisterTestingT(t)
		clearBuilderEnv(t)
		got := detectBuilderID("")
		// With no CI env, builder ID is local://<hostname> (or the static fallback
		// if os.Hostname() fails, which it should not in this environment).
		Expect(strings.HasPrefix(got, "local://")).To(BeTrue())
	})

	t.Run("github server set but repo missing falls through to local", func(t *testing.T) {
		RegisterTestingT(t)
		clearBuilderEnv(t)
		t.Setenv("GITHUB_SERVER_URL", "https://github.com")
		// No GITHUB_REPOSITORY / GITHUB_RUN_ID -> skip Actions branch.
		got := detectBuilderID("")
		Expect(strings.HasPrefix(got, "local://")).To(BeTrue())
	})
}

func TestDetectInvocationID(t *testing.T) {
	RegisterTestingT(t)

	t.Run("uses GITHUB_RUN_ID when present", func(t *testing.T) {
		RegisterTestingT(t)
		for _, k := range []string{"GITHUB_RUN_ID", "CI_PIPELINE_ID", "BUILD_BUILDID"} {
			Expect(os.Unsetenv(k)).To(Succeed())
		}
		t.Setenv("GITHUB_RUN_ID", "abc-run")
		Expect(detectInvocationID()).To(Equal("abc-run"))
	})

	t.Run("falls back to local- prefix", func(t *testing.T) {
		RegisterTestingT(t)
		for _, k := range []string{"GITHUB_RUN_ID", "CI_PIPELINE_ID", "BUILD_BUILDID"} {
			t.Setenv(k, "")
			Expect(os.Unsetenv(k)).To(Succeed())
		}
		Expect(strings.HasPrefix(detectInvocationID(), "local-")).To(BeTrue())
	})
}

func TestGitOutputAndDetectGitMetadata(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	t.Run("gitOutput returns empty for non-git directory", func(t *testing.T) {
		RegisterTestingT(t)
		// A fresh temp dir is not a git repo; `git -C <dir> ...` fails and the
		// helper swallows the error, returning "".
		Expect(gitOutput(ctx, t.TempDir(), "rev-parse", "HEAD")).To(Equal(""))
	})

	t.Run("detectGitMetadata never panics and returns three strings", func(t *testing.T) {
		RegisterTestingT(t)
		// Whether or not git is on PATH, this must return cleanly. On a non-git
		// dir all three are empty; the contract is "best-effort, never error".
		remote, commit, branch := detectGitMetadata(ctx, t.TempDir())
		Expect(remote).To(Equal(""))
		Expect(commit).To(Equal(""))
		Expect(branch).To(Equal(""))
	})
}

func TestBuildPredicateSLSAV10(t *testing.T) {
	RegisterTestingT(t)

	meta := &Metadata{BuilderID: "builder://x", SourceURI: "git://repo", GitCommit: "c0ffee"}
	opts := GenerateOptions{
		ContextPath:      "ctx",
		DockerfilePath:   "Dockerfile",
		IncludeMaterials: true,
	}
	content, err := buildPredicate(FormatSLSAV10, "repo/img@sha256:abc", meta, opts)
	Expect(err).ToNot(HaveOccurred())

	var doc map[string]interface{}
	Expect(json.Unmarshal(content, &doc)).To(Succeed())
	Expect(doc["_type"]).To(Equal("https://in-toto.io/Statement/v1"))
	Expect(doc["predicateType"]).To(Equal("https://slsa.dev/provenance/v1"))

	pred := doc["predicate"].(map[string]interface{})
	bd := pred["buildDefinition"].(map[string]interface{})
	Expect(bd["buildType"]).To(Equal("https://simple-container.com/container-image@v1"))

	rd := pred["runDetails"].(map[string]interface{})
	builder := rd["builder"].(map[string]interface{})
	Expect(builder["id"]).To(Equal("builder://x"))

	// The subject digest is extracted from the image ref.
	subjects := doc["subject"].([]interface{})
	Expect(subjects).To(HaveLen(1))
	subj := subjects[0].(map[string]interface{})
	Expect(subj["digest"]).To(Equal(map[string]interface{}{"sha256": "abc"}))
}

func TestBuildPredicateSLSAV02(t *testing.T) {
	RegisterTestingT(t)

	meta := &Metadata{BuilderID: "builder://y", SourceURI: "git://repo", GitCommit: "c0ffee"}
	opts := GenerateOptions{IncludeMaterials: true}
	content, err := buildPredicate(FormatSLSAV02, "repo/img@sha256:abc", meta, opts)
	Expect(err).ToNot(HaveOccurred())

	var doc map[string]interface{}
	Expect(json.Unmarshal(content, &doc)).To(Succeed())
	Expect(doc["_type"]).To(Equal("https://in-toto.io/Statement/v0.1"))
	Expect(doc["predicateType"]).To(Equal("https://slsa.dev/provenance/v0.2"))

	pred := doc["predicate"].(map[string]interface{})
	builder := pred["builder"].(map[string]interface{})
	Expect(builder["id"]).To(Equal("builder://y"))
	Expect(pred).To(HaveKey("materials"))
}

func TestBuildPredicateUnsupportedFormat(t *testing.T) {
	RegisterTestingT(t)

	_, err := buildPredicate(Format("bogus"), "img", &Metadata{}, GenerateOptions{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("unsupported provenance format"))
}

func TestAttachEarlyErrorOnBadPredicate(t *testing.T) {
	RegisterTestingT(t)

	// Invalid JSON content makes statement.Predicate() fail before any cosign
	// exec, exercising Attach's first error path without needing the binary.
	a := NewAttacher(&signing.Config{Keyless: true})
	stmt := &Statement{Content: []byte(`{not json`)}
	err := a.Attach(context.Background(), stmt, "repo/img@sha256:abc")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("parsing provenance statement"))
}

func TestAttachCosignInvocationFails(t *testing.T) {
	RegisterTestingT(t)

	// With a valid predicate the temp file is written and cosign is invoked.
	// cosign is not available (or the image ref is bogus), so cmd.Run() errors
	// and Attach wraps it with "cosign attest failed". This covers the full
	// body up to the exec error branch (the success `return nil` is the only
	// genuinely cosign-dependent line left).
	a := NewAttacher(&signing.Config{Keyless: true})
	a.Timeout = 5 * time.Second
	stmt := &Statement{Content: []byte(`{"predicate":{"builder":{"id":"x"}}}`)}
	err := a.Attach(context.Background(), stmt, "localhost:0/invalid@sha256:abc")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cosign attest failed"))
}

func TestVerifyCosignInvocationFails(t *testing.T) {
	RegisterTestingT(t)

	// cosign verify-attestation is invoked and fails (binary missing or image
	// invalid), exercising Verify up to its exec error branch.
	a := NewAttacher(&signing.Config{Keyless: true, IdentityRegexp: "i", OIDCIssuer: "https://o"})
	a.Timeout = 5 * time.Second
	_, err := a.Verify(context.Background(), "localhost:0/invalid@sha256:abc", FormatSLSAV10)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cosign verify-attestation failed"))
}

func TestGenerate(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	t.Run("default format and roundtrips through statement", func(t *testing.T) {
		RegisterTestingT(t)
		stmt, err := Generate(ctx, "repo/img@sha256:abc", "", GenerateOptions{
			BuilderID: "explicit-builder",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(stmt.Format).To(Equal(FormatSLSAV10))
		Expect(stmt.ImageRef).To(Equal("repo/img@sha256:abc"))
		Expect(stmt.Metadata.BuilderID).To(Equal("explicit-builder"))
		// SourceRoot defaults to "." but git metadata only collected when IncludeGit.
		Expect(stmt.Metadata.SourceURI).To(Equal(""))

		// The generated content is a parseable in-toto statement.
		format, derr := DetectFormat(stmt.Content)
		Expect(derr).ToNot(HaveOccurred())
		Expect(format).To(Equal(FormatSLSAV10))
	})

	t.Run("with git metadata on non-git source root stays empty", func(t *testing.T) {
		RegisterTestingT(t)
		stmt, err := Generate(ctx, "repo/img@sha256:abc", FormatSLSAV10, GenerateOptions{
			BuilderID:  "b",
			SourceRoot: t.TempDir(),
			IncludeGit: true,
		})
		Expect(err).ToNot(HaveOccurred())
		// Non-git dir -> detectGitMetadata returns empties.
		Expect(stmt.Metadata.SourceURI).To(Equal(""))
		Expect(stmt.Metadata.GitCommit).To(Equal(""))
		Expect(stmt.Metadata.GitBranch).To(Equal(""))
	})
}
