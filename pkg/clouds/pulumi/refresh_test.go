// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"

	"github.com/simple-container-com/api/pkg/api/logger"
)

func TestOnlyTheUpdateRefreshes(t *testing.T) {
	RegisterTestingT(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &pulumi{logger: logger.New()}

	for _, diff := range []bool{false, true} {
		var po optpreview.Options
		for _, o := range p.previewOptions(ctx, ActionContextPreview, diff) {
			o.ApplyOption(&po)
		}
		Expect(po.Refresh).To(BeFalse())
		Expect(po.RunProgram).To(BeNil())
		Expect(po.Diff).To(Equal(diff))
		Expect(po.EventStreams).To(HaveLen(1))

		for _, skip := range []bool{false, true} {
			var uo optup.Options
			for _, o := range p.upOptions(ctx, ActionContextDeploy, skip, diff) {
				o.ApplyOption(&uo)
			}
			Expect(uo.Refresh).To(Equal(!skip))
			if skip {
				Expect(uo.RunProgram).To(BeNil())
			} else {
				Expect(uo.RunProgram).To(HaveValue(BeTrue()))
			}
			Expect(uo.Diff).To(Equal(diff))
			Expect(uo.EventStreams).To(HaveLen(1))
		}
	}
}

func TestRefreshNeedsARecentCLI(t *testing.T) {
	RegisterTestingT(t)
	Expect(checkRefreshCLI("3.231.0", false)).To(Succeed())
	Expect(checkRefreshCLI("v3.245.0", false)).To(Succeed())
	Expect(checkRefreshCLI("3.230.1", false)).To(MatchError(ContainSubstring("too old")))
	Expect(checkRefreshCLI("3.168.0", false)).To(MatchError(ContainSubstring("too old")))
	Expect(checkRefreshCLI("not-a-version", false)).To(MatchError(ContainSubstring("failed to parse")))
	Expect(checkRefreshCLI("3.168.0", true)).To(Succeed())
}
