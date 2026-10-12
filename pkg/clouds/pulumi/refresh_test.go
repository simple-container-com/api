// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

func TestRefreshIsPartOfPreviewAndUpdate(t *testing.T) {
	RegisterTestingT(t)
	var p optpreview.Options
	for _, o := range previewRefresh(false) {
		o.ApplyOption(&p)
	}
	Expect(p.Refresh).To(BeTrue())
	Expect(p.RunProgram).To(HaveValue(BeTrue()))

	var u optup.Options
	for _, o := range upRefresh(false) {
		o.ApplyOption(&u)
	}
	Expect(u.Refresh).To(BeTrue())
	Expect(u.RunProgram).To(HaveValue(BeTrue()))

	Expect(previewRefresh(true)).To(BeEmpty())
	Expect(upRefresh(true)).To(BeEmpty())
}
