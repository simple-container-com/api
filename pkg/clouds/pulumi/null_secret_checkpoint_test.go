// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The exact live-caught error from the Cloud.ru rucreate stack
// (2026-10-24, apt-lock failure left every secret decrypted to JSON
// null, then every subsequent op tripped this on state decode).
// Keeping the verbatim string pins what the guard has to match.
const liveCaughtNullSecretError = `error: could not load resource state: ` +
	`malformed RPC secret: missing value for "stdin": ` +
	`secret marshalled with SkipNulls / dropped unknown value`

func TestAnnotateNullSecretCheckpoint_WrapsOnlyOnSentinelMatch(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantWraps  bool
		wantInside []string
	}{
		{
			name:      "nil → nil",
			err:       nil,
			wantWraps: false,
		},
		{
			name:      "unrelated error → untouched",
			err:       errors.New("some other pulumi failure: timeout waiting for provider"),
			wantWraps: false,
		},
		{
			name:      "the live-caught shape → wrapped with recovery hint",
			err:       errors.New(liveCaughtNullSecretError),
			wantWraps: true,
			wantInside: []string{
				"checkpoint secrets nulled by prior failed update",
				".bak",
				"-S", // the skip-preview flag the recovery names
				"sc provision -s",
			},
		},
		{
			name:      "sentinel without the exact Cloud.ru suffix → still wrapped",
			err:       errors.New(`malformed RPC secret: missing value for "tls_private_key"`),
			wantWraps: true,
			wantInside: []string{
				"checkpoint secrets nulled by prior failed update",
				".bak",
			},
		},
		{
			name: "sentinel buried inside a wrapped error → still wrapped",
			// errors.Wrap preserves the original string in .Error(), so
			// a nested RPC error survives the match.
			err:        errors.New(`stackSource.Up: malformed RPC secret: missing value for "stdin"`),
			wantWraps:  true,
			wantInside: []string{"checkpoint secrets nulled"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := annotateNullSecretCheckpointError(tt.err)
			if tt.err == nil {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			if tt.wantWraps {
				require.NotEqual(t, tt.err.Error(), got.Error(),
					"expected wrap on match; got unchanged error:\n%s", got)
				for _, want := range tt.wantInside {
					require.True(t, strings.Contains(got.Error(), want),
						"wrapped error missing %q; full error:\n%s", want, got)
				}
				// The ORIGINAL error must remain inside the wrap chain so
				// downstream callers that unwrap or type-assert still find it.
				require.True(t, strings.Contains(got.Error(), tt.err.Error()),
					"original error text missing from wrapped output")
			} else {
				require.Equal(t, tt.err.Error(), got.Error(),
					"expected untouched error, got a wrap")
			}
		})
	}
}

// The recovery hint has to name every actionable piece an operator
// needs to recover WITHOUT reading docs: the .bak restore, -S,
// `sc provision`, and the failure class ("secrets nulled by prior
// failed update"). Pin each marker so a well-meaning copy-edit of
// the hint can't silently drop the recovery step.
func TestNullSecretCheckpointRecoveryHint_ContainsEveryStep(t *testing.T) {
	for _, marker := range []string{
		"checkpoint secrets nulled by prior failed update",
		".bak",
		"-S",
		"sc provision -s",
		"skip preview",
		"local.Command",
		"remote.Command",
	} {
		require.True(t, strings.Contains(nullSecretCheckpointRecoveryHint, marker),
			"recovery hint missing marker %q — operator cannot recover from the message alone", marker)
	}
}
