package helpers

import (
	"context"
	"testing"

	"github.com/simple-container-com/api/pkg/api/logger"
)

func Test_lambdaCloudHelper_handler(t *testing.T) {
	tests := []struct {
		name    string
		event   any
		wantErr bool
	}{
		{
			name: "happy path",
			event: map[string]any{
				"accountId": "471112843480",
				"alarmArn":  "arn:aws:cloudwatch:eu-central-1:471112843480:alarm:seeact-max-cpu-metric-alarm-a275ddf",
				"state": map[string]any{
					"reason": "Threshold Crossed: 1 datapoint [6.638074000676473 (14/05/24 09:53:00)] was not greater than the threshold (10.0).",
					"value":  "ALARM",
				},
				"alarmData": map[string]any{
					"alarmName": "seeact-max-cpu-metric-alarm-a275ddf",
					"configuration": map[string]any{
						"description": "SeeAct CPU usage exceeds 10%",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			l := &lambdaCloudHelper{
				log: logger.New(),
			}
			if err := l.handler(ctx, tt.event); (err != nil) != tt.wantErr {
				t.Errorf("handler() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
