// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package caddy holds the parts of SC's Caddy configuration that both edges render.
//
// SC runs Caddy in two places — a container on a vm resource's machines
// (pkg/clouds/vm) and a Deployment in front of a Kubernetes cluster
// (pkg/clouds/pulumi/kubernetes) — and they are configured by different structs. That
// is fine for anything genuinely path-specific, and a liability for anything whose
// correctness lives in Caddy or in a Caddy module, because then the knowledge has to be
// right twice. The shared ACME store was the first case where it was right once: the vm
// path had `ingress.acmeStorage` and the Kubernetes path's only route to the same
// feature was hand-writing the block into `caddyfilePrefix`, with the module's two
// silent parser rules to rediscover and the credentials in cleartext.
//
// Put a thing here when getting it wrong would be invisible until a certificate fails
// to renew.
package caddy

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

const (
	// The three credential env vars. Rendered into a Caddyfile as `{$NAME}`
	// placeholders and resolved by Caddy's own parser at config-load time, so the
	// value is never in the generated file: on the vm path they arrive in a 0600 env
	// file, on the Kubernetes path in a Secret.
	EnvAcmeS3AccessKey     = "SC_ACME_S3_ACCESS_KEY"
	EnvAcmeS3SecretKey     = "SC_ACME_S3_SECRET_KEY"
	EnvAcmeS3EncryptionKey = "SC_ACME_S3_ENCRYPTION_KEY"

	// DefaultAcmeStoragePrefix matches certmagic-s3's own default. Named here because
	// it is rendered explicitly — a generated Caddyfile that says where the
	// certificates are beats one that leaves it to the module.
	DefaultAcmeStoragePrefix = "acme"

	// DefaultAcmeStorageRegion applies only when an endpoint is set, which means the
	// store is not real AWS and conventionally ignores the region.
	DefaultAcmeStorageRegion = "us-east-1"

	// AcmeStorageEncryptionKeyLen is secretbox's key size. certmagic-s3 rejects any
	// other length.
	AcmeStorageEncryptionKeyLen = 32
)

// AcmeStorage is an S3-compatible bucket holding a fleet's certificates and ACME
// challenge state, rendered as Caddy's `storage s3` global option.
//
// S3 and not one module per cloud. The image also carries `caddy.storage.gcs`, but GCS
// serves an S3-compatible interop endpoint and `gcp-bucket` already exports HMAC
// credentials for it, so one code path covers Yandex, AWS, GCP, minio and anything
// else that speaks S3. The GCS module stays what GKE's edge uses. There is deliberately
// no `type:` field: one legal value is not a choice, and adding the field later with a
// default of `s3` stays backwards compatible.
//
// Every field is a property of the STORE, not of a machine or a replica — the whole
// point is that every front door writes to the same place.
type AcmeStorage struct {
	// Bucket is required. It must already exist: SC does not create it here, because
	// the bucket for a fleet generally lives in the same parent stack (a `yc-bucket` /
	// `gcp-bucket` resource) or on an operator-run minio, and inventing a second
	// owner for it would make a `sc destroy` able to take the certificates with it.
	Bucket string `json:"bucket" yaml:"bucket"`
	// Endpoint is the store's base URL, scheme included
	// (`https://storage.yandexcloud.net`, `http://10.0.0.4:9000`). Empty means real
	// AWS S3.
	//
	// Setting it also switches the SDK to computing request checksums only when
	// required, which is what makes most non-AWS implementations work at all.
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// Region is required when Endpoint is empty, since that is real AWS and the wrong
	// region is a failed request rather than a redirect. With an Endpoint it defaults
	// to `us-east-1`, which S3-compatible stores conventionally ignore.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
	// AccessKey / SecretKey are the static credential, both or neither.
	//
	// Neither means the AWS SDK's default chain — an EC2 instance role, in practice.
	// That works on AWS and nowhere else: a VPS has no instance metadata, so on
	// Cloud.ru, MWS or byo, omitting them is a silent fall-through to "no credentials"
	// whose only symptom is certificates that never issue. SecretKey is a credential:
	// write it as `${secret:...}`. Neither value reaches the rendered Caddyfile — see
	// CredentialEnv.
	AccessKey string `json:"accessKey,omitempty" yaml:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty" yaml:"secretKey,omitempty"`
	// Prefix is the key prefix inside the bucket. Defaults to `acme`.
	//
	// Worth setting when one bucket serves several fleets: two fleets sharing a prefix
	// share locks and certificates, which is correct only if they also share every
	// domain name.
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	// PathStyle selects path-style addressing (`<endpoint>/<bucket>/<key>`) over
	// virtual-host style (`<bucket>.<endpoint>/<key>`). Defaults to TRUE whenever
	// Endpoint is set and false otherwise.
	//
	// That default is the useful one: virtual-host addressing needs the bucket name to
	// resolve as a DNS label under the endpoint, which minio does not serve at all and
	// several S3-compatible clouds serve only for some bucket names. Real AWS prefers
	// virtual-host. Set it explicitly to override.
	PathStyle *bool `json:"pathStyle,omitempty" yaml:"pathStyle,omitempty"`
	// Insecure disables TLS verification against the store. For a minio with a
	// self-signed certificate, and nothing else — the credential and every private key
	// the fleet owns travel over this connection. Prefer plain `http://` on a private
	// network, which is at least honest about what it is.
	Insecure bool `json:"insecure,omitempty" yaml:"insecure,omitempty"`
	// EncryptionKey encrypts the stored objects with secretbox. Exactly 32 bytes when
	// set; a credential, so write it as `${secret:...}`.
	//
	// Unset means the module logs `Clear text certificate storage active` and stores
	// the fleet's private keys as they are. Acceptable when the bucket is private AND
	// the cloud encrypts at rest; on a minio running on one of the fleet's own
	// machines it is neither, so set it there.
	EncryptionKey string `json:"encryptionKey,omitempty" yaml:"encryptionKey,omitempty"`
}

// Enabled reports whether this store is configured enough to render. A nil store and a
// store with no bucket both mean "no shared store", because a bucket is the one field
// with no useful default.
func (s *AcmeStorage) Enabled() bool {
	return s != nil && s.Bucket != ""
}

// UsePathStyle resolves PathStyle's default: on whenever there is an endpoint.
func (s *AcmeStorage) UsePathStyle() bool {
	if s.PathStyle != nil {
		return *s.PathStyle
	}
	return s.Endpoint != ""
}

// StoragePrefix resolves Prefix's default. Rendered explicitly rather than left to the
// module, so the generated Caddyfile says where the fleet's certificates actually are.
func (s *AcmeStorage) StoragePrefix() string {
	if s.Prefix != "" {
		return s.Prefix
	}
	return DefaultAcmeStoragePrefix
}

// StorageRegion resolves Region's default, which only applies when there is an endpoint
// — Validate refuses an empty region without one.
func (s *AcmeStorage) StorageRegion() string {
	if s.Region != "" {
		return s.Region
	}
	return DefaultAcmeStorageRegion
}

// CredentialEnv is the name→value map of everything that must NOT be rendered into the
// Caddyfile: the two static-credential halves and the encryption key. The caller's job
// is to deliver these out of band (a 0600 env file on a machine, a Secret in a cluster)
// under exactly these names, because WriteStorageBlock renders `{$NAME}` placeholders
// for them.
//
// AccessKey and SecretKey come as a pair or not at all. The block renders the two
// placeholders together, and a placeholder with no value expands to the empty string
// rather than failing — which would read as "the credential is wrong" instead of "the
// credential is missing".
func (s *AcmeStorage) CredentialEnv() map[string]string {
	env := map[string]string{}
	if s == nil {
		return env
	}
	if s.AccessKey != "" {
		env[EnvAcmeS3AccessKey] = s.AccessKey
		env[EnvAcmeS3SecretKey] = s.SecretKey
	}
	if s.EncryptionKey != "" {
		env[EnvAcmeS3EncryptionKey] = s.EncryptionKey
	}
	return env
}

// Validate checks the shared certificate store. `field` is the operator-facing path of
// this struct in the surrounding config (`ingress.acmeStorage` on a vm resource,
// `caddy.acmeStorage` on a Kubernetes one) so every refusal names something greppable in
// the file the operator is holding.
//
// Every refusal here replaces a failure that is otherwise invisible until a certificate
// does not renew, which is weeks after the deploy that caused it and on a machine nobody
// is watching. The store is only consulted on issuance and renewal, so a misconfigured
// one serves perfectly from cache until it matters.
func (s *AcmeStorage) Validate(field string) error {
	if s == nil {
		return nil
	}
	if s.Bucket == "" {
		return errors.Errorf(
			"`%s` is set without `bucket`: the shared certificate store needs a bucket "+
				"that already exists (SC does not create it — see the field docs)",
			field,
		)
	}
	if (s.AccessKey == "") != (s.SecretKey == "") {
		return errors.Errorf(
			"`%s.accessKey` and `.secretKey` must be set together: the module uses the "+
				"static credential only when it has both, and otherwise falls through to the AWS SDK's "+
				"default chain — which on a VPS finds nothing and presents as certificates that never issue",
			field,
		)
	}
	if s.Endpoint == "" && s.Region == "" {
		return errors.Errorf(
			"`%s` has neither `endpoint` nor `region`: no endpoint means real AWS S3, "+
				"where the wrong region is a failed request rather than a redirect. Set `region`, or set "+
				"`endpoint` (then `region` defaults to %q, which S3-compatible stores ignore)",
			field, DefaultAcmeStorageRegion,
		)
	}
	if s.Endpoint != "" && !strings.HasPrefix(s.Endpoint, "http://") && !strings.HasPrefix(s.Endpoint, "https://") {
		return errors.Errorf(
			"`%s.endpoint` %q has no scheme: it is used as the SDK's base endpoint and "+
				"must be a URL (`https://storage.yandexcloud.net`, `http://10.0.0.4:9000`)",
			field, s.Endpoint,
		)
	}
	if k := s.EncryptionKey; k != "" && len(k) != AcmeStorageEncryptionKeyLen {
		// Checked here even though the module checks it too, because the module's
		// check lands in two different places depending on how the value arrives, and
		// SC always makes it arrive the harder way: a literal fails Caddyfile parsing
		// (visible, the deploy fails), but SC delivers this as a `{$...}` placeholder,
		// and a substituted wrong-length key reaches `Provision` — which returns an
		// error that is only in the Caddy container's own log.
		return errors.Errorf(
			"`%s.encryptionKey` is %d bytes; secretbox needs exactly %d. Leave it unset "+
				"to store the fleet's private keys unencrypted (acceptable only when the bucket is private "+
				"and the cloud encrypts at rest)",
			field, len(k), AcmeStorageEncryptionKeyLen,
		)
	}
	if s.Insecure && strings.HasPrefix(s.Endpoint, "http://") {
		return errors.Errorf(
			"`%s.insecure: true` with an `http://` endpoint: there is no TLS to skip "+
				"verifying. Drop `insecure`",
			field,
		)
	}
	return nil
}

// WriteStorageBlock renders the global `storage s3` block — the thing that makes origin
// HTTPS possible on a fleet at all, rather than a tuning knob.
//
// `indent` prefixes every line and is doubled for the subdirectives inside the block,
// because the two edges nest their global options differently (a tab on the vm path, two
// spaces on the Kubernetes one) and a Caddyfile is whitespace-insensitive but diffs are
// not. `logHint` completes the "if certificates never issue, look here" sentence with the
// command that reads the right log on this edge; an empty string omits it.
//
// The block ends with a newline. A caller assembling it into a larger block alongside
// others should TrimRight it, since Caddy accepts the blank line and a reviewer reads it
// as an accident.
//
// Module: github.com/techknowlogick/certmagic-s3, baked into simplecontainer/caddy and
// gated by the image build's `caddy list-modules | grep -qE '^caddy\.storage\.s3$'`.
// An SC binary newer than the image it names will render a block the image's Caddy
// cannot parse, and Caddy answers that with a config-load failure at container start —
// which is loud, and the correct way for that skew to present.
//
// Two rendering rules, both forced by the module's own parser (s3.go, UnmarshalCaddyfile):
//
//   - EVERY BOOLEAN CARRIES AN EXPLICIT VALUE. The parser reads each subdirective with
//     `if !d.Args(&value) { continue }`, so a bare `use_path_style` is not an error and
//     is not true — it is SILENTLY SKIPPED. A fleet on minio with a bare `use_path_style`
//     would therefore address buckets virtual-host style, fail every request, and say
//     nothing about why.
//   - CREDENTIALS ARE `{$VAR}` PLACEHOLDERS, resolved by Caddy's parser at load time from
//     whatever the caller of CredentialEnv delivered. The Caddyfile is world-readable
//     inside the container and is echoed by `caddy adapt`. They do not go through Token,
//     which would quote the braces into a literal.
func WriteStorageBlock(b *strings.Builder, s *AcmeStorage, indent, logHint string) {
	p := func(format string, args ...any) {
		b.WriteString(indent)
		fmt.Fprintf(b, format, args...)
	}
	// One level in, in the caller's own unit. "\t" would be right on the vm path and
	// wrong next to the two-space `servers { trusted_proxies ... }` the Kubernetes edge
	// renders in the same block.
	pp := func(format string, args ...any) {
		b.WriteString(indent)
		p(format, args...)
	}

	p("# Shared certificate store — every front door of this fleet points at this one\n")
	p("# bucket. That is what lets HTTP-01 work behind N A records: certmagic keeps\n")
	p("# the challenge state in storage, so whichever one the CA happens to dial\n")
	p("# can answer a token a sibling minted, and the fleet holds ONE certificate.\n")
	if logHint != "" {
		p("# If certificates never issue, read %s for EVERY replica, not one.\n", logHint)
	}
	p("storage s3 {\n")
	pp("bucket %s\n", Token(s.Bucket))
	if s.Endpoint != "" {
		pp("endpoint %s\n", Token(s.Endpoint))
	}
	pp("region %s\n", Token(s.StorageRegion()))
	pp("prefix %s\n", Token(s.StoragePrefix()))
	// Always rendered, never bare: the effective default is SC's (true whenever a custom
	// endpoint is set), not the module's, so writing it out is also the only way a reader
	// of the generated file can tell which addressing style is in force.
	pp("use_path_style %t\n", s.UsePathStyle())
	if s.Insecure {
		pp("# TLS verification disabled on the resource (`acmeStorage.insecure`).\n")
		pp("insecure true\n")
	}
	if s.AccessKey != "" {
		// Both or neither — Validate refuses one without the other, because the module
		// only uses static credentials when both are set and otherwise falls through to
		// the AWS default chain, which on a VPS finds nothing.
		pp("access_key {$%s}\n", EnvAcmeS3AccessKey)
		pp("secret_key {$%s}\n", EnvAcmeS3SecretKey)
	}
	if s.EncryptionKey != "" {
		pp("encryption_key {$%s}\n", EnvAcmeS3EncryptionKey)
	} else {
		// The module logs `Clear text certificate storage active` on every start when
		// this is absent. That is expected here and not a misconfiguration: the bucket
		// is the operator's own and its contents are the fleet's own certificates.
		pp("# No encryption_key: certificates are stored in clear text. Caddy says so\n")
		pp("# on every start. Set `acmeStorage.encryptionKey` (exactly %d bytes)\n", AcmeStorageEncryptionKeyLen)
		pp("# to encrypt them at rest.\n")
	}
	p("}\n")
}

// Token quotes a Caddyfile token when it contains anything Caddy's parser would read as
// structure. Not applied to `{$VAR}` placeholders, which must stay unquoted to be
// substituted.
func Token(v string) string {
	if strings.ContainsAny(v, " \t\"{}") {
		return fmt.Sprintf("%q", v)
	}
	return v
}
