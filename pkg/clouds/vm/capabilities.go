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
	// Regionless — the cloud's API has NO region dimension: one global endpoint,
	// and placement is by availability zone alone.
	//
	// The zero value is the common case (a region is required), so only a cloud
	// that genuinely has none sets this. Cloud.ru Evolution is the first: its
	// compute API is `compute.api.cloud.ru` with no region in the host, the path or
	// any request, and placement is the `ru.AZ-*` availability zone. Requiring
	// `region:` there would be a mandatory field with nowhere to go — exactly the
	// dead config this package refuses elsewhere.
	Regionless bool
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
	// Unverified, when non-empty, says the driver's code exists and compiles but has
	// never provisioned a machine on the cloud it addresses — and the string says
	// why not. Validate refuses such a driver unless the resource sets
	// `allowUnverifiedProvider: true`, and quotes this reason verbatim.
	//
	// "Implemented" and "known to work" are different claims, and the DSL had no way
	// to tell them apart: `cloudru-vm` shipped as `Implemented: true` next to
	// `cloudru-evolution-vm`, which had been provisioned end to end — so the one
	// thing an operator could not learn from the config reference was which of the
	// two had ever run. Clear this field when a driver is smoked, in the same change
	// that records the smoke.
	Unverified string
	// RoadmapItem is the forge roadmap item that tracks this driver. It is quoted
	// verbatim in the refusal an unimplemented driver returns, so an operator who
	// hits it can find the state of the work rather than guessing.
	RoadmapItem  string
	Capabilities Capabilities
	// Plugin names the SC plugin that advertised this driver, and is empty for a
	// driver SC core ships itself. Set by RegisterPluginDriver — a row in the
	// `Drivers` literal below must never set it.
	//
	// It is what routes the resource: a non-empty Plugin means the machines are
	// created by that plugin's VmHosts RPC rather than by an in-tree driver.
	Plugin string
	// MigratedToPlugin, when true, marks a `Drivers` row that is RETAINED only
	// for its metadata (Capabilities, Unverified, RoadmapItem) because an SC
	// plugin now owns the actual provisioning. The plugin reads this row via
	// `Drivers[name].ToProto(name)` to populate its SchemaResponse.VmDrivers —
	// so removing the entry would silently zero the plugin's advertised
	// capabilities. Instead, we keep the row AND:
	//
	//   - RegisterPluginDriver allows a plugin to register the same driver
	//     name without "cannot replace a built-in" errors (plugin_drivers.go)
	//   - LookupDriver treats this row as absent and falls through to the
	//     plugin table — plugin registration becomes the source of truth
	//   - PluginForDriver returns the registering plugin's name, so dispatch
	//     routes to its VmHosts RPC instead of an in-tree driver (there is no
	//     in-tree driver — the extraction is already done; the row is pure
	//     capability metadata)
	//
	// Live-caught 2026-10-30 on a two-peaks deploy: sc-plugin-cloudru advertises
	// `cloudru-evolution-vm`, api v2026.10.54 refused the registration against
	// the then-collision-strict check, every cloudru deploy went red with
	// "plugin %q advertises vm provider %q, which SC core provides itself".
	// The data was always the plugin's; this flag just stops api claiming it.
	MigratedToPlugin bool
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
		Implemented:      true,
		MigratedToPlugin: true, // owned by sc-plugin-cloudru; see DriverInfo.MigratedToPlugin
		// Written against `simple-container-com/pulumi-sbercloud` and the Advanced
		// (ex-SberCloud) API, and never run: the only Cloud.ru account SC has ever
		// had is an **Evolution** subscription, which is a different cloud with a
		// different API behind the same brand and the same console. So every claim
		// this driver makes — the flavour lookup, `eip_id` at create, the security
		// group, the cloud-init that fetches docker — is read off a Terraform
		// provider's schema and nothing more.
		Unverified: "it addresses Cloud.ru **Advanced** (ex-SberCloud), and the only Cloud.ru estate SC has " +
			"ever provisioned is **Evolution** — if your project is Evolution (which new accounts get by " +
			"default), use `provider: cloudru-evolution-vm`, which is live-verified end to end",
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
	DriverCloudruEvolution: {
		Implemented:      true,
		MigratedToPlugin: true, // owned by sc-plugin-cloudru; see DriverInfo.MigratedToPlugin
		RoadmapItem:      "sc-cloudru-evolution-vm-driver",
		Capabilities: Capabilities{
			// No region anywhere in this cloud's compute API — see Regionless. The
			// AZ names (`ru.AZ-1`…`ru.AZ-3`) are the whole placement vocabulary.
			Regionless: true,
			// Measured, not assumed, on 2026-10-24: `POST /vms` answers
			// `extra_forbidden` to `user_data` (and to every spelling of it), because
			// the image renders a fixed template from the slots it declares in
			// `image_metadata` and nothing else reaches the first boot. So there is
			// no cloud-init on this cloud, which cascades into three things the
			// driver has to do differently from `cloudru-vm`: docker is installed
			// afterwards over SSH, the readiness gate is the byo one (preconditions,
			// not a marker file), and SC cannot pre-seed the machine's SSH host key —
			// hence trust-on-first-use and the `hostKey:` escape hatch.
			CloudInit: false,
			// Irrelevant with CloudInit false, and stated rather than left to the
			// zero value so it reads as answered: nothing has to happen before the
			// first boot, so the floating IP is attached after the machine exists.
			CloudInitNeedsEgress: false,
			// The VM lands on the availability zone's DEFAULT subnet and DEFAULT
			// security group, and that is not a simplification: `POST /vms` refuses
			// `subnet_id`, `security_group_ids`, `interfaces`, `networks`, `nics` and
			// `floating_ip` alike (measured, same run). Both are still true — there
			// IS a private network and filtering IS per-interface — SC just does not
			// get to choose them at create time. Ingress rules are therefore opened
			// on whichever group the machine actually joined, read back off its own
			// interface (see ApplyVm → ensureIngress).
			PrivateNetwork: true,
			FloatingIP:     true,
			SecurityGroups: true,
			// The project's Artifact Registry, which the Container App driver already
			// creates and pushes to with the same key pair.
			ContainerRegistry: true,
			// True since 2026-10-24: the `cloudru-dns` registrar was live-verified
			// on a delegated apex through both tiers (CLI and Pulumi), each step
			// cross-checked by resolving the name on the public internet. It had
			// been false here with a comment pointing at the roadmap item that has
			// since shipped — roadmap item sc-cloudru-dns-registrar.
			DNSInIaC: true,
			// MultiAZInRegion left at "not measured": the driver resolves zones from
			// `GET /availability-zones` at deploy time and never needs a number
			// compiled in, and a wrong one here would be branched on.
		},
	},
	DriverTimeweb: {RoadmapItem: "sc-ru-clouds-beyond-yandex"},
}
