// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build integration
// +build integration

package docker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// The reuse path only ever runs against a real daemon and a real registry:
// inspectViaDaemon, the not-found classification the daemon actually produces,
// and the cosign verification of an adopted image have no unit coverage by
// construction. These exercise them against a registry container on loopback,
// which go-containerregistry and the daemon both treat as plain HTTP.
//
// Run with: go test -tags integration ./pkg/clouds/pulumi/docker/...

const integrationTestVersion = "2026.09.17-0123abc"

// registryImage is pulled from the public ECR mirror rather than Docker Hub so
// an unauthenticated CI runner is not subject to Hub pull limits.
func registryImage() string {
	if img := os.Getenv("SC_TEST_REGISTRY_IMAGE"); img != "" {
		return img
	}
	return "public.ecr.aws/docker/library/registry:3"
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not installed")
	}
	if out, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		t.Skipf("docker daemon unreachable: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}

func dockerCmd(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startRegistry runs a registry on loopback and returns its host:port and a
// stop function. Stopping it mid-test is how the outage case is produced.
func startRegistry(t *testing.T) (string, func()) {
	t.Helper()
	port := freePort(t)
	name := fmt.Sprintf("sc-reuse-it-%d", port)
	dockerCmd(t, "run", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:5000", port), registryImage())

	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = exec.Command("docker", "rm", "-f", name).Run()
	}
	t.Cleanup(stop)

	host := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + host + "/v2/")
		if err == nil {
			resp.Body.Close()
			return host, stop
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("registry at %s never came up", host)
	return "", stop
}

// pushScratchImage builds a distinct one-file image and pushes it, returning
// the tag reference and the digest reference the registry assigned.
func pushScratchImage(t *testing.T, ref, payload string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(payload), 0o644); err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM scratch\nCOPY payload /payload\n"), 0o644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}
	dockerCmd(t, "build", "-t", ref, dir)
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", ref).Run() })
	dockerCmd(t, "push", ref)
	digestRef := dockerCmd(t, "inspect", "--format", "{{index .RepoDigests 0}}", ref)
	return ref, digestRef
}

// generateKeyPair shells cosign directly instead of using signing.GenerateKeyPair:
// that helper builds its return paths from outputDir but never tells cosign
// where to write, so the key pair lands in the process working directory and
// the paths it hands back do not exist. The gate under test does not use it.
func generateKeyPair(t *testing.T, dir, password string) (string, string) {
	t.Helper()
	prefix := filepath.Join(dir, "cosign")
	cmd := exec.Command("cosign", "generate-key-pair", "--output-key-prefix", prefix)
	cmd.Env = append(os.Environ(), "COSIGN_PASSWORD="+password)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cosign generate-key-pair: %v\n%s", err, out)
	}
	return prefix + ".key", prefix + ".pub"
}

// What the daemon actually returns for a tag that resolves, and that the
// digest it reports is the one the registry stored. A mocked inspector can
// only ever confirm the wiring around this call, never the call.
func TestInspectViaDaemonResolvesAPushedTag(t *testing.T) {
	requireDocker(t)
	RegisterTestingT(t)

	host, _ := startRegistry(t)
	ref := fmt.Sprintf("%s/reuse/app:%s", host, integrationTestVersion)
	_, digestRef := pushScratchImage(t, ref, "resolves-a-pushed-tag")

	got, err := resolveReusableDigest(context.Background(), inspectViaDaemon, ref, "user", "pass")
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(Equal(digestRef))
}

// A tag that was never pushed is the one case that legitimately means "build".
// The daemon reports it as a containerd not-found errdef rather than through
// the NotFound() marker interface, which is the whole reason isNotFound leads
// with cerrdefs.IsNotFound.
func TestInspectViaDaemonClassifiesAMissingTagAsBuild(t *testing.T) {
	requireDocker(t)
	RegisterTestingT(t)

	host, _ := startRegistry(t)
	ref := fmt.Sprintf("%s/reuse/never-pushed:%s", host, integrationTestVersion)

	_, err := inspectViaDaemon(context.Background(), ref, "")
	Expect(err).To(HaveOccurred())
	Expect(isNotFound(err)).To(BeTrue(), "the daemon's absent-tag error must classify as not-found, got %v", err)

	got, err := resolveReusableDigest(context.Background(), inspectViaDaemon, ref, "user", "pass")
	Expect(err).NotTo(HaveOccurred())
	Expect(got).To(BeEmpty())
}

// The failure mode that makes the whole check worthless: an unreachable
// registry read as "tag absent" would push over the tag precisely when nobody
// could see what was under it.
func TestUnreachableRegistryIsAnErrorNotAnAbsentTag(t *testing.T) {
	requireDocker(t)
	RegisterTestingT(t)

	host, stop := startRegistry(t)
	ref := fmt.Sprintf("%s/reuse/app:%s", host, integrationTestVersion)
	pushScratchImage(t, ref, "unreachable-registry")
	stop()

	_, err := inspectViaDaemon(context.Background(), ref, "")
	Expect(err).To(HaveOccurred())
	Expect(isNotFound(err)).To(BeFalse(), "a dead registry must not classify as an absent tag, got %v", err)

	got, err := resolveReusableDigest(context.Background(), inspectViaDaemon, ref, "user", "pass")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("image reuse check failed"))
	Expect(got).To(BeEmpty())
}

// verifyAdoptedImage is the gate that stops push access to the registry from
// becoming a signature from this pipeline. Both arms run against real cosign.
func TestVerifyAdoptedImageAcceptsSignedAndRefusesUnsigned(t *testing.T) {
	requireDocker(t)
	if _, err := exec.LookPath("cosign"); err != nil {
		t.Skip("cosign not installed")
	}
	RegisterTestingT(t)

	ctx := context.Background()
	host, _ := startRegistry(t)

	keyDir := t.TempDir()
	const password = "integration-test"
	privateKey, publicKey := generateKeyPair(t, keyDir, password)

	signedTag := fmt.Sprintf("%s/reuse/signed:%s", host, integrationTestVersion)
	_, signedDigest := pushScratchImage(t, signedTag, "signed")
	unsignedTag := fmt.Sprintf("%s/reuse/unsigned:%s", host, integrationTestVersion)
	_, unsignedDigest := pushScratchImage(t, unsignedTag, "unsigned")

	_, err := signing.SignImage(ctx, &signing.Config{
		Enabled:    true,
		Keyless:    false,
		PrivateKey: privateKey,
		Password:   password,
	}, signedDigest, "")
	Expect(err).NotTo(HaveOccurred())

	security := &api.SecurityDescriptor{
		Enabled: true,
		Signing: &api.SigningDescriptor{
			Enabled:   true,
			PublicKey: publicKey,
			Verify:    &api.VerifyDescriptor{Enabled: true},
		},
	}

	started := time.Now()
	Expect(verifyAdoptedImage(ctx, security, signedDigest)).To(Succeed())
	t.Logf("key-based verification of an adopted image took %s", time.Since(started))

	Expect(verifyAdoptedImage(ctx, security, unsignedDigest)).To(HaveOccurred())
}

func integrationImage(host, repo string) Image {
	return Image{
		Name:    repo,
		Version: integrationTestVersion,
		Registry: docker.RegistryArgs{
			Server:   sdk.String(host),
			Username: sdk.StringPtr("user"),
			Password: sdk.StringPtr("pass"),
		},
	}
}

// The whole decision, end to end: a tag that already resolves is adopted and
// the push is skipped, and an unsigned image under the tag is refused so the
// deploy falls back to building and pushing over it.
func TestResolveReuseOutputAgainstARealRegistry(t *testing.T) {
	requireDocker(t)
	RegisterTestingT(t)

	host, _ := startRegistry(t)

	reusedTag := fmt.Sprintf("%s/reuse/adopted:%s", host, integrationTestVersion)
	_, reusedDigest := pushScratchImage(t, reusedTag, "adopted")
	missingRef := fmt.Sprintf("%s/reuse/absent:%s", host, integrationTestVersion)

	t.Run("an existing tag is adopted and the push is skipped", func(t *testing.T) {
		RegisterTestingT(t)
		stack := reuseTestStack(true, nil)
		image := integrationImage(host, "reuse/adopted")

		got := resolveUnderMocks[string](t, "adopted", false, func(ctx *sdk.Context) sdk.Output {
			return resolveReuseOutput(ctx, stack, image, sdk.String(reusedTag).ToStringOutput())
		})
		Expect(got).To(Equal(reusedDigest))

		skip := resolveUnderMocks[*bool](t, "adopted skipPush", false, func(ctx *sdk.Context) sdk.Output {
			return skipPushOutput(ctx, resolveReuseOutput(ctx, stack, image, sdk.String(reusedTag).ToStringOutput()))
		})
		Expect(skip).NotTo(BeNil())
		Expect(*skip).To(BeTrue())
	})

	t.Run("an absent tag builds and pushes", func(t *testing.T) {
		RegisterTestingT(t)
		stack := reuseTestStack(true, nil)
		image := integrationImage(host, "reuse/absent")

		skip := resolveUnderMocks[*bool](t, "absent skipPush", false, func(ctx *sdk.Context) sdk.Output {
			return skipPushOutput(ctx, resolveReuseOutput(ctx, stack, image, sdk.String(missingRef).ToStringOutput()))
		})
		Expect(skip).NotTo(BeNil())
		Expect(*skip).To(BeFalse())
	})

	t.Run("an image that fails verification is not adopted", func(t *testing.T) {
		if _, err := exec.LookPath("cosign"); err != nil {
			t.Skip("cosign not installed")
		}
		RegisterTestingT(t)

		keyDir := t.TempDir()
		_, publicKey := generateKeyPair(t, keyDir, "integration-test")

		stack := reuseTestStack(true, &api.SecurityDescriptor{
			Enabled: true,
			Signing: &api.SigningDescriptor{
				Enabled:   true,
				PublicKey: publicKey,
				Verify:    &api.VerifyDescriptor{Enabled: true},
			},
		})
		image := integrationImage(host, "reuse/adopted")

		got := resolveUnderMocks[string](t, "unverifiable", false, func(ctx *sdk.Context) sdk.Output {
			return resolveReuseOutput(ctx, stack, image, sdk.String(reusedTag).ToStringOutput())
		})
		Expect(got).To(BeEmpty())
	})
}
