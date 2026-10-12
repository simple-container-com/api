// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
)

func TestRefreshRunsTheProgram(t *testing.T) {
	RegisterTestingT(t)
	var o optrefresh.Options
	for _, opt := range refreshOptions(optrefresh.Message("kept")) {
		opt.ApplyOption(&o)
	}
	Expect(o.RunProgram).NotTo(BeNil())
	Expect(*o.RunProgram).To(BeTrue())
	Expect(o.Message).To(Equal("kept"))
}
