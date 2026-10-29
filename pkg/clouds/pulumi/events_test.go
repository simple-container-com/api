// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"

	"github.com/simple-container-com/api/pkg/api/logger"
)

type logLine struct {
	level string
	text  string
}

type recordingLogger struct {
	logger.Logger
	lines []logLine
}

func (l *recordingLogger) record(level, format string, a ...any) {
	l.lines = append(l.lines, logLine{level: level, text: fmt.Sprintf(format, a...)})
}

func (l *recordingLogger) Error(_ context.Context, format string, a ...any) {
	l.record("ERROR", format, a...)
}

func (l *recordingLogger) Warn(_ context.Context, format string, a ...any) {
	l.record("WARN", format, a...)
}

func (l *recordingLogger) Info(_ context.Context, format string, a ...any) {
	l.record("INFO", format, a...)
}

func (l *recordingLogger) Debug(_ context.Context, format string, a ...any) {
	l.record("DEBUG", format, a...)
}

func diagnosticEvent(urn, severity, message string) events.EngineEvent {
	return events.EngineEvent{EngineEvent: apitype.EngineEvent{
		DiagnosticEvent: &apitype.DiagnosticEvent{URN: urn, Severity: severity, Message: message},
	}}
}

func TestProcessEvent_WarningDiagnosticPrintedAtWarn(t *testing.T) {
	RegisterTestingT(t)

	log := &recordingLogger{}
	p := &pulumi{logger: log}
	ctx := WithContextAction(context.Background(), ActionContextDeploy)

	urn := "urn:pulumi:app--staging-ru::app::yandex:index/serverlessContainer:ServerlessContainer::app--staging-ru"
	msg := "Failed to deploy revision: rpc error: code = InvalidArgument desc = PORT is reserved\nsecond line of detail"
	p.processEvent(ctx, diagnosticEvent(urn, "warning", msg+"\n"))

	Expect(log.lines).To(HaveLen(1))
	Expect(log.lines[0].level).To(Equal("WARN"))
	Expect(log.lines[0].text).To(Equal("[deploy][pulumi/warning] " + urn + ": " + msg))
}

func TestProcessEvent_WarningDiagnosticPrintedInPreview(t *testing.T) {
	RegisterTestingT(t)

	log := &recordingLogger{}
	p := &pulumi{logger: log}
	ctx := WithContextAction(context.Background(), ActionContextPreview)

	p.processEvent(ctx, diagnosticEvent("urn:pulumi:s::p::t::n", "warning", "deprecated field"))

	Expect(log.lines).To(ConsistOf(logLine{level: "WARN", text: "[preview][pulumi/warning] urn:pulumi:s::p::t::n: deprecated field"}))
}

func TestProcessEvent_OtherDiagnosticsStayAtInfo(t *testing.T) {
	RegisterTestingT(t)

	log := &recordingLogger{}
	p := &pulumi{logger: log}
	ctx := WithContextAction(context.Background(), ActionContextDeploy)

	urn := "urn:pulumi:s::p::docker:index/image:Image::app"
	p.processEvent(ctx, diagnosticEvent(urn, "info", "#5 pushing layer"))
	p.processEvent(ctx, diagnosticEvent(urn, "debug", "trace"))

	Expect(log.lines).To(Equal([]logLine{
		{level: "INFO", text: "[deploy][pulumi/diagnostic] " + urn + ": #5 pushing layer"},
		{level: "INFO", text: "[deploy][pulumi/diagnostic] " + urn + ": trace"},
	}))
}
