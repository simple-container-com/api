package gcp

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/monitoring"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AlertsConfig struct {
	MaxErrCount         *float64 `json:"maxErrCount,omitempty"`
	MaxStdoutLinesCount *float64 `json:"maxStdoutLinesCount,omitempty"`
}

type AlertsConfigWithProvider struct {
	AlertsConfig
	Provider *gcp.Provider `json:"provider"`
}

type AlertCfg struct {
	Message  string
	ChatId   *string
	Provider *gcp.Provider
}

type StdErrAlertCfg struct {
	AlertCfg
	ContainerName string
	Threshold     *float64
	AutoClose     *string
}

type TelegramCfg struct {
	DefaultChatId string `json:"defaultChatId"`
}

type DiscordCfg struct {
	WebhookId string `json:"webhookId"`
}

type GCPAlertIncident struct {
	IncidentID string `json:"incident_id,omitempty"`
	URL        string `json:"url,omitempty"`
	PolicyName string `json:"policy_name,omitempty"`
	State      string `json:"state,omitempty"`
}

type GCPAlertPayload struct {
	Incident *GCPAlertIncident `json:"incident,omitempty"`
	Summary  string            `json:"summary,omitempty"`
}

type CreatedAlert struct {
	PolicyName   sdk.Output
	ChannelNames []sdk.Output
}

// nolint: unused
func createMaxStdErrAlertPolicy(ctx *sdk.Context, name string, config StdErrAlertCfg, ntfChannels []sdk.IDOutput) (*monitoring.AlertPolicy, error) {
	filter := fmt.Sprintf(`resource.type = "k8s_container"
		AND resource.labels.container_name = "%s"
		AND metric.type = "logging.googleapis.com/log_entry_count"
		AND metric.labels.log = "stderr"`, config.ContainerName)

	policy, err := monitoring.NewAlertPolicy(ctx, fmt.Sprintf("stderr-%s", name), &monitoring.AlertPolicyArgs{
		Conditions: monitoring.AlertPolicyConditionArray{
			&monitoring.AlertPolicyConditionArgs{
				ConditionThreshold: &monitoring.AlertPolicyConditionConditionThresholdArgs{
					Aggregations: monitoring.AlertPolicyConditionConditionThresholdAggregationArray{
						&monitoring.AlertPolicyConditionConditionThresholdAggregationArgs{
							AlignmentPeriod:    sdk.String("60s"),
							CrossSeriesReducer: sdk.String("REDUCE_SUM"),
							GroupByFields:      sdk.StringArray{sdk.String("metric.label.severity")},
							PerSeriesAligner:   sdk.String("ALIGN_MEAN"),
						},
					},
					Comparison:     sdk.String("COMPARISON_GT"),
					Duration:       sdk.String("0s"),
					Filter:         sdk.String(filter),
					ThresholdValue: sdk.Float64Ptr(lo.FromPtr(config.Threshold)),
					Trigger: &monitoring.AlertPolicyConditionConditionThresholdTriggerArgs{
						Count: sdk.Int(1),
					},
				},
				DisplayName: sdk.String("stderr higher than threshold"),
			},
		},
		Documentation: &monitoring.AlertPolicyDocumentationArgs{
			Content:  sdk.String(config.Message),
			MimeType: sdk.String("text/markdown"),
		},
		AlertStrategy: &monitoring.AlertPolicyAlertStrategyArgs{
			AutoClose: sdk.String(config.AutoCloseOr("86400s")),
		},
		Combiner: sdk.String("AND_WITH_MATCHING_RESOURCE"),
		Enabled:  sdk.Bool(true),
		NotificationChannels: sdk.StringArray(lo.Map(ntfChannels, func(item sdk.IDOutput, _ int) sdk.StringInput {
			return item.ToStringOutput()
		})),
		DisplayName: sdk.String(name),
	}, sdk.Provider(config.Provider))

	return policy, err
}

// nolint: unused
func createMaxStdLinesAlertPolicy(ctx *sdk.Context, name string, config StdErrAlertCfg, ntfChannels []sdk.IDOutput) (*monitoring.AlertPolicy, error) {
	filter := fmt.Sprintf(`resource.type = "k8s_container"
		AND resource.labels.container_name = "%s"
		AND metric.type = "logging.googleapis.com/log_entry_count"
		AND metric.labels.log = "stdout"`, config.ContainerName)

	policy, err := monitoring.NewAlertPolicy(ctx, fmt.Sprintf("stdout-%s", name), &monitoring.AlertPolicyArgs{
		Conditions: monitoring.AlertPolicyConditionArray{
			&monitoring.AlertPolicyConditionArgs{
				ConditionThreshold: &monitoring.AlertPolicyConditionConditionThresholdArgs{
					Aggregations: monitoring.AlertPolicyConditionConditionThresholdAggregationArray{
						&monitoring.AlertPolicyConditionConditionThresholdAggregationArgs{
							AlignmentPeriod:    sdk.String("60s"),
							CrossSeriesReducer: sdk.String("REDUCE_SUM"),
							GroupByFields:      sdk.StringArray{sdk.String("metric.label.severity")},
							PerSeriesAligner:   sdk.String("ALIGN_MEAN"),
						},
					},
					Comparison:     sdk.String("COMPARISON_GT"),
					Duration:       sdk.String("0s"),
					Filter:         sdk.String(filter),
					ThresholdValue: sdk.Float64Ptr(lo.FromPtr(config.Threshold)),
					Trigger: &monitoring.AlertPolicyConditionConditionThresholdTriggerArgs{
						Count: sdk.Int(1),
					},
				},
				DisplayName: sdk.String("stdout higher than threshold"),
			},
		},
		Documentation: &monitoring.AlertPolicyDocumentationArgs{
			Content:  sdk.String(config.Message),
			MimeType: sdk.String("text/markdown"),
		},
		AlertStrategy: &monitoring.AlertPolicyAlertStrategyArgs{
			AutoClose: sdk.String(config.AutoCloseOr("86400s")),
		},
		Combiner: sdk.String("AND_WITH_MATCHING_RESOURCE"),
		Enabled:  sdk.Bool(true),
		NotificationChannels: sdk.StringArray(lo.Map(ntfChannels, func(item sdk.IDOutput, _ int) sdk.StringInput {
			return item.ToStringOutput()
		})),
		DisplayName: sdk.String(name),
	}, sdk.Provider(config.Provider))

	return policy, err
}

// nolint: unused
func (config *StdErrAlertCfg) AutoCloseOr(defaultValue string) string {
	if config.AutoClose != nil {
		return *config.AutoClose
	}
	return defaultValue
}

// nolint: unused
func alertText(incident *GCPAlertIncident, message string) string {
	if incident.State == "open" {
		return fmt.Sprintf("⚠️ triggered: %s", message)
	}
	return fmt.Sprintf("✅ resolved: %s", message)
}

// nolint: unused
var supportedCFLocations = []string{
	"asia-east1", "asia-east2", "asia-northeast1", "asia-northeast2",
	"europe-north1", "europe-west1", "europe-west2", "europe-west4",
	"us-central1", "us-east1", "us-east4", "us-west1",
}

// nolint: unused
const cfFallbackLocation = "europe-west1"

// nolint: unused
func createMaxStderrCountAlert(ctx *sdk.Context, serviceName string, config AlertsConfigWithProvider) (*CreatedAlert, error) {
	alertCfg := StdErrAlertCfg{
		AlertCfg: AlertCfg{
			Message:  fmt.Sprintf("number of errors for '%s' > %.2f per 1 minute", serviceName, *config.MaxErrCount),
			Provider: config.Provider,
		},
		ContainerName: serviceName,
		Threshold:     config.MaxErrCount,
	}

	channels := []sdk.Output{
		// cloudfunctionChannel(ctx, fmt.Sprintf("max-stderr-%s", serviceName), alertCfg),
	}

	policy, err := createMaxStdErrAlertPolicy(ctx, fmt.Sprintf("%s-max-stderr", serviceName), alertCfg, []sdk.IDOutput{})
	if err != nil {
		return nil, err
	}

	return &CreatedAlert{
		PolicyName:   policy.Name,
		ChannelNames: channels,
	}, nil
}

// nolint: unused
func createMaxStdoutLinesAlert(ctx *sdk.Context, serviceName string, config AlertsConfigWithProvider) (*CreatedAlert, error) {
	alertCfg := StdErrAlertCfg{
		AlertCfg: AlertCfg{
			Message:  fmt.Sprintf("number of log lines for '%s' > %.2f per 1 minute", serviceName, *config.MaxStdoutLinesCount),
			Provider: config.Provider,
		},
		ContainerName: serviceName,
		Threshold:     config.MaxStdoutLinesCount,
	}

	channels := []sdk.Output{
		// cloudfunctionChannel(ctx, fmt.Sprintf("max-stdout-%s", serviceName), alertCfg),
	}

	policy, err := createMaxStdLinesAlertPolicy(ctx, fmt.Sprintf("%s-max-stdout", serviceName), alertCfg, []sdk.IDOutput{})
	if err != nil {
		return nil, err
	}

	return &CreatedAlert{
		PolicyName:   policy.Name,
		ChannelNames: channels,
	}, nil
}

// nolint: unused
func createAlertsForService(ctx *sdk.Context, serviceName string, config AlertsConfigWithProvider) ([]CreatedAlert, error) {
	var alerts []CreatedAlert
	if config.MaxErrCount != nil {
		alert, err := createMaxStderrCountAlert(ctx, serviceName, config)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, *alert)
	}
	if config.MaxStdoutLinesCount != nil {
		alert, err := createMaxStdoutLinesAlert(ctx, serviceName, config)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, *alert)
	}
	return alerts, nil
}
