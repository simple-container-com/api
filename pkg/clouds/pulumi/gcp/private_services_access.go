// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/servicenetworking"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

const serviceNetworkingService = "servicenetworking.googleapis.com"

// privateServicesAccess reserves the configured range on the private network and
// peers it to Service Networking, which Cloud SQL needs before an instance can
// take a private IP on that network.
func privateServicesAccess(ctx *sdk.Context, pgCfg *gcloud.PostgresGcpCloudsqlConfig, postgresName string, params pApi.ProvisionParams) (*servicenetworking.Connection, error) {
	prefix, err := pgCfg.PrivateServicesAccessPrefix()
	if err != nil {
		return nil, err
	}
	apiName := fmt.Sprintf("projects/%s/services/%s", pgCfg.ProjectId, serviceNetworkingService)
	if err := enableServicesAPI(ctx.Context(), pgCfg, apiName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", apiName)
	}

	ones, _ := prefix.Mask.Size()
	rangeName := fmt.Sprintf("%s-psa", postgresName)
	reserved, err := compute.NewGlobalAddress(ctx, rangeName, &compute.GlobalAddressArgs{
		Name:         sdk.String(rangeName),
		Purpose:      sdk.String("VPC_PEERING"),
		AddressType:  sdk.String("INTERNAL"),
		Address:      sdk.String(prefix.IP.String()),
		PrefixLength: sdk.Int(ones),
		Network:      sdk.String(lo.FromPtr(pgCfg.PrivateNetwork)),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reserve Private Services Access range %q", rangeName)
	}

	conn, err := servicenetworking.NewConnection(ctx, fmt.Sprintf("%s-psa-connection", postgresName), &servicenetworking.ConnectionArgs{
		Network:               sdk.String(lo.FromPtr(pgCfg.PrivateNetwork)),
		Service:               sdk.String(serviceNetworkingService),
		ReservedPeeringRanges: sdk.StringArray{reserved.Name},
		// Cloud SQL holds the peering for days after its last instance is
		// deleted, so removing the connection on destroy fails; abandon it.
		DeletionPolicy: sdk.String("ABANDON"),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create Private Services Access connection for %q", postgresName)
	}
	return conn, nil
}
