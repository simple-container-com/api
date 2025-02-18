package helpers

import (
	"context"
	"fmt"
	"os"

	"github.com/simple-container-com/api/pkg/clouds/slack"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-secretsmanager-caching-go/secretcache"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/discord"
	"github.com/simple-container-com/api/pkg/clouds/telegram"
	"github.com/simple-container-com/api/pkg/util"
)

const CHCloudwatchAlertLambda api.CloudHelperType = "sc-helper-aws-cloudwatch-alert-lambda"

type AlarmState struct {
	Reason string          `json:"reason"` // Threshold Crossed: 1 datapoint [6.638074000676473 (14/05/24 09:53:00)] was not greater than the threshold (10.0).
	Value  AlarmStateValue `json:"value"`  // OK
}

type AlarmEvent struct {
	AccountId string    `json:"accountId"` // 471112843480
	AlarmArn  string    `json:"alarmArn"`  // arn:aws:cloudwatch:eu-central-1:471112843480:alarm:seeact-max-cpu-metric-alarm-a275ddf
	AlarmData AlarmData `json:"alarmData"`
	Region    string    `json:"region"` // eu-central-1
}

type AlarmData struct {
	AlarmName     string      `json:"alarmName"` // seeact-max-cpu-metric-alarm-a275ddf
	State         AlarmState  `json:"state"`
	PreviousState AlarmState  `json:"previousState"`
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

var secretCache, _ = secretcache.New()

func (l *cloudwatchEventsLambda) handler(ctx context.Context, event any) error {
	l.log.Info(ctx, fmt.Sprintf("lambda executing handler with event... %v", event))

	var alarmEvent *AlarmEvent
	if d, ok := event.(map[string]any); !ok {
		return errors.Errorf("event is not of type map[string]any")
	} else if e, err := util.ToObjectViaJson(d, alarmEvent); err != nil {
		return errors.Wrapf(err, "failed to convert incoming event to *AlarmEvent")
	} else {
		alarmEvent = e
	}

	l.log.Info(ctx, "unmarshalled cloudwatch event: %v", alarmEvent)

	stackName := os.Getenv(api.ComputeEnv.StackName)
	stackEnv := os.Getenv(api.ComputeEnv.StackEnv)
	alertName := os.Getenv(api.ComputeEnv.AlertName)
	alertDescription := os.Getenv(api.ComputeEnv.AlertDescription)

	l.log.Info(ctx, "sending event for stack %q in %q", stackName, stackEnv)

	nfAlert := api.Alert{
		Name:        alertName,
		Title:       alertName,
		Description: alertDescription,
		Reason:      alarmEvent.AlarmData.State.Reason,
		StackName:   stackName,
		StackEnv:    stackEnv,
		AlertType:   lo.If(alarmEvent.AlarmData.State.Value == ALARM, api.AlertTriggered).Else(api.AlertResolved),
		DetailsUrl: fmt.Sprintf("https://%s.console.aws.amazon.com/cloudwatch/home?region=%s#alarmsV2:alarm/%s",
			alarmEvent.Region, alarmEvent.Region, alarmEvent.AlarmData.AlarmName),
	}

	// send discord notifications if configured
	if discordWebhookSecret := os.Getenv(api.ComputeEnv.DiscordWebhookUrl); discordWebhookSecret == "" {
		l.log.Info(ctx, "discord notification isn't configured")
	} else if discordWebhook, err := secretCache.GetSecretString(discordWebhookSecret); err != nil {
		l.log.Error(ctx, "failed to get discord webhook secret value: %v", err)
	} else if d, err := discord.New(discordWebhook); err != nil {
		l.log.Error(ctx, "failed to create discord webhook client: %v", err)
	} else if err := d.Send(nfAlert); err != nil {
		l.log.Error(ctx, "failed to send alert to discord: %v", err)
	}

	// send slack notifications is configured
	if slackWebhookSecret := os.Getenv(api.ComputeEnv.SlackWebhookUrl); slackWebhookSecret == "" {
		l.log.Info(ctx, "slcak notification isn't configured")
	} else if slackWebhook, err := secretCache.GetSecretString(slackWebhookSecret); err != nil {
		l.log.Error(ctx, "failed to get slack webhook secret value: %v", err)
	} else if d, err := slack.New(slackWebhook); err != nil {
		l.log.Error(ctx, "failed to create slack webhook client: %v", err)
	} else if err := d.Send(nfAlert); err != nil {
		l.log.Error(ctx, "failed to send alert to slack: %v", err)
	}

	// send telegram notification if configured
	telegramChatId := os.Getenv(api.ComputeEnv.TelegramChatID)
	if telegramTokenSecret := os.Getenv(api.ComputeEnv.TelegramToken); telegramTokenSecret == "" {
		l.log.Info(ctx, "telegram notification isn't configured")
	} else if telegramToken, err := secretCache.GetSecretString(telegramTokenSecret); err != nil {
		l.log.Error(ctx, "failed to get telegram webhook secret value: %v", err)
	} else {
		if err := telegram.New(telegramChatId, telegramToken).Send(nfAlert); err != nil {
			l.log.Error(ctx, "failed to send alert to telegram: %v", err)
		}
	}

	return nil
}

type cloudwatchEventsLambda struct {
	log logger.Logger
}

func (l *cloudwatchEventsLambda) Run() error {
	l.log.Info(context.Background(), "starting cloudwatch alerts...")
	lambda.Start(l.handler)
	l.log.Info(context.Background(), "lambda helper exited")
	return nil
}

func (l *cloudwatchEventsLambda) SetLogger(log logger.Logger) {
	l.log = log
}

func NewCloudwatchLambdaHelper(opts ...api.CloudHelperOption) (api.CloudHelper, error) {
	res := &cloudwatchEventsLambda{}

	for _, opt := range opts {
		if err := opt(res); err != nil {
			return nil, errors.Wrapf(err, "failed to apply option on lambda helper")
		}
	}
	return res, nil
}
