// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// Plugin-provided VM drivers.
//
// A plugin advertises its `provider:` values in SchemaResponse.vm_drivers, and SC
// core registers them here before the Pulumi engine starts. From that point a
// plugin driver is indistinguishable from a built-in one to everything that reads
// the table: Validate tells "unknown provider" from "not implemented yet" the same
// way, the refusal for an unverified driver quotes the plugin's own words, and the
// list in every error message names it.
//
// A SEPARATE map from Drivers rather than mutating that one, for three reasons:
//
//   - Drivers is read from inside the Pulumi engine (each driver's Capabilities()),
//     and a map being written while the engine runs is a data race. Registration is
//     pre-engine and single-threaded, but nothing in the type system says so.
//   - the invariant in init_test.go — every row core ships has a const and every
//     const has a row — stays checkable. Merged into one map it would silently
//     start covering whatever plugins happened to be installed on the box running
//     the test.
//   - "where did this driver come from" stays answerable, which the error messages
//     need: an operator whose `provider:` is unknown is told a different thing
//     depending on whether the driver is missing or its plugin is.
//
// Why Validate can legally see a plugin row at all: VmConfig.Validate() is called
// from the PROVISIONING tier (pkg/clouds/pulumi/vm's Vm() and provider.go), not
// from ReadVmConfig — see the comment on Validate — and plugins are spawned and
// their schemas read before the engine starts. So by the time any Validate runs,
// every loaded plugin's rows are in.
var (
	pluginDriversMu sync.RWMutex
	pluginDrivers   = map[string]DriverInfo{}
)

// RegisterPluginDriver adds a plugin-advertised driver to the table.
//
// Refuses to shadow a true built-in and refuses a second registration of the same
// name, in both cases naming the plugins involved. Both are configuration errors
// an operator can fix (uninstall one of the plugins), and neither may resolve
// silently: which plugin won would decide which cloud a machine is created in.
//
// A row flagged `MigratedToPlugin: true` is NOT treated as a true built-in — the
// row exists only for its capability metadata (see DriverInfo.MigratedToPlugin's
// doc comment on capabilities.go), and the plugin is the authoritative source of
// implementation for that driver name.
func RegisterPluginDriver(pluginName, driver string, info DriverInfo) error {
	if pluginName == "" {
		return errors.Errorf("cannot register vm driver %q: no plugin name", driver)
	}
	if driver == "" {
		return errors.Errorf("plugin %q advertised a vm driver with no name", pluginName)
	}
	if existing, builtin := Drivers[driver]; builtin && !existing.MigratedToPlugin {
		return errors.Errorf(
			"plugin %q advertises vm provider %q, which SC core provides itself; "+
				"a plugin cannot replace a built-in driver", pluginName, driver,
		)
	}

	pluginDriversMu.Lock()
	defer pluginDriversMu.Unlock()
	if existing, dup := pluginDrivers[driver]; dup {
		if existing.Plugin == pluginName {
			// Same plugin, same row: a second Schema() read of the same plugin. Not
			// an error — the plugin is allowed to be asked twice.
			return nil
		}
		return errors.Errorf(
			"vm provider %q is advertised by two plugins (%q and %q); uninstall one of them",
			driver, existing.Plugin, pluginName,
		)
	}
	info.Plugin = pluginName
	pluginDrivers[driver] = info
	return nil
}

// LookupDriver resolves a `provider:` value against the built-in table first and
// the plugin table second. The ordering is the no-shadowing rule above, enforced a
// second time at read: a built-in always wins — except a `MigratedToPlugin: true`
// row, which is capability metadata rather than an implementation. For those we
// fall through to the plugin table, so a registered plugin driver with the same
// name wins. If no plugin has registered, the row's metadata (Capabilities,
// Unverified, RoadmapItem) still answers so Validate's keyed-off-capabilities
// checks keep working.
func LookupDriver(driver string) (DriverInfo, bool) {
	if info, ok := Drivers[driver]; ok && !info.MigratedToPlugin {
		return info, true
	}
	pluginDriversMu.RLock()
	defer pluginDriversMu.RUnlock()
	if info, ok := pluginDrivers[driver]; ok {
		return info, true
	}
	// No plugin registered a migrated driver — serve the stale built-in metadata
	// so Validate still has a row to reason about (an operator deploying without
	// the plugin pin would otherwise read "unknown provider" instead of the real
	// state, which is "the plugin is not installed").
	if info, ok := Drivers[driver]; ok {
		return info, true
	}
	return DriverInfo{}, false
}

// PluginForDriver names the plugin that provides a driver, or "" for a built-in.
// This is what routes a vm resource to the VmHosts RPC instead of an in-tree
// driver (pkg/clouds/pulumi/vm's driverFor). A `MigratedToPlugin: true` built-in
// row is NOT a true built-in — defer to the plugin table so dispatch routes to
// the registered plugin's RPC.
func PluginForDriver(driver string) string {
	if info, builtin := Drivers[driver]; builtin && !info.MigratedToPlugin {
		return ""
	}
	pluginDriversMu.RLock()
	defer pluginDriversMu.RUnlock()
	return pluginDrivers[driver].Plugin
}

// ResetPluginDrivers drops every registered plugin driver. For tests, and for a
// second `sc` operation in the same process after the plugins were torn down.
func ResetPluginDrivers() {
	pluginDriversMu.Lock()
	defer pluginDriversMu.Unlock()
	pluginDrivers = map[string]DriverInfo{}
}

// pluginDriverNames lists the registered plugin drivers. Unsorted; KnownDrivers
// sorts the union.
func pluginDriverNames() []string {
	pluginDriversMu.RLock()
	defer pluginDriversMu.RUnlock()
	return lo.Keys(pluginDrivers)
}
