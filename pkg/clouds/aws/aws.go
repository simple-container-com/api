package aws

import "github.com/simple-container-com/api/pkg/api"

type TemplateConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type (
	LambdaRoutingType string
	LambdaInvokeMode  string
	LoadBalancerType  string
)

const (
	LambdaRoutingApiGw             = "api-gateway"
	LambdaRoutingFunctionUrl       = "function-url"
	LambdaInvokeModeBuffered       = "BUFFERED"
	LambdaInvokeModeResponseStream = "RESPONSE_STREAM"

	LoadBalancerTypeAlb LoadBalancerType = "alb"
	LoadBalancerTypeNlb LoadBalancerType = "nlb"
)

type CloudExtras struct {
	AwsRoles          []string          `json:"awsRoles" yaml:"awsRoles"`
	LambdaSchedule    *LambdaSchedule   `json:"lambdaSchedule,omitempty" yaml:"lambdaSchedule,omitempty"`   // e.g. for lambda functions to be triggered on schedule
	LambdaSchedules   []LambdaSchedule  `json:"lambdaSchedules,omitempty" yaml:"lambdaSchedules,omitempty"` // e.g. for lambda functions to be triggered on schedule
	LambdaRoutingType LambdaRoutingType `json:"lambdaRoutingType" yaml:"lambdaRoutingType"`
	LambdaInvokeMode  LambdaInvokeMode  `json:"lambdaInvokeMode" yaml:"lambdaInvokeMode"` // invoke mode for lambda

	SecurityGroup    *SecurityGroup   `json:"securityGroup,omitempty" yaml:"securityGroup,omitempty"`
	LoadBalancerType LoadBalancerType `json:"loadBalancerType,omitempty" yaml:"loadBalancerType,omitempty"` // default: alb
}

type SecurityGroup struct {
	Ingress *SecurityGroupRule `json:"ingress,omitempty" yaml:"ingress,omitempty"`
}

type SecurityGroupRule struct {
	AllowOnlyCloudflare *bool     `json:"allowOnlyCloudflare,omitempty" yaml:"allowOnlyCloudflare,omitempty"`
	CidrBlocks          *[]string `json:"cidrBlocks,omitempty" yaml:"cidrBlocks,omitempty"`
	Ipv6CidrBlocks      *[]string `json:"ipv6CidrBlocks,omitempty" yaml:"ipv6CidrBlocks,omitempty"`
}

func ReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TemplateConfig{})
}
