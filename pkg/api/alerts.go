package api

type (
	CloudHelperType string
	CHEnvVariables  struct {
		DiscordWebhookUrl string
		TelegramChatID    string
		TelegramToken     string
		AlertName         string
		AlertDescription  string
		StackName         string
		StackEnv          string
		Type              string
	}
)

var CloudHelpersEnv = CHEnvVariables{
	Type:              "SIMPLE_CONTAINER_CLOUD_HELPER_TYPE",
	DiscordWebhookUrl: "SIMPLE_CONTAINER_DISCORD_WEBHOOK_URL",
	TelegramChatID:    "SIMPLE_CONTAINER_TELEGRAM_CHAT_ID",
	TelegramToken:     "SIMPLE_CONTAINER_TELEGRAM_TOKEN",
	AlertName:         "SIMPLE_CONTAINER_ALERT_NAME",
	AlertDescription:  "SIMPLE_CONTAINER_ALERT_DESCRIPTION",
	StackName:         "SIMPLE_CONTAINER_STACK_NAME",
	StackEnv:          "SIMPLE_CONTAINER_ENV",
}

type AlertsConfig struct {
	MaxCPU    *MaxCPUConfig    `json:"maxCPU,omitempty" yaml:"maxCPU,omitempty"`
	MaxMemory *MaxMemoryConfig `json:"maxMemory,omitempty" yaml:"maxMemory,omitempty"`
	MaxErrors *MaxErrorConfig  `json:"maxErrors,omitempty" yaml:"maxErrors,omitempty"`
	Discord   *DiscordCfg      `json:"discord,omitempty" yaml:"discord,omitempty"`
	Telegram  *TelegramCfg     `json:"telegram,omitempty" yaml:"telegram,omitempty"`
}

type CommonAlertConfig struct {
	Threshold   float64 `json:"threshold" yaml:"threshold"`
	PeriodSec   int     `json:"periodSec" yaml:"periodSec"`
	AlertName   string  `json:"alertName" yaml:"alertName"`
	Description string  `json:"description" yaml:"description"`
}

type MaxCPUConfig struct {
	CommonAlertConfig `json:",inline" yaml:",inline"`
}

type MaxMemoryConfig struct {
	CommonAlertConfig `json:",inline" yaml:",inline"`
}

type TelegramCfg struct {
	ChatID string `json:"chatID" yaml:"chatID"`
	Token  string `json:"token" yaml:"token"`
}

type DiscordCfg struct {
	WebhookUrl string `json:"webhookUrl" yaml:"webhookUrl"`
}

type MaxErrorConfig struct {
	CommonAlertConfig     `json:",inline" yaml:",inline"`
	ErrorLogMessageRegexp string `json:"errorLogMessageRegexp" yaml:"errorLogMessageRegexp"`
}

type AlertType string

const (
	AlertTriggered AlertType = "TRIGGERED"
	AlertResolved  AlertType = "RESOLVED"
)

type Alert struct {
	Name        string    `json:"name" yaml:"name"`
	Title       string    `json:"title" yaml:"title"`
	Reason      string    `json:"reason" yaml:"reason"`
	Description string    `json:"description" yaml:"description"`
	StackName   string    `json:"stackName" yaml:"stackName"`
	StackEnv    string    `json:"stackEnv" yaml:"stackEnv"`
	DetailsUrl  string    `json:"detailsUrl" yaml:"detailsUrl"`
	AlertType   AlertType `json:"alertType" yaml:"alertType"`
}

type AlertSender interface {
	Send(Alert) error
}
