// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	smithy "github.com/aws/smithy-go"
	"github.com/pkg/errors"
)

// kmsRecipientScheme prefixes an AWS KMS recipient in scopes.yaml. The rest is
// sc's canonical KMS key URL — the SAME form the Pulumi state secrets provider
// already uses (see aws.SecretsProviderConfig.KeyName):
//
//	awskms://<key-id|alias/name|arn>?region=<region>
//
// A KMS recipient is the v2 "KeyProvider": instead of sealing the data key to a
// static SSH key, the data key is wrapped with kms:Encrypt and unwrapped with
// kms:Decrypt using the ambient AWS credential chain — which in CI is an
// OIDC-federated role, so no private key is stored in a GitHub secret. It is
// purely additive: a scope with no awskms:// recipient never constructs a KMS
// client and never touches AWS.
const kmsRecipientScheme = "awskms://"

// kmsCallTimeout bounds a single KMS Encrypt/Decrypt (and the ambient-credential
// resolution that precedes it) so a stalled credential probe cannot hang a deploy.
const kmsCallTimeout = 30 * time.Second

// kmsMinCiphertextLen is a conservative lower bound on a real KMS ciphertext blob
// for a 32-byte data key (KMS wraps carry key metadata and are well over 100 bytes).
// It is the offline lint gate: it rejects a raw/plaintext data key (32 bytes) or a
// short string smuggled into a KMS wrap slot without a private key or a KMS call.
const kmsMinCiphertextLen = 64

// kmsAPI is the subset of the AWS KMS client the scoped store uses. It is an
// interface purely so unit tests inject a fake — no unit test ever calls AWS.
type kmsAPI interface {
	Encrypt(ctx context.Context, in *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(ctx context.Context, in *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// newKMSClient builds a KMS client for region using the ambient AWS credential
// chain (env vars, OIDC web-identity token, shared config, or instance profile).
// It is a package var so tests can override it; it is only ever invoked when a
// scope actually has an awskms:// recipient.
var newKMSClient = func(ctx context.Context, region string) (kmsAPI, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS config for KMS")
	}
	return kms.NewFromConfig(cfg), nil
}

// kmsRecipient is a parsed awskms:// recipient.
type kmsRecipient struct {
	raw    string // normalized "awskms://<keyID>?region=<region>" — the wrap-slot map key
	keyID  string // key id, "alias/<name>", or full ARN
	region string
}

// isKMSRecipient reports whether a scopes.yaml recipient entry is an awskms:// URL.
func isKMSRecipient(recipient string) bool {
	return strings.HasPrefix(strings.TrimSpace(recipient), kmsRecipientScheme)
}

// isKMSRecipientID reports whether a wrap-slot map key is a KMS recipient (vs an
// SSH "SHA256:…" fingerprint).
func isKMSRecipientID(id string) bool {
	return strings.HasPrefix(id, kmsRecipientScheme)
}

// parseKMSRecipient parses and normalizes an awskms:// recipient. The region is
// taken from the ?region= query, or derived from a full key ARN when omitted. The
// normalized raw form is stable and offline-derivable, so it is used as the wrap
// map key and for recipient-drift lint without any KMS call.
func parseKMSRecipient(recipient string) (kmsRecipient, error) {
	raw := strings.TrimSpace(recipient)
	if !strings.HasPrefix(raw, kmsRecipientScheme) {
		return kmsRecipient{}, errors.Errorf("not a KMS recipient: %q", recipient)
	}
	rest := strings.TrimPrefix(raw, kmsRecipientScheme)
	keyID := rest
	region := ""
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		keyID = rest[:i]
		q, err := url.ParseQuery(rest[i+1:])
		if err != nil {
			return kmsRecipient{}, errors.Wrapf(err, "invalid query in KMS recipient %q", recipient)
		}
		region = q.Get("region")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return kmsRecipient{}, errors.Errorf("KMS recipient %q has no key id", recipient)
	}
	if region == "" && strings.HasPrefix(keyID, "arn:") {
		// arn:<partition>:kms:<region>:<account>:key/<id> — match structurally so the
		// aws-us-gov and aws-cn partitions derive their region too, not just aws.
		if parts := strings.Split(keyID, ":"); len(parts) >= 6 && parts[0] == "arn" && parts[2] == "kms" {
			region = parts[3]
		}
	}
	if region == "" {
		return kmsRecipient{}, errors.Errorf("KMS recipient %q has no region (use awskms://<key>?region=<region>)", recipient)
	}
	return kmsRecipient{raw: normalizeKMSRecipient(keyID, region), keyID: keyID, region: region}, nil
}

// normalizeKMSRecipient renders the canonical wrap-slot identity for a KMS key.
func normalizeKMSRecipient(keyID, region string) string {
	return fmt.Sprintf("%s%s?region=%s", kmsRecipientScheme, keyID, region)
}

// kmsEncryptionContext binds a KMS-wrapped data key to its (stack, scope, key) —
// the same domain-separated fields the SSH path binds via byte-AAD, expressed as a
// KMS EncryptionContext so the binding is enforced by KMS server-side (a Decrypt
// with a different context fails as InvalidCiphertext). These are non-secret
// identifiers; they are logged in CloudTrail, which is desirable for audit.
func kmsEncryptionContext(stack, scope, key string) map[string]string {
	return map[string]string{
		"sc:domain": aadDomain,
		"sc:stack":  stack,
		"sc:scope":  scope,
		"sc:key":    key,
	}
}

// wrapDEKKMS wraps a data key for a KMS recipient. The operator running set/allow
// needs kms:Encrypt on the key. Returns a single-element slice (the base64 KMS
// ciphertext blob) to match the SSH wrap's []string shape.
func wrapDEKKMS(recipient string, dek []byte, stack, scope, key string) ([]string, error) {
	r, err := parseKMSRecipient(recipient)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kmsCallTimeout)
	defer cancel()
	cli, err := newKMSClient(ctx, r.region)
	if err != nil {
		return nil, err
	}
	out, err := cli.Encrypt(ctx, &kms.EncryptInput{
		KeyId:             aws.String(r.keyID),
		Plaintext:         dek,
		EncryptionContext: kmsEncryptionContext(stack, scope, key),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "KMS encrypt for recipient %s (need kms:Encrypt on the key)", r.raw)
	}
	return []string{base64.StdEncoding.EncodeToString(out.CiphertextBlob)}, nil
}

// decryptKMSWrap unwraps a KMS-wrapped data key with a caller-provided client (the
// Opener caches the client + its credential probe). Its three outcomes map onto the
// resolver's semantics — and, critically, keep them position-independent so a
// per-call KMS hiccup on a later value is never mistaken for a stripped wrap:
//
//   - (dek, true, nil)   — decrypted; the caller is a recipient (has kms:Decrypt).
//   - (nil, false, nil)  — NOT a recipient here: AccessDenied / NotFound / disabled
//     key / invalid key state. Least-privilege skip.
//   - (nil, true, err)   — INTEGRITY failure (tamper): InvalidCiphertext (broken
//     EncryptionContext binding / mangled blob) or IncorrectKey (the pinned key is
//     not the one that produced the ciphertext — a transplant onto a scope whose
//     awskms:// recipient names a different key). The resolver hard-fails as tamper.
//   - (nil, false, err)  — TRANSIENT/unknown backend error (throttle after SDK
//     retries, KMSInternal, timeout, network). The resolver hard-fails as
//     "unavailable" (retry), distinct from tamper and never a silent skip.
func decryptKMSWrap(ctx context.Context, cli kmsAPI, r kmsRecipient, blob []byte, stack, scope, key string) ([]byte, bool, error) {
	out, err := cli.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:    blob,
		KeyId:             aws.String(r.keyID), // pin the key: defense against confused-deputy
		EncryptionContext: kmsEncryptionContext(stack, scope, key),
	})
	if err != nil {
		if isKMSIntegrityError(err) {
			return nil, true, errors.Wrapf(err, "KMS rejected the wrap for %s (tampered blob, wrong key, or wrong stack/scope/key binding)", r.raw)
		}
		if isKMSNotAuthorized(err) {
			return nil, false, nil // not a recipient of this key → skip
		}
		return nil, false, errors.Wrapf(err, "KMS decrypt failed for %s (transient — retry)", r.raw)
	}
	return out.Plaintext, true, nil
}

// isKMSIntegrityError reports whether a KMS Decrypt error means the ciphertext or its
// binding is WRONG — tamper, never a least-privilege skip. InvalidCiphertext =
// mangled blob or EncryptionContext (our AAD) mismatch; IncorrectKey = the pinned
// KeyId is not the key that produced the blob (a transplant that the KeyId pin
// caught). AWS returns IncorrectKeyException — a DISTINCT type from
// InvalidCiphertextException — so it must be matched explicitly or a transplant would
// be misread as "not a recipient".
func isKMSIntegrityError(err error) bool {
	var ice *kmstypes.InvalidCiphertextException
	var ike *kmstypes.IncorrectKeyException
	if errors.As(err, &ice) || errors.As(err, &ike) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidCiphertextException", "IncorrectKeyException":
			return true
		}
	}
	return false
}

// isKMSNotAuthorized reports whether a KMS Decrypt error means the caller is simply
// not a recipient of this key (or the key is not usable for them) — a least-privilege
// skip, distinct from tamper (isKMSIntegrityError) and from a transient backend fault
// (everything not matched by either, which is surfaced as a hard "unavailable").
func isKMSNotAuthorized(err error) bool {
	var nfe *kmstypes.NotFoundException
	var dis *kmstypes.DisabledException
	var invState *kmstypes.KMSInvalidStateException
	if errors.As(err, &nfe) || errors.As(err, &dis) || errors.As(err, &invState) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDeniedException", "NotFoundException", "DisabledException", "KMSInvalidStateException":
			return true
		}
	}
	return false
}

// validateKMSRecipient checks an awskms:// recipient parses (used at governance
// time so a malformed KMS URL is rejected on `allow`, not on first `set`).
func validateKMSRecipient(recipient string) error {
	_, err := parseKMSRecipient(recipient)
	return err
}
