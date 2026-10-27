// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

// Gibibytes parses one of SC's DSL size quantities ("4Gi", "60Gi") into a whole
// number of GiB. Every VM-plugin needs this: cloud APIs that take disk / memory
// arguments in integer GiB are the common case, and SC's own schema spells the
// value as a human quantity string.
//
// Refuses a fractional value rather than rounding. Rounding a flavour-filter
// down matches nothing — reads to an operator as "the region has no such
// machine". Rounding a disk size silently gives the operator a disk they did
// not ask for. Neither is a useful failure mode.
//
// `field` is the server.yaml field name to name in the error ("size.disk",
// "size.memory").
func Gibibytes(quantity, field string) (int, error) {
	bytes, err := humanize.ParseBytes(quantity)
	if err != nil {
		return 0, errors.Wrapf(err, "`%s` is not a size: %q (expected e.g. `4Gi`)", field, quantity)
	}
	const gib = 1 << 30
	if bytes == 0 || bytes%gib != 0 {
		return 0, errors.Errorf("`%s` must be a whole number of GiB, got %q", field, quantity)
	}
	return int(bytes / gib), nil
}
