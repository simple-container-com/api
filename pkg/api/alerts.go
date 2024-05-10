package api

type CloudHelpersEnvVariables struct {
	DiscordWebhookUrl string
	TelegramChatID    string
	TelegramToken     string
	AlertName         string
	AlertDescription  string
}

var CloudHelpersEnv = CloudHelpersEnvVariables{
	DiscordWebhookUrl: "SIMPLE_CONTAINER_DISCORD_WEBHOOK_URL",
	TelegramChatID:    "SIMPLE_CONTAINER_TELEGRAM_CHAT_ID",
	TelegramToken:     "SIMPLE_CONTAINER_TELEGRAM_TOKEN",
	AlertName:         "SIMPLE_CONTAINER_ALERT_NAME",
	AlertDescription:  "SIMPLE_CONTAINER_ALERT_NAME",
}

type AlertsConfig struct {
	MaxCPU    *MaxCPUConfig    `json:"maxCPU,omitempty" yaml:"maxCPU,omitempty"`
	MaxMemory *MaxMemoryConfig `json:"maxMemory,omitempty" yaml:"maxMemory,omitempty"`
	MaxErrors *MaxErrorConfig  `json:"maxErrors,omitempty" yaml:"maxErrors,omitempty"`
	Discord   *DiscordCfg      `json:"discord,omitempty" yaml:"discord,omitempty"`
	Telegram  *TelegramCfg     `json:"telegram,omitempty" yaml:"telegram,omitempty"`
}

type CommonAlertConfig struct {
	Threshold   string `json:"threshold" yaml:"threshold"`
	AlertName   string `json:"alertName" yaml:"alertName"`
	Description string `json:"description" yaml:"description"`
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
