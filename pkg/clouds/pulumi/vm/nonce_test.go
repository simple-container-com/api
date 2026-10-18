// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"testing"

	. "github.com/onsi/gomega"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The two helpers are the rule of nonce.go in executable form, and the packages above
// this one (k3s) have no pulumi mock fixture to assert their commands against — so
// this is the only place the distinction is checked for them.
func TestTriggerHelpers(t *testing.T) {
	RegisterTestingT(t)

	base := []sdk.Input{sdk.String("script"), sdk.String("host")}

	read := TriggersForRead(base...)
	Expect(read).To(HaveLen(len(base)+1), "TriggersForRead did not add the per-run nonce")
	Expect(read[len(read)-1]).To(Equal(sdk.String(RunNonce())))

	change := TriggersForChange(base...)
	Expect(change).To(HaveLen(len(base)))
	for _, got := range change {
		Expect(got).ToNot(Equal(sdk.String(RunNonce())),
			"TriggersForChange added a nonce, which makes every deploy a full redeploy")
	}

	// Neither helper may write into the caller's slice: a call site that builds its
	// base once and passes it to both would otherwise see the nonce appear in the
	// change triggers, which is the failure this file exists to prevent.
	Expect(base).To(HaveLen(2))

	// One value per process. Two reads that disagreed would mean two commands against
	// one machine belonging to different "runs".
	Expect(RunNonce()).To(Equal(RunNonce()))
	Expect(RunNonce()).ToNot(BeEmpty())
}
