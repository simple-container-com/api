// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// cfIPs is a minimal helper that returns Cloudflare's public edge CIDR
// ranges, used by the AWS security-group driver to admit CF-fronted
// traffic. Historically this lived in pkg/clouds/pulumi/cloudflare/ips.go
// (wrapping cloudflare-go's IPs()) — but the whole in-tree cloudflare
// subtree moves to simple-container-com/sc-plugin-cloudflare, and this
// one tiny utility doesn't belong in a cloud-driver plugin. Inlined here
// to drop the cloudflare-go dependency.
//
// Endpoint docs: https://developers.cloudflare.com/api/operations/cloudflare-i-ps-cloudflare-ip-details
type cfIPs struct {
	IPv4Cidrs []string
	IPv6Cidrs []string
}

func getCloudflareIPs() (*cfIPs, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.cloudflare.com/client/v4/ips", nil)
	if err != nil {
		return nil, errors.Wrap(err, "build request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "fetch cloudflare ips")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("cloudflare ips endpoint: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Result struct {
			IPv4Cidrs []string `json:"ipv4_cidrs"`
			IPv6Cidrs []string `json:"ipv6_cidrs"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, errors.Wrap(err, "decode cloudflare ips response")
	}
	if !body.Success {
		return nil, errors.New("cloudflare ips response marked success=false")
	}
	return &cfIPs{IPv4Cidrs: body.Result.IPv4Cidrs, IPv6Cidrs: body.Result.IPv6Cidrs}, nil
}
