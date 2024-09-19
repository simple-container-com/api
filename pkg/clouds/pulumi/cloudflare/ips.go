package cloudflare

import (
	"github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
)

type IPs struct {
	IPv4Cidrs []string
	IPv6Cidrs []string
}

func GetCloudflareIPs() (*IPs, error) {
	res, err := cloudflare.IPs()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get Cloudflare IPs")
	}

	return &IPs{
		IPv4Cidrs: res.IPv4CIDRs,
		IPv6Cidrs: res.IPv6CIDRs,
	}, nil
}
