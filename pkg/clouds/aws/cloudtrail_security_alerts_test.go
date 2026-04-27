package aws

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func TestReadCloudTrailSecurityAlertsConfig(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name    string
		config  *api.Config
		wantErr bool
	}{
		{
			name: "valid config with all CIS alerts enabled",
			config: &api.Config{
				Config: map[string]any{
					"logGroupName":   "aws-cloudtrail-logs-s3-buckets",
					"logGroupRegion": "us-west-2",
					"slack": map[string]any{
						"webhookUrl": "https://hooks.slack.com/services/xxx",
					},
					"alerts": map[string]any{
						"rootAccountUsage":       true,
						"unauthorizedApiCalls":   true,
						"consoleLoginWithoutMfa": true,
						"iamPolicyChanges":       true,
						"cloudTrailTampering":    true,
						"failedConsoleLogins":    true,
						"kmsKeyDeletion":         true,
						"s3BucketPolicyChanges":  true,
						"configChanges":          true,
						"securityGroupChanges":   true,
						"naclChanges":            true,
						"networkGatewayChanges":  true,
						"routeTableChanges":      true,
						"vpcChanges":             true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with partial alerts",
			config: &api.Config{
				Config: map[string]any{
					"logGroupName": "my-trail-logs",
					"alerts": map[string]any{
						"rootAccountUsage":    true,
						"cloudTrailTampering": true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with no alerts",
			config: &api.Config{
				Config: map[string]any{
					"logGroupName": "my-trail-logs",
					"alerts":      map[string]any{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			result, err := ReadCloudTrailSecurityAlertsConfig(tt.config)
			if tt.wantErr {
				Expect(err).ToNot(BeNil())
				return
			}
			Expect(err).To(BeNil())
			cfg, ok := result.Config.(*CloudTrailSecurityAlertsConfig)
			Expect(ok).To(BeTrue())
			Expect(cfg).ToNot(BeNil())
		})
	}
}

func TestReadCloudTrailSecurityAlertsConfig_FieldValues(t *testing.T) {
	RegisterTestingT(t)

	config := &api.Config{
		Config: map[string]any{
			"logGroupName":   "aws-cloudtrail-logs-s3-buckets",
			"logGroupRegion": "us-west-2",
			"slack": map[string]any{
				"webhookUrl": "https://hooks.slack.com/services/xxx",
			},
			"email": map[string]any{
				"addresses": []any{"security@example.com", "ops@example.com"},
			},
			"alerts": map[string]any{
				"rootAccountUsage":       true,
				"unauthorizedApiCalls":   false,
				"consoleLoginWithoutMfa": true,
				"iamPolicyChanges":       true,
				"cloudTrailTampering":    true,
				"failedConsoleLogins":    true,
				"kmsKeyDeletion":         true,
				"s3BucketPolicyChanges":  false,
				"configChanges":          true,
				"securityGroupChanges":   true,
				"naclChanges":            true,
				"networkGatewayChanges":  false,
				"routeTableChanges":      false,
				"vpcChanges":             true,
			},
		},
	}

	result, err := ReadCloudTrailSecurityAlertsConfig(config)
	Expect(err).To(BeNil())

	cfg := result.Config.(*CloudTrailSecurityAlertsConfig)
	Expect(cfg.LogGroupName).To(Equal("aws-cloudtrail-logs-s3-buckets"))
	Expect(cfg.LogGroupRegion).To(Equal("us-west-2"))
	Expect(cfg.Slack).ToNot(BeNil())
	Expect(cfg.Slack.WebhookUrl).To(Equal("https://hooks.slack.com/services/xxx"))
	Expect(cfg.Email).ToNot(BeNil())
	Expect(cfg.Email.Addresses).To(HaveLen(2))

	Expect(cfg.Alerts.RootAccountUsage).To(BeTrue())
	Expect(cfg.Alerts.UnauthorizedApiCalls).To(BeFalse())
	Expect(cfg.Alerts.ConsoleLoginWithoutMfa).To(BeTrue())
	Expect(cfg.Alerts.IamPolicyChanges).To(BeTrue())
	Expect(cfg.Alerts.CloudTrailTampering).To(BeTrue())
	Expect(cfg.Alerts.FailedConsoleLogins).To(BeTrue())
	Expect(cfg.Alerts.KmsKeyDeletion).To(BeTrue())
	Expect(cfg.Alerts.S3BucketPolicyChanges).To(BeFalse())
	Expect(cfg.Alerts.ConfigChanges).To(BeTrue())
	Expect(cfg.Alerts.SecurityGroupChanges).To(BeTrue())
	Expect(cfg.Alerts.NaclChanges).To(BeTrue())
	Expect(cfg.Alerts.NetworkGatewayChanges).To(BeFalse())
	Expect(cfg.Alerts.RouteTableChanges).To(BeFalse())
	Expect(cfg.Alerts.VpcChanges).To(BeTrue())
}
