package aws

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/cloudflare"
)

func toCidrBlocksArrayInput(blocks []string, subnets defaultSubnets, ipv6 bool) sdk.StringArrayOutput {
	subnetCidrs := lo.Map(subnets, func(net Subnet, _ int) sdk.StringOutput {
		return net.cidrBlock
	})
	subnetIpv6Cidrs := lo.Map(subnets, func(net Subnet, _ int) sdk.StringOutput {
		return net.ipv6CidrBlock
	})

	cidrBlocks := sdk.StringArray{}
	for _, b := range blocks {
		cidrBlocks = append(cidrBlocks, sdk.String(b))
	}
	if !ipv6 {
		for _, b := range subnetCidrs {
			cidrBlocks = append(cidrBlocks, b)
		}
	} else {
		for _, b := range subnetIpv6Cidrs {
			cidrBlocks = append(cidrBlocks, b)
		}
	}
	return cidrBlocks.ToStringArrayOutput()
}

func processIngressSGArgs(args *ec2.SecurityGroupIngressArgs, group aws.SecurityGroup, subnets defaultSubnets) (ec2.SecurityGroupIngressArgs, error) {
	ingress := group.Ingress
	if ingress != nil {
		var ipv4Blocks []string
		var ipv6Blocks []string

		if ingress.CidrBlocks != nil {
			ipv4Blocks = append(ipv4Blocks, *ingress.CidrBlocks...)
		}
		if ingress.Ipv6CidrBlocks != nil {
			ipv6Blocks = append(ipv6Blocks, *ingress.Ipv6CidrBlocks...)
		}
		if ingress.AllowOnlyCloudflare != nil && lo.FromPtr(ingress.AllowOnlyCloudflare) {
			ips, err := cloudflare.GetCloudflareIPs()
			if err != nil {
				return *args, errors.Wrapf(err, "failed to get cloudflare IPs")
			}
			ipv4Blocks = append(ipv4Blocks, ips.IPv4Cidrs...)
			ipv6Blocks = append(ipv6Blocks, ips.IPv6Cidrs...)
		}

		if len(ipv4Blocks) > 0 {
			args.Description = sdk.String("Allow only specified IPs")
			args.CidrBlocks = toCidrBlocksArrayInput(ipv4Blocks, subnets, false).ApplyT(func(arg any) []string {
				return lo.Filter(arg.([]string), func(b string, _ int) bool {
					return strings.TrimSpace(b) != ""
				})
			}).(sdk.StringArrayOutput)
		}
		if len(ipv6Blocks) > 0 {
			args.Description = sdk.String("Allow only specified IPs")
			args.Ipv6CidrBlocks = toCidrBlocksArrayInput(ipv6Blocks, subnets, true).ApplyT(func(arg any) []string {
				return lo.Filter(arg.([]string), func(b string, _ int) bool {
					return strings.TrimSpace(b) != ""
				})
			}).(sdk.StringArrayOutput)
		}
	}

	return *args, nil
}
