package gcp

import (
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/pubsub"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func PubSubTopics(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypePubSub {
		return nil, errors.Errorf("unsupported pubsub topics type %q", input.Descriptor.Type)
	}

	pubsubCfg, ok := input.Descriptor.Config.Config.(*gcloud.PubSubConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert pubsub topics config for %q", input.Descriptor.Type)
	}

	out, err := createPubSubResources(ctx, pubsubCfg, input, params)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create pubsub topics and subscriptions for stack %q in %q",
			input.StackParams.StackName, input.StackParams.Environment)
	}

	return &api.ResourceOutput{Ref: out}, nil
}

type PubSubResourcesOutput struct {
	Topics        map[string]*pubsub.Topic
	Subscriptions map[string]*pubsub.Subscription
}

func appendInputLabels(baseLabels, additionalLabels gcloud.PlainLabels) gcloud.PlainLabels {
	for key, value := range additionalLabels {
		baseLabels[key] = value
	}
	return baseLabels
}

func createPubSubResources(ctx *sdk.Context, cfg *gcloud.PubSubConfig, input api.ResourceInput, params pApi.ProvisionParams) (*PubSubResourcesOutput, error) {
	commonLabels := appendInputLabels(cfg.Labels, gcloud.PlainLabels{
		"stack-name": input.StackParams.StackName,
		"stack-env":  input.StackParams.Environment,
	})

	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	topics := make(map[string]*pubsub.Topic)
	for _, topic := range cfg.Topics {
		topicLabels := appendInputLabels(commonLabels, topic.Labels)
		psTopic, err := pubsub.NewTopic(ctx, topic.Name, &pubsub.TopicArgs{
			Name:                     sdk.String(topic.Name),
			Labels:                   sdk.ToStringMap(topicLabels),
			MessageRetentionDuration: sdk.String(topic.MessageRetentionDuration),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pubsub topic %q for stack %q in %q",
				topic.Name, input.StackParams.StackName, input.StackParams.Environment)
		}
		topics[topic.Name] = psTopic
	}

	subscriptions := make(map[string]*pubsub.Subscription)
	for _, subscription := range cfg.Subscriptions {
		subscriptionLabels := appendInputLabels(commonLabels, subscription.Labels)

		var deadLetterPolicyArgs *pubsub.SubscriptionDeadLetterPolicyArgs
		if subscription.DeadLetterPolicy != nil {
			deadLetterPolicyArgs = &pubsub.SubscriptionDeadLetterPolicyArgs{
				DeadLetterTopic:     sdk.StringPtrFromPtr(lo.If(subscription.DeadLetterPolicy.DeadLetterTopic != nil, subscription.DeadLetterPolicy.DeadLetterTopic).Else(nil)),
				MaxDeliveryAttempts: sdk.IntPtrFromPtr(lo.If(subscription.DeadLetterPolicy.MaxDeliveryAttempts != nil, subscription.DeadLetterPolicy.MaxDeliveryAttempts).Else(nil)),
			}
		}
		psSubscription, err := pubsub.NewSubscription(ctx, subscription.Name, &pubsub.SubscriptionArgs{
			Name:                      sdk.String(subscription.Name),
			Labels:                    sdk.ToStringMap(subscriptionLabels),
			DeadLetterPolicy:          deadLetterPolicyArgs,
			Topic:                     topics[subscription.Topic].ID(),
			EnableExactlyOnceDelivery: sdk.Bool(subscription.ExactlyOnceDelivery),
			AckDeadlineSeconds:        sdk.Int(subscription.AckDeadlineSec),
			MessageRetentionDuration:  sdk.String(subscription.MessageRetentionDuration),
		}, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create pubsub subscription %q for stack %q in %q",
				subscription.Name, input.StackParams.StackName, input.StackParams.Environment)
		}
		subscriptions[subscription.Name] = psSubscription
	}

	return &PubSubResourcesOutput{
		Topics:        topics,
		Subscriptions: subscriptions,
	}, nil
}

func PubSubTopicsProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	params.Log.Error(ctx.Context(), "not implemented for gcp bucket")

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}
