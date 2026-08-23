// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// maxCosignAttempts bounds the retry loop for every cosign subcommand routed
// through runCosignWithRetry.
//
// Sized for the contended case. Both retryable classes below are contention
// artifacts of several jobs signing the same digest at once, and the public
// Rekor instance can stay hot for longer than the ~3s that three attempts
// spanned.
const maxCosignAttempts = 5

// maxConfirmProbeTimeout caps a single confirmation probe.
//
// The probe used to inherit the caller's full sign/attest timeout, which makes
// the worst case maxCosignAttempts x (timeout + timeout) plus backoff: ~20
// minutes for an attacher on a 2-minute budget, against 6 minutes before
// confirmation existed. A probe is one read-only registry round trip and a
// signature check; if it needs more than this, treating it as "not confirmed"
// and retrying the real operation is both faster and fail-closed.
const maxConfirmProbeTimeout = 30 * time.Second

// Backoff between retries. Both retryable classes are contention artifacts —
// parallel deploys attesting against the public-good instance — so retrying
// instantly maximizes the chance of colliding again. Jittered, to keep a
// fleet-wide deploy from resynchronizing on one schedule. Vars, not consts, so
// tests can zero the delay instead of sleeping.
var (
	cosignBaseBackoff = time.Second
	cosignMaxBackoff  = 8 * time.Second
)

// rekorConflictRe matches Rekor's createLogEntryConflict response as cosign
// renders it. Anchored on the swagger error shape rather than searching for
// "409" and the endpoint path independently: cosign prints the tlog entry URL on
// success, and OCI registries answer 409 on immutable-tag overwrite and on
// concurrent blob upload, so a loose match would classify a permanently fatal
// registry conflict as a retryable tlog conflict.
var rekorConflictRe = regexp.MustCompile(`\[POST /api/v1/log/entries\]\[409\]`)

// isRekorConflict reports a Rekor createLogEntryConflict (HTTP 409) — an
// identical entry already in the tlog. Two ways to get there: a cosign upload
// retry after a client-side timeout whose first attempt succeeded server-side,
// and a redeploy of an unchanged digest whose predicate is byte-identical to
// the one already attested.
//
// Callers classify each stream separately. Concatenating stdout and stderr first
// would let the marker be assembled from tokens cosign never emitted together.
func isRekorConflict(output string) bool {
	return strings.Contains(output, "createLogEntryConflict") || rekorConflictRe.MatchString(output)
}

// rekorGiveUpRe matches cosign exhausting its own HTTP retry budget against the
// Rekor log-entries endpoint, e.g.
//
//	signing bundle: Post "https://rekor.example/api/v1/log/entries": EOF, giving up after 2 attempt(s)
//
// The two halves must belong to the same message. An earlier version allowed up
// to 400 arbitrary characters between them with (?s), which only required them
// to appear near each other in the same stream: a log-entries POST that
// returned 201, followed by a registry, Fulcio or OIDC give-up, matched and was
// retried five times while the operator was pointed at the transparency-log
// runbook. Excluding quotes and newlines from the gap keeps the match inside
// one message, because every competing give-up names its own quoted URL first.
var rekorGiveUpRe = regexp.MustCompile(`Post "https?://[^"\n]+/api/v1/log/entries": [^"\n]{0,200}giving up after \d+ attempt`)

// isRekorTransient reports a transient failure to reach the transparency log:
// cosign's internal HTTP client ran out of attempts posting the entry. Nothing
// was attached, so unlike a conflict this is retried rather than confirmed —
// but it is retried, where before it fell through to the fail-fast branch and
// broke the deploy on the first hiccup. Concurrent deploys of the same digest
// are exactly when the endpoint is most likely to be slow.
func isRekorTransient(output string) bool {
	return rekorGiveUpRe.MatchString(output)
}

// ConfirmProbe is the read-only cosign invocation that decides whether a Rekor
// conflict is an idempotent no-op. Args is a cosign argv WITHOUT the image
// reference; the retry loop appends the same image reference the conflicting
// attempt used. What names the artifact in the log line.
//
// It must be a verify form, not a download form. `cosign download signature`
// and `cosign download attestation` answer "is anything of this shape stored
// under the legacy signature tag", which accepts a signature made under a
// rotated key or an attestation of the same predicate family produced by an
// unrelated workflow. They also return nothing at all under cosign v3, which
// defaults to the new bundle format and stores nothing in the legacy tag: on a
// current cosign the download probe never confirms, so every conflict runs the
// loop to exhaustion and the operation fails. `cosign verify` and `cosign
// verify-attestation` read both formats and check the identity, so a
// confirmation means the artifact the caller wanted is present AND ours.
//
// The probe's exit code is the whole signal. Its stdout is not inspected:
// cosign prints the verification banner to stderr, so requiring non-empty
// stdout is another way to never confirm.
type ConfirmProbe struct {
	Args []string
	What string
}

// probeEnvAllowlist is the set of environment variables a verify probe may
// inherit from the signing invocation. Everything else is dropped, most
// importantly SIGSTORE_ID_TOKEN: the probe mints no certificate, so handing the
// OIDC identity token to an extra process buys nothing. (The probe still
// inherits the ambient process environment, which this package does not own.)
var probeEnvAllowlist = []string{
	"COSIGN_PASSWORD",           // decrypts a private key handed to `verify --key`
	"COSIGN_REPOSITORY",         // signatures kept in a separate OCI repository
	"COSIGN_DOCKER_MEDIA_TYPES", // registry compatibility toggle
	"DOCKER_CONFIG",             // registry credentials
	"REGISTRY_AUTH_FILE",        // registry credentials
}

func probeEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if ok && slices.Contains(probeEnvAllowlist, name) {
			out = append(out, kv)
		}
	}
	return out
}

// RunCosignWithRetry runs `cosign <args>` with no conflict confirmation: a
// conflict is retried and then reported. See RunCosignWithRetryConfirm.
func RunCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration) (string, error) {
	stdout, _, err := runCosignWithRetry(ctx, label, args, env, timeout, nil, tools.ExecCommand)
	return stdout, err
}

// RunCosignWithRetryConfirm runs `cosign <args>` and returns its stdout,
// retrying Rekor entry conflicts. label names the operation in the retry
// warning ("sbom attest"); the returned error is prefixed from args[0], so it
// reads "cosign attest failed". The second return value reports whether the run
// ended by confirming an existing artifact rather than by producing a new one,
// in which case stdout is empty because no fresh cosign output exists.
//
// Every attempt is a fresh process with its own full timeout budget. Under
// keyless signing each invocation mints a new ephemeral certificate, so the body
// cosign replays differs and the conflict clears.
//
// Two failure classes are retried. A Rekor 409 conflict says the transparency
// log already holds an identical entry, and "already attested" is the desired
// end state of an idempotent re-run — a redeploy of an unchanged digest
// regenerates a byte-identical predicate and lands there every time. It is not
// proof on its own, because cosign uploads to Rekor before it pushes to the
// registry, so each conflict is checked with confirm: a confirmed conflict
// returns success, an unconfirmed one is retried and then reported. A transient
// give-up against the log-entries endpoint attached nothing, so it is simply
// retried. A nil confirm disables confirmation, which is the correct setting
// wherever the caller cannot name the identity to verify against.
func RunCosignWithRetryConfirm(ctx context.Context, label string, args, env []string, timeout time.Duration, confirm *ConfirmProbe) (string, error) {
	stdout, _, err := runCosignWithRetry(ctx, label, args, env, timeout, confirm, tools.ExecCommand)
	return stdout, err
}

func runCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration, confirm *ConfirmProbe, exec execFn) (string, bool, error) {
	subcommand := label
	if len(args) > 0 {
		subcommand = args[0]
	}

	var firstRetryableErr, lastErr error
	for attempt := 1; attempt <= maxCosignAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		if attempt > 1 {
			if err := sleepWithContext(ctx, cosignBackoff(attempt)); err != nil {
				return "", false, err
			}
		}

		stdout, stderr, err := exec(ctx, "cosign", args, env, timeout)
		if err == nil {
			return stdout, false, nil
		}
		lastErr = fmt.Errorf("cosign %s failed: %w\nStderr: %s\nStdout: %s", subcommand, err, stderr, stdout)

		conflict := isRekorConflict(stderr) || isRekorConflict(stdout)
		transient := isRekorTransient(stderr) || isRekorTransient(stdout)
		if !conflict && !transient {
			return "", false, lastErr
		}
		if firstRetryableErr == nil {
			firstRetryableErr = lastErr
		}

		// The artifact this invocation was asked to produce may already be on
		// the image, put there by the run that wrote the conflicting tlog
		// entry. That is the normal outcome of redeploying an unchanged digest
		// with a deterministic predicate. Re-signing it would be a no-op, so
		// stop here and report success rather than failing a deploy over an
		// artifact that is already in place and already verifies.
		if conflict && confirmArtifactAttached(ctx, confirm, args, env, timeout, exec) {
			fmt.Fprintf(os.Stderr,
				"Rekor transparency-log conflict on cosign %s: %s already verifies against the image, nothing to do\n",
				label, confirm.What)
			return "", true, nil
		}

		if attempt < maxCosignAttempts {
			reason := "transparency-log conflict"
			if !conflict {
				reason = "transparency-log upload gave up"
			}
			fmt.Fprintf(os.Stderr, "Warning: Rekor %s on cosign %s attempt %d/%d, retrying\n",
				reason, label, attempt, maxCosignAttempts)
		}
	}

	// Report the first retryable failure rather than the last error. A later
	// attempt can die for an incidental reason, and losing the
	// createLogEntryConflict text strands the operator: that string is what the
	// troubleshooting docs key on.
	if firstRetryableErr != nil {
		return "", false, fmt.Errorf("cosign %s failed after %d attempts: %w", subcommand, maxCosignAttempts, firstRetryableErr)
	}
	if lastErr != nil {
		return "", false, lastErr
	}
	return "", false, fmt.Errorf("cosign %s not attempted: retry bound %d is non-positive", subcommand, maxCosignAttempts)
}

// confirmArtifactAttached reports whether the image already carries the
// artifact the conflicting invocation was producing, signed by the identity the
// caller signs with.
//
// Deliberately fail-closed: a nil probe, an unusable argv, a probe that errors
// and a probe that times out all leave the caller on its existing
// retry-then-report path, so a broken probe can never convert a genuine signing
// failure into a green deploy.
func confirmArtifactAttached(ctx context.Context, confirm *ConfirmProbe, args, env []string, timeout time.Duration, exec execFn) bool {
	if confirm == nil || len(confirm.Args) == 0 || len(args) < 2 {
		return false
	}
	// Every call site in this package appends the image reference last.
	imageRef := args[len(args)-1]
	if imageRef == "" || strings.HasPrefix(imageRef, "-") {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	probeArgs := append(slices.Clone(confirm.Args), imageRef)
	_, _, err := exec(ctx, "cosign", probeArgs, probeEnv(env), confirmProbeTimeout(timeout))
	return err == nil
}

// confirmProbeTimeout bounds the probe independently of the signing budget.
func confirmProbeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return maxConfirmProbeTimeout
	}
	return min(timeout, maxConfirmProbeTimeout)
}

// cosignBackoff returns a jittered delay before the given attempt, capped at
// cosignMaxBackoff. Mirrors oidcBackoff in pkg/security/context.go.
func cosignBackoff(attempt int) time.Duration {
	if cosignBaseBackoff <= 0 {
		return 0
	}
	backoff := cosignBaseBackoff << (attempt - 2)
	if backoff <= 0 || backoff > cosignMaxBackoff {
		backoff = cosignMaxBackoff
	}
	return time.Duration(rand.Int63n(int64(backoff)))
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
