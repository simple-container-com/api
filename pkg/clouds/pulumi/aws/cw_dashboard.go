package aws

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/provisioner/placeholders"
)

const ecsDashboardTemplate = `
{
    "widgets": [
        {
            "height": 6,
            "width": 12,
            "y": 0,
            "x": 0,
            "type": "metric",
            "properties": {
                "metrics": [
                    [ "AWS/ECS", "CPUUtilization", "ServiceName", "${ecsServiceName}", "ClusterName", "${ecsClusterName}", { "stat": "Minimum" } ],
                    [ "...", { "stat": "Maximum" } ],
                    [ "...", { "stat": "Average" } ]
                ],
                "period": 300,
                "region": "${region}",
                "stacked": false,
                "title": "${stackName} CPU usage",
                "view": "timeSeries"
            }
        },
        {
            "height": 6,
            "width": 12,
            "y": 0,
            "x": 12,
            "type": "metric",
            "properties": {
                "metrics": [
                    [ "AWS/ECS", "MemoryUtilization", "ServiceName", "${ecsServiceName}", "ClusterName", "${ecsClusterName}", { "stat": "Minimum" } ],
                    [ "...", { "stat": "Maximum" } ],
                    [ "...", { "stat": "Average" } ]
                ],
                "period": 300,
                "region": "${region}",
                "stacked": false,
                "title": "${stackName} RAM usage",
                "view": "timeSeries"
            }
        },
        {
            "height": 6,
            "width": 24,
            "y": 6,
            "x": 0,
            "type": "log",
            "properties": {
                "query": "SOURCE '${logGroupName}' | fields @timestamp, @message, @logStream, @log\n| sort @timestamp desc\n| limit 1000",
                "region": "${region}",
                "stacked": false,
                "view": "table",
                "title": "${stackName} recent logs"
            }
        }
    ]
}
`

type ecsCloudwatchDashboardCfg struct {
	region         string
	stackName      string
	ecsServiceName string
	logGroupName   string
	ecsClusterName string
}

func createEcsCloudwatchDashboard(ctx *sdk.Context, cfg ecsCloudwatchDashboardCfg, params pApi.ProvisionParams) error {
	params.Log.Info(ctx.Context(), "configure ecs cloudwatch dashboard with config: %v ...", cfg)
	dashboardJSON := ecsDashboardTemplate
	data := placeholders.MapData{
		"stackName":      cfg.stackName,
		"region":         cfg.region,
		"ecsServiceName": cfg.ecsServiceName,
		"logGroupName":   cfg.logGroupName,
		"ecsClusterName": cfg.ecsClusterName,
	}
	if err := placeholders.New().Apply(&dashboardJSON, placeholders.WithData(data)); err != nil {
		return errors.Wrapf(err, "failed to apply placeholders on dashboard template")
	}
	_, err := cloudwatch.NewDashboard(ctx, fmt.Sprintf("%s-cw-dashboard", cfg.stackName), &cloudwatch.DashboardArgs{
		DashboardName: sdk.String(fmt.Sprintf("%s-status", cfg.stackName)),
		DashboardBody: sdk.String(dashboardJSON),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return errors.Wrapf(err, "failed to create cloudwatch dashboard")
	}
	return nil
}
