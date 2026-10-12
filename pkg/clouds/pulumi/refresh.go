// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import "github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"

// refreshOptions makes a refresh run the program. Without it Pulumi refreshes
// with each provider's configuration as stored in state, so after a credential
// change (a rotated or deleted service-account key, a move to ambient
// credentials) every refresh fails to authenticate until a deploy without
// refresh has rewritten the state. Running the program gives the providers the
// configuration the stack uses now.
func refreshOptions(opts ...optrefresh.Option) []optrefresh.Option {
	return append([]optrefresh.Option{optrefresh.RunProgram(true)}, opts...)
}
