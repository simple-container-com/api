package discord

import (
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/webhook"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

type alertSender struct {
	client     webhook.Client
	webhookUrl string
}

func (a *alertSender) Send(alert api.Alert) error {
	icon := lo.If(alert.AlertType == api.AlertResolved, "✅").Else("⚠️")
	_, err := a.client.CreateMessage(discord.WebhookMessageCreate{
		Content: icon + fmt.Sprintf(" **%s** [%s](%s) for **%s** in *%s* \n %s",
			alert.AlertType, alert.Title, alert.DetailsUrl, alert.StackName, alert.StackEnv, alert.Description),
	})
	return err
}

func New(webhookUrl string) (api.AlertSender, error) {
	client, err := webhook.NewWithURL(webhookUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init webhook client")
	}
	return &alertSender{
		client:     client,
		webhookUrl: webhookUrl,
	}, nil
}
