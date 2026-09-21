// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration
// +build integration

package signing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// The confirmation probe is the one piece of the Rekor-conflict path that the
// fake cosign harness cannot honestly certify. The harness decides what the
// probe means; if the probe shape is one real cosign rejects, or one that finds
// nothing under current defaults, every unit test still passes and the feature
// silently never fires in production. That is exactly how a `cosign download`
// probe survived review: it works on cosign v2 and returns nothing on v3, where
// the new bundle format leaves the legacy signature tag empty.
//
// So this runs the real binary against a real registry: push an image, sign it
// for real, then drive the retry loop with a synthetic Rekor conflict and let
// the probe be genuine cosign. The synthetic conflict is deliberate: reaching a
// real 409 would mean uploading to a public transparency log, which a test must
// not do. Everything the probe touches is real.
//
// Requires docker and cosign; skips without either, and under -short.
func TestConfirmProbe_ConfirmsAgainstARealRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: starts a local container registry and shells out to cosign")
	}
	requireBinary(t, "docker")
	requireBinary(t, "cosign")
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registry := startLocalRegistry(t)
	signedRef := buildAndPush(t, registry, "confirm-signed")
	unsignedRef := buildAndPush(t, registry, "confirm-unsigned")

	keyDir := t.TempDir()
	privateKey, publicKey, err := GenerateKeyPair(ctx, keyDir, keyPassword)
	Expect(err).ToNot(HaveOccurred())

	// Sign for real, but keep it off the public transparency log.
	signArgs := []string{
		"sign", "--key", privateKey, "--yes",
		"--tlog-upload=false", "--allow-insecure-registry", signedRef,
	}
	_, stderr, err := tools.ExecCommand(ctx, "cosign", signArgs, cosignEnv(), 2*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "cosign sign failed: %s", stderr)

	// What a caller would build for a key-based signer, plus the two flags this
	// offline local-registry setup needs.
	probe := &ConfirmProbe{
		Args: []string{
			"verify", "--key", publicKey,
			"--insecure-ignore-tlog=true", "--allow-insecure-registry",
		},
		What: "signature",
	}

	// Documented, not asserted: which of these the installed cosign answers is
	// version-dependent, and that dependency is the whole point. On v3 the
	// legacy download path returns nothing for an image the verify path
	// confirms, so a download-shaped probe is a permanent false negative.
	logLegacyDownload(t, ctx, signedRef)

	t.Run("a conflict over an image that really is signed confirms", func(t *testing.T) {
		RegisterTestingT(t)
		noBackoff(t)

		attempts := 0
		_, confirmed, err := runCosignWithRetry(ctx, "sign", signArgs, cosignEnv(), 2*time.Minute, probe,
			conflictOnSign(&attempts))

		Expect(err).ToNot(HaveOccurred())
		Expect(confirmed).To(BeTrue(), "the signature is on the image and verifies, so the conflict is a no-op")
		Expect(attempts).To(Equal(1), "confirmation must short-circuit the retry loop")
	})

	// The half that makes the half above mean something. Same probe, same real
	// cosign, an image nobody signed: it must not confirm.
	t.Run("a conflict over an unsigned image does not confirm", func(t *testing.T) {
		RegisterTestingT(t)
		noBackoff(t)

		unsignedArgs := []string{
			"sign", "--key", privateKey, "--yes",
			"--tlog-upload=false", "--allow-insecure-registry", unsignedRef,
		}
		attempts := 0
		_, confirmed, err := runCosignWithRetry(ctx, "sign", unsignedArgs, cosignEnv(), 2*time.Minute, probe,
			conflictOnSign(&attempts))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("createLogEntryConflict"))
		Expect(confirmed).To(BeFalse())
		Expect(attempts).To(Equal(maxCosignAttempts), "an unconfirmed conflict is retried and then reported")
	})
}

const keyPassword = "confirm-probe-integration"

func cosignEnv() []string { return []string{"COSIGN_PASSWORD=" + keyPassword} }

// conflictOnSign answers every sign/attest with a Rekor 409 and lets everything
// else through to the real cosign binary, so the probe under test is genuine.
func conflictOnSign(attempts *int) execFn {
	return func(ctx context.Context, name string, args, env []string, timeout time.Duration) (string, string, error) {
		if len(args) > 0 && (args[0] == "sign" || args[0] == "attest") {
			*attempts++
			return "", conflictStderr, fmt.Errorf("exit status 1")
		}
		return tools.ExecCommand(ctx, name, args, env, timeout)
	}
}

func logLegacyDownload(t *testing.T, ctx context.Context, ref string) {
	t.Helper()
	stdout, stderr, err := tools.ExecCommand(ctx, "cosign",
		[]string{"download", "signature", "--allow-insecure-registry", ref}, cosignEnv(), 30*time.Second)
	version, _, _ := tools.ExecCommand(ctx, "cosign", []string{"version"}, nil, 30*time.Second)
	t.Logf("cosign version:\n%s", strings.TrimSpace(version))
	t.Logf("legacy `cosign download signature` on a signed image: err=%v stdout=%q stderr=%q",
		err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
}

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("skipping: %s is not installed", name)
	}
}

// startLocalRegistry runs registry:2 on a loopback port and returns host:port.
func startLocalRegistry(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1:0:5000", "registry:2").CombinedOutput()
	if err != nil {
		t.Skipf("skipping: cannot start registry:2 (%v): %s", err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", id).Run()
	})

	portOut, err := exec.Command("docker", "port", id, "5000/tcp").Output()
	if err != nil {
		t.Fatalf("reading the registry port: %v", err)
	}
	addr := strings.TrimSpace(strings.SplitN(string(portOut), "\n", 2)[0])
	// Normalise 0.0.0.0:PORT to a loopback address cosign will treat as local.
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		addr = "127.0.0.1" + addr[i:]
	}

	// The registry needs a moment before it answers /v2/.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := exec.Command("docker", "exec", id, "true").Run(); err == nil {
			if probeErr := exec.Command("curl", "-fsS", "-o", "/dev/null",
				"http://"+addr+"/v2/").Run(); probeErr == nil {
				return addr
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("registry at %s never answered /v2/", addr)
	return ""
}

// buildAndPush builds a minimal scratch image and pushes it, returning the
// digest reference. Signing by digest is what production does, and it is what
// makes the probe's target unambiguous.
func buildAndPush(t *testing.T, registry, name string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatalf("writing the image payload: %v", err)
	}
	dockerfile := "FROM scratch\nCOPY payload /payload\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatalf("writing the Dockerfile: %v", err)
	}

	tag := fmt.Sprintf("%s/%s:%d", registry, name, time.Now().UnixNano())
	if out, err := exec.Command("docker", "build", "-t", tag, dir).CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", tag).Run() })

	if out, err := exec.Command("docker", "push", tag).CombinedOutput(); err != nil {
		t.Fatalf("docker push: %v: %s", err, out)
	}

	out, err := exec.Command("docker", "inspect", "--format", "{{index .RepoDigests 0}}", tag).Output()
	if err != nil {
		t.Fatalf("reading the pushed digest: %v", err)
	}
	return strings.TrimSpace(string(out))
}
