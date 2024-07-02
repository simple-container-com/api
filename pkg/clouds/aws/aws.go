package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type LambdaRoutingType string

const (
	LambdaRoutingApiGw       = "api-gateway"
	LambdaRoutingFunctionUrl = "function-url"
)

type CloudExtras struct {
	AwsRoles          []string          `json:"awsRoles" yaml:"awsRoles"`
	LambdaSchedule    *LambdaSchedule   `json:"lambdaSchedule,omitempty" yaml:"lambdaSchedule,omitempty"`   // e.g. for lambda functions to be triggered on schedule
	LambdaSchedules   []LambdaSchedule  `json:"lambdaSchedules,omitempty" yaml:"lambdaSchedules,omitempty"` // e.g. for lambda functions to be triggered on schedule
	LambdaRoutingType LambdaRoutingType `json:"lambdaRoutingType" yaml:"lambdaRoutingType"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
