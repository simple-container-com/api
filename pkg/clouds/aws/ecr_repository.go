package aws

import "github.com/simple-container-com/api/pkg/api"

const ResourceTypeEcrRepository = "ecr-repository"

type EcrRepository struct {
	AccountConfig   `json:",inline" yaml:",inline"`
	Name            string              `json:"name,omitempty" yaml:"name,omitempty"`
	LifecyclePolicy *EcrLifecyclePolicy `json:"lifecyclePolicy" yaml:"lifecyclePolicy"`
}

type EcrLifecyclePolicy struct {
	Rules []EcrLifecycleRule `json:"rules" yaml:"rules"`
}

// DefaultEcrLifecyclePolicy is the default ECR lifecycle policy (keep only 3 last images)
var DefaultEcrLifecyclePolicy = EcrLifecyclePolicy{
	Rules: []EcrLifecycleRule{
		{
			RulePriority: 1,
			Description:  "Keep only 3 last images",
			Selection: EcrLifecyclePolicySelection{
				TagStatus:   "any",
				CountType:   "imageCountMoreThan",
				CountNumber: 3,
			},
			Action: EcrLifecyclePolicyAction{
				Type: "expire",
			},
		},
	},
}

type EcrLifecycleRule struct {
	RulePriority int                         `json:"rulePriority" yaml:"rulePriority"`
	Description  string                      `json:"description" yaml:"description"`
	Selection    EcrLifecyclePolicySelection `json:"selection" yaml:"selection"`
	Action       EcrLifecyclePolicyAction    `json:"action" yaml:"action"`
}

type EcrLifecyclePolicySelection struct {
	TagStatus      string   `json:"tagStatus" yaml:"tagStatus"`
	CountType      string   `json:"countType" yaml:"countType"`
	CountNumber    int      `json:"countNumber" yaml:"countNumber"`
	TagPatternList []string `json:"tagPatternList" yaml:"tagPatternList"`
	TagPrefixList  []string `json:"tagPrefixList" yaml:"tagPrefixList"`
}

type EcrLifecyclePolicyAction struct {
	Type string `json:"type" yaml:"type"`
}

func EcrRepositoryReadConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &EcrRepository{})
}
