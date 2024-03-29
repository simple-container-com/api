package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
)

type Registrar interface {
	NewRecord(ctx *sdk.Context, dnsRecord api.DnsRecord) (*api.ResourceOutput, error)
}
