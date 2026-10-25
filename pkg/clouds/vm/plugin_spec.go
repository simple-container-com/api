// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The driver table crosses the plugin bus in both directions, and the two
// conversions live next to each other on purpose.
//
// A plugin builds a VmDriverSpec in its Schema() response; core turns that spec
// back into the DriverInfo row that Validate and the capability-keyed checks read.
// Those are inverse functions over a struct with ten capability fields, written in
// two different repositories — exactly the shape that drifts. When it drifts the
// failure is silent and specific: a capability the plugin set arrives false
// core-side, so a check that should have refused the config at validation time
// passes, and the deploy fails on the cloud twenty minutes later.
//
// Keeping both here means (a) a plugin author never hand-writes the wire struct,
// and (b) TestVmDriverSpecRoundTripsEveryField can assert the pair is lossless by
// reflection, so adding a capability without extending the conversion fails a test
// in this package rather than quietly in someone else's cloud.

// ToProto renders the row as the wire spec a plugin returns from Schema().
//
// `driver` is the `provider:` value operators write in client.yaml; it is not part
// of DriverInfo because in-tree rows are keyed by it in the Drivers map.
//
// Plugin is deliberately NOT carried: it names the plugin that advertised the
// driver, which only core can know (it is the name from the parent stack's
// `plugins:` block, not something a plugin gets to claim for itself), and
// RegisterPluginDriver sets it on the way in.
func (i DriverInfo) ToProto(driver string) *scpluginv1.VmDriverSpec {
	return &scpluginv1.VmDriverSpec{
		Driver:      driver,
		Implemented: i.Implemented,
		Unverified:  i.Unverified,
		RoadmapItem: i.RoadmapItem,
		Capabilities: &scpluginv1.VmCapabilities{
			CloudInit:            i.Capabilities.CloudInit,
			CloudInitNeedsEgress: i.Capabilities.CloudInitNeedsEgress,
			Regionless:           i.Capabilities.Regionless,
			MultiAzInRegion:      int32(i.Capabilities.MultiAZInRegion),
			PrivateNetwork:       i.Capabilities.PrivateNetwork,
			FloatingIp:           i.Capabilities.FloatingIP,
			SecurityGroups:       i.Capabilities.SecurityGroups,
			BlockStorageCsi:      i.Capabilities.BlockStorageCSI,
			ContainerRegistry:    i.Capabilities.ContainerRegistry,
			DnsInIac:             i.Capabilities.DNSInIaC,
		},
	}
}

// DriverInfoFromProto converts a plugin's advertised spec into a driver-table row.
//
// Every field is taken verbatim, including `unverified` and `roadmap_item`: those
// are quoted back to the operator by Validate, and a plugin that ships a driver it
// has never run against a real project must be able to say so in its own words —
// the same claim the in-tree `cloudru-vm` row makes.
//
// Plugin is left empty; RegisterPluginDriver fills it from the plugin's own name.
func DriverInfoFromProto(spec *scpluginv1.VmDriverSpec) DriverInfo {
	c := spec.GetCapabilities()
	return DriverInfo{
		Implemented: spec.GetImplemented(),
		Unverified:  spec.GetUnverified(),
		RoadmapItem: spec.GetRoadmapItem(),
		Capabilities: Capabilities{
			CloudInit:            c.GetCloudInit(),
			CloudInitNeedsEgress: c.GetCloudInitNeedsEgress(),
			Regionless:           c.GetRegionless(),
			MultiAZInRegion:      int(c.GetMultiAzInRegion()),
			PrivateNetwork:       c.GetPrivateNetwork(),
			FloatingIP:           c.GetFloatingIp(),
			SecurityGroups:       c.GetSecurityGroups(),
			BlockStorageCSI:      c.GetBlockStorageCsi(),
			ContainerRegistry:    c.GetContainerRegistry(),
			DNSInIaC:             c.GetDnsInIac(),
		},
	}
}
