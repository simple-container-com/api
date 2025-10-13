package slack

import (
	"fmt"

	"github.com/anthonycorbacho/slack-webhook"

	"github.com/simple-container-com/api/pkg/api"
)

type alertSender struct {
	webhookUrl string
}

func (a *alertSender) Send(alert api.Alert) error {
	icon := getIconForAlertType(alert.AlertType)
	err := slack.Send(a.webhookUrl, slack.Message{
		Text: icon + fmt.Sprintf(" *%s* <%s|%s> for *%s* in *%s* \n %s",
			alert.AlertType, alert.DetailsUrl, alert.Title, alert.StackName, alert.StackEnv, alert.Description),
		Markdown: true,
	})
	return err
}

func getIconForAlertType(alertType api.AlertType) string {
	switch alertType {
	// Monitoring Alert Types
	case api.AlertTriggered:
		return "⚠️"
	case api.AlertResolved:
		return "✅"
	// Build/Deployment Notification Types
	case api.BuildStarted:
		return "🚀"
	case api.BuildSucceeded:
		return "✅"
	case api.BuildFailed:
		return "❌"
	case api.BuildCancelled:
		return "⏹️"
	default:
		return "ℹ️"
	}
}

func New(webhookUrl string) (api.AlertSender, error) {
	return &alertSender{
		webhookUrl: webhookUrl,
	}, nil
}
