// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"github.com/pkg/errors"
)

// errPluginConverterNotRegistered is returned by convertPluginOwnedTemplate
// when a template type is plugin-owned but no PluginStackConverter has
// been installed. The two most likely causes are: (a) the caller used
// api.ReadServerDescriptor instead of stackread.ReadServerDescriptor,
// so the two-pass discovery never ran; (b) a test harness that
// deliberately bypasses stackread. In production the caller is
// stackread, which registers itself at init-time.
func errPluginConverterNotRegistered(typeName, pluginName string) error {
	return errors.Errorf(
		"template type %q is owned by plugin %q but no plugin dialler is registered — "+
			"use stackread.ReadServerDescriptor (or call api.RegisterPluginStackConverter at init-time)",
		typeName, pluginName,
	)
}
