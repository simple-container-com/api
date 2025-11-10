package api

type (
	CloudHelperType     string
	ComputeEnvVariables struct {
		DiscordWebhookUrl string
		SlackWebhookUrl   string
		TelegramChatID    string
		TelegramToken     string
		AlertName         string
		AlertDescription  string
		StackName         string
		StackEnv          string
		CloudHelperType   string
		StackVersion      string
	}
)

var ComputeEnv = ComputeEnvVariables{
	CloudHelperType:   "SIMPLE_CONTAINER_CLOUD_HELPER_TYPE",
	DiscordWebhookUrl: "SIMPLE_CONTAINER_DISCORD_WEBHOOK_URL",
	SlackWebhookUrl:   "SIMPLE_CONTAINER_SLACK_WEBHOOK_URL",
	TelegramChatID:    "SIMPLE_CONTAINER_TELEGRAM_CHAT_ID",
	TelegramToken:     "SIMPLE_CONTAINER_TELEGRAM_TOKEN",
	AlertName:         "SIMPLE_CONTAINER_ALERT_NAME",
	AlertDescription:  "SIMPLE_CONTAINER_ALERT_DESCRIPTION",
	StackName:         "SIMPLE_CONTAINER_STACK",
	StackEnv:          "SIMPLE_CONTAINER_ENV",
	StackVersion:      "SIMPLE_CONTAINER_VERSION",
}

type AlertsConfig struct {
	MaxCPU         *MaxCPUConfig         `json:"maxCPU,omitempty" yaml:"maxCPU,omitempty"`
	MaxMemory      *MaxMemoryConfig      `json:"maxMemory,omitempty" yaml:"maxMemory,omitempty"`
	MaxErrors      *MaxErrorConfig       `json:"maxErrors,omitempty" yaml:"maxErrors,omitempty"`
	ServerErrors   *ServerErrorsConfig   `json:"serverErrors,omitempty" yaml:"serverErrors,omitempty"`
	UnhealthyHosts *UnhealthyHostsConfig `json:"unhealthyHosts,omitempty" yaml:"unhealthyHosts,omitempty"`
	ResponseTime   *ResponseTimeConfig   `json:"responseTime,omitempty" yaml:"responseTime,omitempty"`
	Discord        *DiscordCfg           `json:"discord,omitempty" yaml:"discord,omitempty"`
	Slack          *SlackCfg             `json:"slack,omitempty" yaml:"slack,omitempty"`
	Telegram       *TelegramCfg          `json:"telegram,omitempty" yaml:"telegram,omitempty"`
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

type SlackCfg struct {
	WebhookUrl string `json:"webhookUrl" yaml:"webhookUrl"`
}

type MaxErrorConfig struct {
	CommonAlertConfig     `json:",inline" yaml:",inline"`
	ErrorLogMessageRegexp string `json:"errorLogMessageRegexp" yaml:"errorLogMessageRegexp"`
}

// ALB-specific alert configurations
type ServerErrorsConfig struct {
	CommonAlertConfig `json:",inline" yaml:",inline"`
}

type UnhealthyHostsConfig struct {
	CommonAlertConfig `json:",inline" yaml:",inline"`
}

type ResponseTimeConfig struct {
	CommonAlertConfig `json:",inline" yaml:",inline"`
}

type AlertType string

const (
	// Monitoring Alert Types
	AlertTriggered AlertType = "TRIGGERED"
	AlertResolved  AlertType = "RESOLVED"

	// Build/Deployment Notification Types
	BuildStarted   AlertType = "BUILD_STARTED"
	BuildSucceeded AlertType = "BUILD_SUCCEEDED"
	BuildFailed    AlertType = "BUILD_FAILED"
	BuildCancelled AlertType = "BUILD_CANCELLED"
)

type Alert struct {
	Name          string    `json:"name" yaml:"name"`
	Title         string    `json:"title" yaml:"title"`
	Reason        string    `json:"reason" yaml:"reason"`
	Description   string    `json:"description" yaml:"description"`
	StackName     string    `json:"stackName" yaml:"stackName"`
	StackEnv      string    `json:"stackEnv" yaml:"stackEnv"`
	DetailsUrl    string    `json:"detailsUrl" yaml:"detailsUrl"`
	AlertType     AlertType `json:"alertType" yaml:"alertType"`
	CommitAuthor  string    `json:"commitAuthor,omitempty" yaml:"commitAuthor,omitempty"`
	CommitMessage string    `json:"commitMessage,omitempty" yaml:"commitMessage,omitempty"`
}

type AlertSender interface {
	Send(Alert) error
}
