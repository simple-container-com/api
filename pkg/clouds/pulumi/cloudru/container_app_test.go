// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cloudru

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseResult(t *testing.T) {
	RegisterTestingT(t)

	res, err := parseResult(`{"id":"app-1","name":"atrium--staging","url":"https://atrium--staging.containerapps.ru","revision":"atrium--staging-00002"}`)
	Expect(err).To(BeNil())
	Expect(res.ID).To(Equal("app-1"))
	Expect(res.URL).To(Equal("https://atrium--staging.containerapps.ru"))
	Expect(res.Revision).To(Equal("atrium--staging-00002"))
}

// stdout is contractually one JSON document, but SC's own logger writes to the same
// stream and an operator debugging by hand will have progress lines in front of
// them. Hence scanning from the end rather than decoding the whole stream.
func TestParseResultIgnoresSurroundingOutput(t *testing.T) {
	RegisterTestingT(t)

	res, err := parseResult(strings.Join([]string{
		"[info] patching cloud.ru container app atrium--staging...",
		"[info] waiting for a healthy revision...",
		`{"name":"atrium--staging","url":"https://x.containerapps.ru"}`,
		"",
	}, "\n"))
	Expect(err).To(BeNil())
	Expect(res.Name).To(Equal("atrium--staging"))
}

// A JSON object that is not a Result must not be mistaken for one — a log line
// emitted as structured JSON is the realistic case.
func TestParseResultSkipsNonResultObjects(t *testing.T) {
	RegisterTestingT(t)

	res, err := parseResult(strings.Join([]string{
		`{"name":"atrium--staging","url":"https://x.containerapps.ru"}`,
		`{"level":"info","msg":"done"}`,
	}, "\n"))
	Expect(err).To(BeNil())
	Expect(res.Name).To(Equal("atrium--staging"))
}

func TestParseResultFailsWithoutADocument(t *testing.T) {
	RegisterTestingT(t)

	_, err := parseResult("waiting...\nstill waiting...")
	Expect(err).To(HaveOccurred())
	// The error must not echo stdout: LoggingNone keeps the command's output out of
	// the log on purpose.
	Expect(err.Error()).NotTo(ContainSubstring("still waiting"))
}

// scScript is what a runner executes. The two things that matter are that nothing
// is interpolated into it (the request rides stdin) and that the PATH prefix is
// there, since the SC runner image installs `sc` outside the default PATH.
func TestScScript(t *testing.T) {
	RegisterTestingT(t)

	script := scScript("cloudru", "container-app", "apply")
	Expect(script).To(HavePrefix("set -eu\n"))
	Expect(script).To(ContainSubstring(`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`))
	Expect(script).To(HaveSuffix("sc cloudru container-app apply\n"))
	// No shell metacharacters, because no value is ever substituted in.
	Expect(script).NotTo(ContainSubstring("$("))
	Expect(script).NotTo(ContainSubstring("`"))
}
