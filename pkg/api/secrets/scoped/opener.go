// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"strings"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/googleapis/gax-go/v2"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
)

// Opener holds the decryption material a caller has and knows how to open an
// envelope value with any of it: a set of candidate SSH private keys (indexed by
// fingerprint) and, optionally, permission to attempt KMS Decrypt via the ambient
// AWS credential chain. It is the single decrypt path for both the CLI (`get`,
// `doctor`) and deploy-time resolution, so SSH and KMS recipients behave
// identically everywhere. An Opener is single-use per resolve/CLI run and is NOT
// safe for concurrent use (it lazily caches KMS clients).
type Opener struct {
	sshByFP    map[string]any // fingerprint -> *rsa.PrivateKey | ed25519.PrivateKey
	kmsAllowed bool

	// kmsClients caches one KMS client per region so a deploy with many scoped
	// values does not rebuild the client (and re-probe credentials) per value.
	kmsClients map[string]kmsAPI
	// kmsNoIdentity is a one-shot breaker: once building a KMS client fails (no
	// ambient AWS credentials), further KMS slots are skipped without re-probing —
	// so an SSH-only / local deploy in a repo that happens to contain KMS scopes it
	// is not a recipient of does not pay a credential-probe stall per value.
	kmsNoIdentity bool

	// gcpClient / gcpNoIdentity are the Cloud KMS counterparts: one client (Cloud KMS
	// is not regional at the client level) and the same one-shot breaker.
	gcpClient     gcpKMSAPI
	gcpNoIdentity bool
}

// NewOpener parses each candidate PEM private key (skipping empty/unparseable
// ones, so a caller may pass everything it has) and, when kmsAllowed is set,
// permits KMS Decrypt attempts for values that carry a KMS wrap slot. KMS is only
// ever attempted for a value that actually has a KMS (awskms:// or gcpkms://) wrap AND that no held
// SSH key already opened — so an SSH-only store never triggers an AWS call.
func NewOpener(privateKeys []string, kmsAllowed bool) *Opener {
	o := &Opener{sshByFP: map[string]any{}, kmsAllowed: kmsAllowed, kmsClients: map[string]kmsAPI{}}
	for _, pk := range privateKeys {
		if strings.TrimSpace(pk) == "" {
			continue
		}
		fp, signer, err := privateKeyFingerprint(pk)
		if err != nil {
			continue
		}
		o.sshByFP[fp] = signer
	}
	return o
}

// Close releases the cached Cloud KMS client. It is safe on an Opener that never
// built one, and idempotent.
func (o *Opener) Close() error {
	c := o.gcpClient
	if c == nil {
		return nil
	}
	o.gcpClient = nil
	return c.Close()
}

// IsDeclaredSSHRecipient reports whether any held SSH key is listed in recipients.
// This is an OFFLINE proof of "I am a recipient of this file" for SSH keys — so if a
// value fails to open for a declared SSH recipient, that is tampering (a stripped
// wrap), never "not my scope". (KMS recipiency cannot be proven offline — it requires
// a Decrypt attempt — so KMS ownership is established by the first value that opens.)
func (o *Opener) IsDeclaredSSHRecipient(recipients []string) bool {
	for _, r := range recipients {
		if isKMSRecipient(r) {
			continue
		}
		fp, err := recipientID(r)
		if err != nil {
			continue
		}
		if _, ok := o.sshByFP[fp]; ok {
			return true
		}
	}
	return false
}

// kmsClientFor returns a cached KMS client for region, or (nil,false) if no ambient
// KMS identity is available. The "no identity" verdict is cached so credentials are
// probed at most once per Opener.
func (o *Opener) kmsClientFor(ctx context.Context, region string) (kmsAPI, bool, error) {
	if o.kmsNoIdentity {
		return nil, false, nil
	}
	if o.kmsClients == nil {
		o.kmsClients = map[string]kmsAPI{}
	}
	if c, ok := o.kmsClients[region]; ok {
		return c, true, nil
	}
	c, err := newKMSClient(ctx, region)
	if err != nil {
		if !isKMSNoIdentity(err) {
			return nil, false, err // a timed-out or throttled credential probe: retry, not skip
		}
		o.kmsNoIdentity = true
		return nil, false, nil
	}
	b := &kmsIdentityBreaker{kmsAPI: c, o: o}
	o.kmsClients[region] = b
	return b, true, nil
}

// gcpKMSClient returns the cached Cloud KMS client, or (nil,false) when no
// Application Default Credentials are available.
func (o *Opener) gcpKMSClient(ctx context.Context) (gcpKMSAPI, bool) {
	if o.gcpNoIdentity {
		return nil, false
	}
	if o.gcpClient != nil {
		return o.gcpClient, true
	}
	c, err := newGCPKMSClient(ctx)
	if err != nil {
		o.gcpNoIdentity = true
		return nil, false
	}
	o.gcpClient = &gcpIdentityBreaker{gcpKMSAPI: c, o: o}
	return o.gcpClient, true
}

// kmsIdentityBreaker trips the AWS no-identity breaker when a Decrypt fails to
// resolve credentials, so credentials that vanish after the probe cost one failure
// per resolve, not one per value.
type kmsIdentityBreaker struct {
	kmsAPI
	o *Opener
}

func (b *kmsIdentityBreaker) Decrypt(ctx context.Context, in *kms.DecryptInput, opts ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	out, err := b.kmsAPI.Decrypt(ctx, in, opts...)
	if err != nil && isKMSNoIdentity(err) {
		b.o.kmsNoIdentity = true
	}
	return out, err
}

// gcpIdentityBreaker trips the Cloud KMS no-identity breaker on Unauthenticated.
// The Cloud KMS client resolves Application Default Credentials lazily (a missing
// metadata server or key file only surfaces on the first RPC), so without this a
// deploy with no GCP identity would stall once per scoped value.
type gcpIdentityBreaker struct {
	gcpKMSAPI
	o *Opener
}

func (b *gcpIdentityBreaker) Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error) {
	out, err := b.gcpKMSAPI.Decrypt(ctx, req, opts...)
	if status.Code(err) == codes.Unauthenticated {
		b.o.gcpNoIdentity = true
	}
	return out, err
}

// OpenValue attempts to decrypt one envelope value bound to (stack, scope, key).
// It returns:
//
//   - (value, true, nil)  — decrypted with material the caller holds.
//   - ("", false, nil)    — the caller is not a usable recipient of this value
//     (no held SSH key owns a wrap slot, and no KMS slot was decryptable). This is
//     a least-privilege skip, NOT an error.
//   - ("", true, err)     — the caller IS a recipient (owns a slot) but decryption
//     failed: tampered ciphertext, a broken (stack,scope,key) binding, or a KMS
//     InvalidCiphertext. Callers treat this as a hard integrity error.
func (o *Opener) OpenValue(stack, scope, key string, ev EncryptedValue) (string, bool, error) {
	aad := valueAAD(stack, scope, key)
	// 1. SSH: a held private key whose fingerprint owns a wrap slot. Deterministic
	// and offline — an SSH-openable value never triggers a KMS call.
	for fp, signer := range o.sshByFP {
		wrapped, ok := ev.Wraps[fp]
		if !ok {
			continue
		}
		dek, err := unwrapDEKWithSSH(signer, aad, wrapped)
		if err != nil {
			return "", true, errors.Wrapf(err, "failed to unwrap data key for %q in scope %q", key, scope)
		}
		return openWithDEK(ev, dek, aad, scope, key)
	}
	// 2. KMS: a wrap slot decryptable with the ambient credentials. Only attempted
	// for values that actually carry a KMS slot. A malformed slot (bad URL / not
	// base64 / wrong shape) is skipped, not hard-failed — a least-privilege reader
	// must not abort on a foreign/corrupt file it is not a recipient of (lint catches
	// such corruption offline). A transient backend error is remembered and only
	// surfaced if NO slot opens, so another recipient slot still gets a chance.
	if o.kmsAllowed {
		var transient error
		for slotID, wrapped := range ev.Wraps {
			if !isKMSRecipientID(slotID) {
				continue
			}
			r, err := parseKMSRecipient(slotID)
			if err != nil || len(wrapped) != 1 {
				continue // malformed slot → skip
			}
			blob, err := base64.StdEncoding.DecodeString(wrapped[0])
			if err != nil {
				continue // not base64 → skip
			}
			ctx, cancel := context.WithTimeout(context.Background(), kmsCallTimeout)
			var dek []byte
			var owned bool
			if r.provider == kmsProviderGCP {
				cli, ok := o.gcpKMSClient(ctx)
				if !ok {
					cancel()
					continue // no Application Default Credentials → skip (breaker set)
				}
				dek, owned, err = decryptGCPKMSWrap(ctx, cli, r, blob, stack, scope, key)
			} else {
				cli, ok, cerr := o.kmsClientFor(ctx, r.region)
				if cerr != nil {
					cancel()
					transient = errors.Wrapf(cerr, "KMS client for %s (transient, retry)", r.raw)
					continue
				}
				if !ok {
					cancel()
					continue // no ambient KMS identity → skip (breaker set, no re-probe)
				}
				dek, owned, err = decryptKMSWrap(ctx, cli, r, blob, stack, scope, key)
			}
			cancel()
			if err != nil {
				if owned {
					return "", true, err // integrity (tamper) — definitive, stop
				}
				transient = err // remember; another slot may still open this value
				continue
			}
			if !owned {
				continue
			}
			return openWithDEK(ev, dek, aad, scope, key)
		}
		if transient != nil {
			return "", false, transient // no slot opened; surface the transient fault
		}
	}
	return "", false, nil
}

// openWithDEK decodes the shared value ciphertext and opens it under the recovered
// data key, verifying the (stack,scope,key) AAD. A failure here (owned==true) is an
// integrity error, never a "not a recipient".
func openWithDEK(ev EncryptedValue, dek, aad []byte, scope, key string) (string, bool, error) {
	blob, err := base64.StdEncoding.DecodeString(ev.Ciphertext)
	if err != nil {
		return "", true, errors.Wrapf(err, "failed to decode value ciphertext for %q in scope %q", key, scope)
	}
	plain, err := ciphers.OpenAEAD(dek, blob, aad)
	if err != nil {
		return "", true, errors.Wrapf(err, "failed to decrypt %q in scope %q", key, scope)
	}
	return string(plain), true, nil
}

// unwrapDEKWithSSH recovers the data key from a per-recipient SSH wrap, verifying
// the (stack,scope,key) binding carried in aad.
func unwrapDEKWithSSH(signer any, aad []byte, wrapped []string) ([]byte, error) {
	switch k := signer.(type) {
	case *rsa.PrivateKey:
		return ciphers.DecryptLargeStringWithAAD(k, wrapped, aad)
	case ed25519.PrivateKey:
		return ciphers.DecryptLargeStringWithEd25519AAD(k, wrapped, aad)
	case *ed25519.PrivateKey:
		return ciphers.DecryptLargeStringWithEd25519AAD(*k, wrapped, aad)
	default:
		return nil, errors.Errorf("unsupported private key type %T", signer)
	}
}
