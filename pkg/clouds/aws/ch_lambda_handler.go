package aws

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/simple-container-com/api/pkg/api"
)

const CloudHelperLambda = "sc-helper-aws-lambda"

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse form data from the request body
	parsedFormData, err := url.ParseQuery(request.Body)
	if err != nil {
		log.Printf("Error parsing form data: %s", err)
		return events.APIGatewayProxyResponse{StatusCode: 400}, err
	}

	// Access POST parameters
	key1 := parsedFormData.Get("key1")
	key2 := parsedFormData.Get("key2")

	log.Printf("Received Key1: %s", key1)
	log.Printf("Received Key2: %s", key2)

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       fmt.Sprintf(`%s %s`, key1, key2),
	}, nil
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
