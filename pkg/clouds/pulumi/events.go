package pulumi

import (
	"context"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

func (p *pulumi) watchEvents(ctx context.Context) chan events.EngineEvent {
	eventChan := make(chan events.EngineEvent)
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			p.processEvent(ctx, <-eventChan)
		}
	}()
	return eventChan
}

func (p *pulumi) processEvent(ctx context.Context, evt events.EngineEvent) {
	switch {
	case evt.ResOutputsEvent != nil:
		p.procResourceOutputsEvent(ctx, evt)
	case evt.ResourcePreEvent != nil:
		p.procResourcePreEvent(ctx, evt)
	case evt.ResOpFailedEvent != nil:
		p.procResourceFailedEvent(ctx, evt)
	case evt.PolicyEvent != nil:
		p.procPolicyEvent(ctx, evt)
	case evt.DiagnosticEvent != nil:
		p.procDiagnosticEvent(ctx, evt)
	case evt.Error != nil:
		p.logger.Error(ctx, "[pulumi/error] %s", evt.Error)
	default:
		return // other events are not supported, uncomment lines below for debugging
	}
}

func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[pulumi/diagnostic] %s: %s", evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
}

func (p *pulumi) procPolicyEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[pulumi/policy] %s: %s", evt.PolicyEvent.ResourceURN, evt.PolicyEvent.Message)
}

func (p *pulumi) procResourceFailedEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[pulumi/failure] %s: %s/%d", evt.ResOpFailedEvent.Metadata.URN, evt.ResOpFailedEvent.Metadata.Op, evt.ResOpFailedEvent.Status)
}

func (p *pulumi) procResourcePreEvent(ctx context.Context, evt events.EngineEvent) {
	pre := evt.ResourcePreEvent
	p.logger.Info(ctx, strings.TrimSpace("[pulumi/pre] %s: %s \n %s"), pre.Metadata.URN, pre.Metadata.Op, p.diffSummary(pre.Metadata.DetailedDiff))
}

func (p *pulumi) procResourceOutputsEvent(ctx context.Context, evt events.EngineEvent) {
	outputs := evt.ResOutputsEvent
	p.logger.Info(ctx, strings.TrimSpace("[pulumi/out] %s: %s \n %s"), outputs.Metadata.URN, outputs.Metadata.Op, p.diffSummary(outputs.Metadata.DetailedDiff))
}

func (p *pulumi) diffSummary(diff map[string]apitype.PropertyDiff) string {
	res := strings.Builder{}
	for k, v := range diff {
		res.WriteString(fmt.Sprintf("\t %s : %s", k, v.Kind))
	}
	return res.String()
}
