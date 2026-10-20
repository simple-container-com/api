// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

// Capabilities describes what a cloud physically does, so that the differences
// between clouds live in exactly one table instead of becoming `if provider == …`
// branches scattered through shared code.
//
// It is also what turns "you asked for three zones, this cloud has one in that
// region" into a validation error instead of a failure twenty minutes into a deploy
// (vm-matrix.md §1.1, D4).
type Capabilities struct {
	// CloudInit — user-data is executed by cloud-init on first boot.
	CloudInit bool
	// CloudInitNeedsEgress — cloud-init needs a public address AT CREATION TIME,
	// which forces the floating IP to be created before the machine (Timeweb).
	CloudInitNeedsEgress bool
	// MultiAZInRegion is how many availability zones are usable in one region.
	// Zero means "not measured yet" — do not validate against it until a driver
	// fills it in from the cloud's own API.
	MultiAZInRegion int
	// PrivateNetwork — instances can talk over a private network.
	PrivateNetwork bool
	// FloatingIP — a public address is a separate resource rather than something
	// the instance hands out.
	FloatingIP bool
	// SecurityGroups — filtering is per-instance rather than one global firewall.
	SecurityGroups bool
	// BlockStorageCSI — there is a CSI driver for Kubernetes volumes.
	BlockStorageCSI bool
	// ContainerRegistry — the cloud offers a private registry manageable from IaC.
	// False forces image delivery over SSH instead of push/pull.
	ContainerRegistry bool
	// DNSInIaC — public DNS zones are managed by the same provider, so SC can be
	// its own registrar instead of depending on Cloudflare.
	DNSInIaC bool
}

// DriverInfo is one row of the driver table: whether the driver exists today, where
// the work to add it is tracked, and what its cloud can do.
type DriverInfo struct {
	Implemented bool
	// RoadmapItem is the forge roadmap item that tracks this driver. It is quoted
	// verbatim in the refusal an unimplemented driver returns, so an operator who
	// hits it can find the state of the work rather than guessing.
	RoadmapItem  string
	Capabilities Capabilities
}

// Drivers is the whole driver table.
//
// Only `byo` carries real capability values. The cloud rows are intentionally left
// zeroed: vm-matrix.md §2 verified a great deal about each cloud's *resources*, but
// several of these fields (MultiAZInRegion above all) are marked "не проверено"
// there, and a table of half-guesses that shared code branches on is worse than an
// empty one. Each driver fills its own row when it lands, from the cloud's API.
var Drivers = map[string]DriverInfo{
	DriverByo: {
		Implemented: true,
		RoadmapItem: "sc-vm-compose-single-vm",
		Capabilities: Capabilities{
			// A machine the operator already created has already booted: there is
			// no user-data to run, and readiness is a precondition check rather
			// than a wait (see pkg/clouds/pulumi/vm/ready.go).
			CloudInit:       false,
			MultiAZInRegion: 1,
			// Everything below is a property of a cloud SC is driving. With byo
			// there is no cloud: SC cannot create a network, hand out an address,
			// write a firewall rule, provision a volume, stand up a registry or
			// edit a DNS zone. Hence false, and hence `ssh-tar` image delivery.
		},
	},
	DriverYandex:    {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
	DriverOpenstack: {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
	DriverVkcs:      {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
	DriverMws:       {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
	DriverCloudru: {
		Implemented: true,
		RoadmapItem: "sc-ru-clouds-beyond-yandex",
		Capabilities: Capabilities{
			CloudInit: true,
			// The image has no docker on it, so cloud-init fetches get.docker.com —
			// from inside Cloud.ru, over the EIP. Which is why the driver binds the
			// EIP at instance CREATION (`eip_id`, ForceNew) rather than attaching it
			// afterwards: an instance that boots without an address runs cloud-init
			// to completion with no egress and fails the docker install, and the
			// readiness gate then waits out its full ten minutes on a machine that
			// will never become ready.
			CloudInitNeedsEgress: true,
			PrivateNetwork:       true,
			FloatingIP:           true,
			SecurityGroups:       true,
			// `sbercloud_swr_organization` / `_repository` exist in the bridged
			// provider, so a per-stack registry inside Cloud.ru is reachable from
			// IaC. Not yet used: image delivery on this path is `ssh-tar`.
			ContainerRegistry: true,
			// `sbercloud_dns_zone` + `_recordset` exist, so the ingress A record can
			// be managed in the same stack as the machine.
			DNSInIaC: true,
			// Deliberately left at the zero value, which this file defines as "not
			// measured yet" rather than "absent":
			//
			//   - MultiAZInRegion: the driver resolves zones from the cloud's own
			//     `availability_zones` datasource at deploy time, so it never needs
			//     a number compiled in — and a wrong one here would be branched on.
			//   - BlockStorageCSI: `evs_volume`/`evs_snapshot` are bridged, but
			//     whether a k3s cluster on Cloud.ru can run the EVS CSI driver is a
			//     k3s question (track K), unasked and therefore unanswered.
		},
	},
	DriverTimeweb: {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
}
