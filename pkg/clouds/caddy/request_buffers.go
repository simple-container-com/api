// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package caddy

import (
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

// DefaultRequestBufferBytes is the `request_buffers` size every SC-generated
// reverse_proxy block gets unless the stack overrides it;
// MaxRequestBufferBytes caps the override because the buffer is held in the
// edge's memory per in-flight request.
//
// The contract is documented on api.SimpleContainerLBConfig.RequestBufferSize.
const (
	DefaultRequestBufferBytes = 1 << 20  // 1MiB
	MaxRequestBufferBytes     = 16 << 20 // 16MiB
)

// RequestBufferBytes resolves lbConfig.requestBufferSize to a byte count.
//
// Shared by both ingress paths — the Kubernetes edge and the single-VM Caddy —
// for the same reason the assets are: the value decides whether a chunked
// upload reaches the application with a Content-Length (Django #28668), so a
// path that parsed it differently would present as "file upload works in
// Kubernetes and not on the VM". The error strings are part of that: an
// operator who hits the cap should read the same sentence wherever they are.
//
// An empty value yields the default. A parsed 0 yields 0, which both callers
// render as "omit the directive entirely" — the documented escape hatch for an
// edge running Caddy < 2.6.0, which predates `request_buffers`.
func RequestBufferBytes(v string) (uint64, error) {
	if v == "" {
		return DefaultRequestBufferBytes, nil
	}
	n, err := humanize.ParseBytes(v)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid lbConfig.requestBufferSize %q", v)
	}
	if n > MaxRequestBufferBytes {
		return 0, errors.Errorf(
			"lbConfig.requestBufferSize %q exceeds the %s cap (buffered in shared edge memory per in-flight request)",
			v, humanize.IBytes(MaxRequestBufferBytes))
	}
	return n, nil
}
