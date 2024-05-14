package helpers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/pkg/util"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

const CHCloudwatchAlertLambda api.CloudHelperType = "sc-helper-aws-cloudwatch-alert-lambda"

type AlarmState struct {
	Reason string          `json:"reason"` // Threshold Crossed: 1 datapoint [6.638074000676473 (14/05/24 09:53:00)] was not greater than the threshold (10.0).
	Value  AlarmStateValue `json:"value"`  // OK
}

type AlarmEvent struct {
	AccountId     string     `json:"accountId"` // 471112843480
	AlarmArn      string     `json:"alarmArn"`  // arn:aws:cloudwatch:eu-central-1:471112843480:alarm:seeact-max-cpu-metric-alarm-a275ddf
	AlarmData     AlarmData  `json:"alarmData"`
	State         AlarmState `json:"state"`
	PreviousState AlarmState `json:"previousState"`
	Region        string     `json:"region"` // eu-central-1
}

type AlarmData struct {
	AlarmName     string      `json:"alarmName"` // seeact-max-cpu-metric-alarm-a275ddf
	Configuration AlarmConfig `json:"configuration"`
}

type AlarmConfig struct {
	Description string `json:"description"` // SeeAct CPU usage exceeds 10%
}

type AlarmStateValue string

const (
	ALARM AlarmStateValue = "ALARM"
	OK    AlarmStateValue = "OK"
)

func (l *lambdaCloudHelper) handler(ctx context.Context, event any) error {
	l.log.Info(ctx, fmt.Sprintf("lambda executing handler with event... %v", event))

	var alarmEvent *AlarmEvent
	if d, ok := event.(map[string]any); !ok {
		return errors.Errorf("event is not of type map[string]any")
	} else if e, err := util.ToObjectViaJson(d, alarmEvent); err != nil {
		return errors.Wrapf(err, "failed to convert incoming event to *AlarmEvent")
	} else {
		alarmEvent = e
	}

	// TODO: read secrets from secret manager to obtain discord webhook and telegram token
	l.log.Info(ctx, fmt.Sprintf("unmarshalled cloudwatch event: %v", alarmEvent))

	return nil
}

type lambdaCloudHelper struct {
	log logger.Logger
}

func (l *lambdaCloudHelper) Run() error {
	l.log.Info(context.Background(), "starting cloudwatch alerts...")
	lambda.Start(l.handler)
	l.log.Info(context.Background(), "lambda helper exited")
	return nil
}

func (l *lambdaCloudHelper) SetLogger(log logger.Logger) {
	l.log = log
}

func NewLambdaHelper(opts ...api.CloudHelperOption) (api.CloudHelper, error) {
	res := &lambdaCloudHelper{}

	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, errors.Wrapf(err, "failed to apply option on lambda helper")
		}
	}
	return res, nil
}
