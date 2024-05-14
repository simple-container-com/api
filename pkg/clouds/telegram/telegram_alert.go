package telegram

import (
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

type alertSender struct {
	chatId string
	token  string
}

func (a *alertSender) Send(alert api.Alert) error {
	return errors.Errorf("Not implemented")
}

func New(chatId, token string) api.AlertSender {
	return &alertSender{
		chatId: chatId,
		token:  token,
	}
}
