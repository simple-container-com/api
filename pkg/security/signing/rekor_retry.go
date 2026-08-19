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

// maxCosignAttempts bounds the Rekor-conflict retry loop for every cosign
// subcommand routed through runCosignWithRetry.
const maxCosignAttempts = 3

// Backoff between conflict retries. The conflict is a contention artifact —
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
// identical entry already in the tlog, typically a cosign upload retry after a
// client-side timeout whose first attempt succeeded server-side.
//
// Callers classify each stream separately. Concatenating stdout and stderr first
// would let the marker be assembled from tokens cosign never emitted together.
func isRekorConflict(output string) bool {
	return strings.Contains(output, "createLogEntryConflict") || rekorConflictRe.MatchString(output)
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
// Retrying rather than treating a 409 as success is deliberate: cosign uploads
// to Rekor before it pushes to the registry, so a tlog entry does not prove the
// signature or attestation was attached. Exhausting the loop therefore means a
// persistent server-side condition, and surfacing it is correct.
func RunCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration) (string, error) {
	return runCosignWithRetry(ctx, label, args, env, timeout, tools.ExecCommand)
}

func runCosignWithRetry(ctx context.Context, label string, args, env []string, timeout time.Duration, exec execFn) (string, error) {
	subcommand := label
	if len(args) > 0 {
		subcommand = args[0]
	}

	var firstConflictErr, lastErr error
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

		if !isRekorConflict(stderr) && !isRekorConflict(stdout) {
			return "", lastErr
		}
		if firstConflictErr == nil {
			firstConflictErr = lastErr
		}
		if attempt < maxCosignAttempts {
			fmt.Fprintf(os.Stderr, "Warning: Rekor transparency-log conflict on cosign %s attempt %d/%d, retrying\n",
				label, attempt, maxCosignAttempts)
		}
	}

	// Report the first conflict rather than the last error. A later attempt can
	// die for an incidental reason, and losing the createLogEntryConflict text
	// strands the operator: that string is what the troubleshooting docs key on.
	if firstConflictErr != nil {
		return "", fmt.Errorf("cosign %s failed after %d attempts: %w", subcommand, maxCosignAttempts, firstConflictErr)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("cosign %s not attempted: retry bound %d is non-positive", subcommand, maxCosignAttempts)
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
