// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"context"
	"encoding/base64"
	"hash/crc32"
	"regexp"
	"strings"

	gcpkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/googleapis/gax-go/v2"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// gcpKMSRecipientScheme names a Google Cloud KMS symmetric key as a scope recipient:
//
//	gcpkms://projects/<project>/locations/<location>/keyRings/<ring>/cryptoKeys/<key>
//
// The data key is wrapped with cloudkms.cryptoKeyVersions.useToEncrypt and opened
// with useToDecrypt under Application Default Credentials, so a CI job that
// federates through Workload Identity holds no stored key at all. The value's
// (stack, scope, key) binding is the KMS additional authenticated data, which KMS
// enforces server-side exactly like the SSH path's byte AAD.
const gcpKMSRecipientScheme = "gcpkms://"

// A CryptoKey, never a CryptoKeyVersion: Encrypt picks the primary version and
// Decrypt finds the version from the ciphertext, so rotation needs no reseal.
// Project ids follow GCP's own rule (optionally domain-scoped, "example.com:proj").
var gcpKMSKeyNameRe = regexp.MustCompile(`^projects/(?:[a-z0-9-]+(?:\.[a-z0-9-]+)*:)?[a-z][a-z0-9-]{4,28}[a-z0-9]/locations/[a-z0-9-]+/keyRings/[A-Za-z0-9_-]{1,63}/cryptoKeys/[A-Za-z0-9_-]{1,63}$`)

var crc32c = crc32.MakeTable(crc32.Castagnoli)

// gcpKMSAPI is the subset of the Cloud KMS client scoped secrets need; tests
// replace it through newGCPKMSClient.
type gcpKMSAPI interface {
	Encrypt(ctx context.Context, req *kmspb.EncryptRequest, opts ...gax.CallOption) (*kmspb.EncryptResponse, error)
	Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error)
}

var newGCPKMSClient = func(ctx context.Context) (gcpKMSAPI, error) {
	c, err := gcpkms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Cloud KMS client (no Application Default Credentials?)")
	}
	return c, nil
}

func parseGCPKMSRecipient(recipient string) (kmsRecipient, error) {
	raw := strings.TrimSpace(recipient)
	name := strings.TrimPrefix(raw, gcpKMSRecipientScheme)
	if !gcpKMSKeyNameRe.MatchString(name) {
		return kmsRecipient{}, errors.Errorf("GCP KMS recipient %q must be gcpkms://projects/<p>/locations/<l>/keyRings/<r>/cryptoKeys/<k> (a key, not a key version)", recipient)
	}
	return kmsRecipient{provider: kmsProviderGCP, raw: gcpKMSRecipientScheme + name, keyID: name}, nil
}

func crc(b []byte) *wrapperspb.Int64Value {
	return wrapperspb.Int64(int64(crc32.Checksum(b, crc32c)))
}

func wrapDEKGCPKMS(r kmsRecipient, dek []byte, stack, scope, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kmsCallTimeout)
	defer cancel()
	cli, err := newGCPKMSClient(ctx)
	if err != nil {
		return nil, err
	}
	if c, ok := cli.(interface{ Close() error }); ok {
		defer func() { _ = c.Close() }()
	}
	aad := valueAAD(stack, scope, key)
	out, err := cli.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:                              r.keyID,
		Plaintext:                         dek,
		PlaintextCrc32C:                   crc(dek),
		AdditionalAuthenticatedData:       aad,
		AdditionalAuthenticatedDataCrc32C: crc(aad),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "Cloud KMS encrypt for recipient %s (need roles/cloudkms.cryptoKeyEncrypter on the key)", r.raw)
	}
	// End-to-end integrity as Cloud KMS documents it: the request reached KMS intact,
	// the response did too, and the named key (not some other one) did the wrap.
	if !out.GetVerifiedPlaintextCrc32C() || !out.GetVerifiedAdditionalAuthenticatedDataCrc32C() ||
		out.GetCiphertextCrc32C().GetValue() != crc(out.GetCiphertext()).GetValue() {
		return nil, errors.Errorf("Cloud KMS encrypt for %s failed its CRC32C integrity check; retry", r.raw)
	}
	if !strings.HasPrefix(out.GetName(), r.keyID+"/cryptoKeyVersions/") {
		return nil, errors.Errorf("Cloud KMS encrypted with %q, not the recipient key %s", out.GetName(), r.raw)
	}
	return []string{base64.StdEncoding.EncodeToString(out.GetCiphertext())}, nil
}

// decryptGCPKMSWrap has the same contract as decryptKMSWrap: owned=false with no
// error means "this caller cannot use that key" (least-privilege skip); owned=true
// with an error is an integrity failure; owned=false with an error is transient.
func decryptGCPKMSWrap(ctx context.Context, cli gcpKMSAPI, r kmsRecipient, blob []byte, stack, scope, key string) ([]byte, bool, error) {
	aad := valueAAD(stack, scope, key)
	out, err := cli.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:                              r.keyID, // pinned: a wrap made under another key is rejected
		Ciphertext:                        blob,
		CiphertextCrc32C:                  crc(blob),
		AdditionalAuthenticatedData:       aad,
		AdditionalAuthenticatedDataCrc32C: crc(aad),
	})
	if err != nil {
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil, true, errors.Wrapf(err, "Cloud KMS rejected the wrap for %s (tampered blob, wrong key, or wrong stack/scope/key binding)", r.raw)
		case codes.PermissionDenied, codes.NotFound, codes.FailedPrecondition, codes.Unauthenticated:
			return nil, false, nil
		default:
			return nil, false, errors.Wrapf(err, "Cloud KMS decrypt failed for %s (transient, retry)", r.raw)
		}
	}
	if out.GetPlaintextCrc32C().GetValue() != crc(out.GetPlaintext()).GetValue() {
		return nil, false, errors.Errorf("Cloud KMS decrypt for %s failed its CRC32C integrity check (transient, retry)", r.raw)
	}
	return out.GetPlaintext(), true, nil
}
