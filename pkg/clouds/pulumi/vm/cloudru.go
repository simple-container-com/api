// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-tls/sdk/v5/go/tls"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
	sdkSbercloud "github.com/simple-container-com/pulumi-sbercloud/sdk/go/sbercloud"
)

// The address plan SC uses when it creates the network itself.
//
// Deliberately not 10.0.0.0/16 or 192.168.0.0/16: the only thing this range has to do
// is not collide, and the obvious ranges are exactly the ones an operator's existing
// network, VPN or VPC peering already uses. Pin an existing network with
// `network.id` + `network.subnets` if this one is wrong for the account.
const (
	cloudruVpcCidr       = "10.47.0.0/16"
	cloudruSubnetCidr    = "10.47.1.0/24"
	cloudruSubnetGateway = "10.47.1.1"
)

const (
	// cloudruEipType is Cloud.ru's dynamic BGP address pool, the one a plain account
	// has. "5_sbgp" is the single-line BGP pool and is not available everywhere.
	cloudruEipType = "5_bgp"
	// cloudruEipBandwidth is a dedicated 100 Mbit/s pipe billed BY TRAFFIC, not by
	// width: with `chargeMode: traffic` the number is a ceiling rather than a
	// reservation, so a generous one costs nothing while the machine is idle and does
	// not throttle an image ship. Range accepted by the API is 1–300.
	cloudruEipBandwidthMbit = 100
	// cloudruFlavorPerformanceType is the general-purpose flavour family. Without a
	// filter the flavours datasource answers with every shape in the region, and the
	// one SC would pick is whichever the API listed first.
	cloudruFlavorPerformanceType = "normal"
)

// cloudruUUID recognises an `image:` that is already a Cloud.ru image id, so it can be
// used verbatim instead of being looked up by name.
var cloudruUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// cloudruImageAliases maps SC's cloud-independent image aliases onto a regex over
// Cloud.ru's public image names, which look like "Ubuntu 24.04 server 64bit".
//
// A regex and not an exact name on purpose: the public images are rebuilt and the
// name carries the build, so an exact match would break on Cloud.ru's schedule rather
// than ours. An alias that matches nothing fails at provision time with the regex in
// the message, and `image:` also accepts an exact name or an image id as the escape
// hatch.
var cloudruImageAliases = map[string]string{
	"ubuntu-2404-lts": `(?i)^ubuntu 24\.04`,
	"ubuntu-2204-lts": `(?i)^ubuntu 22\.04`,
}

// cloudruDriver creates machines in Cloud.ru Advanced (ex-SberCloud), through the
// bridged provider simple-container-com/pulumi-sbercloud.
//
// Why Advanced and not Evolution: Advanced's Terraform provider has open Go sources
// (sbercloud-terraform/terraform-provider-sbercloud, MPL-2.0), so it bridges into
// Pulumi the classic way — fork, tfgen, release the plugin with a PluginDownloadURL
// on our own host, exactly the pulumi-yandex recipe. That matters beyond convenience:
// `pulumi package add terraform-provider …` resolves through registry.opentofu.org,
// which is geo-blocked from Russia, so the dynamic bridge is not an option for the
// market this driver exists to serve. Evolution ships its provider as a binary only
// and is a separate, unstarted piece of work.
//
// Seven cloud primitives are created here and NONE of them is in the Driver
// interface: VPC, subnet, security group, its rules, an SSH keypair, the floating
// address, the instance. That is the point of the seam — the layers above
// (readiness, image ship, compose, Caddy, k3s) never learn that Cloud.ru calls a
// floating address an EIP or that its subnet carries the gateway and the resolvers.
//
// Design: forge/docs/design/2026-10-13/sc-russia-market-readiness/vm-matrix.md (R4,
// and the 2026-10-15 amendment to §6).
type cloudruDriver struct{}

func (cloudruDriver) Name() string { return vm.DriverCloudru }

func (cloudruDriver) Capabilities() vm.Capabilities {
	return vm.Drivers[vm.DriverCloudru].Capabilities
}

func (d cloudruDriver) Hosts(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	ports []vm.PortRange,
	params pApi.ProvisionParams,
) ([]HostFacts, error) {
	// Every Cloud.ru resource and datasource below is given this provider
	// EXPLICITLY, and nothing else is. params.Provider is the sbercloud provider for
	// this driver, while the layers above build remote.Command and tls.PrivateKey
	// resources that must keep using their own ambient defaults — a provider option
	// whose package does not match the resource is ignored, but passing it as a
	// package-wide default would be a lie about what it is.
	//
	// sdk.Provider returns a ResourceOrInvokeOption, which is why one value seeds
	// both lists.
	useProvider := sdk.Provider(params.Provider)
	resOpts := []sdk.ResourceOption{useProvider}
	invOpts := []sdk.InvokeOption{useProvider}

	name := input.Descriptor.Name
	region := cfg.Region

	zone, err := d.resolveZone(ctx, cfg, region, invOpts)
	if err != nil {
		return nil, err
	}
	imageID, err := d.resolveImage(ctx, cfg, region, invOpts)
	if err != nil {
		return nil, err
	}
	flavorID, err := d.resolveFlavor(ctx, cfg, region, zone, invOpts)
	if err != nil {
		return nil, err
	}
	params.Log.Info(ctx.Context(), "vm %q resolved Cloud.ru placement: region %q zone %q image %q flavor %q",
		name, region, zone, imageID, flavorID)

	subnetID, netDeps, err := d.network(ctx, input, cfg, region, zone, resOpts)
	if err != nil {
		return nil, err
	}

	secGroupID, secDeps, err := d.securityGroup(ctx, input, ports, region, resOpts)
	if err != nil {
		return nil, err
	}

	hostName := fmt.Sprintf("%s-host-0", name)

	// The deploy key is a STACK RESOURCE, not an operator secret (vm-matrix.md §3.3,
	// D10). SC generates it, keeps the private half in the checkpoint marked secret,
	// publishes the public half into cloud-init, and rotates it by recreating this
	// resource. An operator never sees it and never has to store it — which is also
	// why `hosts[].privateKey` has no counterpart on a cloud driver.
	deployKey, err := tls.NewPrivateKey(ctx, input.ToResName(name+"-deploy-key"), &tls.PrivateKeyArgs{
		Algorithm: sdk.String("ED25519"),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create deploy key for vm %q", name)
	}

	// The machine's own SSH host key, generated here for one reason: so that the very
	// first connection can VERIFY it. Without this there is no moment at which SC
	// knows the identity of a machine it just created — `remote.Connection` would
	// have to connect with host-key checking off, and the deploy key plus every
	// secret of every stack deployed onto it would go to whoever answered on that
	// address.
	//
	// A resource, not a value computed in Go, because it has to survive to the next
	// deploy: a key minted per run changes user-data, and user-data is an input of
	// the instance.
	hostKey, err := tls.NewPrivateKey(ctx, input.ToResName(hostName+"-host-key"), &tls.PrivateKeyArgs{
		Algorithm: sdk.String("ED25519"),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create host key for vm %q", name)
	}

	// Cloud.ru wants the deploy key registered as a keypair to attach it to the
	// instance. `keyFile` is deliberately NOT set: it makes the provider write the
	// private half as a .pem next to wherever the deploy ran, which on a CI runner is
	// an artifact directory. SC holds the private half in the checkpoint instead.
	keypair, err := sdkSbercloud.NewComputeKeypair(ctx, input.ToResName(name+"-keypair"), &sdkSbercloud.ComputeKeypairArgs{
		Region: sdk.StringPtr(region),
		PublicKey: deployKey.PublicKeyOpenssh.ApplyT(func(k string) string {
			return strings.TrimSpace(k)
		}).(sdk.StringOutput),
	}, resOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create keypair for vm %q", name)
	}

	// cloud-init, rendered from the two key resources. ApplyT is the only one in this
	// driver — zone, image and flavour all resolve to plain strings at graph-build
	// time through the synchronous datasource variants — and it exists because key
	// material is only knowable after its resource is created.
	//
	// Secretness propagates through ApplyT, so the host key's private half makes the
	// whole user-data document secret. AdditionalSecretOutputs below covers the
	// instance's echo of it.
	userData := sdk.All(deployKey.PublicKeyOpenssh, hostKey.PrivateKeyOpenssh, hostKey.PublicKeyOpenssh).
		ApplyT(func(vals []any) (string, error) {
			return vm.RenderCloudInit(cfg, vm.RoleCompose, vm.HostKeys{
				DeployPublicKey: vals[0].(string),
				HostPrivateKey:  vals[1].(string),
				HostPublicKey:   vals[2].(string),
			})
		}).(sdk.StringOutput)

	eip, err := d.floatingIP(ctx, input, hostName, region, resOpts)
	if err != nil {
		return nil, err
	}

	instanceArgs := &sdkSbercloud.ComputeInstanceArgs{
		Region:           sdk.StringPtr(region),
		AvailabilityZone: sdk.StringPtr(zone),
		ImageId:          sdk.StringPtr(imageID),
		FlavorId:         sdk.StringPtr(flavorID),
		KeyPair:          keypair.Name,
		SecurityGroupIds: sdk.StringArray{secGroupID},
		Networks: sdkSbercloud.ComputeInstanceNetworkArray{
			sdkSbercloud.ComputeInstanceNetworkArgs{Uuid: subnetID},
		},
		UserData: userData.ToStringPtrOutput(),
		// The address is bound AT CREATION rather than attached afterwards, which is
		// what `eip_id` (ForceNew) does. Non-negotiable here: cloud-init installs
		// docker from get.docker.com, so an instance that boots without egress runs
		// its user-data to completion having failed, and the readiness gate then sits
		// out its full ten minutes waiting for a marker that will never be written.
		EipId: eip.ID().ToStringPtrOutput(),
		// Defaults to TRUE upstream, which would make the instance delete an address
		// Pulumi owns — leaving a resource in the checkpoint that no longer exists and
		// a destroy that errors on it.
		DeleteEipOnTermination: sdk.BoolPtr(false),
		// A stop before the delete, so the root volume is detached cleanly rather than
		// yanked from a running kernel.
		StopBeforeDestroy: sdk.BoolPtr(true),
		// userData is secret (see above) but the provider also returns it, and
		// adminPass/privateKey are credentials the cloud may fill in on its own.
		// Without this they are stored in the checkpoint in the clear and printed in
		// an update summary — the exact mechanism by which a Yandex service-account
		// private key reached a world-readable Actions log on 2026-09-30.
	}
	// Both left unset when not configured: `systemDiskSize` and `systemDiskType` are
	// Optional+Computed upstream, so omitting them takes the image's own root disk and
	// the account's default volume family (GPSSD today). An SC-invented default here
	// would instead be a guess about an account's quota.
	size := cfg.EffectiveSize()
	if size.Disk != "" {
		disk, err := gibibytes(size.Disk, "size.disk")
		if err != nil {
			return nil, errors.Wrapf(err, "invalid `size.disk` of vm %q", name)
		}
		instanceArgs.SystemDiskSize = sdk.IntPtr(disk)
	}
	if size.DiskType != "" {
		instanceArgs.SystemDiskType = sdk.StringPtr(size.DiskType)
	}

	instDeps := append(append([]sdk.Resource{keypair}, netDeps...), secDeps...)
	instance, err := sdkSbercloud.NewComputeInstance(ctx, input.ToResName(hostName), instanceArgs,
		append(resOpts,
			sdk.DependsOn(instDeps),
			sdk.AdditionalSecretOutputs([]string{"userData", "adminPass", "privateKey"}),
			// user_data is NOT ForceNew in the upstream resource (huaweicloud v1.88
			// updates it through an ECS API call), so changing cloud-init on a running
			// machine reports GREEN while the booted machine stays exactly as it was —
			// user-data runs once, at first boot. Replacing on change is the only
			// honest answer: the machine SC describes and the machine that exists must
			// not be allowed to diverge silently. Deploy-time cost is a new instance,
			// which is also why cloud-init is kept to "make this machine deployable"
			// and everything else is a remote.Command.
			sdk.ReplaceOnChanges([]string{"userData"}),
		)...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create instance for vm %q", name)
	}

	return []HostFacts{{
		Name:    hostName,
		Address: eip.Address,
		User:    cfg.EffectiveSshUser(),
		// cloud-init leaves sshd on its default port; there is no reason for SC to
		// move it and a non-default port would have to be carried into the security
		// group, the readiness gate and every stack that deploys here.
		Port: vm.DefaultSshPort,
		// Marked secret at the single point where it enters Pulumi, as HostFacts
		// requires. tls.PrivateKey already marks its private outputs secret; ToSecret
		// is idempotent and makes the guarantee local to this line rather than a
		// property of a provider somebody may swap.
		PrivateKey: sdk.ToSecret(deployKey.PrivateKeyOpenssh).(sdk.StringOutput),
		// Empty: an SC-generated key is never an encrypted PEM. Still secret-wrapped
		// and still exported, because the consumer cannot tell which driver produced
		// the host.
		PrivateKeyPassword: secretString(""),
		// Trailing newline trimmed: this value is compared against an
		// authorized_keys-style line by ssh.ParseAuthorizedKey, and PublicKeyOpenssh
		// comes with one.
		HostKey: hostKey.PublicKeyOpenssh.ApplyT(func(k string) string {
			return strings.TrimSpace(k)
		}).(sdk.StringOutput),
		// Always true on this driver, and that is the dividend of generating the host
		// key above: unlike byo, where the operator may simply not have pasted one,
		// SC created this machine and therefore created its identity.
		HostKeyPinned: true,
		Role:          vm.RoleCompose,
		DependsOn:     []sdk.Resource{instance},
	}}, nil
}

// resolveZone picks the availability zone, asking the cloud when the config did not
// say. Validate has already refused more than one.
func (cloudruDriver) resolveZone(ctx *sdk.Context, cfg *vm.VmConfig, region string, invOpts []sdk.InvokeOption) (string, error) {
	if len(cfg.Zones) > 0 && cfg.Zones[0] != "" {
		return cfg.Zones[0], nil
	}
	azs, err := sdkSbercloud.GetAvailabilityZones(ctx, &sdkSbercloud.GetAvailabilityZonesArgs{
		Region: sdk.StringRef(region),
	}, invOpts...)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list availability zones in Cloud.ru region %q", region)
	}
	if len(azs.Names) == 0 {
		return "", errors.Errorf("Cloud.ru region %q has no available availability zone", region)
	}
	// The datasource documents its result as ordered alphanumerically, so the first
	// entry is stable across runs — which is the whole requirement. An unstable choice
	// here would move the machine between zones on a later deploy.
	return azs.Names[0], nil
}

// resolveImage turns SC's `image:` into a Cloud.ru image id.
func (cloudruDriver) resolveImage(ctx *sdk.Context, cfg *vm.VmConfig, region string, invOpts []sdk.InvokeOption) (string, error) {
	image := cfg.EffectiveImage()
	if cloudruUUID.MatchString(image) {
		return image, nil
	}

	args := &sdkSbercloud.LookupImagesImageArgs{
		Region: sdk.StringRef(region),
		// "public" is translated by the provider into the `__imagetype=gold` filter
		// that Cloud.ru's own marketplace images carry.
		Visibility: sdk.StringRef("public"),
		MostRecent: sdk.BoolRef(true),
	}
	if pattern, isAlias := cloudruImageAliases[image]; isAlias {
		args.NameRegex = sdk.StringRef(pattern)
		args.Os = sdk.StringRef("Ubuntu")
	} else {
		// Not an alias and not an id: taken as the exact image name as it appears in
		// the console. `name` and `nameRegex` conflict, so only one is ever set.
		args.Name = sdk.StringRef(image)
	}

	found, err := sdkSbercloud.LookupImagesImage(ctx, args, invOpts...)
	if err != nil {
		return "", errors.Wrapf(err,
			"failed to resolve image %q in Cloud.ru region %q; `image:` takes an SC alias (%s), an exact "+
				"image name, or an image id", image, region, strings.Join(cloudruImageAliasNames(), ", "))
	}
	return found.Id, nil
}

func cloudruImageAliasNames() []string {
	names := make([]string, 0, len(cloudruImageAliases))
	for alias := range cloudruImageAliases {
		names = append(names, alias)
	}
	sort.Strings(names)
	return names
}

// resolveFlavor turns `size.cores` + `size.memory` into a Cloud.ru flavour id.
func (cloudruDriver) resolveFlavor(ctx *sdk.Context, cfg *vm.VmConfig, region, zone string, invOpts []sdk.InvokeOption) (string, error) {
	size := cfg.EffectiveSize()
	memGiB, err := gibibytes(size.Memory, "size.memory")
	if err != nil {
		return "", err
	}

	flavors, err := sdkSbercloud.GetComputeFlavors(ctx, &sdkSbercloud.GetComputeFlavorsArgs{
		Region:           sdk.StringRef(region),
		AvailabilityZone: sdk.StringRef(zone),
		PerformanceType:  sdk.StringRef(cloudruFlavorPerformanceType),
		CpuCoreCount:     sdk.IntRef(size.Cores),
		// The datasource's `memorySize` is in GB and matched EXACTLY — the provider
		// compares `memorySize * 1024` against the flavour's RAM in MB. So this is a
		// whole number of GiB, which is why gibibytes refuses a fractional one rather
		// than rounding and silently matching nothing.
		MemorySize: sdk.IntRef(memGiB),
	}, invOpts...)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list Cloud.ru flavors in %q/%q", region, zone)
	}
	if len(flavors.Ids) == 0 {
		return "", errors.Errorf(
			"no Cloud.ru flavor in %q/%q has %d cores and %s memory (performance type %q); pick a shape the "+
				"region offers under `size`", region, zone, size.Cores, size.Memory, cloudruFlavorPerformanceType)
	}
	// Sorted because the datasource's `ids` are not: an unsorted list means a later
	// deploy can pick a different flavour from the same filter and REPLACE the
	// machine, which on this path is a rebuild of the host every stack is on.
	ids := make([]string, len(flavors.Ids))
	copy(ids, flavors.Ids)
	sort.Strings(ids)
	return ids[0], nil
}

// network either reuses the operator's VPC or creates SC's own. Returns the subnet to
// place the machine in, plus the resources the instance must wait for.
func (cloudruDriver) network(ctx *sdk.Context, input api.ResourceInput, cfg *vm.VmConfig, region, zone string, resOpts []sdk.ResourceOption) (sdk.StringPtrInput, []sdk.Resource, error) {
	name := input.Descriptor.Name

	if cfg.Network != nil && cfg.Network.ID != "" {
		// Validate has already refused `network.id` without subnets, so the only
		// failure left is a subnet map that does not cover the zone the machine landed
		// in — which is only knowable here, after the zone was resolved.
		subnet, ok := cfg.Network.Subnets[zone]
		if !ok || subnet == "" {
			return nil, nil, errors.Errorf(
				"vm %q places its machine in zone %q, but `network.subnets` has no entry for it (has: %s); "+
					"add the subnet, or pin the zone with `zones: [<zone>]`",
				name, zone, strings.Join(sortedKeys(cfg.Network.Subnets), ", "))
		}
		return sdk.StringPtr(subnet), nil, nil
	}

	vpc, err := sdkSbercloud.NewVpc(ctx, input.ToResName(name+"-vpc"), &sdkSbercloud.VpcArgs{
		Region: sdk.StringPtr(region),
		Cidr:   sdk.String(cloudruVpcCidr),
	}, resOpts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create VPC for vm %q", name)
	}

	// DNS is deliberately left unset. The provider fills in the region's own
	// resolvers when `primaryDns` is absent, and those are reachable from inside the
	// VPC — whereas a public resolver written in here would be a bet on egress that
	// cloud-init needs before it has finished setting up.
	subnet, err := sdkSbercloud.NewVpcSubnet(ctx, input.ToResName(name+"-subnet"), &sdkSbercloud.VpcSubnetArgs{
		Region:           sdk.StringPtr(region),
		VpcId:            vpc.ID(),
		Cidr:             sdk.String(cloudruSubnetCidr),
		GatewayIp:        sdk.String(cloudruSubnetGateway),
		AvailabilityZone: sdk.StringPtr(zone),
	}, resOpts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create subnet for vm %q", name)
	}

	// The subnet id, not the VPC id: Cloud.ru's instance `networks[].uuid` is the
	// NETWORK the NIC attaches to, which in its vocabulary is the subnet.
	return subnet.ID().ToStringPtrOutput(), []sdk.Resource{vpc, subnet}, nil
}

// securityGroup opens exactly the ports SC is going to use.
//
// The port list is resolved before any driver runs (firewall.go) and shared with the
// Evolution driver: which holes a machine needs is a property of what SC deploys
// onto it — the ingress, a k3s cluster's NodePort range, a stack's host port — and
// not of the cloud whose firewall ends up holding them.
func (cloudruDriver) securityGroup(
	ctx *sdk.Context,
	input api.ResourceInput,
	ports []vm.PortRange,
	region string,
	resOpts []sdk.ResourceOption,
) (sdk.StringInput, []sdk.Resource, error) {
	name := input.Descriptor.Name

	// `deleteDefaultRules` is deliberately left at its default (false), i.e. the
	// group keeps the two default EGRESS rules. Deleting them is the tidy-looking
	// choice and it breaks this driver outright: cloud-init fetches docker from
	// get.docker.com, so a machine with no egress runs its user-data to a failure and
	// the readiness gate waits ten minutes for a marker that never arrives.
	group, err := sdkSbercloud.NewNetworkingSecgroup(ctx, input.ToResName(name+"-secgroup"), &sdkSbercloud.NetworkingSecgroupArgs{
		Region:      sdk.StringPtr(region),
		Description: sdk.StringPtr(fmt.Sprintf("Simple Container vm %q", name)),
	}, resOpts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create security group for vm %q", name)
	}

	deps := []sdk.Resource{group}
	// Already deduped and ordered by label (vm.VmConfig.FirewallPorts), which is what
	// makes the resource names below stable across deploys.
	for _, p := range ports {
		args := &sdkSbercloud.NetworkingSecgroupRuleArgs{
			Region:          sdk.StringPtr(region),
			SecurityGroupId: group.ID(),
			Direction:       sdk.String("ingress"),
			Ethertype:       sdk.String("IPv4"),
			Protocol:        sdk.StringPtr(p.Protocol),
			PortRangeMin:    sdk.IntPtr(p.From),
			PortRangeMax:    sdk.IntPtr(p.To),
		}
		if p.Internal {
			// Node-to-node traffic: reachable from the other machines of this
			// resource and from nothing else. `RemoteGroupId` pointing at the group
			// itself is the whole mechanism — these ports (the kubelet, flannel's
			// VXLAN, embedded etcd) hand out cluster-admin-equivalent access to
			// anyone who can reach them, so a CIDR would have to be the private
			// subnet's and a security group that every machine of the cluster joins
			// is both narrower and known here.
			args.RemoteGroupId = group.ID()
		} else {
			args.RemoteIpPrefix = sdk.StringPtr("0.0.0.0/0")
		}
		rule, err := sdkSbercloud.NewNetworkingSecgroupRule(ctx,
			input.ToResName(fmt.Sprintf("%s-secgroup-%s", name, p.Label)), args, resOpts...)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to open %s on vm %q", p.Spec(), name)
		}
		deps = append(deps, rule)
	}
	return group.ID().ToStringOutput(), deps, nil
}

// floatingIP creates the public address. Always, on this driver: Validate refuses
// `publicIP: false` because SC deploys over SSH from outside the cloud.
func (cloudruDriver) floatingIP(ctx *sdk.Context, input api.ResourceInput, hostName, region string, resOpts []sdk.ResourceOption) (*sdkSbercloud.VpcEip, error) {
	eip, err := sdkSbercloud.NewVpcEip(ctx, input.ToResName(hostName+"-eip"), &sdkSbercloud.VpcEipArgs{
		Region:   sdk.StringPtr(region),
		Publicip: sdkSbercloud.VpcEipPublicipArgs{Type: sdk.StringPtr(cloudruEipType)},
		Bandwidth: sdkSbercloud.VpcEipBandwidthArgs{
			ShareType: sdk.String("PER"),
			// Mandatory when shareType is PER, and NOT covered by the provider's
			// autonaming — that only reaches a resource's own top-level `name`.
			Name:       sdk.StringPtr(fmt.Sprintf("%s-bandwidth", hostName)),
			Size:       sdk.IntPtr(cloudruEipBandwidthMbit),
			ChargeMode: sdk.StringPtr("traffic"),
		},
	}, resOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create public address for host %q", hostName)
	}
	return eip, nil
}

// gibibytes parses one of SC's DSL quantities ("4Gi", "60Gi") into a whole number of
// GiB, which is the unit both Cloud.ru arguments this driver sets are expressed in.
//
// Refuses a fractional value rather than rounding: rounding a flavour filter down
// matches nothing and reads as "the region has no such machine", and rounding a disk
// size silently gives the operator a disk they did not ask for.
func gibibytes(quantity, field string) (int, error) {
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

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
