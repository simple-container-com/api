// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"regexp"
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
//	signing bundle: Post "https://rekor.sigstore.dev/api/v1/log/entries": EOF — giving up after 2 attempt(s)
//
// Both halves are required, and the URL must be a log-entries POST, so an
// unrelated "giving up" from a registry or from Fulcio does not match.
var rekorGiveUpRe = regexp.MustCompile(`(?s)Post "https?://[^"]+/api/v1/log/entries".{0,400}?giving up after \d+ attempt`)

// isRekorTransient reports a transient failure to reach the transparency log:
// cosign's internal HTTP client ran out of attempts posting the entry. Nothing
// was attached, so unlike a conflict this is retried rather than confirmed —
// but it is retried, where before it fell through to the fail-fast branch and
// broke the deploy on the first hiccup. Concurrent deploys of the same digest
// are exactly when the endpoint is most likely to be slow.
func isRekorTransient(output string) bool {
	return rekorGiveUpRe.MatchString(output)
}

// RunCosignWithRetry runs `cosign <args>` and returns its stdout, retrying Rekor
// entry conflicts. label names the operation in the retry warning ("sbom
// attest"); the returned error is prefixed from args[0], so it reads "cosign
// attest failed".
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
// registry, so each conflict is confirmed against the registry (see
// confirmArtifactAttached): a confirmed conflict returns success, an
// unconfirmed one is retried and then reported. A transient give-up against
// the log-entries endpoint attached nothing, so it is simply retried.
func RunCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration) (string, error) {
	return runCosignWithRetry(ctx, label, args, env, timeout, tools.ExecCommand)
}

func runCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration, exec execFn) (string, error) {
	subcommand := label
	if len(args) > 0 {
		subcommand = args[0]
	}

	var firstRetryableErr, lastErr error
	for attempt := 1; attempt <= maxCosignAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if attempt > 1 {
			if err := sleepWithContext(ctx, cosignBackoff(attempt)); err != nil {
				return "", err
			}
		}

		stdout, stderr, err := exec(ctx, "cosign", args, env, timeout)
		if err == nil {
			return stdout, nil
		}
		lastErr = fmt.Errorf("cosign %s failed: %w\nStderr: %s\nStdout: %s", subcommand, err, stderr, stdout)

		conflict := isRekorConflict(stderr) || isRekorConflict(stdout)
		transient := isRekorTransient(stderr) || isRekorTransient(stdout)
		if !conflict && !transient {
			return "", lastErr
		}
		if firstRetryableErr == nil {
			firstRetryableErr = lastErr
		}

		// The artifact this invocation was asked to produce may already be on
		// the image, put there by the run that wrote the conflicting tlog
		// entry. That is the normal outcome of redeploying an unchanged digest
		// with a deterministic predicate. Re-signing it would be a no-op, so
		// stop here and report success rather than failing a deploy over an
		// artifact that is already in place.
		if conflict {
			if what, attached := confirmArtifactAttached(ctx, args, env, timeout, exec); attached {
				fmt.Fprintf(os.Stderr,
					"Rekor transparency-log conflict on cosign %s: %s already attached to the image, nothing to do\n",
					label, what)
				return "", nil
			}
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
		return "", fmt.Errorf("cosign %s failed after %d attempts: %w", subcommand, maxCosignAttempts, firstRetryableErr)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("cosign %s not attempted: retry bound %d is non-positive", subcommand, maxCosignAttempts)
}

// artifactProbe is a read-only cosign invocation that answers "is the artifact
// this attempt was meant to produce already attached to the image?".
type artifactProbe struct {
	args []string
	what string
}

// probeFor derives the presence check for a cosign argv. Every call site in
// this package appends the image reference last, which is what the probe reads.
//
// `cosign download signature|attestation` reads the registry only: it neither
// contacts Rekor nor needs a verification key, so one probe covers the keyless
// and the key-based signer. Anything other than sign/attest gets no probe and
// keeps the plain retry-then-report behaviour.
func probeFor(args []string) (artifactProbe, bool) {
	if len(args) < 2 {
		return artifactProbe{}, false
	}
	imageRef := args[len(args)-1]
	if imageRef == "" || strings.HasPrefix(imageRef, "-") {
		return artifactProbe{}, false
	}

	switch args[0] {
	case "sign":
		return artifactProbe{args: []string{"download", "signature", imageRef}, what: "signature"}, true
	case "attest":
		probe := []string{"download", "attestation"}
		what := "attestation"
		// `cosign attest --type` and `cosign download attestation
		// --predicate-type` take the same vocabulary (shorthand names and full
		// predicate URIs alike), so the type carries over verbatim. Without it
		// the probe would answer "some attestation exists", and an SBOM
		// attestation would mask a missing provenance one.
		if t := flagValue(args, "--type"); t != "" {
			probe = append(probe, "--predicate-type", t)
			what = t + " attestation"
		}
		return artifactProbe{args: append(probe, imageRef), what: what}, true
	}
	return artifactProbe{}, false
}

// confirmArtifactAttached reports whether the registry already carries the
// artifact the conflicting invocation was producing.
//
// Deliberately fail-closed: only a clean exit AND non-empty output count as
// present. A probe that errors, times out, or prints nothing leaves the caller
// on its existing retry-then-report path, so a broken probe can never convert a
// genuine signing failure into a green deploy.
func confirmArtifactAttached(ctx context.Context, args, env []string, timeout time.Duration, exec execFn) (string, bool) {
	probe, ok := probeFor(args)
	if !ok {
		return "", false
	}
	if ctx.Err() != nil {
		return probe.what, false
	}
	stdout, _, err := exec(ctx, "cosign", probe.args, env, timeout)
	if err != nil || strings.TrimSpace(stdout) == "" {
		return probe.what, false
	}
	return probe.what, true
}

// flagValue returns the value of `--name value` or `--name=value` in args.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, found := strings.CutPrefix(a, name+"="); found {
			return v
		}
	}
	return ""
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
