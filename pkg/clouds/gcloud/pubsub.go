package gcloud

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ResourceTypePubSub = "gcp-pubsub"

type PubSubConfig struct {
	Credentials   `json:",inline" yaml:",inline"`
	Topics        []PubSubTopic        `json:"topics" yaml:"topics"`
	Subscriptions []PubSubSubscription `json:"subscriptions" yaml:"subscriptions"`
	Labels        PlainLabels          `json:"labels" yaml:"labels"`
}

type PlainLabels map[string]string

type SubscriptionDeadLetterPolicyArgs struct {
	DeadLetterTopic     *string `json:"deadLetterTopic,omitempty" yaml:"deadLetterTopic,omitempty"`
	MaxDeliveryAttempts *int    `json:"maxDeliveryAttempts,omitempty" yaml:"maxDeliveryAttempts,omitempty"`
}

type PubSubTopic struct {
	Name                     string      `json:"name" yaml:"name"`
	Labels                   PlainLabels `json:"labels" yaml:"labels"`
	MessageRetentionDuration string      `json:"messageRetentionDuration" yaml:"messageRetentionDuration"`
}

type PubSubSubscription struct {
	Name                     string                            `json:"name" yaml:"name"`
	Topic                    string                            `json:"topic" yaml:"topic"`
	Labels                   PlainLabels                       `json:"labels" yaml:"labels"`
	DeadLetterPolicy         *SubscriptionDeadLetterPolicyArgs `json:"deadLetterPolicy" yaml:"deadLetterPolicy"`
	ExactlyOnceDelivery      bool                              `json:"exactlyOnceDelivery" yaml:"exactlyOnceDelivery"`
	AckDeadlineSec           int                               `json:"ackDeadlineSec" yaml:"ackDeadlineSec"`
	MessageRetentionDuration string                            `json:"messageRetentionDuration" yaml:"messageRetentionDuration"`
}

func GcpPubSubTopicsReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PubSubConfig{})
}
