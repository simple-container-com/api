// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type sessionLifetimeMocks struct{}

func (*sessionLifetimeMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name, args.Inputs, nil
}

func (*sessionLifetimeMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// attachedSession publishes a session with a monitor address already set, the
// way startPluginSession does after StartProxy, and registers its teardown.
// StartProxy itself is skipped — it needs a live engine monitor, and the
// address is not what these tests are about.
func attachedSession(ctx *sdk.Context, addr string) *pluginSession {
	sess := newPluginSession()
	sess.mu.Lock()
	sess.proxyAddr = addr
	sess.mu.Unlock()
	attachPluginSession(ctx, sess)
	registerSessionStop(func() {
		detachPluginSession(sess)
		sess.Close()
	})
	return sess
}

func resetSessionsForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		closePluginSessions()
		detachPluginSession(nil)
	})
}

// The defect this pins: the real work of deployStackProgram and
// provisionProgram happens inside an output continuation, which the engine runs
// AFTER the inline RunFunc has returned. A session torn down at RunFunc return
// is therefore already gone by the time the sidecar dispatches, and the deploy
// dies with "requires an active plugin session" under Pulumi's "waiting for
// RPCs". Live-caught 2026-10-27, api run 40587424994, stack dist--prod.
func TestPluginSession_SurvivesRunFuncReturn(t *testing.T) {
	RegisterTestingT(t)
	resetSessionsForTest(t)

	var (
		addrInApply string
		errInApply  error
		resolved    = make(chan struct{})
	)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		attachedSession(ctx, "127.0.0.1:65001")
		// Mirrors deploy.go: the dispatch rides an output continuation, and
		// the RunFunc returns without waiting for it.
		sdk.String("value").ToStringOutput().ApplyT(func(v string) string {
			addrInApply, errInApply = requireMonitorAddr("cloudflare")
			close(resolved)
			return v
		})
		return nil
	}, sdk.WithMocks("project", "stack", &sessionLifetimeMocks{}))
	Expect(err).NotTo(HaveOccurred())

	select {
	case <-resolved:
	case <-time.After(10 * time.Second):
		t.Fatal("the output continuation never ran")
	}

	Expect(errInApply).NotTo(HaveOccurred(),
		"a sidecar dispatched from an output continuation must still find the session")
	Expect(addrInApply).To(Equal("127.0.0.1:65001"))

	// And the session is still live for anything the engine has not drained yet.
	addr, err := requireMonitorAddr("cloudflare")
	Expect(err).NotTo(HaveOccurred())
	Expect(addr).To(Equal("127.0.0.1:65001"))
}

// The other half of the contract: the outer operation's closePluginSessions
// does tear the session down, so a dispatch after the engine is done is still
// the error it was.
func TestPluginSession_ClosedByOuterOperation(t *testing.T) {
	RegisterTestingT(t)
	resetSessionsForTest(t)

	err := sdk.RunErr(func(ctx *sdk.Context) error {
		attachedSession(ctx, "127.0.0.1:65002")
		return nil
	}, sdk.WithMocks("project", "stack", &sessionLifetimeMocks{}))
	Expect(err).NotTo(HaveOccurred())

	closePluginSessions()

	_, err = requireMonitorAddr("cloudflare")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("requires an active plugin session"))
}

// closePluginSessions must be callable when nothing was attached — every outer
// operation defers it unconditionally, including the ones that run a program
// with no plugins at all.
func TestPluginSession_CloseWithNothingAttachedIsSafe(t *testing.T) {
	RegisterTestingT(t)
	resetSessionsForTest(t)

	Expect(closePluginSessions).NotTo(Panic())
	_, err := requireMonitorAddr("cloudflare")
	Expect(err).To(HaveOccurred())
}

// A deploy runs the program twice (preview, then update), so a second session
// attaches while the first is still published. The newest must win, and a
// stale teardown must not unpublish it — otherwise the update's dispatches
// fail exactly like the bug above, one layer deeper.
func TestPluginSession_SecondRunReplacesFirst(t *testing.T) {
	RegisterTestingT(t)
	resetSessionsForTest(t)

	first := attachedSession(nil, "127.0.0.1:65003")
	second := attachedSession(nil, "127.0.0.1:65004")

	addr, err := requireMonitorAddr("cloudflare")
	Expect(err).NotTo(HaveOccurred())
	Expect(addr).To(Equal("127.0.0.1:65004"))

	// The first session's teardown must leave the second one published.
	detachPluginSession(first)
	addr, err = requireMonitorAddr("cloudflare")
	Expect(err).NotTo(HaveOccurred())
	Expect(addr).To(Equal("127.0.0.1:65004"))

	detachPluginSession(second)
	_, err = requireMonitorAddr("cloudflare")
	Expect(err).To(HaveOccurred())
}
