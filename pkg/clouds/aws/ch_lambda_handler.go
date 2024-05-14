package aws

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

const CloudHelperLambda = "sc-helper-aws-lambda"

func (l *lambdaCloudHelper) handler(ctx context.Context, event *events.CloudWatchEvent) error {
	l.log.Info(ctx, fmt.Sprintf("lambda executing handler with event... %v", event))
	return nil
}

type lambdaCloudHelper struct {
	log logger.Logger
}

func (l *lambdaCloudHelper) Run() error {
	l.log.Info(context.Background(), "starting lambda helper...")
	if os.Getenv("SIMPLE_CONTAINER_DEBUG") == "true" {
		time.Sleep(10 * time.Second)
	}
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
