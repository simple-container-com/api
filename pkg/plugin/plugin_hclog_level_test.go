// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"context"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// hashicorp/go-plugin logs the entire spawn handshake — "starting plugin",
// "waiting for RPC address", "using plugin: version=1", and the matching
// teardown lines — through the hclog.Logger it is handed. That is roughly a
// dozen lines per plugin, and `sc stack outputs` on a stack declaring five
// plugins printed sixty of them around a JSON document.
//
// Debug has to stay the DEFAULT, though: go-plugin relays a crashed plugin's
// stderr at Debug, so a quieter default turns a panic into a bare
// `exit status 2` (live-caught 2026-10-30, forge-baas). So the level follows
// SC's own, and only a command that explicitly asked for quiet gets it.
func TestDefaultPluginHclogFollowsSCLogLevel(t *testing.T) {
	RegisterTestingT(t)

	log := logger.New()

	for _, tc := range []struct {
		name string
		ctx  func() context.Context
		want hclog.Level
		why  string
	}{
		{
			name: "silent",
			ctx:  func() context.Context { return log.Silent(context.Background()) },
			want: hclog.Off,
			why:  "`sc stack outputs` silences SC so its stdout is a clean JSON document; handshake spam is not silent",
		},
		{
			name: "bare context keeps Debug",
			ctx:  func() context.Context { return context.Background() },
			want: hclog.Debug,
			why:  "the default must stay Debug or a plugin panic loses its stderr trace",
		},
		{
			name: "explicit info keeps Debug",
			ctx:  func() context.Context { return log.SetLogLevel(context.Background(), logger.LogLevelInfo) },
			want: hclog.Debug,
			why:  "provision/deploy are unchanged by this fix",
		},
		{
			name: "warn",
			ctx:  func() context.Context { return log.SetLogLevel(context.Background(), logger.LogLevelWarn) },
			want: hclog.Warn,
		},
		{
			name: "error",
			ctx:  func() context.Context { return log.SetLogLevel(context.Background(), logger.LogLevelError) },
			want: hclog.Error,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			got := defaultPluginHclog(tc.ctx(), "mongodb")
			Expect(got.GetLevel()).To(Equal(tc.want), tc.why)
		})
	}
}

// The level has to be readable from the context by a non-Logger consumer, or
// the mapping above has nothing to read.
func TestLevelFromContext(t *testing.T) {
	RegisterTestingT(t)

	log := logger.New()

	// A bare context must NOT read as silent — otherwise every caller that
	// forgot to thread the command context would accidentally go quiet.
	Expect(logger.LevelFromContext(context.Background())).To(Equal(logger.LogLevelInfo))
	Expect(logger.LevelFromContext(log.Silent(context.Background()))).To(Equal(logger.LogLevelSilent))
	Expect(logger.LevelFromContext(log.SetLogLevel(context.Background(), logger.LogLevelDebug))).
		To(Equal(logger.LogLevelDebug))
}
