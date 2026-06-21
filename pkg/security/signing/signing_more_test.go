// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// fakeSignExec builds an execFn returning the given stdout/stderr/err.
func fakeSignExec(stdout, stderr string, err error) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		return stdout, stderr, err
	}
}

func TestSignImage_And_VerifyImage_ErrorPaths(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	_, err := SignImage(ctx, &Config{Enabled: false}, "img", "")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("signing is not enabled"))

	_, err = SignImage(ctx, &Config{Enabled: true, Keyless: true}, "img", "")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating signer"))

	_, err = VerifyImage(ctx, &Config{}, "img")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating verifier"))
}

func TestKeylessSigner_Sign(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("empty token errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := (&KeylessSigner{}).Sign(ctx, "img")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("OIDC token is required"))
	})

	t.Run("success parses rekor entry", func(t *testing.T) {
		RegisterTestingT(t)
		s := NewKeylessSigner("tok", time.Minute)
		s.exec = fakeSignExec("tlog entry created with index: 42", "", nil)
		res, err := s.Sign(ctx, "img")
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RekorEntry).To(ContainSubstring("logIndex=42"))
		Expect(res.SignedAt).ToNot(BeEmpty())
	})

	t.Run("non-conflict error fails fast", func(t *testing.T) {
		RegisterTestingT(t)
		s := &KeylessSigner{OIDCToken: "tok", exec: fakeSignExec("", "boom", fmt.Errorf("exit 1"))}
		_, err := s.Sign(ctx, "img")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cosign sign failed"))
	})
}

func TestKeyBasedSigner_Sign(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("empty key errors", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := (&KeyBasedSigner{}).Sign(ctx, "img")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("private key is required"))
	})

	t.Run("raw key content writes temp file then signs", func(t *testing.T) {
		RegisterTestingT(t)
		s := NewKeyBasedSigner("-----RAW KEY CONTENT-----", "pw", time.Minute)
		s.exec = fakeSignExec("ok", "", nil)
		res, err := s.Sign(ctx, "img")
		Expect(err).ToNot(HaveOccurred())
		Expect(res.SignedAt).ToNot(BeEmpty())
	})

	t.Run("existing key file path", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		keyFile := filepath.Join(dir, "cosign.key")
		Expect(os.WriteFile(keyFile, []byte("KEY"), 0o600)).To(Succeed())
		s := &KeyBasedSigner{PrivateKey: keyFile, Timeout: time.Minute, exec: fakeSignExec("ok", "", nil)}
		_, err := s.Sign(ctx, "img")
		Expect(err).ToNot(HaveOccurred())
	})
}

func TestRunCosignSign_RetryOnRekorConflict(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	t.Run("retries on conflict then succeeds", func(t *testing.T) {
		RegisterTestingT(t)
		calls := 0
		exec := func(_ context.Context, _ string, _, _ []string, _ time.Duration) (string, string, error) {
			calls++
			if calls < 2 {
				return "", "createLogEntryConflict", fmt.Errorf("409")
			}
			return "done", "", nil
		}
		out, err := runCosignSign(ctx, exec, []string{"sign"}, nil, time.Minute)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(Equal("done"))
		Expect(calls).To(Equal(2))
	})

	t.Run("exhausts attempts on persistent conflict", func(t *testing.T) {
		RegisterTestingT(t)
		exec := fakeSignExec("", "createLogEntryConflict", fmt.Errorf("409"))
		_, err := runCosignSign(ctx, exec, []string{"sign"}, nil, time.Minute)
		Expect(err).To(HaveOccurred())
	})
}

func TestSigner_Constructors_And_Verify_ConfigError(t *testing.T) {
	RegisterTestingT(t)

	Expect(NewKeylessVerifier("i", "r", 0).Timeout).To(Equal(2 * time.Minute))
	Expect(NewKeyBasedVerifier("pub", 0).Timeout).To(Equal(2 * time.Minute))
	Expect(NewKeyBasedSigner("k", "", 0).Timeout).To(Equal(5 * time.Minute))
	Expect(NewKeylessSigner("t", 0).Timeout).To(Equal(5 * time.Minute))

	// No public key and no OIDC config -> Verify returns a config error before exec.
	_, err := (&Verifier{}).Verify(context.Background(), "img")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("requires either public key or OIDC"))
}
