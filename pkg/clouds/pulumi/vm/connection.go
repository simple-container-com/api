// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// dialErrorLimit and perDialTimeoutSeconds are set explicitly because the SDK
	// defaults (10 attempts, 15s each) are sized for a machine that is already
	// running, not for one created a second ago. 60 × 10s covers a cold boot plus a
	// cloud-init docker install without turning a genuinely unreachable host into a
	// ten-minute hang: a wrong security group fails on the first dial either way.
	dialErrorLimit        = 60
	perDialTimeoutSeconds = 10
)

// connectionArgs builds the SSH connection to one host.
//
// Single place on purpose: every remote.Command in this package and in the ones
// above it (compose, k3s) goes through here, so the secret marking and the dial
// tuning cannot be forgotten at one call site.
func connectionArgs(h HostFacts) *remote.ConnectionArgs {
	args := &remote.ConnectionArgs{
		Host: h.Address,
		User: sdk.StringPtr(h.User),
		Port: sdk.Float64Ptr(float64(h.Port)),
		// Already secret-wrapped by the driver (HostFacts.PrivateKey). Passing a raw
		// string here is a bug, not a style choice.
		PrivateKey:         h.PrivateKey.ToStringPtrOutput(),
		PrivateKeyPassword: h.PrivateKeyPassword.ToStringPtrOutput(),
		DialErrorLimit:     sdk.IntPtr(dialErrorLimit),
		PerDialTimeout:     sdk.IntPtr(perDialTimeoutSeconds),
	}
	// Set only when pinned: the SDK's contract is "if not provided, the host key will
	// be ignored", so passing an empty string risks being read as "provided, and it is
	// the empty key" — a failure on every connection rather than the documented
	// fallback. Unset is insecure and warned about once, at provision time; it is not
	// hidden behind a connection that silently fails.
	if h.HostKey != "" {
		args.HostKey = sdk.StringPtr(h.HostKey)
	}
	return args
}
