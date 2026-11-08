// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// The bundle's logCtx is what every LogStream consumer logs against, and SC's
// log level lives in the context as a value. Building logCtx from
// context.Background() therefore reset the level to the Info default, so a
// plugin's LOG_SEVERITY_INFO events printed even under `sc stack outputs` —
// which silences its logger precisely because SC's logger writes to STDOUT and
// the command's stdout is a JSON document. forge infra's aws plugin logged
// `configuring s3-bucket backend` into the middle of the payload and broke
// `| jq`.
func TestPreEngineBundleInheritsLogLevelFromCaller(t *testing.T) {
	RegisterTestingT(t)

	log := logger.New()

	t.Run("silence reaches the LogStream consumers", func(t *testing.T) {
		RegisterTestingT(t)

		b := newPreEnginePluginBundle(log.Silent(context.Background()), log)
		defer b.Close()

		Expect(logger.LevelFromContext(b.logCtx)).To(Equal(logger.LogLevelSilent),
			"logCtx dropped the caller's level: plugin log events will print into the outputs JSON")
	})

	t.Run("an ordinary caller is unaffected", func(t *testing.T) {
		RegisterTestingT(t)

		b := newPreEnginePluginBundle(context.Background(), log)
		defer b.Close()

		// provision/deploy must keep showing plugin progress.
		Expect(logger.LevelFromContext(b.logCtx)).To(Equal(logger.LogLevelInfo))
	})

	t.Run("Close still cancels the consumers", func(t *testing.T) {
		RegisterTestingT(t)

		b := newPreEnginePluginBundle(context.Background(), log)
		Expect(b.logCtx.Err()).ToNot(HaveOccurred())

		b.Close()

		// Deriving logCtx from the caller must not break the teardown the
		// consumers rely on to end their stream RPC.
		Expect(b.logCtx.Err()).To(MatchError(context.Canceled))
	})

	t.Run("a cancelled caller cancels the bundle", func(t *testing.T) {
		RegisterTestingT(t)

		ctx, cancel := context.WithCancel(context.Background())
		b := newPreEnginePluginBundle(ctx, log)
		defer b.Close()

		cancel()

		// Inherited cancellation is the price of inheriting values, and it is
		// the behaviour we want: if the operation is abandoned, the log
		// consumers should stop too. Safe because every entry point defers
		// closePreEnginePlugins, so the caller's context always outlives the
		// bundle in the normal path.
		Expect(b.logCtx.Err()).To(MatchError(context.Canceled))
	})
}
