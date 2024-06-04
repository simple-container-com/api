package helpers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

const CHHealthBridgeAlertLambda api.CloudHelperType = "sc-helper-aws-health-bridge-lambda"

func (l *lambdaHealthBridgeCloudHelper) handler(ctx context.Context, event any) error {
	l.log.Info(ctx, fmt.Sprintf("health bridge lambda executing handler with event... %v", event))
	// TODO: CU-86bytgw4y

	return nil
}

type lambdaHealthBridgeCloudHelper struct {
	log logger.Logger
}

func (l *lambdaHealthBridgeCloudHelper) Run() error {
	l.log.Info(context.Background(), "starting events bridge alerts...")
	lambda.Start(l.handler)
	l.log.Info(context.Background(), "lambda helper exited")
	return nil
}

func (l *lambdaHealthBridgeCloudHelper) SetLogger(log logger.Logger) {
	l.log = log
}

func NewHealthBridgeLambdaHelper(opts ...api.CloudHelperOption) (api.CloudHelper, error) {
	res := &lambdaHealthBridgeCloudHelper{}

	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, errors.Wrapf(err, "failed to apply option on lambda helper")
		}
	}
	return res, nil
}
