package aws

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/simple-container-com/api/pkg/api"
)

const CloudHelperLambda = "sc-helper-aws-lambda"

func handler(ctx context.Context, event events.CloudWatchEvent) error {
	log.Printf("Received event: %v", event)
	return nil
}

type lambdaCloudHelper struct {
	opts []api.CloudHelperOption
}

func (l lambdaCloudHelper) Run() error {
	lambda.Start(handler)
	return nil
}

func NewLambdaHelper(opts ...api.CloudHelperOption) (api.CloudHelper, error) {
	return &lambdaCloudHelper{
		opts: opts,
	}, nil
}
