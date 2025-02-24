package slack

import (
	"fmt"

	"github.com/anthonycorbacho/slack-webhook"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

type alertSender struct {
	webhookUrl string
}

func (a *alertSender) Send(alert api.Alert) error {
	icon := lo.If(alert.AlertType == api.AlertResolved, "✅").Else("⚠️")
	err := slack.Send(a.webhookUrl, slack.Message{
		Text: icon + fmt.Sprintf(" *%s* <%s|%s> for *%s* in *%s* \n %s",
			alert.AlertType, alert.DetailsUrl, alert.Title, alert.StackName, alert.StackEnv, alert.Description),
		Markdown: true,
	})
	return err
}

func New(webhookUrl string) (api.AlertSender, error) {
	return &alertSender{
		webhookUrl: webhookUrl,
	}, nil
}
